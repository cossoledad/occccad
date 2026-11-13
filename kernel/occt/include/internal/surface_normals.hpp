#pragma once
#include <Poly_Triangulation.hxx>
#include <TopoDS_Face.hxx>

#include <occccad/kernel/kernel.hpp>

#include <functional>

namespace occccad::kernel::detail {
// Oriented, Part-local normals from the same face UV nodes as the mesh.
// Zero requests the existing within-face area-normal fallback.
void append_surface_normals(const TopoDS_Face& face,
                            const Handle(Poly_Triangulation) & triangulation,
                            std::vector<Vec3>& normals, VisualStageTimings& timings,
                            const std::function<void()>& checkpoint);
}  // namespace occccad::kernel::detail
