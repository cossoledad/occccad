#pragma once
#include <BRepTools_History.hxx>
#include <TopoDS_Shape.hxx>

#include <string>
#include <vector>
namespace occccad::kernel::detail {
struct FilletBoundaryResult {
    TopoDS_Shape shape;
    Handle(BRepTools_History) history;
    std::vector<std::string> diagnostics;
    bool IsDone() const { return !shape.IsNull(); }
    const TopoDS_Shape& Shape() const { return shape; }
    const TopTools_ListOfShape& Modified(const TopoDS_Shape& s) { return history->Modified(s); }
    const TopTools_ListOfShape& Generated(const TopoDS_Shape& s) { return history->Generated(s); }
    bool IsDeleted(const TopoDS_Shape& s) { return history->IsRemoved(s); }
};
// Only analytic, locally bounded constructions. Empty means unsupported, not success.
FilletBoundaryResult rebuild_fillet_boundary(const TopoDS_Shape& input,
                                             const std::vector<TopoDS_Shape>& selected,
                                             double radius);
}  // namespace occccad::kernel::detail
