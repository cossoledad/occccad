#ifndef OCCCCAD_GEOMETRY_FEATURE_HPP
#define OCCCCAD_GEOMETRY_FEATURE_HPP

#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/topology_naming.hpp>

namespace occccad::kernel {
struct GeometryPlane {
    Vec3 origin, normal, u_direction;
    std::string id;
};
struct GeometryFeatureInput {
    BodyToolInput source;
    std::vector<FeatureTopologyRef> members;
    bool reversed{};
    std::optional<GeometryPlane> plane;
};
struct GeometryControlPoint {
    std::string id;
    Vec3 position;
    std::vector<FeatureTopologyRef> sources;
};
struct GeometryCurveDefinition {
    std::vector<GeometryControlPoint> points;
    Vec3 normal, x_direction;
    double radius{}, start_angle{}, end_angle{};
    uint32_t degree{3};
    std::vector<double> knots, weights;
    std::vector<uint32_t> multiplicities;
    bool closed{};
};
struct GeometryOperationDefinition {
    Vec3 direction, origin;
    double distance{}, angle{}, start{}, end{1}, tension{1};
    std::string side;
};
struct GeometrySketchWire {
    std::string sketch_id;
    std::vector<ProfileCurveSpec> curves;
    Vec3 origin, normal, u_direction;
};
struct GeometrySurfaceDefinition {
    bool ruled{}, has_keep_point{};
    std::string frame_mode;
    double tolerance{};
    Vec3 keep_point;
};
struct GeometryFeatureSpec {
    std::string feature_id, result_id{"result"}, operation;
    std::vector<GeometryFeatureInput> inputs;
    GeometryCurveDefinition curve;
    GeometryOperationDefinition parameters;
    GeometrySketchWire sketch_wire;
    GeometrySurfaceDefinition surface;
    std::optional<ImportTopologySeed> import_seed;
    std::vector<uint8_t> import_brep;
};
}  // namespace occccad::kernel
#endif
