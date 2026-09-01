#include <occccad/assembly/solver.hpp>

#include <Eigen/Cholesky>
#include <Eigen/Core>
#include <Eigen/Geometry>
#include <Eigen/LU>
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
            return constraint_residual(swapped, *second, first, options);
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
        const double cosine = std::clamp(first_direction.dot(second_direction), -1.0, 1.0);
        const double sine = first_direction.cross(second_direction).norm();
        const double angle = std::atan2(sine, cosine);
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

double pose_error(const Pose& first, const Pose& second, const SolverOptions& options) {
    const Vector3 translation =
        (eigen(first.translation) - eigen(second.translation)) / options.length_scale;
    const Vector3 rotation =
        rotation_vector(normalized(second.rotation).conjugate() * normalized(first.rotation)) /
        options.angle_scale;
    return std::sqrt(translation.squaredNorm() + rotation.squaredNorm());
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
        : model_(model), options_(options) {
        validate_and_index();
        build_clusters();
        build_components();
    }

    const Model& model() const { return model_; }
    const SolverOptions& options() const { return options_; }
    const std::vector<Cluster>& clusters() const { return clusters_; }
    std::vector<Cluster>& clusters() { return clusters_; }
    const std::vector<Component>& components() const { return components_; }

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
            !(options_.finite_difference_step > 0.0) || !(options_.initial_damping > 0.0) ||
            !(options_.rank_tolerance > 0.0))
            throw std::invalid_argument(
                "solver scales, steps, damping and rank tolerance must be positive");
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
                (constraint.value < 0.0 || constraint.value > kPi))
                throw std::invalid_argument("Angle value must be in [0, pi]");
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
                    } else if (pose_error(found->second, candidate, options_) >
                               options_.residual_tolerance) {
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
            if (cluster.ground_pose && pose_error(*cluster.ground_pose, root_target, options_) >
                                           options_.residual_tolerance)
                throw std::invalid_argument("fixed constraints in one rigid cluster conflict");
            cluster.ground_pose = root_target;
            cluster.initial_pose = root_target;
            cluster.ground_constraint_ids.push_back(constraint.id);
        }
    }

    void build_components() {
        DisjointSet sets(clusters_.size());
        for (const Constraint& constraint : model_.constraints) {
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
            for (std::size_t index = 0; index < model_.constraints.size(); ++index) {
                const Constraint& constraint = model_.constraints[index];
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

    std::vector<ResidualBlock> blocks(const State& state) const {
        const std::vector<Pose> bodies = assembly_.body_poses(cluster_poses(state));
        std::vector<ResidualBlock> result;
        for (const std::size_t constraint_index : component_.constraint_indices) {
            const Constraint& constraint = assembly_.model().constraints[constraint_index];
            const GeometryElement& first_element = assembly_.geometry(constraint.first);
            const GeometryElement& second_element = assembly_.geometry(*constraint.second);
            const WorldGeometry first =
                world_geometry(first_element, bodies[assembly_.body_index(first_element.body_id)]);
            const WorldGeometry second = world_geometry(
                second_element, bodies[assembly_.body_index(second_element.body_id)]);
            result.push_back({constraint.id,
                              constraint_residual(constraint, first, second, assembly_.options())});
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

    Eigen::MatrixXd jacobian(const State& state, const Vector& residual) const {
        Eigen::MatrixXd result(residual.size(), static_cast<Eigen::Index>(parameter_count()));
        for (Eigen::Index column = 0; column < result.cols(); ++column) {
            Vector perturbation = Vector::Zero(result.cols());
            perturbation[column] = assembly_.options().finite_difference_step;
            const Vector shifted = this->residual(incremented(state, perturbation));
            if (shifted.size() != residual.size() || !shifted.allFinite())
                throw std::runtime_error("finite-difference residual is invalid");
            result.col(column) = (shifted - residual) / assembly_.options().finite_difference_step;
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

private:
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
    if (residual.size() == 0 || residual.norm() <= options.residual_tolerance)
        return {SolveStatus::Converged, result.state, 0,
                residual.size() == 0 ? "component has no active equations"
                                     : "constraints already satisfied"};
    if (problem.parameter_count() == 0)
        return {SolveStatus::MaxIterations, result.state, 0,
                "grounded component violates an active constraint"};
    double damping = options.initial_damping;
    for (std::size_t iteration = 1; iteration <= options.max_iterations; ++iteration) {
        const Eigen::MatrixXd jacobian = problem.jacobian(result.state, residual);
        Eigen::MatrixXd normal = jacobian.transpose() * jacobian;
        normal.diagonal().array() += damping;
        const Vector step = normal.ldlt().solve(-jacobian.transpose() * residual);
        if (!step.allFinite())
            return {SolveStatus::NumericalFailure, result.state, iteration,
                    "linear solve produced a non-finite step"};
        if (step.norm() <= options.step_tolerance) {
            const SolveStatus status = residual.norm() <= options.residual_tolerance
                                           ? SolveStatus::Converged
                                           : SolveStatus::MaxIterations;
            return {status, result.state, iteration,
                    status == SolveStatus::Converged ? "converged"
                                                     : "stationary point above tolerance"};
        }
        const State candidate = problem.incremented(result.state, step);
        const Vector candidate_residual = problem.residual(candidate);
        if (candidate_residual.allFinite() &&
            candidate_residual.squaredNorm() < residual.squaredNorm()) {
            result.state = candidate;
            residual = candidate_residual;
            damping = std::max(damping * 0.25, 1.0e-12);
            if (residual.norm() <= options.residual_tolerance)
                return {SolveStatus::Converged, result.state, iteration, "converged"};
        } else {
            damping = std::min(damping * 10.0, 1.0e12);
        }
    }
    return {SolveStatus::MaxIterations, result.state, options.max_iterations,
            "maximum iteration count reached"};
}

std::size_t matrix_rank(const Eigen::MatrixXd& matrix, const double tolerance) {
    if (matrix.rows() == 0 || matrix.cols() == 0)
        return 0;
    Eigen::FullPivLU<Eigen::MatrixXd> decomposition(matrix);
    decomposition.setThreshold(tolerance);
    return static_cast<std::size_t>(decomposition.rank());
}

std::vector<std::string> redundant_constraints(const ComponentProblem& problem, const State& state,
                                               const SolverOptions& options) {
    const auto blocks = problem.blocks(state);
    const Vector all_residuals = problem.residual(state);
    const Eigen::MatrixXd all_jacobian = problem.jacobian(state, all_residuals);
    std::vector<std::string> result;
    Eigen::MatrixXd accepted(0, all_jacobian.cols());
    std::size_t accepted_rank = 0;
    Eigen::Index row = 0;
    for (const ResidualBlock& block : blocks) {
        Eigen::MatrixXd candidate(accepted.rows() + block.values.size(), accepted.cols());
        if (accepted.rows() > 0)
            candidate.topRows(accepted.rows()) = accepted;
        candidate.bottomRows(block.values.size()) =
            all_jacobian.middleRows(row, block.values.size());
        const std::size_t candidate_rank = matrix_rank(candidate, options.rank_tolerance);
        if (candidate_rank == accepted_rank)
            result.push_back(block.id);
        else {
            accepted = std::move(candidate);
            accepted_rank = candidate_rank;
        }
        row += block.values.size();
    }
    return result;
}

ResidualBlock evaluate_constraint(const CompiledAssembly& assembly, const Constraint& constraint,
                                  const std::vector<Pose>& body_poses) {
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
        return {constraint.id, std::move(residual)};
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
        return {constraint.id, std::move(residual)};
    }
    const GeometryElement& first_element = assembly.geometry(constraint.first);
    const GeometryElement& second_element = assembly.geometry(*constraint.second);
    const WorldGeometry first =
        world_geometry(first_element, body_poses[assembly.body_index(first_element.body_id)]);
    const WorldGeometry second =
        world_geometry(second_element, body_poses[assembly.body_index(second_element.body_id)]);
    return {constraint.id, constraint_residual(constraint, first, second, assembly.options())};
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
                     result.status == SolveStatus::Converged)
                result.status = SolveStatus::MaxIterations;

            const Vector residual = problem.residual(solution.state);
            const Eigen::MatrixXd jacobian = problem.jacobian(solution.state, residual);
            const std::size_t rank = matrix_rank(jacobian, options.rank_tolerance);
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
            for (const std::size_t cluster_index : component.cluster_indices) {
                for (const std::size_t body : assembly.clusters()[cluster_index].body_indices)
                    component_dof.body_ids.push_back(model.bodies[body].id);
            }
            std::sort(component_dof.body_ids.begin(), component_dof.body_ids.end());
            result.components.push_back(std::move(component_dof));

            if (component.selected && solution.status == SolveStatus::Converged) {
                const auto redundant = redundant_constraints(problem, solution.state, options);
                result.redundant_constraint_ids.insert(result.redundant_constraint_ids.end(),
                                                       redundant.begin(), redundant.end());
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
        for (const Constraint& constraint : model.constraints) {
            if (constraint.mode == ConstraintMode::Suppressed)
                continue;
            const ResidualBlock block = evaluate_constraint(assembly, constraint, body_poses);
            const double norm = block.values.norm();
            result.residuals.push_back({constraint.id, norm});
            if (active(constraint))
                squared_residual += block.values.squaredNorm();
            for (Eigen::Index equation = 0; equation < block.values.size(); ++equation) {
                result.equation_residuals.push_back(
                    {constraint.id + "/equation/" + std::to_string(equation),
                     connection_id(constraint), constraint.id, static_cast<std::size_t>(equation),
                     block.values[equation]});
            }
            const bool selected =
                options.affected_body_ids.empty() ||
                selected_clusters[assembly.cluster_index(constraint.first.body_id)];
            if (active(constraint) && selected && norm > options.residual_tolerance)
                result.conflicting_constraint_ids.push_back(constraint.id);
        }
        result.normalized_residual = std::sqrt(squared_residual);
        std::sort(result.redundant_constraint_ids.begin(), result.redundant_constraint_ids.end());
        result.redundant_constraint_ids.erase(std::unique(result.redundant_constraint_ids.begin(),
                                                          result.redundant_constraint_ids.end()),
                                              result.redundant_constraint_ids.end());
        std::sort(result.conflicting_constraint_ids.begin(),
                  result.conflicting_constraint_ids.end());

        if (!result.conflicting_constraint_ids.empty()) {
            result.classification = SolveClassification::Conflicting;
            result.diagnostics.push_back({"CONFLICTING_CONSTRAINTS",
                                          {},
                                          {},
                                          result.conflicting_constraint_ids,
                                          "active constraints remain above tolerance"});
        } else if (result.status == SolveStatus::NumericalFailure ||
                   result.status == SolveStatus::MaxIterations) {
            result.classification = SolveClassification::NonConvergent;
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
        result.diagnostic = result.status == SolveStatus::Converged
                                ? "assembly components solved"
                                : "one or more assembly components did not converge";
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
