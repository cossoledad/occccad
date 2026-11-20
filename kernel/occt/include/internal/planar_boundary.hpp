#pragma once

#include <TopoDS_Compound.hxx>
#include <TopoDS_Face.hxx>

#include <functional>

// Add equivalent face checks for spatially separated planar trimming loops.
// Returns false without changing checks when the original face is required.
namespace occccad::kernel::detail {
bool append_planar_face_checks(const TopoDS_Face& face, TopoDS_Compound& checks,
                               const std::function<bool()>& cancelled);
}
