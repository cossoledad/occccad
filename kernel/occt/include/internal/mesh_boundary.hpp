#pragma once

#include <IMeshTools_Context.hxx>
#include <TopoDS_Shape.hxx>

namespace occccad::kernel {

// Retry generic-surface meshing without merging different 3D boundary points
// solely because their UV positions fall inside OCCT's lookup tolerance.
Handle(IMeshTools_Context) boundary_preserving_mesh_context();

bool has_ordered_surface_boundaries(const TopoDS_Shape& shape);

}  // namespace occccad::kernel
