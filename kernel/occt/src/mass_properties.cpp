#include <internal/mass_properties.hpp>
#include <BRepAdaptor_Surface.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <ShapeUpgrade_ShapeDivideContinuity.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>

namespace occccad::kernel::detail {
void volume_properties(const TopoDS_Shape& shape, GProp_GProps& properties, bool centroid) {
    TopoDS_Shape integration = shape;
    for (TopExp_Explorer it(shape, TopAbs_FACE); it.More(); it.Next()) {
        const BRepAdaptor_Surface surface(TopoDS::Face(it.Current()));
        if (surface.GetType() != GeomAbs_BSplineSurface) continue;
        // OCCT 7.9's GetTKnots does not split boundary parameters at surface
        // V-knot crossings. Nested adaptive integration can spend minutes on
        // these discontinuities. Split exact spans on a private topology/geometry
        // copy; business BREP, face identities and naming remain unchanged.
        BRepBuilderAPI_Copy copy(shape, true, false);
        ShapeUpgrade_ShapeDivideContinuity split(copy.Shape());
        split.SetSurfaceCriterion(GeomAbs_CN);
        split.Perform();
        // A failed optional decomposition must not reduce the original exact
        // integrator's supported input domain.
        if (!split.Status(ShapeExtend_FAIL)) integration = split.Result();
        break;
    }
    const auto error = BRepGProp::VolumePropertiesGK(integration, properties, 1e-10,
                                                    false, true, centroid);
    if (!std::isfinite(error) || error < 0 || !std::isfinite(properties.Mass()))
        throw std::runtime_error("VOLUME_INTEGRATION_FAILED");
}
}
