#include <planegcs/GCS.h>

#include "occccad/geometry/sketch/sketch_solver.h"

#include <cmath>
#include <cstddef>
#include <deque>
#include <exception>
#include <string>
#include <unordered_map>
#include <unordered_set>
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
struct CircleState {
    CircleEntity entity;
    GCS::Point center;
    GCS::Circle circle;
};
struct ArcState {
    ArcEntity entity;
    Vec2 start_point;
    Vec2 end_point;
    GCS::Point center;
    GCS::Point start;
    GCS::Point end;
    GCS::Arc arc;
};
struct SplineState {
    SplineEntity entity;
    std::vector<GCS::Point> controls;
};

using Index = std::unordered_map<std::string, std::size_t>;

class PlaneGcsSketchSolver final : public SketchSolver {
public:
    [[nodiscard]] SolveResult solve(const SketchModel& model) const override {
        std::vector<PointState> points;
        std::vector<LineState> lines;
        std::vector<CircleState> circles;
        std::vector<ArcState> arcs;
        std::vector<SplineState> splines;
        points.reserve(model.points.size());
        lines.reserve(model.lines.size());
        circles.reserve(model.circles.size());
        arcs.reserve(model.arcs.size());
        splines.reserve(model.splines.size());
        Index point_indices, line_indices, circle_indices, arc_indices, spline_indices;
        std::unordered_set<std::string> entity_ids;
        const auto unique = [&entity_ids](const std::string& id) {
            return !id.empty() && entity_ids.insert(id).second;
        };

        for (const auto& point : model.points) {
            if (!unique(point.id) || !finite(point.point))
                return invalid("invalid or duplicate point: " + point.id);
            point_indices.emplace(point.id, points.size());
            points.push_back({point, {}});
        }
        for (auto& point : points)
            point.point = {&point.entity.point.x, &point.entity.point.y};
        for (const auto& line : model.lines) {
            if (!unique(line.id) || !finite(line.start) || !finite(line.end) ||
                distance_squared(line.start, line.end) <= 1e-18)
                return invalid("invalid or duplicate line: " + line.id);
            line_indices.emplace(line.id, lines.size());
            lines.push_back({line, {}, {}, {}});
        }
        for (auto& line : lines) {
            line.start = {&line.entity.start.x, &line.entity.start.y};
            line.end = {&line.entity.end.x, &line.entity.end.y};
            line.line.p1 = line.start;
            line.line.p2 = line.end;
        }
        for (const auto& circle : model.circles) {
            if (!unique(circle.id) || !finite(circle.center) || !positive(circle.radius))
                return invalid("invalid or duplicate circle: " + circle.id);
            circle_indices.emplace(circle.id, circles.size());
            circles.push_back({circle, {}, {}});
        }
        for (auto& circle : circles) {
            circle.center = {&circle.entity.center.x, &circle.entity.center.y};
            circle.circle.center = circle.center;
            circle.circle.rad = &circle.entity.radius;
        }
        for (const auto& arc : model.arcs) {
            if (!unique(arc.id) || !finite(arc.center) || !positive(arc.radius) ||
                !std::isfinite(arc.start_angle) || !std::isfinite(arc.end_angle))
                return invalid("invalid or duplicate arc: " + arc.id);
            arc_indices.emplace(arc.id, arcs.size());
            arcs.push_back({arc, {}, {}, {}, {}, {}, {}});
        }
        for (auto& arc : arcs) {
            arc.start_point = {
                arc.entity.center.x + arc.entity.radius * std::cos(arc.entity.start_angle),
                arc.entity.center.y + arc.entity.radius * std::sin(arc.entity.start_angle)};
            arc.end_point = {
                arc.entity.center.x + arc.entity.radius * std::cos(arc.entity.end_angle),
                arc.entity.center.y + arc.entity.radius * std::sin(arc.entity.end_angle)};
            arc.center = {&arc.entity.center.x, &arc.entity.center.y};
            arc.start = {&arc.start_point.x, &arc.start_point.y};
            arc.end = {&arc.end_point.x, &arc.end_point.y};
            arc.arc.center = arc.center;
            arc.arc.start = arc.start;
            arc.arc.end = arc.end;
            arc.arc.rad = &arc.entity.radius;
            arc.arc.startAngle = &arc.entity.start_angle;
            arc.arc.endAngle = &arc.entity.end_angle;
        }
        for (const auto& spline : model.splines) {
            if (!unique(spline.id) || spline.degree < 2U || spline.degree > 3U ||
                spline.control_points.size() < spline.degree + 1U)
                return invalid("invalid or duplicate spline: " + spline.id);
            for (const auto& point : spline.control_points)
                if (!finite(point))
                    return invalid("spline has non-finite control point");
            spline_indices.emplace(spline.id, splines.size());
            splines.push_back({spline, {}});
        }
        for (auto& spline : splines) {
            spline.controls.reserve(spline.entity.control_points.size());
            for (auto& point : spline.entity.control_points)
                spline.controls.push_back({&point.x, &point.y});
        }
        if (entity_ids.empty())
            return invalid("sketch must contain at least one entity");

        std::deque<double> constants;
        const auto constant = [&constants](double value) -> double* {
            constants.push_back(value);
            return &constants.back();
        };
        GCS::Point origin{constant(0), constant(0)}, x_axis_end{constant(1), constant(0)},
            y_axis_end{constant(0), constant(1)};
        GCS::Line x_axis, y_axis;
        x_axis.p1 = origin;
        x_axis.p2 = x_axis_end;
        y_axis.p1 = origin;
        y_axis.p2 = y_axis_end;
        GCS::System system;
        std::unordered_map<int, std::string> constraint_ids;
        std::unordered_map<int, bool> constraint_redundancy_tolerated;
        std::unordered_set<std::string> seen_constraint_ids;

        const auto redundancy_tolerated = [&model](const SketchConstraint& candidate) {
            if (candidate.internal || candidate.kind == ConstraintKind::symmetry)
                return true;
            if (candidate.references.empty() || (candidate.kind != ConstraintKind::horizontal &&
                                                 candidate.kind != ConstraintKind::vertical &&
                                                 candidate.kind != ConstraintKind::parallel))
                return false;
            for (const auto& symmetry : model.constraints) {
                if (symmetry.kind != ConstraintKind::symmetry || symmetry.references.size() != 3U)
                    continue;
                const auto& first = symmetry.references[0];
                const auto& center = symmetry.references[1];
                const auto& second = symmetry.references[2];
                if (first.target != GeometryTarget::entity ||
                    second.target != GeometryTarget::entity ||
                    first.entity_id != second.entity_id || first.entity_id.empty() ||
                    !((first.sub_element == SubElement::start &&
                       second.sub_element == SubElement::end) ||
                      (first.sub_element == SubElement::end &&
                       second.sub_element == SubElement::start)))
                    continue;
                const bool horizontal_axis_symmetry =
                    center.target == GeometryTarget::sketch_y_axis;
                const bool vertical_axis_symmetry = center.target == GeometryTarget::sketch_x_axis;
                if ((candidate.kind == ConstraintKind::horizontal && horizontal_axis_symmetry) ||
                    (candidate.kind == ConstraintKind::vertical && vertical_axis_symmetry)) {
                    const auto& line = candidate.references[0];
                    if (line.target == GeometryTarget::entity && line.entity_id == first.entity_id)
                        return true;
                }
                if (candidate.kind == ConstraintKind::parallel &&
                    candidate.references.size() == 2U) {
                    const GeometryTarget expected_axis =
                        horizontal_axis_symmetry ? GeometryTarget::sketch_x_axis
                        : vertical_axis_symmetry ? GeometryTarget::sketch_y_axis
                                                 : GeometryTarget::entity;
                    bool has_line = false, has_axis = false;
                    for (const auto& reference : candidate.references) {
                        has_line = has_line || (reference.target == GeometryTarget::entity &&
                                                reference.entity_id == first.entity_id);
                        has_axis = has_axis || reference.target == expected_axis;
                    }
                    if (expected_axis != GeometryTarget::entity && has_line && has_axis)
                        return true;
                }
            }
            return false;
        };

        try {
            for (auto& arc : arcs)
                system.addConstraintArcRules(arc.arc, 0, true);
            int tag = 1;
            for (const auto& constraint : model.constraints) {
                if (constraint.id.empty() || !seen_constraint_ids.insert(constraint.id).second)
                    return invalid("constraint ids must be unique");
                constraint_ids.emplace(tag, constraint.id);
                // Composite axis symmetry can make its own scalar equation, or
                // the matching H/V/Parallel relation, appear redundant based
                // on insertion order. Preserve that combined design intent;
                // unrelated user redundancy remains an error.
                constraint_redundancy_tolerated.emplace(tag, redundancy_tolerated(constraint));
                const auto error =
                    add_constraint(constraint, tag, point_indices, line_indices, circle_indices,
                                   arc_indices, spline_indices, points, lines, circles, arcs,
                                   splines, origin, x_axis, y_axis, constant, system);
                if (!error.empty())
                    return invalid(error);
                ++tag;
            }

            GCS::VEC_pD parameters;
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
            for (auto& circle : circles) {
                parameters.push_back(circle.center.x);
                parameters.push_back(circle.center.y);
                parameters.push_back(circle.circle.rad);
            }
            for (auto& arc : arcs) {
                parameters.push_back(arc.center.x);
                parameters.push_back(arc.center.y);
                parameters.push_back(arc.arc.rad);
                parameters.push_back(arc.start.x);
                parameters.push_back(arc.start.y);
                parameters.push_back(arc.end.x);
                parameters.push_back(arc.end.y);
                parameters.push_back(arc.arc.startAngle);
                parameters.push_back(arc.arc.endAngle);
            }
            for (auto& spline : splines)
                for (auto& point : spline.controls) {
                    parameters.push_back(point.x);
                    parameters.push_back(point.y);
                }

            const int status = system.solve(parameters, true, GCS::DogLeg);
            if (status != GCS::Success)
                return failed("PlaneGCS failed with status " + std::to_string(status));
            system.applySolution();
            system.diagnose(GCS::DogLeg);
            GCS::VEC_I conflicting, redundant;
            system.getConflicting(conflicting);
            system.getRedundant(redundant);
            GCS::VEC_I user_redundant;
            for (const int redundant_tag : redundant) {
                if (redundant_tag != 0 && !constraint_redundancy_tolerated[redundant_tag])
                    user_redundant.push_back(redundant_tag);
            }
            if (!conflicting.empty() || !user_redundant.empty()) {
                SolveResult result;
                result.status =
                    !conflicting.empty() ? SolveStatus::conflicting : SolveStatus::redundant;
                result.degrees_of_freedom = system.dofsNumber();
                append_constraint_ids(conflicting, constraint_ids,
                                      result.conflicting_constraint_ids);
                append_constraint_ids(user_redundant, constraint_ids,
                                      result.redundant_constraint_ids);
                result.diagnostic =
                    !conflicting.empty() ? "conflicting constraints" : "redundant constraints";
                return result;
            }
            SolveResult result;
            result.degrees_of_freedom = system.dofsNumber();
            result.status = result.degrees_of_freedom > 0 ? SolveStatus::under_constrained
                                                          : SolveStatus::solved;
            for (const auto& value : points)
                result.points.push_back(value.entity);
            for (const auto& value : lines)
                result.lines.push_back(value.entity);
            for (const auto& value : circles)
                result.circles.push_back(value.entity);
            for (const auto& value : arcs)
                result.arcs.push_back(value.entity);
            for (const auto& value : splines)
                result.splines.push_back(value.entity);
            return result;
        } catch (const std::exception& error) {
            return failed("PlaneGCS exception: " + std::string(error.what()));
        }
    }

private:
    static bool finite(const Vec2& point) {
        return std::isfinite(point.x) && std::isfinite(point.y);
    }
    static bool positive(double value) { return std::isfinite(value) && value > 1e-9; }
    static double distance_squared(const Vec2& a, const Vec2& b) {
        const auto x = a.x - b.x, y = a.y - b.y;
        return x * x + y * y;
    }
    static SolveResult invalid(std::string message) {
        SolveResult result;
        result.status = SolveStatus::invalid_model;
        result.diagnostic = std::move(message);
        return result;
    }
    static SolveResult failed(std::string message) {
        SolveResult result;
        result.status = SolveStatus::failed;
        result.diagnostic = std::move(message);
        return result;
    }
    static void append_constraint_ids(const GCS::VEC_I& tags,
                                      const std::unordered_map<int, std::string>& ids,
                                      std::vector<std::string>& output) {
        for (const int tag : tags)
            if (const auto found = ids.find(tag); found != ids.end())
                output.push_back(found->second);
    }

    template <typename Constant>
    static std::string add_constraint(
        const SketchConstraint& c, int tag, const Index& point_i, const Index& line_i,
        const Index& circle_i, const Index& arc_i, const Index& spline_i,
        std::vector<PointState>& points, std::vector<LineState>& lines,
        std::vector<CircleState>& circles, std::vector<ArcState>& arcs,
        std::vector<SplineState>& splines, GCS::Point& origin, GCS::Line& x_axis, GCS::Line& y_axis,
        Constant&& constant, GCS::System& system) {
        const auto count = [&c](std::size_t expected) { return c.references.size() == expected; };
        if (c.kind == ConstraintKind::coincident) {
            if (!count(2))
                return "coincident requires two point references";
            auto* a = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                    points, lines, circles, arcs, splines, origin);
            auto* b = resolve_point(c.references[1], point_i, line_i, circle_i, arc_i, spline_i,
                                    points, lines, circles, arcs, splines, origin);
            if (!a || !b)
                return "coincident contains an invalid point reference";
            system.addConstraintP2PCoincident(*a, *b, tag);
            return {};
        }
        if (c.kind == ConstraintKind::parallel || c.kind == ConstraintKind::perpendicular ||
            c.kind == ConstraintKind::angle) {
            if (!count(2))
                return "line relationship requires two line references";
            auto* a = resolve_line(c.references[0], line_i, lines, x_axis, y_axis);
            auto* b = resolve_line(c.references[1], line_i, lines, x_axis, y_axis);
            if (!a || !b)
                return "line relationship contains an invalid line reference";
            if (c.kind == ConstraintKind::parallel)
                system.addConstraintParallel(*a, *b, tag);
            else if (c.kind == ConstraintKind::perpendicular)
                system.addConstraintPerpendicular(*a, *b, tag);
            else
                system.addConstraintL2LAngle(
                    *a, *b, constant(c.value * 3.14159265358979323846 / 180.0), tag);
            return {};
        }
        if (c.kind == ConstraintKind::horizontal || c.kind == ConstraintKind::vertical) {
            if (!count(1))
                return "horizontal/vertical requires one line";
            auto* line = resolve_line(c.references[0], line_i, lines, x_axis, y_axis);
            if (!line)
                return "horizontal/vertical has an invalid line";
            if (c.kind == ConstraintKind::horizontal)
                system.addConstraintHorizontal(*line, tag);
            else
                system.addConstraintVertical(*line, tag);
            return {};
        }
        if (c.kind == ConstraintKind::fixed || c.kind == ConstraintKind::fixed_point) {
            if (!count(1))
                return "fixed requires one reference";
            if (auto* point =
                    resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                  points, lines, circles, arcs, splines, origin)) {
                const auto value = c.kind == ConstraintKind::fixed_point
                                       ? c.fixed_point
                                       : Vec2{*point->x, *point->y};
                system.addConstraintCoordinateX(*point, constant(value.x), tag);
                system.addConstraintCoordinateY(*point, constant(value.y), tag);
                return {};
            }
            auto parameters =
                resolve_entity_parameters(c.references[0], point_i, line_i, circle_i, arc_i,
                                          spline_i, points, lines, circles, arcs, splines);
            if (parameters.empty())
                return "fixed has an invalid entity reference";
            for (auto* parameter : parameters)
                system.addConstraintEqual(parameter, constant(*parameter), tag);
            return {};
        }
        if (c.kind == ConstraintKind::distance || c.kind == ConstraintKind::length) {
            GCS::Point *a = nullptr, *b = nullptr;
            if (c.kind == ConstraintKind::distance && count(2)) {
                a = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                  points, lines, circles, arcs, splines, origin);
                b = resolve_point(c.references[1], point_i, line_i, circle_i, arc_i, spline_i,
                                  points, lines, circles, arcs, splines, origin);
                if (a && !b) {
                    if (auto* line = resolve_line(c.references[1], line_i, lines, x_axis, y_axis)) {
                        system.addConstraintP2LDistance(*a, *line, constant(c.value), tag);
                        return {};
                    }
                } else if (!a && b) {
                    if (auto* line = resolve_line(c.references[0], line_i, lines, x_axis, y_axis)) {
                        system.addConstraintP2LDistance(*b, *line, constant(c.value), tag);
                        return {};
                    }
                }
            } else if (c.kind == ConstraintKind::length && count(1)) {
                if (auto* line = resolve_line(c.references[0], line_i, lines, x_axis, y_axis)) {
                    a = &line->p1;
                    b = &line->p2;
                }
            }
            if (!a || !b)
                return "distance/length has invalid references";
            system.addConstraintP2PDistance(*a, *b, constant(c.value), tag);
            return {};
        }
        if (c.kind == ConstraintKind::radius || c.kind == ConstraintKind::diameter) {
            if (!count(1))
                return "radius/diameter requires one circle or arc";
            if (auto* circle = resolve_circle(c.references[0], circle_i, circles)) {
                if (c.kind == ConstraintKind::radius)
                    system.addConstraintCircleRadius(*circle, constant(c.value), tag);
                else
                    system.addConstraintCircleDiameter(*circle, constant(c.value), tag);
                return {};
            }
            if (auto* arc = resolve_arc(c.references[0], arc_i, arcs)) {
                if (c.kind == ConstraintKind::radius)
                    system.addConstraintArcRadius(*arc, constant(c.value), tag);
                else
                    system.addConstraintArcDiameter(*arc, constant(c.value), tag);
                return {};
            }
            return "radius/diameter has an invalid curve";
        }
        if (c.kind == ConstraintKind::concentric) {
            if (!count(2))
                return "concentric requires two circles or arcs";
            auto* a = resolve_center(c.references[0], circle_i, arc_i, circles, arcs);
            auto* b = resolve_center(c.references[1], circle_i, arc_i, circles, arcs);
            if (!a || !b)
                return "concentric has invalid references";
            system.addConstraintP2PCoincident(*a, *b, tag);
            return {};
        }
        if (c.kind == ConstraintKind::tangent || c.kind == ConstraintKind::equal) {
            if (!count(2))
                return "tangent/equal requires two curves";
            auto* la = resolve_line(c.references[0], line_i, lines, x_axis, y_axis);
            auto* lb = resolve_line(c.references[1], line_i, lines, x_axis, y_axis);
            auto* ca = resolve_circle(c.references[0], circle_i, circles);
            auto* cb = resolve_circle(c.references[1], circle_i, circles);
            auto* aa = resolve_arc(c.references[0], arc_i, arcs);
            auto* ab = resolve_arc(c.references[1], arc_i, arcs);
            if (c.kind == ConstraintKind::equal) {
                if (la && lb)
                    system.addConstraintEqualLength(*la, *lb, tag);
                else if (ca && cb)
                    system.addConstraintEqualRadius(*ca, *cb, tag);
                else if (ca && ab)
                    system.addConstraintEqualRadius(*ca, *ab, tag);
                else if (aa && cb)
                    system.addConstraintEqualRadius(*cb, *aa, tag);
                else if (aa && ab)
                    system.addConstraintEqualRadius(*aa, *ab, tag);
                else
                    return "equal requires two lines or two circular curves";
                return {};
            }
            if (la && cb)
                system.addConstraintTangent(*la, *cb, tag);
            else if (lb && ca)
                system.addConstraintTangent(*lb, *ca, tag);
            else if (la && ab)
                system.addConstraintTangent(*la, *ab, tag);
            else if (lb && aa)
                system.addConstraintTangent(*lb, *aa, tag);
            else if (ca && cb)
                system.addConstraintTangent(*ca, *cb, tag);
            else if (ca && ab)
                system.addConstraintTangent(*ca, *ab, tag);
            else if (aa && cb)
                system.addConstraintTangent(*cb, *aa, tag);
            else if (aa && ab)
                system.addConstraintTangent(*aa, *ab, tag);
            else
                return "tangent has unsupported curve references";
            return {};
        }
        if (c.kind == ConstraintKind::point_on_object) {
            if (!count(2))
                return "point-on-object requires a point and curve";
            auto* point = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                        points, lines, circles, arcs, splines, origin);
            if (!point)
                return "point-on-object first reference must be a point";
            if (auto* line = resolve_line(c.references[1], line_i, lines, x_axis, y_axis))
                system.addConstraintPointOnLine(*point, *line, tag);
            else if (auto* circle = resolve_circle(c.references[1], circle_i, circles))
                system.addConstraintPointOnCircle(*point, *circle, tag);
            else if (auto* arc = resolve_arc(c.references[1], arc_i, arcs))
                system.addConstraintPointOnArc(*point, *arc, tag);
            else
                return "point-on-object second reference must be a supported curve";
            return {};
        }
        if (c.kind == ConstraintKind::midpoint) {
            if (!count(2))
                return "midpoint requires a point and line";
            auto* point = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                        points, lines, circles, arcs, splines, origin);
            auto* line = resolve_line(c.references[1], line_i, lines, x_axis, y_axis);
            if (!point || !line)
                return "midpoint requires a point followed by a line";
            GCS::Line first_half, second_half;
            first_half.p1 = line->p1;
            first_half.p2 = *point;
            second_half.p1 = *point;
            second_half.p2 = line->p2;
            system.addConstraintPointOnLine(*point, *line, tag);
            system.addConstraintEqualLength(first_half, second_half, tag);
            return {};
        }
        if (c.kind == ConstraintKind::symmetry) {
            if (!count(3))
                return "symmetry requires point, axis-or-center, point";
            auto* first = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                        points, lines, circles, arcs, splines, origin);
            auto* second = resolve_point(c.references[2], point_i, line_i, circle_i, arc_i,
                                         spline_i, points, lines, circles, arcs, splines, origin);
            if (!first || !second)
                return "symmetry outer references must be points";
            if (auto* axis = resolve_line(c.references[1], line_i, lines, x_axis, y_axis))
                system.addConstraintP2PSymmetric(*first, *second, *axis, tag);
            else if (auto* center =
                         resolve_point(c.references[1], point_i, line_i, circle_i, arc_i, spline_i,
                                       points, lines, circles, arcs, splines, origin))
                system.addConstraintP2PSymmetric(*first, *second, *center, tag);
            else
                return "symmetry center reference must be a line or point";
            return {};
        }
        return "unsupported constraint kind";
    }

    static GCS::Point* resolve_point(const GeometryRef& r, const Index& point_i,
                                     const Index& line_i, const Index& circle_i, const Index& arc_i,
                                     const Index& spline_i, std::vector<PointState>& points,
                                     std::vector<LineState>& lines,
                                     std::vector<CircleState>& circles, std::vector<ArcState>& arcs,
                                     std::vector<SplineState>& splines, GCS::Point& origin) {
        if (r.target == GeometryTarget::sketch_origin && r.sub_element == SubElement::point)
            return &origin;
        if (r.target != GeometryTarget::entity)
            return nullptr;
        if (r.sub_element == SubElement::point) {
            const auto i = point_i.find(r.entity_id);
            return i == point_i.end() ? nullptr : &points[i->second].point;
        }
        if (const auto i = line_i.find(r.entity_id); i != line_i.end()) {
            if (r.sub_element == SubElement::start)
                return &lines[i->second].start;
            if (r.sub_element == SubElement::end)
                return &lines[i->second].end;
        }
        if (const auto i = circle_i.find(r.entity_id);
            i != circle_i.end() && r.sub_element == SubElement::center)
            return &circles[i->second].center;
        if (const auto i = arc_i.find(r.entity_id); i != arc_i.end()) {
            if (r.sub_element == SubElement::center)
                return &arcs[i->second].center;
            if (r.sub_element == SubElement::start)
                return &arcs[i->second].start;
            if (r.sub_element == SubElement::end)
                return &arcs[i->second].end;
        }
        if (const auto i = spline_i.find(r.entity_id);
            i != spline_i.end() && !splines[i->second].controls.empty()) {
            if (r.sub_element == SubElement::start)
                return &splines[i->second].controls.front();
            if (r.sub_element == SubElement::end)
                return &splines[i->second].controls.back();
        }
        return nullptr;
    }
    static GCS::Line* resolve_line(const GeometryRef& r, const Index& indices,
                                   std::vector<LineState>& lines, GCS::Line& x, GCS::Line& y) {
        if (r.target == GeometryTarget::sketch_x_axis && r.sub_element == SubElement::direction)
            return &x;
        if (r.target == GeometryTarget::sketch_y_axis && r.sub_element == SubElement::direction)
            return &y;
        if (r.target != GeometryTarget::entity ||
            (r.sub_element != SubElement::whole && r.sub_element != SubElement::direction))
            return nullptr;
        const auto i = indices.find(r.entity_id);
        return i == indices.end() ? nullptr : &lines[i->second].line;
    }
    static GCS::Circle* resolve_circle(const GeometryRef& r, const Index& indices,
                                       std::vector<CircleState>& values) {
        if (r.target != GeometryTarget::entity || r.sub_element != SubElement::whole)
            return nullptr;
        const auto i = indices.find(r.entity_id);
        return i == indices.end() ? nullptr : &values[i->second].circle;
    }
    static GCS::Arc* resolve_arc(const GeometryRef& r, const Index& indices,
                                 std::vector<ArcState>& values) {
        if (r.target != GeometryTarget::entity || r.sub_element != SubElement::whole)
            return nullptr;
        const auto i = indices.find(r.entity_id);
        return i == indices.end() ? nullptr : &values[i->second].arc;
    }
    static GCS::Point* resolve_center(const GeometryRef& r, const Index& circle_i,
                                      const Index& arc_i, std::vector<CircleState>& circles,
                                      std::vector<ArcState>& arcs) {
        if (r.target != GeometryTarget::entity)
            return nullptr;
        if (const auto i = circle_i.find(r.entity_id); i != circle_i.end())
            return &circles[i->second].center;
        if (const auto i = arc_i.find(r.entity_id); i != arc_i.end())
            return &arcs[i->second].center;
        return nullptr;
    }
    static std::vector<double*> resolve_entity_parameters(
        const GeometryRef& r, const Index& point_i, const Index& line_i, const Index& circle_i,
        const Index& arc_i, const Index& spline_i, std::vector<PointState>& points,
        std::vector<LineState>& lines, std::vector<CircleState>& circles,
        std::vector<ArcState>& arcs, std::vector<SplineState>& splines) {
        if (r.target != GeometryTarget::entity || r.sub_element != SubElement::whole)
            return {};
        if (const auto i = point_i.find(r.entity_id); i != point_i.end())
            return {points[i->second].point.x, points[i->second].point.y};
        if (const auto i = line_i.find(r.entity_id); i != line_i.end())
            return {lines[i->second].start.x, lines[i->second].start.y, lines[i->second].end.x,
                    lines[i->second].end.y};
        if (const auto i = circle_i.find(r.entity_id); i != circle_i.end())
            return {circles[i->second].center.x, circles[i->second].center.y,
                    circles[i->second].circle.rad};
        if (const auto i = arc_i.find(r.entity_id); i != arc_i.end())
            return {arcs[i->second].center.x, arcs[i->second].center.y, arcs[i->second].arc.rad,
                    arcs[i->second].arc.startAngle, arcs[i->second].arc.endAngle};
        if (const auto i = spline_i.find(r.entity_id); i != spline_i.end()) {
            std::vector<double*> out;
            for (auto& p : splines[i->second].controls) {
                out.push_back(p.x);
                out.push_back(p.y);
            }
            return out;
        }
        return {};
    }
};
}  // namespace

std::unique_ptr<SketchSolver> make_plane_gcs_sketch_solver() {
    return std::make_unique<PlaneGcsSketchSolver>();
}
}  // namespace occccad::geometry::sketch
