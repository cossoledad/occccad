#include <planegcs/GCS.h>

#include "occccad/geometry/sketch/sketch_solver.h"

#include <algorithm>
#include <cmath>
#include <cstddef>
#include <deque>
#include <exception>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <utility>
#include <vector>

#include "reflection_normal_constraint.h"

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
struct CurveParameter {
    double value;
    double* start;
    double* end;
    std::string constraint_id;
};

struct EllipseState {
    EllipseEntity entity;
    Vec2 focus;
    GCS::Ellipse ellipse;
};
struct EllipticalArcState {
    EllipticalArcEntity entity;
    Vec2 focus, start_point, end_point;
    GCS::ArcOfEllipse arc;
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
        std::vector<EllipseState> ellipses;
        std::vector<EllipticalArcState> elliptical_arcs;
        points.reserve(model.points.size());
        lines.reserve(model.lines.size());
        circles.reserve(model.circles.size());
        arcs.reserve(model.arcs.size());
        splines.reserve(model.splines.size());
        Index point_indices, line_indices, circle_indices, arc_indices, spline_indices,
            ellipse_indices, elliptical_arc_indices;
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
            const auto& editable_points =
                spline.mode == "CONTROL" ? spline.poles : spline.control_points;
            if (!unique(spline.id) || (spline.mode != "FIT" && spline.mode != "CONTROL") ||
                spline.degree < 1U || spline.degree > 25U || editable_points.size() < 2U)
                return invalid("invalid or duplicate spline: " + spline.id);
            for (const auto& point : editable_points)
                if (!finite(point))
                    return invalid("spline has non-finite control point");
            spline_indices.emplace(spline.id, splines.size());
            splines.push_back({spline, {}});
        }
        for (auto& spline : splines) {
            auto& editable_points = spline.entity.mode == "CONTROL" ? spline.entity.poles
                                                                    : spline.entity.control_points;
            spline.controls.reserve(editable_points.size());
            for (auto& point : editable_points)
                spline.controls.push_back({&point.x, &point.y});
        }
        ellipses.reserve(model.ellipses.size());
        elliptical_arcs.reserve(model.elliptical_arcs.size());
        for (const auto& ellipse : model.ellipses) {
            if (!unique(ellipse.id) || !valid_ellipse(ellipse.center, ellipse.major_radius,
                                                      ellipse.minor_radius, ellipse.rotation))
                return invalid(
                    "ellipse requires finite center/rotation and major radius > minor radius > "
                    "0: " +
                    ellipse.id);
            ellipse_indices.emplace(ellipse.id, ellipses.size());
            ellipses.push_back({ellipse, {}, {}});
        }
        for (auto& e : ellipses) {
            initialize_ellipse(e.entity, e.focus, e.ellipse);
        }
        for (const auto& arc : model.elliptical_arcs) {
            if (!unique(arc.id) ||
                !valid_ellipse(arc.center, arc.major_radius, arc.minor_radius, arc.rotation) ||
                !std::isfinite(arc.start_angle) || !std::isfinite(arc.end_angle) ||
                arc.start_angle == arc.end_angle ||
                std::abs(arc.end_angle - arc.start_angle) > 2.0 * 3.14159265358979323846)
                return invalid("invalid elliptical arc: " + arc.id);
            elliptical_arc_indices.emplace(arc.id, elliptical_arcs.size());
            elliptical_arcs.push_back({arc, {}, {}, {}, {}});
        }
        for (auto& a : elliptical_arcs) {
            initialize_ellipse(a.entity, a.focus, a.arc);
            a.start_point = ellipse_value(a.entity, a.entity.start_angle);
            a.end_point = ellipse_value(a.entity, a.entity.end_angle);
            a.arc.start = {&a.start_point.x, &a.start_point.y};
            a.arc.end = {&a.end_point.x, &a.end_point.y};
            a.arc.startAngle = &a.entity.start_angle;
            a.arc.endAngle = &a.entity.end_angle;
        }
        if (entity_ids.empty())
            return invalid("sketch must contain at least one entity");

        std::deque<double> constants;
        std::deque<GCS::Ellipse> dimension_ellipses;
        std::deque<CurveParameter> curve_parameters;
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
        std::unordered_set<std::string> whole_fixed_entities;
        std::vector<std::string> proven_redundant_ids;
        for (const auto& constraint : model.constraints)
            if (constraint.kind == ConstraintKind::fixed && constraint.references.size() == 1U &&
                constraint.references[0].target == GeometryTarget::entity &&
                constraint.references[0].sub_element == SubElement::whole)
                whole_fixed_entities.insert(constraint.references[0].entity_id);

        const auto redundancy_tolerated = [&model](const SketchConstraint& candidate) {
            if (candidate.internal || candidate.kind == ConstraintKind::symmetry ||
                candidate.kind == ConstraintKind::mirror)
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
            for (auto& arc : elliptical_arcs)
                system.addConstraintArcOfEllipseRules(arc.arc, 0, true);
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
                // Fixed geometry already determines its true endpoint positions.
                // Compiling that proven coincidence again makes native reduced
                // rank diagnosis numerically unstable during circular corner
                // updates. Preserve the formal relation and its redundant ID;
                // never infer a connection or normalize an unfixed point.
                if (constraint.kind == ConstraintKind::coincident &&
                    constraint.references.size() == 2U &&
                    whole_fixed_entities.count(constraint.references[0].entity_id) != 0U &&
                    whole_fixed_entities.count(constraint.references[1].entity_id) != 0U) {
                    auto* first = resolve_point(
                        constraint.references[0], point_indices, line_indices, circle_indices,
                        arc_indices, spline_indices, points, lines, circles, arcs, splines,
                        ellipses, elliptical_arcs, ellipse_indices, elliptical_arc_indices, origin);
                    auto* second = resolve_point(
                        constraint.references[1], point_indices, line_indices, circle_indices,
                        arc_indices, spline_indices, points, lines, circles, arcs, splines,
                        ellipses, elliptical_arcs, ellipse_indices, elliptical_arc_indices, origin);
                    if (first && second &&
                        std::hypot(*first->x - *second->x, *first->y - *second->y) <=
                            system.getFinePrecision()) {
                        if (!redundancy_tolerated(constraint))
                            proven_redundant_ids.push_back(constraint.id);
                        ++tag;
                        continue;
                    }
                }
                const auto error = add_constraint(
                    constraint, tag, point_indices, line_indices, circle_indices, arc_indices,
                    spline_indices, points, lines, circles, arcs, splines, ellipses,
                    elliptical_arcs, ellipse_indices, elliptical_arc_indices, dimension_ellipses,
                    curve_parameters, origin, x_axis, y_axis, model, constant, system);
                if (!error.empty())
                    return invalid(error);
                ++tag;
            }

            // A nonparallel directed angle and collinearity on the same
            // nondegenerate support pair are analytically incompatible. Native
            // rank diagnostics can fail numerically near a collapsed line; retain
            // this exact semantic proof instead of calling nonconvergence conflict.
            for (const auto& collinear : model.constraints) {
                if (collinear.kind != ConstraintKind::collinear ||
                    collinear.references.size() != 2U)
                    continue;
                auto* first =
                    resolve_line(collinear.references[0], line_indices, lines, x_axis, y_axis);
                auto* second =
                    resolve_line(collinear.references[1], line_indices, lines, x_axis, y_axis);
                for (const auto& angle : model.constraints) {
                    if (angle.kind != ConstraintKind::angle || angle.references.size() != 2U ||
                        !std::isfinite(angle.value) || std::remainder(angle.value, 180.0) == 0.0)
                        continue;
                    auto* a =
                        resolve_line(angle.references[0], line_indices, lines, x_axis, y_axis);
                    auto* b =
                        resolve_line(angle.references[1], line_indices, lines, x_axis, y_axis);
                    if ((a == first && b == second) || (a == second && b == first)) {
                        SolveResult result;
                        result.status = SolveStatus::conflicting;
                        result.conflicting_constraint_ids = {collinear.id, angle.id};
                        result.diagnostic =
                            "collinear relationship conflicts with nonparallel angle";
                        return result;
                    }
                }
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

            for (auto& e : ellipses)
                e.ellipse.PushOwnParams(parameters);
            for (auto& a : elliptical_arcs)
                a.arc.PushOwnParams(parameters);
            for (auto& parameter : curve_parameters)
                parameters.push_back(&parameter.value);
            int status = system.solve(parameters, true, GCS::DogLeg);
            if (status != GCS::Success)
                status = system.solve(parameters, true, GCS::LevenbergMarquardt);
            if (status != GCS::Success)
                status = system.solve(parameters, true, GCS::BFGS);
            // PlaneGCS return codes are an implementation detail.  Diagnose every
            // non-successful solve before mapping it to the platform vocabulary so
            // callers never need to know which numerical backend is in use.
            system.diagnose(GCS::DogLeg);
            GCS::VEC_I conflicting, redundant;
            system.getConflicting(conflicting);
            system.getRedundant(redundant);
            GCS::VEC_I user_redundant;
            for (const int redundant_tag : redundant) {
                if (redundant_tag != 0 && !constraint_redundancy_tolerated[redundant_tag])
                    user_redundant.push_back(redundant_tag);
            }
            if (!conflicting.empty()) {
                SolveResult result;
                result.status = SolveStatus::conflicting;
                result.degrees_of_freedom = system.dofsNumber();
                append_constraint_ids(conflicting, constraint_ids,
                                      result.conflicting_constraint_ids);
                append_constraint_ids(user_redundant, constraint_ids,
                                      result.redundant_constraint_ids);
                result.redundant_constraint_ids.insert(result.redundant_constraint_ids.end(),
                                                       proven_redundant_ids.begin(),
                                                       proven_redundant_ids.end());
                result.diagnostic = "conflicting constraints";
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
                append_ellipses(result, ellipses, elliptical_arcs);
                return result;
            }
            if (status != GCS::Success) {
                auto result = failed("constraint solver could not classify the model");
                append_constraint_ids(user_redundant, constraint_ids,
                                      result.redundant_constraint_ids);
                result.redundant_constraint_ids.insert(result.redundant_constraint_ids.end(),
                                                       proven_redundant_ids.begin(),
                                                       proven_redundant_ids.end());
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
                append_ellipses(result, ellipses, elliptical_arcs);
                return result;
            }
            // diagnose() temporarily solves reduced systems and restores the
            // original parameter references. Apply the authoritative full-system
            // solution afterwards; applying before diagnose loses solved values
            // whenever a disconnected component contains redundant constraints.
            system.applySolution();
            for (const auto& parameter : curve_parameters) {
                const double lo = std::min(*parameter.start, *parameter.end);
                const double hi = std::max(*parameter.start, *parameter.end);
                if (parameter.value < lo - 1e-10 || parameter.value > hi + 1e-10)
                    return failed("point-on-object falls outside elliptical arc: " +
                                  parameter.constraint_id);
            }
            for (const auto& constraint : model.constraints) {
                if (constraint.kind != ConstraintKind::tangent ||
                    constraint.references.size() != 2U)
                    continue;
                for (std::size_t k = 0; k < 2U; ++k) {
                    if (const auto spline = spline_indices.find(constraint.references[k].entity_id);
                        spline != spline_indices.end()) {
                        const auto& controls = splines[spline->second].controls;
                        const auto first = constraint.references[k].sub_element == SubElement::start
                                               ? 0U
                                               : controls.size() - 2U;
                        const double dx = *controls[first + 1U].x - *controls[first].x;
                        const double dy = *controls[first + 1U].y - *controls[first].y;
                        if (!std::isfinite(dx) || !std::isfinite(dy) || (dx == 0.0 && dy == 0.0))
                            return failed("spline endpoint tangent reached singular derivative: " +
                                          constraint.id);
                    }
                    const auto i = elliptical_arc_indices.find(constraint.references[k].entity_id);
                    if (i == elliptical_arc_indices.end())
                        continue;
                    auto* line = resolve_line(constraint.references[1U - k], line_indices, lines,
                                              x_axis, y_axis);
                    if (!line)
                        continue;
                    auto& a = elliptical_arcs[i->second];
                    update_ellipse(a.entity, a.arc);
                    const double nx = *line->p1.y - *line->p2.y;
                    const double ny = *line->p2.x - *line->p1.x;
                    const double cr = std::cos(a.entity.rotation), sr = std::sin(a.entity.rotation);
                    const double local_nx = nx * cr + ny * sr;
                    const double local_ny = -nx * sr + ny * cr;
                    const double side = nx * (*line->p1.x - a.entity.center.x) +
                                        ny * (*line->p1.y - a.entity.center.y);
                    const double sign = side < 0.0 ? -1.0 : 1.0;
                    const double u = std::atan2(sign * a.entity.minor_radius * local_ny,
                                                sign * a.entity.major_radius * local_nx);
                    if (!arc_contains(a.entity, u))
                        return failed("tangent contact falls outside elliptical arc: " +
                                      constraint.id);
                }
            }
            SolveResult result;
            result.degrees_of_freedom = system.dofsNumber();
            result.status = result.degrees_of_freedom > 0 ? SolveStatus::under_constrained
                                                          : SolveStatus::solved;
            append_constraint_ids(user_redundant, constraint_ids, result.redundant_constraint_ids);
            result.redundant_constraint_ids.insert(result.redundant_constraint_ids.end(),
                                                   proven_redundant_ids.begin(),
                                                   proven_redundant_ids.end());
            if (!result.redundant_constraint_ids.empty()) {
                result.status = SolveStatus::redundant;
                result.diagnostic = "redundant constraints";
            }
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
            append_ellipses(result, ellipses, elliptical_arcs);
            return result;
        } catch (const std::exception& error) {
            return failed("constraint solver exception: " + std::string(error.what()));
        }
    }

private:
    static bool arc_contains(const EllipticalArcEntity& arc, double u) {
        const double lo = std::min(arc.start_angle, arc.end_angle);
        const double hi = std::max(arc.start_angle, arc.end_angle);
        const double period = 2.0 * 3.14159265358979323846;
        const double mapped = u + std::round(((lo + hi) * 0.5 - u) / period) * period;
        return mapped >= lo - 1e-10 && mapped <= hi + 1e-10;
    }
    static bool valid_ellipse(const Vec2& center, double major, double minor, double rotation) {
        return finite(center) && std::isfinite(major) && std::isfinite(minor) &&
               std::isfinite(rotation) && major > minor && minor > 0.0;
    }
    template <typename Entity>
    static void initialize_ellipse(Entity& entity, Vec2& focus, GCS::Ellipse& ellipse) {
        const double focal = std::sqrt((entity.major_radius - entity.minor_radius) *
                                       (entity.major_radius + entity.minor_radius));
        focus = {entity.center.x + focal * std::cos(entity.rotation),
                 entity.center.y + focal * std::sin(entity.rotation)};
        ellipse.center = {&entity.center.x, &entity.center.y};
        ellipse.focus1 = {&focus.x, &focus.y};
        ellipse.radmin = &entity.minor_radius;
    }
    template <typename Entity>
    static Vec2 ellipse_value(const Entity& e, double u) {
        const double x = e.major_radius * std::cos(u), y = e.minor_radius * std::sin(u);
        return {e.center.x + x * std::cos(e.rotation) - y * std::sin(e.rotation),
                e.center.y + x * std::sin(e.rotation) + y * std::cos(e.rotation)};
    }
    template <typename Entity>
    static void update_ellipse(Entity& entity, GCS::Ellipse& ellipse) {
        entity.major_radius = ellipse.getRadMaj();
        const double rotation = std::atan2(*ellipse.focus1.y - *ellipse.center.y,
                                           *ellipse.focus1.x - *ellipse.center.x);
        entity.rotation += std::remainder(rotation - entity.rotation, 2.0 * 3.14159265358979323846);
    }
    static void append_ellipses(SolveResult& result, std::vector<EllipseState>& ellipses,
                                std::vector<EllipticalArcState>& arcs) {
        for (auto& e : ellipses) {
            update_ellipse(e.entity, e.ellipse);
            if (!valid_ellipse(e.entity.center, e.entity.major_radius, e.entity.minor_radius,
                               e.entity.rotation)) {
                result.status = SolveStatus::failed;
                result.diagnostic = "solver reached degenerate ellipse: " + e.entity.id;
            }
            result.ellipses.push_back(e.entity);
        }
        for (auto& a : arcs) {
            update_ellipse(a.entity, a.arc);
            if (!valid_ellipse(a.entity.center, a.entity.major_radius, a.entity.minor_radius,
                               a.entity.rotation)) {
                result.status = SolveStatus::failed;
                result.diagnostic = "solver reached degenerate elliptical arc: " + a.entity.id;
            }
            result.elliptical_arcs.push_back(a.entity);
        }
    }
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
        std::vector<SplineState>& splines, std::vector<EllipseState>& ellipses,
        std::vector<EllipticalArcState>& elliptical_arcs, const Index& ellipse_i,
        const Index& elliptical_arc_i, std::deque<GCS::Ellipse>& dimension_ellipses,
        std::deque<CurveParameter>& curve_parameters, GCS::Point& origin, GCS::Line& x_axis,
        GCS::Line& y_axis, const SketchModel& model, Constant&& constant, GCS::System& system) {
        const auto count = [&c](std::size_t expected) { return c.references.size() == expected; };
        if (c.kind == ConstraintKind::same_support) {
            if (!count(2) || c.references[0].target != GeometryTarget::entity ||
                c.references[1].target != GeometryTarget::entity ||
                c.references[0].sub_element != SubElement::whole ||
                c.references[1].sub_element != SubElement::whole ||
                c.references[0].entity_id == c.references[1].entity_id)
                return "same-support requires two distinct whole local curves";
            auto* first_line = resolve_line(c.references[0], line_i, lines, x_axis, y_axis);
            auto* second_line = resolve_line(c.references[1], line_i, lines, x_axis, y_axis);
            if (first_line && second_line) {
                if (!has_parallel_relationship(model, first_line, second_line, line_i, lines,
                                               x_axis, y_axis))
                    system.addConstraintParallel(*first_line, *second_line, tag);
                system.addConstraintPointOnLine(second_line->p1, *first_line, tag);
                return {};
            }
            GCS::Circle* first_circle = resolve_circle(c.references[0], circle_i, circles);
            GCS::Circle* second_circle = resolve_circle(c.references[1], circle_i, circles);
            if (!first_circle)
                first_circle = resolve_arc(c.references[0], arc_i, arcs);
            if (!second_circle)
                second_circle = resolve_arc(c.references[1], arc_i, arcs);
            if (first_circle && second_circle) {
                system.addConstraintP2PCoincident(first_circle->center, second_circle->center, tag);
                system.addConstraintEqual(first_circle->rad, second_circle->rad, tag);
                return {};
            }
            auto* first_ellipse = resolve_ellipse(c.references[0], ellipse_i, elliptical_arc_i,
                                                  ellipses, elliptical_arcs);
            auto* second_ellipse = resolve_ellipse(c.references[1], ellipse_i, elliptical_arc_i,
                                                   ellipses, elliptical_arcs);
            if (first_ellipse && second_ellipse) {
                system.addConstraintP2PCoincident(first_ellipse->center, second_ellipse->center,
                                                  tag);
                system.addConstraintP2PCoincident(first_ellipse->focus1, second_ellipse->focus1,
                                                  tag);
                system.addConstraintEqual(first_ellipse->radmin, second_ellipse->radmin, tag);
                return {};
            }
            return "same-support requires matching linear, circular or elliptical supports";
        }
        if (c.kind == ConstraintKind::mirror) {
            if (!count(3) || c.references[0].target != GeometryTarget::entity ||
                c.references[2].target != GeometryTarget::entity ||
                c.references[0].sub_element != SubElement::whole ||
                c.references[2].sub_element != SubElement::whole)
                return "mirror requires whole source/target and a stable line axis";
            auto* axis = resolve_line(c.references[1], line_i, lines, x_axis, y_axis);
            if (!axis)
                return "mirror axis must be a line or intrinsic axis";
            const auto& source = c.references[0].entity_id;
            const auto& target = c.references[2].entity_id;
            const auto symmetric = [&](GCS::Point& a, GCS::Point& b) {
                auto* normal = new ReflectionNormalConstraint(a, b, *axis);
                normal->setTag(tag);
                normal->setDriving(true);
                system.addConstraint(normal);
                system.addConstraintMidpointOnLine(a, b, axis->p1, axis->p2, tag);
            };
            if (source == target) {
                const auto on_axis = [&](GCS::Point& point) {
                    system.addConstraintPointOnLine(point, *axis, tag);
                };
                const auto ellipse_self = [&](GCS::Ellipse& ellipse) -> bool {
                    GCS::Line major;
                    major.p1 = ellipse.center;
                    major.p2 = ellipse.focus1;
                    if (c.self_mirror_mode == "MAJOR_PARALLEL")
                        system.addConstraintParallel(major, *axis, tag);
                    else if (c.self_mirror_mode == "MAJOR_PERPENDICULAR")
                        system.addConstraintPerpendicular(major, *axis, tag);
                    else
                        return false;
                    on_axis(ellipse.center);
                    return true;
                };
                if (const auto i = point_i.find(source); i != point_i.end()) {
                    on_axis(points[i->second].point);
                    return {};
                }
                if (const auto i = circle_i.find(source); i != circle_i.end()) {
                    on_axis(circles[i->second].center);
                    return {};
                }
                if (const auto i = line_i.find(source); i != line_i.end()) {
                    auto& line = lines[i->second];
                    if (c.self_mirror_mode == "ON_AXIS") {
                        on_axis(line.start);
                        on_axis(line.end);
                        return {};
                    }
                    if (c.self_mirror_mode == "PAIRED") {
                        symmetric(line.start, line.end);
                        return {};
                    }
                    return "self mirror line requires ON_AXIS or PAIRED intent";
                }
                if (const auto i = arc_i.find(source); i != arc_i.end()) {
                    auto& arc = arcs[i->second];
                    on_axis(arc.center);
                    symmetric(arc.start, arc.end);
                    return {};
                }
                if (const auto i = ellipse_i.find(source); i != ellipse_i.end())
                    return ellipse_self(ellipses[i->second].ellipse)
                               ? std::string{}
                               : "self mirror ellipse requires explicit major-axis orientation";
                if (const auto i = elliptical_arc_i.find(source); i != elliptical_arc_i.end()) {
                    auto& arc = elliptical_arcs[i->second].arc;
                    if (!ellipse_self(arc))
                        return "self mirror elliptical arc requires explicit major-axis "
                               "orientation";
                    symmetric(arc.start, arc.end);
                    return {};
                }
                if (const auto i = spline_i.find(source); i != spline_i.end()) {
                    auto& spline = splines[i->second];
                    const auto& data = spline.entity;
                    if (c.self_mirror_mode == "ON_AXIS") {
                        for (auto& point : spline.controls)
                            on_axis(point);
                        return {};
                    }
                    if (c.self_mirror_mode != "PAIRED")
                        return "self mirror spline requires ON_AXIS or PAIRED intent";
                    if (data.mode == "CONTROL") {
                        if (data.periodic || data.knots.size() < 2U ||
                            data.knots.size() != data.multiplicities.size())
                            return "self mirror CONTROL spline requires nonperiodic symmetric "
                                   "basis";
                        const double sum = data.knots.front() + data.knots.back();
                        for (std::size_t k = 0; k < data.knots.size(); ++k) {
                            const auto other = data.knots.size() - 1U - k;
                            if (std::abs(data.knots[k] + data.knots[other] - sum) >
                                    1e-12 * std::max(1.0, std::abs(sum)) ||
                                data.multiplicities[k] != data.multiplicities[other])
                                return "self mirror CONTROL spline knots must have symmetric "
                                       "reverse basis";
                        }
                        if (!data.weights.empty())
                            for (std::size_t k = 0; k < data.weights.size(); ++k)
                                if (data.weights[k] != data.weights[data.weights.size() - 1U - k])
                                    return "self mirror rational spline weights must be symmetric";
                        if (data.parameter_start != data.parameter_end &&
                            data.parameter_start + data.parameter_end != sum)
                            return "self mirror spline domain must be symmetric";
                    }
                    for (std::size_t k = 0; k < spline.controls.size(); ++k) {
                        const auto other =
                            data.closed && data.mode != "CONTROL"
                                ? (spline.controls.size() - k) % spline.controls.size()
                                : spline.controls.size() - 1U - k;
                        if (k > other)
                            continue;
                        if (k == other)
                            on_axis(spline.controls[k]);
                        else
                            symmetric(spline.controls[k], spline.controls[other]);
                    }
                    return {};
                }
                return "self mirror has unsupported geometry";
            }
            if (const auto a = point_i.find(source), b = point_i.find(target);
                a != point_i.end() && b != point_i.end()) {
                symmetric(points[a->second].point, points[b->second].point);
                return {};
            }
            if (const auto a = line_i.find(source), b = line_i.find(target);
                a != line_i.end() && b != line_i.end()) {
                symmetric(lines[a->second].start, lines[b->second].start);
                symmetric(lines[a->second].end, lines[b->second].end);
                return {};
            }
            if (const auto a = circle_i.find(source), b = circle_i.find(target);
                a != circle_i.end() && b != circle_i.end()) {
                symmetric(circles[a->second].center, circles[b->second].center);
                system.addConstraintEqual(circles[a->second].circle.rad,
                                          circles[b->second].circle.rad, tag);
                return {};
            }
            if (const auto a = arc_i.find(source), b = arc_i.find(target);
                a != arc_i.end() && b != arc_i.end()) {
                auto& first = arcs[a->second];
                auto& second = arcs[b->second];
                symmetric(first.center, second.center);
                system.addConstraintEqual(first.arc.rad, second.arc.rad, tag);
                symmetric(first.start, second.start);
                symmetric(first.end, second.end);
                return {};
            }
            if (const auto a = ellipse_i.find(source), b = ellipse_i.find(target);
                a != ellipse_i.end() && b != ellipse_i.end()) {
                auto& first = ellipses[a->second].ellipse;
                auto& second = ellipses[b->second].ellipse;
                symmetric(first.center, second.center);
                symmetric(first.focus1, second.focus1);
                system.addConstraintEqual(first.radmin, second.radmin, tag);
                return {};
            }
            if (const auto a = elliptical_arc_i.find(source), b = elliptical_arc_i.find(target);
                a != elliptical_arc_i.end() && b != elliptical_arc_i.end()) {
                auto& first = elliptical_arcs[a->second].arc;
                auto& second = elliptical_arcs[b->second].arc;
                symmetric(first.center, second.center);
                symmetric(first.focus1, second.focus1);
                system.addConstraintEqual(first.radmin, second.radmin, tag);
                symmetric(first.start, second.start);
                symmetric(first.end, second.end);
                return {};
            }
            if (const auto a = spline_i.find(source), b = spline_i.find(target);
                a != spline_i.end() && b != spline_i.end()) {
                auto& first = splines[a->second];
                auto& second = splines[b->second];
                if (first.entity.mode != second.entity.mode ||
                    first.controls.size() != second.controls.size() ||
                    first.entity.closed != second.entity.closed)
                    return "mirror splines require matching mode, point count and open/closed "
                           "state";
                if (first.entity.mode == "CONTROL" &&
                    (first.entity.degree != second.entity.degree ||
                     first.entity.knots != second.entity.knots ||
                     first.entity.multiplicities != second.entity.multiplicities ||
                     first.entity.weights != second.entity.weights ||
                     first.entity.periodic != second.entity.periodic ||
                     first.entity.parameter_start != second.entity.parameter_start ||
                     first.entity.parameter_end != second.entity.parameter_end))
                    return "mirror CONTROL splines require identical exact basis and parameter "
                           "domain";
                for (std::size_t i = 0; i < first.controls.size(); ++i)
                    symmetric(first.controls[i], second.controls[i]);
                return {};
            }
            return "mirror source/target must have matching supported geometry types";
        }
        if (c.kind == ConstraintKind::horizontal_distance ||
            c.kind == ConstraintKind::vertical_distance) {
            if (!count(2) || !std::isfinite(c.value))
                return "projected distance requires two point references and finite signed value";
            auto* first = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                        points, lines, circles, arcs, splines, ellipses,
                                        elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
            auto* second = resolve_point(c.references[1], point_i, line_i, circle_i, arc_i,
                                         spline_i, points, lines, circles, arcs, splines, ellipses,
                                         elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
            if (!first || !second || first == second)
                return "projected distance requires distinct valid points";
            system.addConstraintDifference(
                c.kind == ConstraintKind::horizontal_distance ? first->x : first->y,
                c.kind == ConstraintKind::horizontal_distance ? second->x : second->y,
                constant(c.value), tag);
            return {};
        }
        if (c.kind == ConstraintKind::collinear) {
            if (!count(2))
                return "collinear requires two lines";
            auto* first = resolve_line(c.references[0], line_i, lines, x_axis, y_axis);
            auto* second = resolve_line(c.references[1], line_i, lines, x_axis, y_axis);
            if (!first || !second || first == second)
                return "collinear requires distinct valid lines";
            if (!has_parallel_relationship(model, first, second, line_i, lines, x_axis, y_axis))
                system.addConstraintParallel(*first, *second, tag);
            if (second == &x_axis || second == &y_axis)
                std::swap(first, second);
            system.addConstraintPointOnLine(second->p1, *first, tag);
            return {};
        }
        if (c.kind == ConstraintKind::coincident) {
            if (!count(2))
                return "coincident requires two point references";
            auto* a = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                    points, lines, circles, arcs, splines, ellipses,
                                    elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
            auto* b = resolve_point(c.references[1], point_i, line_i, circle_i, arc_i, spline_i,
                                    points, lines, circles, arcs, splines, ellipses,
                                    elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
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
            else if (!std::isfinite(c.value))
                return "angle requires finite value";
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
                                  points, lines, circles, arcs, splines, ellipses, elliptical_arcs,
                                  ellipse_i, elliptical_arc_i, origin)) {
                const auto value = c.kind == ConstraintKind::fixed_point
                                       ? c.fixed_point
                                       : Vec2{*point->x, *point->y};
                system.addConstraintCoordinateX(*point, constant(value.x), tag);
                system.addConstraintCoordinateY(*point, constant(value.y), tag);
                return {};
            }
            auto parameters = resolve_entity_parameters(
                c.references[0], point_i, line_i, circle_i, arc_i, spline_i, points, lines, circles,
                arcs, splines, ellipses, elliptical_arcs, ellipse_i, elliptical_arc_i);
            if (parameters.empty())
                return "fixed has an invalid entity reference";
            for (auto* parameter : parameters)
                system.addConstraintEqual(parameter, constant(*parameter), tag);
            return {};
        }
        if (c.kind == ConstraintKind::distance || c.kind == ConstraintKind::length) {
            if (!std::isfinite(c.value) || c.value < 0.0)
                return "distance/length must be finite and nonnegative";
            GCS::Point *a = nullptr, *b = nullptr;
            if (c.kind == ConstraintKind::distance && count(2)) {
                auto* first_line = resolve_line(c.references[0], line_i, lines, x_axis, y_axis);
                auto* second_line = resolve_line(c.references[1], line_i, lines, x_axis, y_axis);
                if (first_line && second_line) {
                    if (first_line == second_line)
                        return "parallel line distance requires distinct lines";
                    // The control plane owns the visible composite relationship.
                    // Do not silently add parallelism or fix either selected line.
                    const bool explicitly_parallel = has_parallel_relationship(
                        model, first_line, second_line, line_i, lines, x_axis, y_axis);
                    if (!explicitly_parallel)
                        return "parallel line distance requires an explicit parallel relationship";
                    // Use the immutable axis as the reference support when selected
                    // second; moving a point towards that line has a linear local
                    // chart and avoids rotating a moving line around a fixed point.
                    if (second_line == &x_axis || second_line == &y_axis)
                        std::swap(first_line, second_line);
                    // Supporting-line spacing is unsigned and invariant under
                    // endpoint reversal. At zero use the smooth incidence equation.
                    if (c.value == 0.0)
                        system.addConstraintPointOnLine(second_line->p1, *first_line, tag);
                    else
                        system.addConstraintP2LDistance(second_line->p1, *first_line,
                                                        constant(c.value), tag);
                    return {};
                }
                a = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                  points, lines, circles, arcs, splines, ellipses, elliptical_arcs,
                                  ellipse_i, elliptical_arc_i, origin);
                b = resolve_point(c.references[1], point_i, line_i, circle_i, arc_i, spline_i,
                                  points, lines, circles, arcs, splines, ellipses, elliptical_arcs,
                                  ellipse_i, elliptical_arc_i, origin);
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
        if (c.kind == ConstraintKind::major_radius || c.kind == ConstraintKind::minor_radius) {
            if (!count(1) || !std::isfinite(c.value) || c.value <= 0.0)
                return "ellipse axis radius requires one ellipse and positive finite value";
            auto* ellipse = resolve_ellipse(c.references[0], ellipse_i, elliptical_arc_i, ellipses,
                                            elliptical_arcs);
            if (!ellipse)
                return "ellipse axis radius has invalid references";
            if (c.kind == ConstraintKind::minor_radius) {
                system.addConstraintEqual(ellipse->radmin, constant(c.value), tag);
            } else {
                // Native conic major-axis equation, against an immutable reference
                // ellipse. Its parameters are constants, never solver unknowns.
                dimension_ellipses.emplace_back();
                auto& reference = dimension_ellipses.back();
                reference.center = {constant(0.0), constant(0.0)};
                reference.focus1 = {constant(c.value * std::sqrt(0.75)), constant(0.0)};
                reference.radmin = constant(c.value * 0.5);
                auto* constraint = new GCS::ConstraintEqualMajorAxesConic(ellipse, &reference);
                constraint->setTag(tag);
                system.addConstraint(constraint);
            }
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
            auto* a = resolve_center(c.references[0], circle_i, arc_i, circles, arcs, ellipses,
                                     elliptical_arcs, ellipse_i, elliptical_arc_i);
            auto* b = resolve_center(c.references[1], circle_i, arc_i, circles, arcs, ellipses,
                                     elliptical_arcs, ellipse_i, elliptical_arc_i);
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
            auto* ea = resolve_ellipse(c.references[0], ellipse_i, elliptical_arc_i, ellipses,
                                       elliptical_arcs);
            auto* eb = resolve_ellipse(c.references[1], ellipse_i, elliptical_arc_i, ellipses,
                                       elliptical_arcs);
            if (c.kind == ConstraintKind::tangent) {
                for (std::size_t k = 0; k < 2U; ++k) {
                    const auto i = elliptical_arc_i.find(c.references[k].entity_id);
                    const auto sub = c.references[k].sub_element;
                    if (i == elliptical_arc_i.end() ||
                        (sub != SubElement::start && sub != SubElement::end))
                        continue;
                    auto* line = k == 0U ? lb : la;
                    if (!line)
                        return "elliptical arc endpoint tangent requires a line";
                    auto& arc = elliptical_arcs[i->second].arc;
                    auto& point = sub == SubElement::start ? arc.start : arc.end;
                    const double angle = system.calculateAngleViaPoint(arc, *line, point);
                    bool already_connected = false;
                    for (const auto& relation : model.constraints)
                        if (relation.kind == ConstraintKind::coincident &&
                            relation.references.size() == 2U) {
                            for (std::size_t side = 0; side < 2U; ++side) {
                                const auto& a = relation.references[side];
                                const auto& b = relation.references[1U - side];
                                if (a.entity_id == c.references[k].entity_id &&
                                    a.sub_element == sub &&
                                    b.entity_id == c.references[1U - k].entity_id &&
                                    (b.sub_element == SubElement::start ||
                                     b.sub_element == SubElement::end))
                                    already_connected = true;
                            }
                        }
                    if (!already_connected)
                        system.addConstraintPointOnLine(point, *line, tag);
                    system.addConstraintAngleViaPoint(
                        arc, *line, point,
                        constant(std::abs(std::remainder(angle, 2 * std::acos(-1.0))) <
                                         std::acos(-1.0) / 2
                                     ? 0
                                     : std::acos(-1.0)),
                        tag);
                    return {};
                }
                for (std::size_t k = 0; k < 2U; ++k) {
                    const auto i = arc_i.find(c.references[k].entity_id);
                    const auto sub = c.references[k].sub_element;
                    if (i == arc_i.end() || (sub != SubElement::start && sub != SubElement::end))
                        continue;
                    auto& arc = arcs[i->second];
                    auto& point = sub == SubElement::start ? arc.start : arc.end;
                    if (auto* line = k == 0U ? lb : la) {
                        bool already_connected = false;
                        for (const auto& relation : model.constraints)
                            if (relation.kind == ConstraintKind::coincident &&
                                relation.references.size() == 2U)
                                for (std::size_t side = 0; side < 2U; ++side) {
                                    const auto& a = relation.references[side];
                                    const auto& b = relation.references[1U - side];
                                    if (a.entity_id == c.references[k].entity_id &&
                                        a.sub_element == sub &&
                                        b.entity_id == c.references[1U - k].entity_id &&
                                        (b.sub_element == SubElement::start ||
                                         b.sub_element == SubElement::end))
                                        already_connected = true;
                                }
                        if (!already_connected)
                            system.addConstraintPointOnLine(point, *line, tag);
                        system.addConstraintPerpendicular(line->p1, line->p2, arc.center, point,
                                                          tag);
                        return {};
                    }
                    GCS::Circle* circle = k == 0U ? cb : ca;
                    if (!circle)
                        circle = k == 0U ? ab : aa;
                    if (circle) {
                        system.addConstraintPointOnCircle(point, *circle, tag);
                        GCS::Line source_radius, target_radius;
                        source_radius.p1 = circle->center;
                        source_radius.p2 = point;
                        target_radius.p1 = arc.center;
                        target_radius.p2 = point;
                        system.addConstraintParallel(source_radius, target_radius, tag);
                        return {};
                    }
                    return "arc endpoint tangent requires line or circular support";
                }

                for (std::size_t k = 0; k < 2U; ++k) {
                    const auto i = spline_i.find(c.references[k].entity_id);
                    if (i == spline_i.end())
                        continue;
                    auto* line = k == 0U ? lb : la;
                    if (!line)
                        return "spline endpoint tangent requires a line";
                    auto& spline = splines[i->second];
                    const auto& data = spline.entity;
                    const auto sub = c.references[k].sub_element;
                    if (data.mode != "CONTROL" || data.periodic || data.closed ||
                        (sub != SubElement::start && sub != SubElement::end) ||
                        data.knots.size() < 2U || data.multiplicities.size() != data.knots.size() ||
                        data.multiplicities.front() != data.degree + 1U ||
                        data.multiplicities.back() != data.degree + 1U ||
                        (data.parameter_start != data.parameter_end &&
                         (data.parameter_start != data.knots.front() ||
                          data.parameter_end != data.knots.back())))
                        return "spline tangent requires explicit endpoint of open clamped CONTROL "
                               "curve; convert FIT explicitly";
                    GCS::Line derivative;
                    const std::size_t first =
                        sub == SubElement::start ? 0U : spline.controls.size() - 2U;
                    derivative.p1 = spline.controls[first];
                    derivative.p2 = spline.controls[first + 1U];
                    if (*derivative.p1.x == *derivative.p2.x &&
                        *derivative.p1.y == *derivative.p2.y)
                        return "spline endpoint derivative is singular";
                    // For a clamped rational B-Spline, endpoint D1 is a
                    // positive scalar multiple of this first/last pole chord.
                    // This is an exact endpoint relation, not sampled tangency.
                    system.addConstraintParallel(derivative, *line, tag);
                    return {};
                }
            }
            if (c.kind == ConstraintKind::equal) {
                if (ea && eb)
                    system.addConstraintEqualRadii(*ea, *eb, tag);
                else if (la && lb)
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
            if (la && eb)
                system.addConstraintTangent(*la, *eb, tag);
            else if (lb && ea)
                system.addConstraintTangent(*lb, *ea, tag);
            else if (la && cb)
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
                                        points, lines, circles, arcs, splines, ellipses,
                                        elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
            if (!point)
                return "point-on-object first reference must be a point";
            if (auto* line = resolve_line(c.references[1], line_i, lines, x_axis, y_axis))
                system.addConstraintPointOnLine(*point, *line, tag);
            else if (auto* circle = resolve_circle(c.references[1], circle_i, circles))
                system.addConstraintPointOnCircle(*point, *circle, tag);
            else if (auto* arc = resolve_arc(c.references[1], arc_i, arcs))
                system.addConstraintPointOnArc(*point, *arc, tag);
            else if (const auto i = elliptical_arc_i.find(c.references[1].entity_id);
                     c.references[1].target == GeometryTarget::entity &&
                     c.references[1].sub_element == SubElement::whole &&
                     i != elliptical_arc_i.end()) {
                auto& ellipse_arc = elliptical_arcs[i->second];
                const double x = *point->x - ellipse_arc.entity.center.x,
                             y = *point->y - ellipse_arc.entity.center.y;
                const double cr = std::cos(ellipse_arc.entity.rotation),
                             sr = std::sin(ellipse_arc.entity.rotation);
                const double u = std::atan2((-x * sr + y * cr) / ellipse_arc.entity.minor_radius,
                                            (x * cr + y * sr) / ellipse_arc.entity.major_radius);
                const double lo =
                    std::min(ellipse_arc.entity.start_angle, ellipse_arc.entity.end_angle);
                const double hi =
                    std::max(ellipse_arc.entity.start_angle, ellipse_arc.entity.end_angle);
                const double seed =
                    u + std::round(((lo + hi) * 0.5 - u) / (2.0 * 3.14159265358979323846)) *
                            (2.0 * 3.14159265358979323846);
                curve_parameters.push_back({std::max(lo, std::min(hi, seed)),
                                            ellipse_arc.arc.startAngle, ellipse_arc.arc.endAngle,
                                            c.id});
                system.addConstraintCurveValue(*point, ellipse_arc.arc,
                                               &curve_parameters.back().value, tag);
            } else if (auto* ellipse = resolve_ellipse(c.references[1], ellipse_i, elliptical_arc_i,
                                                       ellipses, elliptical_arcs))
                system.addConstraintPointOnEllipse(*point, *ellipse, tag);
            else
                return "point-on-object second reference must be a supported curve";
            return {};
        }
        if (c.kind == ConstraintKind::midpoint) {
            if (!count(2))
                return "midpoint requires a point and line";
            auto* point = resolve_point(c.references[0], point_i, line_i, circle_i, arc_i, spline_i,
                                        points, lines, circles, arcs, splines, ellipses,
                                        elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
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
                                        points, lines, circles, arcs, splines, ellipses,
                                        elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
            auto* second = resolve_point(c.references[2], point_i, line_i, circle_i, arc_i,
                                         spline_i, points, lines, circles, arcs, splines, ellipses,
                                         elliptical_arcs, ellipse_i, elliptical_arc_i, origin);
            if (!first || !second)
                return "symmetry outer references must be points";
            if (auto* axis = resolve_line(c.references[1], line_i, lines, x_axis, y_axis)) {
                auto* normal = new ReflectionNormalConstraint(*first, *second, *axis);
                normal->setTag(tag);
                normal->setDriving(true);
                system.addConstraint(normal);
                system.addConstraintMidpointOnLine(*first, *second, axis->p1, axis->p2, tag);
            } else if (auto* center =
                           resolve_point(c.references[1], point_i, line_i, circle_i, arc_i,
                                         spline_i, points, lines, circles, arcs, splines, ellipses,
                                         elliptical_arcs, ellipse_i, elliptical_arc_i, origin)) {
                for (const auto coordinates :
                     {std::vector<double*>{center->x, first->x, second->x},
                      std::vector<double*>{center->y, first->y, second->y}}) {
                    auto* relation = new GCS::ConstraintCenterOfGravity(coordinates, {0.5, 0.5});
                    relation->setTag(tag);
                    relation->setDriving(true);
                    system.addConstraint(relation);
                }
            } else
                return "symmetry center reference must be a line or point";
            return {};
        }
        return "unsupported constraint kind";
    }

    static GCS::Point* resolve_point(
        const GeometryRef& r, const Index& point_i, const Index& line_i, const Index& circle_i,
        const Index& arc_i, const Index& spline_i, std::vector<PointState>& points,
        std::vector<LineState>& lines, std::vector<CircleState>& circles,
        std::vector<ArcState>& arcs, std::vector<SplineState>& splines,
        std::vector<EllipseState>& ellipses, std::vector<EllipticalArcState>& elliptical_arcs,
        const Index& ellipse_i, const Index& elliptical_arc_i, GCS::Point& origin) {
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
        if (const auto i = ellipse_i.find(r.entity_id);
            i != ellipse_i.end() && r.sub_element == SubElement::center)
            return &ellipses[i->second].ellipse.center;
        if (const auto i = elliptical_arc_i.find(r.entity_id); i != elliptical_arc_i.end()) {
            auto& arc = elliptical_arcs[i->second].arc;
            if (r.sub_element == SubElement::center)
                return &arc.center;
            if (r.sub_element == SubElement::start)
                return &arc.start;
            if (r.sub_element == SubElement::end)
                return &arc.end;
        }
        if (const auto i = spline_i.find(r.entity_id);
            i != spline_i.end() && !splines[i->second].controls.empty()) {
            if (r.sub_element == SubElement::control &&
                r.control_point_index < splines[i->second].controls.size())
                return &splines[i->second].controls[r.control_point_index];
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
    static bool has_parallel_relationship(const SketchModel& model, GCS::Line* first_line,
                                          GCS::Line* second_line, const Index& line_i,
                                          std::vector<LineState>& lines, GCS::Line& x_axis,
                                          GCS::Line& y_axis) {
        const auto same_line = [&](const GeometryRef& ref, GCS::Line* line) {
            return resolve_line(ref, line_i, lines, x_axis, y_axis) == line;
        };
        bool explicitly_parallel = false;
        bool first_horizontal = first_line == &x_axis;
        bool second_horizontal = second_line == &x_axis;
        bool first_vertical = first_line == &y_axis;
        bool second_vertical = second_line == &y_axis;
        for (const auto& relation : model.constraints) {
            if (relation.kind == ConstraintKind::parallel && relation.references.size() == 2U) {
                const auto& r0 = relation.references[0];
                const auto& r1 = relation.references[1];
                explicitly_parallel = explicitly_parallel ||
                                      (same_line(r0, first_line) && same_line(r1, second_line)) ||
                                      (same_line(r1, first_line) && same_line(r0, second_line));
                // Parallel to an intrinsic axis is equivalent to H/V.
                auto* l0 = resolve_line(r0, line_i, lines, x_axis, y_axis);
                auto* l1 = resolve_line(r1, line_i, lines, x_axis, y_axis);
                first_horizontal = first_horizontal || ((l0 == first_line && l1 == &x_axis) ||
                                                        (l1 == first_line && l0 == &x_axis));
                second_horizontal = second_horizontal || ((l0 == second_line && l1 == &x_axis) ||
                                                          (l1 == second_line && l0 == &x_axis));
                first_vertical = first_vertical || ((l0 == first_line && l1 == &y_axis) ||
                                                    (l1 == first_line && l0 == &y_axis));
                second_vertical = second_vertical || ((l0 == second_line && l1 == &y_axis) ||
                                                      (l1 == second_line && l0 == &y_axis));
            }
            if (relation.references.size() != 1U)
                continue;
            if (relation.kind == ConstraintKind::horizontal) {
                first_horizontal =
                    first_horizontal || same_line(relation.references[0], first_line);
                second_horizontal =
                    second_horizontal || same_line(relation.references[0], second_line);
            }
            if (relation.kind == ConstraintKind::vertical) {
                first_vertical = first_vertical || same_line(relation.references[0], first_line);
                second_vertical = second_vertical || same_line(relation.references[0], second_line);
            }
        }
        return explicitly_parallel || (first_horizontal && second_horizontal) ||
               (first_vertical && second_vertical);
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
    static GCS::Ellipse* resolve_ellipse(const GeometryRef& r, const Index& ellipse_i,
                                         const Index& arc_i, std::vector<EllipseState>& ellipses,
                                         std::vector<EllipticalArcState>& arcs) {
        if (r.target != GeometryTarget::entity || r.sub_element != SubElement::whole)
            return nullptr;
        if (const auto i = ellipse_i.find(r.entity_id); i != ellipse_i.end())
            return &ellipses[i->second].ellipse;
        if (const auto i = arc_i.find(r.entity_id); i != arc_i.end())
            return &arcs[i->second].arc;
        return nullptr;
    }
    static GCS::Point* resolve_center(const GeometryRef& r, const Index& circle_i,
                                      const Index& arc_i, std::vector<CircleState>& circles,
                                      std::vector<ArcState>& arcs,
                                      std::vector<EllipseState>& ellipses,
                                      std::vector<EllipticalArcState>& elliptical_arcs,
                                      const Index& ellipse_i, const Index& elliptical_arc_i) {
        if (r.target != GeometryTarget::entity)
            return nullptr;
        if (const auto i = circle_i.find(r.entity_id); i != circle_i.end())
            return &circles[i->second].center;
        if (const auto i = arc_i.find(r.entity_id); i != arc_i.end())
            return &arcs[i->second].center;
        if (const auto i = ellipse_i.find(r.entity_id); i != ellipse_i.end())
            return &ellipses[i->second].ellipse.center;
        if (const auto i = elliptical_arc_i.find(r.entity_id); i != elliptical_arc_i.end())
            return &elliptical_arcs[i->second].arc.center;
        return nullptr;
    }
    static std::vector<double*> resolve_entity_parameters(
        const GeometryRef& r, const Index& point_i, const Index& line_i, const Index& circle_i,
        const Index& arc_i, const Index& spline_i, std::vector<PointState>& points,
        std::vector<LineState>& lines, std::vector<CircleState>& circles,
        std::vector<ArcState>& arcs, std::vector<SplineState>& splines,
        std::vector<EllipseState>& ellipses, std::vector<EllipticalArcState>& elliptical_arcs,
        const Index& ellipse_i, const Index& elliptical_arc_i) {
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
        if (auto* e = resolve_ellipse(r, ellipse_i, elliptical_arc_i, ellipses, elliptical_arcs)) {
            std::vector<double*> out{e->center.x, e->center.y, e->focus1.x, e->focus1.y, e->radmin};
            if (const auto i = elliptical_arc_i.find(r.entity_id); i != elliptical_arc_i.end()) {
                out.push_back(elliptical_arcs[i->second].arc.startAngle);
                out.push_back(elliptical_arcs[i->second].arc.endAngle);
            }
            return out;
        }
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
