#pragma once

#include <BRepGProp.hxx>
#include <GProp_GProps.hxx>
#include <cmath>
#include <stdexcept>

namespace occccad::kernel::detail {

void volume_properties(const TopoDS_Shape& shape, GProp_GProps& properties,
                       bool centroid = false);

}  // namespace occccad::kernel::detail
