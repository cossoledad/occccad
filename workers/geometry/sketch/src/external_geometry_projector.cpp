#include <occccad/geometry/sketch/external_geometry_projector.h>

#include <cmath>
#include <algorithm>
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
    if (source.kind == ExternalSourceKind::axis) {
        Vec3 direction=source.direction;
        if(!normalize(direction)) return failure("EXTERNAL_SOURCE_CONTRACT_INCOMPLETE","datum axis direction is degenerate");
        const auto origin=project(source.origin), end=project(add_scaled(source.origin,direction,1.0));
        const double dx=end.x-origin.x,dy=end.y-origin.y,length=std::hypot(dx,dy);
        if(length<=angular_tolerance){result.kind=ProjectedGeometryKind::point;result.point=origin;}
        else {result.kind=ProjectedGeometryKind::line;result.start=origin;result.end={origin.x+dx/length,origin.y+dy/length};}
        return result;
    }
    if (source.kind == ExternalSourceKind::line) {
        if (!source.has_parameters || !std::isfinite(source.parameter_start) ||
            !std::isfinite(source.parameter_end) ||
            std::abs(source.parameter_end - source.parameter_start) <= linear_tolerance)
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
            linear_tolerance) {
            result.kind = ProjectedGeometryKind::point;
            result.point = {(result.start.x + result.end.x) * 0.5,
                            (result.start.y + result.end.y) * 0.5};
        }
        return result;
    }
    if (source.kind == ExternalSourceKind::circle || source.kind == ExternalSourceKind::ellipse) {
        constexpr double tau = 2.0 * 3.14159265358979323846;
        Vec3 axis = source.direction, x = source.x_direction;
        const double a = source.radius;
        const double b = source.kind == ExternalSourceKind::circle ? a : source.minor_radius;
        const double sweep = source.parameter_end - source.parameter_start;
        if (!normalize(axis) || !normalize(x) || std::abs(dot(axis, x)) > angular_tolerance ||
            !source.has_parameters || !std::isfinite(source.parameter_start) || !std::isfinite(source.parameter_end) ||
            !std::isfinite(a) || !std::isfinite(b) || a <= linear_tolerance || b <= linear_tolerance ||
            sweep <= angular_tolerance || sweep > tau + angular_tolerance)
            return failure("EXTERNAL_SOURCE_CONTRACT_INCOMPLETE", "conic requires exact radii, oriented axes and finite angular interval");
        const Vec3 y = cross(axis, x);
        // A maps the original parameter circle onto the sketch plane. Its left
        // singular vectors/radii give an exact conic, with no sampled fit.
        const Vec2 u{a * dot(x, frame.x_direction), a * dot(x, y_direction)};
        const Vec2 v{b * dot(y, frame.x_direction), b * dot(y, y_direction)};
        const double xx = u.x*u.x + v.x*v.x, yy = u.y*u.y + v.y*v.y;
        const double xy = u.x*u.y + v.x*v.y, determinant = u.x*v.y-u.y*v.x;
        result.center = project(source.origin);
        result.rotation = 0.5 * std::atan2(2*xy, xx-yy);
        result.major_radius = std::sqrt(std::max(0.0, 0.5*(xx+yy+std::hypot(xx-yy, 2*xy))));
        if (result.major_radius <= linear_tolerance) {
            result.kind = ProjectedGeometryKind::point; result.point = result.center; return result;
        }
        result.minor_radius = std::abs(determinant)/result.major_radius;
        const double c = std::cos(result.rotation), sn = std::sin(result.rotation);
        const auto coordinates = [&](double t) {
            return Vec2{u.x*std::cos(t)+v.x*std::sin(t), u.y*std::cos(t)+v.y*std::sin(t)};
        };
        if (result.minor_radius <= linear_tolerance) {
            // Edge-on conics collapse to an interval; include interior extrema,
            // since projected arc endpoints alone do not define this interval.
            const double uc=c*u.x+sn*u.y, vc=c*v.x+sn*v.y;
            const auto scalar=[&](double t){return uc*std::cos(t)+vc*std::sin(t);};
            double low=std::min(scalar(source.parameter_start),scalar(source.parameter_end));
            double high=std::max(scalar(source.parameter_start),scalar(source.parameter_end));
            const double extreme=std::atan2(vc,uc);
            for (int i=0;i<2;++i) {
                double offset=std::fmod(extreme+i*tau/2-source.parameter_start,tau);
                if(offset<0)offset+=tau;
                if(offset<=sweep+angular_tolerance){const double z=scalar(source.parameter_start+offset);low=std::min(low,z);high=std::max(high,z);}
            }
            result.kind=ProjectedGeometryKind::line;
            result.start={result.center.x+c*low,result.center.y+sn*low};
            result.end={result.center.x+c*high,result.center.y+sn*high};
            if(high-low<=linear_tolerance){result.kind=ProjectedGeometryKind::point;result.point=result.start;}
            return result;
        }
        const bool circular=std::abs(result.major_radius-result.minor_radius)<=linear_tolerance;
        const bool full=std::abs(sweep-tau)<=angular_tolerance;
        // Reflection reverses parameter orientation. Store the same trimmed locus
        // using the sketch's positive sweep convention, without taking a complement.
        const auto p=coordinates(determinant<0 ? source.parameter_end : source.parameter_start);
        result.start_angle = circular ? std::atan2(p.y,p.x) :
            std::atan2((-sn*p.x+c*p.y)/result.minor_radius,(c*p.x+sn*p.y)/result.major_radius);
        result.end_angle=result.start_angle+sweep;
        result.radius=result.major_radius;
        result.kind=circular ? (full ? ProjectedGeometryKind::circle : ProjectedGeometryKind::arc) :
            (full ? ProjectedGeometryKind::ellipse : ProjectedGeometryKind::elliptical_arc);
        return result;
    }
    return failure("EXTERNAL_SOURCE_TYPE_MISMATCH", "only line, circle and vertex sources are supported");
}

}  // namespace occccad::geometry::sketch
