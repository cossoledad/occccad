#pragma once

#include <BRepTools_History.hxx>
#include <TopoDS_Shape.hxx>

namespace occccad::kernel {

struct ShellJunctionRefinement {
    TopoDS_Shape shape;
    Handle(BRepTools_History) history;
    bool changed = false;
};

ShellJunctionRefinement refine_shell_junctions(const TopoDS_Shape& shape, double tolerance);

}  // namespace occccad::kernel
