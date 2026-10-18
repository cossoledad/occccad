#pragma once

#include <BRepTools_History.hxx>
#include <TopoDS_Shape.hxx>

#include <vector>

namespace occccad::kernel {

struct ShellCornerWall {
    TopoDS_Shape shape;
    Handle(BRepTools_History) history;
};

// Exact edge-distance material missing between the normal layers of a sharp,
// concave circular plane/cylinder junction. Opening boundaries are excluded.
std::vector<ShellCornerWall> circular_shell_corner_walls(const TopoDS_Shape& input,
                                                         const std::vector<TopoDS_Shape>& openings,
                                                         double thickness, double tolerance);

}  // namespace occccad::kernel
