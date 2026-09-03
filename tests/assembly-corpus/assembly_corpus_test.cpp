#include "assembly_corpus.hpp"

#include <gtest/gtest.h>

#include <algorithm>
#include <array>
#include <cmath>
#include <cstddef>
#include <set>
#include <string>
#include <tuple>
#include <utility>
#include <vector>

namespace occccad::assembly::corpus {
namespace {

constexpr double kMotionMagnitude = 1.0e-2;
constexpr double kPoseTolerance = 1.0e-7;

Quaternion rotation(const MotionKind motion, const double magnitude) {
    Vec3 axis{};
    switch (motion) {
        case MotionKind::RotateX:
            axis.x = 1.0;
            break;
        case MotionKind::RotateY:
            axis.y = 1.0;
            break;
        case MotionKind::RotateZ:
            axis.z = 1.0;
            break;
        default:
            return {};
    }
    const double half = magnitude * 0.5;
    return {axis.x * std::sin(half), axis.y * std::sin(half), axis.z * std::sin(half),
            std::cos(half)};
}

Pose perturbed(Pose value, const MotionKind motion, const double magnitude = kMotionMagnitude) {
    switch (motion) {
        case MotionKind::TranslateX:
            value.translation.x += magnitude;
            break;
        case MotionKind::TranslateY:
            value.translation.y += magnitude;
            break;
        case MotionKind::TranslateZ:
            value.translation.z += magnitude;
            break;
        case MotionKind::RotateX:
        case MotionKind::RotateY:
        case MotionKind::RotateZ:
            value.rotation = rotation(motion, magnitude);
            break;
    }
    return value;
}

double pose_distance(const Pose& first, const Pose& second) {
    const double dx = first.translation.x - second.translation.x;
    const double dy = first.translation.y - second.translation.y;
    const double dz = first.translation.z - second.translation.z;
    const double direct =
        std::sqrt((first.rotation.x - second.rotation.x) * (first.rotation.x - second.rotation.x) +
                  (first.rotation.y - second.rotation.y) * (first.rotation.y - second.rotation.y) +
                  (first.rotation.z - second.rotation.z) * (first.rotation.z - second.rotation.z) +
                  (first.rotation.w - second.rotation.w) * (first.rotation.w - second.rotation.w));
    const double negated =
        std::sqrt((first.rotation.x + second.rotation.x) * (first.rotation.x + second.rotation.x) +
                  (first.rotation.y + second.rotation.y) * (first.rotation.y + second.rotation.y) +
                  (first.rotation.z + second.rotation.z) * (first.rotation.z + second.rotation.z) +
                  (first.rotation.w + second.rotation.w) * (first.rotation.w + second.rotation.w));
    return std::sqrt(dx * dx + dy * dy + dz * dz) + std::min(direct, negated);
}

Pose solved_pose(const SolveResult& result, const std::string& body_id) {
    const auto found = std::find_if(result.bodies.begin(), result.bodies.end(),
                                    [&](const SolvedBody& body) { return body.id == body_id; });
    EXPECT_NE(found, result.bodies.end());
    return found == result.bodies.end() ? Pose{} : found->pose;
}

const ComponentDof& component_for(const SolveResult& result, const std::string& body_id) {
    const auto found = std::find_if(
        result.components.begin(), result.components.end(), [&](const ComponentDof& component) {
            return std::find(component.body_ids.begin(), component.body_ids.end(), body_id) !=
                   component.body_ids.end();
        });
    EXPECT_NE(found, result.components.end());
    return *found;
}

Body& body(Model& model, const std::string& body_id) {
    const auto found = std::find_if(model.bodies.begin(), model.bodies.end(),
                                    [&](const Body& value) { return value.id == body_id; });
    EXPECT_NE(found, model.bodies.end());
    return *found;
}

class FreedomCorpusTest : public testing::TestWithParam<std::size_t> {};

TEST_P(FreedomCorpusTest, CanonicalMotionsMatchExpectedFreedom) {
    const FreedomCase test = freedom_cases().at(GetParam());

    EXPECT_LE(test.allowed_motions.size(), test.expected_relative_dof + test.expected_gauge_dof)
        << test.name;
    for (const MotionKind motion : test.allowed_motions) {
        Model model = test.model;
        Body& observed = body(model, test.observed_body_id);
        const Pose candidate = perturbed(observed.initial_pose, motion);
        observed.initial_pose = candidate;

        // For an ungrounded component, a gauge perturbation must be applied to
        // every body. Rotations are safe here because the canonical fixture has
        // all constrained points at the world origin.
        if (test.expected_gauge_dof == 6) {
            for (Body& other : model.bodies) {
                if (other.id != test.observed_body_id)
                    other.initial_pose = perturbed(other.initial_pose, motion);
            }
        }

        const SolveResult result = Solver{}.solve(model);
        ASSERT_EQ(result.status, SolveStatus::Converged) << test.name << ": " << result.diagnostic;
        const ComponentDof& component = component_for(result, test.observed_body_id);
        EXPECT_EQ(component.relative_dof, test.expected_relative_dof) << test.name;
        EXPECT_EQ(component.gauge_dof, test.expected_gauge_dof) << test.name;
        EXPECT_EQ(result.iterations, 0U) << test.name;
        EXPECT_LT(pose_distance(solved_pose(result, test.observed_body_id), candidate),
                  kPoseTolerance)
            << test.name;
    }

    for (const MotionKind motion : test.blocked_motions) {
        Model model = test.model;
        Body& observed = body(model, test.observed_body_id);
        const Pose candidate = perturbed(observed.initial_pose, motion);
        observed.initial_pose = candidate;

        const SolveResult result = Solver{}.solve(model);
        ASSERT_EQ(result.status, SolveStatus::Converged) << test.name << ": " << result.diagnostic;
        const ComponentDof& component = component_for(result, test.observed_body_id);
        EXPECT_EQ(component.relative_dof, test.expected_relative_dof) << test.name;
        EXPECT_EQ(component.gauge_dof, test.expected_gauge_dof) << test.name;
        EXPECT_GT(pose_distance(solved_pose(result, test.observed_body_id), candidate),
                  kPoseTolerance)
            << test.name;
    }
}

INSTANTIATE_TEST_SUITE_P(M0, FreedomCorpusTest,
                         testing::Range<std::size_t>(0, freedom_cases().size()));

class ClassificationCorpusTest : public testing::TestWithParam<std::size_t> {};

TEST_P(ClassificationCorpusTest, ReportsStructuredClassification) {
    const ClassificationCase test = classification_cases().at(GetParam());
    const SolveResult result = Solver{}.solve(test.model);
    EXPECT_EQ(result.status, test.baseline_status) << test.name << ": " << result.diagnostic;
    EXPECT_EQ(result.classification, test.expected_classification) << test.name;
}

INSTANTIATE_TEST_SUITE_P(M0, ClassificationCorpusTest,
                         testing::Range<std::size_t>(0, classification_cases().size()));

TEST(AssemblyCorpus, ConstraintAndBodyOrderDoNotChangeSemanticSolution) {
    Model original;
    original.bodies = {{"ground", {}}, {"moving", {{2.0, -3.0, 4.0}, {}}}};
    original.geometry = {{"plane", "ground", PlaneGeometry{}},
                         {"plane", "moving", PlaneGeometry{}},
                         {"point", "ground", PointGeometry{}},
                         {"point", "moving", PointGeometry{}}};
    Constraint plane_constraint;
    plane_constraint.id = "plane";
    plane_constraint.kind = ConstraintKind::Coincident;
    plane_constraint.first = {"moving", "plane"};
    plane_constraint.second = GeometryRef{"ground", "plane"};
    plane_constraint.direction_relation = DirectionRelation::Same;
    Constraint point_constraint;
    point_constraint.id = "point";
    point_constraint.kind = ConstraintKind::Coincident;
    point_constraint.first = {"moving", "point"};
    point_constraint.second = GeometryRef{"ground", "point"};
    Constraint ground;
    ground.id = "ground";
    ground.kind = ConstraintKind::Fix;
    ground.first = {"ground", {}};
    original.constraints = {ground, plane_constraint, point_constraint};

    Model permuted = original;
    std::reverse(permuted.bodies.begin(), permuted.bodies.end());
    std::reverse(permuted.geometry.begin(), permuted.geometry.end());
    std::reverse(permuted.constraints.begin(), permuted.constraints.end());

    const SolveResult first = Solver{}.solve(original);
    const SolveResult second = Solver{}.solve(permuted);
    ASSERT_EQ(first.status, SolveStatus::Converged) << first.diagnostic;
    ASSERT_EQ(second.status, SolveStatus::Converged) << second.diagnostic;
    EXPECT_LT(pose_distance(solved_pose(first, "moving"), solved_pose(second, "moving")), 1.0e-7);
    EXPECT_NEAR(first.normalized_residual, second.normalized_residual, 1.0e-9);
}

TEST(AssemblyCorpus, PreviousSolutionIsAValidWarmStart) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{3.0, -2.0, 5.0}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}}, {"plane", "moving", PlaneGeometry{}}};
    Constraint distance;
    distance.id = "offset";
    distance.kind = ConstraintKind::Distance;
    distance.first = {"moving", "plane"};
    distance.second = GeometryRef{"ground", "plane"};
    distance.value = 2.0;
    distance.direction_relation = DirectionRelation::Same;
    distance.distance_relation = DistanceRelation::AlongSecondNormal;
    Constraint ground;
    ground.id = "ground";
    ground.kind = ConstraintKind::Fix;
    ground.first = {"ground", {}};
    model.constraints = {ground, distance};

    const SolveResult cold = Solver{}.solve(model);
    ASSERT_EQ(cold.status, SolveStatus::Converged) << cold.diagnostic;
    body(model, "moving").initial_pose = solved_pose(cold, "moving");
    const SolveResult warm = Solver{}.solve(model);
    ASSERT_EQ(warm.status, SolveStatus::Converged) << warm.diagnostic;
    EXPECT_EQ(warm.iterations, 0U);
    EXPECT_LT(pose_distance(solved_pose(cold, "moving"), solved_pose(warm, "moving")),
              kPoseTolerance);
}

TEST(AssemblyCorpus, DimensionEditPreservesExplicitPlaneSide) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{0.0, 0.0, 8.0}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}}, {"plane", "moving", PlaneGeometry{}}};
    Constraint distance;
    distance.id = "offset";
    distance.kind = ConstraintKind::Distance;
    distance.first = {"moving", "plane"};
    distance.second = GeometryRef{"ground", "plane"};
    distance.value = 3.0;
    distance.direction_relation = DirectionRelation::Same;
    distance.distance_relation = DistanceRelation::AlongSecondNormal;
    Constraint ground;
    ground.id = "ground";
    ground.kind = ConstraintKind::Fix;
    ground.first = {"ground", {}};
    model.constraints = {ground, distance};

    const SolveResult first = Solver{}.solve(model);
    ASSERT_EQ(first.status, SolveStatus::Converged) << first.diagnostic;
    body(model, "moving").initial_pose = solved_pose(first, "moving");
    model.constraints.back().value = 5.0;
    const SolveResult edited = Solver{}.solve(model);
    ASSERT_EQ(edited.status, SolveStatus::Converged) << edited.diagnostic;
    EXPECT_NEAR(solved_pose(edited, "moving").translation.z, 5.0, kPoseTolerance);
}

TEST(AssemblyCorpus, RigidBodiesCompileIntoOneGroundedCluster) {
    Model model;
    model.bodies = {{"ground", {}}, {"member", {{4.0, 2.0, 0.0}, {}}}};
    Constraint ground;
    ground.id = "ground";
    ground.kind = ConstraintKind::Fix;
    ground.first = {"ground", {}};
    ground.fixed_pose = Pose{};
    Constraint rigid;
    rigid.id = "rigid";
    rigid.connection_id = "connection/fastened";
    rigid.kind = ConstraintKind::Rigid;
    rigid.first = {"member", {}};
    rigid.second = GeometryRef{"ground", {}};
    rigid.fixed_pose = Pose{{4.0, 2.0, 0.0}, {}};
    model.constraints = {ground, rigid};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(), 1U);
    EXPECT_EQ(result.components[0].body_ids.size(), 2U);
    EXPECT_EQ(result.components[0].tangent_variable_count, 0U);
    EXPECT_EQ(result.components[0].relative_dof, 0U);
    EXPECT_EQ(result.components[0].gauge_dof, 0U);
    EXPECT_EQ(result.iterations, 0U);
    EXPECT_NEAR(solved_pose(result, "member").translation.x, 4.0, kPoseTolerance);
}

TEST(AssemblyCorpus, AffectedScopeSolvesOnlyItsConnectedComponent) {
    Model model;
    model.bodies = {{"ground-a", {}},
                    {"moving-a", {{1.0, 0.0, 0.0}, {}}},
                    {"ground-b", {}},
                    {"moving-b", {{2.0, 0.0, 0.0}, {}}}};
    model.geometry = {{"point", "ground-a", PointGeometry{}},
                      {"point", "moving-a", PointGeometry{}},
                      {"point", "ground-b", PointGeometry{}},
                      {"point", "moving-b", PointGeometry{}}};
    Constraint ground_a;
    ground_a.id = "fix-a";
    ground_a.kind = ConstraintKind::Fix;
    ground_a.first = {"ground-a", {}};
    ground_a.fixed_pose = Pose{};
    Constraint ground_b = ground_a;
    ground_b.id = "fix-b";
    ground_b.first = {"ground-b", {}};
    Constraint mate_a;
    mate_a.id = "mate-a";
    mate_a.kind = ConstraintKind::Coincident;
    mate_a.first = {"moving-a", "point"};
    mate_a.second = GeometryRef{"ground-a", "point"};
    Constraint mate_b = mate_a;
    mate_b.id = "mate-b";
    mate_b.first = {"moving-b", "point"};
    mate_b.second = GeometryRef{"ground-b", "point"};
    model.constraints = {ground_a, ground_b, mate_a, mate_b};
    SolverOptions options;
    options.affected_body_ids = {"moving-a"};

    const SolveResult result = Solver{}.solve(model, options);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(), 2U);
    EXPECT_EQ(std::count_if(result.components.begin(), result.components.end(),
                            [](const ComponentDof& component) { return component.solved; }),
              1);
    EXPECT_NEAR(solved_pose(result, "moving-a").translation.x, 0.0, kPoseTolerance);
    EXPECT_NEAR(solved_pose(result, "moving-b").translation.x, 2.0, kPoseTolerance);
    EXPECT_TRUE(result.conflicting_constraint_ids.empty());
}

TEST(AssemblyCorpus, MeasuredAndSuppressedConstraintsDoNotDriveOrConflict) {
    Model model;
    model.bodies = {{"first", {}}, {"second", {{1.0, 0.0, 0.0}, {}}}};
    model.geometry = {{"point", "first", PointGeometry{}}, {"point", "second", PointGeometry{}}};
    Constraint measured;
    measured.id = "measured";
    measured.kind = ConstraintKind::Coincident;
    measured.mode = ConstraintMode::Measured;
    measured.first = {"first", "point"};
    measured.second = GeometryRef{"second", "point"};
    Constraint suppressed = measured;
    suppressed.id = "suppressed";
    suppressed.mode = ConstraintMode::Suppressed;
    model.constraints = {measured, suppressed};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_EQ(result.classification, SolveClassification::SolvedFully);
    ASSERT_EQ(result.residuals.size(), 1U);
    EXPECT_EQ(result.residuals[0].constraint_id, "measured");
    EXPECT_GT(result.residuals[0].normalized_norm, 0.9);
    EXPECT_NEAR(result.normalized_residual, 0.0, kPoseTolerance);
    EXPECT_TRUE(result.conflicting_constraint_ids.empty());
    EXPECT_NEAR(solved_pose(result, "second").translation.x, 1.0, kPoseTolerance);
}

TEST(AssemblyCorpus, DuplicateConstraintReportsStableEquationAndConstraintIdentities) {
    const ClassificationCase test = classification_cases().front();
    const SolveResult result = Solver{}.solve(test.model);

    ASSERT_EQ(result.classification, SolveClassification::Redundant);
    ASSERT_EQ(result.redundant_constraint_ids.size(), 1U);
    EXPECT_EQ(result.redundant_constraint_ids[0], "coincident-b");
    ASSERT_FALSE(result.equation_residuals.empty());
    std::set<std::string> equation_ids;
    for (const EquationResidual& equation : result.equation_residuals) {
        EXPECT_EQ(equation.equation_id.rfind(equation.constraint_id + "/equation/", 0), 0U);
        EXPECT_TRUE(equation_ids.insert(equation.equation_id).second);
        EXPECT_FALSE(equation.connection_id.empty());
    }
}

TEST(AssemblyCorpus, MoveFirstIntentUsesReferenceAsTheUngroundedGaugeAnchor) {
    Model model;
    model.bodies = {{"moving", {{5.0, 0.0, 0.0}, {}}}, {"reference", {{2.0, 1.0, 0.0}, {}}}};
    model.geometry = {{"point", "moving", PointGeometry{}},
                      {"point", "reference", PointGeometry{}}};
    Constraint coincidence;
    coincidence.id = "coincident";
    coincidence.kind = ConstraintKind::Coincident;
    coincidence.first = {"moving", "point"};
    coincidence.second = GeometryRef{"reference", "point"};
    model.constraints = {coincidence};
    SolverOptions options;
    options.solve_intent =
        SolveIntent{{"moving"}, {"reference"}, SolvePreferencePolicy::MoveFirstMinimizeReference};

    const SolveResult result = Solver{}.solve(model, options);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Pose reference = solved_pose(result, "reference");
    EXPECT_NEAR(reference.translation.x, 2.0, kPoseTolerance);
    EXPECT_NEAR(reference.translation.y, 1.0, kPoseTolerance);
    const Pose moving = solved_pose(result, "moving");
    EXPECT_NEAR(moving.translation.x, 2.0, kPoseTolerance);
    EXPECT_NEAR(moving.translation.y, 1.0, kPoseTolerance);
    ASSERT_EQ(result.components.size(), 1U);
    EXPECT_EQ(result.components[0].tangent_variable_count, 12U);
    EXPECT_EQ(result.components[0].jacobian_rank, 3U);
    EXPECT_EQ(result.components[0].relative_dof, 3U);
    EXPECT_EQ(result.components[0].gauge_dof, 6U);
}

TEST(AssemblyCorpus, SolveIntentRejectsOverlappingMovingAndReferenceRoles) {
    Model model;
    model.bodies = {{"body", {}}};
    SolverOptions options;
    options.solve_intent =
        SolveIntent{{"body"}, {"body"}, SolvePreferencePolicy::MoveFirstMinimizeReference};

    const SolveResult result = Solver{}.solve(model, options);

    EXPECT_EQ(result.status, SolveStatus::InvalidModel);
    EXPECT_EQ(result.classification, SolveClassification::InvalidModel);
}

}  // namespace
}  // namespace occccad::assembly::corpus
