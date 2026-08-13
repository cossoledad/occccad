#include <planegcs/GCS.h>

#include "occccad/geometry/sketch/sketch_solver.h"

#include <cmath>
#include <cstddef>
#include <exception>
#include <string>
#include <unordered_map>
#include <utility>
#include <vector>

namespace occccad::geometry::sketch {
namespace {

struct PointState {
    PointEntity entity;
    GCS::Point point;
};

struct LineState {
    LineEntity entity;
    GCS::Point start;
    GCS::Point end;
    GCS::Line line;
};

class PlaneGcsSketchSolver final : public SketchSolver {
public:
    [[nodiscard]] SolveResult solve(const SketchModel& model) const override {
        std::vector<PointState> points;
        std::vector<LineState> lines;
        points.reserve(model.points.size());
        lines.reserve(model.lines.size());
        std::unordered_map<std::string, std::size_t> point_indices;
        std::unordered_map<std::string, std::size_t> line_indices;

        for (const auto& point : model.points) {
            if (!valid_id_and_point(point.id, point.point)) {
                return invalid("point id and coordinates must be valid");
            }
            if (!point_indices.emplace(point.id, points.size()).second ||
                line_indices.count(point.id) != 0U) {
                return invalid("duplicate entity id: " + point.id);
            }
            points.push_back(PointState{point, {}});
        }
        for (auto& point : points) {
            point.point = {&point.entity.point.x, &point.entity.point.y};
        }

        for (const auto& line : model.lines) {
            if (!valid_id_and_point(line.id, line.start) || !finite(line.end) ||
                distance_squared(line.start, line.end) <= 1.0e-18) {
                return invalid("line id and endpoints must define a non-degenerate line");
            }
            if (!line_indices.emplace(line.id, lines.size()).second ||
                point_indices.count(line.id) != 0U) {
                return invalid("duplicate entity id: " + line.id);
            }
            lines.push_back(LineState{line, {}, {}, {}});
        }
        for (auto& line : lines) {
            line.start = {&line.entity.start.x, &line.entity.start.y};
            line.end = {&line.entity.end.x, &line.entity.end.y};
            line.line.p1 = line.start;
            line.line.p2 = line.end;
        }
        if (points.empty() && lines.empty()) {
            return invalid("sketch must contain at least one entity");
        }

        std::vector<double> fixed_values;
        fixed_values.reserve(model.constraints.size() * 4U + 12U);
        const auto append_value = [&fixed_values](double value) -> double* {
            fixed_values.push_back(value);
            return &fixed_values.back();
        };
        GCS::Point origin{append_value(0.0), append_value(0.0)};
        GCS::Point x_axis_end{append_value(1.0), append_value(0.0)};
        GCS::Point y_axis_end{append_value(0.0), append_value(1.0)};
        GCS::Line x_axis;
        x_axis.p1 = origin;
        x_axis.p2 = x_axis_end;
        GCS::Line y_axis;
        y_axis.p1 = origin;
        y_axis.p2 = y_axis_end;
        GCS::System system;
        std::unordered_map<int, std::string> constraint_ids;

        try {
            int tag = 1;
            for (const auto& constraint : model.constraints) {
                if (constraint.id.empty()) {
                    return invalid("constraint id must not be empty");
                }
                if (!constraint_ids.emplace(tag, constraint.id).second) {
                    return invalid("duplicate constraint tag");
                }
                const auto error =
                    add_constraint(constraint, tag, point_indices, line_indices, points, lines,
                                   origin, x_axis, y_axis, append_value, system);
                if (!error.empty()) {
                    return invalid(error);
                }
                ++tag;
            }

            GCS::VEC_pD parameters;
            parameters.reserve(points.size() * 2U + lines.size() * 4U);
            for (auto& point : points) {
                parameters.push_back(point.point.x);
                parameters.push_back(point.point.y);
            }
            for (auto& line : lines) {
                parameters.push_back(line.start.x);
                parameters.push_back(line.start.y);
                parameters.push_back(line.end.x);
                parameters.push_back(line.end.y);
            }

            const int status = system.solve(parameters, true, GCS::DogLeg);
            if (status != GCS::Success) {
                return {SolveStatus::failed,
                        {},
                        {},
                        -1,
                        {},
                        {},
                        "PlaneGCS failed with status " + std::to_string(status)};
            }
            system.applySolution();
            system.diagnose(GCS::DogLeg);

            GCS::VEC_I conflicting;
            GCS::VEC_I redundant;
            system.getConflicting(conflicting);
            system.getRedundant(redundant);
            if (!conflicting.empty() || !redundant.empty()) {
                SolveResult diagnosis;
                diagnosis.status =
                    !conflicting.empty() ? SolveStatus::conflicting : SolveStatus::redundant;
                diagnosis.degrees_of_freedom = system.dofsNumber();
                append_constraint_ids(conflicting, constraint_ids,
                                      diagnosis.conflicting_constraint_ids);
                append_constraint_ids(redundant, constraint_ids,
                                      diagnosis.redundant_constraint_ids);
                diagnosis.diagnostic =
                    !conflicting.empty() ? "conflicting constraints" : "redundant constraints";
                return diagnosis;
            }

            SolveResult result;
            result.degrees_of_freedom = system.dofsNumber();
            result.status = result.degrees_of_freedom > 0 ? SolveStatus::under_constrained
                                                          : SolveStatus::solved;
            result.points.reserve(points.size());
            result.lines.reserve(lines.size());
            for (const auto& point : points)
                result.points.push_back(point.entity);
            for (const auto& line : lines)
                result.lines.push_back(line.entity);
            return result;
        } catch (const std::exception& error) {
            return {SolveStatus::failed,
                    {},
                    {},
                    -1,
                    {},
                    {},
                    "PlaneGCS exception: " + std::string(error.what())};
        }
    }

private:
    using PointIndex = std::unordered_map<std::string, std::size_t>;
    using LineIndex = std::unordered_map<std::string, std::size_t>;

    static bool finite(const Vec2& point) {
        return std::isfinite(point.x) && std::isfinite(point.y);
    }
    static bool valid_id_and_point(const std::string& id, const Vec2& point) {
        return !id.empty() && finite(point);
    }
    static double distance_squared(const Vec2& first, const Vec2& second) {
        const double x = first.x - second.x;
        const double y = first.y - second.y;
        return x * x + y * y;
    }
    static SolveResult invalid(std::string diagnostic) {
        return {SolveStatus::invalid_model, {}, {}, -1, {}, {}, std::move(diagnostic)};
    }
    static void append_constraint_ids(const GCS::VEC_I& tags,
                                      const std::unordered_map<int, std::string>& ids,
                                      std::vector<std::string>& destination) {
        for (const int tag : tags) {
            const auto found = ids.find(tag);
            if (found != ids.end())
                destination.push_back(found->second);
        }
    }

    template <typename AppendValue>
    static std::string add_constraint(const SketchConstraint& constraint, int tag,
                                      const PointIndex& point_indices,
                                      const LineIndex& line_indices,
                                      std::vector<PointState>& points,
                                      std::vector<LineState>& lines, GCS::Point& origin,
                                      GCS::Line& x_axis, GCS::Line& y_axis,
                                      AppendValue&& append_value, GCS::System& system) {
        if (constraint.kind == ConstraintKind::coincident) {
            if (constraint.references.size() != 2U) {
                return "coincident constraint requires two point references";
            }
            GCS::Point* first = resolve_point(constraint.references[0], point_indices, line_indices,
                                              points, lines, origin);
            GCS::Point* second = resolve_point(constraint.references[1], point_indices,
                                               line_indices, points, lines, origin);
            if (first == nullptr || second == nullptr) {
                return "coincident constraint contains an invalid point reference";
            }
            system.addConstraintP2PCoincident(*first, *second, tag);
            return {};
        }
        if (constraint.kind == ConstraintKind::parallel) {
            if (constraint.references.size() != 2U) {
                return "parallel constraint requires two direction references";
            }
            GCS::Line* first =
                resolve_line(constraint.references[0], line_indices, lines, x_axis, y_axis);
            GCS::Line* second =
                resolve_line(constraint.references[1], line_indices, lines, x_axis, y_axis);
            if (first == nullptr || second == nullptr) {
                return "parallel constraint contains an invalid direction reference";
            }
            system.addConstraintParallel(*first, *second, tag);
            return {};
        }
        if (constraint.kind == ConstraintKind::fixed_point) {
            if (constraint.references.size() != 1U || !finite(constraint.fixed_point)) {
                return "fixed-point constraint requires one point reference and finite value";
            }
            GCS::Point* point = resolve_point(constraint.references[0], point_indices, line_indices,
                                              points, lines, origin);
            if (point == nullptr)
                return "fixed-point constraint has an invalid point reference";
            system.addConstraintCoordinateX(*point, append_value(constraint.fixed_point.x), tag);
            system.addConstraintCoordinateY(*point, append_value(constraint.fixed_point.y), tag);
            return {};
        }
        return "unsupported constraint kind";
    }

    static GCS::Point* resolve_point(const GeometryRef& reference, const PointIndex& point_indices,
                                     const LineIndex& line_indices, std::vector<PointState>& points,
                                     std::vector<LineState>& lines, GCS::Point& origin) {
        if (reference.target == GeometryTarget::sketch_origin &&
            reference.sub_element == SubElement::point) {
            return &origin;
        }
        if (reference.target != GeometryTarget::entity)
            return nullptr;
        if (reference.sub_element == SubElement::point) {
            const auto found = point_indices.find(reference.entity_id);
            return found == point_indices.end() ? nullptr : &points[found->second].point;
        }
        const auto found = line_indices.find(reference.entity_id);
        if (found == line_indices.end())
            return nullptr;
        if (reference.sub_element == SubElement::start)
            return &lines[found->second].start;
        if (reference.sub_element == SubElement::end)
            return &lines[found->second].end;
        return nullptr;
    }

    static GCS::Line* resolve_line(const GeometryRef& reference, const LineIndex& line_indices,
                                   std::vector<LineState>& lines, GCS::Line& x_axis,
                                   GCS::Line& y_axis) {
        if (reference.target == GeometryTarget::sketch_x_axis &&
            reference.sub_element == SubElement::direction) {
            return &x_axis;
        }
        if (reference.target == GeometryTarget::sketch_y_axis &&
            reference.sub_element == SubElement::direction) {
            return &y_axis;
        }
        if (reference.target != GeometryTarget::entity ||
            (reference.sub_element != SubElement::whole &&
             reference.sub_element != SubElement::direction)) {
            return nullptr;
        }
        const auto found = line_indices.find(reference.entity_id);
        return found == line_indices.end() ? nullptr : &lines[found->second].line;
    }
};
}  // namespace

std::unique_ptr<SketchSolver> make_plane_gcs_sketch_solver() {
    return std::make_unique<PlaneGcsSketchSolver>();
}
}  // namespace occccad::geometry::sketch
