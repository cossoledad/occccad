#include <Standard_Version.hxx>

#include <internal/occt_kernel.hpp>
#include <occccad/assembly/solver.hpp>
#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>

#include <grpcpp/grpcpp.h>
#include <occccad/geometry/sketch/sketch_solver.h>
#include <occccad/worker/v1/geometry_worker.grpc.pb.h>
#include <spdlog/sinks/rotating_file_sink.h>
#include <spdlog/sinks/stdout_color_sinks.h>
#include <spdlog/spdlog.h>

#include <cctype>
#include <chrono>
#include <cstdint>
#include <cstdlib>
#include <exception>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <iterator>
#include <memory>
#include <mutex>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <vector>

namespace worker_api = occccad::worker::v1;
namespace sketch_api = occccad::geometry::sketch;
namespace assembly_api = occccad::assembly;

namespace {

constexpr std::uintmax_t kMaximumExchangeBytes = 512ULL * 1024ULL * 1024ULL;
constexpr std::size_t kLogFileBytes = 10ULL * 1024ULL * 1024ULL;
constexpr std::size_t kLogFileCount = 5U;

spdlog::level::level_enum configured_log_level() {
    const char* configured = std::getenv("OCCCCAD_LOG_LEVEL");
    const std::string value = configured != nullptr ? configured : "info";
    if (value == "trace")
        return spdlog::level::trace;
    if (value == "debug")
        return spdlog::level::debug;
    if (value == "warn" || value == "warning")
        return spdlog::level::warn;
    if (value == "error")
        return spdlog::level::err;
    if (value == "critical")
        return spdlog::level::critical;
    return spdlog::level::info;
}

std::filesystem::path configure_logging(const std::string& address) {
    const char* configured_directory = std::getenv("OCCCCAD_LOG_DIR");
    const auto directory = std::filesystem::absolute(
        configured_directory != nullptr ? configured_directory : "./logs");
    std::filesystem::create_directories(directory);
    std::string worker_name = address;
    for (char& character : worker_name) {
        if (!std::isalnum(static_cast<unsigned char>(character)))
            character = '-';
    }
    const auto log_file = directory / ("occccad-geometry-" + worker_name + ".log");
    auto console = std::make_shared<spdlog::sinks::stdout_color_sink_mt>();
    auto rotating_file = std::make_shared<spdlog::sinks::rotating_file_sink_mt>(
        log_file.string(), kLogFileBytes, kLogFileCount);
    std::vector<spdlog::sink_ptr> sinks{console, rotating_file};
    auto logger =
        std::make_shared<spdlog::logger>("occccad-geometry-worker", sinks.begin(), sinks.end());
    logger->set_level(configured_log_level());
    logger->set_pattern("%Y-%m-%dT%H:%M:%S.%e%z level=%l service=geometry-worker %v");
    logger->flush_on(spdlog::level::warn);
    spdlog::set_default_logger(logger);
    spdlog::flush_every(std::chrono::seconds(3));
    return log_file;
}

std::filesystem::path artifact_root() {
    const char* configured = std::getenv("OCCCCAD_DATA_DIR");
    return std::filesystem::absolute(configured != nullptr ? configured : "./data")
        .lexically_normal();
}

std::filesystem::path artifact_path(const std::string& key, const bool create_parent = false) {
    const std::filesystem::path relative(key);
    if (key.empty() || relative.is_absolute())
        throw std::invalid_argument("artifact object_key must be a relative path");
    const auto clean = relative.lexically_normal();
    if (clean.empty() || *clean.begin() == "..")
        throw std::invalid_argument("artifact object_key escapes the storage root");
    const auto result = (artifact_root() / clean).lexically_normal();
    const auto relative_to_root = result.lexically_relative(artifact_root());
    if (relative_to_root.empty() || *relative_to_root.begin() == "..")
        throw std::invalid_argument("artifact object_key escapes the storage root");
    if (create_parent)
        std::filesystem::create_directories(result.parent_path());
    return result;
}

std::filesystem::path validate_artifact(const worker_api::ArtifactReference& reference) {
    if (reference.backend() != "LOCAL")
        throw std::invalid_argument("only LOCAL artifacts are available in the current deployment");
    const auto path = artifact_path(reference.object_key());
    const auto size = std::filesystem::file_size(path);
    if (size == 0U || size > kMaximumExchangeBytes) {
        throw std::invalid_argument("artifact size is outside the worker exchange limit");
    }
    return path;
}

std::vector<uint8_t> read_artifact(const worker_api::ArtifactReference& reference) {
    const auto path = validate_artifact(reference);
    std::ifstream stream(path, std::ios::binary);
    if (!stream)
        throw std::runtime_error("cannot open artifact input");
    return {(std::istreambuf_iterator<char>(stream)), std::istreambuf_iterator<char>()};
}

void write_artifact(const std::string& key, const std::vector<uint8_t>& data,
                    const std::string& content_type, worker_api::ArtifactReference* output) {
    if (data.empty() || data.size() > kMaximumExchangeBytes)
        throw std::invalid_argument("artifact output is outside the worker exchange limit");
    const auto path = artifact_path(key, true);
    const auto temporary = path.string() + ".tmp";
    {
        std::ofstream stream(temporary, std::ios::binary | std::ios::trunc);
        stream.write(reinterpret_cast<const char*>(data.data()),
                     static_cast<std::streamsize>(data.size()));
        if (!stream)
            throw std::runtime_error("cannot write artifact output");
    }
    std::filesystem::rename(temporary, path);
    output->set_backend("LOCAL");
    output->set_object_key(key);
    output->set_size_bytes(static_cast<uint64_t>(data.size()));
    output->set_content_type(content_type);
}

std::string exchange_format(std::string value) {
    for (char& character : value)
        character = static_cast<char>(std::toupper(static_cast<unsigned char>(character)));
    if (value != "STEP" && value != "BREP")
        throw std::invalid_argument("exchange format must be STEP or BREP");
    return value;
}

void fill_bbox(const occccad::kernel::BoundingBox& source, worker_api::BoundingBox* destination) {
    destination->set_min_x(source.min.x);
    destination->set_min_y(source.min.y);
    destination->set_min_z(source.min.z);
    destination->set_max_x(source.max.x);
    destination->set_max_y(source.max.y);
    destination->set_max_z(source.max.z);
}

void fill_properties(
    const std::vector<occccad::kernel::TopologyProperty>& source,
    google::protobuf::RepeatedPtrField<worker_api::TopologyProperty>* destination) {
    for (const auto& property : source) {
        auto* output = destination->Add();
        output->set_name(property.name);
        switch (property.kind) {
            case occccad::kernel::TopologyProperty::Kind::NUMBER:
                output->set_number_value(property.number_value);
                break;
            case occccad::kernel::TopologyProperty::Kind::INTEGER:
                output->set_integer_value(property.integer_value);
                break;
            case occccad::kernel::TopologyProperty::Kind::BOOLEAN:
                output->set_bool_value(property.bool_value);
                break;
            case occccad::kernel::TopologyProperty::Kind::TEXT:
                output->set_text_value(property.text_value);
                break;
            case occccad::kernel::TopologyProperty::Kind::VECTOR: {
                auto* vector = output->mutable_vector_value();
                vector->set_x(property.vector_value.x);
                vector->set_y(property.vector_value.y);
                vector->set_z(property.vector_value.z);
                break;
            }
        }
    }
}

std::string metadata_value(grpc::ServerContext* context, const std::string& key) {
    const auto iterator = context->client_metadata().find(key);
    if (iterator == context->client_metadata().end())
        return {};
    return {iterator->second.data(), iterator->second.length()};
}

void log_rpc(const char* operation, const std::string& request_id, const std::string& traceparent,
             const char* status, const std::chrono::steady_clock::time_point started) {
    const auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(
                              std::chrono::steady_clock::now() - started)
                              .count();
    spdlog::info("rpc operation={} request_id={} traceparent={} status={} duration_ms={}",
                 operation, request_id, traceparent, status, duration);
}

sketch_api::GeometryRef read_sketch_reference(const worker_api::SketchGeometryRef& input) {
    sketch_api::GeometryTarget target;
    if (input.target() == "ENTITY")
        target = sketch_api::GeometryTarget::entity;
    else if (input.target() == "SKETCH_ORIGIN")
        target = sketch_api::GeometryTarget::sketch_origin;
    else if (input.target() == "SKETCH_X_AXIS")
        target = sketch_api::GeometryTarget::sketch_x_axis;
    else if (input.target() == "SKETCH_Y_AXIS")
        target = sketch_api::GeometryTarget::sketch_y_axis;
    else
        throw std::invalid_argument("unknown sketch reference target");
    sketch_api::SubElement sub_element;
    if (input.sub_element() == "WHOLE")
        sub_element = sketch_api::SubElement::whole;
    else if (input.sub_element() == "POINT")
        sub_element = sketch_api::SubElement::point;
    else if (input.sub_element() == "START")
        sub_element = sketch_api::SubElement::start;
    else if (input.sub_element() == "END")
        sub_element = sketch_api::SubElement::end;
    else if (input.sub_element() == "CENTER")
        sub_element = sketch_api::SubElement::center;
    else if (input.sub_element() == "DIRECTION")
        sub_element = sketch_api::SubElement::direction;
    else if (input.sub_element() == "CONTROL")
        sub_element = sketch_api::SubElement::control;
    else
        throw std::invalid_argument("unknown sketch sub-element");
    return {target, input.entity_id(), sub_element, input.control_point_index()};
}

sketch_api::SketchModel read_sketch(const worker_api::SketchModel& input) {
    if (input.schema_version() != 1U)
        throw std::invalid_argument("unsupported sketch schema version");
    sketch_api::SketchModel model;
    for (const auto& point : input.points()) {
        model.points.push_back({point.id(),
                                {point.point().x(), point.point().y()},
                                point.role() == "PROFILE" ? sketch_api::EntityRole::profile
                                                          : sketch_api::EntityRole::construction});
    }
    for (const auto& line : input.lines()) {
        model.lines.push_back({line.id(),
                               {line.start().x(), line.start().y()},
                               {line.end().x(), line.end().y()},
                               line.role() == "CONSTRUCTION" ? sketch_api::EntityRole::construction
                                                             : sketch_api::EntityRole::profile});
    }
    for (const auto& circle : input.circles()) {
        model.circles.push_back({circle.id(),
                                 {circle.center().x(), circle.center().y()},
                                 circle.radius(),
                                 circle.role() == "CONSTRUCTION"
                                     ? sketch_api::EntityRole::construction
                                     : sketch_api::EntityRole::profile});
    }
    for (const auto& arc : input.arcs()) {
        model.arcs.push_back({arc.id(),
                              {arc.center().x(), arc.center().y()},
                              arc.radius(),
                              arc.start_angle(),
                              arc.end_angle(),
                              arc.role() == "CONSTRUCTION" ? sketch_api::EntityRole::construction
                                                           : sketch_api::EntityRole::profile});
    }
    for (const auto& spline : input.splines()) {
        sketch_api::SplineEntity output{spline.id(),
                                        {},
                                        spline.degree(),
                                        spline.closed(),
                                        spline.role() == "CONSTRUCTION"
                                            ? sketch_api::EntityRole::construction
                                            : sketch_api::EntityRole::profile};
        for (const auto& point : spline.control_points())
            output.control_points.push_back({point.x(), point.y()});
        model.splines.push_back(std::move(output));
    }
    for (const auto& constraint : input.constraints()) {
        sketch_api::ConstraintKind kind;
        if (constraint.kind() == "COINCIDENT")
            kind = sketch_api::ConstraintKind::coincident;
        else if (constraint.kind() == "PARALLEL")
            kind = sketch_api::ConstraintKind::parallel;
        else if (constraint.kind() == "FIXED_POINT")
            kind = sketch_api::ConstraintKind::fixed_point;
        else if (constraint.kind() == "FIXED")
            kind = sketch_api::ConstraintKind::fixed;
        else if (constraint.kind() == "HORIZONTAL")
            kind = sketch_api::ConstraintKind::horizontal;
        else if (constraint.kind() == "VERTICAL")
            kind = sketch_api::ConstraintKind::vertical;
        else if (constraint.kind() == "PERPENDICULAR")
            kind = sketch_api::ConstraintKind::perpendicular;
        else if (constraint.kind() == "TANGENT")
            kind = sketch_api::ConstraintKind::tangent;
        else if (constraint.kind() == "EQUAL")
            kind = sketch_api::ConstraintKind::equal;
        else if (constraint.kind() == "DISTANCE")
            kind = sketch_api::ConstraintKind::distance;
        else if (constraint.kind() == "LENGTH")
            kind = sketch_api::ConstraintKind::length;
        else if (constraint.kind() == "RADIUS")
            kind = sketch_api::ConstraintKind::radius;
        else if (constraint.kind() == "DIAMETER")
            kind = sketch_api::ConstraintKind::diameter;
        else if (constraint.kind() == "ANGLE")
            kind = sketch_api::ConstraintKind::angle;
        else if (constraint.kind() == "CONCENTRIC")
            kind = sketch_api::ConstraintKind::concentric;
        else if (constraint.kind() == "POINT_ON_OBJECT")
            kind = sketch_api::ConstraintKind::point_on_object;
        else if (constraint.kind() == "MIDPOINT")
            kind = sketch_api::ConstraintKind::midpoint;
        else if (constraint.kind() == "SYMMETRY")
            kind = sketch_api::ConstraintKind::symmetry;
        else
            throw std::invalid_argument("unknown sketch constraint kind");
        sketch_api::SketchConstraint output{
            constraint.id(),
            kind,
            {},
            {constraint.fixed_point().x(), constraint.fixed_point().y()},
            constraint.value(),
            constraint.unit(),
            constraint.internal()};
        for (const auto& reference : constraint.references()) {
            output.references.push_back(read_sketch_reference(reference));
        }
        model.constraints.push_back(std::move(output));
    }
    return model;
}

const char* solve_status(sketch_api::SolveStatus status) {
    switch (status) {
        case sketch_api::SolveStatus::solved:
            return "SOLVED";
        case sketch_api::SolveStatus::under_constrained:
            return "UNDER_CONSTRAINED";
        case sketch_api::SolveStatus::invalid_model:
            return "INVALID_MODEL";
        case sketch_api::SolveStatus::redundant:
            return "REDUNDANT";
        case sketch_api::SolveStatus::conflicting:
            return "CONFLICTING";
        case sketch_api::SolveStatus::failed:
            return "FAILED";
    }
    return "FAILED";
}

void write_solved_sketch(const sketch_api::SolveResult& result, worker_api::SketchModel* output) {
    output->set_schema_version(1U);
    for (const auto& point : result.points) {
        auto* value = output->add_points();
        value->set_id(point.id);
        value->set_role(point.role == sketch_api::EntityRole::profile ? "PROFILE" : "CONSTRUCTION");
        value->mutable_point()->set_x(point.point.x);
        value->mutable_point()->set_y(point.point.y);
    }
    for (const auto& line : result.lines) {
        auto* value = output->add_lines();
        value->set_id(line.id);
        value->set_role(line.role == sketch_api::EntityRole::profile ? "PROFILE" : "CONSTRUCTION");
        value->mutable_start()->set_x(line.start.x);
        value->mutable_start()->set_y(line.start.y);
        value->mutable_end()->set_x(line.end.x);
        value->mutable_end()->set_y(line.end.y);
    }
    for (const auto& circle : result.circles) {
        auto* value = output->add_circles();
        value->set_id(circle.id);
        value->set_role(circle.role == sketch_api::EntityRole::profile ? "PROFILE"
                                                                       : "CONSTRUCTION");
        value->mutable_center()->set_x(circle.center.x);
        value->mutable_center()->set_y(circle.center.y);
        value->set_radius(circle.radius);
    }
    for (const auto& arc : result.arcs) {
        auto* value = output->add_arcs();
        value->set_id(arc.id);
        value->set_role(arc.role == sketch_api::EntityRole::profile ? "PROFILE" : "CONSTRUCTION");
        value->mutable_center()->set_x(arc.center.x);
        value->mutable_center()->set_y(arc.center.y);
        value->set_radius(arc.radius);
        value->set_start_angle(arc.start_angle);
        value->set_end_angle(arc.end_angle);
    }
    for (const auto& spline : result.splines) {
        auto* value = output->add_splines();
        value->set_id(spline.id);
        value->set_role(spline.role == sketch_api::EntityRole::profile ? "PROFILE"
                                                                       : "CONSTRUCTION");
        value->set_degree(spline.degree);
        value->set_closed(spline.closed);
        for (const auto& point : spline.control_points) {
            auto* control = value->add_control_points();
            control->set_x(point.x);
            control->set_y(point.y);
        }
    }
}

class GeometryWorkerService final : public worker_api::GeometryWorker::Service {
public:
    grpc::Status Ping(grpc::ServerContext* /*context*/, const worker_api::PingRequest* /*request*/,
                      worker_api::PingResponse* response) override {
        std::lock_guard<std::mutex> lock(mutex_);
        response->set_worker_id("geometry-worker-local-1");
        response->set_occt_version(OCC_VERSION_COMPLETE);
        response->set_resident_geometry_count(static_cast<uint32_t>(kernel_.resident_count()));
        return grpc::Status::OK;
    }

    grpc::Status SolveSketch(grpc::ServerContext* context,
                             const worker_api::SolveSketchRequest* request,
                             worker_api::SolveSketchResponse* response) override {
        if (context->IsCancelled())
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        if (request->request_id().empty() || !request->has_sketch()) {
            return {grpc::StatusCode::INVALID_ARGUMENT, "request_id and sketch are required"};
        }
        try {
            const auto result = sketch_solver_->solve(read_sketch(request->sketch()));
            response->set_status(solve_status(result.status));
            response->set_degrees_of_freedom(result.degrees_of_freedom);
            response->set_diagnostic(result.diagnostic);
            for (const auto& id : result.conflicting_constraint_ids)
                response->add_conflicting_constraint_ids(id);
            for (const auto& id : result.redundant_constraint_ids)
                response->add_redundant_constraint_ids(id);
            write_solved_sketch(result, response->mutable_sketch());
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

    grpc::Status SolveAssembly(grpc::ServerContext* context,
                               const worker_api::SolveAssemblyRequest* request,
                               worker_api::SolveAssemblyResponse* response) override {
        if (context->IsCancelled())
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        if (request->request_id().empty())
            return {grpc::StatusCode::INVALID_ARGUMENT, "request_id is required"};
        const auto vec = [](const worker_api::Vec3& value) {
            return assembly_api::Vec3{value.x(), value.y(), value.z()};
        };
        const auto pose = [&](const worker_api::RigidPose& value) {
            assembly_api::Pose result;
            if (value.has_translation())
                result.translation = vec(value.translation());
            if (value.has_rotation())
                result.rotation = {value.rotation().x(), value.rotation().y(), value.rotation().z(),
                                   value.rotation().w()};
            return result;
        };
        assembly_api::Model model;
        for (const auto& input : request->bodies())
            model.bodies.push_back({input.id(), pose(input.initial_pose())});
        for (const auto& input : request->geometry()) {
            assembly_api::Geometry geometry;
            if (input.kind() == "POINT")
                geometry = assembly_api::PointGeometry{vec(input.origin())};
            else if (input.kind() == "AXIS")
                geometry = assembly_api::AxisGeometry{vec(input.origin()), vec(input.direction())};
            else if (input.kind() == "PLANE")
                geometry = assembly_api::PlaneGeometry{vec(input.origin()), vec(input.direction())};
            else if (input.kind() == "CYLINDER")
                geometry = assembly_api::CylinderGeometry{vec(input.origin()),
                                                          vec(input.direction()), input.radius()};
            else
                return {grpc::StatusCode::INVALID_ARGUMENT, "unknown assembly geometry kind"};
            model.geometry.push_back({input.id(), input.body_id(), geometry});
        }
        for (const auto& input : request->constraints()) {
            assembly_api::Constraint constraint;
            constraint.id = input.id();
            constraint.connection_id = input.connection_id();
            constraint.first = {input.first().body_id(), input.first().geometry_id()};
            if (input.has_second())
                constraint.second = assembly_api::GeometryRef{input.second().body_id(),
                                                              input.second().geometry_id()};
            constraint.value = input.value();
            if (input.kind() == "FIX")
                constraint.kind = assembly_api::ConstraintKind::Fix;
            else if (input.kind() == "RIGID")
                constraint.kind = assembly_api::ConstraintKind::Rigid;
            else if (input.kind() == "COINCIDENT")
                constraint.kind = assembly_api::ConstraintKind::Coincident;
            else if (input.kind() == "CONCENTRIC")
                constraint.kind = assembly_api::ConstraintKind::Concentric;
            else if (input.kind() == "ANGLE")
                constraint.kind = assembly_api::ConstraintKind::Angle;
            else if (input.kind() == "DISTANCE")
                constraint.kind = assembly_api::ConstraintKind::Distance;
            else
                return {grpc::StatusCode::INVALID_ARGUMENT, "unknown assembly constraint kind"};
            if (input.direction_relation() == "SAME")
                constraint.direction_relation = assembly_api::DirectionRelation::Same;
            else if (input.direction_relation() == "OPPOSITE")
                constraint.direction_relation = assembly_api::DirectionRelation::Opposite;
            if (input.distance_relation() == "ALONG_SECOND_NORMAL")
                constraint.distance_relation = assembly_api::DistanceRelation::AlongSecondNormal;
            else if (input.distance_relation() == "OPPOSITE_SECOND_NORMAL")
                constraint.distance_relation = assembly_api::DistanceRelation::OppositeSecondNormal;
            if (input.has_fixed_pose())
                constraint.fixed_pose = pose(input.fixed_pose());
            if (input.mode() == "MEASURED")
                constraint.mode = assembly_api::ConstraintMode::Measured;
            else if (input.mode() == "CONTROLLED")
                constraint.mode = assembly_api::ConstraintMode::Controlled;
            else if (input.mode() == "SUPPRESSED")
                constraint.mode = assembly_api::ConstraintMode::Suppressed;
            else if (!input.mode().empty() && input.mode() != "DRIVING")
                return {grpc::StatusCode::INVALID_ARGUMENT, "unknown assembly constraint mode"};
            model.constraints.push_back(std::move(constraint));
        }
        assembly_api::SolverOptions options;
        if (request->length_scale() > 0.0)
            options.length_scale = request->length_scale();
        if (request->angle_scale() > 0.0)
            options.angle_scale = request->angle_scale();
        if (request->rank_tolerance() > 0.0)
            options.rank_tolerance = request->rank_tolerance();
        options.affected_body_ids.assign(request->affected_body_ids().begin(),
                                         request->affected_body_ids().end());
        if (request->has_solve_intent()) {
            assembly_api::SolveIntent intent;
            intent.moving_body_ids.assign(request->solve_intent().moving_body_ids().begin(),
                                          request->solve_intent().moving_body_ids().end());
            intent.reference_body_ids.assign(request->solve_intent().reference_body_ids().begin(),
                                             request->solve_intent().reference_body_ids().end());
            if (request->solve_intent().preference_policy() == "MOVE_FIRST_MINIMIZE_REFERENCE")
                intent.policy = assembly_api::SolvePreferencePolicy::MoveFirstMinimizeReference;
            else if (!request->solve_intent().preference_policy().empty() &&
                     request->solve_intent().preference_policy() != "MINIMUM_TOTAL_CHANGE")
                return {grpc::StatusCode::INVALID_ARGUMENT,
                        "unknown assembly solve preference policy"};
            options.solve_intent = std::move(intent);
        }
        const auto result = assembly_solver_.solve(model, options);
        const char* status =
            result.status == assembly_api::SolveStatus::Converged       ? "CONVERGED"
            : result.status == assembly_api::SolveStatus::Unsatisfied   ? "UNSATISFIED"
            : result.status == assembly_api::SolveStatus::MaxIterations ? "MAX_ITERATIONS"
            : result.status == assembly_api::SolveStatus::InvalidModel  ? "INVALID_MODEL"
                                                                        : "NUMERICAL_FAILURE";
        response->set_status(status);
        const char* classification =
            result.classification == assembly_api::SolveClassification::SolvedFully ? "SOLVED_FULLY"
            : result.classification == assembly_api::SolveClassification::SolvedUnderConstrained
                ? "SOLVED_UNDER_CONSTRAINED"
            : result.classification == assembly_api::SolveClassification::Redundant ? "REDUNDANT"
            : result.classification == assembly_api::SolveClassification::Inconsistent
                ? "INCONSISTENT"
            : result.classification == assembly_api::SolveClassification::InvalidModel
                ? "INVALID_MODEL"
                : "NON_CONVERGENT";
        response->set_classification(classification);
        response->set_iterations(result.iterations);
        response->set_normalized_residual(result.normalized_residual);
        response->set_diagnostic(result.diagnostic);
        for (const auto& body : result.bodies) {
            auto* output = response->add_bodies();
            output->set_id(body.id);
            auto* translation = output->mutable_pose()->mutable_translation();
            translation->set_x(body.pose.translation.x);
            translation->set_y(body.pose.translation.y);
            translation->set_z(body.pose.translation.z);
            auto* rotation = output->mutable_pose()->mutable_rotation();
            rotation->set_x(body.pose.rotation.x);
            rotation->set_y(body.pose.rotation.y);
            rotation->set_z(body.pose.rotation.z);
            rotation->set_w(body.pose.rotation.w);
        }
        for (const auto& residual : result.residuals) {
            auto* output = response->add_residuals();
            output->set_constraint_id(residual.constraint_id);
            output->set_normalized_norm(residual.normalized_norm);
        }
        for (const auto& residual : result.equation_residuals) {
            auto* output = response->add_equation_residuals();
            output->set_equation_id(residual.equation_id);
            output->set_connection_id(residual.connection_id);
            output->set_constraint_id(residual.constraint_id);
            output->set_equation_index(residual.equation_index);
            output->set_normalized_value(residual.normalized_value);
        }
        for (const auto& component : result.components) {
            auto* output = response->add_components();
            output->set_component_id(component.component_id);
            for (const auto& id : component.body_ids)
                output->add_body_ids(id);
            output->set_tangent_variable_count(component.tangent_variable_count);
            output->set_jacobian_rank(component.jacobian_rank);
            output->set_relative_dof(component.relative_dof);
            output->set_gauge_dof(component.gauge_dof);
            output->set_solved(component.solved);
        }
        for (const auto& id : result.redundant_constraint_ids)
            response->add_redundant_constraint_ids(id);
        for (const auto& id : result.conflicting_constraint_ids)
            response->add_conflicting_constraint_ids(id);
        for (const auto& diagnostic : result.diagnostics) {
            auto* output = response->add_diagnostics();
            output->set_code(diagnostic.code);
            output->set_component_id(diagnostic.component_id);
            for (const auto& id : diagnostic.body_ids)
                output->add_body_ids(id);
            for (const auto& id : diagnostic.constraint_ids)
                output->add_constraint_ids(id);
            output->set_detail(diagnostic.detail);
        }
        return grpc::Status::OK;
    }

    grpc::Status EvaluatePart(grpc::ServerContext* context,
                              const worker_api::EvaluatePartRequest* request,
                              worker_api::EvaluatePartResponse* response) override {
        const auto started = std::chrono::steady_clock::now();
        const auto traceparent = metadata_value(context, "traceparent");
        if (context->IsCancelled()) {
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        }
        if (request->request_id().empty() || request->geometry_key().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT, "request_id and geometry_key are required"};
        }
        if (!request->has_rectangular_pad() && request->rectangular_pads().empty() &&
            request->profile_pads().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT, "feature chain requires at least one pad"};
        }

        std::lock_guard<std::mutex> lock(mutex_);
        const bool external_outputs =
            !request->brep_output_key().empty() || !request->glb_output_key().empty();
        const auto cached = cache_.find(request->geometry_key());
        if (!external_outputs && cached != cache_.end()) {
            response->CopyFrom(cached->second);
            response->set_cache_hit(true);
            log_rpc("EvaluatePart", request->request_id(), traceparent, "CACHE_HIT", started);
            return grpc::Status::OK;
        }

        try {
            std::vector<occccad::kernel::RectangularPadSpec> specs;
            const auto append_spec = [&specs](const worker_api::RectangularPadSpec& input) {
                if (input.units() != "mm") {
                    throw std::invalid_argument("rectangular pads must use mm");
                }
                specs.push_back({
                    input.origin_x(),
                    input.origin_y(),
                    input.width(),
                    input.height(),
                    input.pad_length(),
                    input.plane().empty() ? "XY" : input.plane(),
                });
            };
            for (const auto& input : request->rectangular_pads())
                append_spec(input);
            if (specs.empty() && request->has_rectangular_pad()) {
                append_spec(request->rectangular_pad());
            }
            std::vector<uint8_t> base_brep(request->base_brep_data().begin(),
                                           request->base_brep_data().end());
            if (request->has_base_brep_artifact())
                base_brep = read_artifact(request->base_brep_artifact());
            std::vector<occccad::kernel::ProfilePadSpec> profile_specs;
            for (const auto& input : request->profile_pads()) {
                if (input.units() != "mm")
                    throw std::invalid_argument("profile pads must use mm");
                occccad::kernel::ProfilePadSpec pad;
                pad.pad_length = input.pad_length();
                pad.plane = input.plane().empty() ? "XY" : input.plane();
                pad.body_operation =
                    input.body_operation().empty() ? "ADD" : input.body_operation();
                pad.generator = input.generator().empty() ? "LINEAR_EXTRUDE" : input.generator();
                pad.revolve_angle = input.revolve_angle();
                pad.axis_start = {input.axis_start().x(), input.axis_start().y()};
                pad.axis_end = {input.axis_end().x(), input.axis_end().y()};
                pad.reversed = input.reversed();
                pad.plane_origin = {input.plane_origin().x(), input.plane_origin().y(),
                                    input.plane_origin().z()};
                pad.plane_normal = {input.plane_normal().x(), input.plane_normal().y(),
                                    input.plane_normal().z()};
                pad.plane_u_direction = {input.plane_u_direction().x(),
                                         input.plane_u_direction().y(),
                                         input.plane_u_direction().z()};
                const auto read_loop = [](const worker_api::ProfileLoop& source) {
                    occccad::kernel::ProfileLoopSpec loop;
                    loop.id = source.id();
                    for (const auto& curve : source.curves()) {
                        occccad::kernel::ProfileCurveSpec value;
                        value.entity_id = curve.entity_id();
                        value.reversed = curve.reversed();
                        value.kind = curve.kind();
                        value.start = {curve.start().x(), curve.start().y()};
                        value.end = {curve.end().x(), curve.end().y()};
                        value.center = {curve.center().x(), curve.center().y()};
                        value.radius = curve.radius();
                        value.start_angle = curve.start_angle();
                        value.end_angle = curve.end_angle();
                        value.degree = curve.degree();
                        value.closed = curve.closed();
                        for (const auto& point : curve.control_points())
                            value.control_points.push_back({point.x(), point.y()});
                        loop.curves.push_back(std::move(value));
                    }
                    return loop;
                };
                for (const auto& region_input : input.regions()) {
                    occccad::kernel::ProfileRegionSpec region;
                    region.id = region_input.id();
                    region.outer = read_loop(region_input.outer());
                    for (const auto& hole : region_input.holes())
                        region.holes.push_back(read_loop(hole));
                    pad.regions.push_back(std::move(region));
                }
                profile_specs.push_back(std::move(pad));
            }
            const auto geometry_id = profile_specs.empty()
                                         ? kernel_.evaluateRectangularPads(specs, base_brep)
                                         : kernel_.evaluateProfilePads(profile_specs, base_brep);
            fill_evaluation(request->geometry_key(), geometry_id, request->linear_deflection(),
                            request->angular_deflection(), response, request->brep_output_key(),
                            request->glb_output_key());

            if (!external_outputs)
                cache_.insert_or_assign(request->geometry_key(), *response);
            log_rpc("EvaluatePart", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

    grpc::Status InspectExchange(grpc::ServerContext* context,
                                 const worker_api::InspectExchangeRequest* request,
                                 worker_api::InspectExchangeResponse* response) override {
        if (context->IsCancelled())
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        try {
            const auto format = exchange_format(request->format());
            if (!request->has_source())
                throw std::invalid_argument("exchange source artifact is required");
            std::lock_guard<std::mutex> lock(mutex_);
            const auto source = validate_artifact(request->source());
            const uint32_t count =
                format == "STEP" ? kernel_.inspectStepRootCount(source.string()) : 1U;
            response->set_document_type(count > 1U ? "PRODUCT" : "PART");
            for (uint32_t index = 1; index <= count; ++index) {
                auto* component = response->add_components();
                component->set_source_index(index);
                component->set_name(count > 1U ? "Component " + std::to_string(index) : "Part");
            }
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

    grpc::Status ImportExchange(grpc::ServerContext* context,
                                const worker_api::ImportExchangeRequest* request,
                                worker_api::EvaluatePartResponse* response) override {
        const auto started = std::chrono::steady_clock::now();
        const auto traceparent = metadata_value(context, "traceparent");
        if (context->IsCancelled()) {
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        }
        if (request->request_id().empty() || request->geometry_key().empty() ||
            !request->has_source() || request->brep_output_key().empty() ||
            request->glb_output_key().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id, geometry_key, source, and output keys are required"};
        }
        std::lock_guard<std::mutex> lock(mutex_);
        try {
            const auto format = exchange_format(request->format());
            const auto source = validate_artifact(request->source());
            occccad::kernel::GeometryId geometry_id;
            if (format == "STEP") {
                geometry_id = kernel_.loadStepRoot(source.string(), request->source_index());
            } else {
                geometry_id = kernel_.loadBrepr(read_artifact(request->source()));
            }
            fill_evaluation(request->geometry_key(), geometry_id, request->linear_deflection(),
                            request->angular_deflection(), response, request->brep_output_key(),
                            request->glb_output_key());
            log_rpc("ImportExchange", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

    grpc::Status GetTopology(grpc::ServerContext* context,
                             const worker_api::GetTopologyRequest* request,
                             worker_api::GetTopologyResponse* response) override {
        const auto started = std::chrono::steady_clock::now();
        if (context->IsCancelled())
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        if (request->geometry_id().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT, "geometry_id is required"};
        }
        std::lock_guard<std::mutex> lock(mutex_);
        try {
            std::string geometry_id = request->geometry_id();
            if (!kernel_.is_loaded(geometry_id)) {
                if (request->brep_data().empty() && !request->has_brep_artifact()) {
                    return {grpc::StatusCode::NOT_FOUND,
                            "geometry is not resident and no B-Rep artifact was supplied"};
                }
                std::vector<uint8_t> brep(request->brep_data().begin(), request->brep_data().end());
                if (request->has_brep_artifact())
                    brep = read_artifact(request->brep_artifact());
                geometry_id = kernel_.loadBrepr(brep);
            }
            const bool topology_cache_hit =
                topology_cached_.find(geometry_id) != topology_cached_.end();
            const auto& topology = kernel_.getTopology(geometry_id);
            topology_cached_.insert(geometry_id);
            response->set_face_count(topology.face_count);
            response->set_edge_count(topology.edge_count);
            response->set_vertex_count(topology.vertex_count);
            response->set_solid_count(topology.solid_count);
            for (const auto& face : topology.faces) {
                if (!request->topology_type().empty() && request->topology_type() != "FACE")
                    continue;
                if (request->local_id() != 0 && request->local_id() != face.local_id)
                    continue;
                auto* output = response->add_faces();
                output->set_local_id(face.local_id);
                output->set_surface_type(face.surface_type);
                fill_bbox(face.bbox, output->mutable_bbox());
                fill_properties(face.properties, output->mutable_properties());
            }
            for (const auto& edge : topology.edges) {
                if (!request->topology_type().empty() && request->topology_type() != "EDGE")
                    continue;
                if (request->local_id() != 0 && request->local_id() != edge.local_id)
                    continue;
                auto* output = response->add_edges();
                output->set_local_id(edge.local_id);
                output->set_curve_type(edge.curve_type);
                fill_bbox(edge.bbox, output->mutable_bbox());
                fill_properties(edge.properties, output->mutable_properties());
                for (const auto& point : edge.render_points) {
                    auto* output_point = output->add_render_points();
                    output_point->set_x(point.x);
                    output_point->set_y(point.y);
                    output_point->set_z(point.z);
                }
            }
            for (const auto& vertex : topology.vertices) {
                if (!request->topology_type().empty() && request->topology_type() != "VERTEX")
                    continue;
                if (request->local_id() != 0 && request->local_id() != vertex.local_id)
                    continue;
                auto* output = response->add_vertices();
                output->set_local_id(vertex.local_id);
                output->mutable_point()->set_x(vertex.point.x);
                output->mutable_point()->set_y(vertex.point.y);
                output->mutable_point()->set_z(vertex.point.z);
                fill_properties(vertex.properties, output->mutable_properties());
            }
            const auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(
                                      std::chrono::steady_clock::now() - started)
                                      .count();
            spdlog::info(
                "topology query geometry_id={} kind={} local_id={} cache_hit={} duration_ms={}",
                geometry_id, request->topology_type(), request->local_id(), topology_cache_hit,
                duration);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            spdlog::warn("topology query rejected geometry_id={} error={}", request->geometry_id(),
                         error.what());
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            spdlog::error("topology query failed geometry_id={} error={}", request->geometry_id(),
                          error.what());
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

    grpc::Status ExportExchange(grpc::ServerContext* context,
                                const worker_api::ExportExchangeRequest* request,
                                worker_api::ExportExchangeResponse* response) override {
        const auto started = std::chrono::steady_clock::now();
        const auto traceparent = metadata_value(context, "traceparent");
        if (context->IsCancelled()) {
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        }
        if (request->request_id().empty() || request->components_size() == 0 ||
            request->output_key().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id, components, and output_key are required"};
        }
        std::lock_guard<std::mutex> lock(mutex_);
        try {
            const auto format = exchange_format(request->format());
            std::vector<occccad::kernel::PlacedGeometry> components;
            components.reserve(static_cast<size_t>(request->components_size()));
            for (const auto& component : request->components()) {
                if (!component.has_brep())
                    throw std::invalid_argument("each export component requires a B-Rep artifact");
                const auto id = kernel_.loadBrepr(read_artifact(component.brep()));
                components.push_back({id,
                                      {component.translation().x(), component.translation().y(),
                                       component.translation().z()}});
            }
            std::vector<uint8_t> data;
            if (format == "STEP") {
                // One transferable root per occurrence is the current flat
                // Product exchange contract. Collapsing them into a compound
                // makes a subsequent import indistinguishable from a Part.
                data = kernel_.serializeStepComponents(components);
            } else {
                const auto geometry_id = components.size() == 1U &&
                                                 components.front().translation.x == 0.0 &&
                                                 components.front().translation.y == 0.0 &&
                                                 components.front().translation.z == 0.0
                                             ? components.front().geometry_id
                                             : kernel_.combine(components);
                data = kernel_.serializeBrepr(geometry_id);
            }
            write_artifact(request->output_key(), data,
                           format == "STEP" ? "model/step" : "application/vnd.opencascade.brep",
                           response->mutable_result());
            log_rpc("ExportExchange", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

private:
    assembly_api::Solver assembly_solver_;
    void fill_evaluation(const std::string& geometry_key,
                         const occccad::kernel::GeometryId& geometry_id,
                         const double requested_linear_deflection,
                         const double requested_angular_deflection,
                         worker_api::EvaluatePartResponse* response,
                         const std::string& brep_output_key = {},
                         const std::string& glb_output_key = {}) {
        const auto bbox = kernel_.getBoundingBox(geometry_id);
        const auto& topology = kernel_.getTopology(geometry_id);
        topology_cached_.insert(geometry_id);
        const double linear_deflection =
            requested_linear_deflection > 0.0 ? requested_linear_deflection : 0.1;
        const double angular_deflection =
            requested_angular_deflection > 0.0 ? requested_angular_deflection : 0.5;
        const auto mesh = kernel_.tessellate(geometry_id, linear_deflection, angular_deflection);
        const auto brep = kernel_.serializeBrepr(geometry_id);
        const auto glb = occccad::kernel::make_glb(mesh);

        response->set_geometry_id(geometry_id);
        response->set_geometry_key(geometry_key);
        if (!brep_output_key.empty())
            write_artifact(brep_output_key, brep, "application/vnd.opencascade.brep",
                           response->mutable_brep_artifact());
        else
            response->set_brep_data(brep.data(), brep.size());
        if (!glb_output_key.empty())
            write_artifact(glb_output_key, glb, "model/gltf-binary",
                           response->mutable_glb_artifact());
        else
            response->set_glb_data(glb.data(), glb.size());
        fill_bbox(bbox, response->mutable_bbox());
        response->set_volume(kernel_.getVolume(geometry_id));
        response->set_cache_hit(false);
        response->set_occt_version(OCC_VERSION_COMPLETE);
        auto* summary = response->mutable_topology();
        summary->set_face_count(topology.face_count);
        summary->set_edge_count(topology.edge_count);
        summary->set_vertex_count(topology.vertex_count);
        summary->set_solid_count(topology.solid_count);
        auto* output_mesh = response->mutable_mesh();
        for (const auto& vertex : mesh.vertices) {
            auto* output_vertex = output_mesh->add_vertices();
            output_vertex->set_x(vertex.x);
            output_vertex->set_y(vertex.y);
            output_vertex->set_z(vertex.z);
        }
        for (const auto& triangle : mesh.triangles) {
            auto* output_triangle = output_mesh->add_triangles();
            output_triangle->set_v0(triangle.v0);
            output_triangle->set_v1(triangle.v1);
            output_triangle->set_v2(triangle.v2);
        }
        for (const uint32_t face_id : mesh.face_ids) {
            output_mesh->add_face_ids(face_id);
        }
        for (const auto& edge : mesh.edges) {
            auto* output_edge = output_mesh->add_edges();
            output_edge->set_local_id(edge.local_id);
            for (const auto& point : edge.points) {
                auto* output_point = output_edge->add_points();
                output_point->set_x(point.x);
                output_point->set_y(point.y);
                output_point->set_z(point.z);
            }
        }
        for (const auto& vertex : mesh.topology_vertices) {
            auto* output_vertex = output_mesh->add_topology_vertices();
            output_vertex->set_local_id(vertex.local_id);
            output_vertex->mutable_point()->set_x(vertex.point.x);
            output_vertex->mutable_point()->set_y(vertex.point.y);
            output_vertex->mutable_point()->set_z(vertex.point.z);
        }
    }

    std::mutex mutex_;
    occccad::kernel::OcctKernel kernel_;
    std::unique_ptr<occccad::geometry::sketch::SketchSolver> sketch_solver_ =
        occccad::geometry::sketch::make_plane_gcs_sketch_solver();
    std::unordered_map<std::string, worker_api::EvaluatePartResponse> cache_;
    std::unordered_set<std::string> topology_cached_;
};

}  // namespace

int main() {
    try {
        const char* configured_address = std::getenv("OCCCCAD_GEOMETRY_WORKER_LISTEN");
        const std::string address =
            configured_address != nullptr ? configured_address : "127.0.0.1:51001";
        const auto log_file = configure_logging(address);

        GeometryWorkerService service;
        grpc::ServerBuilder builder;
        builder.AddListeningPort(address, grpc::InsecureServerCredentials());
        builder.RegisterService(&service);
        std::unique_ptr<grpc::Server> server = builder.BuildAndStart();
        if (!server) {
            spdlog::critical("geometry worker failed to listen address={}", address);
            return EXIT_FAILURE;
        }
        spdlog::info("geometry worker ready address={} occt_version={} log_file={}", address,
                     OCC_VERSION_COMPLETE, log_file.string());
        server->Wait();
        spdlog::info("geometry worker stopped address={}", address);
        spdlog::shutdown();
        return EXIT_SUCCESS;
    } catch (const std::exception& error) {
        try {
            spdlog::critical("geometry worker startup failed error={}", error.what());
            spdlog::shutdown();
        } catch (...) {
            std::cerr << "Geometry Worker startup failed: " << error.what() << '\n';
        }
        return EXIT_FAILURE;
    }
}
