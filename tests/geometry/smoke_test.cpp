#include <occccad/kernel/geometry_id.hpp>
#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>

#include <internal/occt_kernel.hpp>

#include <cmath>
#include <cstdlib>
#include <iostream>
#include <stdexcept>
#include <string>

namespace {

bool near(const double actual, const double expected, const double tolerance = 1.0e-5) {
    return std::abs(actual - expected) <= tolerance;
}

int fail(const std::string& message) {
    std::cerr << "[FAIL] " << message << '\n';
    return EXIT_FAILURE;
}

}  // namespace

int main() {
    using occccad::kernel::OcctKernel;
    using occccad::kernel::RectangularPadSpec;

    if (occccad::kernel::make_geometry_id("abc") !=
        "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad") {
        return fail("SHA-256 implementation returned an unexpected digest");
    }

    OcctKernel kernel;
    const RectangularPadSpec spec{
        0.0,
        0.0,
        100.0,
        60.0,
        40.0,
        "XY",
    };

    const auto geometry_id = kernel.createRectangularPad(spec);
    if (geometry_id.rfind("sha256:", 0) != 0 || geometry_id.size() != 71) {
        return fail("rectangular pad did not receive a SHA-256 GeometryId");
    }

    const auto bbox = kernel.getBoundingBox(geometry_id);
    if (!near(bbox.min.x, 0.0) || !near(bbox.min.y, 0.0) ||
        !near(bbox.min.z, 0.0) || !near(bbox.max.x, 100.0) ||
        !near(bbox.max.y, 60.0) || !near(bbox.max.z, 40.0)) {
        return fail("rectangular pad bounding box is incorrect");
    }

    if (!near(kernel.getVolume(geometry_id), 240000.0, 1.0e-3)) {
        return fail("rectangular pad volume is incorrect");
    }

    const auto topology = kernel.getTopology(geometry_id);
    if (topology.face_count != 6 || topology.edge_count != 12 ||
        topology.vertex_count != 8 || topology.solid_count != 1) {
        return fail("rectangular pad topology counts are incorrect");
    }

    const auto mesh = kernel.tessellate(geometry_id, 0.1, 0.5);
    if (mesh.vertices.empty() || mesh.triangles.size() != 12 ||
        mesh.face_ids.size() != mesh.triangles.size()) {
        return fail("rectangular pad tessellation is incomplete");
    }
    const auto glb = occccad::kernel::make_glb(mesh);
    if (glb.size() < 20 || glb[0] != 'g' || glb[1] != 'l' ||
        glb[2] != 'T' || glb[3] != 'F') {
        return fail("GLB generation returned an invalid header");
    }

    const auto brep = kernel.serializeBrepr(geometry_id);
    if (brep.empty()) {
        return fail("B-Rep serialization returned no data");
    }
    kernel.unload(geometry_id);
    if (kernel.resident_count() != 0) {
        return fail("geometry remained resident after unload");
    }

    const auto reloaded_id = kernel.loadBrepr(brep);
    if (reloaded_id != geometry_id || !near(kernel.getVolume(reloaded_id), 240000.0, 1.0e-3)) {
        return fail("B-Rep round trip changed identity or volume");
    }

    const auto xz_id = kernel.createRectangularPad(
        {-20.0, -10.0, 80.0, 50.0, 35.0, "XZ"});
    const auto xz_bbox = kernel.getBoundingBox(xz_id);
    if (!near(kernel.getVolume(xz_id), 140000.0, 1.0e-3) ||
        !near(xz_bbox.min.x, -20.0) || !near(xz_bbox.min.y, -35.0) ||
        !near(xz_bbox.min.z, -10.0) || !near(xz_bbox.max.x, 60.0) ||
        !near(xz_bbox.max.y, 0.0) || !near(xz_bbox.max.z, 40.0)) {
        return fail("XZ rectangular pad orientation is incorrect");
    }

    const auto yz_id = kernel.createRectangularPad(
        {5.0, 10.0, 30.0, 20.0, 15.0, "YZ"});
    const auto yz_bbox = kernel.getBoundingBox(yz_id);
    if (!near(kernel.getVolume(yz_id), 9000.0, 1.0e-3) ||
        !near(yz_bbox.min.x, 0.0) || !near(yz_bbox.min.y, 5.0) ||
        !near(yz_bbox.min.z, 10.0) || !near(yz_bbox.max.x, 15.0) ||
        !near(yz_bbox.max.y, 35.0) || !near(yz_bbox.max.z, 30.0)) {
        return fail("YZ rectangular pad orientation is incorrect");
    }

    bool rejected_invalid_spec = false;
    try {
        kernel.createRectangularPad({0.0, 0.0, -1.0, 60.0, 40.0});
    } catch (const std::invalid_argument&) {
        rejected_invalid_spec = true;
    }
    if (!rejected_invalid_spec) {
        return fail("invalid rectangular pad dimensions were accepted");
    }
    try {
        kernel.createRectangularPad({0.0, 0.0, 10.0, 10.0, 10.0, "AB"});
        return fail("invalid datum plane was accepted");
    } catch (const std::invalid_argument&) {
        // Expected.
    }

    std::cout << "[PASS] Rectangle Sketch -> Face -> Pad\n"
              << "[PASS] SHA-256 GeometryId = " << geometry_id << '\n'
              << "[PASS] BBox = 100 x 60 x 40 mm\n"
              << "[PASS] Volume = 240000 mm^3\n"
              << "[PASS] Topology = 6 faces / 12 edges / 8 vertices / 1 solid\n"
              << "[PASS] Mesh triangles = " << mesh.triangles.size() << '\n'
              << "[PASS] GLB bytes = " << glb.size() << '\n'
              << "[PASS] B-Rep round trip\n";
    std::cout << "[PASS] XY / XZ / YZ datum plane orientations\n";
    return EXIT_SUCCESS;
}
