#pragma once

#include <BRepGProp.hxx>
#include <GProp_GProps.hxx>
#include <cmath>
#include <cstdint>
#include <stdexcept>

namespace occccad::kernel::detail {

// Counts actual calls, including explicit measurements and callers without a
// Feature ShapeFacts context. A request takes a delta on its owning Worker thread.
inline uint64_t& volume_integration_calls() {
    static thread_local uint64_t count=0;
    return count;
}

void volume_properties(const TopoDS_Shape& shape, GProp_GProps& properties,
                       bool centroid = false);

}  // namespace occccad::kernel::detail
