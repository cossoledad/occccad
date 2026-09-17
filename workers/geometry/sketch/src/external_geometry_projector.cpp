#include <occccad/geometry/sketch/external_geometry_projector.h>

#include <cmath>
#include <utility>

namespace occccad::geometry::sketch {
namespace {

double dot(const Vec3& left, const Vec3& right) {
    return left.x * right.x + left.y * right.y + left.z * right.z;
}

Vec3 subtract(const Vec3& left, const Vec3& right) {
    return {left.x - right.x, left.y - right.y, left.z - right.z};
}

Vec3 add_scaled(const Vec3& origin, const Vec3& direction, const double scale) {
    return {origin.x + direction.x * scale, origin.y + direction.y * scale,
            origin.z + direction.z * scale};
}

Vec3 cross(const Vec3& left, const Vec3& right) {
    return {left.y * right.z - left.z * right.y,
            left.z * right.x - left.x * right.z,
            left.x * right.y - left.y * right.x};
}

double length(const Vec3& value) {
    return std::sqrt(dot(value, value));
}

bool normalize(Vec3& value) {
    const double magnitude = length(value);
    if (!std::isfinite(magnitude) || magnitude <= 1.0e-12)
        return false;
    value = {value.x / magnitude, value.y / magnitude, value.z / magnitude};
    return true;
}

ExternalProjectionResult failure(std::string code, std::string diagnostic) {
    ExternalProjectionResult result;
    result.diagnostic_code = std::move(code);
    result.diagnostic = std::move(diagnostic);
    return result;
}

}  // namespace

ExternalProjectionResult project_external_geometry(const ExternalProjectionSource& source,
                                                    const ProjectionFrame& input_frame,
                                                    const double linear_tolerance,
                                                    const double angular_tolerance) {
    ProjectionFrame frame = input_frame;
    if (!normalize(frame.normal))
        return failure("EXTERNAL_PROJECTION_FRAME_INVALID", "sketch support normal is degenerate");
    const double along_normal = dot(frame.x_direction, frame.normal);
    frame.x_direction = subtract(frame.x_direction,
                                 {frame.normal.x * along_normal, frame.normal.y * along_normal,
                                  frame.normal.z * along_normal});
    if (!normalize(frame.x_direction))
        return failure("EXTERNAL_PROJECTION_FRAME_INVALID",
                       "sketch support X direction is degenerate");
    Vec3 y_direction = cross(frame.normal, frame.x_direction);
    if (!normalize(y_direction))
        return failure("EXTERNAL_PROJECTION_FRAME_INVALID", "sketch support frame is degenerate");
    const auto project = [&](const Vec3& point) {
        const Vec3 relative = subtract(point, frame.origin);
        return Vec2{dot(relative, frame.x_direction), dot(relative, y_direction)};
    };

    ExternalProjectionResult result;
    result.status = ExternalProjectionStatus::connected;
    if (source.kind == ExternalSourceKind::point) {
        result.kind = ProjectedGeometryKind::point;
        result.point = project(source.origin);
        return result;
    }
    if (source.kind == ExternalSourceKind::line) {
        if (!source.has_parameters)
            return failure("EXTERNAL_SOURCE_CONTRACT_INCOMPLETE",
                           "linear edge has no finite parameter range");
        Vec3 direction = source.direction;
        if (!normalize(direction))
            return failure("EXTERNAL_SOURCE_CONTRACT_INCOMPLETE",
                           "linear edge direction is degenerate");
        result.kind = ProjectedGeometryKind::line;
        result.start = project(add_scaled(source.origin, direction, source.parameter_start));
        result.end = project(add_scaled(source.origin, direction, source.parameter_end));
        if (std::hypot(result.end.x - result.start.x, result.end.y - result.start.y) <=
            linear_tolerance)
            return failure("EXTERNAL_PROJECTION_DEGENERATE",
                           "linear edge projects to a point in the sketch plane");
        return result;
    }
    if (source.kind == ExternalSourceKind::circle) {
        Vec3 axis = source.direction;
        if (!normalize(axis) || !source.has_parameters || source.measure_mm <= linear_tolerance)
            return failure("EXTERNAL_SOURCE_CONTRACT_INCOMPLETE",
                           "circular edge evidence is incomplete");
        if (1.0 - std::abs(dot(axis, frame.normal)) > angular_tolerance)
            return failure("EXTERNAL_PROJECTION_TYPE_UNSUPPORTED",
                           "oblique circular projection is an ellipse");
        const double sweep = std::abs(source.parameter_end - source.parameter_start);
        constexpr double full_turn = 2.0 * 3.14159265358979323846;
        if (std::abs(sweep - full_turn) > 1.0e-7)
            return failure("EXTERNAL_PROJECTION_TYPE_UNSUPPORTED",
                           "partial circular edges require arc orientation evidence");
        result.kind = ProjectedGeometryKind::circle;
        result.center = project(source.origin);
        result.radius = source.measure_mm / sweep;
        if (!std::isfinite(result.radius) || result.radius <= linear_tolerance)
            return failure("EXTERNAL_PROJECTION_DEGENERATE",
                           "circular edge projects with a degenerate radius");
        return result;
    }
    return failure("EXTERNAL_SOURCE_TYPE_MISMATCH", "only line, circle and vertex sources are supported");
}

}  // namespace occccad::geometry::sketch
