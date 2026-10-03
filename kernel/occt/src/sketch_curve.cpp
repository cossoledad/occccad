#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepClass_FaceClassifier.hxx>
#include <BRepGProp.hxx>
#include <BRepTopAdaptor_FClass2d.hxx>
#include <GProp_GProps.hxx>
#include <Geom2dAPI_InterCurveCurve.hxx>
#include <Geom2dAPI_Interpolate.hxx>
#include <Geom2d_BSplineCurve.hxx>
#include <Geom2d_Circle.hxx>
#include <Geom2d_Ellipse.hxx>
#include <Geom2d_Line.hxx>
#include <Geom2d_TrimmedCurve.hxx>
#include <GeomAPI.hxx>
#include <IntRes2d_IntersectionSegment.hxx>
#include <TColStd_Array1OfInteger.hxx>
#include <TColStd_Array1OfReal.hxx>
#include <TColgp_HArray1OfPnt2d.hxx>
#include <TopoDS_Face.hxx>
#include <TopoDS_Wire.hxx>
#include <gp_Ax2d.hxx>
#include <gp_Pln.hxx>

#include <occccad/kernel/kernel.hpp>

#include <algorithm>
#include <cmath>
#include <stdexcept>

namespace occccad::kernel {
namespace {
constexpr double pi = 3.14159265358979323846;
void finite_point(const Vec2& p) {
    if (!std::isfinite(p.x) || !std::isfinite(p.y))
        throw std::invalid_argument("nonfinite curve point");
}
Handle(Geom2d_BSplineCurve) spline_curve(const ProfileCurveSpec& s) {
    if (!s.mode.empty() && s.mode != "FIT" && s.mode != "CONTROL")
        throw std::invalid_argument("spline mode must be FIT or CONTROL");
    if (s.poles.empty()) {
        if (s.mode == "CONTROL")
            throw std::invalid_argument("CONTROL spline requires canonical poles/knots");
        if (s.control_points.size() < 3U || s.control_points.size() > 4096U)
            throw std::invalid_argument("FIT spline requires 3..4096 fit points");
        Handle(TColgp_HArray1OfPnt2d) points =
            new TColgp_HArray1OfPnt2d(1, static_cast<int>(s.control_points.size()));
        for (std::size_t i = 0; i < s.control_points.size(); ++i) {
            finite_point(s.control_points[i]);
            points->SetValue(static_cast<int>(i + 1U),
                             gp_Pnt2d(s.control_points[i].x, s.control_points[i].y));
        }
        Geom2dAPI_Interpolate fit(points, s.closed, 1e-7);
        fit.Perform();
        if (!fit.IsDone())
            throw std::runtime_error("FIT spline interpolation failed");
        return fit.Curve();
    }
    if (s.poles.size() < 2U || s.poles.size() > 4096U || s.knots.size() < 2U ||
        s.knots.size() > 4096U || s.knots.size() != s.multiplicities.size() ||
        (!s.weights.empty() && s.weights.size() != s.poles.size()) || s.degree < 1U ||
        s.degree > 25U)
        throw std::invalid_argument("invalid canonical spline array sizes/degree");
    TColgp_Array1OfPnt2d poles(1, static_cast<int>(s.poles.size()));
    TColStd_Array1OfReal weights(1, poles.Length()), knots(1, static_cast<int>(s.knots.size()));
    TColStd_Array1OfInteger multiplicities(1, knots.Length());
    for (std::size_t i = 0; i < s.poles.size(); ++i) {
        finite_point(s.poles[i]);
        poles.SetValue(static_cast<int>(i + 1U), gp_Pnt2d(s.poles[i].x, s.poles[i].y));
        const double w = s.weights.empty() ? 1.0 : s.weights[i];
        if (!std::isfinite(w) || w <= 0.0)
            throw std::invalid_argument("spline weights must be finite positive");
        weights.SetValue(static_cast<int>(i + 1U), w);
    }
    for (std::size_t i = 0; i < s.knots.size(); ++i) {
        if (!std::isfinite(s.knots[i]) || (i != 0U && s.knots[i] <= s.knots[i - 1U]) ||
            s.multiplicities[i] == 0U || s.multiplicities[i] > s.degree + 1U)
            throw std::invalid_argument("invalid spline knots/multiplicities");
        knots.SetValue(static_cast<int>(i + 1U), s.knots[i]);
        multiplicities.SetValue(static_cast<int>(i + 1U), static_cast<int>(s.multiplicities[i]));
    }
    return new Geom2d_BSplineCurve(poles, weights, knots, multiplicities,
                                   static_cast<int>(s.degree), s.periodic);
}
ProfileCurveSpec extract_spline(ProfileCurveSpec result,
                                const Handle(Geom2d_BSplineCurve) & input) {
    auto curve = Handle(Geom2d_BSplineCurve)::DownCast(input->Copy());
    const double first = curve->FirstParameter(), last = curve->LastParameter();
    if (curve->IsPeriodic())
        curve->SetNotPeriodic();
    // Exact knot insertion at domain endpoints; yields canonical clamped data.
    curve->Segment(first, last);
    result.degree = static_cast<uint32_t>(curve->Degree());
    result.periodic = curve->IsPeriodic();
    result.closed = curve->IsClosed();
    result.poles.clear();
    result.knots.clear();
    result.multiplicities.clear();
    result.weights.clear();
    for (int i = 1; i <= curve->NbPoles(); ++i) {
        const auto p = curve->Pole(i);
        result.poles.push_back({p.X(), p.Y()});
        result.weights.push_back(curve->Weight(i));
    }
    for (int i = 1; i <= curve->NbKnots(); ++i) {
        result.knots.push_back(curve->Knot(i));
        result.multiplicities.push_back(static_cast<uint32_t>(curve->Multiplicity(i)));
    }
    result.parameter_start = curve->FirstParameter();
    result.parameter_end = curve->LastParameter();
    const auto start = curve->Value(result.parameter_start),
               end = curve->Value(result.parameter_end);
    result.start = {start.X(), start.Y()};
    result.end = {end.X(), end.Y()};
    return result;
}
struct ExactCurve {
    Handle(Geom2d_Curve) curve;
    double start{}, end{};
};
ExactCurve exact_curve(const ProfileCurveSpec& s) {
    if (s.kind == "LINE") {
        finite_point(s.start);
        finite_point(s.end);
        const double length = std::hypot(s.end.x - s.start.x, s.end.y - s.start.y);
        if (!(length > 0.0))
            throw std::invalid_argument("degenerate line");
        return {new Geom2d_Line(gp_Pnt2d(s.start.x, s.start.y),
                                gp_Dir2d(s.end.x - s.start.x, s.end.y - s.start.y)),
                0.0, length};
    }
    if (s.kind == "SPLINE") {
        auto spline = spline_curve(s);
        const double lo = s.parameter_start, hi = s.parameter_end;
        if (lo != hi && (!std::isfinite(lo) || !std::isfinite(hi) ||
                         std::min(lo, hi) < spline->FirstParameter() ||
                         std::max(lo, hi) > spline->LastParameter()))
            throw std::invalid_argument("spline domain outside canonical curve");
        return {spline, lo == hi ? spline->FirstParameter() : lo,
                lo == hi ? spline->LastParameter() : hi};
    }
    finite_point(s.center);
    const gp_Pnt2d center(s.center.x, s.center.y);
    if (s.kind == "CIRCLE" || s.kind == "ARC") {
        if (!std::isfinite(s.radius) || s.radius <= 0.0)
            throw std::invalid_argument("invalid circle radius");
        return {new Geom2d_Circle(gp_Ax2d(center, gp_Dir2d(1, 0)), s.radius),
                s.kind == "CIRCLE" ? 0.0 : s.start_angle,
                s.kind == "CIRCLE" ? 2 * pi : s.end_angle};
    }
    if (s.kind == "ELLIPSE" || s.kind == "ELLIPTICAL_ARC") {
        if (!std::isfinite(s.major_radius) || !std::isfinite(s.minor_radius) ||
            !std::isfinite(s.rotation) || s.major_radius <= s.minor_radius || s.minor_radius <= 0.0)
            throw std::invalid_argument("invalid ellipse radii/rotation");
        return {new Geom2d_Ellipse(
                    gp_Ax2d(center, gp_Dir2d(std::cos(s.rotation), std::sin(s.rotation))),
                    s.major_radius, s.minor_radius),
                s.kind == "ELLIPSE" ? 0.0 : s.start_angle,
                s.kind == "ELLIPSE" ? 2 * pi : s.end_angle};
    }
    throw std::invalid_argument("unsupported exact curve: " + s.kind);
}
double unwrap_parameter(const ExactCurve& curve, double value) {
    if (!curve.curve->IsPeriodic())
        return value;
    const double period = curve.curve->Period();
    const double lo = std::min(curve.start, curve.end), hi = std::max(curve.start, curve.end);
    value += std::round(((lo + hi) * 0.5 - value) / period) * period;
    if (value - period >= lo && value - period <= hi)
        value -= period;
    return value;
}
Handle(Geom2d_Curve) bounded(const ExactCurve& s) {
    if (!std::isfinite(s.start) || !std::isfinite(s.end) || s.start == s.end)
        throw std::invalid_argument("invalid curve parameter interval");
    return new Geom2d_TrimmedCurve(s.curve, std::min(s.start, s.end), std::max(s.start, s.end));
}
}  // namespace
ProfileCurveSpec canonicalize_sketch_curve(const ProfileCurveSpec& source) {
    if (source.kind != "SPLINE") {
        (void)exact_curve(source);
        return source;
    }
    auto request = source;
    if ((request.mode.empty() || request.mode == "FIT") && !request.control_points.empty())
        request.poles.clear();
    auto spline = spline_curve(request);
    if (request.mode == "CONTROL" && request.parameter_start != request.parameter_end) {
        const auto domain = exact_curve(request);
        spline->Segment(std::min(domain.start, domain.end), std::max(domain.start, domain.end));
        if (domain.end < domain.start)
            spline->Reverse();
    }
    return extract_spline(source, spline);
}
SketchCurveEvaluation evaluate_sketch_curve(const ProfileCurveSpec& curve, double parameter) {
    if (!std::isfinite(parameter))
        throw std::invalid_argument("nonfinite curve parameter");
    const auto c = exact_curve(curve);
    gp_Pnt2d p;
    gp_Vec2d derivative;
    c.curve->D1(parameter, p, derivative);
    return {{p.X(), p.Y()}, {derivative.X(), derivative.Y()}};
}
SketchCurveIntersections intersect_sketch_curves(const ProfileCurveSpec& first,
                                                 const ProfileCurveSpec& second, double tolerance) {
    if (!std::isfinite(tolerance) || tolerance <= 0.0)
        throw std::invalid_argument("invalid intersection tolerance");
    const auto a = exact_curve(first), b = exact_curve(second);
    Geom2dAPI_InterCurveCurve operation(bounded(a), bounded(b), tolerance);
    if (!operation.Intersector().IsDone())
        throw std::runtime_error("exact curve intersection did not converge");
    SketchCurveIntersections result;
    for (int i = 1; i <= operation.NbPoints(); ++i) {
        const auto& intersection = operation.Intersector().Point(i);
        const auto p = intersection.Value();
        const auto da = evaluate_sketch_curve(first, intersection.ParamOnFirst()).first_derivative;
        const auto db =
            evaluate_sketch_curve(second, intersection.ParamOnSecond()).first_derivative;
        const double determinant = da.x * db.y - da.y * db.x;
        const double norm = std::hypot(da.x, da.y) * std::hypot(db.x, db.y);
        if (!std::isfinite(norm) || norm == 0.0)
            throw std::invalid_argument("intersection has singular curve derivative");
        result.points.push_back({std::abs(determinant) <= 1e-10 * norm
                                     ? SketchCurveIntersectionKind::tangent
                                     : SketchCurveIntersectionKind::point,
                                 {p.X(), p.Y()},
                                 unwrap_parameter(a, intersection.ParamOnFirst()),
                                 unwrap_parameter(b, intersection.ParamOnSecond())});
    }
    // Periodic same-support native segments may report a ±period parameter
    // jump for a shared endpoint. Intersect actual bounded domains instead of
    // interpreting that encoding as a full-period overlap.
    const auto circular=[](const ProfileCurveSpec& c){return c.kind=="CIRCLE"||c.kind=="ARC";};
    const auto elliptical=[](const ProfileCurveSpec& c){return c.kind=="ELLIPSE"||c.kind=="ELLIPTICAL_ARC";};
    const bool same_center=first.center.x==second.center.x && first.center.y==second.center.y;
    const bool exact_support=same_center &&
        ((circular(first)&&circular(second)&&first.radius==second.radius) ||
         (elliptical(first)&&elliptical(second)&&first.major_radius==second.major_radius&&
          first.minor_radius==second.minor_radius&&std::remainder(second.rotation-first.rotation,pi)==0));
    if((exact_support || operation.NbSegments()>0) && ((circular(first)&&circular(second)) || (elliptical(first)&&elliptical(second)))) {
        result.points.clear(); // Native contact encodings are replaced by the domain intersection.

        const double phase=elliptical(first)?std::remainder(second.rotation,2*pi)-std::remainder(first.rotation,2*pi):0;
        const double alo=std::min(a.start,a.end),ahi=std::max(a.start,a.end),
                     blo=std::min(b.start,b.end),bhi=std::max(b.start,b.end);
        const double period=a.curve->Period();
        if(!std::isfinite(ahi-alo) || !std::isfinite(bhi-blo) || ahi-alo>period || bhi-blo>period)
            throw std::invalid_argument("periodic intersection domain exceeds one period");
        const auto base=static_cast<long long>(std::llround(((alo+ahi)-(blo+bhi))*0.5/period-phase/period));
        for(long long offset=base-1;offset<=base+1;++offset) {
            const double shift=phase+static_cast<double>(offset)*period;
            const double lo=std::max(alo,blo+shift),hi=std::min(ahi,bhi+shift);
            const auto da=evaluate_sketch_curve(first,lo).first_derivative;
            const auto db=evaluate_sketch_curve(second,lo-shift).first_derivative;
            const double scale=std::max(std::hypot(da.x,da.y),std::hypot(db.x,db.y));
            if((lo-hi)*scale>tolerance)continue;
            if((hi-lo)*scale<=tolerance) {
                const auto point=evaluate_sketch_curve(first,lo).point;
                result.points.push_back({SketchCurveIntersectionKind::tangent,point,lo,lo-shift});
            } else result.overlaps.push_back({lo,hi,lo-shift,hi-shift});
        }
    } else {
    for (int i = 1; i <= operation.NbSegments(); ++i) {
        const auto& segment = operation.Intersector().Segment(i);
        if (!segment.HasFirstPoint() || !segment.HasLastPoint())
            throw std::runtime_error("unbounded overlap in finite curve request");
        const double af = unwrap_parameter(a, segment.FirstPoint().ParamOnFirst());
        const double bf = unwrap_parameter(b, segment.FirstPoint().ParamOnSecond());
        const double a_span =
            segment.LastPoint().ParamOnFirst() - segment.FirstPoint().ParamOnFirst();
        const double b_span =
            segment.LastPoint().ParamOnSecond() - segment.FirstPoint().ParamOnSecond();
        const auto a_derivative = evaluate_sketch_curve(first, af).first_derivative;
        const auto b_derivative = evaluate_sketch_curve(second, bf).first_derivative;
        // Native coincident-support intersections encode isolated shared ends
        // as zero-length segments. They are contacts, not overlapping intervals.
        if (std::abs(a_span) * std::hypot(a_derivative.x, a_derivative.y) <= tolerance &&
            std::abs(b_span) * std::hypot(b_derivative.x, b_derivative.y) <= tolerance) {
            const auto point = evaluate_sketch_curve(first, af).point;
            result.points.push_back({SketchCurveIntersectionKind::tangent, point, af, bf});
            continue;
        }
        result.overlaps.push_back({af, af + a_span, bf, bf + b_span});
    }
    }
    std::sort(result.points.begin(), result.points.end(), [](const auto& left, const auto& right) {
        return left.first_parameter < right.first_parameter ||
               (left.first_parameter == right.first_parameter &&
                left.second_parameter < right.second_parameter);
    });
    return result;
}
ProfileCurveSpec trim_sketch_curve(const ProfileCurveSpec& source, double start, double end) {
    if (!std::isfinite(start) || !std::isfinite(end) || start == end)
        throw std::invalid_argument("invalid trim interval");
    const auto c = exact_curve(source);
    const bool periodic_support = source.kind == "CIRCLE" || source.kind == "ELLIPSE";
    if ((periodic_support && std::abs(end - start) > 2 * pi) ||
        (!periodic_support && (std::min(start, end) < std::min(c.start, c.end) ||
                               std::max(start, end) > std::max(c.start, c.end))))
        throw std::invalid_argument("trim interval outside source curve");
    auto result = source;
    result.reversed = false;
    if (source.kind == "SPLINE") {
        auto spline = Handle(Geom2d_BSplineCurve)::DownCast(c.curve->Copy());
        spline->Segment(std::min(start, end), std::max(start, end));
        if (end < start)
            spline->Reverse();
        result = extract_spline(result, spline);
        result.mode = "CONTROL";
        result.control_points.clear();
    } else if (source.kind == "LINE") {
        result.start = evaluate_sketch_curve(source, start).point;
        result.end = evaluate_sketch_curve(source, end).point;
    } else {
        if (result.kind == "CIRCLE")
            result.kind = "ARC";
        if (result.kind == "ELLIPSE")
            result.kind = "ELLIPTICAL_ARC";
        result.start_angle = start;
        result.end_angle = end;
    }
    return result;
}

std::vector<ProfileRegionSpec> classify_sketch_profile(const std::vector<ProfileLoopSpec>& input) {
    constexpr double tolerance = 1e-7;
    if (input.size() > 4096)
        throw std::invalid_argument("profile loop resource limit");
    const gp_Pln plane(gp_Pnt(0, 0, 0), gp_Dir(0, 0, 1));
    struct Loop {
        ProfileLoopSpec value;
        TopoDS_Face face;
        double area;
        int parent = -1;
        int depth = 0;
    };
    std::vector<Loop> loops;
    const auto reverse = [](ProfileLoopSpec& loop) {
        std::reverse(loop.curves.begin(), loop.curves.end());
        for (auto& curve : loop.curves)
            curve.reversed = !curve.reversed;
    };
    const auto close = [](const Vec2& a, const Vec2& b) {
        return std::hypot(a.x - b.x, a.y - b.y) <= tolerance;
    };
    const auto endpoints = [](const ProfileCurveSpec& curve) {
        const auto exact = exact_curve(curve);
        auto first = evaluate_sketch_curve(curve, exact.start).point;
        auto last = evaluate_sketch_curve(curve, exact.end).point;
        if (curve.reversed)
            std::swap(first, last);
        return std::make_pair(first, last);
    };
    const auto face_for = [&](const ProfileLoopSpec& loop) {
        BRepBuilderAPI_MakeWire wire;
        for (const auto& spec : loop.curves) {
            const auto exact = exact_curve(spec);
            BRepBuilderAPI_MakeEdge edge(GeomAPI::To3d(exact.curve, plane),
                                         std::min(exact.start, exact.end),
                                         std::max(exact.start, exact.end));
            if (!edge.IsDone())
                throw std::invalid_argument("profile edge construction failed: " + spec.entity_id);
            auto result = edge.Edge();
            if ((exact.end < exact.start) != spec.reversed)
                result.Reverse();
            wire.Add(result);
        }
        if (!wire.IsDone())
            throw std::invalid_argument("profile wire construction failed: " + loop.id);
        // Preserve the supplied winding while classifying. Inside=true silently
        // reverses a clockwise wire, hiding the orientation change from loop.value.
        BRepBuilderAPI_MakeFace face(plane, wire.Wire(), false);
        if (!face.IsDone())
            throw std::invalid_argument("profile face construction failed: " + loop.id);
        return face.Face();
    };
    std::size_t total = 0;
    for (const auto& source : input) {
        if (source.id.empty() || source.curves.empty() || (total += source.curves.size()) > 4096)
            throw std::invalid_argument("invalid profile identity/curve resource limit");
        for (const auto& existing : loops)
            if (existing.value.id == source.id)
                throw std::invalid_argument("duplicate profile loop identity");
        for (std::size_t i = 0; i < source.curves.size(); ++i) {
            const auto& curve = source.curves[i];
            if (curve.kind == "SPLINE" && curve.poles.empty())
                throw std::invalid_argument("profile spline requires canonical exact data");
            const auto ends = endpoints(curve),
                       next = endpoints(source.curves[(i + 1) % source.curves.size()]);
            if (!close(ends.second, next.first))
                throw std::invalid_argument("profile has geometrically disconnected endpoints: " +
                                            source.id);
            const auto exact = exact_curve(curve);
            Geom2dAPI_InterCurveCurve self(bounded(exact), tolerance);
            if (!self.Intersector().IsDone())
                throw std::runtime_error("profile self-intersection evaluation failed");
            if (self.NbPoints() != 0 || self.NbSegments() != 0)
                throw std::invalid_argument("profile curve self-intersects: " + curve.entity_id);
            for (std::size_t j = 0; j < i; ++j) {
                const auto hits = intersect_sketch_curves(source.curves[j], curve, tolerance);
                if (!hits.overlaps.empty())
                    throw std::invalid_argument("profile contains overlapping edges: " + source.id);
                for (const auto& hit : hits.points) {
                    const auto previous = endpoints(source.curves[j]);
                    const bool forward = (j + 1 == i) && close(hit.point, previous.second) &&
                                         close(hit.point, ends.first);
                    const bool seam = (j == 0 && i + 1 == source.curves.size()) &&
                                      close(hit.point, previous.first) &&
                                      close(hit.point, ends.second);
                    if (!forward && !seam)
                        throw std::invalid_argument(
                            "profile has crossing or nonadjacent contact: " + source.id);
                }
            }
        }
        Loop loop;
        loop.value = source;
        loop.face = face_for(loop.value);
        BRepTopAdaptor_FClass2d classifier(loop.face, tolerance);
        if (classifier.PerformInfinitePoint() != TopAbs_OUT) {
            reverse(loop.value);
            loop.face = face_for(loop.value);
        }
        if (!BRepCheck_Analyzer(loop.face).IsValid())
            throw std::invalid_argument("invalid exact profile face: " + source.id);
        GProp_GProps properties;
        BRepGProp::SurfaceProperties(loop.face, properties);
        loop.area = std::abs(properties.Mass());
        if (!std::isfinite(loop.area) || loop.area <= tolerance * tolerance)
            throw std::invalid_argument("degenerate exact profile region: " + source.id);
        loops.push_back(std::move(loop));
    }
    std::sort(loops.begin(), loops.end(), [](const Loop& a, const Loop& b) {
        return a.area > b.area || (a.area == b.area && a.value.id < b.value.id);
    });
    for (std::size_t i = 0; i < loops.size(); ++i) {
        for (std::size_t j = 0; j < i; ++j) {
            for (const auto& a : loops[i].value.curves)
                for (const auto& b : loops[j].value.curves) {
                    const auto hits = intersect_sketch_curves(a, b, tolerance);
                    if (!hits.points.empty() || !hits.overlaps.empty())
                        throw std::invalid_argument("profile loops intersect or touch");
                }
            const auto point = endpoints(loops[i].value.curves.front()).first;
            BRepClass_FaceClassifier classifier(loops[j].face, gp_Pnt2d(point.x, point.y),
                                                tolerance);
            if (classifier.State() == TopAbs_IN)
                loops[i].parent = static_cast<int>(j);
            else if (classifier.State() != TopAbs_OUT)
                throw std::invalid_argument("ambiguous exact profile nesting");
        }
        if (loops[i].parent >= 0)
            loops[i].depth = loops[static_cast<std::size_t>(loops[i].parent)].depth + 1;
    }
    std::vector<ProfileRegionSpec> regions;
    std::vector<int> region_for(loops.size(), -1);
    for (std::size_t i = 0; i < loops.size(); ++i)
        if (loops[i].depth % 2 == 0) {
            region_for[i] = static_cast<int>(regions.size());
            regions.push_back({"profile-region:" + loops[i].value.id, loops[i].value, {}});
        }
    for (std::size_t i = 0; i < loops.size(); ++i)
        if (loops[i].depth % 2 != 0) {
            auto hole = loops[i].value;
            reverse(hole);
            regions[static_cast<std::size_t>(region_for[static_cast<std::size_t>(loops[i].parent)])]
                .holes.push_back(std::move(hole));
        }
    return regions;
}
}  // namespace occccad::kernel
