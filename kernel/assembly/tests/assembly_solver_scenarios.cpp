#include <occccad/assembly/solver.hpp>

#include <gtest/gtest.h>

#include <algorithm>
#include <cmath>
#include <string>

namespace occccad::assembly {
namespace {

constexpr double kPi = 3.14159265358979323846;

GeometryRef ref(const std::string& body, const std::string& geometry) {
    return {body, geometry};
}

Constraint fix(const std::string& body) {
    Constraint result;
    result.id = "fix-" + body;
    result.kind = ConstraintKind::Fix;
    result.first = {body, {}};
    return result;
}

Pose pose(const SolveResult& result, const std::string& body) {
    const auto found = std::find_if(result.bodies.begin(), result.bodies.end(),
                                    [&](const SolvedBody& value) { return value.id == body; });
    EXPECT_NE(found, result.bodies.end());
    return found->pose;
}

Constraint binary(const std::string& id, const ConstraintKind kind, const GeometryRef& first,
                  const GeometryRef& second) {
    Constraint result;
    result.id = id;
    result.kind = kind;
    result.first = first;
    result.second = second;
    return result;
}

Vec3 rotate(const Quaternion& q, const Vec3& vector) {
    const Vec3 u{q.x, q.y, q.z};
    const double dot = u.x * vector.x + u.y * vector.y + u.z * vector.z;
    const Vec3 cross{u.y * vector.z - u.z * vector.y, u.z * vector.x - u.x * vector.z,
                     u.x * vector.y - u.y * vector.x};
    return {2.0 * dot * u.x + (q.w * q.w - (u.x * u.x + u.y * u.y + u.z * u.z)) * vector.x +
                2.0 * q.w * cross.x,
            2.0 * dot * u.y + (q.w * q.w - (u.x * u.x + u.y * u.y + u.z * u.z)) * vector.y +
                2.0 * q.w * cross.y,
            2.0 * dot * u.z + (q.w * q.w - (u.x * u.x + u.y * u.y + u.z * u.z)) * vector.z +
                2.0 * q.w * cross.z};
}

TEST(AssemblySolver, FixAndPointCoincidentMoveOnlyTheFreeBody) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{4.0, -2.0, 1.0}, {}}}};
    model.geometry = {{"origin", "ground", PointGeometry{}}, {"origin", "moving", PointGeometry{}}};
    model.constraints = {fix("ground"), binary("coincident", ConstraintKind::Coincident,
                                               ref("moving", "origin"), ref("ground", "origin"))};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_NEAR(pose(result, "moving").translation.x, 0.0, 1.0e-7);
    EXPECT_NEAR(pose(result, "moving").translation.y, 0.0, 1.0e-7);
    EXPECT_NEAR(pose(result, "moving").translation.z, 0.0, 1.0e-7);
}

TEST(AssemblySolver, PointPlaneCoincidentMovesPointOntoPlane) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{1.0, 2.0, 5.0}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}}, {"point", "moving", PointGeometry{}}};
    model.constraints = {fix("ground"), binary("on-plane", ConstraintKind::Coincident,
                                               ref("moving", "point"), ref("ground", "plane"))};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_NEAR(pose(result, "moving").translation.z, 0.0, 1.0e-7);
}

TEST(AssemblySolver, CylinderAndAxisBecomeConcentric) {
    Model model;
    model.bodies = {{"ground", {}},
                    {"moving", {{3.0, -2.0, 4.0}, {0.0, std::sin(0.2), 0.0, std::cos(0.2)}}}};
    model.geometry = {{"axis", "ground", AxisGeometry{}},
                      {"cylinder", "moving", CylinderGeometry{{}, {0.0, 0.0, 1.0}, 2.0}}};
    model.constraints = {fix("ground"), binary("concentric", ConstraintKind::Concentric,
                                               ref("moving", "cylinder"), ref("ground", "axis"))};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Pose moving = pose(result, "moving");
    const Vec3 direction = rotate(moving.rotation, {0.0, 0.0, 1.0});
    EXPECT_NEAR(std::hypot(direction.x, direction.y), 0.0, 1.0e-6);
    EXPECT_NEAR(std::hypot(moving.translation.x, moving.translation.y), 0.0, 1.0e-6);
}

TEST(AssemblySolver, PlaneDistanceUsesAnExplicitSide) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{0.0, 0.0, 8.0}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}}, {"plane", "moving", PlaneGeometry{}}};
    Constraint distance = binary("distance", ConstraintKind::Distance, ref("moving", "plane"),
                                 ref("ground", "plane"));
    distance.value = 3.0;
    distance.direction_relation = DirectionRelation::Same;
    distance.distance_relation = DistanceRelation::AlongSecondNormal;
    model.constraints = {fix("ground"), distance};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_NEAR(pose(result, "moving").translation.z, 3.0, 1.0e-7);
}

TEST(AssemblySolver, SameDirectionDoesNotAcceptAntiparallelPlanes) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}},
                      {"plane", "moving", PlaneGeometry{{}, {0.0, 0.0, -1.0}}}};
    Constraint coincident = binary("coincident", ConstraintKind::Coincident, ref("moving", "plane"),
                                   ref("ground", "plane"));
    coincident.direction_relation = DirectionRelation::Same;
    model.constraints = {fix("ground"), coincident};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Vec3 normal = rotate(pose(result, "moving").rotation, {0.0, 0.0, -1.0});
    EXPECT_NEAR(normal.z, 1.0, 1.0e-6);
}

TEST(AssemblySolver, AxisAngleSolvesToRequestedBranch) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {{"axis", "ground", AxisGeometry{}},
                      {"axis", "moving", AxisGeometry{{}, {1.0, 0.0, 1.0}}}};
    Constraint angle =
        binary("angle", ConstraintKind::Angle, ref("moving", "axis"), ref("ground", "axis"));
    angle.value = kPi / 2.0;
    angle.direction_relation = DirectionRelation::Same;
    model.constraints = {fix("ground"), angle};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Vec3 direction =
        rotate(pose(result, "moving").rotation, {std::sqrt(0.5), 0.0, std::sqrt(0.5)});
    EXPECT_NEAR(direction.z, 0.0, 1.0e-6);
}

TEST(AssemblySolver, PlaneAngleAboveNinetyDegreesConverges) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    const double initial = 20.0 * kPi / 180.0;
    model.geometry = {
        {"plane", "ground", PlaneGeometry{}},
        {"plane", "moving", PlaneGeometry{{}, {std::sin(initial), 0.0, std::cos(initial)}}}};
    Constraint angle = binary("obtuse-angle", ConstraintKind::Angle, ref("moving", "plane"),
                              ref("ground", "plane"));
    angle.value = 120.0 * kPi / 180.0;
    angle.direction_relation = DirectionRelation::Unoriented;
    model.constraints = {fix("ground"), angle};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Vec3 normal =
        rotate(pose(result, "moving").rotation, {std::sin(initial), 0.0, std::cos(initial)});
    EXPECT_NEAR(normal.z, std::cos(120.0 * kPi / 180.0), 1.0e-6);
}

TEST(AssemblySolver, LinePlaneCoincidentPlacesTheWholeLineInThePlane) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{0.0, 0.0, 4.0}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}},
                      {"line", "moving", AxisGeometry{{}, {1.0, 0.0, 0.4}}}};
    model.constraints = {fix("ground"), binary("line-on-plane", ConstraintKind::Coincident,
                                               ref("moving", "line"), ref("ground", "plane"))};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Pose moving = pose(result, "moving");
    const Vec3 direction = rotate(moving.rotation, {1.0, 0.0, 0.4});
    EXPECT_NEAR(direction.z, 0.0, 1.0e-6);
    EXPECT_NEAR(moving.translation.z, 0.0, 1.0e-6);
}

TEST(AssemblySolver, CylinderCoincidentRejectsAnIrreconcilableRadius) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{1.0, 0.0, 0.0}, {}}}};
    model.geometry = {{"cylinder", "ground", CylinderGeometry{{}, {0.0, 0.0, 1.0}, 2.0}},
                      {"cylinder", "moving", CylinderGeometry{{}, {0.0, 0.0, 1.0}, 3.0}}};
    model.constraints = {fix("ground"),
                         binary("coincident", ConstraintKind::Coincident, ref("moving", "cylinder"),
                                ref("ground", "cylinder"))};

    SolverOptions options;
    options.max_iterations = 8;
    const SolveResult result = Solver{}.solve(model, options);

    EXPECT_EQ(result.status, SolveStatus::MaxIterations);
    ASSERT_EQ(result.residuals.size(), 2U);
    EXPECT_GT(result.residuals[1].normalized_norm, 0.9);
}

TEST(AssemblySolver, InvalidGeometryIsReportedWithoutThrowing) {
    Model model;
    model.bodies = {{"body", {}}};
    model.geometry = {{"cylinder", "body", CylinderGeometry{{}, {}, 0.0}}};

    const SolveResult result = Solver{}.solve(model);

    EXPECT_EQ(result.status, SolveStatus::InvalidModel);
    EXPECT_FALSE(result.diagnostic.empty());
}

TEST(AssemblySolver, RigidMaintainsCapturedRelativePose) {
    Model model;
    model.bodies = {{"ground", {}}, {"first", {{4, 2, 0}, {}}}};
    Constraint ground = fix("ground");
    Constraint rigid;
    rigid.id = "rigid";
    rigid.kind = ConstraintKind::Rigid;
    rigid.first = {"first", {}};
    rigid.second = GeometryRef{"ground", {}};
    rigid.fixed_pose = Pose{{4, 2, 0}, {}};
    model.constraints = {ground, rigid};
    const SolveResult solved = Solver{}.solve(model);
    ASSERT_EQ(solved.status, SolveStatus::Converged) << solved.diagnostic;
    EXPECT_NEAR(pose(solved, "first").translation.x, 4.0, 1.0e-7);
    EXPECT_NEAR(pose(solved, "first").translation.y, 2.0, 1.0e-7);
}

TEST(AssemblySolver, GroundedViolationIsInconsistentNotNonConvergent) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{2.0, 0.0, 0.0}, {}}}};
    model.geometry = {{"point", "ground", PointGeometry{}}, {"point", "moving", PointGeometry{}}};
    model.constraints = {fix("ground"), fix("moving"),
                         binary("impossible", ConstraintKind::Coincident, ref("moving", "point"),
                                ref("ground", "point"))};

    const SolveResult result = Solver{}.solve(model);

    EXPECT_EQ(result.status, SolveStatus::Inconsistent);
    EXPECT_EQ(result.classification, SolveClassification::Inconsistent);
    EXPECT_EQ(result.conflicting_constraint_ids, std::vector<std::string>{"impossible"});
}

TEST(AssemblySolver, StationaryUnsatisfiedCandidateIsNotClaimedInconsistent) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}}, {"plane", "moving", PlaneGeometry{}}};
    Constraint first = binary("offset-3", ConstraintKind::Distance, ref("moving", "plane"),
                              ref("ground", "plane"));
    first.value = 3.0;
    first.direction_relation = DirectionRelation::Same;
    first.distance_relation = DistanceRelation::AlongSecondNormal;
    Constraint second = first;
    second.id = "offset-5";
    second.value = 5.0;
    model.constraints = {fix("ground"), first, second};

    const SolveResult result = Solver{}.solve(model);

    EXPECT_EQ(result.status, SolveStatus::Unsatisfied);
    EXPECT_EQ(result.classification, SolveClassification::Unsatisfied);
    EXPECT_FALSE(result.unsatisfied_constraint_ids.empty());
    EXPECT_TRUE(result.conflicting_constraint_ids.empty());
}

TEST(AssemblySolver, ClassificationToleranceIsIndependentFromConvergenceTolerance) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{5.0e-6, 0.0, 0.0}, {}}}};
    model.geometry = {{"point", "ground", PointGeometry{}}, {"point", "moving", PointGeometry{}}};
    model.constraints = {fix("ground"), fix("moving"),
                         binary("coincident", ConstraintKind::Coincident, ref("moving", "point"),
                                ref("ground", "point"))};
    SolverOptions options;
    options.length_tolerance = 1.0e-7;
    options.classification_length_tolerance = 1.0e-5;

    const SolveResult result = Solver{}.solve(model, options);

    EXPECT_EQ(result.status, SolveStatus::Unsatisfied);
    EXPECT_EQ(result.classification, SolveClassification::Unsatisfied);
    EXPECT_TRUE(result.unsatisfied_constraint_ids.empty());
    EXPECT_TRUE(result.conflicting_constraint_ids.empty());
}

TEST(AssemblySolver, ClassificationToleranceCannotBeStricterThanConvergenceTolerance) {
    Model model;
    model.bodies = {{"body", {}}};
    SolverOptions options;
    options.length_tolerance = 1.0e-5;
    options.classification_length_tolerance = 1.0e-6;

    const SolveResult result = Solver{}.solve(model, options);

    EXPECT_EQ(result.status, SolveStatus::InvalidModel);
    EXPECT_EQ(result.classification, SolveClassification::InvalidModel);
}

TEST(AssemblySolver, ExhaustedIterationBudgetIsNonConvergentNotInconsistent) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{100.0, 50.0, -20.0}, {}}}};
    model.geometry = {{"point", "ground", PointGeometry{}}, {"point", "moving", PointGeometry{}}};
    model.constraints = {fix("ground"), binary("coincident", ConstraintKind::Coincident,
                                               ref("moving", "point"), ref("ground", "point"))};
    SolverOptions options;
    options.max_iterations = 1;

    const SolveResult result = Solver{}.solve(model, options);

    EXPECT_EQ(result.status, SolveStatus::MaxIterations);
    EXPECT_EQ(result.classification, SolveClassification::NonConvergent);
    EXPECT_TRUE(result.conflicting_constraint_ids.empty());
}

TEST(AssemblySolver, UnsignedPointPlaneDistanceKeepsItsInitialSide) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{0.0, 0.0, -6.0}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}}, {"point", "moving", PointGeometry{}}};
    Constraint distance = binary("distance", ConstraintKind::Distance, ref("moving", "point"),
                                 ref("ground", "plane"));
    distance.value = 2.0;
    distance.distance_relation = DistanceRelation::Unsigned;
    model.constraints = {fix("ground"), distance};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_NEAR(pose(result, "moving").translation.z, -2.0, 1.0e-6);
}

TEST(AssemblySolver, UnorientedPlaneCoincidenceFreezesTheInitialOppositeBranch) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{0.0, 0.0, 3.0}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}},
                      {"plane", "moving", PlaneGeometry{{}, {0.0, 0.0, -1.0}}}};
    model.constraints = {fix("ground"), binary("coincident", ConstraintKind::Coincident,
                                               ref("moving", "plane"), ref("ground", "plane"))};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Vec3 normal = rotate(pose(result, "moving").rotation, {0.0, 0.0, -1.0});
    EXPECT_NEAR(normal.z, -1.0, 1.0e-7);
    EXPECT_NEAR(pose(result, "moving").translation.z, 0.0, 1.0e-7);
}

TEST(AssemblySolver, LengthAndAngleTolerancesAreIndependent) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{0.0, 0.0, 0.05}, {}}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}},
                      {"line", "moving", AxisGeometry{{}, {1.0, 0.0, 0.01}}}};
    model.constraints = {fix("ground"), fix("moving"),
                         binary("line-on-plane", ConstraintKind::Coincident, ref("moving", "line"),
                                ref("ground", "plane"))};
    SolverOptions strict_angle;
    strict_angle.length_tolerance = 0.1;
    strict_angle.angle_tolerance = 1.0e-4;
    strict_angle.classification_length_tolerance = strict_angle.length_tolerance;
    strict_angle.classification_angle_tolerance = strict_angle.angle_tolerance;
    const SolveResult inconsistent = Solver{}.solve(model, strict_angle);
    EXPECT_EQ(inconsistent.status, SolveStatus::Inconsistent);

    SolverOptions relaxed_angle = strict_angle;
    relaxed_angle.angle_tolerance = 0.02;
    relaxed_angle.classification_angle_tolerance = relaxed_angle.angle_tolerance;
    const SolveResult accepted = Solver{}.solve(model, relaxed_angle);
    EXPECT_EQ(accepted.status, SolveStatus::Converged) << accepted.diagnostic;
}

TEST(AssemblySolver, ExactZeroAndPiAnglesAreStable) {
    Model zero;
    zero.bodies = {{"first", {}}, {"second", {}}};
    zero.geometry = {{"first-axis", "first", AxisGeometry{}},
                     {"second-axis", "second", AxisGeometry{}}};
    Constraint zero_angle = binary("zero-angle", ConstraintKind::Angle, ref("first", "first-axis"),
                                   ref("second", "second-axis"));
    zero_angle.value = 0.0;
    zero_angle.direction_relation = DirectionRelation::Same;
    zero.constraints = {fix("first"), fix("second"), zero_angle};
    EXPECT_EQ(Solver{}.solve(zero).status, SolveStatus::Converged);

    Model pi = zero;
    pi.geometry[1].local_geometry = AxisGeometry{{}, {0.0, 0.0, -1.0}};
    pi.constraints.back().id = "pi-angle";
    pi.constraints.back().value = kPi;
    EXPECT_EQ(Solver{}.solve(pi).status, SolveStatus::Converged);
}

TEST(AssemblySolver, UnsignedAngleConvergesIntoZeroAndPiEndpointNeighborhoods) {
    constexpr double initial_offset = 1.0e-3;
    for (const double target : {0.0, kPi}) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {}}};
        const double z = target == 0.0 ? std::cos(initial_offset) : -std::cos(initial_offset);
        model.geometry = {
            {"ground-axis", "ground", AxisGeometry{}},
            {"moving-axis", "moving", AxisGeometry{{}, {std::sin(initial_offset), 0.0, z}}}};
        Constraint angle = binary("angle", ConstraintKind::Angle, ref("moving", "moving-axis"),
                                  ref("ground", "ground-axis"));
        angle.value = target;
        angle.direction_relation = DirectionRelation::Same;
        model.constraints = {fix("ground"), angle};

        const SolveResult result = Solver{}.solve(model);

        EXPECT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
        EXPECT_NE(result.classification, SolveClassification::NonConvergent);
    }
}

TEST(AssemblySolver, NearParallelAxisDistanceUsesDegenerateLimit) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {{2.0, 0.0, 0.0}, {}}}};
    model.geometry = {{"axis", "ground", AxisGeometry{}},
                      {"axis", "moving", AxisGeometry{{}, {1.0e-12, 0.0, 1.0}}}};
    Constraint distance =
        binary("distance", ConstraintKind::Distance, ref("moving", "axis"), ref("ground", "axis"));
    distance.value = 2.0;
    model.constraints = {fix("ground"), fix("moving"), distance};

    const SolveResult result = Solver{}.solve(model);

    EXPECT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
}

TEST(AssemblySolver, AxisDistanceDegeneracyBlendIsFiniteAndContinuousAcrossProfileScale) {
    std::vector<double> residuals;
    for (const double tilt : {0.0, 0.25e-8, 0.5e-8, 1.0e-8, 2.0e-8}) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {{2.0, 0.0, 0.0}, {}}}};
        model.geometry = {{"axis", "ground", AxisGeometry{}},
                          {"axis", "moving", AxisGeometry{{}, {tilt, 0.0, 1.0}}}};
        Constraint distance = binary("distance", ConstraintKind::Distance, ref("moving", "axis"),
                                     ref("ground", "axis"));
        distance.value = 2.0;
        model.constraints = {fix("ground"), fix("moving"), distance};

        const SolveResult result = Solver{}.solve(model);

        ASSERT_TRUE(std::isfinite(result.normalized_residual));
        residuals.push_back(result.normalized_residual);
    }
    for (std::size_t index = 1; index < residuals.size(); ++index) {
        EXPECT_GE(residuals[index], residuals[index - 1]);
        EXPECT_LT(residuals[index] - residuals[index - 1], 1.0);
    }
}

}  // namespace
}  // namespace occccad::assembly
