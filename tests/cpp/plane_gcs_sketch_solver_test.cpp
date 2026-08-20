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
}  // namespace occccad::geometry::sketch
