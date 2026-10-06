#include <gtest/gtest.h>

#include "occccad/geometry/sketch/external_geometry_projector.h"
#include "occccad/geometry/sketch/sketch_solver.h"

#include <cmath>

#include "../src/reflection_normal_constraint.h"

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
    model.lines = {{"top", {0.0, 5.0}, {20.0, 5.0}}, {"bottom", {20.0, -5.0}, {0.0, -5.0}}};
    model.arcs = {{"right", {20.0, 0.0}, 5.0, 1.5707963267948966, 4.71238898038469},
                  {"left", {0.0, 0.0}, 5.0, 4.71238898038469, 7.853981633974483}};
    const std::vector<std::string> ids = {"top", "right", "bottom", "left"};
    for (std::size_t index = 0; index < ids.size(); ++index) {
        model.constraints.push_back(
            {"join-" + std::to_string(index),
             ConstraintKind::coincident,
             {endpoint(ids[index].c_str(), SubElement::end),
              endpoint(ids[(index + 1U) % ids.size()].c_str(), SubElement::start)},
             {},
             0.0,
             {},
             true});
        model.constraints.push_back(
            {"tangent-" + std::to_string(index),
             ConstraintKind::tangent,
             {endpoint(ids[index].c_str(), SubElement::whole),
              endpoint(ids[(index + 1U) % ids.size()].c_str(), SubElement::whole)},
             {},
             0.0,
             {},
             true});
    }
    model.constraints.push_back(
        {"equal-radius",
         ConstraintKind::equal,
         {endpoint("right", SubElement::whole), endpoint("left", SubElement::whole)},
         {},
         0.0,
         {},
         true});

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    std::string redundant;
    for (const auto& id : result.redundant_constraint_ids)
        redundant += id + ",";
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
        model.constraints.push_back({"join-" + std::to_string(index),
                                     ConstraintKind::coincident,
                                     {endpoint(current.c_str(), SubElement::end),
                                      endpoint(next.c_str(), SubElement::start)},
                                     {},
                                     0.0,
                                     {},
                                     true});
        if (index == 0)
            continue;
        model.constraints.push_back(
            {"equal-" + std::to_string(index),
             ConstraintKind::equal,
             {endpoint("edge-0", SubElement::whole), endpoint(current.c_str(), SubElement::whole)},
             {},
             0.0,
             {},
             true});
        model.constraints.push_back(
            {"angle-" + std::to_string(index),
             ConstraintKind::angle,
             {endpoint(("edge-" + std::to_string(index - 1)).c_str(), SubElement::direction),
              endpoint(current.c_str(), SubElement::direction)},
             {},
             60.0,
             "deg",
             true});
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
        model.constraints.push_back({"join-" + std::to_string(index),
                                     ConstraintKind::coincident,
                                     {endpoint(current.c_str(), SubElement::end),
                                      endpoint(next.c_str(), SubElement::start)},
                                     {},
                                     0.0,
                                     {},
                                     true});
        if (index == 0)
            continue;
        model.constraints.push_back(
            {"equal-" + std::to_string(index),
             ConstraintKind::equal,
             {endpoint("edge-0", SubElement::whole), endpoint(current.c_str(), SubElement::whole)},
             {},
             0.0,
             {},
             true});
        model.constraints.push_back(
            {"angle-" + std::to_string(index),
             ConstraintKind::angle,
             {endpoint(("edge-" + std::to_string(index - 1)).c_str(), SubElement::direction),
              endpoint(current.c_str(), SubElement::direction)},
             {},
             60.0,
             "deg",
             true});
    }
    model.lines.push_back({"later-line", {-58.371609311792255, 56.71477323962336}, {60.0, 90.0}});
    model.splines.push_back({"later-spline",
                             {{40.01896358930731, 65.63619824360909},
                              {126.17443934208364, 85.5182311096344},
                              {174.85993007811993, -25.872132511558654},
                              {84.62608860923586, 38.87192323165195},
                              {105.52771290428815, -86.28292468140478}},
                             3,
                             false});
    model.constraints.push_back({"horizontal",
                                 ConstraintKind::horizontal,
                                 {endpoint("later-line", SubElement::direction)},
                                 {}});
    model.constraints.push_back({"spline-control-at-origin",
                                 ConstraintKind::coincident,
                                 {{GeometryTarget::sketch_origin, {}, SubElement::point},
                                  {GeometryTarget::entity, "later-spline", SubElement::control, 4}},
                                 {}});

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    ASSERT_TRUE(result.status == SolveStatus::under_constrained ||
                result.status == SolveStatus::redundant)
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
         {},
         5.0,
         "mm"}};

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
    model.arcs = {
        {"arc", {-4.17222551406913e-23, 0.0}, 90.0, 2.1599643563596285, 7.265087723106667}};
    model.lines = {{"left", {-50.01025611280795, 74.82629406519713}, {0.0, 0.0}},
                   {"right", {0.0, 0.0}, {49.9897429479288, 74.84000000000002}}};
    model.constraints = {
        {"arc-center",
         ConstraintKind::coincident,
         {endpoint("arc", SubElement::center),
          {GeometryTarget::sketch_origin, {}, SubElement::point}}},
        {"line-join",
         ConstraintKind::coincident,
         {endpoint("left", SubElement::end), endpoint("right", SubElement::start)}},
        {"left-arc",
         ConstraintKind::coincident,
         {endpoint("left", SubElement::start), endpoint("arc", SubElement::start)}},
        {"right-arc",
         ConstraintKind::coincident,
         {endpoint("right", SubElement::end), endpoint("arc", SubElement::end)}},
        {"radius", ConstraintKind::radius, {endpoint("arc", SubElement::whole)}, {}, 90.0, "mm"},
        {"chord",
         ConstraintKind::distance,
         {endpoint("arc", SubElement::start), endpoint("arc", SubElement::end)},
         {},
         100.0,
         "mm"},
        {"at-origin",
         ConstraintKind::coincident,
         {endpoint("left", SubElement::end),
          {GeometryTarget::sketch_origin, {}, SubElement::point}}},
        {"angle",
         ConstraintKind::angle,
         {endpoint("right", SubElement::direction), axis(GeometryTarget::sketch_x_axis)},
         {},
         56.25,
         "deg"},
    };

    const auto result = make_plane_gcs_sketch_solver()->solve(model);

    EXPECT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 0);
}

TEST(ExternalGeometryProjector, ProjectsVertexAndLinearEdgeIntoSupportFrame) {
    const ProjectionFrame frame{{0.0, 0.0, 10.0}, {1.0, 0.0, 0.0}, {0.0, 0.0, 1.0}};
    const auto point =
        project_external_geometry({ExternalSourceKind::point, {3.0, 4.0, 15.0}}, frame);
    EXPECT_EQ(point.status, ExternalProjectionStatus::connected);
    EXPECT_EQ(point.kind, ProjectedGeometryKind::point);
    EXPECT_NEAR(point.point.x, 3.0, 1e-12);
    EXPECT_NEAR(point.point.y, 4.0, 1e-12);

    ExternalProjectionSource line;
    line.kind = ExternalSourceKind::line;
    line.origin = {1.0, 2.0, 7.0};
    line.direction = {1.0, 0.0, 0.0};
    line.parameter_start = 2.0;
    line.parameter_end = 8.0;
    line.has_parameters = true;
    const auto projected = project_external_geometry(line, frame);
    EXPECT_EQ(projected.status, ExternalProjectionStatus::connected);
    EXPECT_EQ(projected.kind, ProjectedGeometryKind::line);
    EXPECT_NEAR(projected.start.x, 3.0, 1e-12);
    EXPECT_NEAR(projected.start.y, 2.0, 1e-12);
    EXPECT_NEAR(projected.end.x, 9.0, 1e-12);
    EXPECT_NEAR(projected.end.y, 2.0, 1e-12);
}

TEST(ExternalGeometryProjector, ProjectsAxisAsDirectionOrPoint) {
    const ProjectionFrame frame{{0, 0, 10}, {1, 0, 0}, {0, 0, 1}};
    ExternalProjectionSource axis;
    axis.kind = ExternalSourceKind::axis;
    axis.origin = {3, 4, 20};
    axis.direction = {2, 0, 2};
    const auto line = project_external_geometry(axis, frame);
    ASSERT_EQ(line.status, ExternalProjectionStatus::connected);
    ASSERT_EQ(line.kind, ProjectedGeometryKind::line);
    EXPECT_NEAR(line.start.x, 3, 1e-12);
    EXPECT_NEAR(line.start.y, 4, 1e-12);
    EXPECT_NEAR(line.end.x, 4, 1e-12);
    EXPECT_NEAR(line.end.y, 4, 1e-12);
    axis.direction = {0, 0, -1};
    const auto point = project_external_geometry(axis, frame);
    ASSERT_EQ(point.status, ExternalProjectionStatus::connected);
    ASSERT_EQ(point.kind, ProjectedGeometryKind::point);
    EXPECT_NEAR(point.point.x, 3, 1e-12);
    EXPECT_NEAR(point.point.y, 4, 1e-12);
    axis.direction = {0, 0, 0};
    EXPECT_EQ(project_external_geometry(axis, frame).status, ExternalProjectionStatus::unresolved);
}

TEST(ExternalGeometryProjector, ProjectsFullCircleAndPerpendicularLineAsPoint) {
    const ProjectionFrame frame{{0.0, 0.0, 0.0}, {1.0, 0.0, 0.0}, {0.0, 0.0, 1.0}};
    ExternalProjectionSource circle;
    circle.kind = ExternalSourceKind::circle;
    circle.origin = {5.0, 6.0, 4.0};
    circle.direction = {0.0, 0.0, -1.0};
    circle.parameter_start = 0.0;
    circle.parameter_end = 2.0 * 3.14159265358979323846;
    circle.measure_mm = circle.parameter_end * 3.0;
    circle.has_parameters = true;
    const auto projected = project_external_geometry(circle, frame);
    EXPECT_EQ(projected.status, ExternalProjectionStatus::connected);
    EXPECT_EQ(projected.kind, ProjectedGeometryKind::circle);
    EXPECT_NEAR(projected.center.x, 5.0, 1e-12);
    EXPECT_NEAR(projected.center.y, 6.0, 1e-12);
    EXPECT_NEAR(projected.radius, 3.0, 1e-12);

    ExternalProjectionSource normal_line;
    normal_line.kind = ExternalSourceKind::line;
    normal_line.origin = {1.0, 2.0, 0.0};
    normal_line.direction = {0.0, 0.0, 1.0};
    normal_line.parameter_start = 0.0;
    normal_line.parameter_end = 5.0;
    normal_line.has_parameters = true;
    const auto degenerate = project_external_geometry(normal_line, frame);
    EXPECT_EQ(degenerate.status, ExternalProjectionStatus::connected);
    EXPECT_EQ(degenerate.kind, ProjectedGeometryKind::point);
    EXPECT_NEAR(degenerate.point.x, 1.0, 1e-12);
    EXPECT_NEAR(degenerate.point.y, 2.0, 1e-12);
    normal_line.direction = {0.0, 0.0, 0.0};
    EXPECT_EQ(project_external_geometry(normal_line, frame).diagnostic_code,
              "EXTERNAL_SOURCE_CONTRACT_INCOMPLETE");
}

// Minimal geometry from CAD_DIAGNOSTIC cd8b897686593a7c: array-member
// EDGE #11 is normal to the planar-face sketch support, not an invalid source.
TEST(ExternalGeometryProjector, ProjectsBottomSupportDiagnosticEdgeAsPoint) {
    const ProjectionFrame frame{{60.0, -20.0, 0.0}, {1.0, 0.0, 0.0}, {0.0, -1.0, 0.0}};
    ExternalProjectionSource edge;
    edge.kind = ExternalSourceKind::line;
    edge.origin = {-60.0, 20.0, 0.0};
    edge.direction = {0.0, -1.0, 0.0};
    edge.parameter_start = 0.0;
    edge.parameter_end = 40.0;
    edge.has_parameters = true;
    const auto projected = project_external_geometry(edge, frame);
    EXPECT_EQ(projected.status, ExternalProjectionStatus::connected);
    EXPECT_EQ(projected.kind, ProjectedGeometryKind::point);
    EXPECT_NEAR(projected.point.x, -120.0, 1e-12);
    EXPECT_NEAR(projected.point.y, 0.0, 1e-12);
    edge.parameter_end = edge.parameter_start;
    EXPECT_EQ(project_external_geometry(edge, frame).diagnostic_code,
              "EXTERNAL_SOURCE_CONTRACT_INCOMPLETE");
}

TEST(ExternalGeometryProjector, RejectsPartialCircleUntilArcEvidenceIsAvailable) {
    const ProjectionFrame frame{{0.0, 0.0, 0.0}, {1.0, 0.0, 0.0}, {0.0, 0.0, 1.0}};
    ExternalProjectionSource arc;
    arc.kind = ExternalSourceKind::circle;
    arc.origin = {0.0, 0.0, 0.0};
    arc.direction = {0.0, 0.0, 1.0};
    arc.parameter_start = 0.0;
    arc.parameter_end = 3.14159265358979323846;
    arc.measure_mm = arc.parameter_end * 5.0;
    arc.has_parameters = true;

    const auto projected = project_external_geometry(arc, frame);

    EXPECT_EQ(projected.status, ExternalProjectionStatus::unresolved);
    EXPECT_EQ(projected.diagnostic_code, "EXTERNAL_PROJECTION_TYPE_UNSUPPORTED");
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
namespace {
SketchModel parallel_spacing(double spacing, bool reverse = false) {
    SketchModel model;
    model.lines = {{"base", {10.0, 20.0}, {16.0, 28.0}}, {"offset", {8.4, 21.2}, {14.4, 29.2}}};
    if (reverse)
        std::swap(model.lines[0].start, model.lines[0].end);
    model.constraints = {
        {"parallel",
         ConstraintKind::parallel,
         {endpoint("offset", SubElement::direction), endpoint("base", SubElement::whole)},
         {}},
        {"spacing",
         ConstraintKind::distance,
         {endpoint("base", SubElement::whole), endpoint("offset", SubElement::whole)},
         {},
         spacing,
         "mm"}};
    return model;
}
void assert_spacing(const SolveResult& result, double spacing) {
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.lines.size(), 2U);
    const auto& a = result.lines[0];
    const auto& b = result.lines[1];
    const double dx = a.end.x - a.start.x, dy = a.end.y - a.start.y;
    const double ex = b.end.x - b.start.x, ey = b.end.y - b.start.y;
    ASSERT_GT(std::hypot(dx, dy), 1e-8);
    EXPECT_NEAR((dx * ey - dy * ex) / (std::hypot(dx, dy) * std::hypot(ex, ey)), 0.0, 1e-8);
    for (const auto& point : {b.start, b.end})
        EXPECT_NEAR(
            std::abs(dx * (point.y - a.start.y) - dy * (point.x - a.start.x)) / std::hypot(dx, dy),
            spacing, 1e-8);
}
}  // namespace

TEST(PlaneGcsSketchSolver, ParallelLineDistanceSolvesWithoutHiddenFix) {
    const auto result = make_plane_gcs_sketch_solver()->solve(parallel_spacing(5.0));
    assert_spacing(result, 5.0);
    EXPECT_EQ(result.degrees_of_freedom, 6);
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceIsInvariantUnderEndpointAndSelectionReversal) {
    auto model = parallel_spacing(3.0, true);
    std::swap(model.constraints.back().references[0], model.constraints.back().references[1]);
    assert_spacing(make_plane_gcs_sketch_solver()->solve(model), 3.0);
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceHandlesZeroAndCoincidentLines) {
    auto model = parallel_spacing(0.0);
    assert_spacing(make_plane_gcs_sketch_solver()->solve(model), 0.0);
    model.lines[1].start = model.lines[0].start;
    model.lines[1].end = model.lines[0].end;
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    assert_spacing(result, 0.0);
    EXPECT_EQ(result.degrees_of_freedom, 6);
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceRequiresVisibleParallelRelationship) {
    auto model = parallel_spacing(3.0);
    model.constraints.erase(model.constraints.begin());
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    EXPECT_EQ(result.status, SolveStatus::invalid_model);
    EXPECT_NE(result.diagnostic.find("explicit parallel"), std::string::npos);
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceAcceptsExistingHorizontalConstraints) {
    auto model = parallel_spacing(3.0);
    model.lines = {{"base", {10.0, 20.0}, {20.0, 20.0}}, {"offset", {12.0, 22.0}, {22.0, 22.0}}};
    model.constraints.erase(model.constraints.begin());
    model.constraints.push_back(
        {"h1", ConstraintKind::horizontal, {endpoint("base", SubElement::whole)}, {}});
    model.constraints.push_back(
        {"h2", ConstraintKind::horizontal, {endpoint("offset", SubElement::whole)}, {}});
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    assert_spacing(result, 3.0);
    EXPECT_EQ(result.degrees_of_freedom, 5);
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceDiagnosesIncompatibleAngle) {
    auto model = parallel_spacing(3.0);
    model.constraints.push_back(
        {"angle",
         ConstraintKind::angle,
         {endpoint("base", SubElement::whole), endpoint("offset", SubElement::whole)},
         {},
         30.0,
         "deg"});
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    EXPECT_EQ(result.status, SolveStatus::conflicting) << result.diagnostic;
    EXPECT_FALSE(result.conflicting_constraint_ids.empty());
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceRejectsNegativeAndSelfDimensions) {
    auto model = parallel_spacing(-3.0);
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
    model = parallel_spacing(3.0);
    model.constraints.back().references[1] = model.constraints.back().references[0];
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceSupportsScaleChangesAndCoincidentInitialGuess) {
    for (const double scale : {0.01, 1.0, 1000.0}) {
        auto model = parallel_spacing(3.0 * scale);
        for (auto& line : model.lines) {
            line.start.x *= scale;
            line.start.y *= scale;
            line.end.x *= scale;
            line.end.y *= scale;
        }
        assert_spacing(make_plane_gcs_sketch_solver()->solve(model), 3.0 * scale);
    }
    auto model = parallel_spacing(3.0);
    model.lines[1].start = model.lines[0].start;
    model.lines[1].end = model.lines[0].end;
    assert_spacing(make_plane_gcs_sketch_solver()->solve(model), 3.0);
}

TEST(PlaneGcsSketchSolver, ParallelLineDistanceSupportsIntrinsicAxisAndAxisParallelRelations) {
    auto model = parallel_spacing(3.0);
    model.lines = {{"base", {10.0, 20.0}, {20.0, 20.0}}, {"offset", {12.0, 22.0}, {22.0, 22.0}}};
    model.constraints.erase(model.constraints.begin());
    for (const char* id : {"base", "offset"})
        model.constraints.push_back(
            {std::string("axis-") + id,
             ConstraintKind::parallel,
             {endpoint(id, SubElement::whole), axis(GeometryTarget::sketch_x_axis)},
             {}});
    assert_spacing(make_plane_gcs_sketch_solver()->solve(model), 3.0);
    model.lines.erase(model.lines.begin() + 1);
    model.constraints.erase(model.constraints.end() - 1);
    model.constraints[0].references[1] = axis(GeometryTarget::sketch_x_axis);
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.lines.size(), 1U);
    EXPECT_NEAR(std::abs(result.lines[0].start.y), 3.0, 1e-8);
    EXPECT_NEAR(std::abs(result.lines[0].end.y), 3.0, 1e-8);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, EllipsePreservesExactParametersAndFiveDegreesOfFreedom) {
    SketchModel model;
    model.ellipses = {{"ellipse", {10.0, 20.0}, 5.0, 3.0, 0.7}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 5);
    ASSERT_EQ(result.ellipses.size(), 1U);
    EXPECT_NEAR(result.ellipses[0].major_radius, 5.0, 1e-9);
    EXPECT_NEAR(result.ellipses[0].minor_radius, 3.0, 1e-9);
    EXPECT_NEAR(result.ellipses[0].rotation, 0.7, 1e-9);
}

TEST(PlaneGcsSketchSolver, EllipseSolvesNativeMajorAndMinorAxisDimensions) {
    SketchModel model;
    model.ellipses = {{"ellipse", {10.0, 20.0}, 5.0, 3.0, 0.7}};
    model.constraints = {{"major",
                          ConstraintKind::major_radius,
                          {endpoint("ellipse", SubElement::whole)},
                          {},
                          8.0,
                          "mm"},
                         {"minor",
                          ConstraintKind::minor_radius,
                          {endpoint("ellipse", SubElement::whole)},
                          {},
                          2.0,
                          "mm"}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 3);
    ASSERT_EQ(result.ellipses.size(), 1U);
    EXPECT_NEAR(result.ellipses[0].major_radius, 8.0, 1e-8);
    EXPECT_NEAR(result.ellipses[0].minor_radius, 2.0, 1e-8);
}

TEST(PlaneGcsSketchSolver, EllipseSupportsPointOnObjectAndExactLineTangency) {
    SketchModel model;
    model.ellipses = {{"ellipse", {10.0, 20.0}, 5.0, 3.0, 0.0}};
    model.points = {{"point", {15.1, 20.0}}};
    model.lines = {{"line", {5.0, 23.1}, {15.0, 23.1}}};
    model.constraints = {
        {"fixed-ellipse",
         ConstraintKind::fixed,
         {endpoint("ellipse", SubElement::whole)},
         {},
         0.0,
         ""},
        {"on-ellipse",
         ConstraintKind::point_on_object,
         {endpoint("point", SubElement::point), endpoint("ellipse", SubElement::whole)},
         {},
         0.0,
         ""},
        {"horizontal",
         ConstraintKind::horizontal,
         {endpoint("line", SubElement::whole)},
         {},
         0.0,
         ""},
        {"tangent",
         ConstraintKind::tangent,
         {endpoint("line", SubElement::whole), endpoint("ellipse", SubElement::whole)},
         {},
         0.0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.points.size(), 1U);
    const double x = (result.points[0].point.x - 10.0) / 5.0;
    const double y = (result.points[0].point.y - 20.0) / 3.0;
    EXPECT_NEAR(x * x + y * y, 1.0, 1e-8);
    EXPECT_NEAR(result.lines[0].start.y, 23.0, 1e-8);
    EXPECT_NEAR(result.lines[0].end.y, 23.0, 1e-8);
}

TEST(PlaneGcsSketchSolver, EllipticalArcPreservesSevenDegreesOfFreedomAndEndpointConnection) {
    SketchModel model;
    model.elliptical_arcs = {{"arc", {10.0, 20.0}, 5.0, 3.0, 0.7, 5.5, 7.0}};
    auto solver = make_plane_gcs_sketch_solver();
    const auto initial = solver->solve(model);
    ASSERT_EQ(initial.status, SolveStatus::under_constrained) << initial.diagnostic;
    EXPECT_EQ(initial.degrees_of_freedom, 7);
    model.lines = {{"line", {13.0, 25.0}, {30.0, 30.0}}};
    model.constraints = {
        {"fixed-arc", ConstraintKind::fixed, {endpoint("arc", SubElement::whole)}, {}, 0.0, ""},
        {"connected",
         ConstraintKind::coincident,
         {endpoint("arc", SubElement::end), endpoint("line", SubElement::start)},
         {},
         0.0,
         ""}};
    const auto result = solver->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.elliptical_arcs.size(), 1U);
    ASSERT_EQ(result.lines.size(), 1U);
    const auto& a = result.elliptical_arcs[0];
    const double x = a.major_radius * std::cos(a.end_angle),
                 y = a.minor_radius * std::sin(a.end_angle);
    EXPECT_NEAR(result.lines[0].start.x,
                a.center.x + x * std::cos(a.rotation) - y * std::sin(a.rotation), 1e-8);
    EXPECT_NEAR(result.lines[0].start.y,
                a.center.y + x * std::sin(a.rotation) + y * std::cos(a.rotation), 1e-8);
}

TEST(PlaneGcsSketchSolver, EllipseRejectsCircularDegeneracyAndUnsupportedTangency) {
    SketchModel model;
    model.ellipses = {{"ellipse", {0.0, 0.0}, 3.0, 3.0, 0.0}};
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
    model.ellipses[0].major_radius = 5.0;
    model.circles = {{"circle", {10.0, 0.0}, 2.0}};
    model.constraints = {
        {"tangent",
         ConstraintKind::tangent,
         {endpoint("ellipse", SubElement::whole), endpoint("circle", SubElement::whole)},
         {},
         0.0,
         ""}};
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, EllipticalArcPointOnObjectUsesExactBoundedParameterCurve) {
    SketchModel model;
    model.elliptical_arcs = {{"arc", {0.0, 0.0}, 5.0, 3.0, 0.0, 0.0, 1.5}};
    model.points = {{"point", {3.1, 2.5}}};
    model.constraints = {
        {"fixed", ConstraintKind::fixed, {endpoint("arc", SubElement::whole)}, {}, 0.0, ""},
        {"on",
         ConstraintKind::point_on_object,
         {endpoint("point", SubElement::point), endpoint("arc", SubElement::whole)},
         {},
         0.0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.points.size(), 1U);
    const auto& p = result.points[0].point;
    EXPECT_NEAR(p.x * p.x / 25.0 + p.y * p.y / 9.0, 1.0, 1e-8);
    const double u = std::atan2(p.y / 3.0, p.x / 5.0);
    EXPECT_GE(u, 0.0);
    EXPECT_LE(u, 1.5);
}
TEST(PlaneGcsSketchSolver, EllipticalArcTangentRejectsSupportingEllipseContactOutsideRange) {
    SketchModel model;
    model.elliptical_arcs = {{"arc", {0.0, 0.0}, 5.0, 3.0, 0.0, 0.0, 1.0}};
    model.lines = {{"line", {-10.0, 3.0}, {10.0, 3.0}}};
    model.constraints = {
        {"fixed", ConstraintKind::fixed, {endpoint("arc", SubElement::whole)}, {}, 0.0, ""},
        {"tangent",
         ConstraintKind::tangent,
         {endpoint("line", SubElement::whole), endpoint("arc", SubElement::whole)},
         {},
         0.0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    EXPECT_EQ(result.status, SolveStatus::failed) << result.diagnostic;
    EXPECT_NE(result.diagnostic.find("outside elliptical arc"), std::string::npos);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, EllipseEqualAndMixedConcentricPreserveBothSemiaxes) {
    SketchModel model;
    model.ellipses = {{"reference", {10.0, 20.0}, 5.0, 3.0, 0.0},
                      {"equal", {0.0, 0.0}, 6.0, 2.0, 0.3}};
    model.circles = {{"circle", {9.0, 21.0}, 1.0}};
    model.constraints = {
        {"fixed", ConstraintKind::fixed, {endpoint("reference", SubElement::whole)}, {}, 0.0, ""},
        {"equal",
         ConstraintKind::equal,
         {endpoint("reference", SubElement::whole), endpoint("equal", SubElement::whole)},
         {},
         0.0,
         ""},
        {"concentric",
         ConstraintKind::concentric,
         {endpoint("reference", SubElement::whole), endpoint("circle", SubElement::whole)},
         {},
         0.0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    ASSERT_EQ(result.ellipses.size(), 2U);
    EXPECT_NEAR(result.ellipses[1].major_radius, 5.0, 1e-8);
    EXPECT_NEAR(result.ellipses[1].minor_radius, 3.0, 1e-8);
    ASSERT_EQ(result.circles.size(), 1U);
    EXPECT_NEAR(result.circles[0].center.x, 10.0, 1e-8);
    EXPECT_NEAR(result.circles[0].center.y, 20.0, 1e-8);
}

TEST(PlaneGcsSketchSolver, EllipticalArcSupportsReverseSweepAndCrossingPeriodicSeam) {
    SketchModel model;
    model.elliptical_arcs = {{"arc", {10.0, 20.0}, 5.0, 3.0, 0.7, 7.0, 5.5}};
    model.constraints = {
        {"fixed", ConstraintKind::fixed, {endpoint("arc", SubElement::whole)}, {}, 0.0, ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
    ASSERT_EQ(result.elliptical_arcs.size(), 1U);
    EXPECT_NEAR(result.elliptical_arcs[0].start_angle, 7.0, 1e-8);
    EXPECT_NEAR(result.elliptical_arcs[0].end_angle, 5.5, 1e-8);
    EXPECT_NEAR(result.elliptical_arcs[0].rotation, 0.7, 1e-8);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, LinkedMirrorLineUpdatesWithSourceAndMovedRotatedAxis) {
    for (const double offset : {0.0, 2.0}) {
        SketchModel model;
        model.lines = {{"source", {1, 2}, {3, 4}},
                       {"target", {-1, 2}, {-3, 4}},
                       {"axis", {offset, 0}, {offset + 1, 1}}};
        model.constraints = {
            {"fixed-source",
             ConstraintKind::fixed,
             {endpoint("source", SubElement::whole)},
             {},
             0,
             ""},
            {"fixed-axis", ConstraintKind::fixed, {endpoint("axis", SubElement::whole)}, {}, 0, ""},
            {"mirror",
             ConstraintKind::mirror,
             {endpoint("source", SubElement::whole), endpoint("axis", SubElement::direction),
              endpoint("target", SubElement::whole)},
             {},
             0,
             ""}};
        const auto result = make_plane_gcs_sketch_solver()->solve(model);
        ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
        EXPECT_NEAR(result.lines[1].start.x, 2 + offset, 1e-8);
        EXPECT_NEAR(result.lines[1].start.y, 1 - offset, 1e-8);
        EXPECT_NEAR(result.lines[1].end.x, 4 + offset, 1e-8);
        EXPECT_NEAR(result.lines[1].end.y, 3 - offset, 1e-8);
    }
}
TEST(PlaneGcsSketchSolver, LinkedMirrorArcPreservesTrueEndpointIdentityAndReverseSweep) {
    SketchModel model;
    model.arcs = {
        {"source", {3, 2}, 2, 0.2, 1.2},
        {"target", {-3, 2}, 2, 3.14159265358979323846 - 0.2, 3.14159265358979323846 - 1.2}};
    model.constraints = {
        {"fixed-source", ConstraintKind::fixed, {endpoint("source", SubElement::whole)}, {}, 0, ""},
        {"mirror",
         ConstraintKind::mirror,
         {endpoint("source", SubElement::whole), axis(GeometryTarget::sketch_y_axis),
          endpoint("target", SubElement::whole)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
    ASSERT_EQ(result.arcs.size(), 2U);
    const auto& a = result.arcs[0];
    const auto& b = result.arcs[1];
    EXPECT_NEAR(b.center.x, -a.center.x, 1e-8);
    EXPECT_NEAR(b.center.y, a.center.y, 1e-8);
    EXPECT_NEAR(b.radius, a.radius, 1e-8);
    EXPECT_NEAR(b.center.x + b.radius * std::cos(b.start_angle),
                -a.center.x - a.radius * std::cos(a.start_angle), 1e-8);
    EXPECT_NEAR(b.center.y + b.radius * std::sin(b.start_angle),
                a.center.y + a.radius * std::sin(a.start_angle), 1e-8);
    EXPECT_LT(b.end_angle - b.start_angle, 0);
}
TEST(PlaneGcsSketchSolver, LinkedMirrorEllipseUsesExactFocusRelation) {
    SketchModel model;
    model.ellipses = {{"source", {3, 2}, 5, 3, 0.2},
                      {"target", {-3, 2}, 5, 3, 3.14159265358979323846 - 0.2}};
    model.constraints = {
        {"fixed-source", ConstraintKind::fixed, {endpoint("source", SubElement::whole)}, {}, 0, ""},
        {"mirror",
         ConstraintKind::mirror,
         {endpoint("source", SubElement::whole), axis(GeometryTarget::sketch_y_axis),
          endpoint("target", SubElement::whole)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
    const auto& b = result.ellipses[1];
    EXPECT_NEAR(b.center.x, -3, 1e-8);
    EXPECT_NEAR(b.center.y, 2, 1e-8);
    EXPECT_NEAR(b.major_radius, 5, 1e-8);
    EXPECT_NEAR(b.minor_radius, 3, 1e-8);
    EXPECT_NEAR(std::cos(b.rotation), -std::cos(0.2), 1e-8);
}
TEST(PlaneGcsSketchSolver, LinkedMirrorSplineRejectsDifferentExactBasis) {
    SketchModel model;
    SplineEntity source;
    source.id = "source";
    source.mode = "CONTROL";
    source.degree = 3;
    source.poles = {{0, 0}, {1, 2}, {2, 2}, {3, 0}};
    source.knots = {0, 1};
    source.multiplicities = {4, 4};
    source.weights = {1, 1, 1, 1};
    source.parameter_end = 1;
    auto target = source;
    target.id = "target";
    target.weights[1] = 2;
    model.splines = {source, target};
    model.constraints = {
        {"mirror",
         ConstraintKind::mirror,
         {endpoint("source", SubElement::whole), axis(GeometryTarget::sketch_y_axis),
          endpoint("target", SubElement::whole)},
         {},
         0,
         ""}};
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, LinkedMirrorPointCircleEllipticalArcAndBothSplineModes) {
    SketchModel model;
    model.points = {{"point", {3, 2}}, {"point-copy", {-2, 1}}};
    model.circles = {{"circle", {4, 2}, 3}, {"circle-copy", {-4, 2}, 3}};
    model.elliptical_arcs = {{"arc", {3, 2}, 5, 3, 0.2, 0.3, 1.3},
                             {"arc-copy", {-3, 2}, 5, 3, 3.14159265358979323846 - 0.2, -0.3, -1.3}};
    SplineEntity fit;
    fit.id = "fit";
    fit.control_points = {{1, 1}, {2, 3}, {4, 3}, {5, 1}};
    auto fit_copy = fit;
    fit_copy.id = "fit-copy";
    for (auto& p : fit_copy.control_points)
        p.x = -p.x;
    SplineEntity control;
    control.id = "control";
    control.mode = "CONTROL";
    control.poles = fit.control_points;
    control.knots = {0, 1};
    control.multiplicities = {4, 4};
    control.weights = {1, 0.8, 0.8, 1};
    control.parameter_end = 1;
    auto control_copy = control;
    control_copy.id = "control-copy";
    for (auto& p : control_copy.poles)
        p.x = -p.x;
    model.splines = {fit, fit_copy, control, control_copy};
    for (const auto& ids :
         std::vector<std::pair<std::string, std::string>>{{"point", "point-copy"},
                                                          {"circle", "circle-copy"},
                                                          {"arc", "arc-copy"},
                                                          {"fit", "fit-copy"},
                                                          {"control", "control-copy"}}) {
        model.constraints.push_back({"fixed-" + ids.first,
                                     ConstraintKind::fixed,
                                     {{GeometryTarget::entity, ids.first, SubElement::whole}},
                                     {},
                                     0,
                                     ""});
        model.constraints.push_back({"mirror-" + ids.first,
                                     ConstraintKind::mirror,
                                     {{GeometryTarget::entity, ids.first, SubElement::whole},
                                      axis(GeometryTarget::sketch_y_axis),
                                      {GeometryTarget::entity, ids.second, SubElement::whole}},
                                     {},
                                     0,
                                     ""});
    }
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
    EXPECT_NEAR(result.points[1].point.x, -3, 1e-8);
    EXPECT_NEAR(result.points[1].point.y, 2, 1e-8);
    EXPECT_NEAR(result.circles[1].center.x, -4, 1e-8);
    EXPECT_NEAR(result.circles[1].radius, 3, 1e-8);
    const auto& a = result.elliptical_arcs[0];
    const auto& b = result.elliptical_arcs[1];
    const auto value = [](const EllipticalArcEntity& e, double u) {
        const double x = e.major_radius * std::cos(u), y = e.minor_radius * std::sin(u);
        return Vec2{e.center.x + x * std::cos(e.rotation) - y * std::sin(e.rotation),
                    e.center.y + x * std::sin(e.rotation) + y * std::cos(e.rotation)};
    };
    const auto first = value(a, a.start_angle), second = value(b, b.start_angle);
    EXPECT_NEAR(second.x, -first.x, 1e-8);
    EXPECT_NEAR(second.y, first.y, 1e-8);
    EXPECT_LT(b.end_angle - b.start_angle, 0);
    for (std::size_t i = 0; i < 4U; ++i) {
        EXPECT_NEAR(result.splines[1].control_points[i].x, -result.splines[0].control_points[i].x,
                    1e-8);
        EXPECT_NEAR(result.splines[1].control_points[i].y, result.splines[0].control_points[i].y,
                    1e-8);
        EXPECT_NEAR(result.splines[3].poles[i].x, -result.splines[2].poles[i].x, 1e-8);
        EXPECT_NEAR(result.splines[3].poles[i].y, result.splines[2].poles[i].y, 1e-8);
    }
}
TEST(PlaneGcsSketchSolver, LinkedMirrorReportsConflictInsteadOfOverwritingFixedTarget) {
    SketchModel model;
    model.points = {{"source", {3, 2}}, {"target", {3, 2}}};
    model.constraints = {
        {"source-fixed", ConstraintKind::fixed, {endpoint("source", SubElement::whole)}, {}, 0, ""},
        {"target-fixed", ConstraintKind::fixed, {endpoint("target", SubElement::whole)}, {}, 0, ""},
        {"mirror",
         ConstraintKind::mirror,
         {endpoint("source", SubElement::whole), axis(GeometryTarget::sketch_y_axis),
          endpoint("target", SubElement::whole)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    EXPECT_EQ(result.status, SolveStatus::conflicting) << result.diagnostic;
    EXPECT_FALSE(result.conflicting_constraint_ids.empty());
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, ProjectedDistanceIsSignedAndSupportsZeroReversalAndScale) {
    for (const double scale : {0.01, 1.0, 1000.0})
        for (const double value : {-3.0, 0.0, 3.0}) {
            SketchModel model;
            model.points = {{"first", {10 * scale, 20 * scale}},
                            {"second", {12 * scale, 24 * scale}}};
            model.constraints = {
                {"x",
                 ConstraintKind::horizontal_distance,
                 {endpoint("first", SubElement::point), endpoint("second", SubElement::point)},
                 {},
                 value * scale,
                 "mm"},
                {"y",
                 ConstraintKind::vertical_distance,
                 {endpoint("first", SubElement::point), endpoint("second", SubElement::point)},
                 {},
                 -2 * scale,
                 "mm"}};
            auto result = make_plane_gcs_sketch_solver()->solve(model);
            ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
            EXPECT_EQ(result.degrees_of_freedom, 2);
            EXPECT_NEAR(result.points[1].point.x - result.points[0].point.x, value * scale, 1e-8);
            EXPECT_NEAR(result.points[1].point.y - result.points[0].point.y, -2 * scale, 1e-8);
            for (auto& c : model.constraints) {
                std::swap(c.references[0], c.references[1]);
                c.value = -c.value;
            }
            result = make_plane_gcs_sketch_solver()->solve(model);
            ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
            EXPECT_NEAR(result.points[1].point.x - result.points[0].point.x, value * scale, 1e-8);
            EXPECT_NEAR(result.points[1].point.y - result.points[0].point.y, -2 * scale, 1e-8);
        }
}
TEST(PlaneGcsSketchSolver, ProjectedDistanceOriginCenterEndpointAndControlReferencesAreExplicit) {
    SketchModel model;
    model.lines = {{"line", {1, 2}, {3, 4}}};
    model.ellipses = {{"ellipse", {5, 6}, 5, 3, 0}};
    SplineEntity spline;
    spline.id = "spline";
    spline.control_points = {{1, 1}, {2, 3}, {4, 3}, {5, 1}};
    model.splines = {spline};
    const GeometryRef origin{GeometryTarget::sketch_origin, {}, SubElement::point};
    model.constraints = {{"line-x",
                          ConstraintKind::horizontal_distance,
                          {origin, endpoint("line", SubElement::end)},
                          {},
                          -7,
                          "mm"},
                         {"center-y",
                          ConstraintKind::vertical_distance,
                          {origin, endpoint("ellipse", SubElement::center)},
                          {},
                          8,
                          "mm"},
                         {"control-x",
                          ConstraintKind::horizontal_distance,
                          {origin, {GeometryTarget::entity, "spline", SubElement::control, 1}},
                          {},
                          9,
                          "mm"}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_NEAR(result.lines[0].end.x, -7, 1e-8);
    EXPECT_NEAR(result.ellipses[0].center.y, 8, 1e-8);
    EXPECT_NEAR(result.splines[0].control_points[1].x, 9, 1e-8);
}
TEST(PlaneGcsSketchSolver, CollinearUsesMinimalEquationsAndDeduplicatesParallel) {
    for (const bool parallel : {false, true}) {
        SketchModel model;
        model.lines = {{"first", {10, 20}, {16, 28}}, {"second", {8.4, 21.2}, {14.4, 29.2}}};
        const std::vector<GeometryRef> refs{endpoint("first", SubElement::whole),
                                            endpoint("second", SubElement::whole)};
        if (parallel)
            model.constraints.push_back({"parallel", ConstraintKind::parallel, refs, {}, 0, ""});
        model.constraints.push_back({"collinear", ConstraintKind::collinear, refs, {}, 0, ""});
        const auto result = make_plane_gcs_sketch_solver()->solve(model);
        assert_spacing(result, 0);
        EXPECT_EQ(result.degrees_of_freedom, 6);
    }
}
TEST(PlaneGcsSketchSolver, CollinearSupportsIntrinsicAxesAndRejectsAngleConflict) {
    SketchModel model;
    model.lines = {{"line", {10, 20}, {16, 28}}};
    model.constraints = {
        {"collinear",
         ConstraintKind::collinear,
         {endpoint("line", SubElement::whole), axis(GeometryTarget::sketch_x_axis)},
         {},
         0,
         ""}};
    auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 2);
    EXPECT_NEAR(result.lines[0].start.y, 0, 1e-8);
    EXPECT_NEAR(result.lines[0].end.y, 0, 1e-8);
    model.constraints.push_back(
        {"angle",
         ConstraintKind::angle,
         {endpoint("line", SubElement::whole), axis(GeometryTarget::sketch_x_axis)},
         {},
         30,
         "deg"});
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::conflicting);
    model.lines[0].end = model.lines[0].start;
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, SameSupportMaintainsEllipseRotationAndLeavesArcRangeFree) {
    SketchModel model;
    model.ellipses = {{"source", {10, 20}, 5, 3, 0.6}};
    model.elliptical_arcs = {{"target", {9, 19}, 6, 2, 0.4, 0.2, 1.2}};
    model.constraints = {
        {"fixed", ConstraintKind::fixed, {endpoint("source", SubElement::whole)}, {}, 0, ""},
        {"support",
         ConstraintKind::same_support,
         {endpoint("source", SubElement::whole), endpoint("target", SubElement::whole)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 2);
    const auto& target = result.elliptical_arcs[0];
    EXPECT_NEAR(target.center.x, 10, 1e-8);
    EXPECT_NEAR(target.center.y, 20, 1e-8);
    EXPECT_NEAR(target.major_radius, 5, 1e-8);
    EXPECT_NEAR(target.minor_radius, 3, 1e-8);
    EXPECT_NEAR(target.rotation, 0.6, 1e-8);
}
TEST(PlaneGcsSketchSolver, SameSupportMaintainsCircularSupportAndRejectsMixedKinds) {
    SketchModel model;
    model.circles = {{"source", {10, 20}, 5}};
    model.arcs = {{"target", {9, 19}, 6, 0.2, 1.2}};
    model.constraints = {
        {"fixed", ConstraintKind::fixed, {endpoint("source", SubElement::whole)}, {}, 0, ""},
        {"support",
         ConstraintKind::same_support,
         {endpoint("source", SubElement::whole), endpoint("target", SubElement::whole)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 2);
    EXPECT_NEAR(result.arcs[0].center.x, 10, 1e-8);
    EXPECT_NEAR(result.arcs[0].center.y, 20, 1e-8);
    EXPECT_NEAR(result.arcs[0].radius, 5, 1e-8);
    model.lines = {{"line", {0, 0}, {1, 1}}};
    model.constraints.back().references[1] = endpoint("line", SubElement::whole);
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, SameSupportCornerCompositeRemainsDrivenBySourceAndRadius) {
    for (const double radius : {1.0, 2.0}) {
        SketchModel model;
        model.lines = {{"source-a", {0, 0}, {10, 0}},
                       {"source-b", {0, 0}, {0, 8}},
                       {"cut-a", {1, 0}, {10, 0}},
                       {"cut-b", {0, 1}, {0, 8}}};
        model.arcs = {{"fillet", {1, 1}, 1, -3.14159265358979323846 / 2, -3.14159265358979323846}};
        model.constraints = {{"fixed-a",
                              ConstraintKind::fixed,
                              {endpoint("source-a", SubElement::whole)},
                              {},
                              0,
                              ""},
                             {"fixed-b",
                              ConstraintKind::fixed,
                              {endpoint("source-b", SubElement::whole)},
                              {},
                              0,
                              ""}};
        for (const auto pair : std::vector<std::pair<std::string, std::string>>{
                 {"source-a", "cut-a"}, {"source-b", "cut-b"}}) {
            const auto ref = [](const std::string& id, SubElement sub) {
                return GeometryRef{GeometryTarget::entity, id, sub};
            };
            model.constraints.push_back(
                {"support-" + pair.first,
                 ConstraintKind::same_support,
                 {ref(pair.first, SubElement::whole), ref(pair.second, SubElement::whole)},
                 {},
                 0,
                 "",
                 true});
            model.constraints.push_back(
                {"uncut-" + pair.first,
                 ConstraintKind::coincident,
                 {ref(pair.first, SubElement::end), ref(pair.second, SubElement::end)},
                 {},
                 0,
                 "",
                 true});
            model.constraints.push_back(
                {"on-" + pair.first,
                 ConstraintKind::point_on_object,
                 {ref(pair.second, SubElement::start), ref(pair.first, SubElement::whole)},
                 {},
                 0,
                 "",
                 true});
            model.constraints.push_back(
                {"tangent-" + pair.first,
                 ConstraintKind::tangent,
                 {ref(pair.first, SubElement::whole),
                  endpoint("fillet",
                           pair.first == "source-a" ? SubElement::start : SubElement::end)},
                 {},
                 0,
                 "",
                 true});
        }
        model.constraints.push_back(
            {"join-a",
             ConstraintKind::coincident,
             {endpoint("cut-a", SubElement::start), endpoint("fillet", SubElement::start)},
             {},
             0,
             "",
             true});
        model.constraints.push_back(
            {"join-b",
             ConstraintKind::coincident,
             {endpoint("cut-b", SubElement::start), endpoint("fillet", SubElement::end)},
             {},
             0,
             "",
             true});
        model.constraints.push_back({"radius",
                                     ConstraintKind::radius,
                                     {endpoint("fillet", SubElement::whole)},
                                     {},
                                     radius,
                                     "mm"});
        const auto result = make_plane_gcs_sketch_solver()->solve(model);
        ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
        EXPECT_EQ(result.degrees_of_freedom, 0);
        EXPECT_NEAR(result.arcs[0].radius, radius, 1e-8);
        EXPECT_NEAR(result.lines[2].start.x, radius, 1e-8);
        EXPECT_NEAR(result.lines[3].start.y, radius, 1e-8);
        EXPECT_NEAR(result.lines[2].end.x, 10, 1e-8);
        EXPECT_NEAR(result.lines[3].end.y, 8, 1e-8);
    }
}
TEST(PlaneGcsSketchSolver, SameSupportReverseArcDoesNotConstrainItsTrimRange) {
    SketchModel model;
    model.arcs = {{"source", {10, 20}, 5, 1.4, 0.2}, {"target", {9, 19}, 6, 1.1, 0.8}};
    model.constraints = {
        {"fixed", ConstraintKind::fixed, {endpoint("source", SubElement::whole)}, {}, 0, ""},
        {"support",
         ConstraintKind::same_support,
         {endpoint("source", SubElement::whole), endpoint("target", SubElement::whole)},
         {},
         0,
         "",
         true}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom, 2);
    EXPECT_NEAR(result.arcs[1].center.x, 10, 1e-8);
    EXPECT_NEAR(result.arcs[1].radius, 5, 1e-8);
    EXPECT_LT(result.arcs[1].end_angle - result.arcs[1].start_angle, 0);
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, LinkedMirrorSemicircleAxisEndpointsCloseWithoutSingularChord) {
    const double pi = std::acos(-1.0);
    for (const double radius : {2.0, 5.0, 1000.0}) {
        SketchModel model;
        model.arcs = {{"source", {0, 0}, radius, -pi / 2, pi / 2},
                      {"target", {0, 0}, radius, 3 * pi / 2, pi / 2}};
        model.constraints = {
            {"center",
             ConstraintKind::fixed_point,
             {endpoint("source", SubElement::center)},
             Vec2{0, 0},
             0,
             ""},
            {"radius",
             ConstraintKind::radius,
             {endpoint("source", SubElement::whole)},
             {},
             radius,
             ""},
            {"start-axis",
             ConstraintKind::point_on_object,
             {endpoint("source", SubElement::start), axis(GeometryTarget::sketch_y_axis)},
             {},
             0,
             ""},
            {"end-axis",
             ConstraintKind::point_on_object,
             {endpoint("source", SubElement::end), axis(GeometryTarget::sketch_y_axis)},
             {},
             0,
             ""},
            {"mirror",
             ConstraintKind::mirror,
             {endpoint("source", SubElement::whole), axis(GeometryTarget::sketch_y_axis),
              endpoint("target", SubElement::whole)},
             {},
             0,
             "",
             true},
            {"start-join",
             ConstraintKind::coincident,
             {endpoint("source", SubElement::start), endpoint("target", SubElement::start)},
             {},
             0,
             "",
             true},
            {"end-join",
             ConstraintKind::coincident,
             {endpoint("source", SubElement::end), endpoint("target", SubElement::end)},
             {},
             0,
             "",
             true}};
        const auto result = make_plane_gcs_sketch_solver()->solve(model);
        ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
        EXPECT_EQ(result.degrees_of_freedom, 0);
        ASSERT_EQ(result.arcs.size(), 2U);
        EXPECT_NEAR(result.arcs[1].radius, radius, 1e-7);
        EXPECT_NEAR(result.arcs[1].center.x, 0, 1e-7);
        EXPECT_LT(result.arcs[1].end_angle - result.arcs[1].start_angle, 0);
    }
}
}  // namespace occccad::geometry::sketch
namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, EllipticalArcEndpointTangentUsesAnalyticNormal) {
    const double pi = std::acos(-1.0);
    SketchModel model;
    model.elliptical_arcs = {{"ellipse", {0, 0}, 5, 2, 0, 0, pi / 2}};
    model.lines = {{"line", {2, 1}, {7, 3}}};
    model.constraints = {
        {"fixed-ellipse",
         ConstraintKind::fixed,
         {endpoint("ellipse", SubElement::whole)},
         {},
         0,
         ""},
        {"length", ConstraintKind::length, {endpoint("line", SubElement::whole)}, {}, 5, ""},
        {"join",
         ConstraintKind::coincident,
         {endpoint("line", SubElement::start), endpoint("ellipse", SubElement::end)},
         {},
         0,
         ""},
        {"tangent",
         ConstraintKind::tangent,
         {endpoint("line", SubElement::whole), endpoint("ellipse", SubElement::end)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::solved) << result.diagnostic;
    EXPECT_NEAR(result.lines[0].start.x, 0, 1e-8);
    EXPECT_NEAR(result.lines[0].start.y, 2, 1e-8);
    EXPECT_NEAR(result.lines[0].end.y, 2, 1e-8);
    EXPECT_NEAR(std::abs(result.lines[0].end.x), 5, 1e-8);
}
TEST(PlaneGcsSketchSolver, ControlSplineEndpointTangentIsExactPoleDerivative) {
    SketchModel model;
    SplineEntity spline;
    spline.id = "spline";
    spline.mode = "CONTROL";
    spline.degree = 3;
    spline.poles = {{0, 0}, {1, 1}, {2, 1}, {3, 0}};
    spline.knots = {0, 1};
    spline.multiplicities = {4, 4};
    spline.weights = {1, 2, 1, 1};
    spline.parameter_end = 1;
    model.splines = {spline};
    model.lines = {{"line", {0, 0}, {5, 0}}};
    model.constraints = {
        {"fixed-line", ConstraintKind::fixed, {endpoint("line", SubElement::whole)}, {}, 0, ""},
        {"fix-start",
         ConstraintKind::fixed_point,
         {{GeometryTarget::entity, "spline", SubElement::start}},
         {0, 0},
         0,
         ""},
        {"tangent",
         ConstraintKind::tangent,
         {endpoint("spline", SubElement::start), endpoint("line", SubElement::whole)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    // D1(0)=degree*w1/w0/(u_last-u_first)*(P1-P0), exact rational derivative.
    EXPECT_NEAR(6 * (result.splines[0].poles[1].y - result.splines[0].poles[0].y), 0, 1e-8);
    EXPECT_GT(std::abs(result.splines[0].poles[1].x - result.splines[0].poles[0].x), 1e-7);
    model.splines[0].mode = "FIT";
    model.splines[0].control_points = model.splines[0].poles;
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
}
TEST(PlaneGcsSketchSolver, SelfMirrorPointCircleAndLineTrackMovingAxisWithoutDuplicateGeometry) {
    SketchModel model;
    model.lines = {{"axis", {3, 1}, {3, 5}, EntityRole::construction},
                   {"on-axis", {3, 2}, {3, 4}},
                   {"paired", {1, 3}, {5, 3}}};
    model.points = {{"point", {3, 7}}};
    model.circles = {{"circle", {3, 3}, 2}};
    model.constraints = {
        {"fix-axis", ConstraintKind::fixed, {endpoint("axis", SubElement::whole)}, {}, 0, ""}};
    for (const auto& id : {"point", "circle", "on-axis", "paired"}) {
        SketchConstraint c;
        c.id = std::string("self-") + id;
        c.kind = ConstraintKind::mirror;
        c.references = {endpoint(id, SubElement::whole), endpoint("axis", SubElement::direction),
                        endpoint(id, SubElement::whole)};
        if (std::string(id) == "on-axis")
            c.self_mirror_mode = "ON_AXIS";
        if (std::string(id) == "paired")
            c.self_mirror_mode = "PAIRED";
        model.constraints.push_back(c);
    }
    model.lines[0].start.x = model.lines[0].end.x = 9;
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_NEAR(result.points[0].point.x, 9, 1e-8);
    EXPECT_NEAR(result.circles[0].center.x, 9, 1e-8);
    EXPECT_NEAR(result.lines[1].start.x, 9, 1e-8);
    EXPECT_NEAR(result.lines[1].end.x, 9, 1e-8);
    EXPECT_NEAR((result.lines[2].start.x + result.lines[2].end.x) / 2, 9, 1e-8);
    EXPECT_NEAR(result.lines[2].start.y, result.lines[2].end.y, 1e-8);
}
TEST(PlaneGcsSketchSolver, SelfMirrorEllipseArcAndSplineHaveExplicitExactCorrespondence) {
    SketchModel model;
    model.ellipses = {{"ellipse", {0, 0}, 5, 2, 0.3}};
    model.arcs = {{"arc", {0, 0}, 5, -0.2, 0.2}};
    SplineEntity spline;
    spline.id = "spline";
    spline.mode = "CONTROL";
    spline.degree = 2;
    spline.poles = {{-2, 0}, {0, 3}, {2, 0}};
    spline.knots = {0, 1};
    spline.multiplicities = {3, 3};
    spline.weights = {1, 2, 1};
    spline.parameter_end = 1;
    model.splines = {spline};
    for (const auto& id : {"ellipse", "arc", "spline"}) {
        SketchConstraint c;
        c.id = std::string("self-") + id;
        c.kind = ConstraintKind::mirror;
        c.references = {endpoint(id, SubElement::whole), axis(GeometryTarget::sketch_y_axis),
                        endpoint(id, SubElement::whole)};
        if (std::string(id) == "ellipse")
            c.self_mirror_mode = "MAJOR_PERPENDICULAR";
        if (std::string(id) == "spline")
            c.self_mirror_mode = "PAIRED";
        model.constraints.push_back(c);
    }
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::under_constrained) << result.diagnostic;
    EXPECT_NEAR(result.ellipses[0].center.x, 0, 1e-8);
    EXPECT_NEAR(std::sin(result.ellipses[0].rotation), 0, 1e-8);
    const auto& a = result.arcs[0];
    EXPECT_NEAR(a.radius * (std::cos(a.start_angle) + std::cos(a.end_angle)), 0, 1e-7);
    EXPECT_NEAR(result.splines[0].poles[0].x, -result.splines[0].poles[2].x, 1e-8);
    EXPECT_NEAR(result.splines[0].poles[1].x, 0, 1e-8);
    model.splines[0].weights = {1, 2, 3};
    EXPECT_EQ(make_plane_gcs_sketch_solver()->solve(model).status, SolveStatus::invalid_model);
}
}  // namespace occccad::geometry::sketch
namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, ReflectionNormalResidualAndAnalyticDerivativeAtZeroAndNonzeroChord) {
    for (const double chord : {0.0, 3.0}) {
        double coordinates[] = {2 - chord, 4, 2 + chord, 4, 2, 1, 2, 7};
        GCS::Point a{&coordinates[0], &coordinates[1]}, b{&coordinates[2], &coordinates[3]};
        GCS::Line axis;
        axis.p1 = {&coordinates[4], &coordinates[5]};
        axis.p2 = {&coordinates[6], &coordinates[7]};
        ReflectionNormalConstraint normal(a, b, axis);
        normal.rescale();
        EXPECT_TRUE(std::isfinite(normal.error()));
        EXPECT_NEAR(normal.error(), 0, 1e-12);
        for (auto& parameter : coordinates) {
            const double original = parameter, h = 1e-6;
            const double exact = normal.grad(&parameter);
            parameter = original + h;
            const double plus = normal.error();
            parameter = original - h;
            const double minus = normal.error();
            parameter = original;
            EXPECT_TRUE(std::isfinite(exact));
            EXPECT_NEAR(exact, (plus - minus) / (2 * h), 1e-8);
        }
        coordinates[3] += 2;
        EXPECT_NEAR(std::abs(normal.error()), 2, 1e-12);
    }
}
}  // namespace occccad::geometry::sketch
namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, RedundantDisconnectedComponentPreservesAcceptedDrivingSolution) {
    SketchModel model;
    model.lines = {{"driven", {0, 0}, {2, 0}}, {"redundant", {10, 3}, {15, 3}}};
    model.constraints = {
        {"origin",
         ConstraintKind::fixed_point,
         {endpoint("driven", SubElement::start)},
         {0, 0},
         0,
         ""},
        {"direction",
         ConstraintKind::horizontal,
         {endpoint("driven", SubElement::whole)},
         {},
         0,
         ""},
        {"length", ConstraintKind::length, {endpoint("driven", SubElement::whole)}, {}, 7, ""},
        {"horizontal-first",
         ConstraintKind::horizontal,
         {endpoint("redundant", SubElement::whole)},
         {},
         0,
         ""},
        {"horizontal-duplicate",
         ConstraintKind::horizontal,
         {endpoint("redundant", SubElement::whole)},
         {},
         0,
         ""}};
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_EQ(result.status, SolveStatus::redundant) << result.diagnostic;
    EXPECT_FALSE(result.redundant_constraint_ids.empty());
    EXPECT_TRUE(result.conflicting_constraint_ids.empty());
    ASSERT_EQ(result.lines.size(), 2U);
    EXPECT_NEAR(result.lines[0].start.x, 0, 1e-8);
    EXPECT_NEAR(result.lines[0].start.y, 0, 1e-8);
    EXPECT_NEAR(std::hypot(result.lines[0].end.x - result.lines[0].start.x,
                           result.lines[0].end.y - result.lines[0].start.y),
                7, 1e-8);
    EXPECT_NEAR(result.lines[0].end.y, 0, 1e-8);
    EXPECT_EQ(result.degrees_of_freedom, 3);
}
}  // namespace occccad::geometry::sketch
namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, LineArcAndArcArcCornerRadiusUpdatesWithFixedSourceSupports) {
    const double pi = std::acos(-1.0);
    for (const bool two_arcs : {false, true}) {
        const double old_radius = two_arcs ? 0.5 : 1;
        const double radius = old_radius * 1.5;
        SketchModel model;
        const double alpha = std::acos(0.6),
                     height = std::sqrt((5 - old_radius) * (5 - old_radius) - 9);
        if (two_arcs)
            model.arcs = {{"source-a", {-3, 0}, 5, -alpha, alpha},
                          {"source-b", {3, 0}, 5, pi - alpha, pi + alpha}};
        else {
            model.lines = {{"source-a", {-5, 0}, {5, 0}}};
            model.arcs = {{"source-b", {0, 0}, 5, 0, pi}};
        }
        Vec2 center =
            two_arcs ? Vec2{0, height} : Vec2{std::sqrt(25 - 10 * old_radius), old_radius};
        const double beta =
            two_arcs ? std::atan2(height, 3) : std::asin(old_radius / (5 - old_radius));
        if (two_arcs) {
            model.arcs.push_back({"cut-a", {-3, 0}, 5, -alpha, beta});
            model.arcs.push_back({"cut-b", {3, 0}, 5, pi - beta, pi + alpha});
        } else {
            model.lines.push_back({"cut-a", {-5, 0}, {center.x, 0}});
            model.arcs.push_back({"cut-b", {0, 0}, 5, beta, pi});
        }
        model.arcs.push_back(
            {"fillet", center, old_radius, two_arcs ? beta : -pi / 2, two_arcs ? pi - beta : beta});
        for (const auto source : {"source-a", "source-b"})
            model.constraints.push_back({std::string("fixed-") + source,
                                         ConstraintKind::fixed,
                                         {endpoint(source, SubElement::whole)},
                                         {},
                                         0,
                                         ""});
        for (const auto pair : std::vector<std::pair<const char*, const char*>>{
                 {"source-a", "cut-a"}, {"source-b", "cut-b"}}) {
            const bool first = std::string(pair.first) == "source-a";
            model.constraints.push_back({std::string("support-") + pair.first,
                                         ConstraintKind::same_support,
                                         {endpoint(pair.first, SubElement::whole),
                                          endpoint(pair.second, SubElement::whole)},
                                         {},
                                         0,
                                         "",
                                         true});
            model.constraints.push_back(
                {std::string("uncut-") + pair.first,
                 ConstraintKind::coincident,
                 {endpoint(pair.first, first ? SubElement::start : SubElement::end),
                  endpoint(pair.second, first ? SubElement::start : SubElement::end)},
                 {},
                 0,
                 "",
                 true});
            model.constraints.push_back(
                {std::string("on-") + pair.first,
                 ConstraintKind::point_on_object,
                 {endpoint(pair.second, first ? SubElement::end : SubElement::start),
                  endpoint(pair.first, SubElement::whole)},
                 {},
                 0,
                 "",
                 true});
            model.constraints.push_back(
                {std::string("tangent-") + pair.first,
                 ConstraintKind::tangent,
                 {endpoint(pair.first, SubElement::whole),
                  endpoint("fillet", first ? SubElement::start : SubElement::end)},
                 {},
                 0,
                 "",
                 true});
            model.constraints.push_back(
                {std::string("join-") + pair.first,
                 ConstraintKind::coincident,
                 {endpoint(pair.second, first ? SubElement::end : SubElement::start),
                  endpoint("fillet", first ? SubElement::start : SubElement::end)},
                 {},
                 0,
                 "",
                 true});
        }
        model.constraints.push_back(
            {"old-join-upper",
             ConstraintKind::coincident,
             {endpoint("source-a", SubElement::end), endpoint("source-b", SubElement::start)},
             {},
             0,
             ""});
        model.constraints.push_back(
            {"old-join-lower",
             ConstraintKind::coincident,
             {endpoint("source-b", SubElement::end), endpoint("source-a", SubElement::start)},
             {},
             0,
             ""});
        model.constraints.push_back({"radius",
                                     ConstraintKind::radius,
                                     {endpoint("fillet", SubElement::whole)},
                                     {},
                                     radius,
                                     ""});
        const auto result = make_plane_gcs_sketch_solver()->solve(model);
        ASSERT_TRUE(result.status == SolveStatus::solved || result.status == SolveStatus::redundant)
            << two_arcs << " " << result.diagnostic;
        EXPECT_EQ(result.degrees_of_freedom, 0);
        EXPECT_EQ(result.redundant_constraint_ids.size(), 2U);
        ASSERT_FALSE(result.arcs.empty());
        const auto& fillet = result.arcs.back();
        EXPECT_NEAR(fillet.radius, radius, 1e-8);
        if (two_arcs) {
            EXPECT_NEAR(fillet.center.x, 0, 1e-8);
            EXPECT_NEAR(fillet.center.y, std::sqrt((5 - radius) * (5 - radius) - 9), 1e-8);
        } else {
            EXPECT_NEAR(fillet.center.y, radius, 1e-8);
            EXPECT_NEAR(fillet.center.x, std::sqrt(25 - 10 * radius), 1e-8);
        }
    }
}
}  // namespace occccad::geometry::sketch
namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver,
     FixedEndpointCoincidenceNormalizationPreservesTrueConflictAndDefinition) {
    for (const double gap : {0.0, 1.0}) {
        SketchModel model;
        model.lines = {{"a", {0, 0}, {3, 0}}, {"b", {3 + gap, 0}, {6, 2}}};
        model.constraints = {
            {"fixed-a", ConstraintKind::fixed, {endpoint("a", SubElement::whole)}, {}, 0, ""},
            {"fixed-b", ConstraintKind::fixed, {endpoint("b", SubElement::whole)}, {}, 0, ""},
            {"join",
             ConstraintKind::coincident,
             {endpoint("a", SubElement::end), endpoint("b", SubElement::start)},
             {},
             0,
             ""}};
        const auto result = make_plane_gcs_sketch_solver()->solve(model);
        if (gap == 0) {
            ASSERT_EQ(result.status, SolveStatus::redundant) << result.diagnostic;
            EXPECT_EQ(result.degrees_of_freedom, 0);
            EXPECT_EQ(result.redundant_constraint_ids, std::vector<std::string>{"join"});
            EXPECT_TRUE(result.conflicting_constraint_ids.empty());
            ASSERT_EQ(result.lines.size(), 2U);
            EXPECT_NEAR(result.lines[0].end.x, 3, 1e-12);
            EXPECT_NEAR(result.lines[1].start.x, 3, 1e-12);
            // A separate genuinely inconsistent driver is still diagnosed even
            // when the existing fixed-endpoint relation has been proven redundant.
            model.constraints.push_back({"bad-length",
                                         ConstraintKind::length,
                                         {endpoint("a", SubElement::whole)},
                                         {},
                                         4,
                                         ""});
            const auto conflict = make_plane_gcs_sketch_solver()->solve(model);
            EXPECT_EQ(conflict.status, SolveStatus::conflicting);
            EXPECT_FALSE(conflict.conflicting_constraint_ids.empty());
        } else {
            EXPECT_EQ(result.status, SolveStatus::conflicting);
            EXPECT_FALSE(result.conflicting_constraint_ids.empty());
        }
        EXPECT_EQ(model.constraints[2].id, "join");
        EXPECT_EQ(model.constraints[2].references[0].entity_id, "a");
    }
}

TEST(PlaneGcsSketchSolver, EndpointDistanceIsNotParallelSpacingContinuation) {
    SketchModel model;
    model.lines = {{"a", {0,0}, {10,0}}, {"b", {12,5}, {20,5}}};
    model.constraints = {
        {"fixed-a", ConstraintKind::fixed, {endpoint("a",SubElement::whole)}},
        {"fixed-b", ConstraintKind::fixed, {endpoint("b",SubElement::whole)}},
        {"point-distance", ConstraintKind::distance,
            {endpoint("a",SubElement::start),endpoint("b",SubElement::start)}, {}, 5, "mm"}};
    const auto solver = make_plane_gcs_sketch_solver();
    EXPECT_EQ(solver->solve(model).status, SolveStatus::conflicting);
    model.constraints.back().value = 13;
    const auto result = solver->solve(model);
    ASSERT_TRUE(result.status == SolveStatus::solved || result.status == SolveStatus::redundant);
    EXPECT_NEAR(std::hypot(result.lines[1].start.x-result.lines[0].start.x,
                         result.lines[1].start.y-result.lines[0].start.y),13,1e-8);
}

TEST(PlaneGcsSketchSolver, FreeRoundedRectangleLargeVisibleDimensionEditsPreserveResiduals) {
    SketchModel model;
    model.lines.push_back({"line0", {10,10}, {130,10}});
    model.lines.push_back({"line1", {130,10}, {130,80}});
    model.lines.push_back({"line2", {130,80}, {10,80}});
    model.lines.push_back({"line3", {10,80}, {10,10}});
    model.lines.push_back({"line4", {15,10}, {125,10}});
    model.lines.push_back({"line5", {130,15}, {130,75}});
    model.arcs.push_back({"arc6", {125,15}, 5, -1.5707963267948966, 0});
    model.lines.push_back({"line7", {125,80}, {15,80}});
    model.arcs.push_back({"arc8", {125,75}, 5, 1.5707963267948966, 0});
    model.lines.push_back({"line9", {10,75}, {10,15}});
    model.arcs.push_back({"arc10", {15,75}, 5, 1.5707963267948966, 3.141592653589793});
    model.arcs.push_back({"arc11", {15,15}, 5, -1.5707963267948966, -3.141592653589793});
    model.constraints.push_back({"coincident0", ConstraintKind::coincident, {endpoint("line0", SubElement::end), endpoint("line1", SubElement::start)}, {}, 0, "", false});
    model.constraints.push_back({"coincident1", ConstraintKind::coincident, {endpoint("line1", SubElement::end), endpoint("line2", SubElement::start)}, {}, 0, "", false});
    model.constraints.push_back({"coincident2", ConstraintKind::coincident, {endpoint("line2", SubElement::end), endpoint("line3", SubElement::start)}, {}, 0, "", false});
    model.constraints.push_back({"coincident3", ConstraintKind::coincident, {endpoint("line3", SubElement::end), endpoint("line0", SubElement::start)}, {}, 0, "", false});
    model.constraints.push_back({"parallel4", ConstraintKind::parallel, {endpoint("line0", SubElement::direction), axis(GeometryTarget::sketch_x_axis)}, {}, 0, "", false});
    model.constraints.push_back({"parallel5", ConstraintKind::parallel, {endpoint("line1", SubElement::direction), axis(GeometryTarget::sketch_y_axis)}, {}, 0, "", false});
    model.constraints.push_back({"parallel6", ConstraintKind::parallel, {endpoint("line2", SubElement::direction), axis(GeometryTarget::sketch_x_axis)}, {}, 0, "", false});
    model.constraints.push_back({"parallel7", ConstraintKind::parallel, {endpoint("line3", SubElement::direction), axis(GeometryTarget::sketch_y_axis)}, {}, 0, "", false});
    model.constraints.push_back({"same_support8", ConstraintKind::same_support, {endpoint("line0", SubElement::whole), endpoint("line4", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object9", ConstraintKind::point_on_object, {endpoint("line4", SubElement::end), endpoint("line0", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"same_support10", ConstraintKind::same_support, {endpoint("line1", SubElement::whole), endpoint("line5", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object11", ConstraintKind::point_on_object, {endpoint("line5", SubElement::start), endpoint("line1", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"tangent12", ConstraintKind::tangent, {endpoint("line0", SubElement::whole), endpoint("arc6", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"tangent13", ConstraintKind::tangent, {endpoint("line1", SubElement::whole), endpoint("arc6", SubElement::end)}, {}, 0, "", true});
    model.constraints.push_back({"radius14", ConstraintKind::radius, {endpoint("arc6", SubElement::whole)}, {}, 5, "mm", false});
    model.constraints.push_back({"coincident15", ConstraintKind::coincident, {endpoint("line4", SubElement::end), endpoint("arc6", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"coincident16", ConstraintKind::coincident, {endpoint("line5", SubElement::start), endpoint("arc6", SubElement::end)}, {}, 0, "", true});
    model.constraints.push_back({"same_support17", ConstraintKind::same_support, {endpoint("line2", SubElement::whole), endpoint("line7", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object18", ConstraintKind::point_on_object, {endpoint("line7", SubElement::start), endpoint("line2", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object19", ConstraintKind::point_on_object, {endpoint("line5", SubElement::end), endpoint("line1", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"tangent20", ConstraintKind::tangent, {endpoint("line2", SubElement::whole), endpoint("arc8", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"tangent21", ConstraintKind::tangent, {endpoint("line1", SubElement::whole), endpoint("arc8", SubElement::end)}, {}, 0, "", true});
    model.constraints.push_back({"radius22", ConstraintKind::radius, {endpoint("arc8", SubElement::whole)}, {}, 5, "mm", false});
    model.constraints.push_back({"coincident23", ConstraintKind::coincident, {endpoint("line7", SubElement::start), endpoint("arc8", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"coincident24", ConstraintKind::coincident, {endpoint("line5", SubElement::end), endpoint("arc8", SubElement::end)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object25", ConstraintKind::point_on_object, {endpoint("line7", SubElement::end), endpoint("line2", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"same_support26", ConstraintKind::same_support, {endpoint("line3", SubElement::whole), endpoint("line9", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object27", ConstraintKind::point_on_object, {endpoint("line9", SubElement::start), endpoint("line3", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"tangent28", ConstraintKind::tangent, {endpoint("line2", SubElement::whole), endpoint("arc10", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"tangent29", ConstraintKind::tangent, {endpoint("line3", SubElement::whole), endpoint("arc10", SubElement::end)}, {}, 0, "", true});
    model.constraints.push_back({"radius30", ConstraintKind::radius, {endpoint("arc10", SubElement::whole)}, {}, 5, "mm", false});
    model.constraints.push_back({"coincident31", ConstraintKind::coincident, {endpoint("line7", SubElement::end), endpoint("arc10", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"coincident32", ConstraintKind::coincident, {endpoint("line9", SubElement::start), endpoint("arc10", SubElement::end)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object33", ConstraintKind::point_on_object, {endpoint("line4", SubElement::start), endpoint("line0", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"point_on_object34", ConstraintKind::point_on_object, {endpoint("line9", SubElement::end), endpoint("line3", SubElement::whole)}, {}, 0, "", true});
    model.constraints.push_back({"tangent35", ConstraintKind::tangent, {endpoint("line0", SubElement::whole), endpoint("arc11", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"tangent36", ConstraintKind::tangent, {endpoint("line3", SubElement::whole), endpoint("arc11", SubElement::end)}, {}, 0, "", true});
    model.constraints.push_back({"radius37", ConstraintKind::radius, {endpoint("arc11", SubElement::whole)}, {}, 5, "mm", false});
    model.constraints.push_back({"coincident38", ConstraintKind::coincident, {endpoint("line4", SubElement::start), endpoint("arc11", SubElement::start)}, {}, 0, "", true});
    model.constraints.push_back({"coincident39", ConstraintKind::coincident, {endpoint("line9", SubElement::end), endpoint("arc11", SubElement::end)}, {}, 0, "", true});
    // Match the production corner macro: SAME_SUPPORT owns support membership;
    // endpoint tangency uses the trimmed curve already joined to the fillet.
    model.constraints.erase(std::remove_if(model.constraints.begin(), model.constraints.end(),
        [](const auto& c) { return c.internal && c.kind == ConstraintKind::point_on_object; }),
        model.constraints.end());
    for (auto& c : model.constraints) if (c.kind == ConstraintKind::tangent) {
        for (auto& ref : c.references) {
            if (ref.entity_id == "line0") ref.entity_id = "line4";
            else if (ref.entity_id == "line1") ref.entity_id = "line5";
            else if (ref.entity_id == "line2") ref.entity_id = "line7";
            else if (ref.entity_id == "line3") ref.entity_id = "line9";
        }
    }
    model.constraints.push_back({"length40", ConstraintKind::length, {endpoint("line4", SubElement::whole)}, {}, 80, "mm", false});
    const auto solver = make_plane_gcs_sketch_solver();
    for (double target : {80.0, 140.0, 80.0}) {
        model.constraints.back().value = target;
        const auto result = solver->solve(model);
        ASSERT_TRUE(result.status == SolveStatus::under_constrained || result.status == SolveStatus::redundant) << result.diagnostic;
        for (const auto& line : result.lines)
            if (line.id == "line4")
                EXPECT_NEAR(std::hypot(line.end.x-line.start.x, line.end.y-line.start.y), target, 1e-7);
        for (const auto& arc : result.arcs) {
            EXPECT_NEAR(arc.radius, 5, 1e-8);
            EXPECT_NEAR(std::abs(arc.end_angle-arc.start_angle), std::acos(-1.0)/2, 1e-7);
        }
        model.lines = result.lines;
        model.arcs = result.arcs;
    }
    model.constraints.push_back({"visible-parallel", ConstraintKind::parallel,
        {endpoint("line4", SubElement::direction), endpoint("line7", SubElement::direction)}, {}, 0, "", true});
    model.constraints.push_back({"visible-distance", ConstraintKind::distance,
        {endpoint("line4", SubElement::whole), endpoint("line7", SubElement::whole)}, {}, 50, "mm"});
    for (double target : {50.0, 100.0, 50.0}) {
        model.constraints.back().value = target;
        const auto result = solver->solve(model);
        ASSERT_TRUE(result.status == SolveStatus::under_constrained || result.status == SolveStatus::redundant) << result.diagnostic;
        const LineEntity* a = nullptr;
        const LineEntity* b = nullptr;
        for (const auto& line : result.lines) {
            if (line.id == "line4") a = &line;
            if (line.id == "line7") b = &line;
        }
        ASSERT_NE(a, nullptr);
        ASSERT_NE(b, nullptr);
        EXPECT_NEAR(std::abs(a->start.y-b->start.y), target, 1e-7);
        for (const auto& constraint : model.constraints) {
            if (constraint.kind != ConstraintKind::coincident) continue;
            const auto point = [&](const GeometryRef& ref) {
                for (const auto& line : result.lines)
                    if (line.id == ref.entity_id) return ref.sub_element == SubElement::start ? line.start : line.end;
                for (const auto& arc : result.arcs) {
                    if (arc.id != ref.entity_id) continue;
                    const double angle = ref.sub_element == SubElement::start ? arc.start_angle : arc.end_angle;
                    return Vec2{arc.center.x+arc.radius*std::cos(angle),arc.center.y+arc.radius*std::sin(angle)};
                }
                return Vec2{};
            };
            const auto first = point(constraint.references[0]), second = point(constraint.references[1]);
            EXPECT_NEAR(std::hypot(first.x-second.x,first.y-second.y), 0, 1e-7);
        }
        model.lines = result.lines;
        model.arcs = result.arcs;
    }
}
}  // namespace occccad::geometry::sketch

namespace occccad::geometry::sketch {
TEST(PlaneGcsSketchSolver, ParallelOnRoundedFreeQuadrilateralDoesNotCollapseTrimmedEdge) {
    SketchModel model;
    model.lines = {
        {"line-1", {-110, 70}, {47.16450811081129, 96.1430357643463}},
        {"line-2", {47.16450811081129, 96.1430357643463}, {196.88158770359186, -31.563940043389}},
        {"line-3", {196.88158770359186, -31.563940043389}, {-196.88158770359198, -99.77107484979308}},
        {"line-4", {-196.88158770359198, -99.77107484979308}, {-110, 70}},
        {"line-5", {-110, 70}, {5.852383217069605, 89.27110028997862}},
        {"line-6", {79.02731173985183, 68.96442459127137}, {196.88158770359186, -31.563940043389}},
    };
    model.arcs = {{"fillet", {20.62023385854509, 0.4909753326939956}, 90, 1.7356289542597216, 0.8645697301037458}};
    model.constraints = {
        {"relation-0", ConstraintKind::coincident, {endpoint("line-1", SubElement::end), endpoint("line-2", SubElement::start)}, {}, 0, "", true},
        {"relation-1", ConstraintKind::coincident, {endpoint("line-2", SubElement::end), endpoint("line-3", SubElement::start)}, {}, 0, "", true},
        {"relation-2", ConstraintKind::coincident, {endpoint("line-3", SubElement::end), endpoint("line-4", SubElement::start)}, {}, 0, "", true},
        {"relation-3", ConstraintKind::coincident, {endpoint("line-4", SubElement::end), endpoint("line-1", SubElement::start)}, {}, 0, "", true},
        {"relation-4", ConstraintKind::same_support, {endpoint("line-1", SubElement::whole), endpoint("line-5", SubElement::whole)}, {}, 0, "", true},
        {"relation-5", ConstraintKind::coincident, {endpoint("line-5", SubElement::start), endpoint("line-1", SubElement::start)}, {}, 0, "", true},
        {"relation-6", ConstraintKind::same_support, {endpoint("line-2", SubElement::whole), endpoint("line-6", SubElement::whole)}, {}, 0, "", true},
        {"relation-7", ConstraintKind::coincident, {endpoint("line-6", SubElement::end), endpoint("line-2", SubElement::end)}, {}, 0, "", true},
        {"relation-8", ConstraintKind::tangent, {endpoint("line-5", SubElement::whole), endpoint("fillet", SubElement::start)}, {}, 0, "", true},
        {"relation-9", ConstraintKind::tangent, {endpoint("line-6", SubElement::whole), endpoint("fillet", SubElement::end)}, {}, 0, "", true},
        {"relation-10", ConstraintKind::radius, {endpoint("fillet", SubElement::whole)}, {}, 90, "", false},
        {"relation-11", ConstraintKind::coincident, {endpoint("line-5", SubElement::end), endpoint("fillet", SubElement::start)}, {}, 0, "", true},
        {"relation-12", ConstraintKind::coincident, {endpoint("line-6", SubElement::start), endpoint("fillet", SubElement::end)}, {}, 0, "", true},
        {"visible-parallel", ConstraintKind::parallel, {endpoint("line-6", SubElement::whole), endpoint("line-4", SubElement::whole)}, {}},
    };
    const auto result = make_plane_gcs_sketch_solver()->solve(model);
    ASSERT_TRUE(result.status == SolveStatus::under_constrained || result.status == SolveStatus::solved || result.status == SolveStatus::redundant) << result.diagnostic;
    ASSERT_EQ(result.lines.size(), 6U);
    const auto& child = result.lines[5]; const auto& other = result.lines[3]; const auto& source = result.lines[1];
    const double dx = child.end.x-child.start.x, dy = child.end.y-child.start.y;
    const double ox = other.end.x-other.start.x, oy = other.end.y-other.start.y;
    ASSERT_GT(std::hypot(dx,dy), 1.0);
    EXPECT_NEAR((dx*oy-dy*ox)/(std::hypot(dx,dy)*std::hypot(ox,oy)), 0, 1e-8);
    const double sx = source.end.x-source.start.x, sy = source.end.y-source.start.y;
    const double t = ((child.start.x-source.start.x)*sx+(child.start.y-source.start.y)*sy)/(sx*sx+sy*sy);
    EXPECT_GT(t, 0); EXPECT_LT(t, 1);
    ASSERT_EQ(result.arcs.size(), 1U); EXPECT_NEAR(result.arcs[0].radius, 90, 1e-8);
    const auto& arc = result.arcs[0];
    EXPECT_NEAR(child.start.x, arc.center.x+90*std::cos(arc.end_angle), 1e-7);
    EXPECT_NEAR(child.start.y, arc.center.y+90*std::sin(arc.end_angle), 1e-7);
    EXPECT_NEAR((arc.center.x-child.start.x)*dx+(arc.center.y-child.start.y)*dy, 0, 1e-6);
}

TEST(PlaneGcsSketchSolver, DragObjectivesPreserveConnectedGeometryAndFormalDof) {
    SketchModel model;
    model.points={{"junction",{10,0}}};
    model.lines={{"source",{0,0},{10,0}},{"neighbor",{10,0},{10,10}}};
    model.constraints={
        {"a",ConstraintKind::coincident,{endpoint("source",SubElement::end),endpoint("junction",SubElement::point)},{}},
        {"b",ConstraintKind::coincident,{endpoint("neighbor",SubElement::start),endpoint("junction",SubElement::point)},{}},
        {"length",ConstraintKind::distance,{endpoint("source",SubElement::start),endpoint("source",SubElement::end)}, {},10}
    };
    auto solver=make_plane_gcs_sketch_solver();const auto baseline=solver->solve(model);
    for(const double angle:{0.0,0.5,1.2}) {
        model.drag_targets={{endpoint("source",SubElement::start),{3,4}},
            {endpoint("source",SubElement::end),{3+10*std::cos(angle),4+10*std::sin(angle)}}};
        const auto result=solver->solve(model);
        ASSERT_EQ(result.status,SolveStatus::under_constrained)<<result.diagnostic;
        EXPECT_EQ(result.degrees_of_freedom,baseline.degrees_of_freedom);
        ASSERT_EQ(result.lines.size(),2U);ASSERT_EQ(result.points.size(),1U);
        EXPECT_NEAR(result.lines[0].start.x,3,1e-6);EXPECT_NEAR(result.lines[0].start.y,4,1e-6);
        EXPECT_NEAR(result.lines[0].end.x,3+10*std::cos(angle),1e-6);
        EXPECT_NEAR(result.lines[0].end.y,4+10*std::sin(angle),1e-6);
        EXPECT_NEAR(result.lines[1].start.x,result.lines[0].end.x,1e-7);
        EXPECT_NEAR(result.lines[1].start.y,result.lines[0].end.y,1e-7);
        EXPECT_NEAR(result.points[0].point.x,result.lines[0].end.x,1e-7);
        EXPECT_TRUE(result.conflicting_constraint_ids.empty());
    }
}
TEST(PlaneGcsSketchSolver, DragObjectivesRespectFixedAndDirectionConstraints) {
    SketchModel model;model.lines={{"source",{0,0},{10,0}}};
    model.constraints={{"fixed",ConstraintKind::fixed,{endpoint("source",SubElement::whole)},{}}};
    model.drag_targets={{endpoint("source",SubElement::start),{3,4}},{endpoint("source",SubElement::end),{3,14}}};
    auto solver=make_plane_gcs_sketch_solver();auto result=solver->solve(model);
    ASSERT_EQ(result.status,SolveStatus::solved)<<result.diagnostic;
    EXPECT_EQ(result.degrees_of_freedom,0);EXPECT_NEAR(result.lines[0].start.x,0,1e-8);EXPECT_NEAR(result.lines[0].end.x,10,1e-8);
    model.drag_targets[1].point={3+10*std::cos(0.5),4+10*std::sin(0.5)};
    model.constraints={{"horizontal",ConstraintKind::horizontal,{endpoint("source",SubElement::whole)},{}}};
    result=solver->solve(model);ASSERT_EQ(result.status,SolveStatus::under_constrained)<<result.diagnostic;
    EXPECT_NEAR(result.lines[0].start.y,result.lines[0].end.y,1e-7);
    EXPECT_NEAR(result.lines[0].start.y,4+5*std::sin(0.5),1e-6);
    EXPECT_EQ(result.degrees_of_freedom,3);
}

} // namespace occccad::geometry::sketch
