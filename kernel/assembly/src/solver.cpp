#include <occccad/assembly/solver.hpp>

#include <Eigen/Cholesky>
#include <Eigen/Core>
#include <Eigen/Geometry>
#include <algorithm>
#include <cmath>
#include <limits>
#include <stdexcept>
#include <type_traits>
#include <unordered_map>
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

Vector3 geometry_direction(const WorldGeometry& geometry) {
    if (is_axis_like(geometry))
        return as_axis(geometry).direction;
    if (const auto* plane = std::get_if<WorldPlane>(&geometry))
        return plane->normal;
    throw std::invalid_argument("geometry has no direction");
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
                               const Constraint& constraint, const SolverOptions& options) {
    const Vector3 direction_a =
        related_direction(first.direction, second.direction, constraint.direction_relation);
    return concatenate(
        (direction_a - second.direction) / options.angle_scale,
        (first.origin - second.origin).cross(second.direction) / options.length_scale);
}

double line_distance(const WorldAxis& first, const WorldAxis& second) {
    const Vector3 cross = first.direction.cross(second.direction);
    if (cross.norm() <= 1.0e-10)
        return (first.origin - second.origin).cross(second.direction).norm();
    return std::abs((first.origin - second.origin).dot(cross.normalized()));
}

Eigen::VectorXd single(const double value) {
    Eigen::VectorXd result(1);
    result[0] = value;
    return result;
}

Eigen::VectorXd constraint_residual(const Constraint& constraint, const WorldGeometry& first,
                                    const std::optional<WorldGeometry>& second,
                                    const SolverOptions& options) {
    if (constraint.kind == ConstraintKind::Fix || constraint.kind == ConstraintKind::Rigid)
        throw std::invalid_argument("rigid-body residual is evaluated from body poses");
    if (!second)
        throw std::invalid_argument("binary constraint requires a second geometry");

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
            return constraint_residual(swapped, *second, first, options);
        }
        if (is_axis_like(first) && is_axis_like(*second)) {
            Eigen::VectorXd result =
                axis_alignment(as_axis(first), as_axis(*second), constraint, options);
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
                const Vector3 normal_a = related_direction(plane_a->normal, plane_b->normal,
                                                           constraint.direction_relation);
                Eigen::VectorXd result(4);
                result << (normal_a - plane_b->normal) / options.angle_scale,
                    (plane_a->origin - plane_b->origin).dot(plane_b->normal) / options.length_scale;
                return result;
            }
        }
        throw std::invalid_argument("unsupported Coincident geometry pair");
    }

    if (constraint.kind == ConstraintKind::Concentric) {
        if (!is_axis_like(first) || !is_axis_like(*second))
            throw std::invalid_argument("Concentric requires Axis or Cylinder geometry");
        return axis_alignment(as_axis(first), as_axis(*second), constraint, options);
    }

    if (constraint.kind == ConstraintKind::Angle) {
        if (!has_direction(first) || !has_direction(*second))
            throw std::invalid_argument("Angle requires Plane, Axis, or Cylinder geometry");
        Vector3 first_direction = geometry_direction(first);
        const Vector3 second_direction = geometry_direction(*second);
        if (constraint.direction_relation == DirectionRelation::Opposite)
            first_direction = -first_direction;
        double cosine = std::clamp(first_direction.dot(second_direction), -1.0, 1.0);
        if (constraint.direction_relation == DirectionRelation::Unoriented)
            cosine = std::abs(cosine);
        const double angle = std::acos(cosine);
        return single((angle - constraint.value) / options.angle_scale);
    }

    if (constraint.kind == ConstraintKind::Distance) {
        if (const auto* point_a = std::get_if<WorldPoint>(&first)) {
            if (const auto* point_b = std::get_if<WorldPoint>(&*second))
                return single(((point_a->position - point_b->position).norm() - constraint.value) /
                              options.length_scale);
            if (const auto* plane = std::get_if<WorldPlane>(&*second)) {
                double measured = (point_a->position - plane->origin).dot(plane->normal);
                if (constraint.distance_relation == DistanceRelation::Unsigned)
                    measured = std::abs(measured);
                if (constraint.distance_relation == DistanceRelation::OppositeSecondNormal)
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
            return constraint_residual(swapped, *second, first, options);
        }
        if (is_axis_like(first) && is_axis_like(*second))
            return single((line_distance(as_axis(first), as_axis(*second)) - constraint.value) /
                          options.length_scale);
        if (const auto* plane_a = std::get_if<WorldPlane>(&first)) {
            if (const auto* plane_b = std::get_if<WorldPlane>(&*second)) {
                const Vector3 normal_a = related_direction(plane_a->normal, plane_b->normal,
                                                           constraint.direction_relation);
                double measured = (plane_a->origin - plane_b->origin).dot(plane_b->normal);
                if (constraint.distance_relation == DistanceRelation::Unsigned)
                    measured = std::abs(measured);
                if (constraint.distance_relation == DistanceRelation::OppositeSecondNormal)
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

class Problem final {
public:
    Problem(const Model& model, const SolverOptions& options) : model_(model), options_(options) {
        if (model.bodies.empty())
            throw std::invalid_argument("assembly model requires at least one body");
        if (!(options.length_scale > 0.0) || !(options.angle_scale > 0.0) ||
            !(options.finite_difference_step > 0.0) || !(options.initial_damping > 0.0))
            throw std::invalid_argument("solver scales and steps must be positive");
        for (std::size_t index = 0; index < model.bodies.size(); ++index) {
            const Body& body = model.bodies[index];
            if (body.id.empty() || body_index_.count(body.id))
                throw std::invalid_argument("body IDs must be non-empty and unique");
            if (!finite(body.initial_pose.translation) || !finite(body.initial_pose.rotation))
                throw std::invalid_argument("body pose must be finite");
            (void)normalized(body.initial_pose.rotation);
            body_index_[body.id] = index;
        }
        for (const GeometryElement& element : model.geometry) {
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
        std::unordered_map<std::string, bool> constraint_ids;
        for (const Constraint& constraint : model.constraints) {
            if (constraint.id.empty() || constraint_ids[constraint.id])
                throw std::invalid_argument("constraint IDs must be non-empty and unique");
            constraint_ids[constraint.id] = true;
            (void)body(constraint.first.body_id);
            if (constraint.kind == ConstraintKind::Fix) {
                if (constraint.second)
                    throw std::invalid_argument("Fix must have exactly one endpoint");
            } else if (constraint.kind == ConstraintKind::Rigid) {
                if (!constraint.second || constraint.first.body_id == constraint.second->body_id)
                    throw std::invalid_argument("Rigid requires two different bodies");
                (void)body(constraint.second->body_id);
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
                (constraint.value < 0.0 || constraint.value > kPi))
                throw std::invalid_argument("Angle value must be in [0, pi]");
        }
    }

    State initial_state() const {
        State state;
        for (const Body& body_value : model_.bodies) {
            Pose pose = body_value.initial_pose;
            pose.rotation = value(normalized(pose.rotation));
            state.poses.push_back(pose);
        }
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

    std::vector<ResidualBlock> blocks(const State& state) const {
        std::vector<ResidualBlock> result;
        for (const Constraint& constraint : model_.constraints) {
            if (constraint.kind == ConstraintKind::Fix) {
                const Pose& current = state.poses[body_index_.at(constraint.first.body_id)];
                const Pose target = constraint.fixed_pose.value_or(
                    model_.bodies[body_index_.at(constraint.first.body_id)].initial_pose);
                Eigen::VectorXd residual(6);
                residual << (eigen(current.translation) - eigen(target.translation)) /
                                options_.length_scale,
                    rotation_vector(normalized(target.rotation).conjugate() *
                                    normalized(current.rotation)) /
                        options_.angle_scale;
                result.push_back({constraint.id, std::move(residual)});
                continue;
            }
            if (constraint.kind == ConstraintKind::Rigid) {
                const Pose& first = state.poses[body_index_.at(constraint.first.body_id)];
                const Pose& second = state.poses[body_index_.at(constraint.second->body_id)];
                const Pose target = *constraint.fixed_pose;
                const EigenQuaternion second_rotation = normalized(second.rotation);
                const Vector3 relative_translation = second_rotation.conjugate() *
                    (eigen(first.translation) - eigen(second.translation));
                const EigenQuaternion relative_rotation = second_rotation.conjugate() * normalized(first.rotation);
                Eigen::VectorXd residual(6);
                residual << (relative_translation - eigen(target.translation)) / options_.length_scale,
                    rotation_vector(normalized(target.rotation).conjugate() * relative_rotation) / options_.angle_scale;
                result.push_back({constraint.id, std::move(residual)});
                continue;
            }
            const GeometryElement& first_element = geometry(constraint.first);
            const GeometryElement& second_element = geometry(*constraint.second);
            const WorldGeometry first =
                world_geometry(first_element, state.poses[body_index_.at(first_element.body_id)]);
            const WorldGeometry second =
                world_geometry(second_element, state.poses[body_index_.at(second_element.body_id)]);
            result.push_back(
                {constraint.id, constraint_residual(constraint, first, second, options_)});
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

    std::size_t parameter_count() const { return model_.bodies.size() * 6; }
    const Model& model() const { return model_; }

private:
    std::size_t body(const std::string& id) const {
        const auto found = body_index_.find(id);
        if (found == body_index_.end())
            throw std::invalid_argument("constraint references an unknown body: " + id);
        return found->second;
    }

    const GeometryElement& geometry(const GeometryRef& reference) const {
        (void)body(reference.body_id);
        const auto found =
            geometry_index_.find(geometry_key(reference.body_id, reference.geometry_id));
        if (found == geometry_index_.end())
            throw std::invalid_argument("constraint references unknown geometry: " +
                                        reference.geometry_id);
        return *found->second;
    }

    const Model& model_;
    const SolverOptions& options_;
    std::unordered_map<std::string, std::size_t> body_index_;
    std::unordered_map<std::string, const GeometryElement*> geometry_index_;
};

SolveResult make_result(const Problem& problem, const State& state, const SolveStatus status,
                        const std::size_t iterations, const std::string& diagnostic) {
    SolveResult result;
    result.status = status;
    result.iterations = iterations;
    result.diagnostic = diagnostic;
    for (std::size_t index = 0; index < problem.model().bodies.size(); ++index)
        result.bodies.push_back({problem.model().bodies[index].id, state.poses[index]});
    const auto blocks = problem.blocks(state);
    double squared = 0.0;
    for (const auto& block : blocks) {
        const double norm = block.values.norm();
        result.residuals.push_back({block.id, norm});
        squared += block.values.squaredNorm();
    }
    result.normalized_residual = std::sqrt(squared);
    return result;
}

}  // namespace

SolveResult Solver::solve(const Model& model, const SolverOptions& options) const {
    try {
        Problem problem(model, options);
        State state = problem.initial_state();
        Vector residual = problem.residual(state);
        if (!residual.allFinite())
            return make_result(problem, state, SolveStatus::NumericalFailure, 0,
                               "initial residual is not finite");
        if (residual.norm() <= options.residual_tolerance)
            return make_result(problem, state, SolveStatus::Converged, 0,
                               "constraints already satisfied");
        if (residual.size() == 0)
            return make_result(problem, state, SolveStatus::Converged, 0,
                               "model has no active constraints");

        double damping = options.initial_damping;
        for (std::size_t iteration = 1; iteration <= options.max_iterations; ++iteration) {
            Eigen::MatrixXd jacobian(residual.size(),
                                     static_cast<Eigen::Index>(problem.parameter_count()));
            for (Eigen::Index column = 0; column < jacobian.cols(); ++column) {
                Vector perturbation = Vector::Zero(jacobian.cols());
                perturbation[column] = options.finite_difference_step;
                const Vector shifted = problem.residual(problem.incremented(state, perturbation));
                if (shifted.size() != residual.size() || !shifted.allFinite())
                    return make_result(problem, state, SolveStatus::NumericalFailure, iteration,
                                       "finite-difference residual is invalid");
                jacobian.col(column) = (shifted - residual) / options.finite_difference_step;
            }
            Eigen::MatrixXd normal = jacobian.transpose() * jacobian;
            normal.diagonal().array() += damping;
            const Vector step = normal.ldlt().solve(-jacobian.transpose() * residual);
            if (!step.allFinite())
                return make_result(problem, state, SolveStatus::NumericalFailure, iteration,
                                   "linear solve produced a non-finite step");
            if (step.norm() <= options.step_tolerance) {
                const SolveStatus status = residual.norm() <= options.residual_tolerance
                                               ? SolveStatus::Converged
                                               : SolveStatus::MaxIterations;
                return make_result(problem, state, status, iteration,
                                   status == SolveStatus::Converged
                                       ? "converged"
                                       : "stationary point above tolerance");
            }
            const State candidate = problem.incremented(state, step);
            const Vector candidate_residual = problem.residual(candidate);
            if (candidate_residual.allFinite() &&
                candidate_residual.squaredNorm() < residual.squaredNorm()) {
                state = candidate;
                residual = candidate_residual;
                damping = std::max(damping * 0.25, 1.0e-12);
                if (residual.norm() <= options.residual_tolerance)
                    return make_result(problem, state, SolveStatus::Converged, iteration,
                                       "converged");
            } else {
                damping = std::min(damping * 10.0, 1.0e12);
            }
        }
        return make_result(problem, state, SolveStatus::MaxIterations, options.max_iterations,
                           "maximum iteration count reached");
    } catch (const std::invalid_argument& error) {
        SolveResult result;
        result.status = SolveStatus::InvalidModel;
        result.diagnostic = error.what();
        return result;
    } catch (const std::exception& error) {
        SolveResult result;
        result.status = SolveStatus::NumericalFailure;
        result.diagnostic = error.what();
        return result;
    }
}

}  // namespace occccad::assembly
