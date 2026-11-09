#include <BRepAdaptor_Curve.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepTools.hxx>
#include <BRep_Tool.hxx>
#include <Geom2d_Line.hxx>
#include <GeomConvert.hxx>
#include <GeomConvert_BSplineCurveToBezierCurve.hxx>
#include <Geom_BSplineCurve.hxx>
#include <Geom_BSplineSurface.hxx>
#include <Geom_BezierCurve.hxx>
#include <Geom_RectangularTrimmedSurface.hxx>
#include <Geom_TrimmedCurve.hxx>
#include <TColgp_Array1OfPnt.hxx>
#include <TColgp_Array2OfPnt.hxx>
#include <TopoDS.hxx>

#include <internal/geometry_feature_support.hpp>

#include <array>
#include <cmath>

namespace occccad::kernel::detail {
using namespace geometry_features;
std::map<int, TopoDS_Edge> surface_rectangle_sides(const TopoDS_Face& face,
                                                   const std::string& prefix) {
    double u0, u1, v0, v1;
    BRepTools::UVBounds(face, u0, u1, v0, v1);
    if (!std::isfinite(u0) || !std::isfinite(u1) || !std::isfinite(v0) || !std::isfinite(v1) ||
        members(face, TopAbs_WIRE).size() != 1)
        throw std::invalid_argument(prefix + "_RECTANGULAR_DOMAIN_REQUIRED");
    std::map<int, TopoDS_Edge> sides;
    for (const auto& shape : members(face, TopAbs_EDGE)) {
        const auto edge = TopoDS::Edge(shape);
        double first, last;
        auto curve = BRep_Tool::CurveOnSurface(edge, face, first, last);
        if (curve.IsNull())
            throw std::invalid_argument(prefix + "_BOUNDARY_PCURVE_REQUIRED");
        const auto line = Handle(Geom2d_Line)::DownCast(curve);
        if (line.IsNull())
            throw std::invalid_argument(prefix + "_ISOPARAMETRIC_BOUNDARY_REQUIRED");
        const auto origin = line->Location();
        const auto dir = line->Direction();
        const double eps = 1e-8 * std::max({1., std::abs(u1 - u0), std::abs(v1 - v0)});
        int side = -1;
        if (std::abs(dir.X()) < 1e-10) {
            if (std::abs(origin.X() - u0) <= eps)
                side = 0;
            else if (std::abs(origin.X() - u1) <= eps)
                side = 1;
        } else if (std::abs(dir.Y()) < 1e-10) {
            if (std::abs(origin.Y() - v0) <= eps)
                side = 2;
            else if (std::abs(origin.Y() - v1) <= eps)
                side = 3;
        }
        if (side < 0 || sides.count(side))
            throw std::invalid_argument(prefix + "_RECTANGULAR_DOMAIN_REQUIRED");
        sides.emplace(side, edge);
    }
    // Periodic charts have the same seam edge twice; the omitted alias is exact
    // topology, and that seam is never a free boundary available to extend.
    if (sides.size() == 3) {
        if (!sides.count(0) && sides.count(1))
            sides[0] = sides[1];
        else if (!sides.count(1) && sides.count(0))
            sides[1] = sides[0];
        else if (!sides.count(2) && sides.count(3))
            sides[2] = sides[3];
        else if (!sides.count(3) && sides.count(2))
            sides[3] = sides[2];
    }
    if (sides.size() != 4)
        throw std::invalid_argument(prefix + "_RECTANGULAR_DOMAIN_REQUIRED");
    return sides;
}
namespace {
using Curve = Handle(Geom_BSplineCurve);
using Jet = std::array<Curve, 3>;
using V = gp_XYZ;
const V zero(0, 0, 0);
Curve copy(const Curve& c) {
    return Handle(Geom_BSplineCurve)::DownCast(c->Copy());
}
Curve curve(const std::vector<V>& poles, const Curve& basis) {
    TColgp_Array1OfPnt ps(1, static_cast<int>(poles.size()));
    for (int i = 1; i <= ps.Length(); ++i)
        ps(i) = gp_Pnt(poles[i - 1]);
    return new Geom_BSplineCurve(ps, basis->Knots(), basis->Multiplicities(), basis->Degree());
}
Curve bezier(const std::array<V, 6>& p) {
    TColgp_Array1OfPnt ps(1, 6);
    for (int i = 1; i <= 6; ++i)
        ps(i) = gp_Pnt(p[i - 1]);
    return GeomConvert::CurveToBSplineCurve(new Geom_BezierCurve(ps));
}
Curve hermite(const std::array<V, 6>& j) {
    // Input roles: value, first and second derivative at each endpoint.
    return bezier({j[0], j[0] + j[1] / 5, j[0] + j[1] * .4 + j[2] / 20,
                   j[3] - j[4] * .4 + j[5] / 20, j[3] - j[4] / 5, j[3]});
}
std::array<Curve, 6> hermite_basis() {
    std::array<Curve, 6> out;
    for (int i = 0; i < 6; ++i) {
        std::array<V, 6> j{};
        j[i] = V(1, 0, 0);
        out[i] = hermite(j);
    }
    return out;
}
void common(std::vector<Curve>& cs) {
    int degree = 1;
    std::map<double, int> knots;
    for (const auto& c : cs) {
        if (c->IsRational() || c->IsPeriodic() || c->Degree() > 12 || c->NbPoles() > 128)
            throw std::invalid_argument("SURFACE_SUPPORT_POLYNOMIAL_DOMAIN_REQUIRED");
        degree = std::max(degree, c->Degree());
    }
    for (auto& c : cs) {
        c->IncreaseDegree(degree);
        for (int i = 1; i <= c->NbKnots(); ++i)
            knots[c->Knot(i)] = std::max(knots[c->Knot(i)], c->Multiplicity(i));
    }
    if (knots.size() * degree > 512)
        throw std::invalid_argument("SURFACE_CONTROL_BUDGET");
    for (auto& c : cs)
        for (const auto& [k, m] : knots)
            c->InsertKnot(k, m, 1e-12, Standard_False);
    for (const auto& c : cs)
        if (c->NbPoles() != cs.front()->NbPoles())
            throw std::invalid_argument("SURFACE_SUPPORT_BASIS_MISMATCH");
}
Curve normalized(Curve c) {
    c = copy(c);
    c->Segment(c->FirstParameter(), c->LastParameter());
    const double a = c->FirstParameter(), d = c->LastParameter() - a;
    if (d <= 1e-12)
        throw std::invalid_argument("SURFACE_DEGENERATE_BOUNDARY");
    TColStd_Array1OfReal ks = c->Knots();
    for (int i = ks.Lower(); i <= ks.Upper(); ++i)
        ks(i) = (ks(i) - a) / d;
    c->SetKnots(ks);
    return c;
}
Curve boundary_curve(const GeometryFeatureBuild& input) {
    const auto e = single_edge(input);
    if (BRep_Tool::IsClosed(e))
        throw std::invalid_argument("SURFACE_OPEN_BOUNDARY_REQUIRED");
    double a, b;
    auto c = BRep_Tool::Curve(e, a, b);
    if (c.IsNull())
        throw std::invalid_argument("CURVE_INPUT_REQUIRED");
    auto out = GeomConvert::CurveToBSplineCurve(new Geom_TrimmedCurve(c, a, b));
    if (e.Orientation() == TopAbs_REVERSED)
        out->Reverse();
    return normalized(out);
}
V derivative(const Curve& c, double p, int n) {
    return n == 0 ? c->Value(p).XYZ() : n > c->Degree() ? zero : c->DN(p, n).XYZ();
}
Curve constant(const Curve& basis, V v = zero) {
    return curve(std::vector<V>(basis->NbPoles(), v), basis);
}
Curve combine(Curve a, Curve b, double sa = 1, double sb = 1) {
    std::vector<Curve> cs{copy(a), copy(b)};
    common(cs);
    std::vector<V> ps;
    for (int i = 1; i <= cs[0]->NbPoles(); ++i)
        ps.push_back(cs[0]->Pole(i).XYZ() * sa + cs[1]->Pole(i).XYZ() * sb);
    return curve(ps, cs[0]);
}
void scale(Curve& c, double a) {
    for (int i = 1; i <= c->NbPoles(); ++i)
        c->SetPole(i, gp_Pnt(c->Pole(i).XYZ() * a));
}
double difference(Curve a, Curve b) {
    std::vector<Curve> cs{copy(a), copy(b)};
    common(cs);
    double d = 0;
    for (int i = 1; i <= cs[0]->NbPoles(); ++i)
        d = std::max(d, cs[0]->Pole(i).Distance(cs[1]->Pole(i)));
    return d;
}
Jet support_jet(const GeometryFeatureSpec& s, const GeometryFeatureBuild& input,
                const GeometryBoundaryControl& control, bool at_end, const Curve& other) {
    if (!control.support)
        throw std::invalid_argument("SURFACE_SUPPORT_REQUIRED");
    Jet out;
    out[0] = boundary_curve(input);
    const int order = control.continuity == "G2" ? 2 : 1;
    for (int i = 2; i < out[0]->NbKnots(); ++i)
        if (out[0]->Degree() - out[0]->Multiplicity(i) < order)
            throw std::invalid_argument("SURFACE_SUPPORT_BOUNDARY_CONTINUITY_REQUIRED");
    if (control.support->plane) {
        const auto& p = *control.support->plane;
        const gp_Vec n(direction(p.normal));
        for (int i = 1; i <= out[0]->NbPoles(); ++i)
            if (std::abs(gp_Vec(point(p.origin), out[0]->Pole(i)).Dot(n)) > s.surface.tolerance)
                throw std::invalid_argument("SURFACE_BOUNDARY_NOT_ON_SUPPORT");
        // Only straight boundaries have a constant transverse plane derivative.
        // General polynomial curves use the exact derivative field below.
        const int degree = out[0]->Degree();
        std::vector<V> ds;
        // Build derivative as a B-spline via OCCT's exact derivative conversion.
        Handle(Geom_BSplineCurve) d = Handle(Geom_BSplineCurve)::DownCast(out[0]->Copy());
        // Interpolating derivatives is deliberately avoided: convert each Bezier
        // span and elevate its derivative into the original polynomial basis.
        GeomConvert_BSplineCurveToBezierCurve spans(d);
        if (spans.NbArcs() != 1)
            throw std::invalid_argument("SURFACE_PLANE_SUPPORT_SINGLE_SPAN_REQUIRED");
        auto b = spans.Arc(1);
        TColgp_Array1OfPnt ps(1, degree);
        for (int i = 1; i <= degree; ++i) {
            gp_Vec t(b->Pole(i), b->Pole(i + 1));
            ps(i) = gp_Pnt(n.Crossed(t.Multiplied(degree)).XYZ());
        }
        if (degree == 1)
            out[1] = constant(out[0], ps(1).XYZ());
        else {
            Handle(Geom_BezierCurve) dc = new Geom_BezierCurve(ps);
            dc->Increase(degree);
            out[1] = GeomConvert::CurveToBSplineCurve(dc);
        }
        out[2] = constant(out[0]);
        // Plane orientation and boundary direction define the transverse side.
        // Reversal is an explicit design input; an upstream change cannot
        // silently switch the branch by a nearest/opposite-boundary heuristic.
        if (at_end && s.operation != "SURFACE_BLEND")
            scale(out[1], -1);
    } else {
        const auto support = read_geometry_input(*control.support);
        const auto fs = members(support.shape, TopAbs_FACE);
        if (fs.size() != 1)
            throw std::invalid_argument("SURFACE_SUPPORT_SINGLE_FACE_REQUIRED");
        const auto f = TopoDS::Face(fs.front());
        const auto ref = named(input, single_edge(input)).ref;
        auto owned = std::find_if(support.named.begin(), support.named.end(), [&](const auto& n) {
            return n.shape.ShapeType() == TopAbs_EDGE && key(n.ref) == key(ref);
        });
        if (owned == support.named.end() || !contains(f, owned->shape))
            throw std::invalid_argument("SURFACE_SUPPORT_OWNED_EDGE_REQUIRED");
        const auto sides = surface_rectangle_sides(f, "SURFACE_SUPPORT");
        int side = -1;
        for (const auto& [role, e] : sides)
            if (e.IsSame(owned->shape)) {
                if (side >= 0)
                    throw std::invalid_argument("SURFACE_SUPPORT_SEAM_UNSUPPORTED");
                side = role;
            }
        if (side < 0)
            throw std::invalid_argument("SURFACE_SUPPORT_ISOPARAMETRIC_EDGE_REQUIRED");
        double u0, u1, v0, v1;
        BRepTools::UVBounds(f, u0, u1, v0, v1);
        auto bs = GeomConvert::SurfaceToBSplineSurface(
            new Geom_RectangularTrimmedSurface(BRep_Tool::Surface(f), u0, u1, v0, v1));
        if (bs->IsURational() || bs->IsVRational())
            throw std::invalid_argument("SURFACE_SUPPORT_POLYNOMIAL_DOMAIN_REQUIRED");
        const bool along_u = side >= 2, high = side == 1 || side == 3;
        const int count = along_u ? bs->NbUPoles() : bs->NbVPoles();
        std::array<std::vector<V>, 3> poles;
        Curve along;
        for (int i = 1; i <= count; ++i) {
            const int cross = along_u ? bs->NbVPoles() : bs->NbUPoles();
            TColgp_Array1OfPnt ps(1, cross);
            for (int j = 1; j <= cross; ++j)
                ps(j) = along_u ? bs->Pole(i, j) : bs->Pole(j, i);
            Curve c = new Geom_BSplineCurve(ps, along_u ? bs->VKnots() : bs->UKnots(),
                                            along_u ? bs->VMultiplicities() : bs->UMultiplicities(),
                                            along_u ? bs->VDegree() : bs->UDegree());
            c = normalized(c);
            const double t = high ? 1 : 0;
            for (int n = 0; n < 3; ++n)
                poles[n].push_back(derivative(c, t, n));
        }
        const auto knots = along_u ? bs->UKnots() : bs->VKnots();
        const auto mults = along_u ? bs->UMultiplicities() : bs->VMultiplicities();
        TColgp_Array1OfPnt aps(1, count);
        for (int i = 1; i <= count; ++i)
            aps(i) = gp_Pnt(poles[0][i - 1]);
        along = normalized(
            new Geom_BSplineCurve(aps, knots, mults, along_u ? bs->UDegree() : bs->VDegree()));
        Jet source;
        for (int n = 0; n < 3; ++n)
            source[n] = curve(poles[n], along);
        // Reversal of this already identified owned edge is an orientation test.
        if (source[0]->Value(0).Distance(out[0]->Value(0)) > s.surface.tolerance)
            for (auto& c : source)
                c->Reverse();
        if (difference(out[0], source[0]) > s.surface.tolerance)
            throw std::invalid_argument("SURFACE_BOUNDARY_SUPPORT_PARAMETERIZATION_MISMATCH");
        out[1] = source[1];
        out[2] = source[2];
        if ((!high) != at_end)
            scale(out[1], -1);
    }
    if (control.support->reversed)
        scale(out[1], -1);
    if (!std::isfinite(control.tension) || control.tension < .01 || control.tension > 100)
        throw std::invalid_argument("SURFACE_TENSION_OUT_OF_RANGE");
    const double magnitude = out[1]->Value(.5).XYZ().Modulus();
    const double gap = out[0]->Value(.5).Distance(other->Value(.5));
    if (magnitude < 1e-10 || gap < s.surface.tolerance)
        throw std::invalid_argument("SURFACE_SUPPORT_SINGULAR_BOUNDARY");
    const double factor = control.tension * gap / magnitude;
    scale(out[1], factor);
    scale(out[2], factor * factor);
    if (control.continuity == "G1")
        out[2] = constant(out[0]);
    return out;
}
// Polynomial Bernstein convex hull certificate. Every complete boundary span
// must have a nonzero normal; inconclusive subdivision is a failure, not G1/G2.
double choose(int n, int k) {
    double v = 1;
    for (int i = 1; i <= k; ++i)
        v = v * (n - i + 1) / i;
    return v;
}
std::vector<V> cross_product(const std::vector<V>& a, const std::vector<V>& b) {
    const int p = static_cast<int>(a.size()) - 1, q = static_cast<int>(b.size()) - 1;
    std::vector<V> c(p + q + 1, zero);
    for (int i = 0; i <= p; ++i)
        for (int j = 0; j <= q; ++j)
            c[i + j] += a[i].Crossed(b[j]) * (choose(p, i) * choose(q, j) / choose(p + q, i + j));
    return c;
}
bool nonzero_hull(const std::vector<V>& p, int depth, const std::function<void()>& check) {
    check();
    V axis = zero;
    for (const auto& v : p)
        axis += v;
    const double len = axis.Modulus();
    if (len > 1e-18) {
        axis /= len;
        bool ok = true;
        for (const auto& v : p)
            ok = ok && v.Dot(axis) > 1e-12;
        if (ok)
            return true;
    }
    if (depth >= 18)
        return false;
    auto tmp = p;
    std::vector<V> left(p.size()), right(p.size());
    left[0] = tmp.front();
    right.back() = tmp.back();
    for (size_t n = 1; n < p.size(); ++n) {
        for (size_t i = 0; i < p.size() - n; ++i)
            tmp[i] = (tmp[i] + tmp[i + 1]) * .5;
        left[n] = tmp[0];
        right[p.size() - 1 - n] = tmp[p.size() - 1 - n];
    }
    return nonzero_hull(left, depth + 1, check) && nonzero_hull(right, depth + 1, check);
}
void regular(const Jet& j, const std::function<void()>& check) {
    std::vector<Curve> cs{copy(j[0]), copy(j[1])};
    common(cs);
    GeomConvert_BSplineCurveToBezierCurve c(cs[0]), d(cs[1]);
    size_t cells = 0;
    const auto bounded = [&] {
        check();
        if (++cells > 8192)
            throw std::invalid_argument("SURFACE_REGULARITY_BUDGET");
    };
    for (int k = 1; k <= c.NbArcs(); ++k) {
        auto a = c.Arc(k), b = d.Arc(k);
        std::vector<V> tangent, transverse;
        for (int i = 1; i < a->NbPoles(); ++i)
            tangent.push_back((a->Pole(i + 1).XYZ() - a->Pole(i).XYZ()) * (a->Degree()));
        for (int i = 1; i <= b->NbPoles(); ++i)
            transverse.push_back(b->Pole(i).XYZ());
        if (!nonzero_hull(cross_product(tangent, transverse), 0, bounded))
            throw std::invalid_argument("SURFACE_BOUNDARY_REGULARITY_UNVERIFIED");
    }
}
Jet patch_jet(const Handle(Geom_BSplineSurface) & surface, bool along_u, bool high) {
    const int count = along_u ? surface->NbUPoles() : surface->NbVPoles();
    std::array<std::vector<V>, 3> poles;
    for (int i = 1; i <= count; ++i) {
        const int cross = along_u ? surface->NbVPoles() : surface->NbUPoles();
        TColgp_Array1OfPnt ps(1, cross);
        for (int j = 1; j <= cross; ++j)
            ps(j) = along_u ? surface->Pole(i, j) : surface->Pole(j, i);
        Curve c =
            new Geom_BSplineCurve(ps, along_u ? surface->VKnots() : surface->UKnots(),
                                  along_u ? surface->VMultiplicities() : surface->UMultiplicities(),
                                  along_u ? surface->VDegree() : surface->UDegree());
        for (int n = 0; n < 3; ++n)
            poles[n].push_back(derivative(c, high ? 1 : 0, n));
    }
    TColgp_Array1OfPnt ps(1, count);
    for (int i = 1; i <= count; ++i)
        ps(i) = gp_Pnt(poles[0][i - 1]);
    Curve basis =
        new Geom_BSplineCurve(ps, along_u ? surface->UKnots() : surface->VKnots(),
                              along_u ? surface->UMultiplicities() : surface->VMultiplicities(),
                              along_u ? surface->UDegree() : surface->VDegree());
    Jet out;
    for (int n = 0; n < 3; ++n)
        out[n] = curve(poles[n], basis);
    return out;
}
void verify_jet(const Jet& expected, const Jet& actual, double tol,
                const std::function<void()>& check) {
    for (int n = 0; n < 3; ++n) {
        check();
        if (difference(expected[n], actual[n]) > tol)
            throw std::invalid_argument("SURFACE_BOUNDARY_JET_RESIDUAL_EXCEEDED");
    }
    regular(actual, check);
}
GeometryFeatureBuild patch(const GeometryFeatureSpec& s,
                           const Handle(Geom_BSplineSurface) & surface,
                           const std::vector<GeometryFeatureBuild>& inputs) {
    BRepBuilderAPI_MakeFace make(surface, s.surface.tolerance);
    if (!make.IsDone())
        throw std::invalid_argument("SURFACE_PATCH_FAILED");
    GeometryFeatureBuild out;
    out.shape = make.Face();
    std::vector<SemanticTopologyRef> sources;
    for (const auto& i : inputs)
        sources.push_back(named(i, single_edge(i)).ref);
    for (const auto& c : s.surface.boundaries)
        if (c.support) {
            auto support = read_geometry_input(*c.support);
            for (const auto& n : support.named)
                if (n.shape.ShapeType() == TopAbs_FACE)
                    sources.push_back(n.ref);
        }
    add(out, derived(s.feature_id, "patch/surface", sources), out.shape, sources);
    const auto sides = surface_rectangle_sides(make.Face(), "SURFACE_PATCH");
    for (const auto& [side, e] : sides)
        add(out,
            derived(s.feature_id,
                    "patch/" + std::string(side == 0   ? "u-start"
                                           : side == 1 ? "u-end"
                                           : side == 2 ? "v-start"
                                                       : "v-end"),
                    sources),
            e, sources);
    if (!BRepAlgoAPI_Check(out.shape, Standard_False, Standard_True).IsValid())
        throw std::invalid_argument("SURFACE_SELF_INTERSECTION_OR_INVALID_RESULT");
    return out;
}
const GeometryBoundaryControl* control(const GeometryFeatureSpec& s, size_t i) {
    const GeometryBoundaryControl* out = nullptr;
    for (const auto& c : s.surface.boundaries) {
        if (c.id.empty() || c.boundary_index >= s.inputs.size() ||
            (c.continuity != "G0" && c.continuity != "G1" && c.continuity != "G2"))
            throw std::invalid_argument("SURFACE_BOUNDARY_CONTROL_INVALID");
        if (c.boundary_index == i) {
            if (out)
                throw std::invalid_argument("SURFACE_BOUNDARY_CONTROL_DUPLICATE");
            out = &c;
        }
    }
    return out;
}
Handle(Geom_BSplineSurface)
    blend(const GeometryFeatureSpec& s, const std::vector<GeometryFeatureBuild>& inputs,
          const std::function<void()>& check) {
    std::array<Jet, 2> js;
    const auto a = boundary_curve(inputs[0]), b = boundary_curve(inputs[1]);
    for (int side = 0; side < 2; ++side) {
        check();
        const auto c = side ? b : a;
        auto ctl = control(s, side);
        if (ctl && ctl->continuity != "G0")
            js[side] = support_jet(s, inputs[side], *ctl, side == 1, side ? a : b);
        else
            js[side] = {c, combine(b, a, 1, -1), constant(c)};
    }
    std::vector<Curve> cs;
    for (auto& j : js)
        for (auto& c : j)
            cs.push_back(c);
    common(cs);
    for (int side = 0; side < 2; ++side) {
        for (int n = 0; n < 3; ++n)
            js[side][n] = cs[side * 3 + n];
        regular(js[side], check);
    }
    TColgp_Array2OfPnt ps(1, cs[0]->NbPoles(), 1, 6);
    for (int i = 1; i <= cs[0]->NbPoles(); ++i) {
        auto h = hermite({cs[0]->Pole(i).XYZ(), cs[1]->Pole(i).XYZ(), cs[2]->Pole(i).XYZ(),
                          cs[3]->Pole(i).XYZ(), cs[4]->Pole(i).XYZ(), cs[5]->Pole(i).XYZ()});
        for (int j = 1; j <= 6; ++j)
            ps(i, j) = h->Pole(j);
    }
    TColStd_Array1OfReal vk(1, 2);
    vk(1) = 0;
    vk(2) = 1;
    TColStd_Array1OfInteger vm(1, 2);
    vm(1) = vm(2) = 6;
    Handle(Geom_BSplineSurface) surface = new Geom_BSplineSurface(
        ps, cs[0]->Knots(), vk, cs[0]->Multiplicities(), vm, cs[0]->Degree(), 5);
    for (int side = 0; side < 2; ++side)
        verify_jet(js[side], patch_jet(surface, true, side == 1), s.surface.tolerance, check);
    return surface;
}
Curve linear(V a, V b) {
    return hermite({a, b - a, zero, b, b - a, zero});
}
Handle(Geom_BSplineSurface)
    boundary(const GeometryFeatureSpec& s, const std::vector<GeometryFeatureBuild>& inputs,
             const std::function<void()>& check) {
    std::array<Curve, 4> c;
    for (int i = 0; i < 4; ++i)
        c[i] = boundary_curve(inputs[i]);
    // Declared loop order is bottom, right, top, left. Upper/left parameter
    // directions are reversed explicitly, never chosen by nearest geometry.
    for (int i = 0; i < 4; ++i)
        if (c[i]->Value(1).Distance(c[(i + 1) % 4]->Value(0)) > s.surface.tolerance)
            throw std::invalid_argument("SURFACE_BOUNDARY_ORDERED_LOOP_REQUIRED");
    c[2]->Reverse();
    c[3]->Reverse();
    const auto& b = c[0];
    const auto& r = c[1];
    const auto& t = c[2];
    const auto& l = c[3];
    std::array<Jet, 2> h, v;
    const auto dh = combine(t, b, 1, -1), dv = combine(r, l, 1, -1);
    const auto hcorner =
        linear(t->Value(0).XYZ() - b->Value(0).XYZ(), t->Value(1).XYZ() - b->Value(1).XYZ());
    const auto vcorner =
        linear(r->Value(0).XYZ() - l->Value(0).XYZ(), r->Value(1).XYZ() - l->Value(1).XYZ());
    for (int side = 0; side < 2; ++side) {
        h[side] = {side ? t : b,
                   combine(combine(dh, linear(derivative(l, side, 1), derivative(r, side, 1))),
                           hcorner, 1, -1),
                   linear(derivative(l, side, 2), derivative(r, side, 2))};
        v[side] = {side ? r : l,
                   combine(combine(dv, linear(derivative(b, side, 1), derivative(t, side, 1))),
                           vcorner, 1, -1),
                   linear(derivative(b, side, 2), derivative(t, side, 2))};
    }
    std::array<int, 2> ho{}, vo{};
    for (int i = 0; i < 4; ++i) {
        check();
        auto ctl = control(s, i);
        if (!ctl || ctl->continuity == "G0")
            continue;
        auto j = support_jet(s, inputs[i], *ctl, i == 1 || i == 2, c[(i + 2) % 4]);
        if (i >= 2)
            for (auto& f : j)
                f->Reverse();
        const int order = ctl->continuity == "G2" ? 2 : 1;
        if (i == 0 || i == 2) {
            const int side = i == 2;
            h[side] = j;
            ho[side] = order;
        } else {
            const int side = i == 1;
            v[side] = j;
            vo[side] = order;
        }
    }
    V corner[2][2][3][3];
    for (int x = 0; x < 2; ++x)
        for (int y = 0; y < 2; ++y)
            for (int a = 0; a < 3; ++a)
                for (int b2 = 0; b2 < 3; ++b2) {
                    const V from_h = derivative(h[y][b2], x, a),
                            from_v = derivative(v[x][a], y, b2);
                    const bool hf = b2 <= ho[y], vf = a <= vo[x];
                    if (hf && vf && (from_h - from_v).Modulus() > s.surface.tolerance)
                        throw std::invalid_argument("SURFACE_SUPPORT_CORNER_JETS_INCOMPATIBLE");
                    corner[x][y][a][b2] = hf ? from_h : vf ? from_v : (from_h + from_v) * .5;
                }
    for (int y = 0; y < 2; ++y)
        for (int b2 = ho[y] + 1; b2 < 3; ++b2) {
            std::array<V, 6> correction;
            for (int x = 0; x < 2; ++x)
                for (int a = 0; a < 3; ++a)
                    correction[x * 3 + a] = corner[x][y][a][b2] - derivative(h[y][b2], x, a);
            h[y][b2] = combine(h[y][b2], hermite(correction));
        }
    for (int x = 0; x < 2; ++x)
        for (int a = vo[x] + 1; a < 3; ++a) {
            std::array<V, 6> correction;
            for (int y = 0; y < 2; ++y)
                for (int b2 = 0; b2 < 3; ++b2)
                    correction[y * 3 + b2] = corner[x][y][a][b2] - derivative(v[x][a], y, b2);
            v[x][a] = combine(v[x][a], hermite(correction));
        }
    std::vector<Curve> us, vs;
    for (int side = 0; side < 2; ++side)
        for (int n = 0; n < 3; ++n) {
            us.push_back(h[side][n]);
            vs.push_back(v[side][n]);
        }
    const auto hb = hermite_basis();
    for (auto& f : hb) {
        us.push_back(copy(f));
        vs.push_back(copy(f));
    }
    common(us);
    common(vs);
    if (us[0]->NbPoles() * vs[0]->NbPoles() > 32768)
        throw std::invalid_argument("SURFACE_CONTROL_BUDGET");
    TColgp_Array2OfPnt poles(1, us[0]->NbPoles(), 1, vs[0]->NbPoles());
    for (int i = 1; i <= poles.ColLength(); ++i) {
        check();
        for (int j = 1; j <= poles.RowLength(); ++j) {
            V p = zero;
            for (int a = 0; a < 6; ++a) {
                p += us[a]->Pole(i).XYZ() * vs[6 + a]->Pole(j).X();
                p += vs[a]->Pole(j).XYZ() * us[6 + a]->Pole(i).X();
                for (int b2 = 0; b2 < 6; ++b2)
                    p -= corner[a / 3][b2 / 3][a % 3][b2 % 3] *
                         (us[6 + a]->Pole(i).X() * vs[6 + b2]->Pole(j).X());
            }
            poles(i, j) = gp_Pnt(p);
        }
    }
    auto surface = Handle(Geom_BSplineSurface)(
        new Geom_BSplineSurface(poles, us[0]->Knots(), vs[0]->Knots(), us[0]->Multiplicities(),
                                vs[0]->Multiplicities(), us[0]->Degree(), vs[0]->Degree()));
    // The tensor construction enforces full polynomial jets, including mixed
    // derivatives at the corners. Check floating point residuals by coefficient
    // convex hull, and certify non-singular normals on every Bezier span.
    for (int side = 0; side < 2; ++side) {
        verify_jet({us[side * 3], us[side * 3 + 1], us[side * 3 + 2]},
                   patch_jet(surface, true, side == 1), s.surface.tolerance, check);
        verify_jet({vs[side * 3], vs[side * 3 + 1], vs[side * 3 + 2]},
                   patch_jet(surface, false, side == 1), s.surface.tolerance, check);
    }
    return surface;
}
}  // namespace
GeometryFeatureBuild build_border_guide_surface(const GeometryFeatureSpec& s,
                                                const std::vector<GeometryFeatureBuild>& inputs,
                                                const std::function<bool()>& cancelled) {
    size_t cells = 0;
    const auto check = [&] {
        if (cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
        if (++cells > 32768)
            throw std::invalid_argument("SURFACE_CONTROL_BUDGET");
    };
    check();
    const bool is_loft = s.operation == "SURFACE_LOFT";
    if (!s.surface.guide || (is_loft && !s.surface.spine))
        throw std::invalid_argument("SURFACE_GUIDE_SPINE_REQUIRED");
    const auto spine = read_geometry_input(is_loft ? *s.surface.spine : s.inputs[1]),
               guide = read_geometry_input(*s.surface.guide);
    const auto spine_edge = single_edge(spine), guide_edge = single_edge(guide);
    BRepAdaptor_Curve path(spine_edge);
    if (path.GetType() != GeomAbs_Line)
        throw std::invalid_argument("SURFACE_BORDER_GUIDE_STRAIGHT_SPINE_REQUIRED");
    if (!internally_g1(guide_edge, tolerance, topology_angular_tolerance_radians, cancelled))
        throw std::invalid_argument("SURFACE_GUIDE_G1_REQUIRED");
    const bool reverse = spine_edge.Orientation() == TopAbs_REVERSED;
    const gp_Pnt start = path.Value(reverse ? path.LastParameter() : path.FirstParameter()),
                 end = path.Value(reverse ? path.FirstParameter() : path.LastParameter());
    const double length = start.Distance(end);
    if (length <= s.surface.tolerance)
        throw std::invalid_argument("SURFACE_SPINE_SINGULAR");
    const gp_Vec axis(start, end);
    const V direction = axis.Normalized().XYZ();
    const auto rail = boundary_curve(guide);
    std::vector<Curve> basis{rail};
    common(basis);
    const auto along = [&](const gp_Pnt& p) { return (p.XYZ() - start.XYZ()).Dot(direction); };
    if (std::abs(along(rail->Value(0))) > s.surface.tolerance ||
        std::abs(along(rail->Value(1)) - length) > s.surface.tolerance)
        throw std::invalid_argument("SURFACE_GUIDE_SPINE_LIMITS_REQUIRED");
    std::vector<V> projected, radial;
    for (int i = 1; i <= rail->NbPoles(); ++i) {
        const auto p = rail->Pole(i);
        const auto q = start.XYZ() + direction * along(p);
        projected.push_back(q);
        radial.push_back(p.XYZ() - q);
    }
    const auto center = curve(projected, rail), radius = curve(radial, rail);
    GeomConvert_BSplineCurveToBezierCurve spans(rail), radial_spans(radius);
    for (int k = 1; k <= spans.NbArcs(); ++k) {
        check();
        const auto arc = spans.Arc(k), r = radial_spans.Arc(k);
        std::vector<V> radial_poles;
        for (int i = 1; i <= r->NbPoles(); ++i)
            radial_poles.push_back(r->Pole(i).XYZ());
        if (!nonzero_hull(radial_poles, 0, check))
            throw std::invalid_argument("SURFACE_GUIDE_CROSSES_SPINE");
        // Positive Bernstein derivative coefficients certify that each normal
        // plane has one intersection with the complete guide, including knots.
        for (int i = 1; i < arc->NbPoles(); ++i)
            if ((arc->Pole(i + 1).XYZ() - arc->Pole(i).XYZ()).Dot(direction) * arc->Degree() <=
                1e-10)
                throw std::invalid_argument("SURFACE_GUIDE_MONOTONICITY_UNVERIFIED");
    }
    double previous = -1;
    for (size_t i = 0; i < (is_loft ? inputs.size() : 1); ++i) {
        check();
        const auto e = single_edge(inputs[i]);
        BRepAdaptor_Curve section(e);
        if (section.GetType() != GeomAbs_Line)
            throw std::invalid_argument("SURFACE_BORDER_GUIDE_LINEAR_SECTION_REQUIRED");
        const GeometrySectionControl* control = nullptr;
        for (const auto& c : s.surface.sections)
            if (c.section_index == i)
                control = &c;
        if (control && (control->closing_point || !control->couplings.empty()))
            throw std::invalid_argument("SURFACE_BORDER_GUIDE_SECTION_CONTROL_UNSUPPORTED");
        double station = 0;
        if (is_loft) {
            if (!control || !control->station_point)
                throw std::invalid_argument("SURFACE_SECTION_STATION_REQUIRED");
            const auto p = point(*control->station_point);
            station = along(p);
            if (p.Distance(gp_Pnt(start.XYZ() + direction * station)) > s.surface.tolerance ||
                station < -s.surface.tolerance || station > length + s.surface.tolerance ||
                station <= previous + s.surface.tolerance)
                throw std::invalid_argument("SURFACE_SPINE_STATION_ORDER_INVALID");
            if ((i == 0 && std::abs(station) > s.surface.tolerance) ||
                (i == inputs.size() - 1 && std::abs(station - length) > s.surface.tolerance))
                throw std::invalid_argument("SURFACE_SPINE_SECTION_LIMITS_REQUIRED");
            previous = station;
        }
        double lo = 0, hi = 1;
        for (int step = 0; step < 64; ++step) {
            const double mid = (lo + hi) / 2;
            if (along(rail->Value(mid)) < station)
                lo = mid;
            else
                hi = mid;
        }
        const double u = (lo + hi) / 2;
        const auto a = section.Value(e.Orientation() == TopAbs_REVERSED ? section.LastParameter()
                                                                        : section.FirstParameter()),
                   b = section.Value(e.Orientation() == TopAbs_REVERSED ? section.FirstParameter()
                                                                        : section.LastParameter());
        if (a.Distance(center->Value(u)) > s.surface.tolerance ||
            b.Distance(rail->Value(u)) > s.surface.tolerance)
            throw std::invalid_argument("SURFACE_BORDER_GUIDE_SECTION_ENDPOINTS_REQUIRED");
    }
    TColgp_Array2OfPnt poles(1, rail->NbPoles(), 1, 2);
    for (int i = 1; i <= rail->NbPoles(); ++i) {
        poles(i, 1) = center->Pole(i);
        poles(i, 2) = rail->Pole(i);
    }
    TColStd_Array1OfReal knots(1, 2);
    knots(1) = 0;
    knots(2) = 1;
    TColStd_Array1OfInteger mults(1, 2);
    mults(1) = mults(2) = 2;
    Handle(Geom_BSplineSurface) surface = new Geom_BSplineSurface(
        poles, rail->Knots(), knots, rail->Multiplicities(), mults, rail->Degree(), 1);
    std::vector<GeometryFeatureBuild> sources;
    for (size_t i = 0; i < (is_loft ? inputs.size() : 1); ++i)
        sources.push_back(inputs[i]);
    sources.push_back(spine);
    sources.push_back(guide);
    check();
    return patch(s, surface, sources);
}

GeometryFeatureBuild build_shape_control_surface(const GeometryFeatureSpec& s,
                                                 const std::vector<GeometryFeatureBuild>& inputs,
                                                 const std::function<bool()>& cancelled) {
    const auto check = [&] {
        if (cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
    };
    check();
    const bool two = s.operation == "SURFACE_BLEND";
    if (inputs.size() != (two ? 2U : 4U))
        throw std::invalid_argument(two ? "SURFACE_BLEND_TWO_BOUNDARIES_REQUIRED"
                                        : "SURFACE_BOUNDARY_FOUR_EDGES_REQUIRED");
    if (!std::isfinite(s.surface.tolerance) || s.surface.tolerance < 1e-7 ||
        s.surface.tolerance > 1e-2)
        throw std::invalid_argument("SURFACE_TOLERANCE_OUT_OF_RANGE");
    if (s.surface.spine || s.surface.guide || !s.surface.sections.empty())
        throw std::invalid_argument("SURFACE_CONTROL_ROLE_INVALID");
    auto surface = two ? blend(s, inputs, check) : boundary(s, inputs, check);
    check();
    return patch(s, surface, inputs);
}
}  // namespace occccad::kernel::detail
