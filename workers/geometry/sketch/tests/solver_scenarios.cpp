#include <gtest/gtest.h>

#include "occccad/geometry/sketch/sketch_solver.h"

#include <cmath>

namespace occccad::geometry::sketch {
namespace {
GeometryRef endpoint(const char* id, SubElement sub_element) {
    return {GeometryTarget::entity, id, sub_element};
}

GeometryRef axis(GeometryTarget target) {
    return {target, {}, SubElement::direction};
}

SketchModel rectangle() {
    SketchModel model;
    model.lines = {
        {"bottom", {1.0, 2.0}, {7.1, 2.2}},
        {"right", {7.0, 2.1}, {6.8, 5.0}},
        {"top", {6.9, 5.1}, {0.9, 4.8}},
        {"left", {1.1, 4.9}, {1.0, 2.1}},
    };
    model.constraints = {
        {"join-0",
         ConstraintKind::coincident,
         {endpoint("bottom", SubElement::end), endpoint("right", SubElement::start)},
         {}},
        {"join-1",
         ConstraintKind::coincident,
         {endpoint("right", SubElement::end), endpoint("top", SubElement::start)},
         {}},
        {"join-2",
         ConstraintKind::coincident,
         {endpoint("top", SubElement::end), endpoint("left", SubElement::start)},
         {}},
        {"join-3",
         ConstraintKind::coincident,
         {endpoint("left", SubElement::end), endpoint("bottom", SubElement::start)},
         {}},
        {"parallel-x-0",
         ConstraintKind::parallel,
         {endpoint("bottom", SubElement::direction), axis(GeometryTarget::sketch_x_axis)},
         {}},
        {"parallel-y-0",
         ConstraintKind::parallel,
         {endpoint("right", SubElement::direction), axis(GeometryTarget::sketch_y_axis)},
         {}},
        {"parallel-x-1",
         ConstraintKind::parallel,
         {endpoint("top", SubElement::direction), axis(GeometryTarget::sketch_x_axis)},
         {}},
        {"parallel-y-1",
         ConstraintKind::parallel,
         {endpoint("left", SubElement::direction), axis(GeometryTarget::sketch_y_axis)},
         {}},
    };
    return model;
}
}  // namespace

TEST(PlaneGcsSketchSolver, SolvesRectangleMacroWithExplicitCoincidentAndAxisConstraints) {
    const auto result = make_plane_gcs_sketch_solver()->solve(rectangle());

    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.lines.size(), 4U);
    EXPECT_EQ(result.degrees_of_freedom, 4);
    EXPECT_NEAR(result.lines[0].end.x, result.lines[1].start.x, 1e-8);
    EXPECT_NEAR(result.lines[0].end.y, result.lines[1].start.y, 1e-8);
    EXPECT_NEAR(result.lines[0].start.y, result.lines[0].end.y, 1e-8);
    EXPECT_NEAR(result.lines[1].start.x, result.lines[1].end.x, 1e-8);
    EXPECT_NEAR(result.lines[2].start.y, result.lines[2].end.y, 1e-8);
    EXPECT_NEAR(result.lines[3].start.x, result.lines[3].end.x, 1e-8);
}

TEST(PlaneGcsSketchSolver, RejectsUnknownReferencesBeforeCallingBackend) {
    SketchModel model;
    model.lines = {{"line", {0.0, 0.0}, {1.0, 1.0}}};
    model.constraints = {
        {"bad",
         ConstraintKind::coincident,
         {endpoint("line", SubElement::start), endpoint("missing", SubElement::end)},
         {}}};

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::invalid_model);
    EXPECT_NE(result.diagnostic.find("invalid point reference"), std::string::npos);
}

TEST(PlaneGcsSketchSolver, AcceptsUnconstrainedPrimitiveEntities) {
    SketchModel model;
    model.points = {{"point", {2.0, 3.0}}};
    model.lines = {{"line", {0.0, 0.0}, {4.0, 5.0}}};

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 6);
    ASSERT_EQ(result.points.size(), 1U);
    ASSERT_EQ(result.lines.size(), 1U);
}

TEST(PlaneGcsSketchSolver, SolvesCircleRadiusAndLineTangentConstraints) {
    SketchModel model;
    model.lines = {{"line", {-10.0, 5.2}, {10.0, 5.2}}};
    model.circles = {{"circle", {0.0, 0.0}, 4.8}};
    model.constraints = {
        {"horizontal", ConstraintKind::horizontal, {endpoint("line", SubElement::direction)}, {}},
        {"radius", ConstraintKind::radius, {endpoint("circle", SubElement::whole)}, {}, 5.0, "mm"},
        {"tangent",
         ConstraintKind::tangent,
         {endpoint("line", SubElement::whole), endpoint("circle", SubElement::whole)},
         {}},
    };

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.circles.size(), 1U);
    EXPECT_NEAR(result.circles[0].radius, 5.0, 1e-8);
    EXPECT_NEAR(std::abs(result.lines[0].start.y - result.circles[0].center.y), 5.0, 1e-8);
}

TEST(PlaneGcsSketchSolver, SolvesSlotMacroWithoutRedundantConstraints) {
    SketchModel model;
    model.lines = {{"top", {0.0, 5.0}, {20.0, 5.0}},
                   {"bottom", {20.0, -5.0}, {0.0, -5.0}}};
    model.arcs = {{"right", {20.0, 0.0}, 5.0, 1.5707963267948966, 4.71238898038469},
                  {"left", {0.0, 0.0}, 5.0, 4.71238898038469, 7.853981633974483}};
    const std::vector<std::string> ids = {"top", "right", "bottom", "left"};
    for (std::size_t index = 0; index < ids.size(); ++index) {
        model.constraints.push_back(
            {"join-" + std::to_string(index), ConstraintKind::coincident,
             {endpoint(ids[index].c_str(), SubElement::end),
              endpoint(ids[(index + 1U) % ids.size()].c_str(), SubElement::start)},
             {}, 0.0, {}, true});
        model.constraints.push_back(
            {"tangent-" + std::to_string(index), ConstraintKind::tangent,
             {endpoint(ids[index].c_str(), SubElement::whole),
              endpoint(ids[(index + 1U) % ids.size()].c_str(), SubElement::whole)},
             {}, 0.0, {}, true});
    }
    model.constraints.push_back(
        {"equal-radius", ConstraintKind::equal,
         {endpoint("right", SubElement::whole), endpoint("left", SubElement::whole)}, {}, 0.0,
         {}, true});

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    std::string redundant;
    for (const auto& id : result.redundant_constraint_ids) redundant += id + ",";
    EXPECT_EQ(result.status, SolveStatus::under_constrained)
        << result.diagnostic << " redundant=" << redundant;
}

TEST(PlaneGcsSketchSolver, SolvesRegularHexagonMacro) {
    SketchModel model;
    constexpr double pi = 3.14159265358979323846;
    for (int index = 0; index < 6; ++index) {
        const double first = index * pi / 3.0;
        const double second = (index + 1) * pi / 3.0;
        model.lines.push_back({"edge-" + std::to_string(index),
                               {10.0 * std::cos(first), 10.0 * std::sin(first)},
                               {10.0 * std::cos(second), 10.0 * std::sin(second)}});
    }
    for (int index = 0; index < 6; ++index) {
        const auto current = "edge-" + std::to_string(index);
        const auto next = "edge-" + std::to_string((index + 1) % 6);
        model.constraints.push_back(
            {"join-" + std::to_string(index), ConstraintKind::coincident,
             {endpoint(current.c_str(), SubElement::end), endpoint(next.c_str(), SubElement::start)},
             {}, 0.0, {}, true});
        if (index == 0) continue;
        model.constraints.push_back(
            {"equal-" + std::to_string(index), ConstraintKind::equal,
             {endpoint("edge-0", SubElement::whole), endpoint(current.c_str(), SubElement::whole)},
             {}, 0.0, {}, true});
        model.constraints.push_back(
            {"angle-" + std::to_string(index), ConstraintKind::angle,
             {endpoint(("edge-" + std::to_string(index - 1)).c_str(), SubElement::direction),
              endpoint(current.c_str(), SubElement::direction)},
             {}, 60.0, "deg", true});
    }

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
}

TEST(PlaneGcsSketchSolver, SolvesConstraintAddedAfterDisconnectedRegularHexagon) {
    auto model = SketchModel{};
    constexpr double pi = 3.14159265358979323846;
    for (int index = 0; index < 6; ++index) {
        const double first = index * pi / 3.0;
        const double second = (index + 1) * pi / 3.0;
        model.lines.push_back({"edge-" + std::to_string(index),
                               {50.0 * std::cos(first), 50.0 * std::sin(first)},
                               {50.0 * std::cos(second), 50.0 * std::sin(second)}});
    }
    for (int index = 0; index < 6; ++index) {
        const auto current = "edge-" + std::to_string(index);
        const auto next = "edge-" + std::to_string((index + 1) % 6);
        model.constraints.push_back(
            {"join-" + std::to_string(index), ConstraintKind::coincident,
             {endpoint(current.c_str(), SubElement::end), endpoint(next.c_str(), SubElement::start)},
             {}, 0.0, {}, true});
        if (index == 0) continue;
        model.constraints.push_back(
            {"equal-" + std::to_string(index), ConstraintKind::equal,
             {endpoint("edge-0", SubElement::whole), endpoint(current.c_str(), SubElement::whole)},
             {}, 0.0, {}, true});
        model.constraints.push_back(
            {"angle-" + std::to_string(index), ConstraintKind::angle,
             {endpoint(("edge-" + std::to_string(index - 1)).c_str(), SubElement::direction),
              endpoint(current.c_str(), SubElement::direction)},
             {}, 60.0, "deg", true});
    }
    model.lines.push_back({"later-line", {-58.371609311792255, 56.71477323962336}, {60.0, 90.0}});
    model.splines.push_back({"later-spline",
                             {{40.01896358930731, 65.63619824360909},
                              {126.17443934208364, 85.5182311096344},
                              {174.85993007811993, -25.872132511558654},
                              {84.62608860923586, 38.87192323165195},
                              {105.52771290428815, -86.28292468140478}},
                             3, false});
    model.constraints.push_back(
        {"horizontal", ConstraintKind::horizontal,
         {endpoint("later-line", SubElement::direction)}, {}});
    model.constraints.push_back(
        {"spline-control-at-origin", ConstraintKind::coincident,
         {{GeometryTarget::sketch_origin, {}, SubElement::point},
          {GeometryTarget::entity, "later-spline", SubElement::control, 4}}, {}});

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    ASSERT_TRUE(result.status == SolveStatus::under_constrained || result.status == SolveStatus::redundant)
        << result.diagnostic;
    ASSERT_EQ(result.lines.size(), 7U);
    EXPECT_NEAR(result.lines.back().start.y, result.lines.back().end.y, 1e-8);
    ASSERT_EQ(result.splines.size(), 1U);
    EXPECT_NEAR(result.splines[0].control_points[4].x, 0.0, 1e-8);
    EXPECT_NEAR(result.splines[0].control_points[4].y, 0.0, 1e-8);
}

TEST(PlaneGcsSketchSolver, SolvesPointLinePointSymmetry) {
    SketchModel model;
    model.points = {{"first", {-4.8, 2.2}}, {"second", {5.1, 1.8}}};
    model.lines = {{"axis", {0.0, -10.0}, {0.0, 10.0}}};
    model.constraints = {
        {"fixed-axis", ConstraintKind::fixed, {endpoint("axis", SubElement::whole)}, {}},
        {"symmetric",
         ConstraintKind::symmetry,
         {endpoint("first", SubElement::point), endpoint("axis", SubElement::direction),
          endpoint("second", SubElement::point)},
         {}}};

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.points.size(), 2U);
    EXPECT_NEAR(result.points[0].point.x, -result.points[1].point.x, 1e-8);
    EXPECT_NEAR(result.points[0].point.y, result.points[1].point.y, 1e-8);
}

TEST(PlaneGcsSketchSolver, SolvesPointPointPointSymmetry) {
    SketchModel model;
    model.points = {{"first", {1.8, 2.9}}, {"center", {0.0, 0.0}}, {"second", {-2.2, -3.1}}};
    model.constraints = {
        {"fixed-center", ConstraintKind::fixed, {endpoint("center", SubElement::whole)}, {}},
        {"symmetric",
         ConstraintKind::symmetry,
         {endpoint("first", SubElement::point), endpoint("center", SubElement::point),
          endpoint("second", SubElement::point)},
         {}}};

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.points.size(), 3U);
    EXPECT_NEAR(result.points[0].point.x + result.points[2].point.x, 0.0, 1e-8);
    EXPECT_NEAR(result.points[0].point.y + result.points[2].point.y, 0.0, 1e-8);
}

TEST(PlaneGcsSketchSolver, KeepsAxisSymmetryWhenOneEquationIsAlreadyImplied) {
    SketchModel model;
    model.lines = {{"edge", {-5.0, 3.0}, {5.0, 3.0}}};
    model.constraints = {
        {"horizontal", ConstraintKind::horizontal, {endpoint("edge", SubElement::direction)}, {}},
        {"symmetric-y",
         ConstraintKind::symmetry,
         {endpoint("edge", SubElement::start), axis(GeometryTarget::sketch_y_axis),
          endpoint("edge", SubElement::end)},
         {}}};

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_TRUE(result.redundant_constraint_ids.empty());
    ASSERT_EQ(result.lines.size(), 1U);
    EXPECT_NEAR(result.lines[0].start.x, -result.lines[0].end.x, 1e-8);
    EXPECT_NEAR(result.lines[0].start.y, result.lines[0].end.y, 1e-8);
}

TEST(PlaneGcsSketchSolver, SolvesPointToIntrinsicAxisDistance) {
    SketchModel model;
    model.points = {{"point", {4.0, 8.0}}};
    model.constraints = {
        {"distance-x",
         ConstraintKind::distance,
         {endpoint("point", SubElement::point), axis(GeometryTarget::sketch_x_axis)},
         {}, 5.0, "mm"}};

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.points.size(), 1U);
    EXPECT_NEAR(std::abs(result.points[0].point.y), 5.0, 1e-8);
}

TEST(PlaneGcsSketchSolver, ClassifiesConflictsWithoutLeakingBackendStatus) {
    SketchModel model;
    model.points = {{"point", {0.0, 0.0}}};
    model.constraints = {
        {"first", ConstraintKind::fixed_point, {endpoint("point", SubElement::point)}, {1.0, 2.0}},
        {"second", ConstraintKind::fixed_point, {endpoint("point", SubElement::point)}, {4.0, 6.0}},
    };

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::conflicting) << result.diagnostic;
    EXPECT_FALSE(result.conflicting_constraint_ids.empty());
    EXPECT_EQ(result.diagnostic.find("PlaneGCS"), std::string::npos);
    ASSERT_EQ(result.points.size(), 1U);
}

TEST(PlaneGcsSketchSolver, SolvesArcClosureAngleToIntrinsicXAxisRegression) {
    SketchModel model;
    model.arcs = {{"arc", {-4.17222551406913e-23, 0.0}, 90.0,
                   2.1599643563596285, 7.265087723106667}};
    model.lines = {{"left", {-50.01025611280795, 74.82629406519713}, {0.0, 0.0}},
                   {"right", {0.0, 0.0}, {49.9897429479288, 74.84000000000002}}};
    model.constraints = {
        {"arc-center", ConstraintKind::coincident,
         {endpoint("arc", SubElement::center), {GeometryTarget::sketch_origin, {}, SubElement::point}}},
        {"line-join", ConstraintKind::coincident,
         {endpoint("left", SubElement::end), endpoint("right", SubElement::start)}},
        {"left-arc", ConstraintKind::coincident,
         {endpoint("left", SubElement::start), endpoint("arc", SubElement::start)}},
        {"right-arc", ConstraintKind::coincident,
         {endpoint("right", SubElement::end), endpoint("arc", SubElement::end)}},
        {"radius", ConstraintKind::radius, {endpoint("arc", SubElement::whole)}, {}, 90.0, "mm"},
        {"chord", ConstraintKind::distance,
         {endpoint("arc", SubElement::start), endpoint("arc", SubElement::end)}, {}, 100.0, "mm"},
        {"at-origin", ConstraintKind::coincident,
         {endpoint("left", SubElement::end), {GeometryTarget::sketch_origin, {}, SubElement::point}}},
        {"angle", ConstraintKind::angle,
         {endpoint("right", SubElement::direction), axis(GeometryTarget::sketch_x_axis)}, {},
         56.25, "deg"},
    };

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 0);
}
}  // namespace occccad::geometry::sketch
