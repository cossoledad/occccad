#pragma once
#include <BRepTools_History.hxx>
#include <TopoDS_Shape.hxx>

#include <functional>
#include <vector>

namespace occccad::kernel::detail {
struct RuledShell {
    TopoDS_Shape shape;
    Handle(BRepTools_History) history;
};
bool has_ruled_wall_ring(const TopoDS_Shape& stock, const std::vector<TopoDS_Shape>& openings);
RuledShell inward_ruled_shell(const TopoDS_Shape&, const std::vector<TopoDS_Shape>&,
                              double thickness, double tolerance,
                              const std::function<void()>& check);
}  // namespace occccad::kernel::detail
