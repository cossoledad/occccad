#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>
#include <occccad/worker/v1/geometry_worker.grpc.pb.h>

#include <internal/occt_kernel.hpp>

#include <Standard_Version.hxx>
#include <grpcpp/grpcpp.h>

#include <cstdlib>
#include <chrono>
#include <exception>
#include <iostream>
#include <memory>
#include <mutex>
#include <string>
#include <unordered_map>
#include <vector>

namespace worker_api = occccad::worker::v1;

namespace {

void fill_bbox(
    const occccad::kernel::BoundingBox& source,
    worker_api::BoundingBox* destination) {
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
            output->set_number_value(property.number_value); break;
        case occccad::kernel::TopologyProperty::Kind::INTEGER:
            output->set_integer_value(property.integer_value); break;
        case occccad::kernel::TopologyProperty::Kind::BOOLEAN:
            output->set_bool_value(property.bool_value); break;
        case occccad::kernel::TopologyProperty::Kind::TEXT:
            output->set_text_value(property.text_value); break;
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
    if (iterator == context->client_metadata().end()) return {};
    return {iterator->second.data(), iterator->second.length()};
}

void log_rpc(
    const char* operation,
    const std::string& request_id,
    const std::string& traceparent,
    const char* status,
    const std::chrono::steady_clock::time_point started) {
    const auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::steady_clock::now() - started).count();
    std::cout << "{\"level\":\"INFO\",\"service\":\"occccad-geometry-worker\""
              << ",\"operation\":\"" << operation << "\""
              << ",\"request_id\":\"" << request_id << "\""
              << ",\"traceparent\":\"" << traceparent << "\""
              << ",\"status\":\"" << status << "\""
              << ",\"duration_ms\":" << duration << "}" << std::endl;
}

class GeometryWorkerService final : public worker_api::GeometryWorker::Service {
public:
    grpc::Status Ping(
        grpc::ServerContext* /*context*/,
        const worker_api::PingRequest* /*request*/,
        worker_api::PingResponse* response) override {
        std::lock_guard<std::mutex> lock(mutex_);
        response->set_worker_id("geometry-worker-local-1");
        response->set_occt_version(OCC_VERSION_COMPLETE);
        response->set_resident_geometry_count(
            static_cast<uint32_t>(kernel_.resident_count()));
        return grpc::Status::OK;
    }

    grpc::Status EvaluatePart(
        grpc::ServerContext* context,
        const worker_api::EvaluatePartRequest* request,
        worker_api::EvaluatePartResponse* response) override {
        const auto started = std::chrono::steady_clock::now();
        const auto traceparent = metadata_value(context, "traceparent");
        if (context->IsCancelled()) {
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        }
        if (request->request_id().empty() || request->geometry_key().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id and geometry_key are required"};
        }
        if (!request->has_rectangular_pad() && request->rectangular_pads().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "feature chain requires at least one rectangular pad"};
        }

        std::lock_guard<std::mutex> lock(mutex_);
        const auto cached = cache_.find(request->geometry_key());
        if (cached != cache_.end()) {
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
                    input.origin_x(), input.origin_y(), input.width(), input.height(),
                    input.pad_length(), input.plane().empty() ? "XY" : input.plane(),
                });
            };
            for (const auto& input : request->rectangular_pads()) append_spec(input);
            if (specs.empty() && request->has_rectangular_pad()) {
                append_spec(request->rectangular_pad());
            }
            const std::vector<uint8_t> base_brep(
                request->base_brep_data().begin(), request->base_brep_data().end());
            const auto geometry_id = kernel_.evaluateRectangularPads(specs, base_brep);
            fill_evaluation(
                request->geometry_key(), geometry_id, request->linear_deflection(),
                request->angular_deflection(), response);

            cache_.insert_or_assign(request->geometry_key(), *response);
            log_rpc("EvaluatePart", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

    grpc::Status ImportStep(
        grpc::ServerContext* context,
        const worker_api::ImportStepRequest* request,
        worker_api::EvaluatePartResponse* response) override {
        const auto started = std::chrono::steady_clock::now();
        const auto traceparent = metadata_value(context, "traceparent");
        if (context->IsCancelled()) {
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        }
        if (request->request_id().empty() || request->geometry_key().empty() ||
            request->step_data().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id, geometry_key, and step_data are required"};
        }
        std::lock_guard<std::mutex> lock(mutex_);
        const auto cached = cache_.find(request->geometry_key());
        if (cached != cache_.end()) {
            response->CopyFrom(cached->second);
            response->set_cache_hit(true);
            log_rpc("ImportStep", request->request_id(), traceparent, "CACHE_HIT", started);
            return grpc::Status::OK;
        }
        try {
            const std::vector<uint8_t> data(
                request->step_data().begin(), request->step_data().end());
            const auto geometry_id = kernel_.loadStepData(data);
            fill_evaluation(
                request->geometry_key(), geometry_id, request->linear_deflection(),
                request->angular_deflection(), response);
            cache_.insert_or_assign(request->geometry_key(), *response);
            log_rpc("ImportStep", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

    grpc::Status GetTopology(
        grpc::ServerContext* context,
        const worker_api::GetTopologyRequest* request,
        worker_api::GetTopologyResponse* response) override {
        if (context->IsCancelled()) return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        if (request->geometry_id().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT, "geometry_id is required"};
        }
        std::lock_guard<std::mutex> lock(mutex_);
        try {
            std::string geometry_id = request->geometry_id();
            if (!kernel_.is_loaded(geometry_id)) {
                if (request->brep_data().empty()) {
                    return {grpc::StatusCode::NOT_FOUND, "geometry is not resident and brep_data was not supplied"};
                }
                const std::vector<uint8_t> brep(request->brep_data().begin(), request->brep_data().end());
                geometry_id = kernel_.loadBrepr(brep);
            }
            const auto topology = kernel_.getTopology(geometry_id);
            response->set_face_count(topology.face_count);
            response->set_edge_count(topology.edge_count);
            response->set_vertex_count(topology.vertex_count);
            response->set_solid_count(topology.solid_count);
            for (const auto& face : topology.faces) {
                if (!request->topology_type().empty() && request->topology_type() != "FACE") continue;
                if (request->local_id() != 0 && request->local_id() != face.local_id) continue;
                auto* output = response->add_faces();
                output->set_local_id(face.local_id);
                output->set_surface_type(face.surface_type);
                fill_bbox(face.bbox, output->mutable_bbox());
                fill_properties(face.properties, output->mutable_properties());
            }
            for (const auto& edge : topology.edges) {
                if (!request->topology_type().empty() && request->topology_type() != "EDGE") continue;
                if (request->local_id() != 0 && request->local_id() != edge.local_id) continue;
                auto* output = response->add_edges();
                output->set_local_id(edge.local_id);
                output->set_curve_type(edge.curve_type);
                fill_bbox(edge.bbox, output->mutable_bbox());
                fill_properties(edge.properties, output->mutable_properties());
                for (const auto& point : edge.render_points) {
                    auto* output_point = output->add_render_points();
                    output_point->set_x(point.x); output_point->set_y(point.y); output_point->set_z(point.z);
                }
            }
            for (const auto& vertex : topology.vertices) {
                if (!request->topology_type().empty() && request->topology_type() != "VERTEX") continue;
                if (request->local_id() != 0 && request->local_id() != vertex.local_id) continue;
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

    grpc::Status ExportStep(
        grpc::ServerContext* context,
        const worker_api::ExportStepRequest* request,
        worker_api::ExportStepResponse* response) override {
        const auto started = std::chrono::steady_clock::now();
        const auto traceparent = metadata_value(context, "traceparent");
        if (context->IsCancelled()) {
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        }
        if (request->request_id().empty() || request->brep_data().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id and brep_data are required"};
        }
        std::lock_guard<std::mutex> lock(mutex_);
        try {
            const std::vector<uint8_t> brep(
                request->brep_data().begin(), request->brep_data().end());
            const auto geometry_id = kernel_.loadBrepr(brep);
            const auto step = kernel_.serializeStep(geometry_id);
            response->set_step_data(step.data(), step.size());
            log_rpc("ExportStep", request->request_id(), traceparent, "OK", started);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

private:
    void fill_evaluation(
        const std::string& geometry_key,
        const occccad::kernel::GeometryId& geometry_id,
        const double requested_linear_deflection,
        const double requested_angular_deflection,
        worker_api::EvaluatePartResponse* response) {
        const auto bbox = kernel_.getBoundingBox(geometry_id);
        const auto topology = kernel_.getTopology(geometry_id);
        const double linear_deflection =
            requested_linear_deflection > 0.0 ? requested_linear_deflection : 0.1;
        const double angular_deflection =
            requested_angular_deflection > 0.0 ? requested_angular_deflection : 0.5;
        const auto mesh = kernel_.tessellate(
            geometry_id, linear_deflection, angular_deflection);
        const auto brep = kernel_.serializeBrepr(geometry_id);
        const auto glb = occccad::kernel::make_glb(mesh);

        response->set_geometry_id(geometry_id);
        response->set_geometry_key(geometry_key);
        response->set_brep_data(brep.data(), brep.size());
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
    std::unordered_map<std::string, worker_api::EvaluatePartResponse> cache_;
};

int run_smoke() {
    occccad::kernel::OcctKernel kernel;
    const occccad::kernel::RectangularPadSpec spec{
        0.0, 0.0, 100.0, 60.0, 40.0, "XY"};
    const auto id = kernel.createRectangularPad(spec);
    const auto topology = kernel.getTopology(id);
    const auto mesh = kernel.tessellate(id);
    if (topology.faces.size() != 6 || topology.edges.size() != 12 || topology.vertices.size() != 8 ||
        topology.faces.front().properties.empty() || topology.edges.front().properties.empty() ||
        mesh.edges.size() != 12 || mesh.topology_vertices.size() != 8) {
        std::cerr << "[FAIL] B-Rep topology detail or selection mesh is incomplete\n";
        return EXIT_FAILURE;
    }
    std::cout << "occccad Geometry Worker " << OCC_VERSION_COMPLETE << '\n'
              << "[SMOKE] GeometryId: " << id << '\n'
              << "[SMOKE] Volume: " << kernel.getVolume(id) << " mm^3\n"
              << "[SMOKE] Topology: " << topology.face_count << " faces / "
              << topology.edge_count << " edges / " << topology.vertex_count
              << " vertices / " << topology.solid_count << " solid\n"
              << "[SMOKE] Triangles: " << mesh.triangles.size() << '\n'
              << "[SMOKE] Selectable topology: " << mesh.edges.size() << " edges / "
              << mesh.topology_vertices.size() << " vertices\n"
              << "[PASS] Rectangle Sketch -> Pad\n";
    return EXIT_SUCCESS;
}

}  // namespace

int main(const int argc, char* argv[]) {
    if (argc > 1 && std::string(argv[1]) == "--smoke") {
        return run_smoke();
    }

    const char* configured_address = std::getenv("OCCCCAD_GEOMETRY_WORKER_LISTEN");
    const std::string address = configured_address != nullptr
        ? configured_address
        : "127.0.0.1:51001";

    GeometryWorkerService service;
    grpc::ServerBuilder builder;
    builder.AddListeningPort(address, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);
    std::unique_ptr<grpc::Server> server = builder.BuildAndStart();
    if (!server) {
        std::cerr << "Failed to start Geometry Worker on " << address << '\n';
        return EXIT_FAILURE;
    }
    std::cout << "occccad Geometry Worker listening on " << address
              << " (OCCT " << OCC_VERSION_COMPLETE << ")\n";
    server->Wait();
    return EXIT_SUCCESS;
}
