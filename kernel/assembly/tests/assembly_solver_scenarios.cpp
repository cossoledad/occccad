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

TEST(AssemblySolver, ProductFaceFiveCoincidenceRegression) {
    Model model;
    model.bodies = {{"instance-21e275df4646172218cb",
                     {{-286.1246164637754, 50.348556618702084, 13.75259704832779},
                      {2.6946118524417014e-11, 5.2818152615750504e-12, -0.3167324979530608,
                       0.9485149048593884}}},
                    {"instance-e227326d55689927ee52",
                     {{-187.31243746150747, -54.70854797482553, 13.75259704324006},
                      {2.47110919716094e-11, 1.2059607413355575e-11, -0.5456708354686278,
                       0.8379996057988152}}}};
    const PlaneGeometry face{{23.83813541685627, -40.0, -13.765954459534102}, {0.0, -1.0, 0.0}};
    model.geometry = {{"face-5", model.bodies[0].id, face}, {"face-5", model.bodies[1].id, face}};
    Constraint coincidence =
        binary("face-coincident", ConstraintKind::Coincident, ref(model.bodies[0].id, "face-5"),
               ref(model.bodies[1].id, "face-5"));
    coincidence.direction_relation = DirectionRelation::Same;
    model.constraints = {coincidence};
    SolverOptions options;
    options.solve_intent = SolveIntent{{model.bodies[0].id},
                                       {model.bodies[1].id},
                                       SolvePreferencePolicy::MoveFirstMinimizeReference};

    const SolveResult result = Solver{}.solve(model, options);
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }
    EXPECT_EQ(result.status, SolveStatus::Converged)
        << result.diagnostic << " residual=" << result.normalized_residual
        << " iterations=" << result.iterations;
    for (const auto& equation : result.equation_residuals)
        EXPECT_NEAR(equation.normalized_value, 0.0, 1.0e-6) << equation.equation_id;
}

TEST(AssemblySolver, PlaneCoincidenceAnalyticJacobianAndNullSpaceAreConformant) {
    Model model;
    model.bodies = {{"first", {{2.0, -1.0, 4.0}, {0.1, -0.2, 0.3, 0.92}}},
                    {"second", {{-3.0, 2.0, -1.0}, {-0.2, 0.1, -0.1, 0.96}}}};
    model.geometry = {{"plane", "first", PlaneGeometry{{7.0, -2.0, 3.0}, {0.0, 1.0, 0.0}}},
                      {"plane", "second", PlaneGeometry{{-4.0, 5.0, 2.0}, {0.0, 0.0, 1.0}}}};
    Constraint coincidence = binary("plane-pair", ConstraintKind::Coincident,
                                    ref("first", "plane"), ref("second", "plane"));
    coincidence.direction_relation = DirectionRelation::Same;
    model.constraints = {coincidence};
    SolverOptions options;
    options.verify_analytic_jacobians = true;
    options.jacobian_check_tolerance = 2.0e-5;

    const SolveResult result = Solver{}.solve(model, options);
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(), 1U);
    const ComponentDof& component = result.components.front();
    EXPECT_EQ(component.jacobian_rank, 3U);
    EXPECT_EQ(component.relative_dof, 3U);
    EXPECT_EQ(component.gauge_dof, 6U);
    EXPECT_EQ(component.tangent_cluster_ids.size(), 2U);
    EXPECT_EQ(component.null_space_basis.size(), 9U);
    for (const auto& basis : component.null_space_basis)
        EXPECT_EQ(basis.size(), 12U);
    ASSERT_EQ(result.constraint_ranks.size(), 1U);
    EXPECT_EQ(result.constraint_ranks[0].declared_generic_rank, 3U);
    ASSERT_EQ(result.equation_residuals.size(), 4U);
    EXPECT_EQ(result.equation_residuals[3].equation_id,
              "plane-pair/equation/PLANE_OFFSET");
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

TEST(AssemblySolver, OppositePlaneCoincidenceUsesTheSameCreationAnchorAsEditing) {
    Model model;
    model.bodies = {{"moving", {{-173.79051029395015, 0.0, 0.0}, {}}}, {"reference", {}}};
    model.geometry = {
        {"plane", "moving", PlaneGeometry{{-5.0, -40.0, 5.0}, {0.0, -1.0, 0.0}}},
        {"plane", "reference", PlaneGeometry{{-5.0, -40.0, 5.0}, {0.0, -1.0, 0.0}}},
    };
    Constraint coincident = binary("opposite", ConstraintKind::Coincident, ref("moving", "plane"),
                                   ref("reference", "plane"));
    coincident.direction_relation = DirectionRelation::Opposite;
    model.constraints = {coincident};
    SolverOptions options;
    options.solve_intent =
        SolveIntent{{"moving"}, {"reference"}, SolvePreferencePolicy::MoveFirstMinimizeReference};

    const SolveResult result = Solver{}.solve(model, options);
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Pose reference = pose(result, "reference");
    EXPECT_NEAR(reference.translation.x, 0.0, 1.0e-9);
    EXPECT_NEAR(reference.rotation.w, 1.0, 1.0e-9);
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

TEST(AssemblySolver, DirectedPlaneAngleDistinguishesNinetyFromTwoHundredSeventy) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}},
                      {"plane", "moving", PlaneGeometry{{}, {1.0, 0.0, 0.0}}}};
    Constraint angle = binary("directed-angle", ConstraintKind::Angle, ref("moving", "plane"),
                              ref("ground", "plane"));
    angle.value = 3.0 * kPi / 2.0;
    angle.direction_relation = DirectionRelation::Same;
    angle.angle_reference_direction = Vec3{0.0, -1.0, 0.0};
    model.constraints = {fix("ground"), angle};

    const SolveResult result = Solver{}.solve(model);

    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    const Vec3 normal = rotate(pose(result, "moving").rotation, {1.0, 0.0, 0.0});
    EXPECT_NEAR(normal.x, -1.0, 1.0e-6);
    EXPECT_NEAR(normal.z, 0.0, 1.0e-6);
}

TEST(AssemblySolver, DirectedAngleCrossesPiOnAConstrainedHinge) {
    for (const double target_degrees : {181.0, 270.0}) {
        Model model;
        model.bodies = {{"moving",
                         {{-169.82900867865035, -6.470427574396004, -5.553248048104228e-9},
                          {7.668012345918533e-12, -5.907059238768305e-11, 0.06176982153323116,
                           0.9980904213285252}}},
                        {"ground",
                         {{-44.43828073416036, -26.5212748744714, 8.660380375883868e-9},
                          {2.2696593137509963e-11, -5.5073654344670444e-11, -0.19865974397266659,
                           0.980068521137535}}}};
        model.geometry = {
            {"edge", "moving", AxisGeometry{{50.0, -40.0, -50.0}, {0.0, 0.0, 1.0}}},
            {"edge", "ground", AxisGeometry{{-60.0, -40.0, 60.0}, {0.0, 0.0, -1.0}}},
            {"point", "moving", PointGeometry{{50.0, -40.0, 60.0}}},
            {"point", "ground", PointGeometry{{-60.0, -40.0, 60.0}}},
            {"plane", "moving", PlaneGeometry{{-5.0, -40.0, 5.0}, {0.0, -1.0, 0.0}}},
            {"plane", "ground", PlaneGeometry{{-5.0, -40.0, 5.0}, {0.0, -1.0, 0.0}}},
        };
        Constraint ground = fix("ground");
        Constraint edges = binary("edge-coincident", ConstraintKind::Coincident,
                                  ref("moving", "edge"), ref("ground", "edge"));
        edges.direction_relation = DirectionRelation::Unoriented;
        Constraint points = binary("point-coincident", ConstraintKind::Coincident,
                                   ref("moving", "point"), ref("ground", "point"));
        Constraint angle = binary("directed-angle", ConstraintKind::Angle, ref("moving", "plane"),
                                  ref("ground", "plane"));
        angle.value = target_degrees * kPi / 180.0;
        angle.direction_relation = DirectionRelation::Same;
        angle.angle_reference_direction = Vec3{0.0, 0.0, -1.0};
        model.constraints = {ground, edges, points, angle};

        const SolveResult result = Solver{}.solve(model);
        EXPECT_EQ(result.status, SolveStatus::Converged)
            << "target=" << target_degrees << ": " << result.diagnostic;
        for (const ConstraintResidual& residual : result.residuals)
            EXPECT_LT(residual.normalized_norm, 1.0e-6)
                << "target=" << target_degrees << ", constraint=" << residual.constraint_id;
    }
}

TEST(AssemblySolver, DirectedAngleProjectsEndpointsAndRejectsDegenerateProjection) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {{"axis", "ground", AxisGeometry{{}, {1.0, 0.0, 1.0}}},
                      {"axis", "moving", AxisGeometry{{}, {0.0, -1.0, 2.0}}}};
    Constraint angle =
        binary("angle", ConstraintKind::Angle, ref("moving", "axis"), ref("ground", "axis"));
    angle.value = kPi / 2.0;
    angle.angle_reference_direction = Vec3{0.0, 0.0, 1.0};
    model.constraints = {fix("ground"), angle};
    const SolveResult projected = Solver{}.solve(model);
    EXPECT_EQ(projected.status, SolveStatus::Converged) << projected.diagnostic;

    model.geometry[1].local_geometry = AxisGeometry{{}, {0.0, 0.0, 1.0}};
    const SolveResult degenerate = Solver{}.solve(model);
    EXPECT_EQ(degenerate.status, SolveStatus::InvalidModel);
    EXPECT_NE(degenerate.diagnostic.find("parallel to its reference axis"), std::string::npos);
}

TEST(AssemblySolver, DirectedAngleReturnsNearestUnwrappedBranch) {
    const double one_degree = kPi / 180.0;
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {
        {"axis", "ground", AxisGeometry{{}, {1.0, 0.0, 0.0}}},
        {"axis", "moving", AxisGeometry{{}, {std::cos(one_degree), -std::sin(one_degree), 0.0}}}};
    Constraint angle =
        binary("angle", ConstraintKind::Angle, ref("moving", "axis"), ref("ground", "axis"));
    angle.value = one_degree;
    angle.angle_reference_direction = Vec3{0.0, 0.0, 1.0};
    angle.angle_branch_state = AngleBranchState{359.0 * one_degree, 359.0 * one_degree, 0};
    model.constraints = {fix("ground"), angle};

    const SolveResult result = Solver{}.solve(model);
    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.angle_branches.size(), 1U);
    EXPECT_NEAR(result.angle_branches[0].state.wrapped_angle, one_degree, 1.0e-8);
    EXPECT_NEAR(result.angle_branches[0].state.unwrapped_angle, 361.0 * one_degree, 1.0e-8);
    EXPECT_EQ(result.angle_branches[0].state.winding, 1);
}

TEST(AssemblySolver, DirectedAngleResidualUsesShortestPeriodicError) {
    const double degree = kPi / 180.0;
    for (const auto& [current_degrees, target_degrees] :
         {std::pair{1.0, 359.0}, std::pair{359.0, 1.0}}) {
        Model model;
        model.bodies = {{"first", {}}, {"second", {}}};
        const double current = current_degrees * degree;
        model.geometry = {
            {"axis", "first", AxisGeometry{{}, {std::cos(current), -std::sin(current), 0.0}}},
            {"axis", "second", AxisGeometry{{}, {1.0, 0.0, 0.0}}}};
        Constraint angle =
            binary("angle", ConstraintKind::Angle, ref("first", "axis"), ref("second", "axis"));
        angle.value = target_degrees * degree;
        angle.angle_reference_direction = Vec3{0.0, 0.0, 1.0};
        model.constraints = {fix("first"), fix("second"), angle};
        const SolveResult result = Solver{}.solve(model);
        ASSERT_EQ(result.residuals.size(), 3U);
        const auto found =
            std::find_if(result.residuals.begin(), result.residuals.end(),
                         [](const auto& residual) { return residual.constraint_id == "angle"; });
        ASSERT_NE(found, result.residuals.end());
        EXPECT_NEAR(found->normalized_norm, 2.0 * degree, 1.0e-10);
    }
}

TEST(AssemblySolver, SpatialAngleEndpointsHaveAlignmentRank) {
    for (const double target : {0.0, 0.001 * kPi / 180.0, 179.999 * kPi / 180.0, kPi, 2.0 * kPi}) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {}}};
        model.geometry = {{"axis", "ground", AxisGeometry{}}, {"axis", "moving", AxisGeometry{}}};
        Constraint angle =
            binary("angle", ConstraintKind::Angle, ref("moving", "axis"), ref("ground", "axis"));
        angle.value = target;
        model.constraints = {fix("ground"), angle};
        const SolveResult result = Solver{}.solve(model);
        ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
        const auto found =
            std::find_if(result.constraint_ranks.begin(), result.constraint_ranks.end(),
                         [](const auto& rank) { return rank.constraint_id == "angle"; });
        ASSERT_NE(found, result.constraint_ranks.end());
        const bool endpoint = target == 0.0 || target == kPi || target == 2.0 * kPi;
        EXPECT_EQ(found->equation_count, endpoint ? 3U : 1U);
        EXPECT_EQ(found->effective_rank, endpoint ? 2U : 1U);
    }
}

TEST(AssemblySolver, SpatialAnglePreservesConeAndComposesWithDifferentCoincidentLines) {
    for (double target : {kPi / 6.0, 11.0 * kPi / 6.0}) {
        for (double azimuth : {0.0, kPi / 4.0, kPi / 2.0, kPi}) {
            SCOPED_TRACE(std::to_string(target) + "/" + std::to_string(azimuth));
            Model model;
            model.bodies = {{"ground", {}}, {"moving", {{}, {std::sin(kPi/12), 0, 0, std::cos(kPi/12)}}}};
            model.geometry = {
                {"plane", "ground", PlaneGeometry{}}, {"plane", "moving", PlaneGeometry{}},
                {"line", "ground", AxisGeometry{{}, {std::cos(azimuth), std::sin(azimuth), 0}}},
                {"line", "moving", AxisGeometry{{}, {1, 0, 0}}}};
            auto angle = binary("angle", ConstraintKind::Angle, ref("moving", "plane"), ref("ground", "plane"));
            angle.value = target;
            model.constraints = {fix("ground"), angle};
            const auto free = Solver{}.solve(model);
            ASSERT_EQ(free.status, SolveStatus::Converged) << free.diagnostic;
            ASSERT_EQ(free.components.size(), 1U);
            EXPECT_EQ(free.components[0].jacobian_rank, 1U);
            auto line = binary("line", ConstraintKind::Coincident, ref("moving", "line"), ref("ground", "line"));
            line.direction_relation = DirectionRelation::Same;
            model.constraints.push_back(line);
            const auto result = Solver{}.solve(model);
            ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
            const auto rotation = pose(result, "moving").rotation;
            const auto normal = rotate(rotation, {0, 0, 1});
            const auto axis = rotate(rotation, {1, 0, 0});
            EXPECT_NEAR(normal.z, std::cos(kPi / 6), 1e-7);
            EXPECT_NEAR(axis.x, std::cos(azimuth), 1e-7);
            EXPECT_NEAR(axis.y, std::sin(azimuth), 1e-7);
            EXPECT_NEAR(axis.z, 0, 1e-7);
            EXPECT_EQ(result.components[0].jacobian_rank, 5U);
        }
    }
}

TEST(AssemblySolver, SpatialAngleAcceptsAllAzimuthsAndReflexValues) {
    for (double target : {kPi/6, kPi/2, 3*kPi/2, 11*kPi/6}) {
        for (double azimuth : {0.0, 0.7, 1.7, 3.7, 5.8}) {
            const double separation = std::min(target, 2*kPi-target);
            Model model;
            model.bodies = {{"ground", {}}, {"moving", {}}};
            model.geometry = {{"plane", "ground", PlaneGeometry{}},
                {"plane", "moving", PlaneGeometry{{}, {std::sin(separation)*std::cos(azimuth),
                    std::sin(separation)*std::sin(azimuth), std::cos(separation)}}}};
            auto angle = binary("angle", ConstraintKind::Angle, ref("moving", "plane"), ref("ground", "plane"));
            angle.value = target;
            const double sense = target > kPi ? -1.0 : 1.0;
            angle.spatial_angle_branch_direction = Vec3{sense*std::sin(azimuth), -sense*std::cos(azimuth), 0};
            model.constraints = {fix("ground"), angle};
            SolverOptions options;
            options.verify_analytic_jacobians = true;
            const auto result = Solver{}.solve(model, options);
            ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
            EXPECT_EQ(result.components[0].jacobian_rank, 1U);
            EXPECT_NEAR(std::abs(pose(result, "moving").rotation.w), 1, 1e-9);
        }
    }
}

TEST(AssemblySolver, SpatialAngleNinetyAndTwoSeventySelectOppositePoses) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {{"plane", "ground", PlaneGeometry{}}, {"plane", "moving", PlaneGeometry{}}};
    auto angle = binary("angle", ConstraintKind::Angle, ref("moving","plane"), ref("ground","plane"));
    angle.spatial_angle_branch_direction = Vec3{1,0,0};
    angle.value = kPi/2;
    model.constraints = {fix("ground"),angle};
    const auto forward = Solver{}.solve(model);
    ASSERT_EQ(forward.status,SolveStatus::Converged) << forward.diagnostic;
    const auto n = rotate(pose(forward,"moving").rotation,{0,0,1});
    EXPECT_NEAR(n.y,1,1e-7);
    model.bodies[1].initial_pose = pose(forward,"moving");
    model.constraints[1].value = 3*kPi/2;
    const auto reverse = Solver{}.solve(model);
    ASSERT_EQ(reverse.status,SolveStatus::Converged) << reverse.diagnostic;
    const auto r = rotate(pose(reverse,"moving").rotation,{0,0,1});
    EXPECT_NEAR(r.y,-1,1e-7);
    EXPECT_EQ(reverse.components[0].jacobian_rank,1U);
    model.bodies[1].initial_pose = pose(reverse,"moving");
    const auto replay = Solver{}.solve(model);
    ASSERT_EQ(replay.status,SolveStatus::Converged) << replay.diagnostic;
    EXPECT_NEAR(rotate(pose(replay,"moving").rotation,{0,0,1}).y,-1,1e-7);
}

TEST(AssemblySolver, RigidClusterAllowsDistinctRolesButRejectsSameBodyOverlap) {
    Model model;
    model.bodies = {{"a", {}}, {"b", {}}};
    Constraint rigid = binary("rigid", ConstraintKind::Rigid, ref("a", ""), ref("b", ""));
    rigid.fixed_pose = Pose{};
    model.constraints = {rigid};
    SolverOptions options;
    options.solve_intent =
        SolveIntent{{"a"}, {"b"}, SolvePreferencePolicy::MoveFirstMinimizeReference};
    const SolveResult result = Solver{}.solve(model, options);
    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    for (const auto& component : result.components)
        EXPECT_EQ(component.preference.status, PreferenceStatus::Converged);
    options.solve_intent->reference_body_ids = {"a"};
    const auto invalid = Solver{}.solve(model, options);
    EXPECT_EQ(invalid.status, SolveStatus::InvalidModel);
    EXPECT_NE(invalid.diagnostic.find("one body cannot be both"), std::string::npos);
}

TEST(AssemblySolver, GroundedComponentUsesReferenceMotionPreference) {
    Model model;
    model.bodies = {
        {"ground", {}}, {"reference", {{2.0, 0.0, 0.0}, {}}}, {"moving", {{4.0, 0.0, 0.0}, {}}}};
    model.geometry = {{"point", "ground", PointGeometry{}},
                      {"point", "reference", PointGeometry{}},
                      {"point", "moving", PointGeometry{}}};
    Constraint radius = binary("radius", ConstraintKind::Distance, ref("reference", "point"),
                               ref("ground", "point"));
    radius.value = 2.0;
    Constraint together = binary("together", ConstraintKind::Coincident, ref("moving", "point"),
                                 ref("reference", "point"));
    model.constraints = {fix("ground"), radius, together};
    SolverOptions options;
    options.solve_intent =
        SolveIntent{{"moving"}, {"reference"}, SolvePreferencePolicy::MoveFirstMinimizeReference};

    const SolveResult result = Solver{}.solve(model, options);
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }
    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_NEAR(pose(result, "reference").translation.x, 2.0, 1.0e-5);
    EXPECT_NEAR(pose(result, "moving").translation.x, 2.0, 1.0e-5);
}

TEST(AssemblySolver, DuplicateConstraintsExposeRankBasisAndConflictProbe) {
    Model duplicate;
    duplicate.bodies = {{"ground", {}}, {"moving", {{10.0, 0.0, 0.0}, {}}}};
    duplicate.geometry = {{"point", "ground", PointGeometry{}},
                          {"point", "moving", PointGeometry{}}};
    Constraint first = binary("distance-a", ConstraintKind::Distance, ref("moving", "point"),
                              ref("ground", "point"));
    first.value = 10.0;
    Constraint second = first;
    second.id = "distance-b";
    duplicate.constraints = {fix("ground"), first, second};
    const SolveResult redundant = Solver{}.solve(duplicate);
    ASSERT_EQ(redundant.status, SolveStatus::Converged);
    ASSERT_EQ(redundant.constraint_ranks.size(), 2U);
    EXPECT_EQ(redundant.constraint_ranks[1].role, ConstraintRankRole::FullyRedundant);
    EXPECT_EQ(redundant.constraint_ranks[1].incremental_rank, 0U);

    duplicate.constraints[2].value = 20.0;
    const SolveResult conflict = Solver{}.solve(duplicate);
    EXPECT_EQ(conflict.status, SolveStatus::Unsatisfied);
    EXPECT_FALSE(conflict.suspected_conflicting_constraint_ids.empty());
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
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }

    // The pose reaches a stationary point; unequal radii remain unsatisfied.
    EXPECT_EQ(result.status, SolveStatus::Unsatisfied);
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
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }

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
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }

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
    if (result.status == SolveStatus::Converged) {
        for (const auto& component : result.components) {
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged)
                << "reference=" << component.preference.reference_optimality
                << " total=" << component.preference.total_optimality;
        }
    }

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

TEST(AssemblySolver, UnorientedPlaneCoincidencePrefersTheInitialOppositeBranch) {
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

TEST(AssemblyComposition, IndependentPositionDirectionClockingControlsSixDof) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    for (const auto& body : model.bodies) {
        model.geometry.push_back({"point", body.id, PointGeometry{}});
        model.geometry.push_back({"axis", body.id, AxisGeometry{{}, {0, 0, 1}}});
        model.geometry.push_back({"clock", body.id, AxisGeometry{{}, {1, 0, 0}}});
    }
    model.constraints = {fix("ground"), binary("anchor", ConstraintKind::Coincident,
                                               ref("moving", "point"), ref("ground", "point"))};
    auto check = [&](std::size_t rank) {
        const auto result = Solver{}.solve(model);
        ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
        ASSERT_EQ(result.components.size(), 1U);
        EXPECT_EQ(result.components[0].jacobian_rank, rank);
        EXPECT_EQ(result.components[0].relative_dof, 6U - rank);
    };
    check(3);
    auto align =
        binary("align", ConstraintKind::Parallel, ref("moving", "axis"), ref("ground", "axis"));
    align.direction_relation = DirectionRelation::Same;
    model.constraints.push_back(align);
    check(5);
    auto clock =
        binary("clock", ConstraintKind::Angle, ref("moving", "clock"), ref("ground", "clock"));
    clock.angle_reference_direction = Vec3{0, 0, 1};
    model.constraints.push_back(clock);
    check(6);
    model.constraints.back().mode = ConstraintMode::Suppressed;
    check(5);
    model.constraints[2].mode = ConstraintMode::Suppressed;
    check(3);
}

TEST(AssemblyComposition, OffsetPointLineAndLinePlaneUseAnalyticJacobians) {
    for (bool linePlane : {false, true}) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {{0, 0, 2}, {}}}};
        model.geometry = {
            {"a", "moving",
             linePlane ? Geometry{AxisGeometry{{}, {1, 0, 0}}} : Geometry{PointGeometry{}}},
            {"b", "ground",
             linePlane ? Geometry{PlaneGeometry{}} : Geometry{AxisGeometry{{}, {1, 0, 0}}}}};
        auto offset =
            binary("offset", ConstraintKind::Distance, ref("moving", "a"), ref("ground", "b"));
        offset.value = 2;
        model.constraints = {fix("ground"), offset};
        SolverOptions options;
        options.verify_analytic_jacobians = true;
        const auto result = Solver{}.solve(model, options);
        ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
        ASSERT_EQ(result.components.size(), 1U);
        EXPECT_EQ(result.components[0].jacobian_rank, linePlane ? 2U : 1U);
        std::swap(model.constraints.back().first, *model.constraints.back().second);
        EXPECT_EQ(Solver{}.solve(model, options).status, SolveStatus::Converged);
    }
}

TEST(AssemblyComposition, EmptyActiveSetAndMeasuredConstraintsLeaveBodiesFree) {
    Model model;
    model.bodies = {{"body", {}}};
    const auto empty = Solver{}.solve(model);
    ASSERT_EQ(empty.status, SolveStatus::Converged) << empty.diagnostic;
    ASSERT_EQ(empty.components.size(), 1U);
    EXPECT_EQ(empty.components[0].gauge_dof, 6U);
    model.constraints = {fix("body")};
    model.constraints[0].mode = ConstraintMode::Suppressed;
    const auto inactive = Solver{}.solve(model);
    EXPECT_EQ(inactive.status, SolveStatus::Converged);
    EXPECT_EQ(inactive.components[0].gauge_dof, 6U);
    Model measured;
    measured.bodies = {{"ground", {}}, {"moving", {{4, 0, 0}, {}}}};
    measured.geometry = {{"point", "ground", PointGeometry{}},
                         {"point", "moving", PointGeometry{}}};
    auto distance =
        binary("measure", ConstraintKind::Distance, ref("moving", "point"), ref("ground", "point"));
    distance.mode = ConstraintMode::Measured;
    distance.value = 10;
    measured.constraints = {fix("ground"), distance};
    const auto observed = Solver{}.solve(measured);
    ASSERT_EQ(observed.status, SolveStatus::Converged) << observed.diagnostic;
    EXPECT_NEAR(pose(observed, "moving").translation.x, 4.0, 1e-9);
    EXPECT_TRUE(observed.unsatisfied_constraint_ids.empty());
    std::size_t freedom = 0;
    for (const auto& component : observed.components)
        freedom += component.relative_dof + component.gauge_dof;
    EXPECT_EQ(freedom, 6U);
}

TEST(AssemblyComposition, EveryRankFromZeroThroughSixHasExpectedMobility) {
    for (std::size_t rank = 0; rank <= 6; ++rank) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {}}};
        for (const auto& body : model.bodies) {
            model.geometry.push_back({"point", body.id, PointGeometry{}});
            model.geometry.push_back({"axis", body.id, AxisGeometry{}});
            model.geometry.push_back({"plane", body.id, PlaneGeometry{}});
            model.geometry.push_back({"clock", body.id, AxisGeometry{{}, {1, 0, 0}}});
        }
        model.constraints = {fix("ground")};
        if (rank == 1 || rank >= 5)
            model.constraints.push_back(binary("height", ConstraintKind::Coincident,
                                               ref("moving", "point"), ref("ground", "plane")));
        if (rank == 2)
            model.constraints.push_back(binary("direction", ConstraintKind::Parallel,
                                               ref("moving", "axis"), ref("ground", "axis")));
        if (rank == 3)
            model.constraints.push_back(binary("point", ConstraintKind::Coincident,
                                               ref("moving", "point"), ref("ground", "point")));
        if (rank >= 4)
            model.constraints.push_back(binary("axis", ConstraintKind::Concentric,
                                               ref("moving", "axis"), ref("ground", "axis")));
        if (rank == 6) {
            auto clock = binary("clock", ConstraintKind::Angle, ref("moving", "clock"),
                                ref("ground", "clock"));
            clock.angle_reference_direction = Vec3{0, 0, 1};
            model.constraints.push_back(clock);
        }
        const auto result = Solver{}.solve(model);
        ASSERT_EQ(result.status, SolveStatus::Converged)
            << "rank=" << rank << " " << result.diagnostic;
        std::size_t actual_rank = 0, mobility = 0;
        for (const auto& component : result.components) {
            actual_rank += component.jacobian_rank;
            mobility += component.relative_dof + component.gauge_dof;
        }
        EXPECT_EQ(actual_rank, rank);
        EXPECT_EQ(mobility, 6U - rank);
    }
}

TEST(AssemblyComposition, JointFamiliesPreserveAllowedFiniteMotionAndRejectBlockedTranslation) {
    // This checks a finite path, not just the Jacobian nullity at identity.
    const std::vector<std::pair<std::string, std::size_t>> families = {
        {"ball", 3},     {"planar", 3},    {"cylindrical", 2},
        {"revolute", 1}, {"prismatic", 1}, {"fixed", 0}};
    for (const auto& [family, mobility] : families) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {}}};
        for (const auto& body : model.bodies) {
            model.geometry.push_back({"point", body.id, PointGeometry{}});
            model.geometry.push_back({"axis", body.id, AxisGeometry{}});
            model.geometry.push_back({"plane", body.id, PlaneGeometry{}});
            model.geometry.push_back({"clock", body.id, AxisGeometry{{}, {1, 0, 0}}});
        }
        model.constraints = {fix("ground")};
        if (family == "ball") {
            model.constraints.push_back(binary("point", ConstraintKind::Coincident,
                                               ref("moving", "point"), ref("ground", "point")));
        } else if (family == "planar") {
            auto c = binary("plane", ConstraintKind::Coincident, ref("moving", "plane"),
                            ref("ground", "plane"));
            c.direction_relation = DirectionRelation::Same;
            model.constraints.push_back(c);
        } else {
            auto c = binary("axis", ConstraintKind::Concentric, ref("moving", "axis"),
                            ref("ground", "axis"));
            c.direction_relation = DirectionRelation::Same;
            model.constraints.push_back(c);
            if (family == "revolute" || family == "fixed")
                model.constraints.push_back(binary("height", ConstraintKind::Coincident,
                                                   ref("moving", "point"), ref("ground", "plane")));
            if (family == "prismatic" || family == "fixed") {
                auto clock = binary("clock", ConstraintKind::Angle, ref("moving", "clock"),
                                    ref("ground", "clock"));
                clock.angle_reference_direction = Vec3{0, 0, 1};
                model.constraints.push_back(clock);
            }
        }
        for (int step = -6; step <= 6; ++step) {
            const double half_angle = step * kPi / 18.0;
            Pose target;
            if (family == "ball")
                target.rotation = {std::sin(half_angle), 0, 0, std::cos(half_angle)};
            if (family == "planar" || family == "cylindrical" || family == "revolute")
                target.rotation = {0, 0, std::sin(half_angle), std::cos(half_angle)};
            if (family == "planar")
                target.translation = {3.0 * step, -2.0 * step, 0};
            if (family == "cylindrical" || family == "prismatic")
                target.translation.z = 3.0 * step;
            model.bodies[1].initial_pose = target;
            const auto result = Solver{}.solve(model);
            ASSERT_EQ(result.status, SolveStatus::Converged) << family << ": " << result.diagnostic;
            ASSERT_EQ(result.components.size(), 1U);
            EXPECT_EQ(result.components[0].relative_dof, mobility) << family;
            const auto accepted = pose(result, "moving");
            EXPECT_NEAR(accepted.translation.x, target.translation.x, 1e-7);
            EXPECT_NEAR(accepted.translation.y, target.translation.y, 1e-7);
            EXPECT_NEAR(accepted.translation.z, target.translation.z, 1e-7);
            const auto& q = accepted.rotation;
            const auto& t = target.rotation;
            EXPECT_NEAR(std::abs(q.x * t.x + q.y * t.y + q.z * t.z + q.w * t.w), 1, 1e-7) << family;
        }
        // The same components must reject translation outside their allowed path.
        model.bodies[1].initial_pose = {};
        if (family == "planar")
            model.bodies[1].initial_pose.translation.z = 4;
        else
            model.bodies[1].initial_pose.translation.x = 4;
        const auto blocked = Solver{}.solve(model);
        ASSERT_EQ(blocked.status, SolveStatus::Converged) << family << ": " << blocked.diagnostic;
        EXPECT_NEAR(pose(blocked, "moving").translation.x, 0, 1e-7);
        EXPECT_NEAR(pose(blocked, "moving").translation.z, 0, 1e-7);
    }
}

TEST(AssemblyComposition, ZeroPointDistancesUseCoincidenceRank) {
    for (bool axis : {false, true}) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {}}};
        model.geometry = {
            {"point", "moving", PointGeometry{}},
            {"support", "ground", axis ? Geometry{AxisGeometry{}} : Geometry{PointGeometry{}}}};
        model.constraints = {fix("ground"),
                             binary("zero", ConstraintKind::Distance, ref("moving", "point"),
                                    ref("ground", "support"))};
        const auto result = Solver{}.solve(model);
        ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
        EXPECT_EQ(result.components[0].jacobian_rank, axis ? 2U : 3U);
    }
}

TEST(AssemblyComposition, PerpendicularIsOneEquationNotProjectedClocking) {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {{"axis", "ground", AxisGeometry{}},
                      {"axis", "moving", AxisGeometry{{}, {1, 0, 0}}}};
    model.constraints = {fix("ground"), binary("normal", ConstraintKind::Perpendicular,
                                               ref("moving", "axis"), ref("ground", "axis"))};
    const auto result = Solver{}.solve(model);
    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_EQ(result.components[0].jacobian_rank, 1U);
    EXPECT_EQ(result.components[0].relative_dof, 5U);
}

TEST(AssemblyComposition, DirectionRelationsEscapeStationaryInitialAlignment) {
    for (const auto kind : {ConstraintKind::Parallel, ConstraintKind::Perpendicular}) {
        Model model;
        model.bodies = {{"ground", {}}, {"moving", {{0, 0, 7}, {}}}};
        model.geometry = {{"plane", "ground", PlaneGeometry{}},
                          {"plane", "moving", PlaneGeometry{}}};
        auto relation = binary("relation", kind, ref("moving", "plane"), ref("ground", "plane"));
        relation.direction_relation = DirectionRelation::Opposite;
        model.constraints = {fix("ground"), relation};
        const auto result = Solver{}.solve(model);
        ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
        ASSERT_EQ(result.components.size(), 1U);
        EXPECT_EQ(result.components[0].preference.status, PreferenceStatus::Converged);
        EXPECT_EQ(result.components[0].jacobian_rank, kind == ConstraintKind::Parallel ? 2U : 1U);
        EXPECT_NEAR(pose(result, "moving").translation.z, 7.0, 1e-7);
    }
}

}  // namespace

TEST(AssemblySolver, UnorientedConcentricCanReverseForLaterPlaneCoincidence) {
    for (const bool grounded : {false, true}) {
    for (const bool reverse_order : {false, true}) {
      for (const double initial_angle : {0.0, 0.2, 1.4, 1.8, 3.141592653589793}) {
        Model model;
        model.bodies = {{"ground", {}}, {"pin", {{}, {0, std::sin(initial_angle/2), 0, std::cos(initial_angle/2)}}}};
        model.geometry = {{"axis", "ground", CylinderGeometry{{}, {0,0,1}, 2}},
                          {"axis", "pin", CylinderGeometry{{}, {0,0,1}, 1}},
                          {"end", "ground", PlaneGeometry{}},
                          {"end", "pin", PlaneGeometry{}}};
        auto concentric = binary("concentric", ConstraintKind::Concentric, ref("pin", "axis"), ref("ground", "axis"));
        auto contact = binary("contact", ConstraintKind::Coincident, ref("pin", "end"), ref("ground", "end"));
        contact.direction_relation = DirectionRelation::Opposite;
        model.constraints = {fix("ground"), concentric, contact};
        if (reverse_order) std::swap(model.constraints[1], model.constraints[2]);
                if (!grounded) model.constraints.erase(model.constraints.begin());
        SolverOptions options;
        options.solve_intent = SolveIntent{{"pin"}, {"ground"}, SolvePreferencePolicy::MoveFirstMinimizeReference};
        const auto result = Solver{}.solve(model, options);
        ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
        EXPECT_NEAR(rotate(pose(result, "pin").rotation, {0,0,1}).z, -1.0, 1e-7);
        for (const auto& component : result.components)
            EXPECT_EQ(component.preference.status, PreferenceStatus::Converged);
      }
    }
    }
}

}  // namespace occccad::assembly
