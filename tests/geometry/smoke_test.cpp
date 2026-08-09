/// OCCT MakeBox Smoke Test
///
/// Verifies:
///   - OCCT headers are found
///   - OCCT libraries are linked
///   - Basic topology traversal works

#include <occccad/kernel/kernel.hpp>

#include <internal/occt_kernel.hpp>

#include <cstdlib>
#include <iostream>

int main() {
    occccad::kernel::OcctKernel kernel;

    // 1. Create a box
    auto box_id = kernel.createBox(10.0, 20.0, 30.0);
    if (box_id.empty()) {
        std::cerr << "[FAIL] createBox returned empty ID\n";
        return 1;
    }

    // 2. Verify it's loaded
    if (!kernel.is_loaded(box_id)) {
        std::cerr << "[FAIL] Box not found after creation\n";
        return 1;
    }

    // 3. Check bounding box
    auto bbox = kernel.getBoundingBox(box_id);
    if (bbox.min.x > 0.001 || bbox.min.y > 0.001 || bbox.min.z > 0.001) {
        std::cerr << "[FAIL] Bounding box min is not near origin\n";
        return 1;
    }
    if (bbox.max.x < 9.999 || bbox.max.y < 19.999 || bbox.max.z < 29.999) {
        std::cerr << "[FAIL] Bounding box max is not expected size\n";
        return 1;
    }
    std::cout << "[PASS] Bounding box correct\n";

    // 4. Check topology: a box should have 6 faces, 12 edges, 8 vertices
    auto topo = kernel.getTopology(box_id);

    if (topo.face_count != 6) {
        std::cerr << "[FAIL] Expected 6 faces, got " << topo.face_count << "\n";
        return 1;
    }
    std::cout << "[PASS] Face count = 6\n";

    if (topo.edge_count != 12) {
        std::cerr << "[FAIL] Expected 12 edges, got " << topo.edge_count << "\n";
        return 1;
    }
    std::cout << "[PASS] Edge count = 12\n";

    if (topo.vertex_count != 8) {
        std::cerr << "[FAIL] Expected 8 vertices, got " << topo.vertex_count << "\n";
        return 1;
    }
    std::cout << "[PASS] Vertex count = 8\n";

    if (topo.solid_count != 1) {
        std::cerr << "[FAIL] Expected 1 solid, got " << topo.solid_count << "\n";
        return 1;
    }
    std::cout << "[PASS] Solid count = 1\n";

    // 5. Unload
    kernel.unload(box_id);
    if (kernel.is_loaded(box_id)) {
        std::cerr << "[FAIL] Box still loaded after unload\n";
        return 1;
    }
    std::cout << "[PASS] Unload works\n";

    // 6. Resident count
    if (kernel.resident_count() != 0) {
        std::cerr << "[FAIL] Resident count not zero after unload\n";
        return 1;
    }
    std::cout << "[PASS] Resident count = 0 after cleanup\n";

    std::cout << "\n[ALL PASS] OCCT MakeBox smoke test\n";
    return 0;
}
