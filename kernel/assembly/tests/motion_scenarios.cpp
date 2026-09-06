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
