#pragma once

#include <IMeshTools_Context.hxx>
#include <TopoDS_Shape.hxx>

namespace occccad::kernel {

// Planes use OCCT's Delabella base triangulation. Curved-surface refinement
// keeps the normal deflection policy. The retry additionally prevents generic
// surfaces from welding distinct 3D boundary samples by UV proximity alone.
Handle(IMeshTools_Context) display_mesh_context(bool preserve_boundary = false);

bool has_ordered_surface_boundaries(const TopoDS_Shape& shape);

}  // namespace occccad::kernel
