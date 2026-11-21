#pragma once

#include <BRepTools_History.hxx>
#include <TopoDS_Solid.hxx>
#include <gp_Pln.hxx>

namespace occccad::kernel::detail {

// Non-destructive partition of one connected solid into two connected solids.
// Business Body identities are assigned by the document, never by TopExp order.
struct PlanePartition {
    TopoDS_Solid positive, negative;
    Handle(BRepTools_History) history;
};

PlanePartition partition_solid(const TopoDS_Shape& input, const gp_Pln& plane);

} // namespace occccad::kernel::detail
