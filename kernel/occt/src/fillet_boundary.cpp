#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAlgoAPI_Fuse.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakePolygon.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepClass3d_SolidClassifier.hxx>
#include <BRepClass_FaceClassifier.hxx>
#include <BRepFilletAPI_MakeFillet.hxx>
#include <BRepGProp.hxx>
#include <BRepLib_CheckCurveOnSurface.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakeHalfSpace.hxx>
#include <BRepPrimAPI_MakePrism.hxx>
#include <BRepTools.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GProp_GProps.hxx>
#include <Geom2d_Curve.hxx>
#include <ShapeFix_ShapeTolerance.hxx>
#include <ShapeUpgrade_UnifySameDomain.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopTools_ListIteratorOfListOfShape.hxx>
#include <TopoDS.hxx>

#include <internal/fillet_boundary.hpp>

#include <algorithm>
#include <chrono>
#include <cmath>
#include <iostream>
#include <stdexcept>

namespace occccad::kernel::detail {
namespace {
constexpr double tolerance = 1e-7;
struct Stage {
    const char* name;
    std::chrono::steady_clock::time_point start = std::chrono::steady_clock::now();
    ~Stage() {
        std::clog << "fillet_boundary stage=" << name << " ms="
                  << std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() -
                                                               start)
                         .count()
                  << '\n';
    }
};
double volume(const TopoDS_Shape& s) {
    GProp_GProps p;
    BRepGProp::VolumeProperties(s, p);
    return p.Mass();
}
gp_Dir normal(const TopoDS_Face& f) {
    auto n = BRepAdaptor_Surface(f).Plane().Axis().Direction();
    if (f.Orientation() == TopAbs_REVERSED)
        n.Reverse();
    return n;
}
bool convex_cylinder(const TopoDS_Face& face) {
    BRepAdaptor_Surface surface(face);
    gp_Pnt p;
    gp_Vec du, dv;
    surface.D1((surface.FirstUParameter() + surface.LastUParameter()) / 2,
               (surface.FirstVParameter() + surface.LastVParameter()) / 2, p, du, dv);
    auto outward = du.Crossed(dv);
    if (face.Orientation() == TopAbs_REVERSED)
        outward.Reverse();
    const auto cylinder = surface.Cylinder();
    gp_Vec radial(cylinder.Location(), p);
    radial -= gp_Vec(cylinder.Axis().Direction()) * radial.Dot(gp_Vec(cylinder.Axis().Direction()));
    return outward.Dot(radial) > 0;
}
void valid(const TopoDS_Shape& s, const char* stage) {
    if (s.IsNull() || !TopExp_Explorer(s, TopAbs_SOLID).More() || volume(s) <= 1e-9)
        throw std::invalid_argument(std::string("FILLET_BOUNDARY_") + stage + "_NO_MATERIAL");
    if (!BRepCheck_Analyzer(s, Standard_True, Standard_False, Standard_True).IsValid())
        throw std::runtime_error(std::string("FILLET_BOUNDARY_") + stage + "_INVALID_SOLID");
}
double shape_tolerance(const TopoDS_Shape& shape) {
    double result = 0;
    for (TopExp_Explorer it(shape, TopAbs_VERTEX); it.More(); it.Next())
        result = std::max(result, BRep_Tool::Tolerance(TopoDS::Vertex(it.Current())));
    for (TopExp_Explorer it(shape, TopAbs_EDGE); it.More(); it.Next())
        result = std::max(result, BRep_Tool::Tolerance(TopoDS::Edge(it.Current())));
    return result;
}
void check_tolerance(const TopoDS_Shape& input, FilletBoundaryResult& result) {
    const double recorded = shape_tolerance(result.shape),
                 limit = std::max(1e-6, shape_tolerance(input) * 1.01);
    if (recorded <= limit)
        return;
    // OCCT can record conservative corner tolerances. Tightening is permitted
    // only after independent curve/surface and endpoint distance checks. This
    // neither moves geometry nor merges topology, and never raises a tolerance.
    double deviation = 0;
    for (TopExp_Explorer f(result.shape, TopAbs_FACE); f.More(); f.Next()) {
        for (TopExp_Explorer e(f.Current(), TopAbs_EDGE); e.More(); e.Next()) {
            const auto edge = TopoDS::Edge(e.Current());
            if (BRep_Tool::Degenerated(edge))
                continue;
            if (!BRep_Tool::SameParameter(edge) || !BRep_Tool::SameRange(edge))
                throw std::runtime_error("FILLET_BOUNDARY_PCURVE_PARAMETER_MISMATCH");
            BRepLib_CheckCurveOnSurface check(edge, TopoDS::Face(f.Current()));
            check.Perform();
            if (!check.IsDone())
                throw std::runtime_error("FILLET_BOUNDARY_PCURVE_CHECK_FAILED");
            deviation = std::max(deviation, check.MaxDistance());
            TopoDS_Vertex first, last;
            TopExp::Vertices(edge, first, last);
            BRepAdaptor_Curve curve(edge);
            if (!first.IsNull())
                deviation = std::max(
                    deviation, BRep_Tool::Pnt(first).Distance(curve.Value(curve.FirstParameter())));
            if (!last.IsNull())
                deviation = std::max(
                    deviation, BRep_Tool::Pnt(last).Distance(curve.Value(curve.LastParameter())));
        }
    }
    std::clog << "fillet_boundary stage=tolerance recorded=" << recorded
              << " deviation=" << deviation << " limit=" << limit << '\n';
    if (deviation > limit)
        throw std::runtime_error("FILLET_BOUNDARY_TOLERANCE_EXCEEDED");
    BRepBuilderAPI_Copy copy(result.shape, true, false);
    const auto tightened = copy.Shape();
    ShapeFix_ShapeTolerance().LimitTolerance(tightened, 0, limit, TopAbs_WIRE);
    valid(tightened, "TIGHTENED_TOLERANCE");
    TopTools_ListOfShape args;
    args.Append(result.shape);
    result.history->Merge(args, copy);
    result.shape = tightened;
}
template <class Op>
TopoDS_Shape boolean_shape(const TopoDS_Shape& a, const TopoDS_Shape& b,
                           Handle(BRepTools_History) & history, const char* stage) {
    Stage timing{stage};
    Op op;
    TopTools_ListOfShape objects, tools;
    objects.Append(a);
    tools.Append(b);
    op.SetArguments(objects);
    op.SetTools(tools);
    op.SetNonDestructive(Standard_True);
    op.Build();
    if (!op.IsDone() || op.HasErrors())
        throw std::runtime_error(std::string("FILLET_BOUNDARY_") + stage + "_INTERSECTION_FAILED");
    TopTools_ListOfShape args;
    args.Append(a);
    args.Append(b);
    history->Merge(args, op);
    return op.Shape();
}
struct Plane {
    TopoDS_Face face;
    gp_Pln plane;
    gp_Dir outward;
    bool bounding;
};
struct Support {
    TopoDS_Shape edge;
    int first, second;
    gp_Pnt point, center, tangent1, tangent2;
    gp_Dir axis;
    bool convex;
};
TopoDS_Shape halfspace(const Plane& p, double offset, TopoDS_Face& boundary) {
    auto plane = p.plane;
    plane.SetLocation(plane.Location().Translated(gp_Vec(p.outward) * offset));
    boundary = BRepBuilderAPI_MakeFace(plane).Face();
    return BRepPrimAPI_MakeHalfSpace(boundary, plane.Location().Translated(gp_Vec(p.outward) * -1))
        .Solid();
}
std::vector<TopoDS_Shape> descendants(const TopoDS_Shape& face,
                                      const Handle(BRepTools_History) & history,
                                      const TopoDS_Shape& shape) {
    TopTools_IndexedMapOfShape members;
    TopExp::MapShapes(shape, TopAbs_FACE, members);
    std::vector<TopoDS_Shape> result;
    if (members.Contains(face))
        result.push_back(face);
    for (TopTools_ListIteratorOfListOfShape it(history->Modified(face)); it.More(); it.Next())
        if (members.Contains(it.Value()))
            result.push_back(it.Value());
    return result;
}
TopoDS_Shape common_edge(const std::vector<TopoDS_Shape>& first,
                         const std::vector<TopoDS_Shape>& second) {
    TopTools_IndexedMapOfShape found;
    for (const auto& a : first)
        for (const auto& b : second) {
            TopTools_IndexedMapOfShape edges;
            TopExp::MapShapes(a, TopAbs_EDGE, edges);
            for (TopExp_Explorer e(b, TopAbs_EDGE); e.More(); e.Next())
                if (edges.Contains(e.Current()))
                    found.Add(e.Current());
        }
    if (found.Extent() != 1)
        throw std::runtime_error("FILLET_BOUNDARY_SUPPORT_INTERSECTION_AMBIGUOUS");
    return found(1);
}
void check_corner_tangency(const TopoDS_Shape& shape) {
    Stage timing{"corner_tangency"};
    TopTools_IndexedDataMapOfShapeListOfShape adjacent;
    TopExp::MapShapesAndAncestors(shape, TopAbs_EDGE, TopAbs_FACE, adjacent);
    for (int i = 1; i <= adjacent.Extent(); ++i) {
        const auto& pair = adjacent.FindFromIndex(i);
        if (pair.Extent() != 2)
            continue;
        const auto first = TopoDS::Face(pair.First()), second = TopoDS::Face(pair.Last());
        BRepAdaptor_Surface a(first), b(second);
        if (a.GetType() == GeomAbs_Plane || b.GetType() == GeomAbs_Plane)
            continue;  // Limiting caps need not be tangent.
        if (a.GetType() != GeomAbs_BSplineSurface && b.GetType() != GeomAbs_BSplineSurface)
            continue;
        const auto edge = TopoDS::Edge(adjacent.FindKey(i));
        if (BRep_Tool::Degenerated(edge))
            continue;
        double lo, hi, otherLo, otherHi;
        const auto ca = BRep_Tool::CurveOnSurface(edge, first, lo, hi),
                   cb = BRep_Tool::CurveOnSurface(edge, second, otherLo, otherHi);
        if (ca.IsNull() || cb.IsNull() || std::abs(lo - otherLo) > tolerance ||
            std::abs(hi - otherHi) > tolerance)
            throw std::runtime_error("FILLET_BOUNDARY_PCURVE_PARAMETER_MISMATCH");
        for (int sample = 1; sample <= 7; ++sample) {
            const double parameter = lo + (hi - lo) * sample / 8.;
            const auto normalAt = [&](const BRepAdaptor_Surface& surface,
                                      const Handle(Geom2d_Curve) & curve) {
                const auto uv = curve->Value(parameter);
                gp_Pnt p;
                gp_Vec du, dv;
                surface.D1(uv.X(), uv.Y(), p, du, dv);
                return gp_Dir(du.Crossed(dv));
            };
            const double cosine = std::abs(normalAt(a, ca).Dot(normalAt(b, cb)));
            if (cosine < 1 - 1e-6) {
                std::clog << "fillet_boundary stage=corner_tangency normal_cosine=" << cosine
                          << '\n';
                throw std::runtime_error("FILLET_BOUNDARY_CORNER_TANGENCY_FAILED");
            }
        }
    }
}
// Parallel-capped convex prisms may include cylindrical side supports.
// Extend the actual cap and its boundary, preserving exact curve provenance.
FilletBoundaryResult prism_boundary(const TopoDS_Shape& input,
                                    const std::vector<TopoDS_Shape>& selected, double radius) {
    bool curvedSupport = false;
    for (TopExp_Explorer it(input, TopAbs_FACE); it.More(); it.Next())
        curvedSupport |=
            BRepAdaptor_Surface(TopoDS::Face(it.Current())).GetType() == GeomAbs_Cylinder;
    if (!curvedSupport)
        return {};
    TopTools_IndexedDataMapOfShapeListOfShape adjacent;
    TopExp::MapShapesAndAncestors(input, TopAbs_EDGE, TopAbs_FACE, adjacent);
    if (!adjacent.Contains(selected.front()))
        return {};
    TopoDS_Face cap;
    for (TopTools_ListIteratorOfListOfShape it(adjacent.FindFromKey(selected.front())); it.More();
         it.Next()) {
        const auto f = TopoDS::Face(it.Value());
        if (BRepAdaptor_Surface(f).GetType() != GeomAbs_Plane)
            continue;
        TopTools_IndexedMapOfShape boundary;
        TopExp::MapShapes(f, TopAbs_EDGE, boundary);
        bool all = true;
        for (const auto& e : selected)
            all &= boundary.Contains(e);
        if (!all)
            continue;
        const auto inward = normal(f).Reversed();
        bool family = true;
        int ends = 0;
        for (TopExp_Explorer faces(input, TopAbs_FACE); faces.More(); faces.Next()) {
            const auto face = TopoDS::Face(faces.Current());
            BRepAdaptor_Surface surface(face);
            if (surface.GetType() == GeomAbs_Plane) {
                const double alignment = std::abs(normal(face).Dot(inward));
                if (alignment > 1 - 1e-9)
                    ++ends;
                else if (alignment > 1e-9)
                    family = false;
            } else if (surface.GetType() != GeomAbs_Cylinder || !convex_cylinder(face) ||
                       std::abs(surface.Cylinder().Axis().Direction().Dot(inward)) < 1 - 1e-9)
                family = false;
        }
        if (family && ends == 2) {
            cap = f;
            break;
        }
    }
    if (cap.IsNull())
        return {};
    TopTools_IndexedMapOfShape wires;
    TopExp::MapShapes(cap, TopAbs_WIRE, wires);
    if (wires.Extent() != 1)
        return {};
    const auto plane = BRepAdaptor_Surface(cap).Plane();
    const auto inward = normal(cap).Reversed();
    double height = 0;
    for (TopExp_Explorer it(input, TopAbs_VERTEX); it.More(); it.Next()) {
        const double d = gp_Vec(plane.Location(), BRep_Tool::Pnt(TopoDS::Vertex(it.Current())))
                             .Dot(gp_Vec(inward));
        if (d < -tolerance)
            return {};
        height = std::max(height, d);
    }
    if (radius < height - tolerance)
        return {};
    // Every planar side is a global material support; reject concave profiles.
    for (TopExp_Explorer it(input, TopAbs_FACE); it.More(); it.Next()) {
        auto face = TopoDS::Face(it.Current());
        BRepAdaptor_Surface surface(face);
        if (surface.GetType() != GeomAbs_Plane)
            continue;
        for (TopExp_Explorer v(input, TopAbs_VERTEX); v.More(); v.Next())
            if (gp_Vec(surface.Plane().Location(), BRep_Tool::Pnt(TopoDS::Vertex(v.Current())))
                    .Dot(gp_Vec(normal(face))) > tolerance)
                return {};
    }
    BRepBuilderAPI_Copy copy(cap, true, false);
    BRepPrimAPI_MakePrism work(copy.Shape(), gp_Vec(inward) * (height + 4 * radius));
    BRepFilletAPI_MakeFillet round(work.Shape());
    round.SetParams(1e-2, 1e-8, 1e-8, 1e-8, 1e-8, 1e-4);
    std::vector<TopoDS_Shape> workEdges;
    for (const auto& e : selected) {
        auto edge = work.FirstShape(copy.ModifiedShape(e));
        workEdges.push_back(edge);
        round.Add(radius, TopoDS::Edge(edge));
    }
    {
        Stage timing{"prismatic_tangent_and_corner"};
        round.Build();
    }
    if (!round.IsDone())
        return {};
    FilletBoundaryResult result;
    result.history = new BRepTools_History();
    for (size_t i = 0; i < selected.size(); ++i)
        for (TopTools_ListIteratorOfListOfShape it(round.Generated(workEdges[i])); it.More();
             it.Next())
            result.history->AddGenerated(selected[i], it.Value());
    result.shape = boolean_shape<BRepAlgoAPI_Common>(round.Shape(), input, result.history,
                                                     "prismatic_limit_trim");
    check_corner_tangency(result.shape);
    Stage validation{"prismatic_validation"};
    valid(result.shape, "PRISMATIC_CLOSURE");
    check_tolerance(input, result);
    BRepAlgoAPI_Check check(result.shape, false, true);
    if (!check.IsValid())
        throw std::runtime_error("FILLET_BOUNDARY_SELF_INTERFERENCE");
    TopTools_IndexedMapOfShape live;
    TopExp::MapShapes(result.shape, TopAbs_FACE, live);
    for (const auto& edge : selected) {
        bool retained = false;
        for (TopTools_ListIteratorOfListOfShape it(result.history->Generated(edge)); it.More();
             it.Next())
            retained |= live.Contains(it.Value());
        if (!retained)
            throw std::runtime_error("FILLET_BOUNDARY_TRANSITION_CONSUMED_BY_LIMITS");
    }
    result.diagnostics.push_back("FILLET_SUPPORT_DOMAIN_EXTENDED:ANALYTIC_PRISM");
    return result;
}
}  // namespace
FilletBoundaryResult rebuild_fillet_boundary(const TopoDS_Shape& input,
                                             const std::vector<TopoDS_Shape>& selected,
                                             double radius) {
    Stage total{"total"};
    TopTools_IndexedMapOfShape faces, vertices, solids;
    TopExp::MapShapes(input, TopAbs_FACE, faces);
    TopExp::MapShapes(input, TopAbs_VERTEX, vertices);
    TopExp::MapShapes(input, TopAbs_SOLID, solids);
    if (solids.Extent() != 1 || faces.Extent() > 128 || selected.empty() || selected.size() > 64)
        return {};
    Bnd_Box budget;
    BRepBndLib::AddOptimal(input, budget, false, false);
    double bx0, by0, bz0, bx1, by1, bz1;
    budget.Get(bx0, by0, bz0, bx1, by1, bz1);
    if (!std::isfinite(radius) || radius <= 0 ||
        radius > 100 * gp_Pnt(bx0, by0, bz0).Distance(gp_Pnt(bx1, by1, bz1)))
        throw std::invalid_argument("FILLET_BOUNDARY_SUPPORT_EXTENSION_LIMIT");
    auto prismatic = prism_boundary(input, selected, radius);
    if (prismatic.IsDone())
        return prismatic;
    std::vector<Plane> planes;
    bool convex = true;
    for (int i = 1; i <= faces.Extent(); ++i) {
        const auto face = TopoDS::Face(faces(i));
        BRepAdaptor_Surface s(face);
        if (s.GetType() != GeomAbs_Plane)
            return {};
        const auto n = normal(face);
        bool bounding = true;
        for (int j = 1; j <= vertices.Extent(); ++j)
            if (gp_Vec(s.Plane().Location(), BRep_Tool::Pnt(TopoDS::Vertex(vertices(j))))
                    .Dot(gp_Vec(n)) > tolerance)
                bounding = false;
        convex &= bounding;
        planes.push_back({face, s.Plane(), n, bounding});
    }
    TopTools_IndexedDataMapOfShapeListOfShape edgeFaces;
    TopExp::MapShapesAndAncestors(input, TopAbs_EDGE, TopAbs_FACE, edgeFaces);
    std::vector<Support> supports;
    std::vector<bool> fixed(planes.size(), false);
    double reach = radius;
    bool exceeds = false;
    for (const auto& edge : selected) {
        if (!edgeFaces.Contains(edge) || edgeFaces.FindFromKey(edge).Extent() != 2)
            return {};
        BRepAdaptor_Curve curve(TopoDS::Edge(edge));
        if (curve.GetType() != GeomAbs_Line)
            return {};
        const auto& adjacent = edgeFaces.FindFromKey(edge);
        const int a = faces.FindIndex(adjacent.First()) - 1,
                  b = faces.FindIndex(adjacent.Last()) - 1;
        const auto n1 = gp_Vec(planes[a].outward), n2 = gp_Vec(planes[b].outward);
        const double cosine = n1.Dot(n2);
        if (1 + cosine < 1e-6 || 1 - cosine < 1e-6)
            return {};
        const auto p = curve.Value((curve.FirstParameter() + curve.LastParameter()) / 2);
        const double probe =
            std::min(radius, curve.LastParameter() - curve.FirstParameter()) * 1e-4;
        if (probe < 10 * tolerance)
            return {};
        const auto state =
            BRepClass3d_SolidClassifier(input, p.Translated((n1 - n2) * probe), tolerance).State();
        if (state != TopAbs_IN && state != TopAbs_OUT)
            return {};
        const bool isConvex = state == TopAbs_OUT;
        const double sign = isConvex ? -1 : 1;
        const auto center = p.Translated((n1 + n2) * (sign * radius / (1 + cosine)));
        const auto t1 = center.Translated(n1 * (-sign * radius)),
                   t2 = center.Translated(n2 * (-sign * radius));
        reach = std::max({reach, p.Distance(t1), p.Distance(t2)});
        exceeds |= BRepClass_FaceClassifier(planes[a].face, t1, tolerance).State() != TopAbs_IN ||
                   BRepClass_FaceClassifier(planes[b].face, t2, tolerance).State() != TopAbs_IN;
        supports.push_back({edge, a, b, p, center, t1, t2, curve.Line().Direction(), isConvex});
        fixed[a] = fixed[b] = true;
    }
    if (!exceeds)
        return {};
    Bnd_Box box;
    BRepBndLib::AddOptimal(input, box, Standard_False, Standard_False);
    double x0, y0, z0, x1, y1, z1;
    box.Get(x0, y0, z0, x1, y1, z1);
    const double diagonal = gp_Pnt(x0, y0, z0).Distance(gp_Pnt(x1, y1, z1));
    if (reach > 100 * diagonal)
        throw std::invalid_argument("FILLET_BOUNDARY_SUPPORT_EXTENSION_LIMIT");
    const double margin = 2 * (reach + radius);
    FilletBoundaryResult result;
    result.history = new BRepTools_History();
    if (convex) {
        Stage stage{"convex_support_extension"};
        auto working = BRepPrimAPI_MakeBox(gp_Pnt(x0 - margin, y0 - margin, z0 - margin),
                                           gp_Pnt(x1 + margin, y1 + margin, z1 + margin))
                           .Shape();
        Handle(BRepTools_History) supportHistory = new BRepTools_History();
        std::vector<TopoDS_Face> boundaries;
        for (std::size_t i = 0; i < planes.size(); ++i) {
            TopoDS_Face boundary;
            const auto space = halfspace(planes[i], fixed[i] ? 0 : margin, boundary);
            boundaries.push_back(boundary);
            working = boolean_shape<BRepAlgoAPI_Common>(working, space, supportHistory,
                                                        "support_extension");
        }
        valid(working, "SUPPORT_EXTENSION");
        BRepFilletAPI_MakeFillet round(working);
        for (const auto& s : supports) {
            const auto edge =
                common_edge(descendants(boundaries[s.first], supportHistory, working),
                            descendants(boundaries[s.second], supportHistory, working));
            result.history->AddModified(s.edge, edge);
            round.Add(radius, TopoDS::Edge(edge));
        }
        {
            Stage timing{"tangent_and_corner"};
            round.Build();
        }
        if (!round.IsDone())
            throw std::runtime_error(round.NbFaultyVertices() > 0
                                         ? "FILLET_BOUNDARY_CORNER_CLOSURE_FAILED"
                                         : "FILLET_BOUNDARY_TANGENT_CONSTRUCTION_FAILED");
        TopTools_ListOfShape args;
        args.Append(working);
        // Fillet's generated faces are reached through the constructed contour;
        // retain that explicit construction relation before composing trim history.
        for (const auto& s : supports) {
            const auto edge =
                common_edge(descendants(boundaries[s.first], supportHistory, working),
                            descendants(boundaries[s.second], supportHistory, working));
            for (TopTools_ListIteratorOfListOfShape it(round.Generated(edge)); it.More(); it.Next())
                result.history->AddGenerated(s.edge, it.Value());
        }
        result.history->Merge(args, round);
        result.shape =
            boolean_shape<BRepAlgoAPI_Common>(round.Shape(), input, result.history, "limit_trim");
        result.diagnostics.push_back("FILLET_SUPPORT_DOMAIN_EXTENDED:CONVEX_PLANAR");
    } else {
        // Concave plane-plane roots. A bounded sector outside the rolling
        // cylinder fills material; all global supporting planes remain limits.
        if (supports.size() != 1)
            return {};
        for (const auto& s : supports)
            if (s.convex)
                return {};
        // A selected root must terminate on global material limits. Otherwise
        // extending its cylinder could fill an unrelated cavity along its axis.
        for (const auto& s : supports) {
            TopoDS_Vertex a, b;
            TopExp::Vertices(TopoDS::Edge(s.edge), a, b);
            for (const auto& vertex : {a, b}) {
                bool bounded = false;
                for (const auto& p : planes)
                    if (p.bounding && std::abs(p.outward.Dot(s.axis)) > 1e-6 &&
                        p.plane.Distance(BRep_Tool::Pnt(vertex)) <= tolerance)
                        bounded = true;
                if (!bounded)
                    return {};
            }
        }
        result.shape = input;
        for (const auto& s : supports) {
            double low = 0, high = 0;
            for (int i = 1; i <= vertices.Extent(); ++i) {
                const auto d = gp_Vec(s.point, BRep_Tool::Pnt(TopoDS::Vertex(vertices(i))))
                                   .Dot(gp_Vec(s.axis));
                low = std::min(low, d);
                high = std::max(high, d);
            }
            low -= margin;
            high += margin;
            const auto shift = gp_Vec(s.axis) * low;
            BRepBuilderAPI_MakePolygon poly;
            poly.Add(s.point.Translated(shift));
            poly.Add(s.tangent1.Translated(shift));
            poly.Add(s.center.Translated(shift));
            poly.Add(s.tangent2.Translated(shift));
            poly.Close();
            const auto prism = BRepPrimAPI_MakePrism(BRepBuilderAPI_MakeFace(poly.Wire()).Face(),
                                                     gp_Vec(s.axis) * (high - low))
                                   .Shape();
            BRepPrimAPI_MakeCylinder cylinder(
                gp_Ax2(s.center.Translated(shift), s.axis, gp_Dir(gp_Vec(s.point, s.center))),
                radius, high - low);
            result.history->AddGenerated(s.edge, cylinder.Face());
            auto fill = boolean_shape<BRepAlgoAPI_Cut>(prism, cylinder.Shape(), result.history,
                                                       "tangent_sector");
            for (const auto& p : planes)
                if (p.bounding) {
                    TopoDS_Face boundary;
                    auto space = halfspace(p, 0, boundary);
                    // The limiting plane is constructed from this actual face.
                    result.history->AddGenerated(p.face, boundary);
                    fill = boolean_shape<BRepAlgoAPI_Common>(fill, space, result.history,
                                                             "limit_trim");
                }
            valid(fill, "LIMIT_TRIM");
            result.shape = boolean_shape<BRepAlgoAPI_Fuse>(result.shape, fill, result.history,
                                                           "material_assembly");
        }
        result.diagnostics.push_back("FILLET_SUPPORT_DOMAIN_EXTENDED:CONCAVE_PLANAR");
    }
    {
        Stage stage{"boundary_unify"};
        ShapeUpgrade_UnifySameDomain unify(result.shape, Standard_True, Standard_True,
                                           Standard_False);
        unify.SetSafeInputMode(Standard_True);
        unify.Build();
        result.history->Merge(unify.History());
        result.shape = unify.Shape();
    }
    {
        Stage stage{"validation"};
        valid(result.shape, "CORNER_CLOSURE");
        check_tolerance(input, result);
        BRepAlgoAPI_Check check(result.shape, Standard_False, Standard_True);
        if (!check.IsValid())
            throw std::runtime_error("FILLET_BOUNDARY_SELF_INTERFERENCE");
    }
    // A valid solid with the entire blend consumed is not a successful fillet.
    TopTools_IndexedMapOfShape liveFaces;
    TopExp::MapShapes(result.shape, TopAbs_FACE, liveFaces);
    for (const auto& source : selected) {
        bool retained = false;
        for (TopTools_ListIteratorOfListOfShape it(result.history->Generated(source)); it.More();
             it.Next()) {
            if (it.Value().ShapeType() != TopAbs_FACE || !liveFaces.Contains(it.Value()))
                continue;
            BRepAdaptor_Surface surface(TopoDS::Face(it.Value()));
            if (surface.GetType() == GeomAbs_Cylinder &&
                std::abs(surface.Cylinder().Radius() - radius) <= tolerance)
                retained = true;
        }
        if (!retained)
            throw std::runtime_error("FILLET_BOUNDARY_TRANSITION_CONSUMED_BY_LIMITS");
    }
    if ((convex && volume(result.shape) >= volume(input) - 1e-9) ||
        (!convex && volume(result.shape) <= volume(input) + 1e-9))
        throw std::runtime_error("FILLET_BOUNDARY_NO_MATERIAL_CHANGE");
    return result;
}
}  // namespace occccad::kernel::detail
