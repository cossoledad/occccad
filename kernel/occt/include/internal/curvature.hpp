#pragma once
#include <TopoDS_Shape.hxx>

#include <occccad/kernel/measurement.hpp>

#include <functional>
namespace occccad::kernel::detail {
MeasurementResult analyze_curvature(const std::vector<TopoDS_Shape>&, uint32_t,
                                    const std::function<bool()>&);
}
