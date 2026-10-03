#include <occccad/kernel/kernel.hpp>

#include <gtest/gtest.h>

#include <cmath>

namespace occccad::kernel {
namespace {
ProfileCurveSpec cubic() {
    ProfileCurveSpec curve;
    curve.kind = "SPLINE";
    curve.mode = "CONTROL";
    curve.degree = 3;
    curve.poles = {{0, 0}, {1, 2}, {2, -1}, {3, 0}};
    curve.knots = {0, 1};
    curve.multiplicities = {4, 4};
    curve.weights = {1, 1, 1, 1};
    curve.parameter_start = 0;
    curve.parameter_end = 1;
    return curve;
}
}  // namespace
TEST(ExactSketchCurve, RationalCanonicalCurveHasAnalyticValueAndDerivative) {
    auto curve = cubic();
    curve.degree = 2;
    curve.poles = {{1, 0}, {1, 1}, {0, 1}};
    curve.weights = {1, std::sqrt(0.5), 1};
    curve.multiplicities = {3, 3};
    const auto middle = evaluate_sketch_curve(curve, 0.5);
    EXPECT_NEAR(middle.point.x, std::sqrt(0.5), 1e-12);
    EXPECT_NEAR(middle.point.y, std::sqrt(0.5), 1e-12);
    EXPECT_NEAR(middle.first_derivative.x, -middle.first_derivative.y, 1e-12);
    const auto start = evaluate_sketch_curve(curve, 0);
    EXPECT_NEAR(start.first_derivative.x, 0, 1e-12);
    EXPECT_NEAR(start.first_derivative.y, std::sqrt(2.0), 1e-12);
}
TEST(ExactSketchCurve, FitPointsRemainFitPointsAndCanonicalRoundTripIsExact) {
    auto curve = cubic();
    curve.mode = "FIT";
    curve.poles.clear();
    curve.control_points = {{0, 0}, {1, 2}, {2, -1}, {3, 0}};
    const auto canonical = canonicalize_sketch_curve(curve);
    ASSERT_FALSE(canonical.poles.empty());
    EXPECT_FALSE(canonical.periodic);
    EXPECT_EQ(canonical.control_points.size(), 4U);
    for (int i = 0; i <= 16; ++i) {
        const double u = canonical.parameter_start +
                         (canonical.parameter_end - canonical.parameter_start) * i / 16.0;
        const auto expected = evaluate_sketch_curve(curve, u),
                   actual = evaluate_sketch_curve(canonical, u);
        EXPECT_NEAR(actual.point.x, expected.point.x, 1e-11);
        EXPECT_NEAR(actual.point.y, expected.point.y, 1e-11);
        EXPECT_NEAR(actual.first_derivative.x, expected.first_derivative.x, 1e-10);
        EXPECT_NEAR(actual.first_derivative.y, expected.first_derivative.y, 1e-10);
    }
}
TEST(ExactSketchCurve, ClosedFitCanonicalizesToEquivalentClampedNonperiodicCurve) {
    ProfileCurveSpec curve;
    curve.kind = "SPLINE";
    curve.mode = "FIT";
    curve.closed = true;
    curve.control_points = {{0, 0}, {2, 0}, {2, 2}, {0, 2}};
    const auto canonical = canonicalize_sketch_curve(curve);
    EXPECT_TRUE(canonical.closed);
    EXPECT_FALSE(canonical.periodic);
    EXPECT_EQ(canonical.multiplicities.front(), canonical.degree + 1);
    EXPECT_EQ(canonical.multiplicities.back(), canonical.degree + 1);
    for (int i = 0; i <= 24; ++i) {
        const double u = canonical.parameter_start +
                         (canonical.parameter_end - canonical.parameter_start) * i / 24.0;
        const auto expected = evaluate_sketch_curve(curve, u),
                   actual = evaluate_sketch_curve(canonical, u);
        EXPECT_NEAR(actual.point.x, expected.point.x, 1e-10);
        EXPECT_NEAR(actual.point.y, expected.point.y, 1e-10);
        EXPECT_NEAR(actual.first_derivative.x, expected.first_derivative.x, 1e-9);
        EXPECT_NEAR(actual.first_derivative.y, expected.first_derivative.y, 1e-9);
    }
}
TEST(ExactSketchCurve, SplineSplitUsesExactKnotInsertionAndRetainsDerivative) {
    const auto source = cubic();
    const auto segment = trim_sketch_curve(source, 0.2, 0.8);
    EXPECT_EQ(segment.mode, "CONTROL");
    EXPECT_TRUE(segment.control_points.empty());
    for (int i = 0; i <= 20; ++i) {
        const double u = 0.2 + 0.6 * i / 20.0;
        const auto expected = evaluate_sketch_curve(source, u),
                   actual = evaluate_sketch_curve(segment, u);
        EXPECT_NEAR(actual.point.x, expected.point.x, 1e-12);
        EXPECT_NEAR(actual.point.y, expected.point.y, 1e-12);
        EXPECT_NEAR(actual.first_derivative.x, expected.first_derivative.x, 1e-11);
        EXPECT_NEAR(actual.first_derivative.y, expected.first_derivative.y, 1e-11);
    }
}
TEST(ExactSketchCurve, IntersectionsDistinguishCrossingTangencyOverlapAndNoIntersection) {
    ProfileCurveSpec line;
    line.kind = "LINE";
    line.start = {-2, 0};
    line.end = {2, 0};
    ProfileCurveSpec circle;
    circle.kind = "CIRCLE";
    circle.center = {0, 0};
    circle.radius = 1;
    const auto crossing = intersect_sketch_curves(line, circle);
    ASSERT_EQ(crossing.points.size(), 2U);
    EXPECT_TRUE(crossing.overlaps.empty());
    for (const auto& p : crossing.points) {
        EXPECT_NEAR(p.point.x * p.point.x + p.point.y * p.point.y, 1, 1e-12);
        EXPECT_EQ(p.kind, SketchCurveIntersectionKind::point);
    }
    line.start.y = 1;
    line.end.y = 1;
    const auto tangent = intersect_sketch_curves(line, circle);
    ASSERT_EQ(tangent.points.size(), 1U);
    EXPECT_EQ(tangent.points[0].kind, SketchCurveIntersectionKind::tangent);
    line.start.y = 2;
    line.end.y = 2;
    EXPECT_TRUE(intersect_sketch_curves(line, circle).points.empty());
    auto second = line;
    second.start = {0, 2};
    second.end = {3, 2};
    const auto overlap = intersect_sketch_curves(line, second);
    EXPECT_TRUE(overlap.points.empty());
    ASSERT_EQ(overlap.overlaps.size(), 1U);
    EXPECT_NEAR(overlap.overlaps[0].first_end - overlap.overlaps[0].first_start, 2, 1e-10);
}
TEST(ExactSketchCurve, EllipseArcAndSplineIntersectionsUseKernelParameters) {
    ProfileCurveSpec ellipse;
    ellipse.kind = "ELLIPSE";
    ellipse.center = {0, 0};
    ellipse.major_radius = 3;
    ellipse.minor_radius = 2;
    ProfileCurveSpec line;
    line.kind = "LINE";
    line.start = {-4, 0};
    line.end = {4, 0};
    const auto intersections = intersect_sketch_curves(ellipse, line);
    ASSERT_EQ(intersections.points.size(), 2U);
    for (const auto& p : intersections.points) {
        const auto point = evaluate_sketch_curve(ellipse, p.first_parameter).point;
        EXPECT_NEAR(point.x, p.point.x, 1e-12);
        EXPECT_NEAR(point.y, p.point.y, 1e-12);
    }
    line.start = {1.5, -5};
    line.end = {1.5, 5};
    const auto spline = intersect_sketch_curves(cubic(), line);
    ASSERT_EQ(spline.points.size(), 1U);
    EXPECT_NEAR(spline.points[0].first_parameter, 0.5, 1e-9);
}
}  // namespace occccad::kernel

#include <internal/occt_kernel.hpp>
namespace occccad::kernel {
TEST(ExactSketchCurve, ProfilePadUsesCanonicalCurveWithoutRefittingStaleFitPoints) {
    ProfileCurveSpec fit;
    fit.entity_id = "closed-spline";
    fit.kind = "SPLINE";
    fit.mode = "FIT";
    fit.closed = true;
    fit.control_points = {{0, 0}, {20, 0}, {20, 20}, {0, 20}};
    auto canonical = canonicalize_sketch_curve(fit);
    OcctKernel kernel;
    ProfileRegionSpec region;
    region.id = "region";
    region.outer = {"loop", {canonical}};
    ProfilePadSpec pad;
    pad.regions = {region};
    pad.pad_length = 5;
    pad.plane = "XY";
    const auto original = kernel.evaluateProfilePads({pad});
    const auto volume = kernel.getVolume(original);
    canonical.control_points = {{1000, 1000}, {1001, 1000}, {1001, 1001}, {1000, 1001}};
    pad.regions[0].outer.curves[0] = canonical;
    const auto reopened = kernel.evaluateProfilePads({pad});
    EXPECT_NEAR(kernel.getVolume(reopened), volume, 1e-8);
    EXPECT_EQ(kernel.getTopology(reopened).solid_count, 1U);
}
TEST(ExactSketchCurve, ReverseSplineTrimAndPeriodicCircleSeamAreExact) {
    const auto source = cubic(), reverse = trim_sketch_curve(source, 0.8, 0.2);
    EXPECT_NEAR(reverse.start.x, evaluate_sketch_curve(source, 0.8).point.x, 1e-12);
    EXPECT_NEAR(reverse.start.y, evaluate_sketch_curve(source, 0.8).point.y, 1e-12);
    EXPECT_NEAR(reverse.end.x, evaluate_sketch_curve(source, 0.2).point.x, 1e-12);
    EXPECT_NEAR(reverse.end.y, evaluate_sketch_curve(source, 0.2).point.y, 1e-12);
    ProfileCurveSpec circle;
    circle.kind = "CIRCLE";
    circle.radius = 2;
    circle.center = {10, 20};
    const auto arc = trim_sketch_curve(circle, 5.5, 7.0);
    EXPECT_EQ(arc.kind, "ARC");
    EXPECT_DOUBLE_EQ(arc.start_angle, 5.5);
    EXPECT_DOUBLE_EQ(arc.end_angle, 7.0);
    const auto midpoint = evaluate_sketch_curve(arc, 6.25).point;
    EXPECT_NEAR(midpoint.x, 10 + 2 * std::cos(6.25), 1e-12);
    EXPECT_NEAR(midpoint.y, 20 + 2 * std::sin(6.25), 1e-12);
}
}  // namespace occccad::kernel

namespace occccad::kernel {
TEST(ExactSketchCurve, FitAndRationalControlMirrorAreWholeCurveReflectionEquivariant) {
    for (const std::string mode : {"FIT", "CONTROL"}) {
        auto source = cubic();
        source.mode = mode;
        if (mode == "FIT") {
            source.control_points = source.poles;
            source.poles.clear();
        } else
            source.weights = {1, 0.7, 1.2, 1};
        auto target = source;
        const auto reflect = [](const Vec2& p) { return Vec2{p.y + 2, p.x - 2}; };  // axis y=x-2
        for (auto& p : target.control_points)
            p = reflect(p);
        for (auto& p : target.poles)
            p = reflect(p);
        const auto first = canonicalize_sketch_curve(source),
                   second = canonicalize_sketch_curve(target);
        for (int i = 0; i <= 30; ++i) {
            const double fraction = i / 30.0;
            const auto a = evaluate_sketch_curve(
                first,
                first.parameter_start + (first.parameter_end - first.parameter_start) * fraction);
            const auto b = evaluate_sketch_curve(
                second, second.parameter_start +
                            (second.parameter_end - second.parameter_start) * fraction);
            const auto expected = reflect(a.point);
            EXPECT_NEAR(b.point.x, expected.x, 1e-10);
            EXPECT_NEAR(b.point.y, expected.y, 1e-10);
            EXPECT_NEAR(b.first_derivative.x, a.first_derivative.y, 1e-9);
            EXPECT_NEAR(b.first_derivative.y, a.first_derivative.x, 1e-9);
        }
    }
}
}  // namespace occccad::kernel

namespace occccad::kernel {
TEST(ExactSketchCurve, NegativeArcAndEllipticalArcIntersectionsRetainOriginalUnwrappedDomain) {
    for (const std::string kind : {"ARC", "ELLIPTICAL_ARC"}) {
        ProfileCurveSpec arc;
        arc.kind = kind;
        arc.radius = 3;
        arc.major_radius = 3;
        arc.minor_radius = 2;
        arc.start_angle = 0.4;
        arc.end_angle = -2.0;
        ProfileCurveSpec line;
        line.kind = "LINE";
        line.start = {-4, -1};
        line.end = {4, -1};
        const auto intersections = intersect_sketch_curves(arc, line);
        ASSERT_FALSE(intersections.points.empty());
        for (const auto& p : intersections.points) {
            EXPECT_GE(p.first_parameter, arc.end_angle - 1e-12);
            EXPECT_LE(p.first_parameter, arc.start_angle + 1e-12);
            const auto value = evaluate_sketch_curve(arc, p.first_parameter).point;
            EXPECT_NEAR(value.x, p.point.x, 1e-10);
            EXPECT_NEAR(value.y, p.point.y, 1e-10);
            const double t =
                (p.first_parameter - arc.start_angle) / (arc.end_angle - arc.start_angle);
            EXPECT_GE(t, 0);
            EXPECT_LE(t, 1);
        }
    }
}
}  // namespace occccad::kernel

namespace occccad::kernel {
TEST(ExactSketchCurve, ProfileClassifiesExactNestedEllipsesAndIndependentRegions) {
    const auto ellipse = [](const char* id, double x, double a, double b, bool reversed) {
        ProfileCurveSpec c;
        c.kind = "ELLIPSE";
        c.entity_id = id;
        c.center = {x, 0};
        c.major_radius = a;
        c.minor_radius = b;
        c.rotation = 0.4;
        c.reversed = reversed;
        return ProfileLoopSpec{id, {c}};
    };
    const auto regions = classify_sketch_profile(
        {ellipse("island", 0, 1, 0.5, true), ellipse("hole", 0, 3, 2, false),
         ellipse("outer", 0, 6, 5, true), ellipse("independent", 20, 2, 1, true)});
    ASSERT_EQ(regions.size(), 3U);
    EXPECT_EQ(regions[0].outer.id, "outer");
    ASSERT_EQ(regions[0].holes.size(), 1U);
    EXPECT_EQ(regions[0].holes[0].id, "hole");
    OcctKernel kernel;
    ProfilePadSpec pad;
    pad.regions = regions;
    pad.pad_length = 2;
    const auto shape = kernel.evaluateProfilePads({pad});
    EXPECT_NEAR(kernel.getVolume(shape), 2 * std::acos(-1.0) * (30 - 6 + 0.5 + 2), 1e-5);
}
TEST(ExactSketchCurve, ProfileRejectsExactCrossingTouchOverlapAndSelfIntersection) {
    const auto circle = [](const char* id, double x) {
        ProfileCurveSpec c;
        c.kind = "CIRCLE";
        c.entity_id = id;
        c.center = {x, 0};
        c.radius = 2;
        return ProfileLoopSpec{id, {c}};
    };
    EXPECT_THROW(classify_sketch_profile({circle("a", 0), circle("b", 1)}), std::invalid_argument);
    EXPECT_THROW(classify_sketch_profile({circle("a", 0), circle("b", 4)}), std::invalid_argument);
    EXPECT_THROW(classify_sketch_profile({circle("a", 0), circle("b", 0)}), std::invalid_argument);
    ProfileLoopSpec bow;
    bow.id = "bow";
    const std::vector<Vec2> p = {{0, 0}, {3, 3}, {0, 3}, {3, 0}};
    for (std::size_t i = 0; i < p.size(); ++i) {
        ProfileCurveSpec c;
        c.kind = "LINE";
        c.entity_id = std::to_string(i);
        c.start = p[i];
        c.end = p[(i + 1) % p.size()];
        bow.curves.push_back(c);
    }
    EXPECT_THROW(classify_sketch_profile({bow}), std::invalid_argument);
    ProfileCurveSpec self;
    self.kind = "SPLINE";
    self.entity_id = "self";
    self.mode = "CONTROL";
    self.closed = true;
    self.degree = 1;
    self.poles = {{0, 0}, {3, 3}, {0, 3}, {3, 0}, {0, 0}};
    self.knots = {0, 1, 2, 3, 4};
    self.multiplicities = {2, 1, 1, 1, 2};
    self.weights = {1, 1, 1, 1, 1};
    self.parameter_end = 4;
    EXPECT_THROW(classify_sketch_profile({{"self-loop", {self}}}), std::invalid_argument);
}
TEST(ExactSketchCurve, ProfilePermitsTwoAdjacentSemicircleContactsAndStableReordering) {
    const double pi = std::acos(-1.0);
    ProfileCurveSpec right;
    right.kind = "ARC";
    right.entity_id = "right";
    right.center = {0, 0};
    right.radius = 5;
    right.start_angle = -pi / 2;
    right.end_angle = pi / 2;
    auto left = right;
    left.entity_id = "left";
    left.start_angle = 3 * pi / 2;
    left.end_angle = pi / 2;
    left.reversed = true;
    const auto hits = intersect_sketch_curves(right, left);
    EXPECT_TRUE(hits.overlaps.empty());
    EXPECT_EQ(hits.points.size(), 2U);
    const auto a = classify_sketch_profile({{"circle-loop", {right, left}}});
    const auto b = classify_sketch_profile({{"circle-loop", {left, right}}});
    ASSERT_EQ(a.size(), 1U);
    ASSERT_EQ(b.size(), 1U);
    EXPECT_EQ(a[0].id, b[0].id);
    OcctKernel kernel;
    ProfilePadSpec pad;
    pad.regions = a;
    pad.pad_length = 3;
    EXPECT_NEAR(kernel.getVolume(kernel.evaluateProfilePads({pad})), 75 * pi, 1e-5);
}
}  // namespace occccad::kernel
namespace occccad::kernel {
TEST(ExactSketchCurve, SplitEllipseAdjacentSubarcsRemainExactClosedProfile) {
    ProfileCurveSpec source;
    source.kind = "ELLIPSE";
    source.entity_id = "ellipse";
    source.center = {8, 9};
    source.major_radius = 5;
    source.minor_radius = 2;
    source.rotation = 0.7;
    const double split = 2 * std::acos(-1.0) * 0.37;
    auto a = trim_sketch_curve(source, 0, split),
         b = trim_sketch_curve(source, split, 2 * std::acos(-1.0));
    a.entity_id = "first";
    b.entity_id = "second";
    const auto hits = intersect_sketch_curves(a, b);
    EXPECT_TRUE(hits.overlaps.empty());
    EXPECT_EQ(hits.points.size(), 2U);
    const auto regions = classify_sketch_profile({{"split-ellipse", {a, b}}});
    ASSERT_EQ(regions.size(), 1U);
    OcctKernel kernel;
    ProfilePadSpec pad;
    pad.regions = regions;
    pad.pad_length = 3;
    EXPECT_NEAR(kernel.getVolume(kernel.evaluateProfilePads({pad})), 30 * std::acos(-1.0), 1e-5);
}
TEST(ExactSketchCurve, FullPeriodicOverlapRetainsNonzeroNativeInterval) {
    ProfileCurveSpec circle;
    circle.kind = "CIRCLE";
    circle.center = {0, 0};
    circle.radius = 5;
    const auto hits = intersect_sketch_curves(circle, circle);
    ASSERT_FALSE(hits.overlaps.empty());
    double total = 0;
    for (const auto& overlap : hits.overlaps)
        total += std::abs(overlap.first_end - overlap.first_start);
    EXPECT_NEAR(total, 2 * std::acos(-1.0), 1e-8);
}
}  // namespace occccad::kernel
