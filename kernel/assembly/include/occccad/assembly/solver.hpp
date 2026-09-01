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

// Unoriented chooses the same/opposite branch nearest the current iterate.
enum class DirectionRelation { Unoriented, Same, Opposite };

// Distance to a plane and plane-to-plane distance require an explicit side
// when sign matters. Unsigned uses the nearest absolute-distance branch.
enum class DistanceRelation { Unsigned, AlongSecondNormal, OppositeSecondNormal };

struct Constraint {
    std::string id;
    ConstraintKind kind{ConstraintKind::Coincident};
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

struct SolverOptions {
    std::size_t max_iterations{100};
    double residual_tolerance{1.0e-9};
    double step_tolerance{1.0e-10};
    double finite_difference_step{1.0e-7};
    double initial_damping{1.0e-4};
    double length_scale{1.0};
    double angle_scale{1.0};
};

enum class SolveStatus { Converged, MaxIterations, InvalidModel, NumericalFailure };

struct ConstraintResidual {
    std::string constraint_id;
    double normalized_norm{};
};

struct SolvedBody {
    std::string id;
    Pose pose{};
};

struct SolveResult {
    SolveStatus status{SolveStatus::InvalidModel};
    std::vector<SolvedBody> bodies;
    std::vector<ConstraintResidual> residuals;
    std::size_t iterations{};
    double normalized_residual{};
    std::string diagnostic;
};

class Solver final {
public:
    SolveResult solve(const Model& model, const SolverOptions& options = {}) const;
};

}  // namespace occccad::assembly
