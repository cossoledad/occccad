#include <BRep_Tool.hxx>
#include <Precision.hxx>
#include <Standard_Failure.hxx>
#include <gp_Pln.hxx>

#include <internal/edge_profile.hpp>

#include <algorithm>
#include <array>
#include <cmath>
#include <optional>

namespace occccad::kernel::detail {
namespace {
constexpr double pi = 3.14159265358979323846;
double dot(const Vec3& a, const Vec3& b) {
    return a.x * b.x + a.y * b.y + a.z * b.z;
}
bool valid(const Vec3& n) {
    return std::isfinite(dot(n, n)) && dot(n, n) > .5;
}
Vec3 normal(const VisualEdgeUse& use, const gp_Pnt2d& uv, uint64_t* evaluations = nullptr) {
    if (!use.surface)
        return {};
    if (evaluations)
        ++*evaluations;
    try {
        gp_Pnt p;
        gp_Vec du, dv;
        use.surface->D1(uv.X(), uv.Y(), p, du, dv);
        gp_Vec n = du.Crossed(dv);
        if (!std::isfinite(n.SquareMagnitude()) || n.SquareMagnitude() <= 1e-28)
            return {};
        n.Normalize();
        if (use.face.Orientation() == TopAbs_REVERSED)
            n.Reverse();
        return {n.X(), n.Y(), n.Z()};
    } catch (const Standard_Failure&) {
        return {};
    }
}
Vec3 at(const VisualEdgeUse& use, size_t segment, double f, uint64_t& evaluations) {
    if (valid(use.plane_normal))
        return use.plane_normal;
    if (use.reversed) {
        segment = use.points.size() - 2 - segment;
        f = 1 - f;
    }
    if (use.uv.size() != use.points.size())
        return {};
    auto evaluate = [&](double x) -> Vec3 {
        if (x <= 0)
            return normal(use, use.uv[segment], &evaluations);
        if (x >= 1)
            return normal(use, use.uv[segment + 1], &evaluations);
        if (use.parameters.size() != use.points.size() || use.pcurve.IsNull())
            return {};
        try {
            return normal(use,
                          use.pcurve->Value(use.parameters[segment] * (1 - x) +
                                            use.parameters[segment + 1] * x),
                          &evaluations);
        } catch (const Standard_Failure&) {
            return {};
        }
    };
    auto n = evaluate(f);
    // A one-sided limit at a parametric pole, not an invented global normal.
    if (!valid(n) && (f == 0 || f == 1))
        n = evaluate(f == 0 ? 1e-6 : 1 - 1e-6);
    return n;
}
double sharpness(const Vec3& a, const Vec3& b) {
    if (!valid(a) || !valid(b))
        return 1;
    const auto t = std::clamp(
        (dot(a, b) - std::cos(35 * pi / 180)) / (std::cos(5 * pi / 180) - std::cos(35 * pi / 180)),
        0.0, 1.0);
    return 1 - t * t * (3 - 2 * t);
}
}  // namespace
VisualEdgeUse visual_edge_use(uint32_t face_id, const TopoDS_Face& face, const TopoDS_Edge& edge,
                              const Handle(Poly_Triangulation) & triangulation,
                              const Handle(Poly_PolygonOnTriangulation) & polygon,
                              const std::vector<Vec3>& vertices, uint32_t base) {
    VisualEdgeUse use;
    use.face_id = face_id;
    use.face = face;
    use.edge = edge;
    use.surface = std::make_shared<BRepAdaptor_Surface>(face);
    double first = 0, last = 0;
    use.pcurve = BRep_Tool::CurveOnSurface(edge, face, first, last);
    for (int i = 1; i <= polygon->NbNodes(); ++i) {
        const auto node = polygon->Node(i);
        use.points.push_back(vertices[base + node - 1]);
        if (triangulation->HasUVNodes())
            use.uv.push_back(triangulation->UVNode(node));
        if (polygon->HasParameters())
            use.parameters.push_back(polygon->Parameter(i));
    }
    if (use.surface->GetType() == GeomAbs_Plane && !use.uv.empty())
        use.plane_normal = normal(use, use.uv.front());
    return use;
}
void append_edge_profile(TessellationResult& mesh, uint32_t edge_id, const TopoDS_Edge& edge,
                         const std::vector<Vec3>& canonical, const std::vector<VisualEdgeUse>& uses,
                         const std::function<void()>& checkpoint) {
    EdgeDisplayKind kind = uses.empty()       ? EdgeDisplayKind::wire
                           : uses.size() == 1 ? EdgeDisplayKind::boundary
                           : uses.size() > 2  ? EdgeDisplayKind::non_manifold
                                              : EdgeDisplayKind::manifold;
    if (uses.size() == 2) {
        if (uses[0].face_id == uses[1].face_id && BRep_Tool::IsClosed(uses[0].edge, uses[0].face))
            kind = EdgeDisplayKind::seam;
        else if (uses[0].surface && uses[1].surface && valid(uses[0].plane_normal) &&
                 valid(uses[1].plane_normal) &&
                 dot(uses[0].plane_normal, uses[1].plane_normal) > 1 - 1e-10 &&
                 uses[0].surface->Plane().Distance(uses[1].surface->Plane().Location()) <=
                     std::max(Precision::Confusion(), BRep_Tool::Tolerance(edge)))
            kind = EdgeDisplayKind::coplanar;
    }
    constexpr int max_depth = 5, grid = 1 << (max_depth + 2);
    constexpr double normal_cos = 0.9961946980917455;  // 5 degrees
    struct Sample {
        EdgeDisplayPoint point;
        bool missing = false;
        double weight = 1;
    };
    for (size_t segment = 0; segment + 1 < canonical.size(); ++segment) {
        if (checkpoint)
            checkpoint();
        const auto& p = canonical[segment];
        const auto& q = canonical[segment + 1];
        if (std::hypot(q.x - p.x, q.y - p.y, q.z - p.z) <= 1e-12)
            continue;
        std::array<std::optional<Sample>, grid + 1> cache;
        auto sample = [&](int index) -> const Sample& {
            auto& value = cache[index];
            if (value)
                return *value;
            if (checkpoint && (mesh.timings.edge_normal_evaluations % 256) == 0)
                checkpoint();
            const double f = double(index) / grid;
            Sample s;
            s.point.point = {p.x + (q.x - p.x) * f, p.y + (q.y - p.y) * f, p.z + (q.z - p.z) * f};
            if (uses.size() <= 2) {
                if (!uses.empty())
                    s.point.normal_a =
                        at(uses[0], segment, f, mesh.timings.edge_normal_evaluations);
                if (uses.size() == 2)
                    s.point.normal_b =
                        at(uses[1], segment, f, mesh.timings.edge_normal_evaluations);
            }
            s.missing = uses.size() == 2 && (!valid(s.point.normal_a) || !valid(s.point.normal_b));
            s.weight = sharpness(s.point.normal_a, s.point.normal_b);
            value = s;
            return *value;
        };
        auto emit = [&](const Sample& a, const Sample& b, bool missing, bool limited) {
            EdgeDisplaySegment out;
            out.local_id = edge_id;
            out.kind = kind;
            out.quality = (missing ? 1U : 0U) | (limited ? 2U : 0U);
            out.face_a = uses.empty() ? 0 : uses[0].face_id;
            out.face_b = uses.size() < 2 ? 0 : uses[1].face_id;
            out.first = a.point;
            out.second = b.point;
            mesh.edge_profile.push_back(out);
            if (limited)
                ++mesh.timings.edge_sampling_limits;
        };
        auto refine = [&](auto&& self, int lo, int hi, int depth) -> void {
            const auto& a = sample(lo);
            const auto& b = sample(hi);
            bool split = false, missing = a.missing || b.missing;
            for (const int index : {(3 * lo + hi) / 4, (lo + hi) / 2, (lo + 3 * hi) / 4}) {
                const auto& m = sample(index);
                missing |= m.missing;
                const double f = double(index - lo) / (hi - lo);
                split |= std::abs(m.weight - (a.weight * (1 - f) + b.weight * f)) > .02;
                for (const auto pair : {std::pair{a.point.normal_a, m.point.normal_a},
                                        std::pair{m.point.normal_a, b.point.normal_a},
                                        std::pair{a.point.normal_b, m.point.normal_b},
                                        std::pair{m.point.normal_b, b.point.normal_b}})
                    if (valid(pair.first) && valid(pair.second))
                        split |= dot(pair.first, pair.second) < normal_cos;
            }
            if (split && depth < max_depth) {
                self(self, lo, (lo + hi) / 2, depth + 1);
                self(self, (lo + hi) / 2, hi, depth + 1);
            } else
                emit(a, b, missing, split);
        };
        if (uses.size() != 2 || kind == EdgeDisplayKind::coplanar)
            emit(sample(0), sample(grid), false, false);
        else
            refine(refine, 0, grid, 0);
    }
}
}  // namespace occccad::kernel::detail
