#pragma once

#include <TopoDS_Shape.hxx>

#include <functional>

namespace occccad::kernel::detail {

// The predecessor must be a validated, immutable input to a non-destructive
// operation. An absent predecessor always invokes the whole-shape OCCT oracle.
bool valid_body_boundary(const TopoDS_Shape& result, const TopoDS_Shape& predecessor,
                         const std::function<bool()>& cancelled, bool& parallel);

// A positive result proves membership changed across an ADD or REMOVE.
// Inconclusive probes leave the Boolean partition as the authority.
bool proves_material_change(const TopoDS_Shape& input, const TopoDS_Shape& tool,
                            const TopoDS_Shape& result, bool addition,
                            const std::function<bool()>& cancelled);

}  // namespace occccad::kernel::detail
