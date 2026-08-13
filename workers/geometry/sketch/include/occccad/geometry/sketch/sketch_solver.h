#pragma once

#include <memory>
#include <string>
#include <vector>

namespace occccad::geometry::sketch {

struct Vec2 {
    double x{};
    double y{};
};

enum class EntityRole { profile, construction };

struct PointEntity {
    std::string id;
    Vec2 point;
    EntityRole role{EntityRole::construction};
};

struct LineEntity {
    std::string id;
    Vec2 start;
    Vec2 end;
    EntityRole role{EntityRole::profile};
};

enum class GeometryTarget { entity, sketch_x_axis, sketch_y_axis, sketch_origin };
enum class SubElement { whole, point, start, end, direction };

struct GeometryRef {
    GeometryTarget target{GeometryTarget::entity};
    std::string entity_id;
    SubElement sub_element{SubElement::whole};
};

enum class ConstraintKind { coincident, parallel, fixed_point };

struct SketchConstraint {
    std::string id;
    ConstraintKind kind{ConstraintKind::coincident};
    std::vector<GeometryRef> references;
    Vec2 fixed_point;
};

struct SketchModel {
    std::vector<PointEntity> points;
    std::vector<LineEntity> lines;
    std::vector<SketchConstraint> constraints;
};

enum class SolveStatus { solved, under_constrained, invalid_model, redundant, conflicting, failed };

struct SolveResult {
    SolveStatus status{SolveStatus::failed};
    std::vector<PointEntity> points;
    std::vector<LineEntity> lines;
    int degrees_of_freedom{-1};
    std::vector<std::string> conflicting_constraint_ids;
    std::vector<std::string> redundant_constraint_ids;
    std::string diagnostic;
};

class SketchSolver {
public:
    virtual ~SketchSolver() = default;
    [[nodiscard]] virtual SolveResult solve(const SketchModel& model) const = 0;
};

[[nodiscard]] std::unique_ptr<SketchSolver> make_plane_gcs_sketch_solver();
}  // namespace occccad::geometry::sketch
