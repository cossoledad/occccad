#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>
#include <occccad/worker/v1/geometry_worker.grpc.pb.h>

#include <internal/occt_kernel.hpp>

#include <Standard_Version.hxx>
#include <grpcpp/grpcpp.h>

#include <cstdlib>
#include <exception>
#include <iostream>
#include <memory>
#include <mutex>
#include <string>
#include <unordered_map>

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
        if (context->IsCancelled()) {
            return {grpc::StatusCode::CANCELLED, "request was cancelled"};
        }
        if (request->request_id().empty() || request->geometry_key().empty()) {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "request_id and geometry_key are required"};
        }
        if (!request->has_rectangular_pad() ||
            request->rectangular_pad().units() != "mm") {
            return {grpc::StatusCode::INVALID_ARGUMENT,
                    "Demo 01 requires a rectangular pad expressed in mm"};
        }

        std::lock_guard<std::mutex> lock(mutex_);
        const auto cached = cache_.find(request->geometry_key());
        if (cached != cache_.end()) {
            response->CopyFrom(cached->second);
            response->set_cache_hit(true);
            return grpc::Status::OK;
        }

        try {
            const auto& input = request->rectangular_pad();
            const occccad::kernel::RectangularPadSpec spec{
                input.origin_x(),
                input.origin_y(),
                input.width(),
                input.height(),
                input.pad_length(),
                input.plane().empty() ? "XY" : input.plane(),
            };
            const auto geometry_id = kernel_.createRectangularPad(spec);
            const auto bbox = kernel_.getBoundingBox(geometry_id);
            const auto topology = kernel_.getTopology(geometry_id);
            const double linear_deflection =
                request->linear_deflection() > 0.0 ? request->linear_deflection() : 0.1;
            const double angular_deflection =
                request->angular_deflection() > 0.0 ? request->angular_deflection() : 0.5;
            const auto mesh = kernel_.tessellate(
                geometry_id, linear_deflection, angular_deflection);
            const auto brep = kernel_.serializeBrepr(geometry_id);
            const auto glb = occccad::kernel::make_glb(mesh);

            response->set_geometry_id(geometry_id);
            response->set_geometry_key(request->geometry_key());
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

            cache_.insert_or_assign(request->geometry_key(), *response);
            return grpc::Status::OK;
        } catch (const std::invalid_argument& error) {
            return {grpc::StatusCode::INVALID_ARGUMENT, error.what()};
        } catch (const std::exception& error) {
            return {grpc::StatusCode::INTERNAL, error.what()};
        }
    }

private:
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
    std::cout << "occccad Geometry Worker " << OCC_VERSION_COMPLETE << '\n'
              << "[SMOKE] GeometryId: " << id << '\n'
              << "[SMOKE] Volume: " << kernel.getVolume(id) << " mm^3\n"
              << "[SMOKE] Topology: " << topology.face_count << " faces / "
              << topology.edge_count << " edges / " << topology.vertex_count
              << " vertices / " << topology.solid_count << " solid\n"
              << "[SMOKE] Triangles: " << mesh.triangles.size() << '\n'
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
