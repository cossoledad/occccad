#include <occccad/assembly/solver.hpp>

#include <Eigen/Cholesky>
#include <Eigen/Core>
#include <Eigen/Geometry>
#include <Eigen/LU>
#include <Eigen/QR>
#include <Eigen/SVD>
#include <algorithm>
#include <cmath>
#include <limits>
#include <numeric>
#include <queue>
#include <set>
#include <stdexcept>
#include <type_traits>
#include <unordered_map>
#include <unordered_set>
#include <utility>

namespace occccad::assembly {
namespace {

using Vector = Eigen::VectorXd;
using Vector3 = Eigen::Vector3d;
using EigenQuaternion = Eigen::Quaterniond;

constexpr double kDirectionEpsilon = 1.0e-12;
constexpr double kPi = 3.141592653589793238462643383279502884;

struct WorldPoint {
    Vector3 position;
};
struct WorldAxis {
    Vector3 origin;
    Vector3 direction;
};
struct WorldPlane {
    Vector3 origin;
    Vector3 normal;
};
struct WorldCylinder {
    Vector3 origin;
    Vector3 direction;
    double radius;
};
using WorldGeometry = std::variant<WorldPoint, WorldAxis, WorldPlane, WorldCylinder>;

struct ResidualBlock {
    std::string id;
    Eigen::VectorXd values;
    Eigen::VectorXd tolerances;
    double satisfaction_ratio{};
    std::vector<std::string> equation_kinds;
    std::size_t declared_generic_rank{};
};

struct ConstraintBranchState {
    DirectionRelation direction_relation{DirectionRelation::Unoriented};
    DistanceRelation distance_relation{DistanceRelation::Unsigned};
    std::optional<Vec3> unsigned_angle_axis_in_second;
    std::optional<AngleBranchState> angle;
};

struct State {
    std::vector<Pose> poses;
};

Vector3 eigen(const Vec3& value) {
    return {value.x, value.y, value.z};
}
Vec3 value(const Vector3& input) {
    return {input.x(), input.y(), input.z()};
}

EigenQuaternion eigen(const Quaternion& value) {
    return {value.w, value.x, value.y, value.z};
}

Quaternion value(EigenQuaternion input) {
    input.normalize();
    if (input.w() < 0.0)
        input.coeffs() *= -1.0;
    return {input.x(), input.y(), input.z(), input.w()};
}

bool finite(const double input) {
    return std::isfinite(input);
}
bool finite(const Vec3& input) {
    return finite(input.x) && finite(input.y) && finite(input.z);
}
bool finite(const Quaternion& input) {
    return finite(input.x) && finite(input.y) && finite(input.z) && finite(input.w);
}

EigenQuaternion normalized(const Quaternion& input) {
    EigenQuaternion result = eigen(input);
    if (!result.coeffs().allFinite() || result.norm() <= kDirectionEpsilon)
        throw std::invalid_argument("body rotation quaternion is invalid");
    result.normalize();
    return result;
}

Vector3 normalized(const Vec3& input, const char* label) {
    Vector3 result = eigen(input);
    if (!result.allFinite() || result.norm() <= kDirectionEpsilon)
        throw std::invalid_argument(std::string(label) + " direction is invalid");
    return result.normalized();
}

Vector3 rotation_vector(const EigenQuaternion& input) {
    EigenQuaternion q = input.normalized();
    if (q.w() < 0.0)
        q.coeffs() *= -1.0;
    const double sin_half = q.vec().norm();
    if (sin_half <= 1.0e-15)
        return 2.0 * q.vec();
    return q.vec() * (2.0 * std::atan2(sin_half, std::clamp(q.w(), -1.0, 1.0)) / sin_half);
}

EigenQuaternion exponential(const Vector3& input) {
    const double angle = input.norm();
    if (angle <= 1.0e-15)
        return EigenQuaternion(1.0, input.x() * 0.5, input.y() * 0.5, input.z() * 0.5).normalized();
    return EigenQuaternion(Eigen::AngleAxisd(angle, input / angle));
}

Vector3 position(const Pose& pose, const Vec3& local) {
    return normalized(pose.rotation) * eigen(local) + eigen(pose.translation);
}

Vector3 direction(const Pose& pose, const Vec3& local, const char* label) {
    return normalized(pose.rotation) * normalized(local, label);
}

std::string geometry_key(const std::string& body, const std::string& geometry) {
    return body + '\x1f' + geometry;
}

WorldGeometry world_geometry(const GeometryElement& element, const Pose& pose) {
    return std::visit(
        [&](const auto& local) -> WorldGeometry {
            using T = std::decay_t<decltype(local)>;
            if constexpr (std::is_same_v<T, PointGeometry>) {
                return WorldPoint{position(pose, local.position)};
            } else if constexpr (std::is_same_v<T, AxisGeometry>) {
                return WorldAxis{position(pose, local.origin),
                                 direction(pose, local.direction, "axis")};
            } else if constexpr (std::is_same_v<T, PlaneGeometry>) {
                return WorldPlane{position(pose, local.origin),
                                  direction(pose, local.normal, "plane")};
            } else {
                return WorldCylinder{position(pose, local.axis_origin),
                                     direction(pose, local.axis_direction, "cylinder"),
                                     local.radius};
            }
        },
        element.local_geometry);
}

bool is_axis_like(const WorldGeometry& geometry) {
    return std::holds_alternative<WorldAxis>(geometry) ||
           std::holds_alternative<WorldCylinder>(geometry);
}

WorldAxis as_axis(const WorldGeometry& geometry) {
    if (const auto* axis = std::get_if<WorldAxis>(&geometry))
        return *axis;
    if (const auto* cylinder = std::get_if<WorldCylinder>(&geometry))
        return {cylinder->origin, cylinder->direction};
    throw std::invalid_argument("geometry is not axis-like");
}

bool has_direction(const WorldGeometry& geometry) {
    return is_axis_like(geometry) || std::holds_alternative<WorldPlane>(geometry);
}

enum class DescriptorKind { Point, Axis, Plane, Cylinder };

DescriptorKind descriptor_kind(const WorldGeometry& geometry) {
    if (std::holds_alternative<WorldPoint>(geometry))
        return DescriptorKind::Point;
    if (std::holds_alternative<WorldAxis>(geometry))
        return DescriptorKind::Axis;
    if (std::holds_alternative<WorldPlane>(geometry))
        return DescriptorKind::Plane;
    return DescriptorKind::Cylinder;
}

bool axis_descriptor(const DescriptorKind kind) {
    return kind == DescriptorKind::Axis || kind == DescriptorKind::Cylinder;
}

struct EquationDefinition {
    std::vector<std::string> kinds;
    std::size_t generic_rank{};
};

Eigen::Matrix3d skew(const Vector3& value);

struct DifferentialScalar {
    double value{};
    Eigen::RowVectorXd derivative;
};

struct DifferentialVector {
    Vector3 value;
    Eigen::MatrixXd derivative;
};

DifferentialScalar scalar(const double value, const Eigen::Index variables) {
    return {value, Eigen::RowVectorXd::Zero(variables)};
}

DifferentialVector vector(const Vector3& value, const Eigen::Index variables) {
    return {value, Eigen::MatrixXd::Zero(3, variables)};
}

DifferentialVector operator-(const DifferentialVector& a, const DifferentialVector& b) {
    return {a.value - b.value, a.derivative - b.derivative};
}
DifferentialVector operator-(const DifferentialVector& input) {
    return {-input.value, -input.derivative};
}
DifferentialScalar operator+(const DifferentialScalar& a, const DifferentialScalar& b) {
    return {a.value + b.value, a.derivative + b.derivative};
}
DifferentialScalar operator-(const DifferentialScalar& a, const DifferentialScalar& b) {
    return {a.value - b.value, a.derivative - b.derivative};
}
DifferentialScalar operator*(const DifferentialScalar& a, const DifferentialScalar& b) {
    return {a.value * b.value, b.value * a.derivative + a.value * b.derivative};
}
DifferentialScalar operator/(const DifferentialScalar& a, const DifferentialScalar& b) {
    return {a.value / b.value,
            (b.value * a.derivative - a.value * b.derivative) / (b.value * b.value)};
}
DifferentialVector operator*(const DifferentialScalar& scale, const DifferentialVector& input) {
    return {scale.value * input.value,
            scale.value * input.derivative + input.value * scale.derivative};
}
DifferentialVector divided(const DifferentialVector& input, const double scale) {
    return {input.value / scale, input.derivative / scale};
}
DifferentialScalar divided(const DifferentialScalar& input, const double scale) {
    return {input.value / scale, input.derivative / scale};
}
DifferentialScalar dot(const DifferentialVector& a, const DifferentialVector& b) {
    return {a.value.dot(b.value),
            b.value.transpose() * a.derivative + a.value.transpose() * b.derivative};
}
DifferentialVector cross(const DifferentialVector& a, const DifferentialVector& b) {
    return {a.value.cross(b.value), -skew(b.value) * a.derivative + skew(a.value) * b.derivative};
}
DifferentialScalar norm(const DifferentialVector& input) {
    const double length = input.value.norm();
    Eigen::RowVectorXd derivative = Eigen::RowVectorXd::Zero(input.derivative.cols());
    if (length > kDirectionEpsilon)
        derivative = input.value.transpose() * input.derivative / length;
    return {length, std::move(derivative)};
}
DifferentialVector normalized(const DifferentialVector& input) {
    const double length = input.value.norm();
    if (length <= kDirectionEpsilon)
        throw std::invalid_argument("differentiated direction is degenerate");
    const Vector3 unit = input.value / length;
    return {unit, (Eigen::Matrix3d::Identity() - unit * unit.transpose()) * input.derivative /
                      length};
}
DifferentialScalar absolute(const DifferentialScalar& input) {
    const double sign = input.value > 0.0 ? 1.0 : input.value < 0.0 ? -1.0 : 0.0;
    return {std::abs(input.value), sign * input.derivative};
}
DifferentialScalar differentiated_atan2(const DifferentialScalar& y,
                                        const DifferentialScalar& x) {
    const double denominator = x.value * x.value + y.value * y.value;
    if (denominator <= kDirectionEpsilon * kDirectionEpsilon)
        throw std::invalid_argument("atan2 differential is degenerate");
    return {std::atan2(y.value, x.value),
            (x.value * y.derivative - y.value * x.derivative) / denominator};
}

struct DifferentialPoint { DifferentialVector position; };
struct DifferentialAxis { DifferentialVector origin; DifferentialVector direction; };
struct DifferentialPlane { DifferentialVector origin; DifferentialVector normal; };
struct DifferentialCylinder {
    DifferentialVector origin;
    DifferentialVector direction;
    double radius{};
};
using DifferentialGeometry =
    std::variant<DifferentialPoint, DifferentialAxis, DifferentialPlane, DifferentialCylinder>;

bool differential_axis_like(const DifferentialGeometry& geometry) {
    return std::holds_alternative<DifferentialAxis>(geometry) ||
           std::holds_alternative<DifferentialCylinder>(geometry);
}
DifferentialAxis differential_axis(const DifferentialGeometry& geometry) {
    if (const auto* axis = std::get_if<DifferentialAxis>(&geometry))
        return *axis;
    const auto& cylinder = std::get<DifferentialCylinder>(geometry);
    return {cylinder.origin, cylinder.direction};
}
DifferentialVector differential_direction(const DifferentialGeometry& geometry) {
    if (differential_axis_like(geometry))
        return differential_axis(geometry).direction;
    return std::get<DifferentialPlane>(geometry).normal;
}

EquationDefinition equation_definition(const Constraint& constraint,
                                       const WorldGeometry& first,
                                       const WorldGeometry& second) {
    const DescriptorKind a = descriptor_kind(first);
    const DescriptorKind b = descriptor_kind(second);
    if (constraint.kind == ConstraintKind::Coincident) {
        if (a == DescriptorKind::Point && b == DescriptorKind::Point)
            return {{"POSITION_X", "POSITION_Y", "POSITION_Z"}, 3};
        if ((a == DescriptorKind::Point && axis_descriptor(b)) ||
            (b == DescriptorKind::Point && axis_descriptor(a)))
            return {{"POINT_LINE_X", "POINT_LINE_Y", "POINT_LINE_Z"}, 2};
        if ((a == DescriptorKind::Point && b == DescriptorKind::Plane) ||
            (b == DescriptorKind::Point && a == DescriptorKind::Plane))
            return {{"POINT_PLANE_DISTANCE"}, 1};
        if (axis_descriptor(a) && axis_descriptor(b)) {
            std::vector<std::string> kinds{"DIRECTION_X", "DIRECTION_Y", "DIRECTION_Z",
                                           "LINE_OFFSET_X", "LINE_OFFSET_Y", "LINE_OFFSET_Z"};
            if (a == DescriptorKind::Cylinder && b == DescriptorKind::Cylinder)
                kinds.push_back("CYLINDER_RADIUS");
            return {std::move(kinds), 4};
        }
        if (a == DescriptorKind::Plane && b == DescriptorKind::Plane)
            return {{"NORMAL_X", "NORMAL_Y", "NORMAL_Z", "PLANE_OFFSET"}, 3};
        if ((axis_descriptor(a) && b == DescriptorKind::Plane) ||
            (axis_descriptor(b) && a == DescriptorKind::Plane))
            return {{"LINE_PLANE_DIRECTION", "LINE_PLANE_OFFSET"}, 2};
    }
    if (constraint.kind == ConstraintKind::Concentric && axis_descriptor(a) && axis_descriptor(b))
        return {{"DIRECTION_X", "DIRECTION_Y", "DIRECTION_Z", "LINE_OFFSET_X",
                 "LINE_OFFSET_Y", "LINE_OFFSET_Z"}, 4};
    if (constraint.kind == ConstraintKind::Angle && has_direction(first) &&
        has_direction(second))
        return {{constraint.angle_reference_direction ? "DIRECTED_ANGLE" : "UNSIGNED_ANGLE"}, 1};
    if (constraint.kind == ConstraintKind::Distance) {
        if (a == DescriptorKind::Point && b == DescriptorKind::Point)
            return {{"POINT_POINT_DISTANCE"}, 1};
        if ((a == DescriptorKind::Point && b == DescriptorKind::Plane) ||
            (b == DescriptorKind::Point && a == DescriptorKind::Plane))
            return {{"POINT_PLANE_DISTANCE"}, 1};
        if (axis_descriptor(a) && axis_descriptor(b))
            return {{"LINE_LINE_DISTANCE"}, 1};
        if (a == DescriptorKind::Plane && b == DescriptorKind::Plane)
            return {{"NORMAL_X", "NORMAL_Y", "NORMAL_Z", "PLANE_DISTANCE"}, 3};
    }
    throw std::invalid_argument("unsupported constraint and descriptor pair");
}

Vector3 geometry_direction(const WorldGeometry& geometry) {
    if (is_axis_like(geometry))
        return as_axis(geometry).direction;
    if (const auto* plane = std::get_if<WorldPlane>(&geometry))
        return plane->normal;
    throw std::invalid_argument("geometry has no direction");
}

Vector3 geometry_origin(const WorldGeometry& geometry) {
    if (is_axis_like(geometry))
        return as_axis(geometry).origin;
    if (const auto* plane = std::get_if<WorldPlane>(&geometry))
        return plane->origin;
    throw std::invalid_argument("geometry has no direction origin");
}

Vector3 related_direction(Vector3 first, const Vector3& second, const DirectionRelation relation) {
    if (relation == DirectionRelation::Opposite)
        return -first;
    if (relation == DirectionRelation::Unoriented && first.dot(second) < 0.0)
        return -first;
    return first;
}

Eigen::VectorXd concatenate(const Vector3& first, const Vector3& second) {
    Eigen::VectorXd result(6);
    result << first, second;
    return result;
}

Eigen::VectorXd axis_alignment(const WorldAxis& first, const WorldAxis& second,
                               const DirectionRelation relation, const SolverOptions& options) {
    const Vector3 direction_a = related_direction(first.direction, second.direction, relation);
    return concatenate(
        (direction_a - second.direction) / options.angle_scale,
        (first.origin - second.origin).cross(second.direction) / options.length_scale);
}

double line_distance(const WorldAxis& first, const WorldAxis& second,
                     const double degeneracy_tolerance) {
    const Vector3 cross = first.direction.cross(second.direction);
    const double cross_norm = cross.norm();
    const Vector3 delta = first.origin - second.origin;
    const double parallel = delta.cross(second.direction).norm();
    if (cross_norm <= kDirectionEpsilon)
        return parallel;
    const double skew = std::abs(delta.dot(cross)) / cross_norm;
    const double square = cross_norm * cross_norm;
    const double epsilon_square = degeneracy_tolerance * degeneracy_tolerance;
    const double skew_weight = square / (square + epsilon_square);
    return skew_weight * skew + (1.0 - skew_weight) * parallel;
}

Eigen::VectorXd single(const double value) {
    Eigen::VectorXd result(1);
    result[0] = value;
    return result;
}

double wrap_two_pi(double angle) {
    angle = std::fmod(angle, 2.0 * kPi);
    return angle < 0.0 ? angle + 2.0 * kPi : angle;
}

double wrapped_angle_error(const double current, const double target) {
    return std::atan2(std::sin(current - target), std::cos(current - target));
}

Vector3 perpendicular_to(const Vector3& direction) {
    const Vector3 basis = std::abs(direction.x()) <= std::abs(direction.y()) &&
                                  std::abs(direction.x()) <= std::abs(direction.z())
                              ? Vector3::UnitX()
                          : std::abs(direction.y()) <= std::abs(direction.z()) ? Vector3::UnitY()
                                                                               : Vector3::UnitZ();
    return direction.cross(basis).normalized();
}

Eigen::Matrix3d skew(const Vector3& value) {
    Eigen::Matrix3d result;
    result << 0.0, -value.z(), value.y(), value.z(), 0.0, -value.x(), -value.y(),
        value.x(), 0.0;
    return result;
}

double directed_angle(const Vector3& first, const Vector3& second, const Vector3& reference,
                      const double degeneracy_tolerance) {
    const Vector3 first_projected = first - first.dot(reference) * reference;
    const Vector3 second_projected = second - second.dot(reference) * reference;
    if (first_projected.norm() <= degeneracy_tolerance ||
        second_projected.norm() <= degeneracy_tolerance)
        throw std::invalid_argument(
            "directed angle endpoint direction is parallel to its reference axis");
    const Vector3 a = first_projected.normalized();
    const Vector3 b = second_projected.normalized();
    return wrap_two_pi(std::atan2(reference.dot(a.cross(b)), a.dot(b)));
}

Eigen::VectorXd constraint_residual(const Constraint& constraint, const WorldGeometry& first,
                                    const std::optional<WorldGeometry>& second,
                                    const SolverOptions& options,
                                    const ConstraintBranchState* branch = nullptr) {
    if (constraint.kind == ConstraintKind::Fix || constraint.kind == ConstraintKind::Rigid)
        throw std::invalid_argument("rigid-body residual is evaluated from body poses");
    if (!second)
        throw std::invalid_argument("binary constraint requires a second geometry");
    const DirectionRelation direction_relation =
        branch ? branch->direction_relation : constraint.direction_relation;

    if (constraint.kind == ConstraintKind::Coincident) {
        if (const auto* point_a = std::get_if<WorldPoint>(&first)) {
            if (const auto* point_b = std::get_if<WorldPoint>(&*second))
                return (point_a->position - point_b->position) / options.length_scale;
            if (is_axis_like(*second)) {
                const WorldAxis axis = as_axis(*second);
                return (point_a->position - axis.origin).cross(axis.direction) /
                       options.length_scale;
            }
            if (const auto* plane = std::get_if<WorldPlane>(&*second))
                return single((point_a->position - plane->origin).dot(plane->normal) /
                              options.length_scale);
        }
        if (std::holds_alternative<WorldPoint>(*second)) {
            Constraint swapped = constraint;
            return constraint_residual(swapped, *second, first, options, branch);
        }
        if (is_axis_like(first) && is_axis_like(*second)) {
            Eigen::VectorXd result =
                axis_alignment(as_axis(first), as_axis(*second), direction_relation, options);
            if (const auto* cylinder_a = std::get_if<WorldCylinder>(&first)) {
                if (const auto* cylinder_b = std::get_if<WorldCylinder>(&*second)) {
                    Eigen::VectorXd with_radius(result.size() + 1);
                    with_radius << result,
                        (cylinder_a->radius - cylinder_b->radius) / options.length_scale;
                    return with_radius;
                }
            }
            return result;
        }
        if (const auto* plane_a = std::get_if<WorldPlane>(&first)) {
            if (const auto* plane_b = std::get_if<WorldPlane>(&*second)) {
                const Vector3 normal_a =
                    related_direction(plane_a->normal, plane_b->normal, direction_relation);
                Eigen::VectorXd result(4);
                result << (normal_a - plane_b->normal) / options.angle_scale,
                    (plane_a->origin - plane_b->origin).dot(plane_b->normal) / options.length_scale;
                return result;
            }
        }
        if (is_axis_like(first)) {
            if (const auto* plane = std::get_if<WorldPlane>(&*second)) {
                const WorldAxis axis = as_axis(first);
                Eigen::VectorXd result(2);
                result << axis.direction.dot(plane->normal) / options.angle_scale,
                    (axis.origin - plane->origin).dot(plane->normal) / options.length_scale;
                return result;
            }
        }
        if (std::holds_alternative<WorldPlane>(first) && is_axis_like(*second)) {
            Constraint swapped = constraint;
            return constraint_residual(swapped, *second, first, options, branch);
        }
        throw std::invalid_argument("unsupported Coincident geometry pair");
    }

    if (constraint.kind == ConstraintKind::Concentric) {
        if (!is_axis_like(first) || !is_axis_like(*second))
            throw std::invalid_argument("Concentric requires Axis or Cylinder geometry");
        return axis_alignment(as_axis(first), as_axis(*second), direction_relation, options);
    }

    if (constraint.kind == ConstraintKind::Angle) {
        if (!has_direction(first) || !has_direction(*second))
            throw std::invalid_argument("Angle requires Plane, Axis, or Cylinder geometry");
        Vector3 first_direction = geometry_direction(first);
        const Vector3 second_direction = geometry_direction(*second);
        if (direction_relation == DirectionRelation::Opposite)
            first_direction = -first_direction;
        const double cosine = std::clamp(first_direction.dot(second_direction), -1.0, 1.0);
        const Vector3 cross = first_direction.cross(second_direction);
        const bool directed = constraint.angle_reference_direction.has_value();
        double angle{};
        if (directed) {
            const Vector3 reference =
                normalized(*constraint.angle_reference_direction, "angle reference direction");
            angle = directed_angle(first_direction, second_direction, reference,
                                   options.degeneracy_tolerance);
        } else if (branch && branch->unsigned_angle_axis_in_second) {
            const Vector3 axis =
                normalized(*branch->unsigned_angle_axis_in_second, "unsigned angle branch axis");
            angle = std::atan2(axis.dot(cross), cosine);
        } else {
            angle = std::atan2(cross.norm(), cosine);
        }
        return single(wrapped_angle_error(angle, constraint.value) / options.angle_scale);
    }

    if (constraint.kind == ConstraintKind::Distance) {
        if (const auto* point_a = std::get_if<WorldPoint>(&first)) {
            if (const auto* point_b = std::get_if<WorldPoint>(&*second))
                return single(((point_a->position - point_b->position).norm() - constraint.value) /
                              options.length_scale);
            if (const auto* plane = std::get_if<WorldPlane>(&*second)) {
                double measured = (point_a->position - plane->origin).dot(plane->normal);
                const DistanceRelation distance_relation =
                    branch ? branch->distance_relation : constraint.distance_relation;
                if (distance_relation == DistanceRelation::Unsigned)
                    measured = std::abs(measured);
                if (distance_relation == DistanceRelation::OppositeSecondNormal)
                    measured = -measured;
                return single((measured - constraint.value) / options.length_scale);
            }
        }
        if (std::holds_alternative<WorldPoint>(*second)) {
            Constraint swapped = constraint;
            if (swapped.distance_relation == DistanceRelation::AlongSecondNormal)
                swapped.distance_relation = DistanceRelation::OppositeSecondNormal;
            else if (swapped.distance_relation == DistanceRelation::OppositeSecondNormal)
                swapped.distance_relation = DistanceRelation::AlongSecondNormal;
            ConstraintBranchState swapped_branch = branch ? *branch : ConstraintBranchState{};
            if (swapped_branch.distance_relation == DistanceRelation::AlongSecondNormal)
                swapped_branch.distance_relation = DistanceRelation::OppositeSecondNormal;
            else if (swapped_branch.distance_relation == DistanceRelation::OppositeSecondNormal)
                swapped_branch.distance_relation = DistanceRelation::AlongSecondNormal;
            return constraint_residual(swapped, *second, first, options,
                                       branch ? &swapped_branch : nullptr);
        }
        if (is_axis_like(first) && is_axis_like(*second))
            return single(
                (line_distance(as_axis(first), as_axis(*second), options.degeneracy_tolerance) -
                 constraint.value) /
                options.length_scale);
        if (const auto* plane_a = std::get_if<WorldPlane>(&first)) {
            if (const auto* plane_b = std::get_if<WorldPlane>(&*second)) {
                const Vector3 normal_a =
                    related_direction(plane_a->normal, plane_b->normal, direction_relation);
                double measured = (plane_a->origin - plane_b->origin).dot(plane_b->normal);
                const DistanceRelation distance_relation =
                    branch ? branch->distance_relation : constraint.distance_relation;
                if (distance_relation == DistanceRelation::Unsigned)
                    measured = std::abs(measured);
                if (distance_relation == DistanceRelation::OppositeSecondNormal)
                    measured = -measured;
                Eigen::VectorXd result(4);
                result << (normal_a - plane_b->normal) / options.angle_scale,
                    (measured - constraint.value) / options.length_scale;
                return result;
            }
        }
        throw std::invalid_argument("unsupported Distance geometry pair");
    }
    throw std::invalid_argument("unsupported constraint kind");
}

// Forward analytic differential primitives. They carry a value and derivatives
// with respect to the stable free-cluster tangent ordering.
Eigen::MatrixXd differential_residual(const Constraint& constraint,
                                      DifferentialGeometry first,
                                      DifferentialGeometry second,
                                      const ConstraintBranchState& branch,
                                      const SolverOptions& options,
                                      const std::optional<DifferentialVector>& angle_reference,
                                      const std::optional<DifferentialVector>& unsigned_angle_axis) {
    const Eigen::Index variables = std::visit([](const auto& geometry) {
        if constexpr (std::is_same_v<std::decay_t<decltype(geometry)>, DifferentialPoint>)
            return geometry.position.derivative.cols();
        else
            return geometry.origin.derivative.cols();
    }, first);
    std::vector<DifferentialScalar> rows;
    auto append = [&](const DifferentialVector& value) {
        for (Eigen::Index row = 0; row < 3; ++row)
            rows.push_back({value.value[row], value.derivative.row(row)});
    };
    auto related = [&](DifferentialVector value, const DifferentialVector& reference) {
        if (branch.direction_relation == DirectionRelation::Opposite ||
            (branch.direction_relation == DirectionRelation::Unoriented &&
             value.value.dot(reference.value) < 0.0))
            return -value;
        return value;
    };
    auto align_axes = [&](const DifferentialAxis& a, const DifferentialAxis& b) {
        append(divided(related(a.direction, b.direction) - b.direction, options.angle_scale));
        append(divided(cross(a.origin - b.origin, b.direction), options.length_scale));
    };
    if (constraint.kind == ConstraintKind::Coincident) {
        if (const auto* first_point = std::get_if<DifferentialPoint>(&first)) {
            if (const auto* second_point = std::get_if<DifferentialPoint>(&second))
                append(divided(first_point->position - second_point->position, options.length_scale));
            else if (differential_axis_like(second)) {
                const auto second_axis = differential_axis(second);
                append(divided(cross(first_point->position - second_axis.origin,
                                     second_axis.direction), options.length_scale));
            } else {
                const auto& second_plane = std::get<DifferentialPlane>(second);
                rows.push_back(divided(dot(first_point->position - second_plane.origin,
                                           second_plane.normal), options.length_scale));
            }
        } else if (std::holds_alternative<DifferentialPoint>(second)) {
            return differential_residual(constraint, second, first, branch, options, angle_reference,
                                         unsigned_angle_axis);
        } else if (differential_axis_like(first) && differential_axis_like(second)) {
            align_axes(differential_axis(first), differential_axis(second));
            if (const auto* first_cylinder = std::get_if<DifferentialCylinder>(&first))
                if (const auto* second_cylinder = std::get_if<DifferentialCylinder>(&second))
                    rows.push_back(scalar((first_cylinder->radius - second_cylinder->radius) /
                                              options.length_scale,
                                          variables));
        } else if (const auto* first_plane = std::get_if<DifferentialPlane>(&first)) {
            if (const auto* second_plane = std::get_if<DifferentialPlane>(&second)) {
                append(divided(related(first_plane->normal, second_plane->normal) -
                                   second_plane->normal,
                               options.angle_scale));
                rows.push_back(divided(dot(first_plane->origin - second_plane->origin,
                                           second_plane->normal), options.length_scale));
            } else {
                return differential_residual(constraint, second, first, branch, options,
                                             angle_reference, unsigned_angle_axis);
            }
        } else {
            const auto axis_value = differential_axis(first);
            const auto& plane_value = std::get<DifferentialPlane>(second);
            rows.push_back(divided(dot(axis_value.direction, plane_value.normal), options.angle_scale));
            rows.push_back(divided(dot(axis_value.origin - plane_value.origin, plane_value.normal), options.length_scale));
        }
    } else if (constraint.kind == ConstraintKind::Concentric) {
        align_axes(differential_axis(first), differential_axis(second));
    } else if (constraint.kind == ConstraintKind::Angle) {
        DifferentialVector a = differential_direction(first);
        const DifferentialVector b = differential_direction(second);
        if (branch.direction_relation == DirectionRelation::Opposite)
            a = -a;
        DifferentialScalar angle;
        if (angle_reference) {
            const DifferentialVector k = normalized(*angle_reference);
            const DifferentialVector ap = normalized(a - dot(a, k) * k);
            const DifferentialVector bp = normalized(b - dot(b, k) * k);
            angle = differentiated_atan2(dot(k, cross(ap, bp)), dot(ap, bp));
        } else {
            const DifferentialVector product = cross(a, b);
            const DifferentialVector k = unsigned_angle_axis
                                             ? normalized(*unsigned_angle_axis)
                                             : (product.value.norm() > options.degeneracy_tolerance
                                                    ? normalized(product)
                                                    : vector(perpendicular_to(b.value), variables));
            angle = differentiated_atan2(dot(k, product), dot(a, b));
        }
        rows.push_back(divided(angle - scalar(constraint.value, variables), options.angle_scale));
    } else if (constraint.kind == ConstraintKind::Distance) {
        if (const auto* first_point = std::get_if<DifferentialPoint>(&first)) {
            if (const auto* second_point = std::get_if<DifferentialPoint>(&second)) {
                rows.push_back(divided(norm(first_point->position - second_point->position) -
                                           scalar(constraint.value, variables), options.length_scale));
            } else {
                const auto& plane_value = std::get<DifferentialPlane>(second);
                DifferentialScalar measured =
                    dot(first_point->position - plane_value.origin, plane_value.normal);
                if (branch.distance_relation == DistanceRelation::Unsigned)
                    measured = absolute(measured);
                else if (branch.distance_relation == DistanceRelation::OppositeSecondNormal)
                    measured = scalar(0.0, variables) - measured;
                rows.push_back(divided(measured - scalar(constraint.value, variables),
                                       options.length_scale));
            }
        } else if (std::holds_alternative<DifferentialPoint>(second)) {
            ConstraintBranchState swapped = branch;
            if (swapped.distance_relation == DistanceRelation::AlongSecondNormal)
                swapped.distance_relation = DistanceRelation::OppositeSecondNormal;
            else if (swapped.distance_relation == DistanceRelation::OppositeSecondNormal)
                swapped.distance_relation = DistanceRelation::AlongSecondNormal;
            return differential_residual(constraint, second, first, swapped, options, angle_reference,
                                         unsigned_angle_axis);
        } else if (differential_axis_like(first) && differential_axis_like(second)) {
            const auto first_axis = differential_axis(first);
            const auto second_axis = differential_axis(second);
            const DifferentialVector axis_cross = cross(first_axis.direction,
                                                         second_axis.direction);
            const DifferentialScalar cross_norm = norm(axis_cross);
            const DifferentialVector delta = first_axis.origin - second_axis.origin;
            const DifferentialScalar parallel = norm(cross(delta, second_axis.direction));
            DifferentialScalar distance = parallel;
            if (cross_norm.value > kDirectionEpsilon) {
                const DifferentialScalar skew_distance = absolute(dot(delta, axis_cross)) / cross_norm;
                const DifferentialScalar square = cross_norm * cross_norm;
                const DifferentialScalar weight = square /
                    (square + scalar(options.degeneracy_tolerance * options.degeneracy_tolerance,
                                     variables));
                distance = weight * skew_distance +
                           (scalar(1.0, variables) - weight) * parallel;
            }
            rows.push_back(divided(distance - scalar(constraint.value, variables), options.length_scale));
        } else {
            const auto& first_plane = std::get<DifferentialPlane>(first);
            const auto& second_plane = std::get<DifferentialPlane>(second);
            append(divided(related(first_plane.normal, second_plane.normal) - second_plane.normal,
                           options.angle_scale));
            DifferentialScalar measured = dot(first_plane.origin - second_plane.origin,
                                               second_plane.normal);
            if (branch.distance_relation == DistanceRelation::Unsigned)
                measured = absolute(measured);
            else if (branch.distance_relation == DistanceRelation::OppositeSecondNormal)
                measured = scalar(0.0, variables) - measured;
            rows.push_back(divided(measured - scalar(constraint.value, variables), options.length_scale));
        }
    }
    Eigen::MatrixXd result(static_cast<Eigen::Index>(rows.size()), variables);
    for (std::size_t row = 0; row < rows.size(); ++row)
        result.row(static_cast<Eigen::Index>(row)) = rows[row].derivative;
    return result;
}

Eigen::VectorXd constraint_tolerances(const Constraint& constraint, const WorldGeometry& first,
                                      const std::optional<WorldGeometry>& second,
                                      const SolverOptions& options) {
    const double length = options.length_tolerance / options.length_scale;
    const double angle = options.angle_tolerance / options.angle_scale;
    if (!second)
        throw std::invalid_argument("binary constraint requires a second geometry");
    if (constraint.kind == ConstraintKind::Angle) {
        return Eigen::VectorXd::Constant(1, angle);
    }
    if (constraint.kind == ConstraintKind::Concentric) {
        Eigen::VectorXd result(6);
        result << Vector3::Constant(angle), Vector3::Constant(length);
        return result;
    }
    if (constraint.kind == ConstraintKind::Coincident) {
        if (std::holds_alternative<WorldPoint>(first) ||
            std::holds_alternative<WorldPoint>(*second)) {
            const Eigen::Index count = (std::holds_alternative<WorldPlane>(first) ||
                                        std::holds_alternative<WorldPlane>(*second))
                                           ? 1
                                           : 3;
            return Eigen::VectorXd::Constant(count, length);
        }
        if (is_axis_like(first) && is_axis_like(*second)) {
            const bool radii = std::holds_alternative<WorldCylinder>(first) &&
                               std::holds_alternative<WorldCylinder>(*second);
            Eigen::VectorXd result(radii ? 7 : 6);
            result.head<3>().setConstant(angle);
            result.segment<3>(3).setConstant(length);
            if (radii)
                result[6] = length;
            return result;
        }
        if (std::holds_alternative<WorldPlane>(first) &&
            std::holds_alternative<WorldPlane>(*second)) {
            Eigen::VectorXd result(4);
            result << Vector3::Constant(angle), length;
            return result;
        }
        Eigen::VectorXd result(2);
        result << angle, length;
        return result;
    }
    if (constraint.kind == ConstraintKind::Distance) {
        if (std::holds_alternative<WorldPlane>(first) &&
            std::holds_alternative<WorldPlane>(*second)) {
            Eigen::VectorXd result(4);
            result << Vector3::Constant(angle), length;
            return result;
        }
        return Eigen::VectorXd::Constant(1, length);
    }
    throw std::invalid_argument("unsupported constraint kind for tolerance assignment");
}

double satisfaction_ratio(const Constraint& constraint, const WorldGeometry& first,
                          const std::optional<WorldGeometry>& second,
                          const Eigen::VectorXd& residual, const SolverOptions& options,
                          const ConstraintBranchState* branch = nullptr) {
    const double length = options.length_tolerance / options.length_scale;
    const double angle = options.angle_tolerance / options.angle_scale;
    if (!second)
        throw std::invalid_argument("binary constraint requires a second geometry");
    if (constraint.kind == ConstraintKind::Angle)
        return std::abs(residual[0]) / angle;
    if (constraint.kind == ConstraintKind::Concentric ||
        ((constraint.kind == ConstraintKind::Coincident) && is_axis_like(first) &&
         is_axis_like(*second))) {
        const DirectionRelation relation =
            branch ? branch->direction_relation : constraint.direction_relation;
        const double direction_error = std::acos(std::clamp(
            related_direction(geometry_direction(first), geometry_direction(*second), relation)
                .dot(geometry_direction(*second)),
            -1.0, 1.0));
        double ratio = std::max((direction_error / options.angle_scale) / angle,
                                residual.segment<3>(3).norm() / length);
        if (residual.size() == 7)
            ratio = std::max(ratio, std::abs(residual[6]) / length);
        return ratio;
    }
    if ((constraint.kind == ConstraintKind::Coincident ||
         constraint.kind == ConstraintKind::Distance) &&
        std::holds_alternative<WorldPlane>(first) && std::holds_alternative<WorldPlane>(*second)) {
        const DirectionRelation relation =
            branch ? branch->direction_relation : constraint.direction_relation;
        const double direction_error = std::acos(std::clamp(
            related_direction(geometry_direction(first), geometry_direction(*second), relation)
                .dot(geometry_direction(*second)),
            -1.0, 1.0));
        return std::max((direction_error / options.angle_scale) / angle,
                        std::abs(residual[3]) / length);
    }
    if (constraint.kind == ConstraintKind::Coincident && residual.size() == 2)
        return std::max(std::abs(residual[0]) / angle, std::abs(residual[1]) / length);
    const double tolerance = constraint.kind == ConstraintKind::Angle ? angle : length;
    return residual.norm() / tolerance;
}

bool block_satisfied(const ResidualBlock& block) {
    return finite(block.satisfaction_ratio) && block.satisfaction_ratio <= 1.0;
}

Pose identity_pose() {
    return {};
}

Pose compose(const Pose& first, const Pose& second) {
    const EigenQuaternion rotation = normalized(first.rotation) * normalized(second.rotation);
    return {
        value(normalized(first.rotation) * eigen(second.translation) + eigen(first.translation)),
        value(rotation)};
}

Pose inverse(const Pose& input) {
    const EigenQuaternion rotation = normalized(input.rotation).conjugate();
    return {value(rotation * -eigen(input.translation)), value(rotation)};
}

Pose relative_pose(const Pose& first, const Pose& second) {
    return compose(inverse(second), first);
}

bool pose_satisfied(const Pose& first, const Pose& second, const SolverOptions& options) {
    const double translation = (eigen(first.translation) - eigen(second.translation)).norm();
    const double rotation =
        rotation_vector(normalized(second.rotation).conjugate() * normalized(first.rotation))
            .norm();
    return translation <= options.length_tolerance && rotation <= options.angle_tolerance;
}

bool active(const Constraint& constraint) {
    return constraint.mode == ConstraintMode::Driving ||
           constraint.mode == ConstraintMode::Controlled;
}

class DisjointSet final {
public:
    explicit DisjointSet(const std::size_t size) : parent_(size), rank_(size) {
        std::iota(parent_.begin(), parent_.end(), 0);
    }

    std::size_t find(const std::size_t value) {
        if (parent_[value] != value)
            parent_[value] = find(parent_[value]);
        return parent_[value];
    }

    void join(const std::size_t first, const std::size_t second) {
        std::size_t a = find(first);
        std::size_t b = find(second);
        if (a == b)
            return;
        if (rank_[a] < rank_[b])
            std::swap(a, b);
        parent_[b] = a;
        if (rank_[a] == rank_[b])
            ++rank_[a];
    }

private:
    std::vector<std::size_t> parent_;
    std::vector<std::size_t> rank_;
};

struct RigidEdge {
    std::size_t neighbor{};
    Pose current_to_neighbor{};
    std::string constraint_id;
};

struct Cluster {
    std::string id;
    std::vector<std::size_t> body_indices;
    std::unordered_map<std::size_t, Pose> root_to_body;
    Pose initial_pose{};
    std::optional<Pose> ground_pose;
    std::vector<std::string> ground_constraint_ids;
};

struct Component {
    std::string id;
    std::vector<std::size_t> cluster_indices;
    std::vector<std::size_t> constraint_indices;
    bool selected{true};
};

class CompiledAssembly final {
public:
    CompiledAssembly(const Model& model, const SolverOptions& options)
        : model_(model), options_(options), constraints_(model.constraints) {
        validate_and_index();
        build_clusters();
        freeze_branches();
        build_components();
    }

    const Model& model() const { return model_; }
    const SolverOptions& options() const { return options_; }
    const std::vector<Cluster>& clusters() const { return clusters_; }
    std::vector<Cluster>& clusters() { return clusters_; }
    const std::vector<Component>& components() const { return components_; }
    const Constraint& constraint(const std::size_t index) const { return constraints_.at(index); }
    const ConstraintBranchState& branch(const std::size_t index) const {
        return branches_.at(index);
    }
    const std::vector<Constraint>& constraints() const { return constraints_; }

    const GeometryElement& geometry(const GeometryRef& reference) const {
        const auto found =
            geometry_index_.find(geometry_key(reference.body_id, reference.geometry_id));
        if (found == geometry_index_.end())
            throw std::invalid_argument("constraint references unknown geometry: " +
                                        reference.geometry_id);
        return *found->second;
    }

    std::size_t body_index(const std::string& id) const { return body_index_.at(id); }
    std::size_t cluster_index(const std::string& body_id) const {
        return body_to_cluster_.at(body_index(body_id));
    }

    std::vector<Pose> body_poses(const std::vector<Pose>& cluster_poses) const {
        std::vector<Pose> result(model_.bodies.size());
        for (std::size_t cluster_index = 0; cluster_index < clusters_.size(); ++cluster_index) {
            const Cluster& cluster = clusters_[cluster_index];
            for (const std::size_t body : cluster.body_indices)
                result[body] = compose(cluster_poses[cluster_index], cluster.root_to_body.at(body));
        }
        return result;
    }

private:
    void validate_and_index() {
        if (model_.bodies.empty())
            throw std::invalid_argument("assembly model requires at least one body");
        if (!(options_.length_scale > 0.0) || !(options_.angle_scale > 0.0) ||
            !(options_.length_tolerance > 0.0) || !(options_.angle_tolerance > 0.0) ||
            !(options_.classification_length_tolerance > 0.0) ||
            !(options_.classification_angle_tolerance > 0.0) ||
            !(options_.translation_step_tolerance > 0.0) ||
            !(options_.rotation_step_tolerance > 0.0) || !(options_.degeneracy_tolerance > 0.0) ||
            !(options_.translation_finite_difference_step > 0.0) ||
            !(options_.rotation_finite_difference_step > 0.0) ||
            !(options_.initial_damping > 0.0) || !(options_.rank_absolute_tolerance > 0.0) ||
            !(options_.rank_relative_tolerance > 0.0) || !(options_.gradient_tolerance > 0.0) ||
            !(options_.moving_preference_weight > 0.0) ||
            !(options_.neutral_preference_weight > 0.0) ||
            !(options_.reference_preference_weight > 0.0))
            throw std::invalid_argument(
                "solver scales, tolerances, steps, damping and rank tolerance must be positive");
        if (options_.classification_length_tolerance < options_.length_tolerance ||
            options_.classification_angle_tolerance < options_.angle_tolerance)
            throw std::invalid_argument(
                "classification tolerances must not be stricter than convergence tolerances");
        for (std::size_t index = 0; index < model_.bodies.size(); ++index) {
            const Body& body = model_.bodies[index];
            if (body.id.empty() || body_index_.count(body.id))
                throw std::invalid_argument("body IDs must be non-empty and unique");
            if (!finite(body.initial_pose.translation) || !finite(body.initial_pose.rotation))
                throw std::invalid_argument("body pose must be finite");
            (void)normalized(body.initial_pose.rotation);
            body_index_[body.id] = index;
        }
        for (const std::string& id : options_.affected_body_ids) {
            if (!body_index_.count(id))
                throw std::invalid_argument("affected body ID is unknown: " + id);
        }
        if (options_.solve_intent) {
            std::unordered_set<std::string> moving;
            for (const std::string& id : options_.solve_intent->moving_body_ids) {
                if (!body_index_.count(id))
                    throw std::invalid_argument("moving body ID is unknown: " + id);
                moving.insert(id);
            }
            for (const std::string& id : options_.solve_intent->reference_body_ids) {
                if (!body_index_.count(id))
                    throw std::invalid_argument("reference body ID is unknown: " + id);
                if (moving.count(id))
                    throw std::invalid_argument(
                        "one body cannot be both moving and reference in a solve intent: " + id);
            }
        }
        for (const GeometryElement& element : model_.geometry) {
            if (element.id.empty() || !body_index_.count(element.body_id))
                throw std::invalid_argument("geometry must have an ID and existing body");
            const std::string key = geometry_key(element.body_id, element.id);
            if (geometry_index_.count(key))
                throw std::invalid_argument("geometry IDs must be unique within a body");
            std::visit(
                [](const auto& geometry) {
                    using T = std::decay_t<decltype(geometry)>;
                    if constexpr (std::is_same_v<T, PointGeometry>) {
                        if (!finite(geometry.position))
                            throw std::invalid_argument("point must be finite");
                    } else if constexpr (std::is_same_v<T, AxisGeometry>) {
                        if (!finite(geometry.origin))
                            throw std::invalid_argument("axis origin must be finite");
                        (void)normalized(geometry.direction, "axis");
                    } else if constexpr (std::is_same_v<T, PlaneGeometry>) {
                        if (!finite(geometry.origin))
                            throw std::invalid_argument("plane origin must be finite");
                        (void)normalized(geometry.normal, "plane");
                    } else {
                        if (!finite(geometry.axis_origin) || !finite(geometry.radius) ||
                            geometry.radius <= 0.0)
                            throw std::invalid_argument("cylinder origin and radius are invalid");
                        (void)normalized(geometry.axis_direction, "cylinder");
                    }
                },
                element.local_geometry);
            geometry_index_[key] = &element;
        }
        std::unordered_set<std::string> constraint_ids;
        for (const Constraint& constraint : model_.constraints) {
            if (constraint.id.empty() || !constraint_ids.insert(constraint.id).second)
                throw std::invalid_argument("constraint IDs must be non-empty and unique");
            if (!body_index_.count(constraint.first.body_id))
                throw std::invalid_argument("constraint references an unknown body: " +
                                            constraint.first.body_id);
            if (constraint.kind == ConstraintKind::Fix) {
                if (constraint.second)
                    throw std::invalid_argument("Fix must have exactly one endpoint");
            } else if (constraint.kind == ConstraintKind::Rigid) {
                if (!constraint.second || constraint.first.body_id == constraint.second->body_id)
                    throw std::invalid_argument("Rigid requires two different bodies");
                if (!body_index_.count(constraint.second->body_id))
                    throw std::invalid_argument("Rigid references an unknown body");
                if (!constraint.fixed_pose)
                    throw std::invalid_argument("Rigid requires a captured relative pose");
            } else {
                (void)geometry(constraint.first);
                if (!constraint.second)
                    throw std::invalid_argument("binary constraint is missing its second endpoint");
                (void)geometry(*constraint.second);
            }
            if (!finite(constraint.value))
                throw std::invalid_argument("constraint value must be finite");
            if (constraint.kind == ConstraintKind::Distance && constraint.value < 0.0)
                throw std::invalid_argument("Distance value must not be negative");
            if (constraint.kind == ConstraintKind::Angle &&
                (constraint.value < 0.0 ||
                 constraint.value > (constraint.angle_reference_direction ? 2.0 * kPi : kPi)))
                throw std::invalid_argument(constraint.angle_reference_direction
                                                ? "Directed Angle value must be in [0, 2pi]"
                                                : "Angle value must be in [0, pi]");
            if (constraint.angle_reference_direction)
                (void)normalized(*constraint.angle_reference_direction,
                                 "angle reference direction");
            if (constraint.angle_branch_state &&
                (!finite(constraint.angle_branch_state->wrapped_angle) ||
                 !finite(constraint.angle_branch_state->unwrapped_angle)))
                throw std::invalid_argument("angle branch state must be finite");
        }
    }

    void build_clusters() {
        DisjointSet sets(model_.bodies.size());
        std::vector<std::vector<RigidEdge>> rigid_edges(model_.bodies.size());
        for (const Constraint& constraint : model_.constraints) {
            if (!active(constraint) || constraint.kind != ConstraintKind::Rigid)
                continue;
            const std::size_t first = body_index(constraint.first.body_id);
            const std::size_t second = body_index(constraint.second->body_id);
            sets.join(first, second);
            rigid_edges[second].push_back({first, *constraint.fixed_pose, constraint.id});
            rigid_edges[first].push_back({second, inverse(*constraint.fixed_pose), constraint.id});
        }

        std::unordered_map<std::size_t, std::vector<std::size_t>> members;
        for (std::size_t body = 0; body < model_.bodies.size(); ++body)
            members[sets.find(body)].push_back(body);
        std::vector<std::vector<std::size_t>> ordered;
        for (auto& [unused, values] : members) {
            (void)unused;
            std::sort(values.begin(), values.end(),
                      [&](const std::size_t first, const std::size_t second) {
                          return model_.bodies[first].id < model_.bodies[second].id;
                      });
            ordered.push_back(std::move(values));
        }
        std::sort(ordered.begin(), ordered.end(), [&](const auto& first, const auto& second) {
            return model_.bodies[first.front()].id < model_.bodies[second.front()].id;
        });

        for (const auto& values : ordered) {
            Cluster cluster;
            cluster.id = "cluster/" + model_.bodies[values.front()].id;
            cluster.body_indices = values;
            const std::size_t root = values.front();
            cluster.initial_pose = model_.bodies[root].initial_pose;
            cluster.root_to_body[root] = identity_pose();
            std::queue<std::size_t> pending;
            pending.push(root);
            while (!pending.empty()) {
                const std::size_t current = pending.front();
                pending.pop();
                for (const RigidEdge& edge : rigid_edges[current]) {
                    const Pose candidate =
                        compose(cluster.root_to_body.at(current), edge.current_to_neighbor);
                    const auto found = cluster.root_to_body.find(edge.neighbor);
                    if (found == cluster.root_to_body.end()) {
                        cluster.root_to_body[edge.neighbor] = candidate;
                        pending.push(edge.neighbor);
                    } else if (!pose_satisfied(found->second, candidate, options_)) {
                        throw std::invalid_argument("rigid constraint cycle is inconsistent: " +
                                                    edge.constraint_id);
                    }
                }
            }
            for (const std::size_t body : values) {
                if (!cluster.root_to_body.count(body))
                    cluster.root_to_body[body] =
                        relative_pose(model_.bodies[body].initial_pose, cluster.initial_pose);
            }
            const std::size_t cluster_index = clusters_.size();
            clusters_.push_back(std::move(cluster));
            for (const std::size_t body : values)
                body_to_cluster_[body] = cluster_index;
        }

        for (const Constraint& constraint : model_.constraints) {
            if (!active(constraint) || constraint.kind != ConstraintKind::Fix)
                continue;
            const std::size_t body = body_index(constraint.first.body_id);
            Cluster& cluster = clusters_[body_to_cluster_.at(body)];
            const Pose target = constraint.fixed_pose.value_or(model_.bodies[body].initial_pose);
            const Pose root_target = compose(target, inverse(cluster.root_to_body.at(body)));
            if (cluster.ground_pose && !pose_satisfied(*cluster.ground_pose, root_target, options_))
                throw std::invalid_argument("fixed constraints in one rigid cluster conflict");
            cluster.ground_pose = root_target;
            cluster.initial_pose = root_target;
            cluster.ground_constraint_ids.push_back(constraint.id);
        }

        if (options_.solve_intent) {
            std::unordered_set<std::size_t> moving_clusters;
            for (const std::string& id : options_.solve_intent->moving_body_ids)
                moving_clusters.insert(cluster_index(id));
            for (const std::string& id : options_.solve_intent->reference_body_ids) {
                if (moving_clusters.count(cluster_index(id)))
                    throw std::invalid_argument(
                        "a rigid cluster cannot contain both moving and reference bodies");
            }
        }
    }

    void freeze_branches() {
        branches_.resize(constraints_.size());
        std::vector<Pose> cluster_poses;
        cluster_poses.reserve(clusters_.size());
        for (const Cluster& cluster : clusters_)
            cluster_poses.push_back(cluster.ground_pose.value_or(cluster.initial_pose));
        const std::vector<Pose> bodies = body_poses(cluster_poses);
        for (std::size_t constraint_index = 0; constraint_index < constraints_.size();
             ++constraint_index) {
            Constraint& constraint = constraints_[constraint_index];
            ConstraintBranchState& branch = branches_[constraint_index];
            branch.direction_relation = constraint.direction_relation;
            branch.distance_relation = constraint.distance_relation;
            if (constraint.kind == ConstraintKind::Fix ||
                constraint.kind == ConstraintKind::Rigid || !constraint.second)
                continue;
            const GeometryElement& first_element = geometry(constraint.first);
            const GeometryElement& second_element = geometry(*constraint.second);
            const WorldGeometry first =
                world_geometry(first_element, bodies[body_index(first_element.body_id)]);
            const WorldGeometry second =
                world_geometry(second_element, bodies[body_index(second_element.body_id)]);
            if (constraint.direction_relation == DirectionRelation::Unoriented &&
                constraint.kind != ConstraintKind::Angle && has_direction(first) &&
                has_direction(second)) {
                branch.direction_relation =
                    geometry_direction(first).dot(geometry_direction(second)) <
                            -options_.degeneracy_tolerance
                        ? DirectionRelation::Opposite
                        : DirectionRelation::Same;
            }
            if (constraint.kind == ConstraintKind::Angle) {
                Vector3 first_direction = geometry_direction(first);
                const Vector3 second_direction = geometry_direction(second);
                if (branch.direction_relation == DirectionRelation::Opposite)
                    first_direction = -first_direction;
                if (constraint.angle_reference_direction) {
                    const Vector3 reference = direction(bodies[body_index(second_element.body_id)],
                                                        *constraint.angle_reference_direction,
                                                        "angle reference direction");
                    const double wrapped = directed_angle(first_direction, second_direction,
                                                          reference, options_.degeneracy_tolerance);
                    const double previous = constraint.angle_branch_state
                                                ? constraint.angle_branch_state->unwrapped_angle
                                                : wrapped;
                    const auto winding =
                        static_cast<std::int64_t>(std::llround((previous - wrapped) / (2.0 * kPi)));
                    branch.angle =
                        AngleBranchState{wrapped, wrapped + 2.0 * kPi * winding, winding};
                } else {
                    Vector3 axis = first_direction.cross(second_direction);
                    if (axis.norm() <= options_.degeneracy_tolerance)
                        axis = perpendicular_to(second_direction);
                    else
                        axis.normalize();
                    const EigenQuaternion inverse_second =
                        normalized(bodies[body_index(second_element.body_id)].rotation).conjugate();
                    branch.unsigned_angle_axis_in_second = value(inverse_second * axis);
                }
            }
            if (constraint.kind != ConstraintKind::Distance ||
                branch.distance_relation != DistanceRelation::Unsigned)
                continue;
            if (const auto* point = std::get_if<WorldPoint>(&first)) {
                if (const auto* plane = std::get_if<WorldPlane>(&second)) {
                    const double signed_distance =
                        (point->position - plane->origin).dot(plane->normal);
                    branch.distance_relation = signed_distance < -options_.degeneracy_tolerance
                                                   ? DistanceRelation::OppositeSecondNormal
                                                   : DistanceRelation::AlongSecondNormal;
                }
            } else if (const auto* plane = std::get_if<WorldPlane>(&first)) {
                if (const auto* second_point = std::get_if<WorldPoint>(&second)) {
                    const double signed_distance =
                        (second_point->position - plane->origin).dot(plane->normal);
                    branch.distance_relation = signed_distance < -options_.degeneracy_tolerance
                                                   ? DistanceRelation::AlongSecondNormal
                                                   : DistanceRelation::OppositeSecondNormal;
                } else if (const auto* second_plane = std::get_if<WorldPlane>(&second)) {
                    const double signed_distance =
                        (plane->origin - second_plane->origin).dot(second_plane->normal);
                    branch.distance_relation = signed_distance < -options_.degeneracy_tolerance
                                                   ? DistanceRelation::OppositeSecondNormal
                                                   : DistanceRelation::AlongSecondNormal;
                }
            }
        }
    }

    void build_components() {
        DisjointSet sets(clusters_.size());
        for (const Constraint& constraint : constraints_) {
            if (!active(constraint) || constraint.kind == ConstraintKind::Fix ||
                constraint.kind == ConstraintKind::Rigid)
                continue;
            const std::size_t first = cluster_index(constraint.first.body_id);
            const std::size_t second = cluster_index(constraint.second->body_id);
            sets.join(first, second);
        }
        std::unordered_map<std::size_t, std::vector<std::size_t>> cluster_groups;
        for (std::size_t cluster = 0; cluster < clusters_.size(); ++cluster)
            cluster_groups[sets.find(cluster)].push_back(cluster);
        std::vector<std::vector<std::size_t>> ordered;
        for (auto& [unused, values] : cluster_groups) {
            (void)unused;
            std::sort(values.begin(), values.end());
            ordered.push_back(std::move(values));
        }
        std::sort(ordered.begin(), ordered.end(), [&](const auto& first, const auto& second) {
            return clusters_[first.front()].id < clusters_[second.front()].id;
        });
        std::unordered_set<std::size_t> affected_clusters;
        for (const std::string& id : options_.affected_body_ids)
            affected_clusters.insert(cluster_index(id));
        for (const auto& values : ordered) {
            Component component;
            component.id = "component/" + clusters_[values.front()].id.substr(8);
            component.cluster_indices = values;
            component.selected = affected_clusters.empty();
            for (const std::size_t cluster : values)
                component.selected = component.selected || affected_clusters.count(cluster) != 0;
            std::unordered_set<std::size_t> in_component(values.begin(), values.end());
            for (std::size_t index = 0; index < constraints_.size(); ++index) {
                const Constraint& constraint = constraints_[index];
                if (!active(constraint) || constraint.kind == ConstraintKind::Fix ||
                    constraint.kind == ConstraintKind::Rigid)
                    continue;
                if (in_component.count(cluster_index(constraint.first.body_id)))
                    component.constraint_indices.push_back(index);
            }
            components_.push_back(std::move(component));
        }
    }

    const Model& model_;
    const SolverOptions& options_;
    std::vector<Constraint> constraints_;
    std::vector<ConstraintBranchState> branches_;
    std::unordered_map<std::string, std::size_t> body_index_;
    std::unordered_map<std::string, const GeometryElement*> geometry_index_;
    std::unordered_map<std::size_t, std::size_t> body_to_cluster_;
    std::vector<Cluster> clusters_;
    std::vector<Component> components_;
};

class ComponentProblem final {
public:
    ComponentProblem(const CompiledAssembly& assembly, const Component& component)
        : assembly_(assembly), component_(component) {
        for (const std::size_t cluster : component_.cluster_indices)
            physically_grounded_ =
                physically_grounded_ || assembly_.clusters()[cluster].ground_pose.has_value();
        if (!physically_grounded_ && assembly_.options().solve_intent &&
            assembly_.options().solve_intent->policy ==
                SolvePreferencePolicy::MoveFirstMinimizeReference) {
            for (const std::string& body_id :
                 assembly_.options().solve_intent->reference_body_ids) {
                const std::size_t candidate = assembly_.cluster_index(body_id);
                if (std::find(component_.cluster_indices.begin(), component_.cluster_indices.end(),
                              candidate) != component_.cluster_indices.end()) {
                    gauge_anchor_cluster_ = candidate;
                    break;
                }
            }
        }
        for (const std::size_t cluster : component_.cluster_indices) {
            if (!assembly_.clusters()[cluster].ground_pose &&
                (!gauge_anchor_cluster_ || *gauge_anchor_cluster_ != cluster)) {
                free_cluster_indices_.push_back(cluster);
            }
        }
    }

    State initial_state() const {
        State state;
        state.poses.reserve(free_cluster_indices_.size());
        for (const std::size_t cluster : free_cluster_indices_)
            state.poses.push_back(assembly_.clusters()[cluster].initial_pose);
        initialize_singular_direction_branches(state);
        return state;
    }

    State incremented(const State& state, const Vector& increment) const {
        State result = state;
        for (std::size_t index = 0; index < result.poses.size(); ++index) {
            const Eigen::Index offset = static_cast<Eigen::Index>(index * 6);
            Pose& pose = result.poses[index];
            pose.translation = value(eigen(pose.translation) + increment.segment<3>(offset));
            pose.rotation =
                value(exponential(increment.segment<3>(offset + 3)) * normalized(pose.rotation));
        }
        return result;
    }

    std::vector<Pose> cluster_poses(const State& state) const {
        std::vector<Pose> result;
        result.reserve(assembly_.clusters().size());
        for (const Cluster& cluster : assembly_.clusters())
            result.push_back(cluster.ground_pose.value_or(cluster.initial_pose));
        for (std::size_t index = 0; index < free_cluster_indices_.size(); ++index)
            result[free_cluster_indices_[index]] = state.poses[index];
        return result;
    }

    std::vector<ResidualBlock> blocks(const State& state,
                                      const bool use_classification_tolerance = false) const {
        const std::vector<Pose> bodies = assembly_.body_poses(cluster_poses(state));
        SolverOptions tolerance_options = assembly_.options();
        if (use_classification_tolerance) {
            tolerance_options.length_tolerance =
                assembly_.options().classification_length_tolerance;
            tolerance_options.angle_tolerance = assembly_.options().classification_angle_tolerance;
        }
        std::vector<ResidualBlock> result;
        for (const std::size_t constraint_index : component_.constraint_indices) {
            const Constraint& constraint = assembly_.constraint(constraint_index);
            const GeometryElement& first_element = assembly_.geometry(constraint.first);
            const GeometryElement& second_element = assembly_.geometry(*constraint.second);
            const WorldGeometry first =
                world_geometry(first_element, bodies[assembly_.body_index(first_element.body_id)]);
            const WorldGeometry second = world_geometry(
                second_element, bodies[assembly_.body_index(second_element.body_id)]);
            Constraint evaluated = constraint;
            ConstraintBranchState branch = assembly_.branch(constraint_index);
            if (evaluated.angle_reference_direction)
                evaluated.angle_reference_direction = value(
                    normalized(bodies[assembly_.body_index(second_element.body_id)].rotation) *
                    eigen(*evaluated.angle_reference_direction));
            if (branch.unsigned_angle_axis_in_second)
                branch.unsigned_angle_axis_in_second = value(
                    normalized(bodies[assembly_.body_index(second_element.body_id)].rotation) *
                    eigen(*branch.unsigned_angle_axis_in_second));
            Eigen::VectorXd residual =
                constraint_residual(evaluated, first, second, assembly_.options(), &branch);
            const EquationDefinition definition = equation_definition(evaluated, first, second);
            if (definition.kinds.size() != static_cast<std::size_t>(residual.size()))
                throw std::logic_error("equation registry row count does not match residual block");
            result.push_back({constraint.id, residual,
                              constraint_tolerances(evaluated, first, second, tolerance_options),
                              satisfaction_ratio(evaluated, first, second, residual,
                                                 tolerance_options, &branch),
                              definition.kinds, definition.generic_rank});
        }
        return result;
    }

    Vector residual(const State& state) const {
        const auto values = blocks(state);
        Eigen::Index size = 0;
        for (const auto& block : values)
            size += block.values.size();
        Vector result(size);
        Eigen::Index offset = 0;
        for (const auto& block : values) {
            result.segment(offset, block.values.size()) = block.values;
            offset += block.values.size();
        }
        return result;
    }

    bool satisfied(const State& state) const {
        const auto values = blocks(state);
        return std::all_of(values.begin(), values.end(),
                           [](const ResidualBlock& block) { return block_satisfied(block); });
    }

    bool classification_satisfied(const State& state) const {
        const auto values = blocks(state, true);
        return std::all_of(values.begin(), values.end(),
                           [](const ResidualBlock& block) { return block_satisfied(block); });
    }

    Eigen::MatrixXd finite_difference_jacobian(const State& state, const Vector& residual) const {
        Eigen::MatrixXd result(residual.size(), static_cast<Eigen::Index>(parameter_count()));
        std::vector<bool> periodic_rows;
        for (const ResidualBlock& block : blocks(state))
            for (const std::string& kind : block.equation_kinds)
                periodic_rows.push_back(kind.find("ANGLE") != std::string::npos);
        for (Eigen::Index column = 0; column < result.cols(); ++column) {
            Vector perturbation = Vector::Zero(result.cols());
            const bool translation = column % 6 < 3;
            const double configured = translation
                                          ? assembly_.options().translation_finite_difference_step
                                          : assembly_.options().rotation_finite_difference_step;
            const double step = assembly_.options().finite_difference_step > 0.0
                                    ? assembly_.options().finite_difference_step
                                    : configured;
            perturbation[column] = step;
            const Vector plus = this->residual(incremented(state, perturbation));
            perturbation[column] = -step;
            const Vector minus = this->residual(incremented(state, perturbation));
            if (plus.size() != residual.size() || minus.size() != residual.size() ||
                !plus.allFinite() || !minus.allFinite())
                throw std::runtime_error("finite-difference residual is invalid");
            Vector difference = plus - minus;
            for (Eigen::Index row = 0; row < difference.size(); ++row) {
                if (periodic_rows[static_cast<std::size_t>(row)])
                    difference[row] = wrapped_angle_error(
                                          plus[row] * assembly_.options().angle_scale,
                                          minus[row] * assembly_.options().angle_scale) /
                                      assembly_.options().angle_scale;
            }
            result.col(column) = difference / (2.0 * step);
        }
        return result;
    }

    Eigen::MatrixXd jacobian(const State& state, const Vector& residual) const {
        const std::vector<Pose> clusters = cluster_poses(state);
        const std::vector<Pose> bodies = assembly_.body_poses(clusters);
        const Eigen::Index variables = static_cast<Eigen::Index>(parameter_count());
        auto differentiated_geometry = [&](const GeometryElement& element) -> DifferentialGeometry {
            const WorldGeometry world = world_geometry(
                element, bodies[assembly_.body_index(element.body_id)]);
            auto differentiated_vector = [&](const Vector3& value, const bool point) {
                DifferentialVector result = vector(value, variables);
                const std::size_t cluster_index = assembly_.cluster_index(element.body_id);
                const auto found = std::find(free_cluster_indices_.begin(),
                                             free_cluster_indices_.end(), cluster_index);
                if (found == free_cluster_indices_.end())
                    return result;
                const Eigen::Index column = static_cast<Eigen::Index>(
                    std::distance(free_cluster_indices_.begin(), found) * 6);
                if (point) {
                    result.derivative.block<3, 3>(0, column).setIdentity();
                    result.derivative.block<3, 3>(0, column + 3) =
                        -skew(value - eigen(clusters[cluster_index].translation));
                } else {
                    result.derivative.block<3, 3>(0, column + 3) = -skew(value);
                }
                return result;
            };
            if (const auto* value = std::get_if<WorldPoint>(&world))
                return DifferentialPoint{differentiated_vector(value->position, true)};
            if (const auto* value = std::get_if<WorldAxis>(&world))
                return DifferentialAxis{differentiated_vector(value->origin, true),
                                        differentiated_vector(value->direction, false)};
            if (const auto* value = std::get_if<WorldPlane>(&world))
                return DifferentialPlane{differentiated_vector(value->origin, true),
                                         differentiated_vector(value->normal, false)};
            const auto& value = std::get<WorldCylinder>(world);
            return DifferentialCylinder{differentiated_vector(value.origin, true),
                                        differentiated_vector(value.direction, false), value.radius};
        };
        Eigen::MatrixXd result(residual.size(), variables);
        Eigen::Index row = 0;
        for (const std::size_t constraint_index : component_.constraint_indices) {
            const Constraint& constraint = assembly_.constraint(constraint_index);
            const GeometryElement& first_element = assembly_.geometry(constraint.first);
            const GeometryElement& second_element = assembly_.geometry(*constraint.second);
            const WorldGeometry first =
                world_geometry(first_element, bodies[assembly_.body_index(first_element.body_id)]);
            const WorldGeometry second = world_geometry(
                second_element, bodies[assembly_.body_index(second_element.body_id)]);
            const EquationDefinition definition = equation_definition(constraint, first, second);
            auto local_second_direction = [&](const Vec3& local) {
                const std::size_t second_body = assembly_.body_index(second_element.body_id);
                const Vector3 world_value = direction(bodies[second_body], local,
                                                      "angle branch direction");
                DifferentialVector differentiated = vector(world_value, variables);
                const std::size_t cluster_index = assembly_.cluster_index(second_element.body_id);
                const auto found = std::find(free_cluster_indices_.begin(),
                                             free_cluster_indices_.end(), cluster_index);
                if (found != free_cluster_indices_.end()) {
                    const Eigen::Index column = static_cast<Eigen::Index>(
                        std::distance(free_cluster_indices_.begin(), found) * 6 + 3);
                    differentiated.derivative.block<3, 3>(0, column) = -skew(world_value);
                }
                return differentiated;
            };
            std::optional<DifferentialVector> reference;
            if (constraint.angle_reference_direction)
                reference = local_second_direction(*constraint.angle_reference_direction);
            std::optional<DifferentialVector> unsigned_axis;
            if (assembly_.branch(constraint_index).unsigned_angle_axis_in_second)
                unsigned_axis = local_second_direction(
                    *assembly_.branch(constraint_index).unsigned_angle_axis_in_second);
            const Eigen::MatrixXd block = differential_residual(
                constraint, differentiated_geometry(first_element),
                differentiated_geometry(second_element), assembly_.branch(constraint_index),
                assembly_.options(), reference, unsigned_axis);
            if (block.rows() != static_cast<Eigen::Index>(definition.kinds.size()))
                throw std::logic_error("analytic Jacobian row count does not match equation registry");
            result.middleRows(row, block.rows()) = block;
            row += static_cast<Eigen::Index>(definition.kinds.size());
        }
        if (assembly_.options().verify_analytic_jacobians) {
            const Eigen::MatrixXd reference = finite_difference_jacobian(state, residual);
            if (result.size() == 0)
                return result;
            const double scale = std::max(1.0, reference.cwiseAbs().maxCoeff());
            const double error = (result - reference).cwiseAbs().maxCoeff();
            if (error > assembly_.options().jacobian_check_tolerance * scale)
                throw std::runtime_error("analytic Jacobian differential check failed: error=" +
                                         std::to_string(error) +
                                         " scale=" + std::to_string(scale));
        }
        return result;
    }

    std::size_t parameter_count() const { return free_cluster_indices_.size() * 6; }
    bool physically_grounded() const { return physically_grounded_; }
    std::size_t gauge_dof() const { return physically_grounded_ ? 0 : 6; }
    std::size_t logical_parameter_count() const {
        return parameter_count() + (gauge_anchor_cluster_ ? gauge_dof() : 0);
    }
    std::size_t relative_dof(const std::size_t rank) const {
        const std::size_t nullity = parameter_count() >= rank ? parameter_count() - rank : 0;
        return gauge_anchor_cluster_ ? nullity : nullity - std::min(nullity, gauge_dof());
    }
    const std::vector<std::size_t>& free_clusters() const { return free_cluster_indices_; }
    Vector preference_gradient(const State& state) const {
        Vector result = Vector::Zero(static_cast<Eigen::Index>(parameter_count()));
        for (std::size_t index = 0; index < free_cluster_indices_.size(); ++index) {
            const Cluster& cluster = assembly_.clusters()[free_cluster_indices_[index]];
            const double weight = preference_weight(free_cluster_indices_[index]);
            const Eigen::Index offset = static_cast<Eigen::Index>(index * 6);
            result.segment<3>(offset) =
                weight *
                (eigen(state.poses[index].translation) - eigen(cluster.initial_pose.translation)) /
                (assembly_.options().length_scale * assembly_.options().length_scale);
            result.segment<3>(offset + 3) =
                weight *
                rotation_vector(normalized(cluster.initial_pose.rotation).conjugate() *
                                normalized(state.poses[index].rotation)) /
                (assembly_.options().angle_scale * assembly_.options().angle_scale);
        }
        return result;
    }

    Vector preference_diagonal() const {
        Vector result = Vector::Zero(static_cast<Eigen::Index>(parameter_count()));
        for (std::size_t index = 0; index < free_cluster_indices_.size(); ++index) {
            const double weight = preference_weight(free_cluster_indices_[index]);
            const Eigen::Index offset = static_cast<Eigen::Index>(index * 6);
            result.segment<3>(offset).setConstant(
                weight / (assembly_.options().length_scale * assembly_.options().length_scale));
            result.segment<3>(offset + 3)
                .setConstant(weight /
                             (assembly_.options().angle_scale * assembly_.options().angle_scale));
        }
        return result;
    }

private:
    double preference_weight(const std::size_t cluster) const {
        if (!assembly_.options().solve_intent ||
            assembly_.options().solve_intent->policy == SolvePreferencePolicy::MinimumTotalChange)
            return assembly_.options().neutral_preference_weight;
        const SolveIntent& intent = *assembly_.options().solve_intent;
        for (const std::string& id : intent.reference_body_ids)
            if (assembly_.cluster_index(id) == cluster)
                return assembly_.options().reference_preference_weight;
        for (const std::string& id : intent.moving_body_ids)
            if (assembly_.cluster_index(id) == cluster)
                return assembly_.options().moving_preference_weight;
        return assembly_.options().neutral_preference_weight;
    }

    void initialize_singular_direction_branches(State& state) const {
        for (const std::size_t constraint_index : component_.constraint_indices) {
            const Constraint& constraint = assembly_.constraint(constraint_index);
            if ((constraint.kind != ConstraintKind::Coincident &&
                 constraint.kind != ConstraintKind::Concentric) ||
                constraint.direction_relation == DirectionRelation::Unoriented)
                continue;
            const GeometryElement& first_element = assembly_.geometry(constraint.first);
            const GeometryElement& second_element = assembly_.geometry(*constraint.second);
            const std::size_t moving_cluster = assembly_.cluster_index(first_element.body_id);
            const auto free = std::find(free_cluster_indices_.begin(), free_cluster_indices_.end(),
                                        moving_cluster);
            if (free == free_cluster_indices_.end())
                continue;
            const std::vector<Pose> bodies = assembly_.body_poses(cluster_poses(state));
            const WorldGeometry first =
                world_geometry(first_element, bodies[assembly_.body_index(first_element.body_id)]);
            const WorldGeometry second = world_geometry(
                second_element, bodies[assembly_.body_index(second_element.body_id)]);
            if (!has_direction(first) || !has_direction(second))
                continue;
            const Vector3 first_direction = geometry_direction(first);
            const Vector3 second_direction = geometry_direction(second);
            const Vector3 target = constraint.direction_relation == DirectionRelation::Same
                                       ? second_direction
                                       : -second_direction;
            if (first_direction.dot(target) > -1.0 + 1.0e-10)
                continue;

            // The vector-difference objective has zero gradient at the exact
            // antipodal branch. Seed that discrete branch geometrically by a
            // half turn about the selected geometry origin, keeping its anchor
            // fixed. The normal solve remains responsible for every other
            // constraint in the component.
            const Vector3 axis = perpendicular_to(first_direction);
            const EigenQuaternion turn(Eigen::AngleAxisd(kPi, axis));
            Pose& cluster_pose = state.poses[static_cast<std::size_t>(
                std::distance(free_cluster_indices_.begin(), free))];
            const Vector3 pivot = geometry_origin(first);
            cluster_pose.translation =
                value(pivot + turn * (eigen(cluster_pose.translation) - pivot));
            cluster_pose.rotation = value(turn * normalized(cluster_pose.rotation));

            if (std::holds_alternative<WorldPlane>(first) &&
                std::holds_alternative<WorldPlane>(second)) {
                const std::vector<Pose> turned_bodies = assembly_.body_poses(cluster_poses(state));
                const auto turned = std::get<WorldPlane>(world_geometry(
                    first_element, turned_bodies[assembly_.body_index(first_element.body_id)]));
                const auto& second_plane = std::get<WorldPlane>(second);
                cluster_pose.translation =
                    value(eigen(cluster_pose.translation) -
                          (turned.origin - second_plane.origin).dot(second_plane.normal) *
                              second_plane.normal);
            }
        }
    }

    const CompiledAssembly& assembly_;
    const Component& component_;
    std::vector<std::size_t> free_cluster_indices_;
    std::optional<std::size_t> gauge_anchor_cluster_;
    bool physically_grounded_{false};
};

struct ComponentSolution {
    SolveStatus status{SolveStatus::Converged};
    State state;
    std::size_t iterations{};
    std::string diagnostic;
};

ComponentSolution solve_component(const ComponentProblem& problem, const SolverOptions& options) {
    ComponentSolution result;
    result.state = problem.initial_state();
    Vector residual = problem.residual(result.state);
    if (!residual.allFinite())
        return {SolveStatus::NumericalFailure, result.state, 0, "initial residual is not finite"};
    if (residual.size() == 0 || problem.satisfied(result.state))
        return {SolveStatus::Converged, result.state, 0,
                residual.size() == 0 ? "component has no active equations"
                                     : "constraints already satisfied"};
    if (problem.parameter_count() == 0) {
        const SolveStatus status = problem.classification_satisfied(result.state)
                                       ? SolveStatus::Unsatisfied
                                       : SolveStatus::Inconsistent;
        return {status, result.state, 0,
                status == SolveStatus::Inconsistent
                    ? "grounded component violates classification tolerance"
                    : "grounded component is above convergence tolerance"};
    }
    double damping = options.initial_damping;
    for (std::size_t iteration = 1; iteration <= options.max_iterations; ++iteration) {
        const Eigen::MatrixXd jacobian = problem.jacobian(result.state, residual);
        const Vector constraint_gradient = jacobian.transpose() * residual;
        const Vector gradient = constraint_gradient + problem.preference_gradient(result.state);
        const Vector diagonal = problem.preference_diagonal().array() + damping;
        Eigen::MatrixXd augmented(jacobian.rows() + jacobian.cols(), jacobian.cols());
        augmented.topRows(jacobian.rows()) = jacobian;
        augmented.bottomRows(jacobian.cols()).setZero();
        Vector right_hand_side(residual.size() + jacobian.cols());
        right_hand_side.head(residual.size()) = -residual;
        for (Eigen::Index index = 0; index < jacobian.cols(); ++index) {
            const double scale = std::sqrt(diagonal[index]);
            augmented(jacobian.rows() + index, index) = scale;
            right_hand_side[residual.size() + index] =
                -problem.preference_gradient(result.state)[index] / scale;
        }
        Eigen::ColPivHouseholderQR<Eigen::MatrixXd> decomposition(augmented);
        const Vector step = decomposition.solve(right_hand_side);
        if (!step.allFinite())
            return {SolveStatus::NumericalFailure, result.state, iteration,
                    "linear solve produced a non-finite step"};
        bool small_step = true;
        for (Eigen::Index offset = 0; offset < step.size(); offset += 6) {
            small_step = small_step &&
                         step.segment<3>(offset).norm() <= options.translation_step_tolerance &&
                         step.segment<3>(offset + 3).norm() <= options.rotation_step_tolerance;
        }
        if (small_step) {
            if (constraint_gradient.norm() > options.gradient_tolerance) {
                damping = std::max(damping * 0.1, 1.0e-12);
                continue;
            }
            const SolveStatus status =
                problem.satisfied(result.state) ? SolveStatus::Converged : SolveStatus::Unsatisfied;
            return {status, result.state, iteration,
                    status == SolveStatus::Converged ? "converged"
                                                     : "stationary point above tolerance"};
        }
        std::optional<State> accepted_state;
        Vector accepted_residual;
        // A rigid-body rotation changes both a support direction and its world
        // origin. Even simple plane coincidence is therefore nonlinear when
        // the local support origin is far from the cluster origin. Apply the
        // same bounded descent backtracking to every component instead of only
        // to directed-angle edits.
        constexpr std::size_t trials = 12;
        double step_scale = 1.0;
        for (std::size_t trial = 0; trial < trials; ++trial, step_scale *= 0.5) {
            const State candidate = problem.incremented(result.state, step_scale * step);
            const Vector candidate_residual = problem.residual(candidate);
            if (candidate_residual.allFinite() &&
                candidate_residual.squaredNorm() < residual.squaredNorm()) {
                accepted_state = candidate;
                accepted_residual = candidate_residual;
                break;
            }
        }
        if (accepted_state) {
            result.state = *accepted_state;
            residual = std::move(accepted_residual);
            damping = std::max(damping * 0.25, 1.0e-12);
            if (problem.satisfied(result.state))
                return {SolveStatus::Converged, result.state, iteration, "converged"};
        } else {
            damping = std::min(damping * 10.0, 1.0e12);
        }
    }
    return {SolveStatus::MaxIterations, result.state, options.max_iterations,
            "maximum iteration count reached"};
}

Eigen::MatrixXd rank_normalized(Eigen::MatrixXd matrix, const double absolute_tolerance) {
    for (Eigen::Index column = 0; column < matrix.cols(); ++column) {
        const double norm = matrix.col(column).norm();
        if (norm > absolute_tolerance)
            matrix.col(column) /= norm;
    }
    return matrix;
}

struct NullSpaceAnalysis {
    std::size_t rank{};
    std::vector<std::vector<double>> basis;
    std::vector<double> singular_values;
    double threshold{};
};

NullSpaceAnalysis analyze_null_space(const Eigen::MatrixXd& matrix,
                                     const SolverOptions& options) {
    NullSpaceAnalysis result;
    if (matrix.cols() == 0)
        return result;
    const double absolute =
        options.rank_tolerance > 0.0 ? options.rank_tolerance : options.rank_absolute_tolerance;
    if (matrix.rows() == 0) {
        result.threshold = absolute;
        for (Eigen::Index column = 0; column < matrix.cols(); ++column) {
            std::vector<double> vector(static_cast<std::size_t>(matrix.cols()), 0.0);
            vector[static_cast<std::size_t>(column)] = 1.0;
            result.basis.push_back(std::move(vector));
        }
        return result;
    }
    Eigen::VectorXd scales(matrix.cols());
    Eigen::MatrixXd normalized_matrix = matrix;
    for (Eigen::Index column = 0; column < matrix.cols(); ++column) {
        scales[column] = matrix.col(column).norm();
        if (scales[column] > absolute)
            normalized_matrix.col(column) /= scales[column];
        else
            scales[column] = 1.0;
    }
    Eigen::JacobiSVD<Eigen::MatrixXd> decomposition(normalized_matrix, Eigen::ComputeFullV);
    for (Eigen::Index index = 0; index < decomposition.singularValues().size(); ++index)
        result.singular_values.push_back(decomposition.singularValues()[index]);
    const double largest = decomposition.singularValues().size() == 0
                               ? 0.0
                               : decomposition.singularValues()[0];
    result.threshold = std::max(absolute, options.rank_relative_tolerance * largest);
    result.rank = static_cast<std::size_t>(
        (decomposition.singularValues().array() > result.threshold).count());
    for (Eigen::Index column = static_cast<Eigen::Index>(result.rank);
         column < decomposition.matrixV().cols(); ++column) {
        Eigen::VectorXd vector = decomposition.matrixV().col(column).cwiseQuotient(scales);
        if (vector.norm() > absolute)
            vector.normalize();
        std::vector<double> values(static_cast<std::size_t>(vector.size()));
        for (Eigen::Index index = 0; index < vector.size(); ++index)
            values[static_cast<std::size_t>(index)] = vector[index];
        result.basis.push_back(std::move(values));
    }
    return result;
}

std::size_t matrix_rank(const Eigen::MatrixXd& matrix, const SolverOptions& options) {
    if (matrix.rows() == 0 || matrix.cols() == 0)
        return 0;
    const double absolute =
        options.rank_tolerance > 0.0 ? options.rank_tolerance : options.rank_absolute_tolerance;
    const Eigen::MatrixXd normalized_matrix = rank_normalized(matrix, absolute);
    Eigen::JacobiSVD<Eigen::MatrixXd> decomposition(normalized_matrix,
                                                    Eigen::ComputeThinU | Eigen::ComputeThinV);
    if (decomposition.singularValues().size() == 0)
        return 0;
    const double threshold =
        std::max(absolute, options.rank_relative_tolerance * decomposition.singularValues()[0]);
    return static_cast<std::size_t>((decomposition.singularValues().array() > threshold).count());
}

std::vector<ConstraintRankInfo> constraint_rank_info(const ComponentProblem& problem,
                                                     const State& state,
                                                     const SolverOptions& options) {
    const auto blocks = problem.blocks(state);
    const Vector all_residuals = problem.residual(state);
    const Eigen::MatrixXd all_jacobian = problem.jacobian(state, all_residuals);
    std::vector<ConstraintRankInfo> result;
    Eigen::MatrixXd accepted(0, all_jacobian.cols());
    std::size_t accepted_rank = 0;
    Eigen::Index row = 0;
    for (const ResidualBlock& block : blocks) {
        Eigen::MatrixXd candidate(accepted.rows() + block.values.size(), accepted.cols());
        if (accepted.rows() > 0)
            candidate.topRows(accepted.rows()) = accepted;
        candidate.bottomRows(block.values.size()) =
            all_jacobian.middleRows(row, block.values.size());
        const Eigen::MatrixXd local = all_jacobian.middleRows(row, block.values.size());
        const std::size_t effective_rank = matrix_rank(local, options);
        const std::size_t candidate_rank = matrix_rank(candidate, options);
        const std::size_t incremental_rank = candidate_rank - accepted_rank;
        const ConstraintRankRole role = incremental_rank == 0 ? ConstraintRankRole::FullyRedundant
                                        : incremental_rank < effective_rank
                                            ? ConstraintRankRole::PartiallyRedundant
                                            : ConstraintRankRole::Independent;
        result.push_back({block.id, static_cast<std::size_t>(block.values.size()), effective_rank,
                          incremental_rank, block.declared_generic_rank, role});
        if (incremental_rank != 0) {
            accepted = std::move(candidate);
            accepted_rank = candidate_rank;
        }
        row += block.values.size();
    }
    return result;
}

ResidualBlock evaluate_constraint(const CompiledAssembly& assembly,
                                  const std::size_t constraint_index,
                                  const std::vector<Pose>& body_poses,
                                  const bool use_classification_tolerance = false) {
    const Constraint& constraint = assembly.constraint(constraint_index);
    SolverOptions tolerance_options = assembly.options();
    if (use_classification_tolerance) {
        tolerance_options.length_tolerance = assembly.options().classification_length_tolerance;
        tolerance_options.angle_tolerance = assembly.options().classification_angle_tolerance;
    }
    if (constraint.kind == ConstraintKind::Fix) {
        const std::size_t body = assembly.body_index(constraint.first.body_id);
        const Pose target =
            constraint.fixed_pose.value_or(assembly.model().bodies[body].initial_pose);
        Eigen::VectorXd residual(6);
        residual << (eigen(body_poses[body].translation) - eigen(target.translation)) /
                        assembly.options().length_scale,
            rotation_vector(normalized(target.rotation).conjugate() *
                            normalized(body_poses[body].rotation)) /
                assembly.options().angle_scale;
        Eigen::VectorXd tolerances(6);
        tolerances << Vector3::Constant(tolerance_options.length_tolerance /
                                        assembly.options().length_scale),
            Vector3::Constant(tolerance_options.angle_tolerance / assembly.options().angle_scale);
        const double ratio =
            std::max(residual.head<3>().norm() /
                         (tolerance_options.length_tolerance / assembly.options().length_scale),
                     residual.tail<3>().norm() /
                         (tolerance_options.angle_tolerance / assembly.options().angle_scale));
        return {constraint.id, std::move(residual), std::move(tolerances), ratio,
                {"FIX_TRANSLATION_X", "FIX_TRANSLATION_Y", "FIX_TRANSLATION_Z",
                 "FIX_ROTATION_X", "FIX_ROTATION_Y", "FIX_ROTATION_Z"}, 6};
    }
    if (constraint.kind == ConstraintKind::Rigid) {
        const Pose& first = body_poses[assembly.body_index(constraint.first.body_id)];
        const Pose& second = body_poses[assembly.body_index(constraint.second->body_id)];
        const Pose current = relative_pose(first, second);
        const Pose target = *constraint.fixed_pose;
        Eigen::VectorXd residual(6);
        residual << (eigen(current.translation) - eigen(target.translation)) /
                        assembly.options().length_scale,
            rotation_vector(normalized(target.rotation).conjugate() *
                            normalized(current.rotation)) /
                assembly.options().angle_scale;
        Eigen::VectorXd tolerances(6);
        tolerances << Vector3::Constant(tolerance_options.length_tolerance /
                                        assembly.options().length_scale),
            Vector3::Constant(tolerance_options.angle_tolerance / assembly.options().angle_scale);
        const double ratio =
            std::max(residual.head<3>().norm() /
                         (tolerance_options.length_tolerance / assembly.options().length_scale),
                     residual.tail<3>().norm() /
                         (tolerance_options.angle_tolerance / assembly.options().angle_scale));
        return {constraint.id, std::move(residual), std::move(tolerances), ratio,
                {"RIGID_TRANSLATION_X", "RIGID_TRANSLATION_Y", "RIGID_TRANSLATION_Z",
                 "RIGID_ROTATION_X", "RIGID_ROTATION_Y", "RIGID_ROTATION_Z"}, 6};
    }
    const GeometryElement& first_element = assembly.geometry(constraint.first);
    const GeometryElement& second_element = assembly.geometry(*constraint.second);
    const WorldGeometry first =
        world_geometry(first_element, body_poses[assembly.body_index(first_element.body_id)]);
    const WorldGeometry second =
        world_geometry(second_element, body_poses[assembly.body_index(second_element.body_id)]);
    Constraint evaluated = constraint;
    if (evaluated.angle_reference_direction)
        evaluated.angle_reference_direction =
            value(normalized(body_poses[assembly.body_index(second_element.body_id)].rotation) *
                  eigen(*evaluated.angle_reference_direction));
    ConstraintBranchState branch = assembly.branch(constraint_index);
    if (branch.unsigned_angle_axis_in_second)
        branch.unsigned_angle_axis_in_second =
            value(normalized(body_poses[assembly.body_index(second_element.body_id)].rotation) *
                  eigen(*branch.unsigned_angle_axis_in_second));
    Eigen::VectorXd residual =
        constraint_residual(evaluated, first, second, assembly.options(), &branch);
    const EquationDefinition definition = equation_definition(evaluated, first, second);
    if (definition.kinds.size() != static_cast<std::size_t>(residual.size()))
        throw std::logic_error("equation registry row count does not match residual block");
    return {constraint.id, residual,
            constraint_tolerances(evaluated, first, second, tolerance_options),
            satisfaction_ratio(evaluated, first, second, residual, tolerance_options, &branch),
            definition.kinds, definition.generic_rank};
}

std::string connection_id(const Constraint& constraint) {
    return constraint.connection_id.empty() ? "connection/" + constraint.id
                                            : constraint.connection_id;
}

}  // namespace

SolveResult Solver::solve(const Model& model, const SolverOptions& options) const {
    try {
        CompiledAssembly assembly(model, options);
        SolveResult result;
        result.status = SolveStatus::Converged;
        std::vector<Pose> cluster_poses;
        std::vector<bool> selected_clusters(assembly.clusters().size(), false);
        std::vector<bool> unsatisfied_clusters(assembly.clusters().size(), false);
        std::vector<bool> inconsistent_clusters(assembly.clusters().size(), false);
        cluster_poses.reserve(assembly.clusters().size());
        for (const Cluster& cluster : assembly.clusters())
            cluster_poses.push_back(cluster.ground_pose.value_or(cluster.initial_pose));

        bool has_relative_dof = false;
        for (const Component& component : assembly.components()) {
            for (const std::size_t cluster : component.cluster_indices)
                selected_clusters[cluster] = component.selected;
            ComponentProblem problem(assembly, component);
            ComponentSolution solution;
            if (component.selected)
                solution = solve_component(problem, options);
            else {
                solution.state = problem.initial_state();
                solution.diagnostic = "component was outside the affected solve scope";
            }
            for (std::size_t index = 0; index < problem.free_clusters().size(); ++index)
                cluster_poses[problem.free_clusters()[index]] = solution.state.poses[index];
            result.iterations += solution.iterations;
            if (solution.status == SolveStatus::NumericalFailure)
                result.status = SolveStatus::NumericalFailure;
            else if (solution.status == SolveStatus::MaxIterations &&
                     result.status != SolveStatus::NumericalFailure)
                result.status = SolveStatus::MaxIterations;
            else if (solution.status == SolveStatus::Inconsistent &&
                     result.status != SolveStatus::NumericalFailure &&
                     result.status != SolveStatus::MaxIterations)
                result.status = SolveStatus::Inconsistent;
            else if (solution.status == SolveStatus::Unsatisfied &&
                     result.status == SolveStatus::Converged)
                result.status = SolveStatus::Unsatisfied;
            if (solution.status == SolveStatus::Unsatisfied) {
                for (const std::size_t cluster : component.cluster_indices)
                    unsatisfied_clusters[cluster] = true;
            }
            if (solution.status == SolveStatus::Inconsistent) {
                for (const std::size_t cluster : component.cluster_indices)
                    inconsistent_clusters[cluster] = true;
            }

            const Vector residual = problem.residual(solution.state);
            const Eigen::MatrixXd jacobian = problem.jacobian(solution.state, residual);
            const NullSpaceAnalysis null_space = analyze_null_space(jacobian, options);
            const std::size_t rank = null_space.rank;
            const std::size_t variables = problem.logical_parameter_count();
            const std::size_t gauge = problem.gauge_dof();
            const std::size_t relative = problem.relative_dof(rank);
            has_relative_dof = has_relative_dof || relative > 0;
            ComponentDof component_dof;
            component_dof.component_id = component.id;
            component_dof.tangent_variable_count = variables;
            component_dof.jacobian_rank = rank;
            component_dof.relative_dof = relative;
            component_dof.gauge_dof = gauge;
            component_dof.solved = component.selected;
            component_dof.null_space_basis = null_space.basis;
            component_dof.singular_values = null_space.singular_values;
            component_dof.rank_threshold = null_space.threshold;
            for (const std::size_t cluster_index : problem.free_clusters())
                component_dof.tangent_cluster_ids.push_back(
                    assembly.clusters()[cluster_index].id);
            for (const std::size_t cluster_index : component.cluster_indices) {
                for (const std::size_t body : assembly.clusters()[cluster_index].body_indices)
                    component_dof.body_ids.push_back(model.bodies[body].id);
            }
            std::sort(component_dof.body_ids.begin(), component_dof.body_ids.end());
            result.components.push_back(std::move(component_dof));

            if (component.selected && solution.status != SolveStatus::NumericalFailure) {
                const auto ranks = constraint_rank_info(problem, solution.state, options);
                for (const ConstraintRankInfo& info : ranks) {
                    if (info.role == ConstraintRankRole::FullyRedundant)
                        result.redundant_constraint_ids.push_back(info.constraint_id);
                    result.constraint_ranks.push_back(info);
                }
            }
            if (!component.selected) {
                result.diagnostics.push_back(
                    {"COMPONENT_NOT_SOLVED", component.id, {}, {}, solution.diagnostic});
            }
        }

        const std::vector<Pose> body_poses = assembly.body_poses(cluster_poses);
        for (std::size_t index = 0; index < model.bodies.size(); ++index)
            result.bodies.push_back({model.bodies[index].id, body_poses[index]});

        double squared_residual = 0.0;
        for (std::size_t constraint_index = 0; constraint_index < assembly.constraints().size();
             ++constraint_index) {
            const Constraint& constraint = assembly.constraint(constraint_index);
            if (constraint.mode == ConstraintMode::Suppressed)
                continue;
            const ResidualBlock block =
                evaluate_constraint(assembly, constraint_index, body_poses, true);
            const double norm = block.values.norm();
            result.residuals.push_back({constraint.id, norm});
            if (active(constraint))
                squared_residual += block.values.squaredNorm();
            for (Eigen::Index equation = 0; equation < block.values.size(); ++equation) {
                result.equation_residuals.push_back(
                    {constraint.id + "/equation/" + block.equation_kinds[static_cast<std::size_t>(equation)],
                     connection_id(constraint), constraint.id, static_cast<std::size_t>(equation),
                     block.values[equation]});
            }
            const bool selected =
                options.affected_body_ids.empty() ||
                selected_clusters[assembly.cluster_index(constraint.first.body_id)];
            if (active(constraint) && selected && !block_satisfied(block)) {
                const std::size_t cluster = assembly.cluster_index(constraint.first.body_id);
                if (inconsistent_clusters[cluster])
                    result.conflicting_constraint_ids.push_back(constraint.id);
                else if (unsatisfied_clusters[cluster])
                    result.unsatisfied_constraint_ids.push_back(constraint.id);
            }
            if (constraint.kind == ConstraintKind::Angle && constraint.angle_reference_direction) {
                Constraint evaluated = constraint;
                const GeometryElement& first_element = assembly.geometry(constraint.first);
                const GeometryElement& second_element = assembly.geometry(*constraint.second);
                const WorldGeometry first = world_geometry(
                    first_element, body_poses[assembly.body_index(first_element.body_id)]);
                const WorldGeometry second = world_geometry(
                    second_element, body_poses[assembly.body_index(second_element.body_id)]);
                const Vector3 reference =
                    direction(body_poses[assembly.body_index(second_element.body_id)],
                              *constraint.angle_reference_direction, "angle reference direction");
                const Vector3 first_direction =
                    related_direction(geometry_direction(first), geometry_direction(second),
                                      assembly.branch(constraint_index).direction_relation);
                const double wrapped = directed_angle(first_direction, geometry_direction(second),
                                                      reference, options.degeneracy_tolerance);
                const AngleBranchState& initial = *assembly.branch(constraint_index).angle;
                const auto winding = static_cast<std::int64_t>(
                    std::llround((initial.unwrapped_angle - wrapped) / (2.0 * kPi)));
                result.angle_branches.push_back(
                    {constraint.id, {wrapped, wrapped + 2.0 * kPi * winding, winding}});
            }
        }
        result.normalized_residual = std::sqrt(squared_residual);
        std::sort(result.redundant_constraint_ids.begin(), result.redundant_constraint_ids.end());
        result.redundant_constraint_ids.erase(std::unique(result.redundant_constraint_ids.begin(),
                                                          result.redundant_constraint_ids.end()),
                                              result.redundant_constraint_ids.end());
        std::sort(result.conflicting_constraint_ids.begin(),
                  result.conflicting_constraint_ids.end());
        std::sort(result.unsatisfied_constraint_ids.begin(),
                  result.unsatisfied_constraint_ids.end());

        if (result.status == SolveStatus::NumericalFailure ||
            result.status == SolveStatus::MaxIterations) {
            result.classification = SolveClassification::NonConvergent;
        } else if (result.status == SolveStatus::Inconsistent) {
            result.classification = SolveClassification::Inconsistent;
            result.diagnostics.push_back({"CONFLICTING_CONSTRAINTS",
                                          {},
                                          {},
                                          result.conflicting_constraint_ids,
                                          "a zero-variable component violates active constraints"});
        } else if (result.status == SolveStatus::Unsatisfied) {
            result.classification = SolveClassification::Unsatisfied;
            const char* detail = result.unsatisfied_constraint_ids.empty()
                                     ? "the solver stopped above convergence tolerance; no "
                                       "equation exceeds classification tolerance"
                                     : "the final candidate remains above classification tolerance";
            result.diagnostics.push_back(
                {"UNSATISFIED_CONSTRAINTS", {}, {}, result.unsatisfied_constraint_ids, detail});
        } else if (!result.redundant_constraint_ids.empty()) {
            result.classification = SolveClassification::Redundant;
            result.diagnostics.push_back({"REDUNDANT_CONSTRAINTS",
                                          {},
                                          {},
                                          result.redundant_constraint_ids,
                                          "constraints add no independent Jacobian rank"});
        } else if (has_relative_dof) {
            result.classification = SolveClassification::SolvedUnderConstrained;
        } else {
            result.classification = SolveClassification::SolvedFully;
        }
        result.diagnostic = result.status == SolveStatus::Converged ? "assembly components solved"
                            : result.status == SolveStatus::Unsatisfied
                                ? "assembly candidate does not satisfy all constraints"
                            : result.status == SolveStatus::Inconsistent
                                ? "assembly constraints are inconsistent"
                                : "one or more assembly components did not converge";
        if (result.status == SolveStatus::Unsatisfied && options.max_conflict_probes > 0) {
            std::size_t probes = 0;
            for (std::size_t index = 0;
                 index < model.constraints.size() && probes < options.max_conflict_probes;
                 ++index) {
                if (!active(model.constraints[index]))
                    continue;
                ++probes;
                Model probe_model = model;
                probe_model.constraints[index].mode = ConstraintMode::Suppressed;
                SolverOptions probe_options = options;
                probe_options.max_conflict_probes = 0;
                const SolveResult probe = Solver{}.solve(probe_model, probe_options);
                if (probe.status == SolveStatus::Converged ||
                    (finite(probe.normalized_residual) && result.normalized_residual > 0.0 &&
                     probe.normalized_residual < 0.5 * result.normalized_residual))
                    result.suspected_conflicting_constraint_ids.push_back(
                        model.constraints[index].id);
            }
            if (!result.suspected_conflicting_constraint_ids.empty())
                result.diagnostics.push_back(
                    {"LIKELY_INCONSISTENT",
                     {},
                     {},
                     result.suspected_conflicting_constraint_ids,
                     "bounded single-constraint removal probes significantly improved solvability; "
                     "this is a suspected set, not a minimal conflict proof"});
        }
        return result;
    } catch (const std::invalid_argument& error) {
        SolveResult result;
        result.status = SolveStatus::InvalidModel;
        result.classification = SolveClassification::InvalidModel;
        result.diagnostic = error.what();
        return result;
    } catch (const std::exception& error) {
        SolveResult result;
        result.status = SolveStatus::NumericalFailure;
        result.classification = SolveClassification::NonConvergent;
        result.diagnostic = error.what();
        return result;
    }
}

}  // namespace occccad::assembly
