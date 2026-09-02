#pragma once

#include <array>
#include <cstddef>
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

struct Constraint {
    std::string id;
    std::string connection_id;
    ConstraintKind kind{ConstraintKind::Coincident};
    ConstraintMode mode{ConstraintMode::Driving};
    GeometryRef first;
    std::optional<GeometryRef> second;
    double value{};  // radians for Angle, model length for Distance
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
    double translation_step_tolerance{1.0e-9};
    double rotation_step_tolerance{1.0e-10};
    double degeneracy_tolerance{1.0e-8};
    double finite_difference_step{1.0e-7};
    double initial_damping{1.0e-4};
    double length_scale{1.0};
    double angle_scale{1.0};
    double rank_tolerance{1.0e-9};
    // Empty solves every connected component. Otherwise only components
    // containing at least one listed body are numerically updated.
    std::vector<std::string> affected_body_ids;
    std::optional<SolveIntent> solve_intent;
};

enum class SolveStatus { Converged, Unsatisfied, MaxIterations, InvalidModel, NumericalFailure };

enum class SolveClassification {
    SolvedFully,
    SolvedUnderConstrained,
    Redundant,
    Inconsistent,
    InvalidModel,
    NonConvergent
};

struct ConstraintResidual {
    std::string constraint_id;
    double normalized_norm{};
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
    std::vector<ComponentDof> components;
    std::vector<std::string> redundant_constraint_ids;
    std::vector<std::string> conflicting_constraint_ids;
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
