// occccad Kernel — Public API
//
// This is the only header that business/control-plane code should include.
// No OCCT types are exposed through this API.

#ifndef OCCCCAD_KERNEL_HPP
#define OCCCCAD_KERNEL_HPP

#include <cstdint>
#include <string>
#include <string_view>
#include <vector>

namespace occccad::kernel {

// ---------------------------------------------------------------------------
// Geometry Identity
// ---------------------------------------------------------------------------

using GeometryId = std::string;  // SHA-256 hex string

// ---------------------------------------------------------------------------
// Topology Types
// ---------------------------------------------------------------------------

enum class TopologyType : uint8_t {
    VERTEX = 0,
    EDGE = 1,
    FACE = 2,
    SOLID = 3,
    COMPOUND = 4,
};

struct TopologyRef {
    GeometryId geometry_id;
    TopologyType type;
    uint64_t local_id;
};

// ---------------------------------------------------------------------------
// Geometry Primitives
// ---------------------------------------------------------------------------

struct Vec3 {
    double x = 0.0;
    double y = 0.0;
    double z = 0.0;
};

struct BoundingBox {
    Vec3 min;
    Vec3 max;
};

struct RectangularPadSpec {
    double origin_x = 0.0;
    double origin_y = 0.0;
    double width = 0.0;
    double height = 0.0;
    double pad_length = 0.0;
    std::string plane{"XY"};
};

// ---------------------------------------------------------------------------
// Tessellation Result
// ---------------------------------------------------------------------------

struct Triangle {
    uint32_t v0, v1, v2;
};

struct TessellationResult {
    std::vector<Vec3> vertices;
    std::vector<Triangle> triangles;
    std::vector<uint32_t> face_ids;  // triangle -> face index
    BoundingBox bbox;
};

// ---------------------------------------------------------------------------
// Topology Info
// ---------------------------------------------------------------------------

struct FaceInfo {
    uint64_t local_id;
    int surface_type;  // 0=plane, 1=cylinder, 2=cone, 3=sphere, 4=torus, 5=bspline, -1=other
    BoundingBox bbox;
};

struct EdgeInfo {
    uint64_t local_id;
    int curve_type;  // 0=line, 1=circle, 2=ellipse, 3=bspline, -1=other
    BoundingBox bbox;
};

struct TopologyInfo {
    uint32_t face_count;
    uint32_t edge_count;
    uint32_t vertex_count;
    uint32_t solid_count;
    std::vector<FaceInfo> faces;
    std::vector<EdgeInfo> edges;
};

// ---------------------------------------------------------------------------
// Abstract Kernel Interface
// ---------------------------------------------------------------------------

class ICadKernel {
public:
    virtual ~ICadKernel() = default;

    // Lifecycle
    virtual GeometryId loadBrepr(const std::vector<uint8_t>& data) = 0;
    virtual GeometryId loadStep(const std::string& path) = 0;
    virtual void unload(const GeometryId& id) = 0;

    // Primitive creation
    virtual GeometryId createBox(double dx, double dy, double dz) = 0;
    virtual GeometryId createRectangularPad(const RectangularPadSpec& spec) = 0;

    // Queries
    virtual BoundingBox getBoundingBox(const GeometryId& id) = 0;
    virtual TopologyInfo getTopology(const GeometryId& id) = 0;
    virtual double getVolume(const GeometryId& id) = 0;

    // Tessellation
    virtual TessellationResult tessellate(
        const GeometryId& id,
        double linear_deflection = 0.1,
        double angular_deflection = 0.5) = 0;

    // Feature operations (return new GeometryId)
    virtual GeometryId chamfer(
        const GeometryId& id,
        const std::vector<uint64_t>& edge_local_ids,
        double distance) = 0;

    virtual GeometryId fillet(
        const GeometryId& id,
        const std::vector<uint64_t>& edge_local_ids,
        double radius) = 0;

    // Serialization
    virtual std::vector<uint8_t> serializeBrepr(const GeometryId& id) = 0;
};

}  // namespace occccad::kernel

#endif  // OCCCCAD_KERNEL_HPP
