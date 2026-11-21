#include "internal/ruled_shell.hpp"

#include <BRepAdaptor_Surface.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeSolid.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepBuilderAPI_Sewing.hxx>
#include <BRepLib.hxx>
#include <BRepTools.hxx>
#include <BRep_Tool.hxx>
#include <GeomAPI_Interpolate.hxx>
#include <Geom_BSplineCurve.hxx>
#include <Geom_BSplineSurface.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Shell.hxx>
#include <TopoDS_Solid.hxx>
#include <TopoDS_Wire.hxx>
#include <gp_Pln.hxx>

#include <stdexcept>

namespace occccad::kernel::detail {
namespace {
void require(bool condition, const char* code) {
    if (!condition)
        throw std::runtime_error(code);
}
struct Wall {
    TopoDS_Face face;
    Handle(Geom_Surface) surface;
    double u0, u1, v0, v1;
    bool swap = false, reverse = false;
    gp_Pnt evaluate(double s, double v, double thickness) const {
        if (reverse)
            s = 1 - s;
        const auto u = u0 + (u1 - u0) * s, w = v0 + (v1 - v0) * v;
        gp_Pnt p;
        gp_Vec du, dv;
        surface->D1(swap ? w : u, swap ? u : w, p, du, dv);
        gp_Vec n = du.Crossed(dv);
        n.Normalize();
        return p.Translated(n * (face.Orientation() == TopAbs_REVERSED ? thickness : -thickness));
    }
};
struct Stations {
    std::vector<Wall> walls;
    gp_Pnt origin;
    gp_Vec axis;
    double height, thickness, tolerance;
    gp_Pnt section(const Wall& w, double u, double z) const {
        double v = z / height;
        for (int j = 0; j < 12; ++j) {
            auto p = w.evaluate(u, v, thickness);
            const auto error = gp_Vec(origin, p).Dot(axis) - z;
            if (std::abs(error) < tolerance * .002)
                return p;
            const auto derivative =
                gp_Vec(p, w.evaluate(u, v + 1.e-5, thickness)).Dot(axis) / 1.e-5;
            require(std::abs(derivative) > height * .1, "SHELL_STATION_FOLDS");
            v -= error / derivative;
        }
        throw std::runtime_error("SHELL_STATION_UNRESOLVED");
    }
    struct Trim {
        double left = 0, right = 1;
        gp_Pnt start, end;
    };
    std::vector<Trim> trims(double z) const {
        std::vector<Trim> ranges(walls.size());
        for (size_t i = 0; i < walls.size(); ++i) {
            auto k = (i + 1) % walls.size();
            double a = 1, b = 0;
            for (int j = 0;; ++j) {
                auto p = section(walls[i], a, z), q = section(walls[k], b, z);
                gp_Vec gap(p, q);
                if (gap.Magnitude() < tolerance * .005) {
                    ranges[i].right = a;
                    ranges[k].left = b;
                    ranges[i].end = ranges[k].start = gp_Pnt((p.XYZ() + q.XYZ()) * .5);
                    break;
                }
                require(j < 36, "SHELL_CORNER_INTERSECTION_UNRESOLVED");
                const auto x = gp_Vec(p, section(walls[i], a + 1.e-6, z)) / 1.e-6;
                const auto y = gp_Vec(q, section(walls[k], b + 1.e-6, z)) / 1.e-6;
                const double det = x.Crossed(y).Dot(axis);
                require(std::abs(det) > 1e-13, "SHELL_CORNER_TANGENCY");
                const double da = gap.Crossed(y).Dot(axis) / det,
                             db = gap.Crossed(x).Dot(axis) / det;
                const double step = std::max({1., std::abs(da) / .01, std::abs(db) / .01});
                a += da / step;
                b += db / step;
                require(std::abs(a - 1) < .25 && std::abs(b) < .25, "SHELL_CORNER_OUT_OF_RANGE");
            }
        }
        return ranges;
    }
    gp_Pnt sample(size_t i, const Trim& trim, double s, double z) const {
        if (s == 0)
            return trim.start;
        if (s == 1)
            return trim.end;
        return section(walls[i], trim.left + s * (trim.right - trim.left), z);
    }
};
Handle(Geom_BSplineCurve) interpolate(const std::vector<gp_Pnt>& points) {
    const auto count = static_cast<int>(points.size());
    Handle(TColgp_HArray1OfPnt) p = new TColgp_HArray1OfPnt(1, count);
    Handle(TColStd_HArray1OfReal) t = new TColStd_HArray1OfReal(1, count);
    for (int i = 1; i <= count; ++i) {
        p->SetValue(i, points[i - 1]);
        t->SetValue(i, double(i - 1) / (count - 1));
    }
    GeomAPI_Interpolate fit(p, t, false, 1.e-10);
    fit.Perform();
    require(fit.IsDone(), "SHELL_OFFSET_INTERPOLATION_FAILED");
    return fit.Curve();
}
Handle(Geom_BSplineSurface) tensor(const std::vector<std::vector<gp_Pnt>>& rows) {
    std::vector<Handle(Geom_BSplineCurve)> curves;
    for (const auto& r : rows)
        curves.push_back(interpolate(r));
    const auto u = curves.front();
    std::vector<Handle(Geom_BSplineCurve)> columns;
    for (int i = 1; i <= u->NbPoles(); ++i) {
        std::vector<gp_Pnt> data;
        for (auto& c : curves)
            data.push_back(c->Pole(i));
        columns.push_back(interpolate(data));
    }
    const auto v = columns.front();
    TColgp_Array2OfPnt poles(1, u->NbPoles(), 1, v->NbPoles());
    for (int i = 1; i <= u->NbPoles(); ++i)
        for (int j = 1; j <= v->NbPoles(); ++j)
            poles.SetValue(i, j, columns[i - 1]->Pole(j));
    return new Geom_BSplineSurface(poles, u->Knots(), v->Knots(), u->Multiplicities(),
                                   v->Multiplicities(), u->Degree(), v->Degree());
}
}  // namespace

// Screen supports before making a candidate copy of the entire Body.
bool has_ruled_wall_ring(const TopoDS_Shape& stock, const std::vector<TopoDS_Shape>& openings) {
    if (openings.size() != 1 ||
        BRepAdaptor_Surface(TopoDS::Face(openings.front())).GetType() != GeomAbs_Plane)
        return false;
    int caps = 0, walls = 0;
    for (TopExp_Explorer it(stock, TopAbs_FACE); it.More(); it.Next()) {
        BRepAdaptor_Surface surface(TopoDS::Face(it.Current()));
        if (surface.GetType() == GeomAbs_Plane) {
            if (++caps > 2)
                return false;
        } else if (surface.GetType() == GeomAbs_BSplineSurface) {
            if ((surface.UDegree() != 1 && surface.VDegree() != 1) || ++walls > 32)
                return false;
        } else
            return false;
    }
    return caps == 2 && walls >= 2;
}

RuledShell inward_ruled_shell(const TopoDS_Shape& stock, const std::vector<TopoDS_Shape>& selected,
                              double thickness, double tolerance,
                              const std::function<void()>& check) {
    require(selected.size() == 1, "SHELL_RULED_REQUIRES_ONE_OPENING");
    const auto lid = TopoDS::Face(selected.front());
    BRepAdaptor_Surface top(lid);
    require(top.GetType() == GeomAbs_Plane, "SHELL_RULED_REQUIRES_PLANAR_CAPS");
    TopoDS_Face bottom;
    std::vector<Wall> unordered;
    for (TopExp_Explorer faces(stock, TopAbs_FACE); faces.More(); faces.Next()) {
        auto f = TopoDS::Face(faces.Current());
        if (f.IsSame(lid))
            continue;
        BRepAdaptor_Surface surface(f);
        if (surface.GetType() == GeomAbs_Plane &&
            surface.Plane().Axis().IsParallel(top.Plane().Axis(), 1e-9)) {
            require(bottom.IsNull(), "SHELL_RULED_REQUIRES_PLANAR_CAPS");
            bottom = f;
            continue;
        }
        require(surface.GetType() == GeomAbs_BSplineSurface, "SHELL_RULED_SUPPORT_REQUIRED");
        auto spline = surface.BSpline();
        require(spline->UDegree() == 1 || spline->VDegree() == 1, "SHELL_RULED_SUPPORT_REQUIRED");
        Wall w;
        w.face = f;
        w.surface = BRep_Tool::Surface(f);
        BRepTools::UVBounds(f, w.u0, w.u1, w.v0, w.v1);
        if (spline->VDegree() != 1) {
            w.swap = true;
            std::swap(w.u0, w.v0);
            std::swap(w.u1, w.v1);
        }
        unordered.push_back(w);
    }
    require(!bottom.IsNull() && unordered.size() >= 2 && unordered.size() <= 32,
            "SHELL_RULED_REQUIRES_PLANAR_CAPS");
    Stations stations;
    stations.origin = BRepAdaptor_Surface(bottom).Plane().Location();
    stations.axis = gp_Vec(top.Plane().Axis().Direction());
    stations.height = gp_Vec(stations.origin, top.Plane().Location()).Dot(stations.axis);
    if (stations.height < 0) {
        stations.height = -stations.height;
        stations.axis.Reverse();
    }
    stations.thickness = thickness;
    stations.tolerance = tolerance;
    require(stations.height > thickness + tolerance, "SHELL_THICKNESS_EXCEEDS_INTERIOR");
    for (auto& w : unordered) {
        if (std::abs(gp_Vec(stations.origin, w.evaluate(.5, 0, 0)).Dot(stations.axis)) > tolerance)
            std::swap(w.v0, w.v1);
        for (double s : {0., .5, 1.})
            for (double v : {0., 1.})
                require(std::abs(gp_Vec(stations.origin, w.evaluate(s, v, 0)).Dot(stations.axis) -
                                 stations.height * v) < tolerance,
                        "SHELL_RULED_NONPLANAR_STATION");
    }
    // Recover boundary order from shared source edges. Geometric parameters
    // orient each chart only after its exact topological neighbour is known.
    auto share = [](const auto& l, const auto& r) {
        TopTools_IndexedMapOfShape e;
        TopExp::MapShapes(l, TopAbs_EDGE, e);
        for (TopExp_Explorer it(r, TopAbs_EDGE); it.More(); it.Next())
            if (e.Contains(it.Current()))
                return true;
        return false;
    };
    stations.walls.push_back(unordered.back());
    unordered.pop_back();
    while (!unordered.empty()) {
        auto& prior = stations.walls.back();
        bool found = false;
        for (auto it = unordered.begin(); it != unordered.end(); ++it) {
            if (!share(prior.face, it->face))
                continue;
            auto p = prior.evaluate(1, .5, 0);
            if (p.Distance(it->evaluate(0, .5, 0)) > tolerance) {
                if (p.Distance(it->evaluate(1, .5, 0)) > tolerance)
                    continue;
                it->reverse = true;
            }
            stations.walls.push_back(*it);
            unordered.erase(it);
            found = true;
            break;
        }
        require(found, "SHELL_RULED_OPEN_BOUNDARY");
    }
    require(share(stations.walls.back().face, stations.walls.front().face),
            "SHELL_RULED_OPEN_BOUNDARY");
    const auto count = stations.walls.size();
    std::vector<Handle(Geom_BSplineSurface)> surfaces;
    // Validate interior and boundary interpolation against normal-offset
    // stations; refine both parametric directions under the model budget.
    for (int n : {8, 16, 32, 64}) {
        std::vector<std::vector<std::vector<gp_Pnt>>> samples(count);
        for (int j = 0; j <= n; ++j) {
            check();
            auto z = thickness + (stations.height - thickness) * j / n;
            auto tr = stations.trims(z);
            for (size_t i = 0; i < count; ++i) {
                samples[i].emplace_back();
                for (int k = 0; k <= n; ++k)
                    samples[i].back().push_back(stations.sample(i, tr[i], double(k) / n, z));
            }
        }
        surfaces.clear();
        for (auto& rows : samples)
            surfaces.push_back(tensor(rows));
        double error = 0;
        for (int j = 0; j <= 2 * n && error < tolerance * .6; ++j) {
            check();
            auto v = double(j) / (2 * n), z = thickness + (stations.height - thickness) * v;
            auto tr = stations.trims(z);
            for (size_t i = 0; i < count; ++i)
                for (int k = 0; k <= 2 * n; ++k) {
                    double u = double(k) / (2 * n);
                    error = std::max(
                        error, surfaces[i]->Value(u, v).Distance(stations.sample(i, tr[i], u, z)));
                }
        }
        if (error < tolerance * .6)
            break;
        require(n < 64, "SHELL_OFFSET_SURFACE_PRECISION");
    }
    // Fewer knots reduce the cost of downstream B-Rep intersections. Bound
    // cumulative deviation independently of how many knots can be removed.
    for (auto& surface : surfaces) {
        double budget = tolerance * .04 / (surface->NbUKnots() + surface->NbVKnots());
        for (int j = surface->NbUKnots() - 1; j > 1; --j)
            surface->RemoveUKnot(j, 0, budget);
        for (int j = surface->NbVKnots() - 1; j > 1; --j)
            surface->RemoveVKnot(j, 0, budget);
        check();
    }
    BRepBuilderAPI_Sewing sewing(tolerance * .25);
    std::vector<TopoDS_Face> inner;
    BRepBuilderAPI_MakeWire topWire, baseWire;
    for (size_t i = 0; i < count; ++i) {
        const auto f = BRepBuilderAPI_MakeFace(surfaces[i], 1e-8).Face();
        inner.push_back(f);
        sewing.Add(f);
        // Use exact iso curves from the fitted patches for both planar trims.
        topWire.Add(BRepBuilderAPI_MakeEdge(surfaces[i]->VIso(1)).Edge());
        baseWire.Add(BRepBuilderAPI_MakeEdge(surfaces[i]->VIso(0)).Edge());
    }
    auto base = BRepAdaptor_Surface(bottom).Plane();
    base.Translate(stations.axis * thickness);
    auto floor = BRepBuilderAPI_MakeFace(base, baseWire.Wire(), true).Face();
    sewing.Add(floor);
    auto aperture = BRepBuilderAPI_MakeFace(top.Plane(), topWire.Wire(), true).Face();
    BRepBuilderAPI_MakeFace ring(TopoDS::Face(lid.Oriented(TopAbs_FORWARD)));
    ring.Add(TopoDS::Wire(BRepTools::OuterWire(aperture).Reversed()));
    auto rim = ring.Face();
    sewing.Add(rim);
    for (TopExp_Explorer it(stock, TopAbs_FACE); it.More(); it.Next())
        if (!it.Current().IsSame(lid))
            sewing.Add(it.Current());
    sewing.Perform();
    require(sewing.NbFreeEdges() == 0 && sewing.SewedShape().ShapeType() == TopAbs_SHELL,
            "SHELL_RULED_SEWING_FAILED");
    auto solid = BRepBuilderAPI_MakeSolid(TopoDS::Shell(sewing.SewedShape())).Solid();
    require(BRepLib::OrientClosedSolid(solid), "SHELL_RULED_ORIENTATION_FAILED");
    auto image = [&](const TopoDS_Shape& f) {
        return sewing.IsModifiedSubShape(f) ? sewing.ModifiedSubShape(f) : f;
    };
    Handle(BRepTools_History) history = new BRepTools_History();
    TopTools_IndexedMapOfShape sources;
    TopExp::MapShapes(stock, sources);
    for (int j = 1; j <= sources.Extent(); ++j) {
        const auto& source = sources(j);
        if (!BRepTools_History::IsSupportedType(source))
            continue;
        auto result = image(source);
        if (!source.IsSame(result))
            history->AddModified(source, result);
    }
    history->AddModified(lid, image(rim));
    history->AddGenerated(bottom, image(floor));
    for (size_t i = 0; i < count; ++i)
        history->AddGenerated(stations.walls[i].face, image(inner[i]));
    return {solid, history};
}
}  // namespace occccad::kernel::detail
