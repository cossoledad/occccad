#include <BRepBuilderAPI_Copy.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Geom2d_Curve.hxx>
#include <GeomAPI_ProjectPointOnCurve.hxx>
#include <Geom_Curve.hxx>
#include <Geom_Surface.hxx>
#include <Precision.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopTools_MapOfShape.hxx>
#include <TopoDS.hxx>

#include <internal/shell_junction.hpp>

#include <algorithm>
#include <cmath>
#include <iostream>
#include <limits>
#include <stdexcept>
#include <vector>

namespace occccad::kernel {
namespace {
struct CurveEnd {
    TopoDS_Edge edge;
    Handle(Geom_Curve) curve;
    bool first;
    double original, parameter, lower, upper;
};

[[noreturn]] void failed() {
    throw std::runtime_error("SHELL_JUNCTION_PRECISION_EXCEEDED");
}

// An axis-aligned box center can overestimate the required vertex tolerance.
// A finite point set in 3D has a minimum enclosing ball supported by at most
// four points. Enumerate those supports in coordinates relative to one endpoint.
gp_Pnt enclosing_center(const std::vector<gp_Pnt>& points) {
    const auto origin = points.front();
    gp_Pnt best = origin;
    double bestRadius = std::numeric_limits<double>::infinity();
    const auto consider = [&](const gp_XYZ& offset) {
        const gp_Pnt center(origin.XYZ() + offset);
        double radius = 0;
        for (const auto& point : points)
            radius = std::max(radius, center.SquareDistance(point));
        if (radius < bestRadius) {
            bestRadius = radius;
            best = center;
        }
    };
    for (std::size_t i = 0; i < points.size(); ++i) {
        consider(points[i].XYZ() - origin.XYZ());
        for (std::size_t j = 0; j < i; ++j) {
            consider((points[i].XYZ() + points[j].XYZ()) * .5 - origin.XYZ());
            const auto base = points[j].XYZ();
            const auto a = points[i].XYZ() - base;
            for (std::size_t k = 0; k < j; ++k) {
                const auto b = points[k].XYZ() - base;
                const auto normal = a.Crossed(b);
                if (normal.SquareModulus() > 1e-60)
                    consider(base - origin.XYZ() +
                             (b.Crossed(normal) * a.SquareModulus() +
                              normal.Crossed(a) * b.SquareModulus()) /
                                 (2 * normal.SquareModulus()));
                for (std::size_t l = 0; l < k; ++l) {
                    const auto c = points[l].XYZ() - base;
                    const double det = a.Dot(b.Crossed(c));
                    if (std::abs(det) > 1e-45)
                        consider(base - origin.XYZ() +
                                 (b.Crossed(c) * a.SquareModulus() +
                                  c.Crossed(a) * b.SquareModulus() +
                                  a.Crossed(b) * c.SquareModulus()) /
                                     (2 * det));
                }
            }
        }
    }
    return best;
}

gp_Pnt project_ends(std::vector<CurveEnd>& ends, gp_Pnt center, double search, double tolerance) {
    for (auto& end : ends) {
        gp_Pnt point;
        gp_Vec tangent;
        end.curve->D1(end.original, point, tangent);
        const auto interval = search / std::max(tangent.Magnitude(), 1e-12);
        end.lower = std::max(end.curve->FirstParameter(), end.original - interval);
        end.upper = std::min(end.curve->LastParameter(), end.original + interval);
        if (!(end.lower < end.upper))
            failed();
    }
    const auto closest = [](const gp_Pnt& target, CurveEnd& end) {
        // Curve extrema omit a constrained minimum at an interval endpoint.
        auto parameter = end.lower;
        auto point = end.curve->Value(parameter);
        const auto upper = end.curve->Value(end.upper);
        if (target.SquareDistance(upper) < target.SquareDistance(point)) {
            parameter = end.upper;
            point = upper;
        }
        GeomAPI_ProjectPointOnCurve projection(target, end.curve, end.lower, end.upper);
        if (projection.NbPoints() && projection.LowerDistance() < target.Distance(point)) {
            parameter = projection.LowerDistanceParameter();
            point = projection.NearestPoint();
        }
        end.parameter = parameter;
        return point;
    };
    for (int iteration = 0; iteration < 64; ++iteration) {
        std::vector<gp_Pnt> projected;
        for (auto& end : ends) {
            const auto point = closest(center, end);
            if (point.Distance(end.curve->Value(end.original)) > search)
                failed();
            projected.push_back(point);
        }
        const gp_Pnt next = enclosing_center(projected);
        const auto movement = center.Distance(next);
        center = next;
        if (movement < tolerance * .01)
            break;
    }
    double radius = 0;
    for (auto& end : ends) {
        radius = std::max(radius, center.Distance(closest(center, end)));
    }
    if (!std::isfinite(radius) || radius > tolerance) {
        std::clog << "shell stage=junction_projection radius=" << radius << " limit=" << tolerance
                  << '\n';
        failed();
    }
    return center;
}
}  // namespace

ShellJunctionRefinement refine_shell_junctions(const TopoDS_Shape& shape, double tolerance) {
    TopTools_IndexedDataMapOfShapeListOfShape vertexEdges, edgeFaces;
    TopExp::MapShapesAndAncestors(shape, TopAbs_VERTEX, TopAbs_EDGE, vertexEdges);
    TopExp::MapShapesAndAncestors(shape, TopAbs_EDGE, TopAbs_FACE, edgeFaces);
    BRepBuilderAPI_Copy copy(shape, Standard_True, Standard_False);
    bool changed = false;
    BRep_Builder builder;
    for (int i = 1; i <= vertexEdges.Extent(); ++i) {
        const auto vertex = TopoDS::Vertex(vertexEdges.FindKey(i));
        const auto oldPoint = BRep_Tool::Pnt(vertex);
        std::vector<CurveEnd> ends;
        std::vector<gp_Pnt> points;
        TopTools_MapOfShape seen;
        for (TopTools_ListIteratorOfListOfShape it(vertexEdges.FindFromIndex(i)); it.More();
             it.Next()) {
            const auto edge = TopoDS::Edge(it.Value().Oriented(TopAbs_FORWARD));
            if (!seen.Add(edge) || BRep_Tool::Degenerated(edge))
                continue;
            double first, last;
            const auto curve = BRep_Tool::Curve(edge, first, last);
            if (curve.IsNull())
                failed();
            TopoDS_Vertex a, b;
            TopExp::Vertices(edge, a, b);
            for (const bool isFirst : {true, false}) {
                if (!(isFirst ? a : b).IsSame(vertex))
                    continue;
                const double parameter = isFirst ? first : last;
                ends.push_back({edge, curve, isFirst, parameter, parameter, 0, 0});
                points.push_back(curve->Value(parameter));
                for (TopTools_ListIteratorOfListOfShape faces(edgeFaces.FindFromKey(edge));
                     faces.More(); faces.Next()) {
                    const auto face = TopoDS::Face(faces.Value());
                    double lo, hi;
                    const auto pc = BRep_Tool::CurveOnSurface(edge, face, lo, hi);
                    if (pc.IsNull())
                        failed();
                    const auto uv = pc->Value(parameter);
                    points.push_back(BRep_Tool::Surface(face)->Value(uv.X(), uv.Y()));
                }
            }
        }
        double error = 0;
        for (const auto& p : points)
            error = std::max(error, oldPoint.Distance(p));
        if (error <= tolerance)
            continue;
        if (ends.empty())
            failed();
        gp_Pnt center = enclosing_center(points);
        double radius = 0;
        for (const auto& p : points)
            radius = std::max(radius, center.Distance(p));
        if (radius > tolerance) {
            const double search = std::min(tolerance * 64, std::max(tolerance * 4, error * 4));
            center = project_ends(ends, center, search, tolerance);
        }
        auto replacement = TopoDS::Vertex(copy.ModifiedShape(vertex));
        builder.UpdateVertex(replacement, center, tolerance);
        for (const auto& end : ends) {
            auto edge = TopoDS::Edge(copy.ModifiedShape(end.edge).Oriented(TopAbs_FORWARD));
            double first, last;
            BRep_Tool::Range(edge, first, last);
            if (end.first)
                first = end.parameter;
            else
                last = end.parameter;
            if (!(first < last))
                failed();
            builder.Range(edge, first, last);
            builder.UpdateVertex(replacement, end.parameter, edge, tolerance);
            for (TopTools_ListIteratorOfListOfShape faces(edgeFaces.FindFromKey(end.edge));
                 faces.More(); faces.Next())
                builder.UpdateVertex(replacement, end.parameter, edge,
                                     TopoDS::Face(copy.ModifiedShape(faces.Value())), tolerance);
            builder.SameParameter(edge, Standard_True);
            builder.SameRange(edge, Standard_True);
        }
        changed = true;
    }
    TopTools_ListOfShape sources;
    sources.Append(shape);
    return {copy.Shape(), new BRepTools_History(sources, copy), changed};
}

}  // namespace occccad::kernel
