#pragma once
#include <occccad/kernel/kernel.hpp>

#include <map>

namespace occccad::kernel {
// Ephemeral locators, valid only in the supplied immutable B-Rep. The control
// plane resolves persistent supports before constructing this input.
struct MeasurementElement {
    PlacedGeometry geometry;
    std::string kind{"BODY"};  // BODY, FACE, EDGE, VERTEX
    uint64_t local_id{};
    std::string mode{"GEOMETRY"};  // GEOMETRY, CENTER, AXIS
};
struct CurvatureSample {
    Vec3 position, normal;
    double minimum{}, maximum{}, mean{}, gaussian{};
    bool surface{};
};
struct MeasurementResult {
    bool complete{};
    std::string diagnostic, geometry_type;
    // Suffixes specify units. Coordinates and anchors are in the owner frame.
    std::map<std::string, double> values;
    std::map<std::string, Vec3> anchors;
    std::vector<CurvatureSample> curvature;
    uint32_t undefined_samples{};
};
}  // namespace occccad::kernel
