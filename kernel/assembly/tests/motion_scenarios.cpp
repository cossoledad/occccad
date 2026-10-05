#include <occccad/assembly/solver.hpp>

#include <gtest/gtest.h>

#include <Eigen/Core>
#include <algorithm>
#include <cmath>
#include <limits>
using namespace occccad::assembly;
namespace {
Constraint fixed(std::string id) {
    Constraint c;
    c.id = "fix-" + id;
    c.kind = ConstraintKind::Fix;
    c.first = {id, {}};
    return c;
}
Constraint mate(std::string id, ConstraintKind kind, GeometryRef a, GeometryRef b) {
    Constraint c;
    c.id = id;
    c.kind = kind;
    c.first = a;
    c.second = b;
    c.direction_relation = DirectionRelation::Same;
    return c;
}
const Pose& pose(const SolveResult& r, const std::string id) {
    for (const auto& b : r.bodies)
        if (b.id == id)
            return b.pose;
    throw std::runtime_error("missing body");
}
BodyFreedom freedom(const SolveResult& r, const std::string id) {
    for (const auto& c : r.components)
        for (const auto& b : c.freedoms)
            if (b.body_id == id)
                return b;
    throw std::runtime_error("missing freedom");
}
SolverOptions intent() {
    SolverOptions o;
    o.solve_intent = SolveIntent{{"a"}, {"b"}, SolvePreferencePolicy::MoveFirstMinimizeReference};
    return o;
}
void certified(const SolveResult& r) {
    ASSERT_EQ(r.status, SolveStatus::Converged) << r.diagnostic;
    for (const auto& c : r.components)
        if (c.solved) {
            EXPECT_EQ(c.preference.status, PreferenceStatus::Converged)
                << c.component_id << " ref " << c.preference.reference_optimality << " total "
                << c.preference.total_optimality;
            EXPECT_TRUE(c.preference.geometrically_feasible);
        }
}
Model point_pair() {
    Model m;
    m.bodies = {{"a", {{0, 2, 0}, {}}}, {"b", {{3, 0, 0}, {}}}};
    m.geometry = {{"p", "a", PointGeometry{}}, {"p", "b", PointGeometry{}}};
    m.constraints = {mate("join", ConstraintKind::Coincident, {"a", "p"}, {"b", "p"})};
    return m;
}
TEST(AssemblyMotion, FirstFixedMakesReferenceMoveWithoutReleasingAnyConstraint) {
    auto m = point_pair();
    m.constraints.push_back(fixed("a"));
    auto r = Solver{}.solve(m, intent());
    certified(r);
    EXPECT_NEAR(pose(r, "a").translation.y, 2, 1e-8);
    EXPECT_NEAR(pose(r, "b").translation.y, 2, 1e-7);
    EXPECT_NEAR(r.components[0].preference.reference_objective, 13, 1e-6);
}
TEST(AssemblyMotion, UngroundedReferenceAndIndirectlyGroundedFirst) {
    auto m = point_pair();
    auto r = Solver{}.solve(m, intent());
    certified(r);
    EXPECT_NEAR(r.components[0].preference.reference_objective, 0, 1e-15);
    m.bodies.push_back({"ground", {}});
    auto rigid = mate("rigid", ConstraintKind::Rigid, {"a", {}}, {"ground", {}});
    rigid.fixed_pose = m.bodies[0].initial_pose;
    m.constraints.push_back(rigid);
    m.constraints.push_back(fixed("ground"));
    r = Solver{}.solve(m, intent());
    certified(r);
    EXPECT_NEAR(pose(r, "b").translation.y, 2, 1e-7);
}
TEST(AssemblyMotion, GroundedComponentStillKeepsReferenceWhenFirstCanMove) {
    auto m = point_pair();
    m.bodies.push_back({"ground", {}});
    m.geometry.push_back({"plane", "ground", PlaneGeometry{}});
    m.constraints.push_back(fixed("ground"));
    m.constraints.push_back(
        mate("on-plane", ConstraintKind::Coincident, {"a", "p"}, {"ground", "plane"}));
    auto r = Solver{}.solve(m, intent());
    certified(r);
    EXPECT_NEAR(pose(r, "b").translation.x, 3, 1e-7);
    EXPECT_NEAR(pose(r, "b").translation.y, 0, 1e-7);
}
TEST(AssemblyMotion, PerpendicularSlidersTransferOnlyNecessaryMotion) {
    auto m = point_pair();
    m.bodies.push_back({"ground", {}});
    m.geometry.push_back({"x", "ground", AxisGeometry{{0, 2, 0}, {1, 0, 0}}});
    m.geometry.push_back({"y", "ground", AxisGeometry{{3, 0, 0}, {0, 1, 0}}});
    m.constraints.push_back(fixed("ground"));
    m.constraints.push_back(
        mate("slide-a", ConstraintKind::Coincident, {"a", "p"}, {"ground", "x"}));
    m.constraints.push_back(
        mate("slide-b", ConstraintKind::Coincident, {"b", "p"}, {"ground", "y"}));
    auto r = Solver{}.solve(m, intent());
    certified(r);
    EXPECT_NEAR(pose(r, "a").translation.x, 3, 1e-7);
    EXPECT_NEAR(pose(r, "b").translation.y, 2, 1e-7);
    EXPECT_NEAR(r.components[0].preference.reference_objective, 4, 1e-6);
}
TEST(AssemblyMotion, FeasibleWarmStartDoesNotRedefineNominal) {
    Model m;
    m.bodies = {{"ground", {}}, {"a", {{2, 3, 1}, {}}}};
    m.bodies[1].initial_guess = Pose{{8, -4, 0}, {}};
    m.geometry = {{"plane", "ground", PlaneGeometry{}}, {"p", "a", PointGeometry{}}};
    m.constraints = {fixed("ground"),
                     mate("on", ConstraintKind::Coincident, {"a", "p"}, {"ground", "plane"})};
    auto r = Solver{}.solve(m);
    certified(r);
    EXPECT_NEAR(pose(r, "a").translation.x, 2, 1e-7);
    EXPECT_NEAR(pose(r, "a").translation.y, 3, 1e-7);
    EXPECT_NEAR(r.components[0].preference.total_objective, 1, 1e-6);
}
TEST(AssemblyMotion, MultipleReferencesAreOptimizedTogetherIndependentOfOrder) {
    auto m = point_pair();
    SolverOptions o;
    o.solve_intent = SolveIntent{{}, {"a", "b"}, SolvePreferencePolicy::MoveFirstMinimizeReference};
    auto r = Solver{}.solve(m, o);
    certified(r);
    EXPECT_NEAR(pose(r, "a").translation.x, 1.5, 1e-6);
    EXPECT_NEAR(pose(r, "b").translation.y, 1, 1e-6);
    std::reverse(m.bodies.begin(), m.bodies.end());
    std::reverse(o.solve_intent->reference_body_ids.begin(),
                 o.solve_intent->reference_body_ids.end());
    auto other = Solver{}.solve(m, o);
    certified(other);
    EXPECT_NEAR(other.components[0].preference.reference_objective,
                r.components[0].preference.reference_objective, 1e-8);
}
TEST(AssemblyMotion, PreferenceBudgetFailureDoesNotMasqueradeAsGeometricConflict) {
    auto m = point_pair();
    SolverOptions o;
    o.max_preference_iterations = 0;
    auto r = Solver{}.solve(m, o);
    ASSERT_EQ(r.status, SolveStatus::Converged);
    ASSERT_EQ(r.components[0].preference.status, PreferenceStatus::IterationLimit);
    EXPECT_TRUE(r.conflicting_constraint_ids.empty());
}
TEST(AssemblyMotion, BothFixedRemainInconsistent) {
    auto m = point_pair();
    m.constraints.push_back(fixed("a"));
    m.constraints.push_back(fixed("b"));
    auto r = Solver{}.solve(m, intent());
    EXPECT_EQ(r.status, SolveStatus::Inconsistent);
}
TEST(AssemblyMotion, RotationObjectiveOracleAndWarmStartNearPi) {
    for (double angle : {0.01, 1.0, 3.141592653589793 - 1e-5}) {
        Model m;
        m.bodies = {{"a", {{0, 0, 0}, {0, 0, std::sin(.3), std::cos(.3)}}}};
        m.bodies[0].initial_guess =
            Pose{{0, 0, 0}, {std::sin(angle / 2), 0, 0, std::cos(angle / 2)}};
        auto r = Solver{}.solve(m);
        certified(r);
        EXPECT_NEAR(r.components[0].preference.total_objective, 0, 1e-12);
    }
}
Eigen::MatrixXd projector(const std::vector<std::vector<double>>& basis) {
    Eigen::MatrixXd p = Eigen::MatrixXd::Zero(6, 6);
    for (const auto& b : basis) {
        Eigen::Map<const Eigen::VectorXd> v(b.data(), 6);
        p += v * v.transpose();
    }
    return p;
}
TEST(AssemblyFreedom, PlanarCylindricalRevolutePrismaticAndSpherical) {
    for (auto kind : {FreedomKind::Planar, FreedomKind::Cylindrical, FreedomKind::Revolute,
                      FreedomKind::Prismatic, FreedomKind::Spherical}) {
        Model m;
        m.bodies = {{"ground", {}}, {"a", {}}};
        m.geometry = {{"plane", "ground", PlaneGeometry{}},
                      {"plane", "a", PlaneGeometry{}},
                      {"axis", "ground", AxisGeometry{}},
                      {"axis", "a", AxisGeometry{}},
                      {"p", "ground", PointGeometry{}},
                      {"p", "a", PointGeometry{}},
                      {"x", "ground", AxisGeometry{{}, {1, 0, 0}}},
                      {"x", "a", AxisGeometry{{}, {1, 0, 0}}}};
        m.constraints = {fixed("ground")};
        if (kind == FreedomKind::Planar)
            m.constraints.push_back(
                mate("plane", ConstraintKind::Coincident, {"a", "plane"}, {"ground", "plane"}));
        else if (kind == FreedomKind::Spherical)
            m.constraints.push_back(
                mate("point", ConstraintKind::Coincident, {"a", "p"}, {"ground", "p"}));
        else {
            m.constraints.push_back(
                mate("axis", ConstraintKind::Concentric, {"a", "axis"}, {"ground", "axis"}));
            if (kind == FreedomKind::Revolute)
                m.constraints.push_back(
                    mate("point", ConstraintKind::Coincident, {"a", "p"}, {"ground", "p"}));
            if (kind == FreedomKind::Prismatic)
                m.constraints.push_back(
                    mate("angle", ConstraintKind::Angle, {"a", "x"}, {"ground", "x"}));
        }
        auto r = Solver{}.solve(m);
        certified(r);
        const auto& f = freedom(r, "a");
        EXPECT_EQ(f.kind, kind);
        EXPECT_EQ(f.allowed_basis.size() + f.blocked_basis.size(), 6U);
        EXPECT_LT((projector(f.allowed_basis) + projector(f.blocked_basis) -
                   Eigen::MatrixXd::Identity(6, 6))
                      .norm(),
                  1e-7);
        EXPECT_EQ(freedom(r, "ground").kind, FreedomKind::Fixed);
    }
}
TEST(AssemblyFreedom, GaugeIsNotReportedAsRelativeBodyFreedom) {
    auto m = point_pair();
    auto r = Solver{}.solve(m, intent());
    certified(r);
    EXPECT_EQ(r.components[0].gauge_dof, 6U);
    EXPECT_EQ(freedom(r, "a").kind, FreedomKind::Fixed);
    EXPECT_EQ(freedom(r, "b").kind, FreedomKind::Spherical);
}
TEST(AssemblyMotion, RequiredReferenceMotionWithRemainingFreedomHasAnalyticMinimum) {
    auto m = point_pair();
    m.bodies.push_back({"ground", {}});
    m.geometry.push_back({"plane", "ground", PlaneGeometry{{0, 2, 0}, {0, 1, 0}}});
    m.constraints.push_back(fixed("ground"));
    m.constraints.push_back(
        mate("a-on-plane", ConstraintKind::Coincident, {"a", "p"}, {"ground", "plane"}));
    auto r = Solver{}.solve(m, intent());
    certified(r);
    EXPECT_NEAR(pose(r, "b").translation.x, 3, 1e-7);
    EXPECT_NEAR(pose(r, "b").translation.y, 2, 1e-7);
    EXPECT_NEAR(r.components[0].preference.reference_objective, 4, 1e-6);
}
TEST(AssemblyMotion, UnitAndWorldFrameChangesPreserveThePolicy) {
    for (double unit : {0.001, 1.0, 1000.0}) {
        auto m = point_pair();
        m.bodies.push_back({"ground", {}});
        // World Rz(90deg) and translated coordinates, including the physical ground.
        for (auto& b : m.bodies) {
            auto t = b.initial_pose.translation;
            b.initial_pose.translation = {unit * (10 - t.y), unit * (-7 + t.x), unit * 5};
            b.initial_pose.rotation = {0, 0, std::sqrt(.5), std::sqrt(.5)};
        }
        m.geometry.push_back({"plane", "ground", PlaneGeometry{}});
        m.constraints.push_back(fixed("ground"));
        m.constraints.push_back(
            mate("on", ConstraintKind::Coincident, {"a", "p"}, {"ground", "plane"}));
        auto o = intent();
        o.motion_length_scale = unit;
        o.length_scale = unit;
        o.length_tolerance *= unit;
        o.classification_length_tolerance *= unit;
        o.translation_finite_difference_step *= unit;
        auto r = Solver{}.solve(m, o);
        certified(r);
        EXPECT_NEAR(pose(r, "b").translation.x / unit, 10, 1e-6);
        EXPECT_NEAR(pose(r, "b").translation.y / unit, -4, 1e-6);
        EXPECT_NEAR(r.components[0].preference.reference_objective, 0, 1e-12);
    }
}
TEST(AssemblyMotion, RigidMateCanMergeMovingAndReferenceWithExistingConstraints) {
    for (bool grounded : {false, true}) {
        Model m;
        m.bodies = {{"a", {{2, 0, 0}, {}}}, {"b", {}}};
        m.geometry = {{"plane", "a", PlaneGeometry{}}, {"plane", "b", PlaneGeometry{}}};
        auto on = mate("on", ConstraintKind::Coincident, {"a", "plane"}, {"b", "plane"});
        auto rigid = mate("rigid", ConstraintKind::Rigid, {"a", {}}, {"b", {}});
        rigid.fixed_pose = Pose{{2, 0, 0}, {}};
        m.constraints = {on, rigid};
        if (grounded)
            m.constraints.push_back(fixed("b"));
        auto result = Solver{}.solve(m, intent());
        certified(result);
        EXPECT_NEAR(pose(result, "a").translation.x, 2, 1e-8);
        EXPECT_NEAR(pose(result, "b").translation.x, 0, 1e-8);
        EXPECT_EQ(result.components[0].relative_dof, 0U);
        // A conflicting hard equation must still fail, not be masked by Rigid.
        m.constraints[0].kind = ConstraintKind::Distance;
        m.constraints[0].value = 3;
        EXPECT_NE(Solver{}.solve(m, intent()).status, SolveStatus::Converged);
    }
}

TEST(AssemblyMotion, SharedRigidClusterReferenceRemainsAPreference) {
    Model m;
    m.bodies = {{"a", {{2, 0, 4}, {}}}, {"b", {{0, 0, 4}, {}}}, {"ground", {}}};
    m.geometry = {{"point", "a", PlaneGeometry{}}, {"plane", "ground", PlaneGeometry{}}};
    auto rigid = mate("rigid", ConstraintKind::Rigid, {"a", {}}, {"b", {}});
    rigid.fixed_pose = Pose{{2, 0, 0}, {}};
    m.constraints = {fixed("ground"), rigid,
                     mate("on", ConstraintKind::Coincident, {"a", "point"}, {"ground", "plane"})};
    const auto result = Solver{}.solve(m, intent());
    certified(result);
    EXPECT_NEAR(pose(result, "a").translation.z, 0, 1e-7);
    EXPECT_NEAR(pose(result, "b").translation.z, 0, 1e-7);
    EXPECT_NEAR(pose(result, "a").translation.x - pose(result, "b").translation.x, 2, 1e-7);
}

TEST(AssemblyMotion, RigidClusterRepresentativeDoesNotChangeOccurrenceObjective) {
    Model m;
    m.bodies = {{"a", {{0, 0, 4}, {}}}, {"member", {{2, 0, 4}, {}}}, {"ground", {}}};
    m.geometry = {{"p", "a", PointGeometry{}}, {"plane", "ground", PlaneGeometry{}}};
    auto rigid = mate("rigid", ConstraintKind::Rigid, {"member", {}}, {"a", {}});
    rigid.fixed_pose = Pose{{2, 0, 0}, {}};
    m.constraints = {fixed("ground"), rigid,
                     mate("on", ConstraintKind::Coincident, {"a", "p"}, {"ground", "plane"})};
    auto r = Solver{}.solve(m);
    certified(r);
    m.bodies[0].id = "z";
    m.geometry[0].body_id = "z";
    m.constraints[1].second->body_id = "z";
    m.constraints[2].first.body_id = "z";
    auto other = Solver{}.solve(m);
    certified(other);
    EXPECT_NEAR(r.components[0].preference.total_objective,
                other.components[0].preference.total_objective, 1e-7);
    EXPECT_NEAR(pose(r, "member").translation.z, pose(other, "member").translation.z, 1e-6);
}
TEST(AssemblyFreedom, SubspacesTransformWithWorldFrameAndPermutation) {
    Model m;
    m.bodies = {{"ground", {}}, {"a", {}}};
    m.geometry = {{"p", "a", PlaneGeometry{}}, {"p", "ground", PlaneGeometry{}}};
    m.constraints = {fixed("ground"),
                     mate("on", ConstraintKind::Coincident, {"a", "p"}, {"ground", "p"})};
    auto first = Solver{}.solve(m);
    certified(first);
    for (auto& body : m.bodies) {
        body.initial_pose.translation = {3, -7, 4};
        body.initial_pose.rotation = {std::sqrt(.5), 0, 0, std::sqrt(.5)};
    }
    std::reverse(m.bodies.begin(), m.bodies.end());
    std::reverse(m.constraints.begin(), m.constraints.end());
    auto second = Solver{}.solve(m);
    certified(second);
    Eigen::Matrix3d rotation;
    rotation << 1, 0, 0, 0, 0, -1, 0, 1, 0;
    Eigen::MatrixXd transform = Eigen::MatrixXd::Zero(6, 6);
    transform.topLeftCorner<3, 3>() = rotation;
    transform.bottomRightCorner<3, 3>() = rotation;
    EXPECT_LT((projector(freedom(second, "a").allowed_basis) -
               transform * projector(freedom(first, "a").allowed_basis) * transform.transpose())
                  .norm(),
              1e-7);
    EXPECT_EQ(freedom(second, "a").kind, FreedomKind::Planar);
}
TEST(AssemblyFreedom, OffOriginRevoluteAxisHasGeometricProvenance) {
    Model m;
    m.bodies = {{"ground", {}}, {"a", {}}};
    m.geometry = {{"axis", "a", AxisGeometry{{2, 3, 0}, {0, 0, 1}}},
                  {"axis", "ground", AxisGeometry{{2, 3, 0}, {0, 0, 1}}},
                  {"p", "a", PointGeometry{{2, 3, 0}}},
                  {"p", "ground", PointGeometry{{2, 3, 0}}}};
    m.constraints = {fixed("ground"),
                     mate("axis", ConstraintKind::Concentric, {"a", "axis"}, {"ground", "axis"}),
                     mate("point", ConstraintKind::Coincident, {"a", "p"}, {"ground", "p"})};
    auto r = Solver{}.solve(m);
    certified(r);
    auto f = freedom(r, "a");
    ASSERT_EQ(f.kind, FreedomKind::Revolute);
    ASSERT_EQ(f.rotations.size(), 1U);
    EXPECT_NEAR(f.rotations[0].axis_point.x, 2, 1e-7);
    EXPECT_NEAR(f.rotations[0].axis_point.y, 3, 1e-7);
    EXPECT_NEAR(f.rotations[0].pitch, 0, 1e-7);
}

TEST(AssemblyMotion, NonzeroReferenceMinimumPreservesItsWholeOptimalSet) {
    Model model;
    model.bodies = {{"ground", {}}, {"b", {}}, {"c", {{0, 1, 0}, {}}}};
    model.bodies[1].initial_guess = Pose{{1, 0, 0}, {}};
    model.bodies[2].initial_guess = Pose{{1, 0, 0}, {}};
    model.geometry = {
        {"p", "ground", PointGeometry{}}, {"p", "b", PointGeometry{}}, {"p", "c", PointGeometry{}}};
    auto radius = mate("radius", ConstraintKind::Distance, {"b", "p"}, {"ground", "p"});
    radius.value = 1;
    model.constraints = {fixed("ground"), radius,
                         mate("join", ConstraintKind::Coincident, {"c", "p"}, {"b", "p"})};
    auto options = intent();
    options.solve_intent->moving_body_ids = {"c"};
    const auto result = Solver{}.solve(model, options);
    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    certified(result);
    EXPECT_NEAR(result.components[0].preference.reference_objective, 1, 1e-8);
    EXPECT_NEAR(pose(result, "b").translation.y, 1, 1e-6);
    EXPECT_NEAR(pose(result, "c").translation.x, 0, 1e-6);
}

TEST(AssemblyMotion, AffectedScopeDoesNotApplyUnselectedWarmSeeds) {
    Model model;
    model.bodies = {{"a", {}}, {"b", {{3, 0, 0}, {}}}};
    model.bodies[1].initial_guess = Pose{{8, 0, 0}, {}};
    SolverOptions options;
    options.affected_body_ids = {"a"};
    const auto result = Solver{}.solve(model, options);
    certified(result);
    EXPECT_NEAR(pose(result, "b").translation.x, 3, 1e-12);
}
TEST(AssemblyMotion, InvalidMotionScaleAndInconsistentRigidSeedsAreRejected) {
    auto model = point_pair();
    auto options = intent();
    options.motion_length_scale = std::numeric_limits<double>::infinity();
    EXPECT_EQ(Solver{}.solve(model, options).status, SolveStatus::InvalidModel);
    model = Model{};
    model.bodies = {{"a", {}}, {"b", {{2, 0, 0}, {}}}};
    auto rigid = mate("rigid", ConstraintKind::Rigid, {"b", {}}, {"a", {}});
    rigid.fixed_pose = Pose{{2, 0, 0}, {}};
    model.constraints = {rigid};
    model.bodies[0].initial_guess = Pose{{1, 0, 0}, {}};
    model.bodies[1].initial_guess = Pose{{2, 0, 0}, {}};
    EXPECT_EQ(Solver{}.solve(model).status, SolveStatus::InvalidModel);
}
}  // namespace

Model face4_face6_regression() {
    Model m;
    m.bodies = {
        {"a", {{-252.55719832993879, 0, 0}, {0, 0, 0.41411770542833365, 0.9102233385553086}}},
        {"b", {}}};
    m.geometry = {
        {"face4", "a", PlaneGeometry{{89.88533068174983, 0, 38.522284577892705}, {0, 0, -1}}},
        {"face6", "b", PlaneGeometry{{22.64967658052029, -40, -32.815279455242084}, {0, -1, 0}}}};
    auto join = mate("join", ConstraintKind::Coincident, {"a", "face4"}, {"b", "face6"});
    join.direction_relation = DirectionRelation::Same;
    m.constraints = {fixed("b"), join};
    return m;
}
TEST(AssemblyMotion, RotatedTranslatedFace4CoincidentWithFixedFace6) {
    auto r = Solver{}.solve(face4_face6_regression(), intent());
    EXPECT_EQ(r.status, SolveStatus::Converged) << r.diagnostic;
    if (!r.components.empty()) {
        EXPECT_EQ(r.components[0].preference.status, PreferenceStatus::Converged);
    }
}

TEST(AssemblyMotion, FaceCoincidenceAfterTranslationAndRotationSweep) {
    for (int axis = 0; axis < 3; ++axis) {
        for (int degrees = 0; degrees < 360; degrees += 30) {
            SCOPED_TRACE(::testing::Message() << "axis=" << axis << " degrees=" << degrees);
            auto m = face4_face6_regression();
            double half = degrees * std::acos(-1.0) / 360.0;
            double v[3] = {0, 0, 0};
            v[axis] = std::sin(half);
            m.bodies[0].initial_pose.rotation = {v[0], v[1], v[2], std::cos(half)};
            m.bodies[0].initial_pose.translation = {-252.55719832993879, 37, -81};
            const auto r = Solver{}.solve(m, intent());
            ASSERT_EQ(r.status, SolveStatus::Converged) << r.diagnostic;
            ASSERT_EQ(r.components[0].preference.status, PreferenceStatus::Converged);
        }
    }
}

TEST(AssemblyMotion, FixedFirstAntipodalPlaneSeedsFreeReference) {
    auto m = face4_face6_regression();
    m.bodies[0].initial_pose.rotation = {std::sqrt(0.5), 0, 0, std::sqrt(0.5)};
    auto& join = m.constraints.back();
    const auto selected = join.first;
    join.first = *join.second;
    join.second = selected;
    SolverOptions options;
    options.solve_intent =
        SolveIntent{{"b"}, {"a"}, SolvePreferencePolicy::MoveFirstMinimizeReference};
    const auto result = Solver{}.solve(m, options);
    ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
    EXPECT_EQ(result.components[0].preference.status, PreferenceStatus::Converged);
}

TEST(AssemblyMotion, OffsetPerpendicularCylindersPreserveReferenceAndConverge) {
    for (const double offset : {25.0, 250.0, 1000.0}) {
        for (const bool reverse_roles : {false, true}) {
            SCOPED_TRACE(offset);
            SCOPED_TRACE(reverse_roles);
            Model model;
            model.bodies = {{"a", {}}, {"b", {}}};
            model.geometry = {{"c", "a", CylinderGeometry{{0, -offset, 10}, {-1, 0, 0}, 74.15842513149224}},
                              {"c", "b", CylinderGeometry{{}, {0, 0, -1}, 28.284271247461902}}};
            auto c = mate("concentric", ConstraintKind::Concentric, {"a", "c"}, {"b", "c"});
            c.direction_relation = DirectionRelation::Unoriented;
            model.constraints = {c};
            auto options = intent();
            if (reverse_roles)
                options.solve_intent = SolveIntent{{"b"}, {"a"}, SolvePreferencePolicy::MoveFirstMinimizeReference};
            const auto result = Solver{}.solve(model, options);
            ASSERT_EQ(result.status, SolveStatus::Converged) << result.diagnostic;
            certified(result);
            EXPECT_NEAR(result.components[0].preference.reference_objective, 0.0, 1e-12);
            EXPECT_NEAR(result.components[0].preference.total_objective,
                        offset * offset + 100 + std::pow(std::acos(-1.0) / 2, 2), 1e-7);
        }
    }
}

TEST(AssemblyMotion, ActualWindmillCylinderConstraintPreservesReferenceOptimum) {
 Model m;
 m.bodies.push_back({"instance-28a791a16a4cc0569c9a",{{78.28273815544775,0,0},{0,0,0,1}}});
 m.bodies.push_back({"instance-397e904441a02023f36e",{{-238.8774706446041,8.581127052453197e-16,72.49038344517723},{-0.9330127762592088,-0.2499998234046462,0.24999991898044924,0.06698722374078664}}});
 m.bodies.push_back({"instance-85429570771c100c22ca",{{-246.12746810936875,1.2255677699061126e-18,59.93301362658437},{0.25881894759604973,5.086359740301957e-08,0.9659258524158232,1.8982560629592644e-07}}});
 m.bodies.push_back({"instance-bc824134e902ae300fb4",{{-250.70759554559956,0,0},{0,0,0,1}}});
 m.bodies.push_back({"instance-f2ee71eed0a51b14abd6",{{-230.05276229924706,-9.569912297784242,73.74629030279985},{-1.8108185866175285e-15,0.2588189475960547,-6.648706345408536e-15,0.965925852415842}}});
 m.geometry.push_back({"g000","instance-397e904441a02023f36e",PlaneGeometry{{0,0,12.5},{0,0,1}}});
 m.geometry.push_back({"g001","instance-397e904441a02023f36e",CylinderGeometry{{0,0,0},{0,0,-1},12.2,-1}});
 m.geometry.push_back({"g002","instance-397e904441a02023f36e",CylinderGeometry{{-7.144709581221619,-4.125,12.5},{0,0,1},0.85,-1}});
 m.geometry.push_back({"g003","instance-397e904441a02023f36e",CylinderGeometry{{0,0,-0.7},{0,0,-1},1,1}});
 m.geometry.push_back({"g004","instance-397e904441a02023f36e",PlaneGeometry{{0,0,18.5},{0,0,1}}});
 m.geometry.push_back({"g005","instance-85429570771c100c22ca",PlaneGeometry{{0,0,1.9999999999999964},{0,0,1}}});
 m.geometry.push_back({"g006","instance-85429570771c100c22ca",CylinderGeometry{{0,0,20.000020100000203},{0,0,1},3.25,-1}});
 m.geometry.push_back({"g007","instance-85429570771c100c22ca",CylinderGeometry{{8.25,0,20.000020100000203},{0,0,1},0.9,-1}});
 m.geometry.push_back({"g008","instance-85429570771c100c22ca",PlaneGeometry{{0,0,20},{0,0,1}}});
 m.geometry.push_back({"g009","instance-85429570771c100c22ca",CylinderGeometry{{10.969655114635927,-18.999999999971166,20},{0,0,1},1.5,-1}});
 m.geometry.push_back({"g010","instance-bc824134e902ae300fb4",CylinderGeometry{{4.33012752365271,0,59.5000008742191},{0.4999998251561808,5.760052454432016e-17,0.8660255047305413},1,-1}});
 m.geometry.push_back({"g011","instance-bc824134e902ae300fb4",PlaneGeometry{{1.5096617415105655,-3.8042260651806146,57.08694538029326},{0.4999998251561808,5.760052454432016e-17,0.8660255047305413}}});
 m.geometry.push_back({"g012","instance-f2ee71eed0a51b14abd6",PlaneGeometry{{26.55811238272279,-1.4210854715202004e-14,0},{0,0,-1}}});
 m.geometry.push_back({"g013","instance-f2ee71eed0a51b14abd6",CylinderGeometry{{-10.969655114602885,19,0},{0,0,1},1.5,1}});
 { Constraint c; c.id="assembly-constraint-26078cbe-8c9e-5a3f-99fc-e04f5bf393eb";c.kind=ConstraintKind::Coincident;
 c.first={"instance-397e904441a02023f36e","g000"};
 c.second=GeometryRef{"instance-85429570771c100c22ca","g005"};
 c.direction_relation=DirectionRelation::Opposite;m.constraints.push_back(c); }
 { Constraint c; c.id="assembly-constraint-41df2941-39ac-554d-bab8-d98745cfa5cd";c.kind=ConstraintKind::Concentric;
 c.first={"instance-397e904441a02023f36e","g001"};
 c.second=GeometryRef{"instance-85429570771c100c22ca","g006"};
 c.direction_relation=DirectionRelation::Unoriented;m.constraints.push_back(c); }
 { Constraint c; c.id="assembly-constraint-639f408d-04ca-585b-b768-d35169622baf";c.kind=ConstraintKind::Concentric;
 c.first={"instance-397e904441a02023f36e","g003"};
 c.second=GeometryRef{"instance-bc824134e902ae300fb4","g010"};
 c.direction_relation=DirectionRelation::Unoriented;m.constraints.push_back(c); }
 { Constraint c; c.id="assembly-constraint-99e907ae-23fc-5ec2-8943-9d8b0eb7bae5";c.kind=ConstraintKind::Concentric;
 c.first={"instance-f2ee71eed0a51b14abd6","g013"};
 c.second=GeometryRef{"instance-85429570771c100c22ca","g009"};
 c.direction_relation=DirectionRelation::Unoriented;m.constraints.push_back(c); }
 { Constraint c; c.id="assembly-constraint-acfac830-1734-54fd-acb3-620002c69370";c.kind=ConstraintKind::Concentric;
 c.first={"instance-397e904441a02023f36e","g002"};
 c.second=GeometryRef{"instance-85429570771c100c22ca","g007"};
 c.direction_relation=DirectionRelation::Unoriented;m.constraints.push_back(c); }
 { Constraint c; c.id="assembly-constraint-b8eddae1-359d-5976-9b1b-d75a4428cb77";c.kind=ConstraintKind::Coincident;
 c.first={"instance-397e904441a02023f36e","g004"};
 c.second=GeometryRef{"instance-bc824134e902ae300fb4","g011"};
 c.direction_relation=DirectionRelation::Opposite;m.constraints.push_back(c); }
 { Constraint c; c.id="assembly-constraint-e0d1520e-39ea-5c19-a85f-b72e065d1e5d";c.kind=ConstraintKind::Fix;
 c.first={"instance-bc824134e902ae300fb4",""};
 c.fixed_pose=Pose{{-250.70759554559956,0,0},{0,0,0,1}};
 c.direction_relation=DirectionRelation::Unoriented;m.constraints.push_back(c); }
 { Constraint c; c.id="assembly-constraint-e1a2b2d7-489b-57a7-bebe-153afbbca3c7";c.kind=ConstraintKind::Coincident;
 c.first={"instance-f2ee71eed0a51b14abd6","g012"};
 c.second=GeometryRef{"instance-85429570771c100c22ca","g008"};
 c.direction_relation=DirectionRelation::Opposite;m.constraints.push_back(c); }
 SolverOptions o; o.solve_intent=SolveIntent{{"instance-f2ee71eed0a51b14abd6"},{"instance-85429570771c100c22ca"},SolvePreferencePolicy::MoveFirstMinimizeReference};
 const auto result=Solver{}.solve(m,o);certified(result);
 for (const auto& component : result.components) {
     if (component.solved) {
         EXPECT_LT(component.preference.reference_objective, 1e-16);
     }
 }
}
