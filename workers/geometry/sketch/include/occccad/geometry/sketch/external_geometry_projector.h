#pragma once

#include <occccad/geometry/sketch/sketch_solver.h>

#include <string>

namespace occccad::geometry::sketch {

struct Vec3 {
    double x{};
    double y{};
    double z{};
};

struct ProjectionFrame {
    Vec3 origin;
    Vec3 x_direction;
    Vec3 normal;
};

enum class ExternalSourceKind { point, line, circle };

struct ExternalProjectionSource {
    ExternalSourceKind kind{ExternalSourceKind::point};
    Vec3 origin;
    Vec3 direction;
    double measure_mm{};
    double parameter_start{};
    double parameter_end{};
    bool has_parameters{};
};

enum class ExternalProjectionStatus { connected, unresolved };
enum class ProjectedGeometryKind { none, point, line, circle };

struct ExternalProjectionResult {
    ExternalProjectionStatus status{ExternalProjectionStatus::unresolved};
    ProjectedGeometryKind kind{ProjectedGeometryKind::none};
    Vec2 point;
    Vec2 start;
    Vec2 end;
    Vec2 center;
    double radius{};
    std::string diagnostic_code;
    std::string diagnostic;
};

[[nodiscard]] ExternalProjectionResult project_external_geometry(
    const ExternalProjectionSource& source, const ProjectionFrame& frame,
    double linear_tolerance = 1.0e-7, double angular_tolerance = 1.0e-9);

}  // namespace occccad::geometry::sketch
