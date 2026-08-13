#include <gtest/gtest.h>

#include "occccad/geometry/sketch/sketch_solver.h"

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
         {endpoint("bottom", SubElement::end), endpoint("right", SubElement::start)}},
        {"join-1",
         ConstraintKind::coincident,
         {endpoint("right", SubElement::end), endpoint("top", SubElement::start)}},
        {"join-2",
         ConstraintKind::coincident,
         {endpoint("top", SubElement::end), endpoint("left", SubElement::start)}},
        {"join-3",
         ConstraintKind::coincident,
         {endpoint("left", SubElement::end), endpoint("bottom", SubElement::start)}},
        {"parallel-x-0",
         ConstraintKind::parallel,
         {endpoint("bottom", SubElement::direction), axis(GeometryTarget::sketch_x_axis)}},
        {"parallel-y-0",
         ConstraintKind::parallel,
         {endpoint("right", SubElement::direction), axis(GeometryTarget::sketch_y_axis)}},
        {"parallel-x-1",
         ConstraintKind::parallel,
         {endpoint("top", SubElement::direction), axis(GeometryTarget::sketch_x_axis)}},
        {"parallel-y-1",
         ConstraintKind::parallel,
         {endpoint("left", SubElement::direction), axis(GeometryTarget::sketch_y_axis)}},
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
         {endpoint("line", SubElement::start), endpoint("missing", SubElement::end)}}};

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
}  // namespace occccad::geometry::sketch
