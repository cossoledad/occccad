#pragma once
#include <occccad/kernel/kernel.hpp>

namespace occccad::kernel {
// Exact solid-only analysis, units mm and mm^3. Inconclusive never passes.
struct InterferenceResult {
    std::string classification{"INCONCLUSIVE"}, diagnostic;
    double distance{}, common_volume{};
    Vec3 first_witness, second_witness;
    bool complete{}, clearance_satisfied{}, common_tested{}, distance_exact{};
    std::string witness_kind{"NONE"}, detail_status{"NOT_REQUESTED"}, containment;
    std::vector<Vec3> section_points, overlap_triangles;
};
struct SpatialGeometryOptions {
    std::string kind, section_type;
    PlacedGeometry frame;
    Vec3 size{100,100,10};
    double minimum=0,maximum=10,accuracy=5;
};
struct SpatialGeometryResult {
    bool complete=false;
    std::string diagnostic;
    std::vector<Vec3> segments,triangles;
    std::vector<uint32_t> regions;
    double area=0;
    Vec3 centroid;
};
}
