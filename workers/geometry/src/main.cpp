// occccad Geometry Worker — entry point
//
// Phase 0 smoke test: creates a box, prints bounding box and topology info.
// Later phases will start a gRPC server.

#include <occccad/kernel/kernel.hpp>

#include <internal/occt_kernel.hpp>

#include <iostream>

int main() {
    std::cout << "occccad Geometry Worker v0.1.0\n";
    std::cout << "================================\n";

    occccad::kernel::OcctKernel kernel;

    // Smoke test: create a box
    auto box_id = kernel.createBox(10.0, 20.0, 30.0);
    std::cout << "\n[SMOKE] Created box: " << box_id << "\n";

    // Bounding box
    auto bbox = kernel.getBoundingBox(box_id);
    std::cout << "[SMOKE] Bounding box:\n"
              << "  min = (" << bbox.min.x << ", " << bbox.min.y << ", " << bbox.min.z << ")\n"
              << "  max = (" << bbox.max.x << ", " << bbox.max.y << ", " << bbox.max.z << ")\n";

    // Topology
    auto topo = kernel.getTopology(box_id);
    std::cout << "[SMOKE] Topology:\n"
              << "  faces:  " << topo.face_count << "\n"
              << "  edges:  " << topo.edge_count << "\n"
              << "  vertices: " << topo.vertex_count << "\n"
              << "  solids: " << topo.solid_count << "\n";

    kernel.unload(box_id);

    std::cout << "\n[PASS] OCCT MakeBox smoke test passed.\n";
    return 0;
}
