#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepClass3d_SolidClassifier.hxx>
#include <BRepLib.hxx>
#include <BRepPrimAPI_MakeRevol.hxx>
#include <BRep_Tool.hxx>
#include <GC_MakeArcOfCircle.hxx>
#include <Geom_TrimmedCurve.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopoDS.hxx>
#include <gp_Cylinder.hxx>
#include <gp_Lin.hxx>
#include <gp_Pln.hxx>

#include <internal/shell_corner.hpp>

#include <algorithm>
#include <cmath>
#include <stdexcept>

namespace occccad::kernel {
namespace {
TopoDS_Shape circle_corner_sector(const gp_Circ& circle, double thickness, int axialSide) {
    const gp_Vec radial(circle.XAxis().Direction());
    const gp_Vec axial = gp_Vec(circle.Axis().Direction()) * axialSide;
    const auto center = circle.Location().Translated(radial * circle.Radius());
    const auto first = center.Translated(radial * thickness);
    const auto middle = center.Translated((radial + axial) * (thickness / std::sqrt(2.)));
    const auto last = center.Translated(axial * thickness);
    // Only the material quadrant missing from the two face-normal layers.
    // Its radius stays >= the source circle radius, including when thickness
    // equals or exceeds it; no horn/spindle torus or overlapping disk is built.
    const auto arc = GC_MakeArcOfCircle(first, middle, last).Value();
    const auto wire =
        BRepBuilderAPI_MakeWire(BRepBuilderAPI_MakeEdge(arc), BRepBuilderAPI_MakeEdge(last, center),
                                BRepBuilderAPI_MakeEdge(center, first))
            .Wire();
    const auto disk = BRepBuilderAPI_MakeFace(wire).Face();
    BRepPrimAPI_MakeRevol sweep(disk, circle.Axis(), 2 * M_PI, Standard_True);
    if (!sweep.IsDone() || sweep.Shape().ShapeType() != TopAbs_SOLID)
        throw std::runtime_error("SHELL_CIRCULAR_CORNER_CONSTRUCTION_FAILED");
    auto solid = TopoDS::Solid(sweep.Shape());
    if (!BRepLib::OrientClosedSolid(solid) || !BRepCheck_Analyzer(solid).IsValid() ||
        !BRepAlgoAPI_Check(solid, Standard_False, Standard_True).IsValid())
        throw std::runtime_error("SHELL_CIRCULAR_CORNER_CONSTRUCTION_FAILED");
    return solid;
}

}  // namespace

std::vector<ShellCornerWall> circular_shell_corner_walls(const TopoDS_Shape& input,
                                                         const std::vector<TopoDS_Shape>& openings,
                                                         double thickness, double tolerance) {
    std::vector<ShellCornerWall> result;
    TopTools_IndexedMapOfShape solids;
    TopExp::MapShapes(input, TopAbs_SOLID, solids);
    if (solids.Extent() != 1)
        return result;
    BRepClass3d_SolidClassifier classifier(solids(1));
    TopTools_IndexedDataMapOfShapeListOfShape ancestors;
    TopExp::MapShapesAndAncestors(input, TopAbs_EDGE, TopAbs_FACE, ancestors);
    for (int i = 1; i <= ancestors.Extent(); ++i) {
        const auto edge = TopoDS::Edge(ancestors.FindKey(i));
        if (BRep_Tool::Degenerated(edge))
            continue;
        BRepAdaptor_Curve curve(edge);
        if (curve.GetType() != GeomAbs_Circle ||
            std::abs(curve.LastParameter() - curve.FirstParameter() - 2 * M_PI) > 1e-7)
            continue;
        TopTools_IndexedMapOfShape faces;
        bool opening = false;
        for (TopTools_ListIteratorOfListOfShape it(ancestors.FindFromIndex(i)); it.More();
             it.Next()) {
            faces.Add(it.Value());
            opening |= std::any_of(openings.begin(), openings.end(),
                                   [&](const auto& f) { return f.IsSame(it.Value()); });
        }
        if (opening || faces.Extent() != 2)
            continue;
        const auto face1 = TopoDS::Face(faces(1)), face2 = TopoDS::Face(faces(2));
        BRepAdaptor_Surface a(face1), b(face2);
        const auto circle = curve.Circle();
        gp_Cylinder cylinder;
        gp_Pln plane;
        TopoDS_Face cylinderFace;
        if (a.GetType() == GeomAbs_Cylinder && b.GetType() == GeomAbs_Plane) {
            cylinder = a.Cylinder();
            cylinderFace = face1;
            plane = b.Plane();
        } else if (b.GetType() == GeomAbs_Cylinder && a.GetType() == GeomAbs_Plane) {
            cylinder = b.Cylinder();
            cylinderFace = face2;
            plane = a.Plane();
        } else
            continue;
        if (cylinderFace.Orientation() != TopAbs_REVERSED)
            continue;
        if (std::abs(cylinder.Radius() - circle.Radius()) > tolerance ||
            !cylinder.Axis().Direction().IsParallel(circle.Axis().Direction(), 1e-7) ||
            !plane.Axis().Direction().IsParallel(circle.Axis().Direction(), 1e-7) ||
            gp_Lin(cylinder.Axis()).Distance(circle.Location()) > tolerance ||
            plane.Distance(circle.Location()) > tolerance)
            continue;
        const double step = std::max(tolerance * 64, std::min(circle.Radius(), thickness) * .001);
        bool concave = true;
        int axialSide = 0;
        for (const double angle : {.37, 2., 4.}) {
            const gp_Vec radial = gp_Vec(circle.XAxis().Direction()) * std::cos(angle) +
                                  gp_Vec(circle.YAxis().Direction()) * std::sin(angle);
            const gp_Vec axial(circle.Axis().Direction());
            const auto point = circle.Location().Translated(radial * circle.Radius());
            int inside = 0, outside = 0;
            for (double r : {-step, step})
                for (double z : {-step, step}) {
                    classifier.Perform(point.Translated(radial * r + axial * z), tolerance);
                    inside += classifier.State() == TopAbs_IN;
                    outside += classifier.State() == TopAbs_OUT;
                    if (classifier.State() == TopAbs_OUT) {
                        // The void must be on the cylinder-inside side. The
                        // opposite axial quadrant supplies the missing material.
                        const int side = z > 0 ? -1 : 1;
                        concave &= r < 0 && (axialSide == 0 || axialSide == side);
                        axialSide = side;
                    }
                }
            concave &= inside == 3 && outside == 1;
        }
        if (!concave)
            continue;
        ShellCornerWall join{circle_corner_sector(circle, thickness, axialSide),
                             new BRepTools_History()};
        for (TopExp_Explorer f(join.shape, TopAbs_FACE); f.More(); f.Next())
            join.history->AddGenerated(edge, f.Current());
        result.push_back(std::move(join));
    }
    return result;
}
}  // namespace occccad::kernel
