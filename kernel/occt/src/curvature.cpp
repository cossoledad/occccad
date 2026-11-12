#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepBndLib.hxx>
#include <BRepClass_FaceClassifier.hxx>
#include <BRepExtrema_DistShapeShape.hxx>
#include <BRepLProp_SLProps.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GCPnts_UniformAbscissa.hxx>
#include <Standard_Failure.hxx>
#include <TColStd_Array1OfReal.hxx>
#include <TopExp.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>

#include <internal/curvature.hpp>

#include <algorithm>
#include <cmath>
#include <stdexcept>
namespace occccad::kernel::detail {
namespace {
Vec3 vec(const gp_Pnt& p) {
    return {p.X(), p.Y(), p.Z()};
}
Vec3 vec(const gp_Vec& p) {
    return {p.X(), p.Y(), p.Z()};
}
bool finite(double v) {
    return std::isfinite(v);
}
void boundary_quality(const std::vector<TopoDS_Shape>& shapes, MeasurementResult& result,
                      const std::function<bool()>& cancelled) {
    const auto check = [&] {
        if (cancelled && cancelled())
            throw std::runtime_error("MEASUREMENT_CANCELLED");
    };
    BRep_Builder builder;
    TopoDS_Compound compound;
    builder.MakeCompound(compound);
    for (const auto& shape : shapes)
        builder.Add(compound, shape);
    TopTools_IndexedDataMapOfShapeListOfShape adjacent;
    TopExp::MapShapesAndAncestors(compound, TopAbs_EDGE, TopAbs_FACE, adjacent);
    std::vector<TopoDS_Edge> free;
    result.boundary_complete = true;
    for (int i = 1; i <= adjacent.Extent(); ++i) {
        check();
        const auto edge = TopoDS::Edge(adjacent.FindKey(i));
        const auto& faces = adjacent.FindFromIndex(i);
        if (faces.Extent() != 1 || BRep_Tool::Degenerated(edge) ||
            BRep_Tool::IsClosed(edge, TopoDS::Face(faces.First())))
            continue;
        free.push_back(edge);
        if (free.size() > 256) {
            result.boundary_complete = false;
            break;
        }
    }
    if (free.size() > 256)
        free.resize(256);
    for (size_t i = 0; i < free.size(); ++i) {
        check();
        BRepAdaptor_Curve c(free[i]);
        MeasurementResult::BoundaryEdge edge;
        edge.edge_id = i + 1;
        GCPnts_UniformAbscissa samples(c, 17);
        if (!samples.IsDone()) {
            result.boundary_complete = false;
            continue;
        }
        for (int j = 1; j <= samples.NbPoints(); ++j)
            edge.points.push_back(vec(c.Value(samples.Parameter(j))));
        result.boundary_edges.push_back(std::move(edge));
    }
    if (free.size() > 64) {
        result.boundary_complete = false;
        return;
    }
    std::vector<Bnd_Box> boxes(free.size());
    for (size_t i = 0; i < free.size(); ++i) {
        BRepBndLib::Add(free[i], boxes[i]);
        boxes[i].Enlarge(.01);
    }
    for (size_t i = 0; i < free.size(); ++i)
        for (size_t j = i + 1; j < free.size(); ++j) {
            check();
            if (boxes[i].IsOut(boxes[j]))
                continue;
            BRepExtrema_DistShapeShape distance(free[i], free[j]);
            if (!distance.IsDone() || distance.NbSolution() == 0) {
                result.boundary_complete = false;
                continue;
            }
            const double gap = distance.Value();
            if (gap <= 1e-7 || gap > .01)
                continue;
            if (result.gap_candidates.size() >= 256) {
                result.boundary_complete = false;
                return;
            }
            result.gap_candidates.push_back({i + 1, j + 1, vec(distance.PointOnShape1(1)),
                                             vec(distance.PointOnShape2(1)), gap});
        }
}
}  // namespace
MeasurementResult analyze_curvature(const std::vector<TopoDS_Shape>& shapes, uint32_t count,
                                    const std::function<bool()>& cancelled) {
    const auto check = [&] {
        if (cancelled && cancelled())
            throw std::runtime_error("MEASUREMENT_CANCELLED");
    };
    check();
    if (count < 16 || count > 4096)
        throw std::invalid_argument("CURVATURE_SAMPLE_COUNT_INVALID");
    TopTools_IndexedMapOfShape fs, es;
    for (const auto& shape : shapes) {
        if (shape.ShapeType() == TopAbs_EDGE)
            es.Add(shape);
        else {
            TopTools_IndexedMapOfShape faces;
            TopExp::MapShapes(shape, TopAbs_FACE, faces);
            if (faces.IsEmpty())
                TopExp::MapShapes(shape, TopAbs_EDGE, es);
            else
                for (int i = 1; i <= faces.Extent(); ++i)
                    fs.Add(faces(i));
        }
    }
    if (fs.Extent() > 16 || es.Extent() > 64 || fs.Extent() + es.Extent() == 0)
        throw std::invalid_argument("CURVATURE_SCOPE_LIMIT");
    MeasurementResult r;
    r.geometry_type = fs.IsEmpty() ? "CURVE" : es.IsEmpty() ? "SURFACE" : "MIXED";
    const int budget = std::max(4, static_cast<int>(count) / (fs.Extent() + es.Extent()));
    for (int i = 1; i <= es.Extent(); ++i) {
        check();
        BRepAdaptor_Curve c(TopoDS::Edge(es(i)));
        const int spans = c.NbIntervals(GeomAbs_C2);
        if (spans > 128)
            throw std::invalid_argument("CURVATURE_CONTINUITY_SPAN_LIMIT");
        TColStd_Array1OfReal intervals(1, spans + 1);
        c.Intervals(intervals, GeomAbs_C2);
        const int n = std::max(2, budget / spans);
        for (int span = 1; span <= spans; ++span) {
            check();
            const double a = intervals(span), b = intervals(span + 1);
            if (!finite(a) || !finite(b) || b <= a)
                throw std::invalid_argument("CURVATURE_FINITE_DOMAIN_REQUIRED");
            GCPnts_UniformAbscissa sample(c, n + 2, a, b, 1e-7);
            if (!sample.IsDone())
                throw std::invalid_argument("CURVATURE_SAMPLING_FAILED");
            for (int j = 2; j < sample.NbPoints(); ++j) {
                check();
                try {
                    gp_Pnt p;
                    gp_Vec d1, d2;
                    c.D2(sample.Parameter(j), p, d1, d2);
                    const double speed = d1.Magnitude();
                    if (speed < 1e-12) {
                        ++r.undefined_samples;
                        continue;
                    }
                    const double k = d1.Crossed(d2).Magnitude() / (speed * speed * speed);
                    gp_Vec normal = d2 - d1 * (d2.Dot(d1) / (speed * speed));
                    if (normal.Magnitude() > 1e-12)
                        normal.Normalize();
                    else
                        normal = gp_Vec(0, 0, 0);
                    if (!finite(k))
                        throw std::runtime_error("nonfinite curvature");
                    r.curvature.push_back({vec(p), vec(normal), k, k, k, 0, false});
                } catch (const Standard_Failure&) {
                    ++r.undefined_samples;
                }
            }
        }
        // Internal C2 breaks are deliberately excluded from two-sided curvature.
        r.undefined_samples += static_cast<uint32_t>(spans - 1);
    }
    for (int i = 1; i <= fs.Extent(); ++i) {
        check();
        const auto face = TopoDS::Face(fs(i));
        BRepAdaptor_Surface s(face, Standard_True);
        double u0, u1, v0, v1;
        BRepTools::UVBounds(face, u0, u1, v0, v1);
        if (!finite(u0) || !finite(u1) || !finite(v0) || !finite(v1) || u1 <= u0 || v1 <= v0)
            throw std::invalid_argument("CURVATURE_FINITE_DOMAIN_REQUIRED");
        const int grid = std::max(2, static_cast<int>(std::sqrt(budget)));
        BRepLProp_SLProps props(s, 2, 1e-10);
        for (int u = 0; u < grid; ++u)
            for (int v = 0; v < grid; ++v) {
                check();
                const double pu = u0 + (u + .5) * (u1 - u0) / grid,
                             pv = v0 + (v + .5) * (v1 - v0) / grid;
                BRepClass_FaceClassifier inside(face, gp_Pnt2d(pu, pv), 1e-7);
                if (inside.State() != TopAbs_IN && inside.State() != TopAbs_ON)
                    continue;
                try {
                    props.SetParameters(pu, pv);
                    if (!props.IsCurvatureDefined() || !props.IsNormalDefined()) {
                        ++r.undefined_samples;
                        continue;
                    }
                    double a = props.MinCurvature(), b = props.MaxCurvature();
                    gp_Vec n(props.Normal());
                    if (face.Orientation() == TopAbs_REVERSED) {
                        const auto old = a;
                        a = -b;
                        b = -old;
                        n.Reverse();
                    }
                    if (!finite(a) || !finite(b))
                        throw std::runtime_error("CURVATURE_NONFINITE_RESULT");
                    r.curvature.push_back(
                        {vec(s.Value(pu, pv)), vec(n), a, b, (a + b) / 2, a * b, true});
                } catch (const Standard_Failure&) {
                    ++r.undefined_samples;
                }
            }
    }
    check();
    if (r.curvature.size() > 8192)
        throw std::invalid_argument("CURVATURE_GRAPHICS_LIMIT");
    if (r.curvature.empty())
        throw std::invalid_argument("CURVATURE_NO_REGULAR_SAMPLES");
    r.complete = true;
    boundary_quality(shapes, r, cancelled);
    return r;
}
}  // namespace occccad::kernel::detail
