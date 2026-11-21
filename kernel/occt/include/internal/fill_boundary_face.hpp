#pragma once
#include <BRepTools_History.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Face.hxx>

#include <functional>
#include <optional>
#include <vector>

namespace occccad::kernel::detail {
struct FillEdgeDeviation {
    double distance;
    gp_Pnt point;
};
std::optional<FillEdgeDeviation> measure_fill_edge(const TopoDS_Edge& source,
                                                   const TopoDS_Edge& result,
                                                   const TopoDS_Face& face);

enum class FillSupport { plane, ruled };
struct FillBoundaryFace {
    TopoDS_Face face;
    BRepTools_History history;
};
// One trimmed face with the original finite 3D curves, verified against its support.
bool build_fill_boundary_face(const std::vector<TopoDS_Edge>& loop, double tolerance,
                              FillSupport support, const std::function<void()>& check,
                              FillBoundaryFace& out);
}  // namespace occccad::kernel::detail
