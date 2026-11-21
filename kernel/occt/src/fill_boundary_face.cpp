#include <Adaptor3d_CurveOnSurface.hxx>
#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Curve2d.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepBuilderAPI_FindPlane.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Geom2d_BSplineCurve.hxx>
#include <GeomConvert.hxx>
#include <GeomConvert_CompCurveToBSplineCurve.hxx>
#include <GeomFill_Generator.hxx>
#include <GeomLib_CheckCurveOnSurface.hxx>
#include <Geom_BSplineCurve.hxx>
#include <Geom_BSplineSurface.hxx>
#include <Geom_Plane.hxx>
#include <Geom_TrimmedCurve.hxx>
#include <TopExp.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopoDS_Vertex.hxx>

#include <internal/fill_boundary_face.hpp>

#include <algorithm>
#include <cmath>
#include <limits>

namespace occccad::kernel::detail {
namespace {
using Loop = std::vector<TopoDS_Edge>;
struct Side {
    TopoDS_Edge source;
    gp_Pnt2d from, to;  // In wire traversal order.
};
struct Rail {
    Handle(Geom_BSplineCurve) curve;
    std::vector<double> stops{0};
};

Handle(Geom_BSplineCurve) polynomial_span(const TopoDS_Edge& edge) {
    BRepAdaptor_Curve c(edge);
    const auto type = c.GetType();
    if (type != GeomAbs_Line && type != GeomAbs_BezierCurve && type != GeomAbs_BSplineCurve)
        return {};
    if (type != GeomAbs_Line && (c.Degree() > 12 || c.NbPoles() > 128))
        return {};
    double a, b;
    auto geom = BRep_Tool::Curve(edge, a, b);
    if (geom.IsNull())
        return {};
    auto span = GeomConvert::CurveToBSplineCurve(new Geom_TrimmedCurve(geom, a, b));
    return span->IsRational() ? Handle(Geom_BSplineCurve){} : span;
}

Rail make_rail(const Loop& edges, double tolerance) {
    GeomConvert_CompCurveToBSplineCurve join;
    Rail rail;
    for (const auto& e : edges) {
        auto span = polynomial_span(e);
        if (span.IsNull())
            return {};
        if (e.Orientation() == TopAbs_REVERSED)
            span->Reverse();
        // Chord lengths keep short segments local in the common longitudinal parameter.
        const double length = span->StartPoint().Distance(span->EndPoint());
        if (length <= Precision::Confusion())
            return {};
        auto knots = span->Knots();
        for (int i = 1; i <= knots.Length(); ++i)
            knots(i) = (knots(i) - span->FirstParameter()) /
                       (span->LastParameter() - span->FirstParameter()) * length;
        span->SetKnots(knots);
        if (!join.Add(span, tolerance, true, false, Geom_BSplineCurve::MaxDegree()))
            return {};
        rail.curve = join.BSplineCurve();
        if (rail.curve->NbPoles() > 128)
            return {};
        rail.stops.push_back(rail.curve->LastParameter());
    }
    const double length = rail.stops.back();
    for (auto& t : rail.stops)
        t /= length;
    auto knots = rail.curve->Knots();
    for (int i = 1; i <= knots.Length(); ++i)
        knots(i) /= length;
    rail.curve->SetKnots(knots);
    return rail;
}

// All longitudinal derivative controls must cross every ruling control with the
// same sign in projection. Then even an integral of tangents between two stations
// cannot be parallel to the ruling joining two points at the second station.
bool regular_strip(const Handle(Geom_BSplineSurface) & s, const std::function<void()>& check) {
    if (s.IsNull() || s->IsURational() || s->IsVRational() || s->NbVPoles() != 2 ||
        s->NbUPoles() > 256)
        return false;
    const int n = s->NbUPoles();
    gp_Vec normal =
        gp_Vec(s->Pole(1, 1), s->Pole(n, 1)).Crossed(gp_Vec(s->Pole(1, 1), s->Pole(1, 2)));
    if (normal.Magnitude() <= Precision::SquareConfusion())
        return false;
    normal.Normalize();
    for (int i = 1; i < n; ++i) {
        check();
        for (int side : {1, 2}) {
            const gp_Vec tangent(s->Pole(i, side), s->Pole(i + 1, side));
            for (int j = 1; j <= n; ++j) {
                const gp_Vec width(s->Pole(j, 1), s->Pole(j, 2));
                if (tangent.Crossed(width).Dot(normal) <=
                    Precision::Angular() * tangent.Magnitude() * width.Magnitude())
                    return false;
            }
        }
    }
    return true;
}

// Imported and generated straight edges can be encoded as polynomial splines.
// The linear interpolant has coefficients at the Greville parameters in this basis.
double straight_span(const TopoDS_Edge& edge) {
    BRepAdaptor_Curve c(edge);
    const auto a = c.FirstParameter(), b = c.LastParameter();
    const auto start = c.Value(a), end = c.Value(b);
    if (c.GetType() != GeomAbs_Line) {
        auto spline = polynomial_span(edge);
        if (spline.IsNull())
            return 0;
        const auto& knots = spline->KnotSequence();
        for (int i = 1; i <= spline->NbPoles(); ++i) {
            double u = 0;
            for (int j = 1; j <= spline->Degree(); ++j)
                u += knots(i + j);
            const double t = (u / spline->Degree() - a) / (b - a);
            if (spline->Pole(i).Distance(start.Translated(gp_Vec(start, end) * t)) >
                Precision::Confusion())
                return 0;
        }
    }
    return start.Distance(end);
}

Handle(Geom_Surface) strip_support(const Loop& loop, double tolerance, std::vector<Side>& sides,
                                   const std::function<void()>& check) {
    const size_t n = loop.size();
    if (n < 4 || n > 32)
        return {};
    // Prefer short transverse ends. Ambiguous equal pairs have no preferred ruling.
    double best = std::numeric_limits<double>::infinity();
    size_t first = 0, last = 0;
    std::vector<double> lengths(n);
    for (size_t i = 0; i < n; ++i)
        lengths[i] = straight_span(loop[i]);
    double runner_up = best;
    for (size_t i = 0; i < n; ++i)
        for (size_t j = i + 2; j < n; ++j) {
            if ((j + 1) % n == i || !lengths[i] || !lengths[j])
                continue;
            const double score = lengths[i] + lengths[j];
            if (score < best) {
                runner_up = best;
                best = score;
                first = i;
                last = j;
            } else
                runner_up = std::min(runner_up, score);
        }
    if (!std::isfinite(best) || runner_up - best <= Precision::Confusion())
        return {};
    Loop bottom, top;
    for (size_t i = first + 1; i < last; ++i)
        bottom.push_back(loop[i]);
    for (size_t i = (first + n - 1) % n; i != last; i = (i + n - 1) % n)
        top.push_back(TopoDS::Edge(loop[i].Reversed()));
    auto a = make_rail(bottom, tolerance), b = make_rail(top, tolerance);
    if (a.curve.IsNull() || b.curve.IsNull())
        return {};
    GeomFill_Generator generator;
    generator.AddCurve(a.curve);
    generator.AddCurve(b.curve);
    generator.Perform(1e-12);
    const auto surface = Handle(Geom_BSplineSurface)::DownCast(generator.Surface());
    if (!regular_strip(surface, check))
        return {};
    for (size_t i = 0; i < bottom.size(); ++i)
        sides.push_back({bottom[i], {a.stops[i], 0}, {a.stops[i + 1], 0}});
    sides.push_back({loop[last], {1, 0}, {1, 1}});
    for (size_t i = top.size(); i > 0; --i)
        sides.push_back(
            {TopoDS::Edge(top[i - 1].Reversed()), {b.stops[i], 1}, {b.stops[i - 1], 1}});
    sides.push_back({loop[first], {0, 1}, {0, 0}});
    return surface;
}

Handle(Geom2d_Curve) parameter_line(gp_Pnt2d from, gp_Pnt2d to, double a, double b) {
    TColgp_Array1OfPnt2d poles(1, 2);
    poles(1) = from;
    poles(2) = to;
    TColStd_Array1OfReal knots(1, 2);
    knots(1) = a;
    knots(2) = b;
    TColStd_Array1OfInteger mults(1, 2);
    mults.Init(2);
    return new Geom2d_BSplineCurve(poles, knots, mults, 1);
}
}  // namespace

std::optional<FillEdgeDeviation> measure_fill_edge(const TopoDS_Edge& source,
                                                   const TopoDS_Edge& result,
                                                   const TopoDS_Face& face) {
    if (!BRep_Tool::SameParameter(result) || !BRep_Tool::SameRange(result) ||
        BRep_Tool::IsClosed(result, face))
        return {};
    Handle(BRepAdaptor_Curve) original = new BRepAdaptor_Curve(source);
    Handle(BRepAdaptor_Curve2d) pcurve = new BRepAdaptor_Curve2d(result, face);
    if (std::abs(original->FirstParameter() - pcurve->FirstParameter()) > Precision::PConfusion() ||
        std::abs(original->LastParameter() - pcurve->LastParameter()) > Precision::PConfusion())
        return {};
    GeomLib_CheckCurveOnSurface deviation(original);
    deviation.Perform(new Adaptor3d_CurveOnSurface(pcurve, new BRepAdaptor_Surface(face, false)));
    if (!deviation.IsDone() || !std::isfinite(deviation.MaxDistance()) ||
        !std::isfinite(deviation.MaxParameter()))
        return {};
    return FillEdgeDeviation{deviation.MaxDistance(), original->Value(deviation.MaxParameter())};
}

bool build_fill_boundary_face(const Loop& loop, double tolerance, FillSupport support,
                              const std::function<void()>& check, FillBoundaryFace& out) {
    try {
        BRep_Builder builder;
        Handle(Geom_Surface) surface;
        std::vector<Side> sides;
        if (support == FillSupport::plane) {
            TopoDS_Compound all;
            builder.MakeCompound(all);
            for (const auto& edge : loop)
                builder.Add(all, edge);
            BRepBuilderAPI_FindPlane fit(all, Precision::Confusion());
            if (!fit.Found())
                return false;
            surface = fit.Plane();
            for (const auto& edge : loop)
                sides.push_back({edge, {}, {}});
        } else {
            surface = strip_support(loop, tolerance, sides, check);
            if (surface.IsNull())
                return false;
        }
        BRepBuilderAPI_MakeWire wire;
        Loop built;
        for (const auto& side : sides) {
            check();
            double a, b;
            auto curve = BRep_Tool::Curve(side.source, a, b);
            if (curve.IsNull())
                return false;
            BRepBuilderAPI_MakeEdge make(curve, a, b);
            if (!make.IsDone())
                return false;
            auto edge = make.Edge();
            const bool reverse = side.source.Orientation() == TopAbs_REVERSED;
            if (support == FillSupport::ruled) {
                auto uv = reverse ? parameter_line(side.to, side.from, a, b)
                                  : parameter_line(side.from, side.to, a, b);
                builder.UpdateEdge(edge, uv, surface, TopLoc_Location(), Precision::Confusion());
            }
            if (reverse)
                edge.Reverse();
            wire.Add(edge);
            if (!wire.IsDone())
                return false;
            built.push_back(TopoDS::Edge(wire.Edge().Oriented(side.source.Orientation())));
        }
        BRepBuilderAPI_MakeFace face(surface, wire.Wire(), true);
        if (!face.IsDone() || !BRepAlgoAPI_Check(face.Face(), false, true).IsValid())
            return false;
        TopTools_IndexedMapOfShape reachable;
        TopExp::MapShapes(face.Face(), TopAbs_EDGE, reachable);
        if (static_cast<size_t>(reachable.Extent()) != sides.size())
            return false;
        for (size_t i = 0; i < sides.size(); ++i) {
            check();
            const auto& edge = built[i];
            if (!reachable.Contains(edge))
                return false;
            const auto deviation = measure_fill_edge(sides[i].source, edge, face.Face());
            if (!deviation || deviation->distance > tolerance)
                return false;
        }
        for (size_t i = 0; i < sides.size(); ++i) {
            const auto& src = sides[i].source;
            out.history.AddGenerated(src, built[i]);
            out.history.AddGenerated(TopExp::FirstVertex(src, true),
                                     TopExp::FirstVertex(built[i], true));
            out.history.AddGenerated(TopExp::LastVertex(src, true),
                                     TopExp::LastVertex(built[i], true));
        }
        out.face = face.Face();
        return true;
    } catch (const Standard_Failure&) {
        return false;
    }
}
}  // namespace occccad::kernel::detail
