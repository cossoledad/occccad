#include "assembly_corpus.hpp"

#include <cmath>
#include <optional>
#include <string>
#include <utility>
#include <vector>

namespace occccad::assembly::corpus {
namespace {

constexpr double kPi = 3.141592653589793238462643383279502884;

GeometryRef ref(const std::string& body, const std::string& geometry = {}) {
    return {body, geometry};
}

Constraint unary(const std::string& id, const ConstraintKind kind, const std::string& body) {
    Constraint result;
    result.id = id;
    result.kind = kind;
    result.first = ref(body);
    return result;
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

Constraint fixed(const std::string& body) {
    Constraint result = unary("fix-" + body, ConstraintKind::Fix, body);
    result.fixed_pose = Pose{};
    return result;
}

GeometryElement point(const std::string& body, const std::string& id, const Vec3 value = {}) {
    return {id, body, PointGeometry{value}};
}

GeometryElement axis(const std::string& body, const std::string& id,
                     const Vec3 direction = {0.0, 0.0, 1.0}) {
    return {id, body, AxisGeometry{{}, direction}};
}

GeometryElement plane(const std::string& body, const std::string& id,
                      const Vec3 normal = {0.0, 0.0, 1.0}) {
    return {id, body, PlaneGeometry{{}, normal}};
}

FreedomCase free_body() {
    Model model;
    model.bodies = {{"moving", {}}};
    return {"free-body",
            std::move(model),
            "moving",
            0,
            6,
            {MotionKind::TranslateX, MotionKind::TranslateY, MotionKind::TranslateZ,
             MotionKind::RotateX, MotionKind::RotateY, MotionKind::RotateZ},
            {}};
}

FreedomCase fixed_body() {
    Model model;
    model.bodies = {{"moving", {}}};
    model.constraints = {fixed("moving")};
    return {"fixed-body",
            std::move(model),
            "moving",
            0,
            0,
            {},
            {MotionKind::TranslateX, MotionKind::TranslateY, MotionKind::TranslateZ,
             MotionKind::RotateX, MotionKind::RotateY, MotionKind::RotateZ}};
}

FreedomCase plane_coincidence() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {plane("ground", "plane"), plane("moving", "plane")};
    Constraint coincidence = binary("plane-coincidence", ConstraintKind::Coincident,
                                    ref("moving", "plane"), ref("ground", "plane"));
    coincidence.direction_relation = DirectionRelation::Same;
    model.constraints = {fixed("ground"), coincidence};
    return {"plane-coincidence",
            std::move(model),
            "moving",
            3,
            0,
            {MotionKind::TranslateX, MotionKind::TranslateY, MotionKind::RotateZ},
            {MotionKind::TranslateZ, MotionKind::RotateX, MotionKind::RotateY}};
}

FreedomCase cylindrical_connection() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {axis("ground", "axis"), axis("moving", "axis")};
    Constraint concentric = binary("concentric", ConstraintKind::Concentric, ref("moving", "axis"),
                                   ref("ground", "axis"));
    concentric.direction_relation = DirectionRelation::Same;
    model.constraints = {fixed("ground"), concentric};
    return {
        "cylindrical",
        std::move(model),
        "moving",
        2,
        0,
        {MotionKind::TranslateZ, MotionKind::RotateZ},
        {MotionKind::TranslateX, MotionKind::TranslateY, MotionKind::RotateX, MotionKind::RotateY}};
}

FreedomCase revolute_connection() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {axis("ground", "axis"), axis("moving", "axis"), point("ground", "origin"),
                      point("moving", "origin")};
    Constraint concentric = binary("concentric", ConstraintKind::Concentric, ref("moving", "axis"),
                                   ref("ground", "axis"));
    concentric.direction_relation = DirectionRelation::Same;
    model.constraints = {fixed("ground"), concentric,
                         binary("axial-position", ConstraintKind::Coincident,
                                ref("moving", "origin"), ref("ground", "origin"))};
    return {"revolute",
            std::move(model),
            "moving",
            1,
            0,
            {MotionKind::RotateZ},
            {MotionKind::TranslateX, MotionKind::TranslateY, MotionKind::TranslateZ,
             MotionKind::RotateX, MotionKind::RotateY}};
}

FreedomCase prismatic_connection() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {axis("ground", "z-axis"), axis("moving", "z-axis"),
                      axis("ground", "clocking-axis", {1.0, 0.0, 0.0}),
                      axis("moving", "clocking-axis", {1.0, 0.0, 0.0})};
    Constraint concentric = binary("concentric", ConstraintKind::Concentric,
                                   ref("moving", "z-axis"), ref("ground", "z-axis"));
    concentric.direction_relation = DirectionRelation::Same;
    Constraint clocking = binary("clocking", ConstraintKind::Angle, ref("moving", "clocking-axis"),
                                 ref("ground", "clocking-axis"));
    clocking.value = kPi / 2.0;
    // The local moving clocking axis starts at +Y so that the target angle is at
    // a regular, differentiable branch rather than the acos singularity at zero.
    model.geometry.back() = axis("moving", "clocking-axis", {0.0, 1.0, 0.0});
    clocking.direction_relation = DirectionRelation::Same;
    model.constraints = {fixed("ground"), concentric, clocking};
    return {"prismatic",
            std::move(model),
            "moving",
            1,
            0,
            {MotionKind::TranslateZ},
            {MotionKind::TranslateX, MotionKind::TranslateY, MotionKind::RotateX,
             MotionKind::RotateY, MotionKind::RotateZ}};
}

FreedomCase ungrounded_point_pair() {
    Model model;
    model.bodies = {{"first", {}}, {"second", {}}};
    model.geometry = {point("first", "origin"), point("second", "origin")};
    model.constraints = {binary("coincident", ConstraintKind::Coincident, ref("first", "origin"),
                                ref("second", "origin"))};
    // The observed first body retains its full six-dimensional common rigid
    // motion. Three additional relative rotations belong to the pair, but are
    // not represented by independent single-body canonical perturbations here.
    return {"ungrounded-point-pair",
            std::move(model),
            "first",
            3,
            6,
            {MotionKind::TranslateX, MotionKind::TranslateY, MotionKind::TranslateZ,
             MotionKind::RotateX, MotionKind::RotateY, MotionKind::RotateZ},
            {}};
}

ClassificationCase duplicate_constraint() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {plane("ground", "plane"), plane("moving", "plane")};
    Constraint first = binary("coincident-a", ConstraintKind::Coincident, ref("moving", "plane"),
                              ref("ground", "plane"));
    first.direction_relation = DirectionRelation::Same;
    Constraint second = first;
    second.id = "coincident-b";
    model.constraints = {fixed("ground"), first, second};
    return {"duplicate-plane-coincidence", std::move(model), SolveStatus::Converged,
            SolveClassification::Redundant};
}

ClassificationCase conflicting_offsets() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {plane("ground", "plane"), plane("moving", "plane")};
    Constraint first = binary("offset-3", ConstraintKind::Distance, ref("moving", "plane"),
                              ref("ground", "plane"));
    first.value = 3.0;
    first.direction_relation = DirectionRelation::Same;
    first.distance_relation = DistanceRelation::AlongSecondNormal;
    Constraint second = first;
    second.id = "offset-5";
    second.value = 5.0;
    model.constraints = {fixed("ground"), first, second};
    return {"conflicting-plane-offsets", std::move(model), SolveStatus::Unsatisfied,
            SolveClassification::Inconsistent};
}

ClassificationCase conflicting_angles() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {axis("ground", "axis"), axis("moving", "axis", {1.0, 0.0, 1.0})};
    Constraint first =
        binary("angle-45", ConstraintKind::Angle, ref("moving", "axis"), ref("ground", "axis"));
    first.value = kPi / 4.0;
    first.direction_relation = DirectionRelation::Same;
    Constraint second = first;
    second.id = "angle-90";
    second.value = kPi / 2.0;
    model.constraints = {fixed("ground"), first, second};
    return {"conflicting-axis-angles", std::move(model), SolveStatus::Unsatisfied,
            SolveClassification::Inconsistent};
}

ClassificationCase degenerate_axis() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {axis("ground", "axis"), axis("moving", "axis", {0.0, 0.0, 0.0})};
    model.constraints = {fixed("ground"), binary("concentric", ConstraintKind::Concentric,
                                                 ref("moving", "axis"), ref("ground", "axis"))};
    return {"zero-length-axis", std::move(model), SolveStatus::InvalidModel,
            SolveClassification::InvalidModel};
}

ClassificationCase near_degenerate_plane() {
    Model model;
    model.bodies = {{"ground", {}}, {"moving", {}}};
    model.geometry = {plane("ground", "plane"), plane("moving", "plane", {0.0, 0.0, 1.0e-13})};
    model.constraints = {fixed("ground"), binary("coincident", ConstraintKind::Coincident,
                                                 ref("moving", "plane"), ref("ground", "plane"))};
    return {"near-zero-plane-normal", std::move(model), SolveStatus::InvalidModel,
            SolveClassification::InvalidModel};
}

}  // namespace

std::vector<FreedomCase> freedom_cases() {
    return {free_body(),
            fixed_body(),
            plane_coincidence(),
            cylindrical_connection(),
            revolute_connection(),
            prismatic_connection(),
            ungrounded_point_pair()};
}

std::vector<ClassificationCase> classification_cases() {
    return {duplicate_constraint(), conflicting_offsets(), conflicting_angles(), degenerate_axis(),
            near_degenerate_plane()};
}

}  // namespace occccad::assembly::corpus
