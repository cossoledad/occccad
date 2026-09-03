#pragma once

#include <array>
#include <cstddef>
#include <cstdint>
#include <optional>
#include <string>
#include <variant>
#include <vector>

namespace occccad::assembly {

struct Vec3 {
    double x{};
    double y{};
    double z{};
};

struct Quaternion {
    double x{};
    double y{};
    double z{};
    double w{1.0};
};

// Right-handed rigid transform. A local point is mapped as R * p + t.
struct Pose {
    Vec3 translation{};
    Quaternion rotation{};
};

struct PointGeometry {
    Vec3 position{};
};

struct AxisGeometry {
    Vec3 origin{};
    Vec3 direction{0.0, 0.0, 1.0};
};

struct PlaneGeometry {
    Vec3 origin{};
    Vec3 normal{0.0, 0.0, 1.0};
};

struct CylinderGeometry {
    Vec3 axis_origin{};
    Vec3 axis_direction{0.0, 0.0, 1.0};
    double radius{1.0};
};

using Geometry = std::variant<PointGeometry, AxisGeometry, PlaneGeometry, CylinderGeometry>;

struct Body {
    std::string id;
    Pose initial_pose{};
};

struct GeometryElement {
    std::string id;
    std::string body_id;
    Geometry local_geometry;
};

struct GeometryRef {
    std::string body_id;
    std::string geometry_id;
};

enum class ConstraintKind { Fix, Rigid, Coincident, Concentric, Angle, Distance };

enum class ConstraintMode { Driving, Measured, Controlled, Suppressed };

// For alignment constraints Unoriented chooses the nearest same/opposite branch.
// Angle always measures the full [0, pi] separation; Same/Opposite explicitly
// controls which endpoint direction is used before measuring it.
enum class DirectionRelation { Unoriented, Same, Opposite };

// Distance to a plane and plane-to-plane distance require an explicit side
// when sign matters. Unsigned uses the nearest absolute-distance branch.
enum class DistanceRelation { Unsigned, AlongSecondNormal, OppositeSecondNormal };

struct AngleBranchState {
    double wrapped_angle{};
    double unwrapped_angle{};
    std::int64_t winding{};
};

struct Constraint {
    std::string id;
    std::string connection_id;
    ConstraintKind kind{ConstraintKind::Coincident};
    ConstraintMode mode{ConstraintMode::Driving};
    GeometryRef first;
    std::optional<GeometryRef> second;
    double value{};  // radians for Angle, model length for Distance
    // Optional reference direction expressed in the second body's local frame.
    // When present, Angle is directed in [0, 2pi); otherwise it is unsigned [0, pi].
    std::optional<Vec3> angle_reference_direction;
    // Previous accepted directed-angle branch. The solver chooses the nearest
    // equivalent angle and returns the updated state in SolveResult.
    std::optional<AngleBranchState> angle_branch_state;
    DirectionRelation direction_relation{DirectionRelation::Unoriented};
    DistanceRelation distance_relation{DistanceRelation::Unsigned};
    // Fix: target world pose. Rigid: target pose of first relative to second.
    std::optional<Pose> fixed_pose;
};

struct Model {
    std::vector<Body> bodies;
    std::vector<GeometryElement> geometry;
    std::vector<Constraint> constraints;
};

enum class SolvePreferencePolicy { MinimumTotalChange, MoveFirstMinimizeReference };

// Request-scoped placement intent. Endpoint order does not make the persisted
// geometric constraint asymmetric; it only selects a deterministic solution
// from the feasible family for this solve.
struct SolveIntent {
    std::vector<std::string> moving_body_ids;
    std::vector<std::string> reference_body_ids;
    SolvePreferencePolicy policy{SolvePreferencePolicy::MinimumTotalChange};
};

struct SolverOptions {
    std::size_t max_iterations{100};
    double length_tolerance{1.0e-7};
    double angle_tolerance{1.0e-8};
    double classification_length_tolerance{1.0e-7};
    double classification_angle_tolerance{1.0e-8};
    double translation_step_tolerance{1.0e-9};
    double rotation_step_tolerance{1.0e-10};
    double degeneracy_tolerance{1.0e-8};
    // Deprecated compatibility override. When positive it overrides both
    // translation/rotation finite-difference steps.
    double finite_difference_step{};
    double translation_finite_difference_step{1.0e-6};
    double rotation_finite_difference_step{1.0e-7};
    double initial_damping{1.0e-4};
    double length_scale{1.0};
    double angle_scale{1.0};
    // Deprecated compatibility absolute rank threshold.
    double rank_tolerance{};
    double rank_absolute_tolerance{1.0e-10};
    double rank_relative_tolerance{1.0e-8};
    double gradient_tolerance{1.0e-8};
    double moving_preference_weight{1.0e-14};
    double neutral_preference_weight{1.0e-12};
    double reference_preference_weight{1.0e-8};
    std::size_t max_conflict_probes{16};
#ifdef NDEBUG
    bool verify_analytic_jacobians{false};
#else
    bool verify_analytic_jacobians{true};
#endif
    double jacobian_check_tolerance{1.0e-5};
    // Empty solves every connected component. Otherwise only components
    // containing at least one listed body are numerically updated.
    std::vector<std::string> affected_body_ids;
    std::optional<SolveIntent> solve_intent;
};

enum class SolveStatus {
    Converged,
    Unsatisfied,
    Inconsistent,
    MaxIterations,
    InvalidModel,
    NumericalFailure
};

enum class SolveClassification {
    SolvedFully,
    SolvedUnderConstrained,
    Redundant,
    Unsatisfied,
    Inconsistent,
    InvalidModel,
    NonConvergent
};

struct ConstraintResidual {
    std::string constraint_id;
    double normalized_norm{};
};

enum class ConstraintRankRole { Independent, PartiallyRedundant, FullyRedundant };

struct ConstraintRankInfo {
    std::string constraint_id;
    std::size_t equation_count{};
    std::size_t effective_rank{};
    std::size_t incremental_rank{};
    std::size_t declared_generic_rank{};
    ConstraintRankRole role{ConstraintRankRole::Independent};
};

struct SolvedAngleBranch {
    std::string constraint_id;
    AngleBranchState state;
};

struct EquationResidual {
    std::string equation_id;
    std::string connection_id;
    std::string constraint_id;
    std::size_t equation_index{};
    double normalized_value{};
};

struct ComponentDof {
    std::string component_id;
    std::vector<std::string> body_ids;
    std::size_t tangent_variable_count{};
    std::size_t jacobian_rank{};
    std::size_t relative_dof{};
    std::size_t gauge_dof{};
    bool solved{true};
    // Stable free-cluster tangent ordering used by every basis vector. Each
    // cluster contributes [tx, ty, tz, rx, ry, rz].
    std::vector<std::string> tangent_cluster_ids;
    std::vector<std::vector<double>> null_space_basis;
    std::vector<double> singular_values;
    double rank_threshold{};
};

struct SolveDiagnostic {
    std::string code;
    std::string component_id;
    std::vector<std::string> body_ids;
    std::vector<std::string> constraint_ids;
    std::string detail;
};

struct SolvedBody {
    std::string id;
    Pose pose{};
};

struct SolveResult {
    SolveStatus status{SolveStatus::InvalidModel};
    SolveClassification classification{SolveClassification::InvalidModel};
    std::vector<SolvedBody> bodies;
    std::vector<ConstraintResidual> residuals;
    std::vector<EquationResidual> equation_residuals;
    std::vector<ConstraintRankInfo> constraint_ranks;
    std::vector<SolvedAngleBranch> angle_branches;
    std::vector<ComponentDof> components;
    std::vector<std::string> redundant_constraint_ids;
    std::vector<std::string> unsatisfied_constraint_ids;
    std::vector<std::string> conflicting_constraint_ids;
    std::vector<std::string> suspected_conflicting_constraint_ids;
    std::vector<SolveDiagnostic> diagnostics;
    std::size_t iterations{};
    double normalized_residual{};
    std::string diagnostic;
};

class Solver final {
public:
    SolveResult solve(const Model& model, const SolverOptions& options = {}) const;
};

}  // namespace occccad::assembly
