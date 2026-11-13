#if defined(__linux__)
#include <sys/resource.h>
#endif
#include <Standard_Version.hxx>
#include "naming_artifact.hpp"
#include "cancellation.hpp"

#include <internal/occt_kernel.hpp>
#include <occccad/assembly/solver.hpp>
#include <occccad/kernel/geometry_id.hpp>
#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>
#include <occccad/kernel/topology_naming.hpp>

#include <grpcpp/grpcpp.h>
#include <google/protobuf/struct.pb.h>
#include <google/protobuf/util/json_util.h>
#include <occccad/geometry/sketch/external_geometry_projector.h>
#include <occccad/geometry/sketch/sketch_solver.h>
#include <occccad/worker/v1/geometry_worker.grpc.pb.h>
#include <spdlog/sinks/rotating_file_sink.h>
#include <spdlog/sinks/stdout_color_sinks.h>
#include <spdlog/spdlog.h>

#include <atomic>
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
#include <OSD_ThreadPool.hxx>
#include <OSD_Parallel.hxx>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <vector>

namespace worker_api = occccad::worker::v1;
namespace sketch_api = occccad::geometry::sketch;
namespace assembly_api = occccad::assembly;

namespace {

std::uintmax_t maximum_exchange_bytes() {
    const char* configured = std::getenv("OCCCCAD_EXCHANGE_MAX_BYTES");
    if (configured != nullptr) {
        const std::string value(configured);
        if (!value.empty() && value.find_first_not_of("0123456789") == std::string::npos) {
            try { const auto bytes = std::stoull(value); if (bytes > 0 && bytes <= 9223372036854775807ULL) return bytes; }
            catch (const std::exception&) {}
        }
    }
    return 16ULL * 1024ULL * 1024ULL * 1024ULL;
}
// Same bounded transport budget as services/internal/geometryrpc/limits.go.
constexpr int kGeometryRpcMaxMessageBytes = 128 * 1024 * 1024;
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
        throw std::invalid_argument("worker requires a materialized LOCAL scratch reference");
    const auto path = artifact_path(reference.object_key());
    const auto size = std::filesystem::file_size(path);
    if (size == 0U || size > maximum_exchange_bytes()) {
        throw std::invalid_argument("artifact size is outside the worker exchange limit");
    }
    return path;
}

std::vector<uint8_t> read_artifact(const worker_api::ArtifactReference& reference) {
    const auto path = validate_artifact(reference);
    std::ifstream stream(path, std::ios::binary);
    if (!stream)
        throw std::runtime_error("cannot open artifact input");
    std::vector<uint8_t> data{std::istreambuf_iterator<char>(stream), std::istreambuf_iterator<char>()};
    if (data.size() != reference.size_bytes() ||
        occccad::kernel::make_geometry_id(data.data(), data.size()) != "sha256:" + reference.sha256())
        throw std::invalid_argument("artifact input integrity mismatch");
    return data;
}

void write_artifact(const std::string& key, const std::vector<uint8_t>& data,
                    const std::string& content_type, worker_api::ArtifactReference* output) {
    if (data.empty() || data.size() > maximum_exchange_bytes())
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
    const auto digest = occccad::kernel::make_geometry_id(data.data(), data.size());
    output->set_sha256(digest.rfind("sha256:", 0) == 0 ? digest.substr(7) : digest);
    output->set_size_bytes(static_cast<uint64_t>(data.size()));
    output->set_content_type(content_type);
}

void fill_vec3(const occccad::kernel::Vec3& source, worker_api::Vec3* target) {
    target->set_x(source.x);
    target->set_y(source.y);
    target->set_z(source.z);
}

void fill_semantic_ref(const occccad::kernel::SemanticTopologyRef& source,
                       worker_api::SemanticTopologyRef* target) {
    target->set_feature_id(source.feature_id);
    target->set_output_slot(source.output_slot);
    for (const auto& id : source.source_ids)
        target->add_source_ids(id);
}

worker_api::TopologyLineageKind lineage_kind(const occccad::kernel::TopologyLineageKind kind) {
    using Source = occccad::kernel::TopologyLineageKind;
    switch (kind) {
        case Source::generated:
            return worker_api::TOPOLOGY_LINEAGE_GENERATED;
        case Source::modified:
            return worker_api::TOPOLOGY_LINEAGE_MODIFIED;
        case Source::split:
            return worker_api::TOPOLOGY_LINEAGE_SPLIT;
        case Source::merged:
            return worker_api::TOPOLOGY_LINEAGE_MERGED;
        case Source::unchanged:
            return worker_api::TOPOLOGY_LINEAGE_UNCHANGED;
        default:
            return worker_api::TOPOLOGY_LINEAGE_UNSPECIFIED;
    }
}

worker_api::PersistentTopologyType persistent_topology_type(
    const occccad::kernel::PersistentTopologyType type) {
    using Source = occccad::kernel::PersistentTopologyType;
    switch (type) {
        case Source::face:
            return worker_api::PERSISTENT_TOPOLOGY_TYPE_FACE;
        case Source::edge:
            return worker_api::PERSISTENT_TOPOLOGY_TYPE_EDGE;
        case Source::vertex:
            return worker_api::PERSISTENT_TOPOLOGY_TYPE_VERTEX;
        default:
            return worker_api::PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED;
    }
}

void fill_selection_evidence(const occccad::kernel::SelectionEvidence& source,
                             worker_api::SelectionEvidence* target) {
    target->set_geometry_type(source.geometry_type);
    if (source.radius_mm) target->set_radius_mm(*source.radius_mm);
    if (source.minor_radius_mm) target->set_minor_radius_mm(*source.minor_radius_mm);
    if (source.half_angle_radians) target->set_half_angle_radians(*source.half_angle_radians);
    if (source.cone_leaf) target->set_cone_leaf(*source.cone_leaf);
    if (source.material_side) target->set_material_side(*source.material_side);
    if (source.x_direction) fill_vec3(*source.x_direction, target->mutable_x_direction());
    if (source.measure_si)
        target->set_measure_si(*source.measure_si);
    target->set_measure_dimension(source.measure_dimension);
    if (source.parameter_start)
        target->set_parameter_start(*source.parameter_start);
    if (source.parameter_end)
        target->set_parameter_end(*source.parameter_end);
    target->set_endpoint_role(source.endpoint_role);
    fill_vec3(source.centroid, target->mutable_centroid());
    fill_vec3(source.origin, target->mutable_origin());
    fill_vec3(source.direction, target->mutable_direction());
    for (const auto& adjacent : source.adjacent)
        fill_semantic_ref(adjacent, target->add_adjacent());
    target->set_evidence_digest(source.evidence_digest);
}

void fill_feature_result(const occccad::kernel::FeatureResult& source,
                         worker_api::FeatureResult* target) {
    target->set_feature_id(source.feature_id);
    target->set_body_id(source.body_id);
    target->set_input_feature_id(source.input_feature_id);
    target->set_profile_feature_id(source.profile_feature_id);
    target->set_result_geometry_id(source.result_geometry_id);
    target->set_topology_history_complete(source.topology_history_complete);
    if(!source.result_id.empty()) {
        target->mutable_result()->set_feature_id(source.feature_id);
        target->mutable_result()->set_result_id(source.result_id);
    }
    for (const auto& output : source.semantic_outputs) {
        auto* value = target->add_semantic_outputs();
        fill_semantic_ref(output.semantic_ref, value->mutable_semantic_ref());
        value->set_topology_type(persistent_topology_type(output.topology_type));
        value->set_local_id(output.local_id);
        fill_selection_evidence(output.evidence, value->mutable_evidence());
    }
    const auto& history = source.topology_history;
    auto* target_history = target->mutable_topology_history();
    target_history->set_schema_version(history.schema_version);
    target_history->set_feature_id(history.feature_id);
    target_history->set_input_geometry_id(history.input_geometry_id);
    target_history->set_result_geometry_id(history.result_geometry_id);
    target_history->set_evidence_digest(history.evidence_digest);
    target_history->set_policy_digest(history.policy_digest);
    for (const auto& lineage : history.lineage) {
        auto* value = target_history->add_lineage();
        for (const auto& ref : lineage.sources)
            fill_semantic_ref(ref, value->add_sources());
        fill_semantic_ref(lineage.result, value->mutable_result());
        value->set_kind(lineage_kind(lineage.kind));
        fill_selection_evidence(lineage.evidence, value->mutable_evidence());
    }
    for (const auto& deleted : history.deleted) {
        auto* value = target_history->add_deleted();
        fill_semantic_ref(deleted.source, value->mutable_source());
        value->set_reason(deleted.reason);
        fill_selection_evidence(deleted.evidence, value->mutable_evidence());
    }
    for (const auto& ambiguous : history.ambiguous) {
        auto* value = target_history->add_ambiguous();
        for (const auto& ref : ambiguous.sources)
            fill_semantic_ref(ref, value->add_sources());
        for (const auto& ref : ambiguous.candidates)
            fill_semantic_ref(ref, value->add_candidates());
        value->set_diagnostic_code(ambiguous.diagnostic_code);
    }
    for (const auto& diagnostic : source.diagnostics)
        target->add_diagnostics(diagnostic);
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
    for (const auto& e : input.ellipses()) {
        model.ellipses.push_back({e.id(),{e.center().x(),e.center().y()},e.major_radius(),e.minor_radius(),e.rotation(),e.role()=="CONSTRUCTION"?sketch_api::EntityRole::construction:sketch_api::EntityRole::profile});
    }
    for (const auto& e : input.elliptical_arcs()) {
        model.elliptical_arcs.push_back({e.id(),{e.center().x(),e.center().y()},e.major_radius(),e.minor_radius(),e.rotation(),e.start_angle(),e.end_angle(),e.role()=="CONSTRUCTION"?sketch_api::EntityRole::construction:sketch_api::EntityRole::profile});
    }
    for (const auto& spline : input.splines()) {
        sketch_api::SplineEntity output{};
        output.id=spline.id();output.degree=spline.degree();output.closed=spline.closed();
        output.role=spline.role()=="CONSTRUCTION"?sketch_api::EntityRole::construction:sketch_api::EntityRole::profile;
        for (const auto& point : spline.control_points())
            output.control_points.push_back({point.x(), point.y()});
        output.mode = spline.mode().empty() ? "FIT" : spline.mode();
        for (const auto& p : spline.poles()) output.poles.push_back({p.x(), p.y()});
        output.knots.assign(spline.knots().begin(), spline.knots().end());
        output.multiplicities.assign(spline.multiplicities().begin(), spline.multiplicities().end());
        output.weights.assign(spline.weights().begin(), spline.weights().end());
        output.periodic = spline.periodic();
        output.parameter_start = spline.parameter_start(); output.parameter_end = spline.parameter_end();
        model.splines.push_back(std::move(output));
    }
    for (const auto& constraint : input.constraints()) {
        sketch_api::ConstraintKind kind;
        if (constraint.kind() == "COINCIDENT")
            kind = sketch_api::ConstraintKind::coincident;
        else if (constraint.kind() == "SAME_SUPPORT")
            kind = sketch_api::ConstraintKind::same_support;
        else if (constraint.kind() == "MIRROR")
            kind = sketch_api::ConstraintKind::mirror;
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
        else if (constraint.kind() == "HORIZONTAL_DISTANCE")
            kind = sketch_api::ConstraintKind::horizontal_distance;
        else if (constraint.kind() == "VERTICAL_DISTANCE")
            kind = sketch_api::ConstraintKind::vertical_distance;
        else if (constraint.kind() == "COLLINEAR")
            kind = sketch_api::ConstraintKind::collinear;
        else if (constraint.kind() == "DISTANCE")
            kind = sketch_api::ConstraintKind::distance;
        else if (constraint.kind() == "LENGTH")
            kind = sketch_api::ConstraintKind::length;
        else if (constraint.kind() == "MAJOR_RADIUS")
            kind = sketch_api::ConstraintKind::major_radius;
        else if (constraint.kind() == "MINOR_RADIUS")
            kind = sketch_api::ConstraintKind::minor_radius;
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
            constraint.internal(),constraint.self_mirror_mode()};
        for (const auto& reference : constraint.references()) {
            output.references.push_back(read_sketch_reference(reference));
        }
        model.constraints.push_back(std::move(output));
    }
    if(input.drag_targets_size()>4096)throw std::invalid_argument("too many sketch drag targets");
    for(const auto& target:input.drag_targets()) {
        if(!target.has_reference()||!target.has_point())throw std::invalid_argument("drag target requires reference and point");
        model.drag_targets.push_back({read_sketch_reference(target.reference()),{target.point().x(),target.point().y()}});
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
    for (const auto& e : result.ellipses) {
        auto* value=output->add_ellipses(); value->set_id(e.id); value->set_role(e.role==sketch_api::EntityRole::construction?"CONSTRUCTION":"PROFILE");
        value->mutable_center()->set_x(e.center.x);value->mutable_center()->set_y(e.center.y);
        value->set_major_radius(e.major_radius);value->set_minor_radius(e.minor_radius);value->set_rotation(e.rotation);
    }
    for (const auto& e : result.elliptical_arcs) {
        auto* value=output->add_elliptical_arcs(); value->set_id(e.id); value->set_role(e.role==sketch_api::EntityRole::construction?"CONSTRUCTION":"PROFILE");
        value->mutable_center()->set_x(e.center.x);value->mutable_center()->set_y(e.center.y);
        value->set_major_radius(e.major_radius);value->set_minor_radius(e.minor_radius);value->set_rotation(e.rotation);
        value->set_start_angle(e.start_angle);value->set_end_angle(e.end_angle);
    }
    for (const auto& spline : result.splines) {
        auto* value = output->add_splines();
        value->set_id(spline.id);
        value->set_role(spline.role == sketch_api::EntityRole::profile ? "PROFILE"
                                                                       : "CONSTRUCTION");
        occccad::kernel::ProfileCurveSpec source;
        source.kind="SPLINE"; source.mode=spline.mode; source.degree=spline.degree; source.closed=spline.closed;
        for(const auto& p:spline.control_points) source.control_points.push_back({p.x,p.y});
        for(const auto& p:spline.poles) source.poles.push_back({p.x,p.y});
        source.knots=spline.knots; source.multiplicities=spline.multiplicities; source.weights=spline.weights;
        source.periodic=spline.periodic; source.parameter_start=spline.parameter_start; source.parameter_end=spline.parameter_end;
        const auto canonical=occccad::kernel::canonicalize_sketch_curve(source);
        value->set_degree(canonical.degree); value->set_closed(canonical.closed); value->set_mode(spline.mode);
        for(const auto& p:canonical.poles) {auto* pole=value->add_poles();pole->set_x(p.x);pole->set_y(p.y);}
        for(const auto k:canonical.knots) value->add_knots(k);
        for(const auto m:canonical.multiplicities) value->add_multiplicities(m);
        for(const auto w:canonical.weights) value->add_weights(w);
        value->set_periodic(canonical.periodic);value->set_parameter_start(canonical.parameter_start);value->set_parameter_end(canonical.parameter_end);
        for (const auto& point : spline.control_points) {
            auto* control = value->add_control_points();
            control->set_x(point.x);
            control->set_y(point.y);
        }
    }
}

occccad::kernel::ProfileCurveSpec exact_sketch_curve(const worker_api::ProfileCurve& c) {
    if (c.control_points_size()>512 || c.poles_size()>512 || c.knots_size()>1024 || c.multiplicities_size()>1024 || c.weights_size()>512 || c.degree()>16)
        throw std::invalid_argument("sketch curve exceeds resource limits");
    occccad::kernel::ProfileCurveSpec v;
    v.entity_id=c.entity_id();v.kind=c.kind();v.reversed=c.reversed();
    v.start={c.start().x(),c.start().y()};v.end={c.end().x(),c.end().y()};v.center={c.center().x(),c.center().y()};
    v.radius=c.radius();v.start_angle=c.start_angle();v.end_angle=c.end_angle();
    v.major_radius=c.major_radius();v.minor_radius=c.minor_radius();v.rotation=c.rotation();
    v.degree=c.degree();v.closed=c.closed();v.mode=c.mode().empty()?"FIT":c.mode();v.periodic=c.periodic();
    v.parameter_start=c.parameter_start();v.parameter_end=c.parameter_end();
    for (const auto& p:c.control_points()) v.control_points.push_back({p.x(),p.y()});
    for (const auto& p:c.poles()) v.poles.push_back({p.x(),p.y()});
    v.knots.assign(c.knots().begin(),c.knots().end());v.multiplicities.assign(c.multiplicities().begin(),c.multiplicities().end());v.weights.assign(c.weights().begin(),c.weights().end());
    return v;
}
void exact_sketch_curve_proto(const occccad::kernel::ProfileCurveSpec& v,worker_api::ProfileCurve* c) {
    c->set_entity_id(v.entity_id);c->set_kind(v.kind);c->set_reversed(v.reversed);
    c->mutable_start()->set_x(v.start.x);c->mutable_start()->set_y(v.start.y);c->mutable_end()->set_x(v.end.x);c->mutable_end()->set_y(v.end.y);c->mutable_center()->set_x(v.center.x);c->mutable_center()->set_y(v.center.y);
    c->set_radius(v.radius);c->set_start_angle(v.start_angle);c->set_end_angle(v.end_angle);c->set_major_radius(v.major_radius);c->set_minor_radius(v.minor_radius);c->set_rotation(v.rotation);
    c->set_degree(v.degree);c->set_closed(v.closed);c->set_mode(v.mode);c->set_periodic(v.periodic);c->set_parameter_start(v.parameter_start);c->set_parameter_end(v.parameter_end);
    for (const auto& p:v.control_points) {auto* point=c->add_control_points();point->set_x(p.x);point->set_y(p.y);}
    for (const auto& p:v.poles) {auto* point=c->add_poles();point->set_x(p.x);point->set_y(p.y);}
    for (double knot:v.knots)c->add_knots(knot);
    for (auto mult:v.multiplicities)c->add_multiplicities(mult);
    for (double weight:v.weights)c->add_weights(weight);
}

occccad::kernel::ProfileLoopSpec read_loop(const worker_api::ProfileLoop& source) {
    occccad::kernel::ProfileLoopSpec loop;
    loop.id = source.id();
    for (const auto& curve : source.curves())
        loop.curves.push_back(exact_sketch_curve(curve));
    return loop;
}
occccad::kernel::LoftSectionSpec read_loft_section(const worker_api::LoftSectionSpec& source) {
    occccad::kernel::LoftSectionSpec section;
    section.sketch_id = source.sketch_id();
    section.seam_entity_id = source.seam_entity_id();
    section.seam_angle = source.seam_angle();
    section.reversed = source.reversed();
    section.region.id = source.region().id();
    section.region.outer = read_loop(source.region().outer());
    for (const auto& hole : source.region().holes())
        section.region.holes.push_back(read_loop(hole));
    section.origin = {source.origin().x(), source.origin().y(), source.origin().z()};
    section.normal = {source.normal().x(), source.normal().y(), source.normal().z()};
    section.u_direction = {source.u_direction().x(), source.u_direction().y(),
                           source.u_direction().z()};
    section.correspondence_resolved = source.correspondence_resolved();
    section.point_id = source.point_id();
    section.point = {source.point().x(), source.point().y(), source.point().z()};
    return section;
}

class GeometryWorkerService final : public worker_api::GeometryWorker::Service {
public:
    GeometryWorkerService() {
        if(const char* value=std::getenv("OCCCCAD_STAGE_CACHE_BYTES")) {
            size_t consumed=0;const auto budget=std::stoull(value,&consumed);
            if(consumed!=std::string(value).size()||budget>2ULL*1024*1024*1024)throw std::invalid_argument("invalid stage cache byte budget");
            kernel_.set_runtime_cache_budget(static_cast<size_t>(budget));
        }
    }
    grpc::Status Ping(grpc::ServerContext* /*context*/, const worker_api::PingRequest* /*request*/,
                      worker_api::PingResponse* response) override {
        // Liveness must not queue behind a long OCCT operation. Resident
        // count is a telemetry snapshot and may lag while geometry is busy.
        std::unique_lock<std::mutex> lock(mutex_, std::try_to_lock);
        if (lock.owns_lock())
            resident_snapshot_.store(static_cast<uint32_t>(kernel_.resident_count()), std::memory_order_relaxed);
        const char* identity = std::getenv("OCCCCAD_GEOMETRY_WORKER_ID");
        response->set_worker_id(identity ? identity : "geometry-worker-local-1");
        response->set_occt_version(OCC_VERSION_COMPLETE);
        response->set_resident_geometry_count(resident_snapshot_.load(std::memory_order_relaxed));
        return grpc::Status::OK;
    }

    grpc::Status AnalyzeInterference(grpc::ServerContext* context,
        const worker_api::AnalyzeInterferenceRequest* request,
        worker_api::AnalyzeInterferenceResponse* response) override {
        const auto started=std::chrono::steady_clock::now();
        size_t restored=0,reused=0;
        if(request->geometry_size()<2 || request->geometry_size()>128 || request->pairs_size()>8192 ||
            !std::isfinite(request->clearance_mm()) || request->clearance_mm()<0 ||
            !std::isfinite(request->tolerance_mm()) || request->tolerance_mm()<=0 || (request->details() && request->pairs_size()!=1))
            return {grpc::StatusCode::INVALID_ARGUMENT,"invalid interference request limits"};
        std::unique_lock<std::mutex> lock(mutex_,std::defer_lock);
        while(!lock.try_lock()) {
            if(context->IsCancelled()) return {grpc::StatusCode::CANCELLED,"analysis cancelled while queued"};
            std::this_thread::sleep_for(std::chrono::milliseconds(5));
        }
        // This cache owns private exact geometry and never aliases the modelling kernel.
        // Reset as a unit before loading a request; the full request budget still applies.
        uint64_t incoming=0;std::unordered_set<std::string> refs;
        for(const auto& input:request->geometry())if(input.has_brep()){
            const auto k=input.brep().SerializeAsString();
            if(refs.insert(k).second){if(input.brep().size_bytes()>256ULL*1024*1024-incoming)
                return {grpc::StatusCode::INVALID_ARGUMENT,"analysis B-Rep byte budget exceeded"};incoming+=input.brep().size_bytes();}
        }
        if(!interference_kernel_ || interference_bytes_+incoming>256ULL*1024*1024 || interference_loaded_.size()+refs.size()>128){
            interference_kernel_=std::make_unique<occccad::kernel::OcctKernel>();interference_loaded_.clear();interference_bytes_=0;
        }
        auto& analysis=*interference_kernel_;
        std::unordered_map<std::string,occccad::kernel::PlacedGeometry> placed;
        std::unordered_map<std::string,std::string> failures;
        uint64_t bytes=0;
        for(const auto& input:request->geometry()) {
            if(input.id().empty() || placed.count(input.id()))
                return {grpc::StatusCode::INVALID_ARGUMENT,"duplicate/empty analysis instance identity"};
            const auto& p=input.pose();
            occccad::kernel::PlacedGeometry value{input.geometry_id(),
                {p.translation().x(),p.translation().y(),p.translation().z()},
                {p.rotation().x(),p.rotation().y(),p.rotation().z(),p.rotation().w()}};
            placed.emplace(input.id(),value);
            try {
                if(context->IsCancelled()) throw std::runtime_error("CANCELLED");
                if(!input.has_brep()) throw std::invalid_argument("B-Rep artifact required");
                const auto cache_key=input.brep().SerializeAsString();
                auto existing=interference_loaded_.find(cache_key);
                std::string id;
                if(existing!=interference_loaded_.end()){id=existing->second;++reused;}
                else {
                    if(input.brep().size_bytes()>256ULL*1024*1024-bytes)
                        throw std::invalid_argument("analysis B-Rep byte budget exceeded");
                    bytes+=input.brep().size_bytes();
                    id=analysis.loadBrepr(read_artifact(input.brep()));++restored;
                    interference_loaded_.emplace(cache_key,id);interference_bytes_+=input.brep().size_bytes();
                }
                if(id!=input.geometry_id()) throw std::invalid_argument("analysis geometry digest mismatch");
            } catch(const Standard_Failure& e) {failures[input.id()]=e.GetMessageString();}
              catch(const std::exception& e) {failures[input.id()]=e.what();}
        }
        const auto loaded_at=std::chrono::steady_clock::now();
        bool complete=true;
        for(const auto& pair:request->pairs()) {
            if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"interference cancelled"};
            auto a=placed.find(pair.first_id()),b=placed.find(pair.second_id());
            if(a==placed.end() || b==placed.end() || a==b)
                return {grpc::StatusCode::INVALID_ARGUMENT,"invalid interference pair identity"};
            occccad::kernel::InterferenceResult result;
            if(failures.count(a->first) || failures.count(b->first))
                result.diagnostic="INPUT_GEOMETRY_FAILED:"+failures[a->first]+":"+failures[b->first];
            else result=analysis.analyze_interference(a->second,b->second,request->clearance_mm(),
                request->tolerance_mm(),[&]{return context->IsCancelled();},request->threshold_only(),request->details());
            auto* output=response->add_pairs();
            output->set_first_id(a->first);output->set_second_id(b->first);
            output->set_classification(result.classification);output->set_diagnostic(result.diagnostic);
            output->set_distance_mm(result.distance);output->set_common_volume_mm3(result.common_volume);
            output->set_complete(result.complete);output->set_clearance_satisfied(result.clearance_satisfied);
            output->set_common_tested(result.common_tested);
            output->set_distance_exact(result.distance_exact);output->set_witness_kind(result.witness_kind);
            output->set_detail_status(result.detail_status);output->set_containment(result.containment);
            for(const auto& p:result.section_points){auto* v=output->add_section_points();v->set_x(p.x);v->set_y(p.y);v->set_z(p.z);}
            for(const auto& p:result.overlap_triangles){auto* v=output->add_overlap_triangles();v->set_x(p.x);v->set_y(p.y);v->set_z(p.z);}
            output->mutable_first_witness()->set_x(result.first_witness.x);
            output->mutable_first_witness()->set_y(result.first_witness.y);
            output->mutable_first_witness()->set_z(result.first_witness.z);
            output->mutable_second_witness()->set_x(result.second_witness.x);
            output->mutable_second_witness()->set_y(result.second_witness.y);
            output->mutable_second_witness()->set_z(result.second_witness.z);
            complete=complete && result.complete;
        }
        response->set_complete(complete);response->set_kernel_build("occt-" OCC_VERSION_COMPLETE "/solid-dmu-v2");
        spdlog::info("interference request={} bodies={} pairs={} restored={} reused={} input_ms={} pairs_ms={} details={} complete={}",request->request_id(),request->geometry_size(),request->pairs_size(),restored,reused,std::chrono::duration<double,std::milli>(loaded_at-started).count(),std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-loaded_at).count(),request->details(),complete);
        return grpc::Status::OK;
    }

    grpc::Status MeasureGeometry(grpc::ServerContext* context,
        const worker_api::MeasureGeometryRequest* request,worker_api::MeasureGeometryResponse* response) override {
        const int required=(request->kind()=="ITEM"||request->kind()=="CURVATURE")?1:request->kind()=="BETWEEN"?2:request->kind()=="ANGLE_POINTS"?3:0;
        if(!required||request->operands_size()!=required||request->geometry_size()<1||request->geometry_size()>128)
            return {grpc::StatusCode::INVALID_ARGUMENT,"invalid measurement scope"};
        if(request->kind()=="CURVATURE"&&(request->sample_count()<16||request->sample_count()>4096))return {grpc::StatusCode::INVALID_ARGUMENT,"invalid curvature sample budget"};
        std::unique_lock<std::mutex> lock(mutex_,std::defer_lock);
        while(!lock.try_lock()){if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"measurement cancelled"};std::this_thread::sleep_for(std::chrono::milliseconds(5));}
        try {
            occccad::kernel::OcctKernel kernel;
            uint64_t bytes=0;std::unordered_map<std::string,occccad::kernel::PlacedGeometry> geometry;
            std::unordered_map<std::string,std::string> loaded;
            for(const auto& g:request->geometry()){
                if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"measurement cancelled"};
                if(g.id().empty()||geometry.count(g.id())||!g.has_brep()||!g.has_pose())return {grpc::StatusCode::INVALID_ARGUMENT,"invalid measurement geometry"};
                const auto key=g.brep().SerializeAsString();std::string id;
                if(loaded.count(key))id=loaded.at(key);
                else {
                    if(g.brep().size_bytes()>256ULL*1024*1024-bytes)return {grpc::StatusCode::RESOURCE_EXHAUSTED,"measurement B-Rep limit"};
                    bytes+=g.brep().size_bytes();id=kernel.loadBrepr(read_artifact(g.brep()));loaded.emplace(key,id);
                }
                if(id!=g.geometry_id())return {grpc::StatusCode::INVALID_ARGUMENT,"measurement geometry digest mismatch"};
                const auto& p=g.pose();geometry.emplace(g.id(),occccad::kernel::PlacedGeometry{id,{p.translation().x(),p.translation().y(),p.translation().z()},{p.rotation().x(),p.rotation().y(),p.rotation().z(),p.rotation().w()}});
            }
            size_t count=0;std::vector<std::vector<occccad::kernel::MeasurementElement>> operands;
            for(const auto& op:request->operands()){
                if(op.elements_size()<1||op.elements_size()>128||count+op.elements_size()>128)return {grpc::StatusCode::INVALID_ARGUMENT,"measurement element limit"};
                operands.emplace_back();
                for(const auto& e:op.elements()){
                    if(!geometry.count(e.input_id()))return {grpc::StatusCode::INVALID_ARGUMENT,"measurement element missing"};
                    operands.back().push_back({geometry.at(e.input_id()),e.kind(),e.local_id(),e.mode()});++count;
                }
            }
            const auto r=kernel.measure_geometry(request->kind(),operands,[&]{return context->IsCancelled();},request->kind()=="CURVATURE"?request->sample_count():256);
            if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"measurement cancelled"};
            response->set_complete(r.complete);response->set_diagnostic(r.diagnostic);response->set_geometry_type(r.geometry_type);
            for(const auto& [key,value]:r.values)(*response->mutable_values())[key]=value;
            for(const auto& [key,p]:r.anchors){auto& v=(*response->mutable_anchors())[key];v.set_x(p.x);v.set_y(p.y);v.set_z(p.z);}
            for(const auto& p:r.curvature){auto* v=response->add_curvature();v->mutable_position()->set_x(p.position.x);v->mutable_position()->set_y(p.position.y);v->mutable_position()->set_z(p.position.z);v->mutable_normal()->set_x(p.normal.x);v->mutable_normal()->set_y(p.normal.y);v->mutable_normal()->set_z(p.normal.z);v->set_minimum(p.minimum);v->set_maximum(p.maximum);v->set_mean(p.mean);v->set_gaussian(p.gaussian);v->set_surface(p.surface);}
            response->set_undefined_samples(r.undefined_samples);
            response->set_boundary_complete(r.boundary_complete);
            for (const auto& edge : r.boundary_edges) {
                auto* target = response->add_boundary_edges();
                target->set_edge_id(edge.edge_id);
                for (const auto& point : edge.points) {
                    auto* v = target->add_points();
                    v->set_x(point.x);
                    v->set_y(point.y);
                    v->set_z(point.z);
                }
            }
            for (const auto& gap : r.gap_candidates) {
                auto* target = response->add_gap_candidates();
                target->set_first_edge(gap.first_edge);
                target->set_second_edge(gap.second_edge);
                target->mutable_first()->set_x(gap.first.x);
                target->mutable_first()->set_y(gap.first.y);
                target->mutable_first()->set_z(gap.first.z);
                target->mutable_second()->set_x(gap.second.x);
                target->mutable_second()->set_y(gap.second.y);
                target->mutable_second()->set_z(gap.second.z);
                target->set_distance_mm(gap.distance_mm);
            }

            response->set_kernel_build(request->kind()=="CURVATURE"?"occt-" OCC_VERSION_COMPLETE "/curvature-v1":"occt-" OCC_VERSION_COMPLETE "/measurement-v2");return grpc::Status::OK;
        }catch(const Standard_Failure&){return {grpc::StatusCode::INTERNAL,"measurement input unavailable"};}
        catch(const std::exception& e){return {grpc::StatusCode::INVALID_ARGUMENT,e.what()};}
    }

    grpc::Status AnalyzeSpatialGeometry(grpc::ServerContext* context,
        const worker_api::AnalyzeSpatialGeometryRequest* request,worker_api::AnalyzeSpatialGeometryResponse* response) override {
        if(request->geometry_size()<1||request->geometry_size()>128||request->source_ids_size()<1||request->source_ids_size()>128||
           (request->kind()!="SECTION"&&request->kind()!="BAND")||request->target_ids_size()>128||
           !std::isfinite(request->accuracy_mm())||request->accuracy_mm()<0.01||request->accuracy_mm()>1000||
           (request->kind()=="BAND"&&(request->source_ids_size()>16||request->target_ids_size()<1)))
            return {grpc::StatusCode::INVALID_ARGUMENT,"invalid spatial analysis scope"};
        std::unique_lock<std::mutex> lock(mutex_,std::defer_lock);
        while(!lock.try_lock()){if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"spatial analysis cancelled"};std::this_thread::sleep_for(std::chrono::milliseconds(5));}
        uint64_t incoming=0;std::unordered_set<std::string> unique;
        for(const auto& g:request->geometry()){
            if(!g.has_brep())return {grpc::StatusCode::INVALID_ARGUMENT,"spatial B-Rep required"};
            if(unique.insert(g.brep().SerializeAsString()).second){if(g.brep().size_bytes()>256ULL*1024*1024-incoming)return {grpc::StatusCode::RESOURCE_EXHAUSTED,"spatial byte limit"};incoming+=g.brep().size_bytes();}
        }
        if(!interference_kernel_||interference_bytes_+incoming>256ULL*1024*1024||interference_loaded_.size()+unique.size()>128){interference_kernel_=std::make_unique<occccad::kernel::OcctKernel>();interference_loaded_.clear();interference_bytes_=0;}
        std::unordered_map<std::string,occccad::kernel::PlacedGeometry> inputs;
        std::unordered_map<std::string,std::string> failures;
        for(const auto& g:request->geometry()){
            if(g.id().empty()||inputs.count(g.id()))return {grpc::StatusCode::INVALID_ARGUMENT,"duplicate spatial identity"};
            const auto& p=g.pose();inputs.emplace(g.id(),occccad::kernel::PlacedGeometry{g.geometry_id(),{p.translation().x(),p.translation().y(),p.translation().z()},{p.rotation().x(),p.rotation().y(),p.rotation().z(),p.rotation().w()}});
            try{const auto key=g.brep().SerializeAsString();const auto found=interference_loaded_.find(key);std::string id;
                if(found!=interference_loaded_.end())id=found->second;else{id=interference_kernel_->loadBrepr(read_artifact(g.brep()));interference_loaded_.emplace(key,id);interference_bytes_+=g.brep().size_bytes();}
                if(id!=g.geometry_id())throw std::runtime_error("spatial geometry digest mismatch");
            }catch(const Standard_Failure& e){failures[g.id()]=e.GetMessageString()?e.GetMessageString():"SPATIAL_INPUT_UNAVAILABLE";}catch(const std::exception& e){failures[g.id()]=e.what();}
        }
        occccad::kernel::SpatialGeometryOptions options;options.kind=request->kind();options.section_type=request->section_type();
        const auto& f=request->frame();options.frame.translation={f.translation().x(),f.translation().y(),f.translation().z()};options.frame.rotation={f.rotation().x(),f.rotation().y(),f.rotation().z(),f.rotation().w()};
        options.size={request->size().x(),request->size().y(),request->size().z()};options.minimum=request->minimum_mm();options.maximum=request->maximum_mm();options.accuracy=request->accuracy_mm();
        std::vector<occccad::kernel::PlacedGeometry> targets;bool bad_target=false;std::unordered_set<std::string> target_ids,source_ids;
        for(const auto& id:request->target_ids()){if(!inputs.count(id)||!target_ids.insert(id).second)return {grpc::StatusCode::INVALID_ARGUMENT,"invalid spatial target"};targets.push_back(inputs.at(id));bad_target=bad_target||failures.count(id);}
        size_t graphics_count=0;bool complete=true;
        for(const auto& id:request->source_ids()){
            if(!inputs.count(id)||!source_ids.insert(id).second||target_ids.count(id))return {grpc::StatusCode::INVALID_ARGUMENT,"invalid spatial source"};
            if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"spatial analysis cancelled"};
            occccad::kernel::SpatialGeometryResult value;
            if(failures.count(id)||bad_target)value.diagnostic="SPATIAL_INPUT_UNAVAILABLE";
            else value=interference_kernel_->analyze_spatial(inputs.at(id),targets,options,[&]{return context->IsCancelled();});
            if(value.segments.size()+value.triangles.size()>120000-graphics_count){value=occccad::kernel::SpatialGeometryResult{};value.diagnostic="SPATIAL_GRAPHICS_LIMIT";}graphics_count+=value.segments.size()+value.triangles.size();
            auto* body=response->add_bodies();body->set_id(id);body->set_complete(value.complete);body->set_diagnostic(value.diagnostic);body->set_area_mm2(value.area);
            body->mutable_centroid()->set_x(value.centroid.x);body->mutable_centroid()->set_y(value.centroid.y);body->mutable_centroid()->set_z(value.centroid.z);
            for(const auto& p:value.segments){auto* v=body->add_segments();v->set_x(p.x);v->set_y(p.y);v->set_z(p.z);}
            for(const auto& p:value.triangles){auto* v=body->add_triangles();v->set_x(p.x);v->set_y(p.y);v->set_z(p.z);}
            for(auto tag:value.regions)body->add_regions(tag);complete=complete&&value.complete;
        }
        response->set_complete(complete);response->set_kernel_build("occt-" OCC_VERSION_COMPLETE "/spatial-geometry-v1");return grpc::Status::OK;
    }

    grpc::Status ResolveLoftCorrespondence(
        grpc::ServerContext* context, const worker_api::ResolveLoftCorrespondenceRequest* request,
        worker_api::ResolveLoftCorrespondenceResponse* response) override {
        try {
            if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED,"loft correspondence cancelled"};
            if (request->sections_size()<2 || request->sections_size()>32)
                return {grpc::StatusCode::INVALID_ARGUMENT,"INVALID_LOFT_SECTION_COUNT"};
            std::vector<occccad::kernel::LoftSectionSpec> sections;
            for (const auto& s : request->sections())
                sections.push_back(read_loft_section(s));
            const auto resolved = occccad::kernel::resolve_loft_correspondence(sections);
            const auto points = occccad::kernel::loft_connection_points(resolved);
            for (std::size_t i = 0; i < resolved.size(); ++i) {
                const auto& s = resolved[i];
                auto* out = response->add_sections();
                out->set_seam_entity_id(s.seam_entity_id);
                out->set_reversed(s.reversed);
                out->set_seam_angle(s.seam_angle);
                for (const auto& p : points[i]) {
                    auto* v = out->add_boundary_points();
                    v->set_x(p.x);
                    v->set_y(p.y);
                    v->set_z(p.z);
                }
            }
            return grpc::Status::OK;
        } catch (const Standard_Failure& e) {
            return {grpc::StatusCode::INVALID_ARGUMENT, e.GetMessageString()};
        } catch (const std::exception& e) {
            return {grpc::StatusCode::INVALID_ARGUMENT, e.what()};
        }
    }

    grpc::Status ComputeSketchCurves(grpc::ServerContext* context,
                                     const worker_api::ComputeSketchCurvesRequest* request,
                                     worker_api::ComputeSketchCurvesResponse* response) override {
        try {
            if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED,"curve computation cancelled"};
            if (request->operation()=="PROFILE") {
                if(request->loops_size()<1||request->loops_size()>256)throw std::invalid_argument("profile requires 1..256 loops");
                std::vector<occccad::kernel::ProfileLoopSpec> loops;size_t count=0;
                for(const auto& input:request->loops()){occccad::kernel::ProfileLoopSpec loop;loop.id=input.id();for(const auto& c:input.curves()){if(++count>512)throw std::invalid_argument("profile exceeds 512 curves");loop.curves.push_back(exact_sketch_curve(c));}loops.push_back(std::move(loop));}
                const auto output=occccad::kernel::classify_sketch_profile(loops);
                auto write_loop=[](const occccad::kernel::ProfileLoopSpec& loop,worker_api::ProfileLoop* value){value->set_id(loop.id);for(const auto& c:loop.curves)exact_sketch_curve_proto(c,value->add_curves());};
                for(const auto& region:output){auto* value=response->add_regions();value->set_id(region.id);write_loop(region.outer,value->mutable_outer());for(const auto& hole:region.holes)write_loop(hole,value->add_holes());}
            } else if (request->curves_size()<1 || request->curves_size()>64) throw std::invalid_argument("curve computation requires 1..64 curves");
            else if (request->operation()=="NORMALIZE") {
                for (const auto& c:request->curves()) exact_sketch_curve_proto(occccad::kernel::canonicalize_sketch_curve(exact_sketch_curve(c)),response->add_curves());
            } else if (request->operation()=="INTERSECT") {
                if (request->curves_size()!=2) throw std::invalid_argument("intersection requires two curves");
                const auto result=occccad::kernel::intersect_sketch_curves(exact_sketch_curve(request->curves(0)),exact_sketch_curve(request->curves(1)),1e-7);
                for (const auto& p:result.points) {
                    auto* value=response->add_intersections();value->set_kind(p.kind==occccad::kernel::SketchCurveIntersectionKind::tangent?"TANGENT":"POINT");
                    value->mutable_point()->set_x(p.point.x);value->mutable_point()->set_y(p.point.y);value->set_first_parameter(p.first_parameter);value->set_second_parameter(p.second_parameter);
                }
                for (const auto& p:result.overlaps) {auto* value=response->add_overlaps();value->set_first_start(p.first_start);value->set_first_end(p.first_end);value->set_second_start(p.second_start);value->set_second_end(p.second_end);}
            } else if (request->operation()=="TRIM") {
                if (request->curves_size()!=1 || !request->has_parameter_start() || !request->has_parameter_end()) throw std::invalid_argument("trim requires one curve and an explicit interval");
                exact_sketch_curve_proto(occccad::kernel::trim_sketch_curve(exact_sketch_curve(request->curves(0)),request->parameter_start(),request->parameter_end()),response->add_curves());
            } else throw std::invalid_argument("unsupported exact sketch curve operation");
            response->set_status("OK");
        } catch (const std::exception& error) {
            response->Clear();response->set_status("INVALID");response->set_diagnostic(error.what());
        }
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
            const auto input=read_sketch(request->sketch());
            const auto solve_started=std::chrono::steady_clock::now();
            const auto result = sketch_solver_->solve(input);
            response->set_kernel_solve_ms(std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-solve_started).count());
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

    grpc::Status ProjectExternalGeometry(
        grpc::ServerContext* context,
        const worker_api::ProjectExternalGeometryRequest* request,
        worker_api::ProjectExternalGeometryResponse* response) override {
        if (context->IsCancelled())
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        if (request->request_id().empty() || !request->has_source() || !request->has_frame())
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id, source and projection frame are required"};
        if (request->projection_kind() != "ORTHOGONAL")
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "only ORTHOGONAL external geometry projection is supported"};
        const auto vec = [](const worker_api::Vec3& value) {
            return sketch_api::Vec3{value.x(), value.y(), value.z()};
        };
        sketch_api::ExternalProjectionSource source;
        const auto& input = request->source();
        const auto& evidence = input.evidence();
        source.origin = vec(evidence.origin());
        source.direction = vec(evidence.direction());
        source.measure_mm = evidence.has_measure_si() ? evidence.measure_si() * 1000.0 : 0.0;
        source.has_parameters = evidence.has_parameter_start() && evidence.has_parameter_end();
        source.parameter_start = evidence.parameter_start();
        source.parameter_end = evidence.parameter_end();
        source.x_direction = vec(evidence.x_direction());
        source.radius = evidence.radius_mm();
        source.minor_radius = evidence.minor_radius_mm();
        if (input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED && evidence.geometry_type()=="AXIS") {
            source.kind=sketch_api::ExternalSourceKind::axis;
        } else if ((input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_VERTEX || input.topology_type()==worker_api::PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED) &&
            evidence.geometry_type() == "POINT") {
            source.kind = sketch_api::ExternalSourceKind::point;
        } else if ((input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_EDGE || (input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED && request->source_kind()=="SKETCH_ENTITY")) &&
                   evidence.geometry_type() == "LINE") {
            source.kind = sketch_api::ExternalSourceKind::line;
        } else if ((input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_EDGE || (input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED && request->source_kind()=="SKETCH_ENTITY")) &&
                   evidence.geometry_type() == "CIRCLE") {
            source.kind = sketch_api::ExternalSourceKind::circle;
        } else if ((input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_EDGE || (input.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED && request->source_kind()=="SKETCH_ENTITY")) && evidence.geometry_type() == "ELLIPSE") {
            source.kind = sketch_api::ExternalSourceKind::ellipse;
        } else {
            response->set_status("UNRESOLVED_EXTERNAL");
            response->set_diagnostic_code("EXTERNAL_SOURCE_TYPE_MISMATCH");
            response->set_diagnostic("projection requires a point, axis, line, circle/arc or ellipse/elliptical arc edge");
            response->set_source_digest(evidence.evidence_digest());
            return grpc::Status::OK;
        }
        const auto& frame = request->frame();
        const auto projected = sketch_api::project_external_geometry(
            source, {vec(frame.origin()), vec(frame.x_direction()), vec(frame.normal())});
        response->set_source_digest(evidence.evidence_digest());
        if (projected.status != sketch_api::ExternalProjectionStatus::connected) {
            response->set_status("UNRESOLVED_EXTERNAL");
            response->set_diagnostic_code(projected.diagnostic_code);
            response->set_diagnostic(projected.diagnostic);
            return grpc::Status::OK;
        }
        response->set_status("CONNECTED");
        const auto fill = [](const sketch_api::Vec2& value, worker_api::Vec2* target) {
            target->set_x(value.x);
            target->set_y(value.y);
        };
        fill(projected.center, response->mutable_center());
        response->set_radius(projected.radius);
        response->set_major_radius(projected.major_radius);
        response->set_minor_radius(projected.minor_radius);
        response->set_rotation(projected.rotation);
        response->set_start_angle(projected.start_angle);
        response->set_end_angle(projected.end_angle);
        switch (projected.kind) {
            case sketch_api::ProjectedGeometryKind::point:
                response->set_geometry_kind("POINT");
                fill(projected.point, response->mutable_point());
                break;
            case sketch_api::ProjectedGeometryKind::line:
                response->set_geometry_kind("LINE");
                fill(projected.start, response->mutable_start());
                fill(projected.end, response->mutable_end());
                break;
            case sketch_api::ProjectedGeometryKind::circle:
                response->set_geometry_kind("CIRCLE");
                fill(projected.center, response->mutable_center());
                response->set_radius(projected.radius);
                break;
            case sketch_api::ProjectedGeometryKind::arc: response->set_geometry_kind("ARC"); break;
            case sketch_api::ProjectedGeometryKind::ellipse: response->set_geometry_kind("ELLIPSE"); break;
            case sketch_api::ProjectedGeometryKind::elliptical_arc: response->set_geometry_kind("ELLIPTICAL_ARC"); break;
            default:
                return {grpc::StatusCode::INTERNAL, "projection returned no geometry"};
        }
        return grpc::Status::OK;
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
            model.bodies.push_back({input.id(), pose(input.initial_pose()),
                                    input.has_initial_guess() ? std::optional<assembly_api::Pose>(
                                                                    pose(input.initial_guess()))
                                                              : std::nullopt});
        for (const auto& input : request->geometry()) {
            if (!input.length_unit().empty() && input.length_unit() != "mm" && input.length_unit() != "MM")
                return {grpc::StatusCode::INVALID_ARGUMENT, "assembly descriptor length_unit must be mm"};
            assembly_api::Geometry geometry;
            if (input.kind() == "POINT")
                geometry = assembly_api::PointGeometry{vec(input.origin())};
            else if (input.kind() == "AXIS")
                geometry = assembly_api::AxisGeometry{vec(input.origin()), vec(input.direction())};
            else if (input.kind() == "PLANE")
                geometry = assembly_api::PlaneGeometry{vec(input.origin()), vec(input.direction())};
            else if (input.kind() == "CYLINDER")
                geometry = assembly_api::CylinderGeometry{vec(input.origin()),
                                                          vec(input.direction()), input.radius(), input.material_side()==0?1:input.material_side()};
            else if (input.kind() == "SPHERE")
                geometry = assembly_api::SphereGeometry{vec(input.origin()),input.radius(),input.material_side()==0?1:input.material_side()};
            else if (input.kind() == "CONE")
                geometry = assembly_api::ConeGeometry{vec(input.origin()),vec(input.direction()),input.half_angle(),input.cone_leaf(),input.material_side()};
            else if (input.kind() == "CIRCLE")
                geometry = assembly_api::CircleGeometry{vec(input.origin()),vec(input.direction()),input.radius(),vec(input.x_direction())};
            else if (input.kind() == "FRAME") {
                assembly_api::Pose frame; frame.translation=vec(input.origin());
                frame.rotation={input.rotation().x(),input.rotation().y(),input.rotation().z(),input.rotation().w()};
                geometry = assembly_api::FrameGeometry{frame};
            }
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
            if (input.has_angle_reference_direction())
                constraint.angle_reference_direction = vec(input.angle_reference_direction());
            if (input.has_angle_reference()) {
                constraint.angle_reference_geometry=assembly_api::GeometryRef{input.angle_reference().body_id(),input.angle_reference().geometry_id()};
                constraint.reverse_angle_reference=input.reverse_angle_reference();
            }
            if (input.has_spatial_angle_branch_direction())
                constraint.spatial_angle_branch_direction = vec(input.spatial_angle_branch_direction());
            if (input.has_angle_branch_state())
                constraint.angle_branch_state =
                    assembly_api::AngleBranchState{input.angle_branch_state().wrapped_angle(),
                                                   input.angle_branch_state().unwrapped_angle(),
                                                   input.angle_branch_state().winding()};
            if (input.kind() == "FIX")
                constraint.kind = assembly_api::ConstraintKind::Fix;
            else if (input.kind() == "RIGID")
                constraint.kind = assembly_api::ConstraintKind::Rigid;
            else if (input.kind() == "COINCIDENT")
                constraint.kind = assembly_api::ConstraintKind::Coincident;
            else if (input.kind() == "SURFACE_INCIDENCE")
                constraint.kind = assembly_api::ConstraintKind::SurfaceIncidence;
            else if (input.kind() == "CONCENTRIC")
                constraint.kind = assembly_api::ConstraintKind::Concentric;
            else if (input.kind() == "PARALLEL")
                constraint.kind = assembly_api::ConstraintKind::Parallel;
            else if (input.kind() == "PERPENDICULAR")
                constraint.kind = assembly_api::ConstraintKind::Perpendicular;
            else if (input.kind() == "ANGLE")
                constraint.kind = assembly_api::ConstraintKind::Angle;
            else if (input.kind() == "DISTANCE")
                constraint.kind = assembly_api::ConstraintKind::Distance;
            else if (input.kind() == "CONTACT") {
                constraint.kind = assembly_api::ConstraintKind::Contact;
                if (input.contact_kind()=="FACE") constraint.contact_kind=assembly_api::ContactKind::Face;
                else if(input.contact_kind()=="LINE") constraint.contact_kind=assembly_api::ContactKind::Line;
                else if(input.contact_kind()=="POINT") constraint.contact_kind=assembly_api::ContactKind::Point;
                else if(input.contact_kind()=="RING") constraint.contact_kind=assembly_api::ContactKind::Ring;
                else return {grpc::StatusCode::INVALID_ARGUMENT,"invalid Contact subtype"};
                if(input.contact_side()=="EXTERNAL") constraint.contact_side=assembly_api::ContactSide::External;
                else if(input.contact_side()=="INTERNAL") constraint.contact_side=assembly_api::ContactSide::Internal;
                else return {grpc::StatusCode::INVALID_ARGUMENT,"invalid Contact side"};
                constraint.contact_branch=input.contact_branch();
                if(constraint.contact_branch!=1 && constraint.contact_branch!=-1)
                    return {grpc::StatusCode::INVALID_ARGUMENT,"invalid Contact branch"};
            }
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
            else if (input.distance_relation() == "SELECTED_PLANE_NORMAL_V1")
                constraint.distance_relation = assembly_api::DistanceRelation::SelectedPlaneNormal;
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
        if (request->has_solver_profile()) {
            const auto& profile = request->solver_profile();
            if (profile.schema_version() != 2)
                return {grpc::StatusCode::INVALID_ARGUMENT,
                        "assembly solver profile schema_version must be 2"};
            if (profile.max_iterations() > 0)
                options.max_iterations = profile.max_iterations();
            if (profile.length_tolerance() > 0.0)
                options.length_tolerance = profile.length_tolerance();
            if (profile.angle_tolerance() > 0.0)
                options.angle_tolerance = profile.angle_tolerance();
            if (profile.classification_length_tolerance() > 0.0)
                options.classification_length_tolerance = profile.classification_length_tolerance();
            if (profile.classification_angle_tolerance() > 0.0)
                options.classification_angle_tolerance = profile.classification_angle_tolerance();
            if (profile.translation_step_tolerance() > 0.0)
                options.translation_step_tolerance = profile.translation_step_tolerance();
            if (profile.rotation_step_tolerance() > 0.0)
                options.rotation_step_tolerance = profile.rotation_step_tolerance();
            if (profile.degeneracy_tolerance() > 0.0)
                options.degeneracy_tolerance = profile.degeneracy_tolerance();
            if (profile.finite_difference_step() > 0.0)
                options.finite_difference_step = profile.finite_difference_step();
            if (profile.initial_damping() > 0.0)
                options.initial_damping = profile.initial_damping();
            if (profile.rank_tolerance() > 0.0)
                options.rank_tolerance = profile.rank_tolerance();
            if (profile.translation_finite_difference_step() > 0.0)
                options.translation_finite_difference_step =
                    profile.translation_finite_difference_step();
            if (profile.rotation_finite_difference_step() > 0.0)
                options.rotation_finite_difference_step = profile.rotation_finite_difference_step();
            if (profile.rank_absolute_tolerance() > 0.0)
                options.rank_absolute_tolerance = profile.rank_absolute_tolerance();
            if (profile.rank_relative_tolerance() > 0.0)
                options.rank_relative_tolerance = profile.rank_relative_tolerance();
            if (profile.gradient_tolerance() > 0.0)
                options.gradient_tolerance = profile.gradient_tolerance();
            if (profile.motion_length_scale() != 0.0)
                options.motion_length_scale = profile.motion_length_scale();
            if (profile.motion_angle_scale() != 0.0)
                options.motion_angle_scale = profile.motion_angle_scale();
            if (profile.preference_tolerance() != 0.0)
                options.preference_tolerance = profile.preference_tolerance();
            if (profile.objective_tolerance() != 0.0)
                options.objective_tolerance = profile.objective_tolerance();
            if (profile.has_max_preference_iterations())
                options.max_preference_iterations = profile.max_preference_iterations();
            if (profile.max_conflict_probes() > 0)
                options.max_conflict_probes = profile.max_conflict_probes();
            options.verify_analytic_jacobians = profile.verify_analytic_jacobians();
            if (profile.jacobian_check_tolerance() > 0.0)
                options.jacobian_check_tolerance = profile.jacobian_check_tolerance();
        }
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
        options.should_cancel = [context] { return context->IsCancelled(); };
        if(request->disable_conflict_probes()) options.max_conflict_probes=0;
        if(request->has_drag_target()) {
            const auto& input=request->drag_target();
            if(input.translation_components_size()!=3 || input.rotation_components_size()!=3)
                return {grpc::StatusCode::INVALID_ARGUMENT,"drag component masks must have three entries"};
            assembly_api::DragTarget target;
            target.body_id=input.body_id(); target.local_grab_point=vec(input.local_grab_point());
            target.target_pose=pose(input.target_pose());
            target.frame_rotation={input.frame_rotation().x(),input.frame_rotation().y(),input.frame_rotation().z(),input.frame_rotation().w()};
            for(int i=0;i<3;++i){target.translation_components[i]=input.translation_components(i);target.rotation_components[i]=input.rotation_components(i);}
            if((input.hold_translation_components_size()!=0 && input.hold_translation_components_size()!=3) ||
               (input.hold_rotation_components_size()!=0 && input.hold_rotation_components_size()!=3))
                return {grpc::StatusCode::INVALID_ARGUMENT,"hold masks must be absent or have three entries"};
            for(int i=0;i<3;++i){
                target.hold_translation_components[i]=input.hold_translation_components_size()?input.hold_translation_components(i):false;
                target.hold_rotation_components[i]=input.hold_rotation_components_size()?input.hold_rotation_components(i):false;
                if((target.hold_translation_components[i]&&target.translation_components[i]) || (target.hold_rotation_components[i]&&target.rotation_components[i]))
                    return {grpc::StatusCode::INVALID_ARGUMENT,"driven and held components must be disjoint"};
            }
            target.target_sequence=input.target_sequence(); options.drag_target=target;
        }
        auto* effective = response->mutable_effective_solver_profile();
        effective->set_schema_version(2);
        effective->set_max_iterations(options.max_iterations);
        effective->set_length_tolerance(options.length_tolerance);
        effective->set_angle_tolerance(options.angle_tolerance);
        effective->set_classification_length_tolerance(options.classification_length_tolerance);
        effective->set_classification_angle_tolerance(options.classification_angle_tolerance);
        effective->set_translation_step_tolerance(options.translation_step_tolerance);
        effective->set_rotation_step_tolerance(options.rotation_step_tolerance);
        effective->set_degeneracy_tolerance(options.degeneracy_tolerance);
        effective->set_finite_difference_step(options.finite_difference_step);
        effective->set_initial_damping(options.initial_damping);
        effective->set_rank_tolerance(options.rank_tolerance);
        effective->set_translation_finite_difference_step(
            options.translation_finite_difference_step);
        effective->set_rotation_finite_difference_step(options.rotation_finite_difference_step);
        effective->set_rank_absolute_tolerance(options.rank_absolute_tolerance);
        effective->set_rank_relative_tolerance(options.rank_relative_tolerance);
        effective->set_gradient_tolerance(options.gradient_tolerance);
        effective->set_motion_length_scale(options.motion_length_scale);
        effective->set_motion_angle_scale(options.motion_angle_scale);
        effective->set_preference_tolerance(options.preference_tolerance);
        effective->set_objective_tolerance(options.objective_tolerance);
        effective->set_max_preference_iterations(options.max_preference_iterations);
        effective->set_max_conflict_probes(options.max_conflict_probes);
        effective->set_verify_analytic_jacobians(options.verify_analytic_jacobians);
        effective->set_jacobian_check_tolerance(options.jacobian_check_tolerance);
        std::size_t trace_bytes = 0;
        const auto debug = request->debug_options();
        const std::size_t trace_budget = debug.byte_budget() == 0
                                             ? 4U * 1024U * 1024U
                                             : static_cast<std::size_t>(std::min<uint64_t>(
                                                   debug.byte_budget(), 4U * 1024U * 1024U));
        if (request->has_debug_options()) {
            auto* effective_debug = response->mutable_effective_debug_options();
            effective_debug->set_capture_evaluations(debug.capture_evaluations());
            effective_debug->set_capture_matrices(debug.capture_matrices());
            effective_debug->set_byte_budget(trace_budget);
        }
        response->set_implementation_id(assembly_api::Solver::implementation_id());
        options.record_matrices = debug.capture_matrices();
        if (debug.capture_evaluations())
            options.record_evaluation = [&](assembly_api::EvaluationTrace trace) {
                const std::size_t charge = 1024 + trace.component_id.size() +
                                           trace.body_poses.size() * 512 +
                                           (trace.residual.size() + trace.jacobian.size()) * 32;
                if (charge > trace_budget - trace_bytes ||
                    response->evaluation_trace_size() >= 1024) {
                    response->set_trace_truncated(true);
                    return;
                }
                trace_bytes += charge;
                auto* out = response->add_evaluation_trace();
                out->set_component_id(trace.component_id);
                out->set_stage(trace.stage);
                out->set_iteration(trace.iteration);
                out->set_evaluation(trace.evaluation);
                for (const auto& traced_pose : trace.body_poses) {
                    auto* p = out->add_body_poses();
                    p->mutable_translation()->set_x(traced_pose.translation.x);
                    p->mutable_translation()->set_y(traced_pose.translation.y);
                    p->mutable_translation()->set_z(traced_pose.translation.z);
                    p->mutable_rotation()->set_x(traced_pose.rotation.x);
                    p->mutable_rotation()->set_y(traced_pose.rotation.y);
                    p->mutable_rotation()->set_z(traced_pose.rotation.z);
                    p->mutable_rotation()->set_w(traced_pose.rotation.w);
                }
                for (double v : trace.residual)
                    out->add_residual(v);
                out->set_jacobian_rows(trace.jacobian_rows);
                out->set_jacobian_columns(trace.jacobian_columns);
                for (double v : trace.jacobian)
                    out->add_jacobian(v);
            };
        const auto result = assembly_solver_.solve(model, options);
        auto* metrics = response->mutable_metrics();
        metrics->set_residual_evaluations(result.metrics.residual_evaluations);
        metrics->set_jacobian_evaluations(result.metrics.jacobian_evaluations);
        metrics->set_hot_string_lookups(result.metrics.hot_string_lookups);
        metrics->set_hot_string_bytes(result.metrics.hot_string_bytes);
        metrics->set_input_compile_ms(result.metrics.input_compile_ms);
        metrics->set_residual_ms(result.metrics.residual_ms);
        metrics->set_jacobian_ms(result.metrics.jacobian_ms);
        metrics->set_solve_ms(result.metrics.solve_ms);
        metrics->set_hard_feasibility_ms(result.metrics.hard_feasibility_ms);
        metrics->set_feasibility_retraction_ms(result.metrics.feasibility_retraction_ms);
        metrics->set_preference_ms(result.metrics.preference_ms);
        for(const auto& branch:result.distance_branches) {
            auto* output=response->add_distance_branches();output->set_constraint_id(branch.constraint_id);
            output->set_side(branch.distance_relation==assembly_api::DistanceRelation::AlongSecondNormal?
                worker_api::DISTANCE_SIDE_ALONG_SECOND_NORMAL:worker_api::DISTANCE_SIDE_OPPOSITE_SECOND_NORMAL);
        }
        for(const auto& branch:result.alignment_branches) {
            auto* output=response->add_alignment_branches();output->set_constraint_id(branch.constraint_id);
            output->set_direction(branch.direction_relation==assembly_api::DirectionRelation::Same?
                worker_api::ALIGNMENT_DIRECTION_SAME:worker_api::ALIGNMENT_DIRECTION_OPPOSITE);
        }
        if(result.interaction) {
            auto* out=response->mutable_interaction();const auto& proof=*result.interaction;
            using IS=assembly_api::InteractionStatus;
            out->set_status(proof.status==IS::Reached?worker_api::ASSEMBLY_INTERACTION_REACHED:
                proof.status==IS::Constrained?worker_api::ASSEMBLY_INTERACTION_CONSTRAINED:
                proof.status==IS::Budget?worker_api::ASSEMBLY_INTERACTION_BUDGET:
                proof.status==IS::Cancelled?worker_api::ASSEMBLY_INTERACTION_CANCELLED:worker_api::ASSEMBLY_INTERACTION_FAILED);
            out->set_hard_feasible(proof.hard_feasible);out->set_target_converged(proof.target_converged);
            out->set_eligible_for_commit(proof.eligible_for_commit);out->set_target_error(proof.target_error);
            out->set_target_optimality(proof.target_optimality);out->set_target_sequence(proof.target_sequence);
            out->set_termination_stage(proof.termination_stage);out->set_termination_reason(proof.termination_reason);
            out->set_iterations(proof.iterations);out->set_restorations(proof.restorations);
            out->set_hard_error(proof.hard_error);out->set_hold_optimality(proof.hold_optimality);out->set_hold_converged(proof.hold_converged);
            if(proof.status==IS::Cancelled)out->set_termination_reason(std::chrono::system_clock::now()>=context->deadline()?"WALL_CLOCK_DEADLINE":"CANCELLED");
        }
        const char* status =
            result.status == assembly_api::SolveStatus::Converged       ? "CONVERGED"
            : result.status == assembly_api::SolveStatus::Unsatisfied   ? "UNSATISFIED"
            : result.status == assembly_api::SolveStatus::Inconsistent  ? "INCONSISTENT"
            : result.status == assembly_api::SolveStatus::MaxIterations ? "MAX_ITERATIONS"
            : result.status == assembly_api::SolveStatus::InvalidModel  ? "INVALID_MODEL"
                                                                        : "NUMERICAL_FAILURE";
        response->set_solver_build("assembly-m4m5-reference-retraction-v13");
        response->set_status(status);
        const char* classification =
            result.classification == assembly_api::SolveClassification::SolvedFully ? "SOLVED_FULLY"
            : result.classification == assembly_api::SolveClassification::SolvedUnderConstrained
                ? "SOLVED_UNDER_CONSTRAINED"
            : result.classification == assembly_api::SolveClassification::Redundant ? "REDUNDANT"
            : result.classification == assembly_api::SolveClassification::Unsatisfied
                ? "UNSATISFIED"
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
            for (const auto& id : component.tangent_cluster_ids)
                output->add_tangent_cluster_ids(id);
            for (const auto& vector : component.null_space_basis) {
                auto* basis = output->add_null_space_basis();
                for (const double coefficient : vector)
                    basis->add_values(coefficient);
            }
            for (const double singular_value : component.singular_values)
                output->add_singular_values(singular_value);
            output->set_rank_threshold(component.rank_threshold);
            auto* preference = output->mutable_preference();
            const auto& p = component.preference;
            preference->set_status(static_cast<worker_api::AssemblyPreferenceStatus>(p.status));
            preference->set_geometrically_feasible(p.geometrically_feasible);
            preference->set_reference_objective(p.reference_objective);
            preference->set_total_objective(p.total_objective);
            preference->set_reference_optimality(p.reference_optimality);
            preference->set_total_optimality(p.total_optimality);
            preference->set_length_scale(p.length_scale);
            preference->set_angle_scale(p.angle_scale);
            preference->set_iterations(p.iterations);
            for (const auto& motion : p.bodies) {
                auto* item = preference->add_bodies();
                item->set_body_id(motion.body_id);
                item->set_role(static_cast<worker_api::AssemblyMotionRole>(motion.role));
                item->set_translation(motion.translation);
                item->set_rotation(motion.rotation);
            }
            const auto set_vec = [](worker_api::Vec3* out, const assembly_api::Vec3& v) {
                out->set_x(v.x);
                out->set_y(v.y);
                out->set_z(v.z);
            };
            for (const auto& freedom : component.freedoms) {
                auto* item = output->add_freedoms();
                item->set_body_id(freedom.body_id);
                item->set_relative_to_body_id(freedom.relative_to_body_id);
                item->set_kind(static_cast<worker_api::AssemblyFreedomKind>(freedom.kind));
                item->set_translation_dof(freedom.translation_dof);
                item->set_rotation_dof(freedom.rotation_dof);
                item->set_rank_threshold(freedom.rank_threshold);
                set_vec(item->mutable_linearization_pose()->mutable_translation(),
                        freedom.linearization_pose.translation);
                auto* rotation = item->mutable_linearization_pose()->mutable_rotation();
                rotation->set_x(freedom.linearization_pose.rotation.x);
                rotation->set_y(freedom.linearization_pose.rotation.y);
                rotation->set_z(freedom.linearization_pose.rotation.z);
                rotation->set_w(freedom.linearization_pose.rotation.w);
                for (const auto& basis : freedom.allowed_basis) {
                    auto* out = item->add_allowed_basis();
                    for (const double v : basis)
                        out->add_values(v);
                }
                for (const auto& basis : freedom.blocked_basis) {
                    auto* out = item->add_blocked_basis();
                    for (const double v : basis)
                        out->add_values(v);
                }
                for (const auto& direction : freedom.translation_directions)
                    set_vec(item->add_translation_directions(), direction);
                for (const auto& axis : freedom.rotations) {
                    auto* out = item->add_rotations();
                    set_vec(out->mutable_direction(), axis.direction);
                    set_vec(out->mutable_axis_point(), axis.axis_point);
                    out->set_pitch(axis.pitch);
                }
            }
        }
        for (const auto& id : result.redundant_constraint_ids)
            response->add_redundant_constraint_ids(id);
        for (const auto& id : result.conflicting_constraint_ids)
            response->add_conflicting_constraint_ids(id);
        for (const auto& id : result.unsatisfied_constraint_ids)
            response->add_unsatisfied_constraint_ids(id);
        for (const auto& id : result.suspected_conflicting_constraint_ids)
            response->add_suspected_conflicting_constraint_ids(id);
        for (const auto& rank : result.constraint_ranks) {
            auto* output = response->add_constraint_ranks();
            output->set_constraint_id(rank.constraint_id);
            output->set_equation_count(rank.equation_count);
            output->set_effective_rank(rank.effective_rank);
            output->set_incremental_rank(rank.incremental_rank);
            output->set_declared_generic_rank(rank.declared_generic_rank);
            output->set_role(rank.role == assembly_api::ConstraintRankRole::Independent
                                 ? "INDEPENDENT"
                             : rank.role == assembly_api::ConstraintRankRole::PartiallyRedundant
                                 ? "PARTIALLY_REDUNDANT"
                                 : "FULLY_REDUNDANT");
        }
        for (const auto& branch : result.angle_branches) {
            auto* output = response->add_angle_branches();
            output->set_constraint_id(branch.constraint_id);
            auto* state = output->mutable_state();
            state->set_wrapped_angle(branch.state.wrapped_angle);
            state->set_unwrapped_angle(branch.state.unwrapped_angle);
            state->set_winding(branch.state.winding);
        }
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

    template<typename Reader>
    occccad::kernel::BodyToolInput read_boolean_source(const worker_api::BodyToolInput& source,Reader&& read) {
    occccad::kernel::BodyToolInput tool;
    tool.body_id = source.body_id();
    tool.feature_id = source.feature_id();
    tool.result_id = source.result().result_id();
    tool.brep = read(source.brep());
    const auto bytes = read(source.naming());
    worker_api::PartTopologyManifest naming;
    if (!naming.ParseFromArray(bytes.data(), static_cast<int>(bytes.size())) ||
        naming.schema_version() != 2 || naming.brep_sha256() != source.brep().sha256())
        throw std::invalid_argument("GEOMETRY_INPUT_NAMING_SNAPSHOT_MISMATCH");
    uint32_t tip_transition=0;
    const google::protobuf::RepeatedPtrField<worker_api::NamingTopologyLocator>* tip_outputs=nullptr;
    if(source.has_result()) {
        if(!tool.body_id.empty() || source.result().feature_id()!=tool.feature_id || tool.result_id.empty() || naming.bodies_size()!=0 || naming.results_size()!=1 || naming.results(0).reference().SerializeAsString()!=source.result().SerializeAsString())
            throw std::invalid_argument("GEOMETRY_INPUT_RESULT_MISMATCH");
        tip_transition=naming.results(0).tip_transition();tip_outputs=&naming.results(0).tip();
    } else {
        if(tool.body_id.empty() || naming.results_size()!=0 || naming.bodies_size()!=1 || naming.bodies(0).body_id()!=tool.body_id)
            throw std::invalid_argument("BOOLEAN_TOOL_NAMING_SNAPSHOT_MISMATCH");
        tip_transition=naming.bodies(0).tip_transition();tip_outputs=&naming.bodies(0).tip();
    }
    if(tip_transition==0 || tip_transition>static_cast<unsigned>(naming.transitions_size()))throw std::invalid_argument("GEOMETRY_INPUT_STAGE_INVALID");
    const auto& tip=naming.transitions(static_cast<int>(tip_transition)-1);
    if(tip.feature_id()!=tool.feature_id || !tip.complete() || tip.body_id()!=tool.body_id || tip.result().SerializeAsString()!=source.result().SerializeAsString())
        throw std::invalid_argument("GEOMETRY_INPUT_STAGE_MISMATCH");
    tool.geometry_id=naming.geometry_id();
    for (const auto& output : *tip_outputs) {
        if (output.semantic_ref() == 0 ||
            output.semantic_ref() >
                static_cast<unsigned>(naming.semantic_refs_size()))
            throw std::invalid_argument("BOOLEAN_TOOL_NAMING_REF_INVALID");
        const auto& ref = naming.semantic_refs(output.semantic_ref() - 1);
        const auto type =
            output.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_FACE
                ? occccad::kernel::TopologyType::FACE
            : output.topology_type() == worker_api::PERSISTENT_TOPOLOGY_TYPE_EDGE
                ? occccad::kernel::TopologyType::EDGE
                : occccad::kernel::TopologyType::VERTEX;
        tool.topology.push_back({ref.feature_id(),
                 ref.output_slot(),
                 {ref.source_ids().begin(), ref.source_ids().end()},
                 type,
                 output.local_id()});
    }
    return tool;
    }
    template<class Reader>
    occccad::kernel::GeometryFeatureInput read_geometry_input(const worker_api::GeometryFeatureInput& input,Reader&& read) {
        occccad::kernel::GeometryFeatureInput result;
        const auto vec=[](const worker_api::Vec3& v){return occccad::kernel::Vec3{v.x(),v.y(),v.z()};};
        if(input.has_plane()) {
            if(input.has_source() || input.members_size()!=0)throw std::invalid_argument("GEOMETRY_PLANE_INPUT_AMBIGUOUS");
            const auto& p=input.plane();result.plane=occccad::kernel::GeometryPlane{vec(p.origin()),vec(p.normal()),vec(p.u_direction()),p.id()};
        } else result.source=read_boolean_source(input.source(),read);
        if(input.members_size()>4096)throw std::invalid_argument("GEOMETRY_MEMBER_BUDGET");
        for(const auto& member:input.members())result.members.push_back({member.feature_id(),member.output_slot(),{member.source_ids().begin(),member.source_ids().end()}});
        result.reversed=input.reversed();return result;
    }
    template<class Reader>
    occccad::kernel::GeometryFeatureSpec read_geometry_feature(const worker_api::GeometryFeatureSpec& input,Reader&& read) {
        if(input.inputs_size()>128 || input.curve().points_size()>4096 || input.curve().knots_size()>4096 || input.sketch_wire().curves_size()>4096)throw std::invalid_argument("GEOMETRY_INPUT_BUDGET");
        const auto vec=[](const worker_api::Vec3& v){return occccad::kernel::Vec3{v.x(),v.y(),v.z()};};
        occccad::kernel::GeometryFeatureSpec result;
        result.feature_id = input.feature_id();
        result.result_id = input.result_id();
        result.operation = input.operation();
        result.construction_tolerance =
            input.construction_tolerance() > 0 ? input.construction_tolerance() : 1e-7;
        for(const auto& v:input.inputs())result.inputs.push_back(read_geometry_input(v,read));
        const auto& c=input.curve();for(const auto& v:c.points()){occccad::kernel::GeometryControlPoint point{v.id(),vec(v.position()),{}};for(const auto& r:v.sources())point.sources.push_back({r.feature_id(),r.output_slot(),{r.source_ids().begin(),r.source_ids().end()}});result.curve.points.push_back(std::move(point));}
        result.curve.normal=vec(c.normal());result.curve.x_direction=vec(c.x_direction());result.curve.radius=c.radius();result.curve.start_angle=c.start_angle();result.curve.end_angle=c.end_angle();result.curve.degree=c.degree();
        result.curve.knots={c.knots().begin(),c.knots().end()};result.curve.multiplicities={c.multiplicities().begin(),c.multiplicities().end()};result.curve.weights={c.weights().begin(),c.weights().end()};result.curve.closed=c.closed();
        const auto& p=input.parameters();result.parameters.direction=vec(p.direction());result.parameters.origin=vec(p.origin());result.parameters.distance=p.distance();result.parameters.angle=p.angle();result.parameters.start=p.start();result.parameters.end=p.end();result.parameters.side=p.side();result.parameters.tension=p.tension();
        const auto& w=input.sketch_wire();result.sketch_wire.sketch_id=w.sketch_id();result.sketch_wire.origin=vec(w.origin());result.sketch_wire.normal=vec(w.normal());result.sketch_wire.u_direction=vec(w.u_direction());for(const auto& c2:w.curves())result.sketch_wire.curves.push_back(exact_sketch_curve(c2));
        const auto& surface = input.surface();
        result.surface.ruled = surface.ruled();
        result.surface.frame_mode = surface.frame_mode();
        result.surface.tolerance = surface.tolerance();
        result.surface.has_keep_point = surface.has_keep_point();
        result.surface.has_second_keep_point = surface.has_second_keep_point();
        if (surface.has_second_keep_point())
            result.surface.second_keep_point = vec(surface.second_keep_point());
        result.surface.keep_point = vec(surface.keep_point());
        if(surface.sections_size()>32||surface.boundaries_size()>128)throw std::invalid_argument("SURFACE_CONTROL_BUDGET");
        if(surface.has_spine())result.surface.spine=read_geometry_input(surface.spine(),read);
        if(surface.has_guide())result.surface.guide=read_geometry_input(surface.guide(),read);
        for(const auto& c:surface.sections()){
            occccad::kernel::GeometrySectionControl control;control.id=c.id();control.section_index=c.section_index();
            if(c.has_closing_point())control.closing_point=vec(c.closing_point());
            if(c.has_station_point())control.station_point=vec(c.station_point());
            if(c.couplings_size()>16)throw std::invalid_argument("SURFACE_COUPLING_LIMIT");
            for(const auto& p:c.couplings())control.couplings.push_back({p.id(),vec(p.position())});
            result.surface.sections.push_back(std::move(control));
        }
        for(const auto& c:surface.boundaries()){
            occccad::kernel::GeometryBoundaryControl control;control.id=c.id();control.boundary_index=c.boundary_index();control.continuity=c.continuity();control.tension=c.tension();
            if(c.has_support())control.support=read_geometry_input(c.support(),read);
            result.surface.boundaries.push_back(std::move(control));
        }
        if(input.has_import_seed()){
            auto seed=input.import_seed();if(seed.has_identity_artifact()){
                const auto data=read(seed.identity_artifact());worker_api::ImportTopologySeed full;
                if(!full.ParseFromArray(data.data(),static_cast<int>(data.size()))||full.feature_id()!=seed.feature_id()||full.brep_sha256()!=seed.brep_sha256()||full.body_id()!=seed.body_id()||full.result_id()!=seed.result_id())throw std::invalid_argument("IMPORT_SEED_ARTIFACT_MISMATCH");seed=std::move(full);
            }
            if(seed.occt_version()!=OCC_VERSION_COMPLETE||seed.policy_id()!="occccad.import.frozen-brep.v1")throw std::invalid_argument("IMPORT_SEED_CONTRACT_MISMATCH");
            occccad::kernel::ImportTopologySeed frozen;frozen.feature_id=seed.feature_id();frozen.body_id=seed.body_id();frozen.result_id=seed.result_id();frozen.brep_sha256=seed.brep_sha256();
            for(const auto& entry:seed.identities())frozen.identities.push_back({entry.stable_id(),static_cast<occccad::kernel::PersistentTopologyType>(entry.topology_type()),entry.local_id()});
            result.import_seed=std::move(frozen);result.import_brep=read(input.import_brep());
        }
        return result;
    }

    grpc::Status AnalyzeBooleanRegions(grpc::ServerContext* context,const worker_api::AnalyzeBooleanRegionsRequest* request,
        worker_api::AnalyzeBooleanRegionsResponse* response) override {
        if(request->request_id().empty()||request->output_prefix().empty()||!request->has_target()||!request->has_tool())return {grpc::StatusCode::INVALID_ARGUMENT,"Boolean partition inputs required"};
        std::unique_lock<std::mutex> lock(mutex_,std::defer_lock);
        while(!lock.try_lock()){if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"cancelled"};std::this_thread::sleep_for(std::chrono::milliseconds(5));}
        try {
            const auto read=[this](const auto& ref){return read_input(ref,false);};
            const auto target=read_boolean_source(request->target(),read),tool=read_boolean_source(request->tool(),read);
            if(target.body_id==tool.body_id)throw std::invalid_argument("BOOLEAN_REGION_DISTINCT_BODIES_REQUIRED");
            const auto regions=kernel_.analyze_boolean_regions(target,tool,[context](){return context->IsCancelled();});
            for(const auto& cell:regions){
                if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"cancelled"};
                auto* value=response->add_regions();value->set_in_target(cell.in_target);value->set_in_tool(cell.in_tool);
                auto* ref=value->mutable_reference();for(const auto& token:cell.reference.boundary)ref->add_boundary(token);
                ref->set_boundary_unique(cell.reference.boundary_unique);ref->set_input_digest(cell.reference.input_digest);ref->set_cell_digest(cell.reference.cell_digest);
                const auto prefix=request->output_prefix()+"/"+cell.reference.cell_digest.substr(7);
                fill_evaluation(cell.geometry_id,cell.geometry_id,0.1,0.5,value->mutable_geometry(),prefix+".brep",prefix+".glb");
            }
            return grpc::Status::OK;
        } catch(const std::invalid_argument& e){return {grpc::StatusCode::INVALID_ARGUMENT,e.what()};}
          catch(const std::exception& e){return {context->IsCancelled()?grpc::StatusCode::CANCELLED:grpc::StatusCode::INTERNAL,e.what()};}
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
            request->profile_pads().empty() && !request->has_import_seed() && !request->has_geometry_feature()) {
            return {grpc::StatusCode::INVALID_ARGUMENT, "feature chain requires at least one pad"};
        }

        if (!request->exact_only() && request->brep_output_key().empty() != request->glb_output_key().empty())
            return {grpc::StatusCode::INVALID_ARGUMENT, "persistent evaluation requires both BREP and visual output keys"};
        std::unique_lock<std::mutex> lock(mutex_,std::defer_lock);
        while(!lock.try_lock()) {
            if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"evaluation cancelled in queue"};
            std::this_thread::sleep_for(std::chrono::milliseconds(5));
        }
        const auto acquired=std::chrono::steady_clock::now();
        const bool external_outputs =
            !request->brep_output_key().empty() || !request->glb_output_key().empty();
        if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED, "request cancelled while queued"};
        try {
            double input_io_ms=0;
            const auto input_artifact=[&](const auto& ref){const auto began=std::chrono::steady_clock::now();auto bytes=read_input(ref,request->force_cold());input_io_ms+=std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-began).count();return bytes;};
            const auto publish=[&](const auto& key,const auto& bytes,const auto& type,auto* ref){const auto began=std::chrono::steady_clock::now();if(context->IsCancelled())throw std::runtime_error("EVALUATION_CANCELLED");write_artifact(key,bytes,type,ref);auto* timings=response->mutable_runtime_stats();timings->set_artifact_io_ms(timings->artifact_io_ms()+std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-began).count());};
            // The lock coalesces in-flight work; the key includes the actual frozen
            // inputs, naming policy and precision, never only a caller-supplied key.
            auto canonical = *request;
            canonical.clear_request_id(); canonical.clear_geometry_key();
            canonical.clear_brep_output_key(); canonical.clear_glb_output_key();
            canonical.clear_force_cold();canonical.clear_runtime_affinity();
            const auto request_key = occccad::kernel::make_geometry_id(canonical.SerializeAsString()) + (external_outputs ? ":persistent" : ":transient");
            const auto cached = cache_.find(request_key);
            if (!request->force_cold() && cached != cache_.end()) {
                cached->second.used=++cache_clock_;
                response->CopyFrom(cached->second.response);
                const auto old_stats=response->runtime_stats();response->clear_runtime_stats();
                response->set_geometry_key(request->geometry_key());
                if (external_outputs && cached->second.brep && !cached->second.brep->empty()) {
                    publish(request->brep_output_key(), *cached->second.brep, "application/vnd.opencascade.brep", response->mutable_brep_artifact());
                    if(!request->exact_only())publish(request->glb_output_key(), *cached->second.glb, "model/gltf-binary", response->mutable_glb_artifact());
                    if (!cached->second.naming.empty()) publish(request->brep_output_key()+".naming.pb", cached->second.naming,
                        "application/vnd.occccad.topology-manifest.v2+protobuf", response->mutable_evaluation_manifest()->mutable_topology_manifest_artifact());
                    response->set_representation_kind(request->exact_only()?"EXACT_STAGE":"PERSISTENT"); response->clear_preview_mesh();
                }
                if (!response->geometry_id().empty()) {

                    auto* stats=response->mutable_runtime_stats();
                    stats->set_stages_reused(request->profile_pads_size());
                    for(const auto& pad:request->profile_pads())stats->add_reused_feature_ids(pad.feature_id());
                    for(const auto& stage:old_stats.stages()){auto* out=stats->add_stages();*out=stage;out->set_reused(true);}
                    stats->set_queue_ms(std::chrono::duration<double,std::milli>(acquired-started).count());
                    stats->set_stage_cache_bytes(kernel_.runtime_cache_bytes());
                    stats->set_response_cache_bytes(cache_bytes_);stats->set_visual_cache_bytes(visual_bytes_);
#if defined(__linux__)
                    struct rusage usage{};if(getrusage(RUSAGE_SELF,&usage)==0)stats->set_peak_rss_bytes(static_cast<uint64_t>(usage.ru_maxrss)*1024);
#endif
                    response->set_cache_hit(true);
                    log_rpc("EvaluatePart", request->request_id(), traceparent, "CACHE_HIT", started);
                    return grpc::Status::OK;
                }
            }

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
                base_brep = input_artifact(request->base_brep_artifact());
            std::vector<occccad::kernel::ProfilePadSpec> profile_specs;
            if (!request->profile_pads().empty() || request->has_import_seed() || request->has_geometry_feature()) {
                const auto& policy = request->topology_policy();
                if (policy.schema_version() != occccad::kernel::topology_naming_schema_version ||
                    policy.policy_id() != occccad::kernel::topology_naming_policy_id ||
                    policy.evaluator_version() != occccad::kernel::topology_evaluator_version ||
                    policy.linear_tolerance_meters() !=
                        occccad::kernel::topology_linear_tolerance_meters ||
                    policy.angular_tolerance_radians() !=
                        occccad::kernel::topology_angular_tolerance_radians ||
                    policy.policy_digest() !=
                        occccad::kernel::topology_naming_policy_digest) {
                    throw std::invalid_argument("unsupported or incomplete topology naming policy");
                }
            }
            for (const auto& input : request->profile_pads()) {
                if (input.units() != "mm")
                    throw std::invalid_argument("profile pads must use mm");
                if (input.feature_id().empty() || input.body_id().empty() ||
                    (input.profile_feature_id().empty() &&
                     (input.generator() == "LINEAR_EXTRUDE" || input.generator() == "REVOLVE" ||
                      input.generator().empty()))) {
                    throw std::invalid_argument(
                        "profile pads require feature_id, body_id, and profile_feature_id");
                }
                occccad::kernel::ProfilePadSpec pad;
                const auto read_boundary=[](const worker_api::LoftBoundarySpec& b){occccad::kernel::LoftBoundarySpec out;if(b.supports_size()>128)throw std::invalid_argument("LOFT_SUPPORT_BUDGET");out.condition=b.condition();out.magnitude=b.magnitude();out.reversed=b.reversed();out.direction={b.direction().x(),b.direction().y(),b.direction().z()};for(const auto& r:b.supports())out.supports.push_back({r.feature_id(),r.output_slot(),{r.source_ids().begin(),r.source_ids().end()}});return out;};
 if(input.has_geometry_generator())pad.geometry_generator=std::make_shared<occccad::kernel::GeometryFeatureSpec>(read_geometry_feature(input.geometry_generator(),input_artifact));
 pad.loft_start=read_boundary(input.loft_start());pad.loft_end=read_boundary(input.loft_end());
 pad.drill_tip_angle=input.drill_tip_angle();pad.depth_convention=input.depth_convention();
 if(input.hole_positions_size()>128)throw std::invalid_argument("HOLE_POSITION_BUDGET");
 for(const auto& p:input.hole_positions()){occccad::kernel::HolePositionSpec position;position.profile_feature_id=p.profile_feature_id();position.region.id=p.region().id();position.region.outer=read_loop(p.region().outer());for(const auto& h:p.region().holes())position.region.holes.push_back(read_loop(h));position.origin={p.origin().x(),p.origin().y(),p.origin().z()};position.normal={p.normal().x(),p.normal().y(),p.normal().z()};position.u_direction={p.u_direction().x(),p.u_direction().y(),p.u_direction().z()};pad.hole_positions.push_back(std::move(position));}
 pad.hole_kind=input.hole_kind();pad.recess_diameter=input.recess_diameter();pad.recess_depth=input.recess_depth();pad.countersink_angle=input.countersink_angle();
                pad.explicit_boolean_regions=input.explicit_boolean_regions();
                for(const auto& ref:input.boolean_regions())pad.boolean_regions.push_back({ref.boundary_unique(),{ref.boundary().begin(),ref.boundary().end()},ref.input_digest(),ref.cell_digest()});
                if(pad.explicit_boolean_regions&&(input.generator()!="BOOLEAN"||input.tools_size()!=1))throw std::invalid_argument("BOOLEAN_REGIONS_REQUIRE_ONE_TOOL");
                pad.feature_id = input.feature_id();
                pad.body_id = input.body_id();
                pad.input_feature_id = input.input_feature_id();
                pad.profile_feature_id = input.profile_feature_id();
                pad.pattern_source_feature_id = input.pattern_source_feature_id();
                pad.pattern_source_kind = input.pattern_source_kind();
                pad.pattern_start_feature_id = input.pattern_start_feature_id();
                pad.pattern_result_mode = input.pattern_result_mode();
                if(input.additional_pattern_sources_size()>31)throw std::invalid_argument("PATTERN_SEED_SET_INVALID");
                for(const auto& source:input.additional_pattern_sources())pad.additional_pattern_sources.push_back({source.feature_id(),source.source_kind(),source.start_feature_id()});
                if (input.pattern_placements_size() > 256)
                    throw std::invalid_argument("PATTERN_MEMBER_BUDGET");
                for (const auto& placement : input.pattern_placements()) {
                    if (placement.matrix_size() != 12)
                        throw std::invalid_argument("PATTERN_TRANSFORM_REQUIRED");
                    occccad::kernel::PatternPlacement member;
                    member.slot = placement.slot();
                    std::copy(placement.matrix().begin(), placement.matrix().end(), member.matrix.begin());
                    pad.pattern_placements.push_back(member);
                }
                pad.pad_length = input.pad_length();
                pad.plane = input.plane().empty() ? "XY" : input.plane();
                pad.body_operation =
                    input.body_operation().empty() ? "ADD" : input.body_operation();
                pad.generator = input.generator().empty() ? "LINEAR_EXTRUDE" : input.generator();
                pad.revolve_angle = input.revolve_angle();
                pad.axis_start = {input.axis_start().x(), input.axis_start().y()};
                pad.axis_end = {input.axis_end().x(), input.axis_end().y()};
                pad.reversed = input.reversed();
                for (const auto& ref : input.selections())
                    pad.selections.push_back({ref.feature_id(),
                                              ref.output_slot(),
                                              {ref.source_ids().begin(), ref.source_ids().end()}});
                pad.neutral_origin = {input.neutral_origin().x(), input.neutral_origin().y(),
                                      input.neutral_origin().z()};
                pad.neutral_normal = {input.neutral_normal().x(), input.neutral_normal().y(),
                                      input.neutral_normal().z()};
                pad.termination_origin = {input.termination_origin().x(), input.termination_origin().y(), input.termination_origin().z()};
                pad.termination_normal = {input.termination_normal().x(), input.termination_normal().y(), input.termination_normal().z()};
                pad.start_offset = input.start_offset(); pad.end_offset = input.end_offset();
                for(const auto& value:input.profile_origins()) {
                    occccad::kernel::ProfileOrigin origin;origin.derived_id=value.derived_id();
                    for(const auto& source:value.sources())origin.sources.push_back({source.feature_id(),source.output_slot(),{source.source_ids().begin(),source.source_ids().end()}});
                    pad.profile_origins.push_back(std::move(origin));
                }
                if(input.has_sweep_path()) {
 const auto& p=input.sweep_path(); if(p.curves_size()>256)throw std::invalid_argument("SWEEP_PATH_BUDGET");
 auto& path=pad.sweep_path;if(p.has_exact_path())path.exact_path=std::make_shared<occccad::kernel::GeometryFeatureInput>(read_geometry_input(p.exact_path(),input_artifact));path.sketch_id=p.sketch_id();path.chain_id=p.chain_id();path.closed=p.closed();path.frame_mode=p.frame_mode();path.corner_mode=p.corner_mode();
 path.plane_origin={p.plane_origin().x(),p.plane_origin().y(),p.plane_origin().z()};path.plane_normal={p.plane_normal().x(),p.plane_normal().y(),p.plane_normal().z()};path.plane_u_direction={p.plane_u_direction().x(),p.plane_u_direction().y(),p.plane_u_direction().z()};
 for(const auto& c:p.curves())path.curves.push_back(exact_sketch_curve(c));
 }
 pad.termination_surface = {input.termination_surface().feature_id(), input.termination_surface().output_slot(), {input.termination_surface().source_ids().begin(), input.termination_surface().source_ids().end()}};
                pad.extent = input.extent();
                pad.second_length = input.second_length();
                pad.plane_origin = {input.plane_origin().x(), input.plane_origin().y(),
                                    input.plane_origin().z()};
                pad.plane_normal = {input.plane_normal().x(), input.plane_normal().y(),
                                    input.plane_normal().z()};
                pad.plane_u_direction = {input.plane_u_direction().x(),
                                         input.plane_u_direction().y(),
                                         input.plane_u_direction().z()};
                for (const auto& region_input : input.regions()) {
                    occccad::kernel::ProfileRegionSpec region;
                    region.id = region_input.id();
                    region.outer = read_loop(region_input.outer());
                    for (const auto& hole : region_input.holes())
                        region.holes.push_back(read_loop(hole));
                    pad.regions.push_back(std::move(region));
                }
                pad.ruled = input.ruled();
                for (const auto& source : input.sections()) {
                    auto section = read_loft_section(source);
                    pad.sections.push_back(std::move(section));
                }
                for (const auto& source : input.tools()) {
                    pad.tools.push_back(read_boolean_source(source,input_artifact));
                }
                profile_specs.push_back(std::move(pad));
            }
            occccad::kernel::ImportTopologySeed seed;
            if (request->has_import_seed()) {
                if (!specs.empty())
                    throw std::invalid_argument("import naming seed requires the profile feature contract");
                auto input = request->import_seed();
                if (input.has_identity_artifact()) {
                    const auto data = input_artifact(input.identity_artifact());
                    worker_api::ImportTopologySeed full;
                    if (!full.ParseFromArray(data.data(), static_cast<int>(data.size())) || full.feature_id()!=input.feature_id() || full.brep_sha256()!=input.brep_sha256() || full.body_id()!=input.body_id() || full.result_id()!=input.result_id())
                        throw std::invalid_argument("IMPORT_SEED_ARTIFACT_MISMATCH");
                    input = std::move(full);
                }
                if(input.body_id().empty() || !input.result_id().empty())throw std::invalid_argument("IMPORT_SEED_CONTRACT_MISMATCH");
                if (input.occt_version() != OCC_VERSION_COMPLETE || input.policy_id() != "occccad.import.frozen-brep.v1")
                    throw std::invalid_argument("IMPORT_SEED_CONTRACT_MISMATCH");
                seed.feature_id = input.feature_id();
                seed.body_id = input.body_id();
                seed.brep_sha256 = input.brep_sha256();
                for (const auto& entry : input.identities())
                    seed.identities.push_back({entry.stable_id(),
                        static_cast<occccad::kernel::PersistentTopologyType>(entry.topology_type()), entry.local_id()});
            }
            occccad::kernel::PartRuntimeStats runtime_stats;
            occccad::kernel::PartRuntimeOptions runtime;
            runtime.force_cold=request->force_cold();runtime.stats=&runtime_stats;
            const auto cancellation=std::make_shared<occccad::worker::CancellationPolling>([context](){return context->IsCancelled();});
            runtime.cancelled=[cancellation](){return cancellation->poll();};
            runtime.fresh_cancelled=[cancellation](){return cancellation->check();};
            if(request->has_base_stage() && !runtime.force_cold) {
                if(!request->has_import_seed() || base_brep.empty())throw std::invalid_argument("BASE_STAGE_REQUIRES_IMPORT");
                const auto& source=request->base_stage();
                const auto data=input_artifact(source.naming());
                worker_api::PartTopologyManifest snapshot;
                const auto geometry=occccad::kernel::make_geometry_id(base_brep.data(),base_brep.size());
                if(!snapshot.ParseFromArray(data.data(),static_cast<int>(data.size())) || snapshot.schema_version()!=2 ||
                   source.geometry_id()!=geometry ||
                   snapshot.policy_digest()!=request->topology_policy().policy_digest() || snapshot.length_unit()!="mm" || snapshot.coordinate_space()!="PART_LOCAL" ||
                   snapshot.bodies_size()!=1 || snapshot.results_size()!=0 || snapshot.bodies(0).body_id()!=seed.body_id)
                    throw std::invalid_argument("BASE_STAGE_SNAPSHOT_MISMATCH");
                const worker_api::NamingTransition* found_root=nullptr;
                for(const auto& candidate:snapshot.transitions())if(candidate.feature_id()==seed.feature_id) {
                    if(found_root)throw std::invalid_argument("BASE_STAGE_DUPLICATE_ROOT");
                    found_root=&candidate;
                }
                if(!found_root)throw std::invalid_argument("BASE_STAGE_ROOT_MISSING");
                const auto& transition=*found_root;
                if(transition.feature_id()!=seed.feature_id || transition.body_id()!=seed.body_id || transition.result_geometry_id()!=geometry ||
                   !transition.complete() || !transition.input_geometry_id().empty() || transition.has_result() ||
                   transition.deleted_size()!=0 || transition.ambiguous_size()!=0)
                    throw std::invalid_argument("BASE_STAGE_ROOT_MISMATCH");
                occccad::kernel::PartBaseStage frozen;frozen.geometry_id=geometry;frozen.volume=source.volume();
                auto& root=frozen.root;root.feature_id=seed.feature_id;root.body_id=seed.body_id;root.result_geometry_id=geometry;
                root.topology_history_complete=true;root.topology_history.feature_id=seed.feature_id;
                root.topology_history.result_geometry_id=geometry;root.topology_history.policy_digest=request->topology_policy().policy_digest();
                root.topology_history.evidence_digest=transition.evidence_digest();
                std::unordered_map<std::string,const worker_api::NamingLineage*> root_evidence;
                for(const auto& lineage:transition.lineage()) {
                    if(lineage.sources_size()!=0 || lineage.kind()!=worker_api::TOPOLOGY_LINEAGE_GENERATED || !lineage.result() ||
                       lineage.result()>static_cast<unsigned>(snapshot.semantic_refs_size()) || !lineage.evidence() ||
                       lineage.evidence()>static_cast<unsigned>(snapshot.evidence_size()) ||
                       !root_evidence.emplace(snapshot.semantic_refs(lineage.result()-1).SerializeAsString(),&lineage).second)
                        throw std::invalid_argument("BASE_STAGE_LOCATOR_INVALID");
                }
                if(root_evidence.size()!=seed.identities.size())throw std::invalid_argument("BASE_STAGE_COVERAGE_MISMATCH");
                for(const auto& identity:seed.identities) {
                    worker_api::SemanticTopologyRef ref;ref.set_feature_id(seed.feature_id);ref.set_output_slot("import.topology");ref.add_source_ids(identity.stable_id);
                    const auto found=root_evidence.find(ref.SerializeAsString());
                    if(found==root_evidence.end())throw std::invalid_argument("BASE_STAGE_IDENTITY_MISMATCH");
                    occccad::kernel::SemanticTopologyOutput restored;
                    restored.semantic_ref={ref.feature_id(),ref.output_slot(),{ref.source_ids().begin(),ref.source_ids().end()}};
                    restored.topology_type=identity.topology_type;restored.local_id=identity.local_id;
                    restored.evidence=occccad::worker::read_naming_evidence(snapshot.evidence(found->second->evidence()-1),snapshot);
                    root.topology_history.lineage.push_back({{},restored.semantic_ref,occccad::kernel::TopologyLineageKind::generated,restored.evidence});
                    root.semantic_outputs.push_back(std::move(restored));
                }
                runtime.base_stage=std::move(frozen);
            }
            // Canonical keys include actual immutable source digests, stable Body /
            // Feature identity and the naming policy; transport IDs/outputs do not.
            std::ostringstream base_facts;
            if(runtime.base_stage)base_facts<<std::setprecision(std::numeric_limits<double>::max_digits10)
                <<runtime.base_stage->geometry_id<<':'<<runtime.base_stage->volume<<':'
                <<runtime.base_stage->root.topology_history.evidence_digest;
            std::string prefix=occccad::kernel::make_geometry_id(base_brep.data(),base_brep.size())+
                request->topology_policy().SerializeAsString()+request->import_seed().SerializeAsString()+
                base_facts.str()+"stage-runtime-v7-frozen-root-facts";
            for(const auto& pad:request->profile_pads()) {
                prefix=occccad::kernel::make_geometry_id(prefix+pad.SerializeAsString());
                runtime.stage_keys.push_back(prefix);
            }
            const auto exact_started=std::chrono::steady_clock::now();
            const double queue_ms=std::chrono::duration<double,std::milli>(acquired-started).count();
            occccad::kernel::ProfileEvaluationResult profile_evaluation;
            if(request->has_geometry_feature() && (!profile_specs.empty() || request->has_import_seed() || !specs.empty() || !base_brep.empty()))throw std::invalid_argument("GEOMETRY_REQUEST_VARIANT_AMBIGUOUS");
            const auto geometry_id = request->has_geometry_feature()
                ? (profile_evaluation=kernel_.evaluateGeometryFeature(read_geometry_feature(request->geometry_feature(),input_artifact),runtime),profile_evaluation.geometry_id)
                : (profile_specs.empty() && !request->has_import_seed())
                    ? kernel_.evaluateRectangularPads(specs, base_brep)
                    : (profile_evaluation =
                           kernel_.evaluateProfilePadsWithHistory(profile_specs, base_brep, request->has_import_seed() ? &seed : nullptr,runtime),
                       profile_evaluation.geometry_id);
            if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"evaluation cancelled before display"};
            std::shared_ptr<const std::vector<uint8_t>> cached_brep,cached_glb;
            std::vector<uint8_t> cached_naming;
            auto* stats=response->mutable_runtime_stats();
            stats->set_artifact_io_ms(input_io_ms);
            stats->set_queue_ms(queue_ms);
            stats->set_input_prepare_ms(std::chrono::duration<double,std::milli>(exact_started-acquired).count());
            stats->set_exact_ms(runtime_stats.exact_ms);
            stats->set_naming_ms(runtime_stats.naming_ms);
            stats->set_naming_kernel_ms(runtime_stats.naming_ms);
            stats->set_base_ms(runtime_stats.base_ms);stats->set_adjacency_ms(runtime_stats.adjacency_ms);
            stats->set_volume_integrations(runtime_stats.volume_integrations);
            stats->set_face_evidence_computed(runtime_stats.face_evidence_computed);
            stats->set_face_evidence_reused(runtime_stats.face_evidence_reused);
            stats->set_base_stage_restored(runtime_stats.base_stage_restored);
            stats->set_stages_executed(runtime_stats.stages_executed);stats->set_stages_reused(runtime_stats.stages_reused);
            stats->set_generator_calls(runtime_stats.generator_calls);stats->set_modifier_calls(runtime_stats.modifier_calls);
            stats->set_body_operation_calls(runtime_stats.body_operation_calls);stats->set_stage_cache_bytes(runtime_stats.stage_cache_bytes);
            stats->set_stage_evictions(runtime_stats.stage_evictions);
            for(const auto& stage:runtime_stats.stages) {
                auto* out=stats->add_stages();out->set_feature_id(stage.feature_id);out->set_input_digest(stage.input_digest);
                out->set_geometry_id(stage.geometry_id);out->set_reused(stage.reused);
            }
            for(const auto& id:runtime_stats.executed_feature_ids)stats->add_executed_feature_ids(id);
            for(const auto& id:runtime_stats.reused_feature_ids)stats->add_reused_feature_ids(id);
            const auto display_started=std::chrono::steady_clock::now();
            fill_evaluation(request->geometry_key(), geometry_id, request->linear_deflection(),
                            request->angular_deflection(), response, request->brep_output_key(),
                            request->glb_output_key(),request->exact_only(),request->force_cold(),&cached_brep,&cached_glb,runtime.cancelled,runtime.fresh_cancelled);
            stats->set_display_ms(std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-display_started).count());
            if (!profile_specs.empty() || request->has_import_seed() || request->has_geometry_feature()) {
                auto* manifest = response->mutable_evaluation_manifest();
                manifest->set_schema_version(occccad::kernel::topology_naming_schema_version);
                manifest->set_topology_policy_id(request->topology_policy().policy_id());
                manifest->set_topology_evaluator_version(
                    request->topology_policy().evaluator_version());
                manifest->set_topology_policy_digest(request->topology_policy().policy_digest());
                if(request->has_geometry_feature()) {
                    auto* identity=manifest->add_features();identity->set_feature_id(request->geometry_feature().feature_id());
                    identity->mutable_result()->set_feature_id(request->geometry_feature().feature_id());identity->mutable_result()->set_result_id(request->geometry_feature().result_id());
                }
                if (request->has_import_seed()) {
                    auto* identity = manifest->add_features();
                    identity->set_feature_id(seed.feature_id);
                    identity->set_body_id(seed.body_id);
                }
                for (const auto& spec : profile_specs) {
                    auto* identity = manifest->add_features();
                    identity->set_feature_id(spec.feature_id);
                    identity->set_body_id(spec.body_id);
                    identity->set_input_feature_id(spec.input_feature_id);
                    identity->set_profile_feature_id(spec.profile_feature_id);
                }
                const auto naming_started=std::chrono::steady_clock::now();
                std::string topology_bytes;
                const auto naming_key=prefix+request->geometry_feature().SerializeAsString()+"|"+response->brep_artifact().sha256();
                const auto naming_hit=naming_cache_.find(naming_key);
                if(!request->force_cold()&&naming_hit!=naming_cache_.end()&&naming_hit->second.geometry_id==geometry_id) {
                    topology_bytes=naming_hit->second.bytes;naming_hit->second.used=++cache_clock_;
                } else {
                worker_api::PartTopologyManifest topology_manifest;
                topology_manifest.set_schema_version(
                    occccad::kernel::topology_naming_schema_version);
                topology_manifest.set_policy_id(request->topology_policy().policy_id());
                topology_manifest.set_evaluator_version(
                    request->topology_policy().evaluator_version());
                topology_manifest.set_policy_digest(request->topology_policy().policy_digest());
                std::vector<worker_api::FeatureResult> working_results;
                for (const auto& feature : profile_evaluation.feature_results) {
                    working_results.emplace_back();
                    fill_feature_result(feature, &working_results.back());
                }
                occccad::worker::pack_naming(working_results, topology_manifest);
                topology_manifest.set_geometry_id(geometry_id);
                topology_manifest.set_brep_sha256(response->brep_artifact().sha256());
                if (!topology_manifest.SerializeToString(&topology_bytes)) {
                    throw std::runtime_error("topology manifest serialization failed");
                }

                    if(!request->force_cold()&&topology_bytes.size()<=cache_budget_/2) {
                        const auto bytes=topology_bytes.capacity()+naming_key.capacity()+geometry_id.capacity()+sizeof(NamingSnapshot);
                        if (auto existing=naming_cache_.find(naming_key);existing!=naming_cache_.end()) {
                            naming_bytes_-=existing->second.charge;naming_cache_.erase(existing);
                        }
                        while(!naming_cache_.empty()&&naming_bytes_+bytes>cache_budget_/2) {
                            auto oldest=std::min_element(naming_cache_.begin(),naming_cache_.end(),[](const auto& a,const auto& b){return a.second.used<b.second.used;});
                            naming_bytes_-=oldest->second.charge;naming_cache_.erase(oldest);
                        }
                        if(bytes<=cache_budget_/2){naming_bytes_+=bytes;naming_cache_.emplace(naming_key,NamingSnapshot{geometry_id,topology_bytes,bytes,++cache_clock_});}
                    }
                }
                const auto packing_ms=std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-naming_started).count();
                stats->set_naming_artifact_ms(packing_ms);stats->set_naming_ms(stats->naming_ms()+packing_ms);
                const auto topology_id = occccad::kernel::make_geometry_id(topology_bytes);
                manifest->set_topology_manifest_digest(
                    topology_id.rfind("sha256:", 0) == 0 ? topology_id.substr(7) : topology_id);
                cached_naming.assign(topology_bytes.begin(),topology_bytes.end());
                if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"evaluation cancelled before naming publication"};
                if (external_outputs) {
                    const std::vector<uint8_t>& bytes=cached_naming;
                    publish(request->brep_output_key() + ".naming.pb", bytes,
                                   "application/vnd.occccad.topology-manifest.v2+protobuf",
                                   manifest->mutable_topology_manifest_artifact());
                }
            }

            if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,"evaluation cancelled before result admission"};
            const auto bytes = response->ByteSizeLong() + response->brep_artifact().size_bytes()
                + response->glb_artifact().size_bytes() + response->evaluation_manifest().topology_manifest_artifact().size_bytes();
            if (!request->force_cold() && bytes <= cache_budget_) {
                EvaluationCache entry;
                entry.response = *response;
                if (external_outputs) {
                    entry.brep=std::move(cached_brep);entry.glb=std::move(cached_glb);entry.naming=std::move(cached_naming);
                }
                // Reject oversized entries before reading artifacts back. A hit
                // never depends on prior attempt files remaining on disk.
                while(!cache_.empty() && cache_bytes_+bytes>cache_budget_) {
                    auto oldest=std::min_element(cache_.begin(),cache_.end(),[](const auto& a,const auto& b){return a.second.used<b.second.used;});
                    cache_bytes_-=oldest->second.bytes;cache_.erase(oldest);
                }
                entry.bytes=bytes;entry.used=++cache_clock_;
                if (cache_.find(request_key)==cache_.end()) { cache_bytes_+=bytes; cache_.emplace(request_key,std::move(entry)); }
            }
            spdlog::info("part_runtime request={} executed={} reused={} generator_calls={} modifier_calls={} queue_ms={} exact_ms={} naming_ms={} display_ms={} cache_bytes={}",
                request->request_id(),stats->stages_executed(),stats->stages_reused(),stats->generator_calls(),stats->modifier_calls(),stats->queue_ms(),stats->exact_ms(),stats->naming_ms(),stats->display_ms(),stats->stage_cache_bytes());
#if defined(__linux__)
            struct rusage usage{};if(getrusage(RUSAGE_SELF,&usage)==0)stats->set_peak_rss_bytes(static_cast<uint64_t>(usage.ru_maxrss)*1024);
#endif
            stats->set_response_cache_bytes(cache_bytes_);stats->set_visual_cache_bytes(visual_bytes_);
            stats->set_cancellation_polls(cancellation->polls());stats->set_cancellation_poll_ms(cancellation->milliseconds());
            log_rpc("EvaluatePart", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            if(context->IsCancelled())return {grpc::StatusCode::CANCELLED,error.what()};
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
            if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED, "exchange preparation cancelled"};
            const auto source = validate_artifact(request->source());
            occccad::kernel::OcctKernel preparation(true);
            occccad::kernel::ExchangeGraph graph;
            if (format == "STEP")
                graph = preparation.readStepGraph(source.string());
            else {
                auto id = preparation.loadBrepr(read_artifact(request->source()));
                graph.definitions.push_back({"brep", "", "PART", id, {}});
                graph.roots.push_back({"root", "brep", "", {}});
            }
            auto* output = response->mutable_graph();
            output->set_kernel_version(OCC_VERSION_COMPLETE);
            output->set_source_schema(graph.source_schema);
            auto warning = [&](const std::string& code, const std::string& message, const std::string& definition, uint64_t count = 1) {
                auto* item = output->add_warnings();
                item->set_code(code); item->set_message(message); item->set_definition_id(definition); item->set_count(count);
            };
            for (const auto& item : graph.warnings) warning(item.code, item.message, item.definition_id, item.count);
            auto occurrence = [](const auto& from, auto* to) {
                to->set_id(from.id);
                to->set_definition_id(from.definition_id);
                to->set_name(from.name);
                to->set_color(from.color);
                to->set_visible(from.visible);
                if (from.opacity) to->set_opacity(*from.opacity);
                to->mutable_translation()->set_x(from.placement.translation.x);
                to->mutable_translation()->set_y(from.placement.translation.y);
                to->mutable_translation()->set_z(from.placement.translation.z);
                to->mutable_rotation()->set_x(from.placement.rotation.x);
                to->mutable_rotation()->set_y(from.placement.rotation.y);
                to->mutable_rotation()->set_z(from.placement.rotation.z);
                to->mutable_rotation()->set_w(from.placement.rotation.w);
            };
            size_t ordinal = 0;
            for (const auto& def : graph.definitions) {
                if (context->IsCancelled())
                    return {grpc::StatusCode::CANCELLED, "exchange preparation cancelled"};
                auto* item = output->add_definitions();
                item->set_id(def.id);
                item->set_name(def.name);
                item->set_kind(def.kind);
                item->set_empty(def.empty);
                item->set_visible(def.visible);
                item->mutable_appearance()->set_color(def.appearance.color);
                if (def.appearance.opacity) item->mutable_appearance()->set_opacity(*def.appearance.opacity);
                for (const auto& color : def.appearance.faces) {
                    auto* face = item->mutable_appearance()->add_faces();
                    face->set_local_id(color.local_id);
                    face->set_color(color.color);
                    if (color.opacity) face->set_opacity(*color.opacity);
                }
                for (const auto& child : def.children)
                    occurrence(child, item->add_children());
                if (def.kind == "PART" && !request->component_output_prefix().empty()) {
                    if (def.empty || def.geometry_id.empty()) {
                        warning("EXCHANGE_EMPTY_PART", "No supported geometry; an empty Part will be created", def.id);
                        continue;
                    }
                    const auto summary = preparation.getTopologySummary(def.geometry_id);
                    if (summary.face_count + summary.edge_count + summary.vertex_count == 0) {
                        item->set_empty(true);
                        warning("EXCHANGE_EMPTY_PART", "No supported geometry; an empty Part will be created", def.id);
                        continue;
                    }
                    occccad::kernel::GeometryId repaired;
                    try {
                        repaired = preparation.repairImportedGeometry(def.geometry_id);
                    } catch (const std::invalid_argument& error) {
                        // A readable source may contain shapes that cannot enter
                        // our validated BREP/Naming model. Keep its document and
                        // other definitions; do not invent topology for this one.
                        if (std::string(error.what()).find("IMPORT_") != 0) throw;
                        item->set_empty(true);
                        item->clear_appearance();
                        warning("EXCHANGE_UNSUPPORTED_GEOMETRY", error.what(), def.id);
                        warning("EXCHANGE_EMPTY_PART", "No supported geometry; an empty Part will be created", def.id);
                        continue;
                    }
                    // A repair can change topology before persistent IDs are allocated.
                    // Do not attach source face colors by ordinal to repaired topology.
                    if (repaired != def.geometry_id && !def.appearance.faces.empty()) {
                        warning("EXCHANGE_FACE_APPEARANCE_SKIPPED_AFTER_REPAIR", "Face appearance was skipped after topology repair", def.id, def.appearance.faces.size());
                        item->mutable_appearance()->clear_faces();
                    }
                    write_artifact(request->component_output_prefix() + "/" +
                                       std::to_string(++ordinal) + ".brep",
                                   preparation.serializeBrepr(repaired),
                                   "application/vnd.opencascade.brep", item->mutable_brep());
                }
            }
            for (const auto& root : graph.roots)
                occurrence(root, output->add_roots());
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
        if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED, "exchange import cancelled"};
        try {
            const auto format = exchange_format(request->format());
            const auto source = validate_artifact(request->source());
            occccad::kernel::GeometryId geometry_id;
            if (format == "STEP") {
                occccad::kernel::OcctKernel preparation(true);
                const auto graph = preparation.readStepGraph(source.string());
                for (const auto& def : graph.definitions) {
                    if (!def.geometry_id.empty() && def.kind == "PART" &&
                        (def.id == request->definition_id() ||
                         (request->definition_id().empty() && graph.definitions.size() == 1))) {
                        geometry_id = kernel_.loadBrepr(preparation.serializeBrepr(def.geometry_id));
                        break;
                    }
                }
                if (geometry_id.empty())
                    throw std::invalid_argument("STEP_PART_DEFINITION_REQUIRED");
                geometry_id = kernel_.repairImportedGeometry(geometry_id);
            } else {
                geometry_id = kernel_.repairImportedGeometry(
                    kernel_.loadBrepr(read_artifact(request->source())));
            }
            const auto summary = kernel_.getTopologySummary(geometry_id);
            if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED, "exchange import cancelled"};
            if (summary.geometry_domain == "SOLID_SET") {
                const auto mass_started = std::chrono::steady_clock::now();
                spdlog::info("exchange_mass_properties geometry={} phase=started", geometry_id);
                kernel_.getVolume(geometry_id);
                spdlog::info("exchange_mass_properties geometry={} phase=completed duration_ms={}", geometry_id,
                    std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now()-mass_started).count());
            }
            fill_evaluation(request->geometry_key(), geometry_id, request->linear_deflection(),
                            request->angular_deflection(), response, request->brep_output_key(),
                            request->glb_output_key(),false,false,nullptr,nullptr,
                            [context](){return context->IsCancelled();},[context](){return context->IsCancelled();});
            log_rpc("ImportExchange", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED, "exchange import cancelled"};
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
            bool topology_cache_hit=false;
            const auto& topology = kernel_.getTopologyProperties(geometry_id,request->topology_type(),request->local_id(),&topology_cache_hit);
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
        if (request->request_id().empty() || !request->has_graph() ||
            request->output_key().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id, graph, and output_key are required"};
        }
        std::lock_guard<std::mutex> lock(mutex_);
        try {
            const auto format = exchange_format(request->format());
            occccad::kernel::OcctKernel preparation(true);
            if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED, "exchange export cancelled"};
            occccad::kernel::ExchangeGraph graph;
            auto occurrence = [](const auto& from) {
                occccad::kernel::ExchangeOccurrence to;
                to.id = from.id();
                to.definition_id = from.definition_id();
                to.name = from.name();
                to.color = from.color();
                to.visible = !from.has_visible() || from.visible();
                if (from.has_opacity()) to.opacity = from.opacity();
                to.placement.translation = {from.translation().x(), from.translation().y(),
                                            from.translation().z()};
                to.placement.rotation = {from.rotation().x(), from.rotation().y(),
                                         from.rotation().z(),
                                         from.has_rotation() ? from.rotation().w() : 1.0};
                const auto& q=to.placement.rotation;
                const double norm=q.x*q.x+q.y*q.y+q.z*q.z+q.w*q.w;
                if(!std::isfinite(norm) || std::abs(norm-1.0)>1e-6 || !std::isfinite(to.placement.translation.x) || !std::isfinite(to.placement.translation.y) || !std::isfinite(to.placement.translation.z))
                    throw std::invalid_argument("EXCHANGE_INVALID_PLACEMENT");
                return to;
            };
            if(request->graph().definitions_size()==0 || request->graph().definitions_size()>100000 || request->graph().roots_size()==0 || request->graph().roots_size()>100000)
                throw std::invalid_argument("EXCHANGE_GRAPH_LIMIT");
            std::set<std::string> definitionIds;
            for (const auto& def : request->graph().definitions()) {
                if(def.id().empty() || !definitionIds.insert(def.id()).second || (def.kind()!="PART" && def.kind()!="PRODUCT"))
                    throw std::invalid_argument("EXCHANGE_INVALID_DEFINITION");
                occccad::kernel::ExchangeDefinition item;
                item.id = def.id();
                item.name = def.name();
                item.kind = def.kind();
                item.visible = !def.has_visible() || def.visible();
                auto appearance = [](const auto& from) {
                    occccad::kernel::ExchangeAppearance to;
                    to.color = from.color();
                    if (from.has_opacity()) to.opacity = from.opacity();
                    for (const auto& face : from.faces()) {
                        std::optional<double> opacity;
                        if (face.has_opacity()) opacity = face.opacity();
                        to.faces.push_back({face.local_id(), face.color(), opacity});
                    }
                    return to;
                };
                item.appearance = appearance(def.appearance());
                if (item.kind == "PART") {
                    if (def.bodies_size() > 0 || def.geometries_size() > 0) {
                        std::vector<occccad::kernel::PlacedGeometry> bodies;
                        for (const auto& source : def.bodies()) {
                            occccad::kernel::PlacedGeometry body;
                            body.geometry_id = preparation.loadBrepr(read_artifact(source.brep()));
                            bodies.push_back(body);
                            item.bodies.push_back({body.geometry_id, appearance(source.appearance()), !source.has_visible() || source.visible(), source.name()});
                        }
                        for(const auto& source:def.geometries()){
                            if(!source.has_result()||source.result().feature_id().empty()||source.result().result_id().empty())throw std::invalid_argument("EXCHANGE_GEOMETRY_OWNER_REQUIRED");
                            occccad::kernel::PlacedGeometry result;result.geometry_id=preparation.loadBrepr(read_artifact(source.brep()));bodies.push_back(result);
                            item.bodies.push_back({result.geometry_id, appearance(source.appearance()), !source.has_visible() || source.visible(), source.name()});
                        }
                        if (bodies.size() == 1 && def.geometries_size()==0) {
                            item.geometry_id = bodies.front().geometry_id;
                            item.appearance = item.bodies.front().appearance;
                            item.visible = item.visible && item.bodies.front().visible;
                            item.bodies.clear();
                        } else if(bodies.size()==1)item.geometry_id=bodies.front().geometry_id;
                        else item.geometry_id = preparation.combine(bodies);
                    } else item.geometry_id = preparation.loadBrepr(read_artifact(def.brep()));
                }
                for (const auto& child : def.children())
                    item.children.push_back(occurrence(child));
                graph.definitions.push_back(std::move(item));
            }
            for (const auto& root : request->graph().roots())
                graph.roots.push_back(occurrence(root));
            graph.step_schema = request->graph().step_schema();
            std::vector<uint8_t> data;
            if (format == "STEP")
                data = preparation.writeStepGraph(graph);
            else {
                std::map<std::string, occccad::kernel::GeometryId> resolved;
                std::set<std::string> visiting;
                std::function<occccad::kernel::GeometryId(const std::string&)> resolve;
                resolve = [&](const std::string& id) -> occccad::kernel::GeometryId {
                    if (resolved.count(id))
                        return resolved.at(id);
                    if (visiting.size() > 128 || !visiting.insert(id).second)
                        throw std::invalid_argument("EXCHANGE_REFERENCE_CYCLE");
                    for (const auto& def : graph.definitions)
                        if (def.id == id) {
                            auto geometry = def.geometry_id;
                            if (def.kind == "PRODUCT") {
                                std::vector<occccad::kernel::PlacedGeometry> parts;
                                for (const auto& child : def.children) {
                                    auto p = child.placement;
                                    p.geometry_id = resolve(child.definition_id);
                                    parts.push_back(p);
                                }
                                geometry = preparation.combine(parts);
                            }
                            visiting.erase(id);
                            resolved[id] = geometry;
                            return geometry;
                        }
                    throw std::invalid_argument("EXCHANGE_MISSING_DEFINITION");
                };
                std::vector<occccad::kernel::PlacedGeometry> roots;
                for (const auto& root : graph.roots) {
                    auto p = root.placement;
                    p.geometry_id = resolve(root.definition_id);
                    roots.push_back(p);
                }
                data = preparation.serializeBrepr(preparation.combine(roots));
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
                         const std::string& glb_output_key = {}, bool exact_only = false, bool force_cold = false,
                         std::shared_ptr<const std::vector<uint8_t>>* brep_buffer=nullptr,std::shared_ptr<const std::vector<uint8_t>>* glb_buffer=nullptr,std::function<bool()> cancelled={},std::function<bool()> fresh_cancelled={}) {
        const auto check_cancel=[&](){if(cancelled&&cancelled())throw std::runtime_error("EVALUATION_CANCELLED");};
        check_cancel();
        const auto publish=[&](const auto& key,const auto& bytes,const auto& type,auto* ref){const auto began=std::chrono::steady_clock::now();check_cancel();write_artifact(key,bytes,type,ref);auto* stats=response->mutable_runtime_stats();stats->set_artifact_io_ms(stats->artifact_io_ms()+std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-began).count());};
        const auto bbox = kernel_.getBoundingBox(geometry_id);
        if(exact_only) {
            const auto topology=kernel_.getTopologySummary(geometry_id);
            const auto brep=kernel_.borrowBrepr(geometry_id);
            if(brep_buffer)*brep_buffer=brep;
            if(!brep_output_key.empty())publish(brep_output_key,*brep,"application/vnd.opencascade.brep",response->mutable_brep_artifact());
            response->set_geometry_id(geometry_id);response->set_geometry_key(geometry_key);
            fill_bbox(bbox,response->mutable_bbox());response->set_volume(topology.geometry_domain=="SOLID_SET"?kernel_.getVolume(geometry_id):0);
            auto* summary=response->mutable_topology();summary->set_face_count(topology.face_count);summary->set_edge_count(topology.edge_count);
            summary->set_vertex_count(topology.vertex_count);summary->set_solid_count(topology.solid_count);summary->set_geometry_domain(topology.geometry_domain);
            response->set_occt_version(OCC_VERSION_COMPLETE);response->set_representation_kind("EXACT_STAGE");
            return;
        }
        // Display consumes topology counts and the mesher's locators. Exact
        // area/length/centroid properties belong to explicit inspection.
        const auto topology=kernel_.getTopologySummary(geometry_id);
        const double linear_deflection =
            requested_linear_deflection > 0.0 ? requested_linear_deflection : occccad::kernel::visual_linear_deflection_mm;
        const double angular_deflection =
            requested_angular_deflection > 0.0 ? requested_angular_deflection : occccad::kernel::visual_angular_deflection_radians;
        // Geometry identity freezes local topology order. Naming is deliberately
        // kept outside this cache and bound to these IDs by the response manifest.
        std::ostringstream policy;
        policy.precision(std::numeric_limits<double>::max_digits10);
        policy << geometry_id << ":visual-surface-normals-v4:" << linear_deflection << ':' << angular_deflection;
        const auto key = policy.str();
        auto found = visual_cache_.find(key);
        const bool visual_hit = !force_cold && found != visual_cache_.end();
        std::shared_ptr<const VisualSnapshot> snapshot;
        if (visual_hit) {
            found->second->used=++cache_clock_;
            snapshot = found->second;
            spdlog::info("visual_snapshot geometry={} cache_hit=true",geometry_id);
        }
        else {
            auto computed = std::make_shared<VisualSnapshot>();
            computed->mesh = kernel_.tessellate(geometry_id, linear_deflection, angular_deflection,true,cancelled,fresh_cancelled);
            check_cancel();
            google::protobuf::Struct association;
            (*association.mutable_fields())["geometryId"].set_string_value(geometry_id);
            std::string association_json;
            if (!google::protobuf::util::MessageToJsonString(association,&association_json).ok()) throw std::runtime_error("GLB association serialization failed");
            check_cancel();
            const auto encoding = std::chrono::steady_clock::now();
            computed->glb = std::make_shared<const std::vector<uint8_t>>(occccad::kernel::make_glb(computed->mesh, association_json));
            check_cancel();
            if(fresh_cancelled&&fresh_cancelled())throw std::runtime_error("EVALUATION_CANCELLED");
            const auto& m = computed->mesh;
            response->mutable_runtime_stats()->set_mesh_ms(m.timings.meshing_ms);
            response->mutable_runtime_stats()->set_mesh_reused_faces(m.timings.reused_faces);
            response->mutable_runtime_stats()->set_mesh_computed_faces(m.timings.computed_faces);
            response->mutable_runtime_stats()->set_encoding_ms(std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-encoding).count());
            size_t edge_points = 0; for (const auto& edge : m.edges) edge_points += edge.points.size();
            spdlog::info("edge_profile geometry={} sampling_ms={} normal_evaluations={} segments={} limited={}",
                geometry_id,m.timings.edge_profile_ms,m.timings.edge_normal_evaluations,m.edge_profile.size(),m.timings.edge_sampling_limits);
            spdlog::info("surface_normals geometry={} ms={} evaluations={} fallbacks={}",geometry_id,m.timings.normals_ms,m.timings.surface_normal_evaluations,m.timings.surface_normal_fallbacks);
            spdlog::info("visual_snapshot geometry={} mesh_ms={} faces_ms={} edges_ms={} normals_ms={} encoding_ms={} vertices={} triangles={} edges={} edge_points={} glb_bytes={}",
                geometry_id, m.timings.meshing_ms, m.timings.faces_ms, m.timings.edges_ms, m.timings.normals_ms,
                std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-encoding).count(),
                m.vertices.size(),m.triangles.size(),m.edges.size(),edge_points,computed->glb->size());
            size_t edge_capacity = 0; for (const auto& edge : m.edges) edge_capacity += edge.points.capacity();
            const auto bytes = computed->glb->capacity() + (m.vertices.capacity()+m.normals.capacity()+edge_capacity)*sizeof(occccad::kernel::Vec3)
                + m.triangles.capacity()*sizeof(occccad::kernel::Triangle) + m.face_ids.capacity()*sizeof(uint32_t)
                + m.topology_vertices.capacity()*sizeof(occccad::kernel::TopologyPoint) + m.edges.capacity()*sizeof(occccad::kernel::EdgePolyline)
                + m.edge_profile.capacity()*sizeof(occccad::kernel::EdgeDisplaySegment);
            snapshot = computed;
            if (!force_cold && bytes <= cache_budget_) {
                while(!visual_cache_.empty() && visual_bytes_+bytes>cache_budget_) {
                    auto oldest=std::min_element(visual_cache_.begin(),visual_cache_.end(),[](const auto& a,const auto& b){return a.second->used<b.second->used;});
                    visual_bytes_-=oldest->second->bytes;visual_cache_.erase(oldest);
                }
                computed->bytes=bytes;computed->used=++cache_clock_;
                visual_bytes_+=bytes; visual_cache_.emplace(key,std::move(computed));
            }
        }
        const auto& mesh = snapshot->mesh;
        const auto& glb = snapshot->glb;
        const auto brep = kernel_.borrowBrepr(geometry_id);
        if(brep_buffer)*brep_buffer=brep;
        if(glb_buffer)*glb_buffer=glb;

        response->set_geometry_id(geometry_id);
        response->set_geometry_key(geometry_key);
        if (!brep_output_key.empty())
            publish(brep_output_key, *brep, "application/vnd.opencascade.brep",
                           response->mutable_brep_artifact());

        if (!glb_output_key.empty())
            publish(glb_output_key, *glb, "model/gltf-binary",
                           response->mutable_glb_artifact());

        fill_bbox(bbox, response->mutable_bbox());
        response->set_volume(topology.geometry_domain=="SOLID_SET"?kernel_.getVolume(geometry_id):0);
        response->set_cache_hit(false);
        response->set_occt_version(OCC_VERSION_COMPLETE);
        auto* summary = response->mutable_topology();
        summary->set_face_count(topology.face_count);
        summary->set_edge_count(topology.edge_count);
        summary->set_vertex_count(topology.vertex_count);
        summary->set_solid_count(topology.solid_count);summary->set_geometry_domain(topology.geometry_domain);
        response->set_triangle_count(mesh.triangles.size());
        response->set_display_vertex_count(mesh.vertices.size());
        response->set_representation_kind(brep_output_key.empty() ? "TRANSIENT_PREVIEW" : "PERSISTENT");
        if (!brep_output_key.empty()) return;
        auto* output_mesh = response->mutable_preview_mesh();
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

    std::atomic<uint32_t> resident_snapshot_{0};
    struct NamingSnapshot {std::string geometry_id,bytes;size_t charge=0;uint64_t used=0;};
    size_t naming_bytes_=0;
    std::unordered_map<std::string,NamingSnapshot> naming_cache_;
    struct InputArtifact {std::vector<uint8_t> bytes;uint64_t used=0;size_t charge=0;};
    size_t input_bytes_=0;
    std::unordered_map<std::string,InputArtifact> inputs_;
    std::vector<uint8_t> read_input(const worker_api::ArtifactReference& ref,bool cold) {
        const auto key=ref.sha256()+"/"+std::to_string(ref.size_bytes());
        if(!cold){const auto hit=inputs_.find(key);if(hit!=inputs_.end()){hit->second.used=++cache_clock_;return hit->second.bytes;}}
        auto data=read_artifact(ref);const size_t bytes=data.capacity()+key.capacity()+sizeof(InputArtifact);
        if(!cold && bytes<=cache_budget_/2) {
            while(!inputs_.empty() && input_bytes_+bytes>cache_budget_/2) {
                auto oldest=std::min_element(inputs_.begin(),inputs_.end(),[](const auto& a,const auto& b){return a.second.used<b.second.used;});
                input_bytes_-=oldest->second.charge;inputs_.erase(oldest);
            }
            input_bytes_+=bytes;inputs_.emplace(key,InputArtifact{data,++cache_clock_,bytes});
        }
        return data;
    }
    std::mutex mutex_;
    std::unique_ptr<occccad::kernel::OcctKernel> interference_kernel_;
    std::unordered_map<std::string,std::string> interference_loaded_;
    uint64_t interference_bytes_=0;
    occccad::kernel::OcctKernel kernel_;
    std::unique_ptr<occccad::geometry::sketch::SketchSolver> sketch_solver_ =
        occccad::geometry::sketch::make_plane_gcs_sketch_solver();
    struct EvaluationCache {
        worker_api::EvaluatePartResponse response;
        std::shared_ptr<const std::vector<uint8_t>> brep, glb;
        std::vector<uint8_t> naming;
        size_t bytes=0;uint64_t used=0;
    };
    struct VisualSnapshot {
        occccad::kernel::TessellationResult mesh;
        std::shared_ptr<const std::vector<uint8_t>> glb;
        size_t bytes=0;mutable uint64_t used=0;
    };
    // Per-worker budgets, in addition to the resident exact model working set.
    static constexpr size_t cache_budget_ = 64 * 1024 * 1024;
    size_t cache_bytes_ = 0, visual_bytes_ = 0;
    uint64_t cache_clock_=0;
    std::unordered_map<std::string, EvaluationCache> cache_;
    std::unordered_map<std::string, std::shared_ptr<const VisualSnapshot>> visual_cache_;
};

}  // namespace

int main() {
    try {
        const char* configured_address = std::getenv("OCCCCAD_GEOMETRY_WORKER_LISTEN");
        const std::string address =
            configured_address != nullptr ? configured_address : "127.0.0.1:51001";
        const auto log_file = configure_logging(address);

        // One coarse geometry RPC at a time in this worker, bounded internal
        // OCCT parallelism. Do not nest per-face application thread pools.
        OSD_Parallel::SetUseOcctThreads(Standard_True);
        OSD_ThreadPool::DefaultPool(4)->SetNbDefaultThreadsToLaunch(4);
        GeometryWorkerService service;
        grpc::ServerBuilder builder;
        builder.AddChannelArgument("grpc.so_reuseport", 0);
        builder.SetMaxReceiveMessageSize(kGeometryRpcMaxMessageBytes);
        builder.SetMaxSendMessageSize(kGeometryRpcMaxMessageBytes);
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
