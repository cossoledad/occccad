#include <Standard_Version.hxx>

#include <internal/occt_kernel.hpp>
#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>

#include <grpcpp/grpcpp.h>
#include <occccad/geometry/sketch/sketch_solver.h>
#include <occccad/worker/v1/geometry_worker.grpc.pb.h>

#include <chrono>
#include <cctype>
#include <cstdint>
#include <cmath>
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
#include <vector>

namespace worker_api = occccad::worker::v1;
namespace sketch_api = occccad::geometry::sketch;

namespace {

constexpr std::uintmax_t kMaximumExchangeBytes = 512ULL * 1024ULL * 1024ULL;

std::filesystem::path artifact_root() {
    const char* configured = std::getenv("OCCCCAD_DATA_DIR");
    return std::filesystem::absolute(configured != nullptr ? configured : "./data").lexically_normal();
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
    std::cout << "{\"level\":\"INFO\",\"service\":\"occccad-geometry-worker\""
              << ",\"operation\":\"" << operation << "\""
              << ",\"request_id\":\"" << request_id << "\""
              << ",\"traceparent\":\"" << traceparent << "\""
              << ",\"status\":\"" << status << "\""
              << ",\"duration_ms\":" << duration << "}" << std::endl;
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
    else if (input.sub_element() == "DIRECTION")
        sub_element = sketch_api::SubElement::direction;
    else
        throw std::invalid_argument("unknown sketch sub-element");
    return {target, input.entity_id(), sub_element};
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
    for (const auto& constraint : input.constraints()) {
        sketch_api::ConstraintKind kind;
        if (constraint.kind() == "COINCIDENT")
            kind = sketch_api::ConstraintKind::coincident;
        else if (constraint.kind() == "PARALLEL")
            kind = sketch_api::ConstraintKind::parallel;
        else if (constraint.kind() == "FIXED_POINT")
            kind = sketch_api::ConstraintKind::fixed_point;
        else
            throw std::invalid_argument("unknown sketch constraint kind");
        sketch_api::SketchConstraint output{
            constraint.id(),
            kind,
            {},
            {constraint.fixed_point().x(), constraint.fixed_point().y()}};
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
        if (!request->has_rectangular_pad() && request->rectangular_pads().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "feature chain requires at least one rectangular pad"};
        }

        std::lock_guard<std::mutex> lock(mutex_);
        const bool external_outputs = !request->brep_output_key().empty() ||
                                      !request->glb_output_key().empty();
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
            const auto geometry_id = kernel_.evaluateRectangularPads(specs, base_brep);
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
            const uint32_t count = format == "STEP" ? kernel_.inspectStepRootCount(source.string()) : 1U;
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
            const auto topology = kernel_.getTopology(geometry_id);
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
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
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
                components.push_back({id, {component.translation().x(), component.translation().y(),
                                           component.translation().z()}});
            }
            const auto geometry_id = components.size() == 1U &&
                                             components.front().translation.x == 0.0 &&
                                             components.front().translation.y == 0.0 &&
                                             components.front().translation.z == 0.0
                                         ? components.front().geometry_id
                                         : kernel_.combine(components);
            const auto data = format == "STEP" ? kernel_.serializeStep(geometry_id)
                                                : kernel_.serializeBrepr(geometry_id);
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
    void fill_evaluation(const std::string& geometry_key,
                         const occccad::kernel::GeometryId& geometry_id,
                         const double requested_linear_deflection,
                         const double requested_angular_deflection,
                         worker_api::EvaluatePartResponse* response,
                         const std::string& brep_output_key = {},
                         const std::string& glb_output_key = {}) {
        const auto bbox = kernel_.getBoundingBox(geometry_id);
        const auto topology = kernel_.getTopology(geometry_id);
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
};

int run_smoke() {
    occccad::kernel::OcctKernel kernel;
    const occccad::kernel::RectangularPadSpec spec{0.0, 0.0, 100.0, 60.0, 40.0, "XY"};
    const auto id = kernel.createRectangularPad(spec);
    const auto topology = kernel.getTopology(id);
    const auto mesh = kernel.tessellate(id);
    if (topology.faces.size() != 6 || topology.edges.size() != 12 ||
        topology.vertices.size() != 8 || topology.faces.front().properties.empty() ||
        topology.edges.front().properties.empty() || mesh.edges.size() != 12 ||
        mesh.topology_vertices.size() != 8) {
        std::cerr << "[FAIL] B-Rep topology detail or selection mesh is incomplete\n";
        return EXIT_FAILURE;
    }
	const auto step = kernel.serializeStep(id);
	const char* requested_exchange_path = std::getenv("OCCCCAD_SMOKE_STEP_OUTPUT");
	const auto exchange_path = requested_exchange_path != nullptr
	                               ? std::filesystem::path(requested_exchange_path)
	                               : std::filesystem::temp_directory_path() /
	                                     ("occccad-exchange-smoke-" +
	                                      std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()) +
	                                      ".step");
	{
		std::ofstream output(exchange_path, std::ios::binary);
		output.write(reinterpret_cast<const char*>(step.data()), static_cast<std::streamsize>(step.size()));
	}
	const auto roots = kernel.inspectStepRootCount(exchange_path.string());
	const auto imported = kernel.loadStepRoot(exchange_path.string(), 1U);
	if (requested_exchange_path == nullptr)
		std::filesystem::remove(exchange_path);
	const auto brep = kernel.serializeBrepr(imported);
	const auto brep_round_trip = kernel.loadBrepr(brep);
	const auto assembly = kernel.combine({{brep_round_trip, {0.0, 0.0, 0.0}},
	                                      {brep_round_trip, {150.0, 0.0, 0.0}}});
	if (roots != 1U || brep.empty() || std::abs(kernel.getVolume(imported) - kernel.getVolume(id)) > 1e-6 ||
	    std::abs(kernel.getVolume(assembly) - 2.0 * kernel.getVolume(id)) > 1e-6) {
		std::cerr << "[FAIL] STEP/BREP exchange round-trip or Product combine is invalid\n";
		return EXIT_FAILURE;
	}
    std::cout << "occccad Geometry Worker " << OCC_VERSION_COMPLETE << '\n'
              << "[SMOKE] GeometryId: " << id << '\n'
              << "[SMOKE] Volume: " << kernel.getVolume(id) << " mm^3\n"
              << "[SMOKE] Topology: " << topology.face_count << " faces / " << topology.edge_count
              << " edges / " << topology.vertex_count << " vertices / " << topology.solid_count
              << " solid\n"
              << "[SMOKE] Triangles: " << mesh.triangles.size() << '\n'
              << "[SMOKE] Selectable topology: " << mesh.edges.size() << " edges / "
              << mesh.topology_vertices.size() << " vertices\n"
			  << "[SMOKE] STEP/BREP round-trip: " << roots << " root / Product combine verified\n"
              << "[PASS] Rectangle Sketch -> Pad\n";
    return EXIT_SUCCESS;
}

}  // namespace

int main(const int argc, char* argv[]) {
    if (argc > 1 && std::string(argv[1]) == "--smoke") {
        return run_smoke();
    }

    const char* configured_address = std::getenv("OCCCCAD_GEOMETRY_WORKER_LISTEN");
    const std::string address =
        configured_address != nullptr ? configured_address : "127.0.0.1:51001";

    GeometryWorkerService service;
    grpc::ServerBuilder builder;
    builder.AddListeningPort(address, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);
    std::unique_ptr<grpc::Server> server = builder.BuildAndStart();
    if (!server) {
        std::cerr << "Failed to start Geometry Worker on " << address << '\n';
        return EXIT_FAILURE;
    }
    std::cout << "occccad Geometry Worker listening on " << address << " (OCCT "
              << OCC_VERSION_COMPLETE << ")\n";
    server->Wait();
    return EXIT_SUCCESS;
}
