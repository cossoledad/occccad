#pragma once

#include <BRepGProp.hxx>
#include <GProp_GProps.hxx>
#include <cmath>
#include <stdexcept>

namespace occccad::kernel::detail {

inline void volume_properties(const TopoDS_Shape& shape, GProp_GProps& properties,
                              bool centroid = false) {
    // Split BSpline spans before adaptive Gauss-Kronrod integration. Fixed
    // quadrature misses rational lofts; unsplit adaptive Gauss can stop early
    // on trimmed loft faces, even after tightening its requested tolerance.
    const auto error = BRepGProp::VolumePropertiesGK(shape, properties, 1e-10,
                                                    false, true, centroid);
    if (!std::isfinite(error) || error < 0 || !std::isfinite(properties.Mass()))
        throw std::runtime_error("VOLUME_INTEGRATION_FAILED");
}

}  // namespace occccad::kernel::detail
