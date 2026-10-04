// occccad Kernel — Public API
//
// This is the only header that business/control-plane code should include.
// No OCCT types are exposed through this API.

#ifndef OCCCCAD_KERNEL_HPP
#define OCCCCAD_KERNEL_HPP

#include <cstdint>
#include <array>
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

struct Vec2 {
    double x = 0.0;
    double y = 0.0;
};

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

struct ProfileCurveSpec {
    std::string entity_id;
    bool reversed{};
    std::string kind;
    Vec2 start;
    Vec2 end;
    Vec2 center;
    double radius{};
    double start_angle{};
    double end_angle{};
    double major_radius{};
    double minor_radius{};
    double rotation{};
    std::vector<Vec2> control_points;
    uint32_t degree{};
    bool closed{};
    std::string mode{"FIT"};
    std::vector<Vec2> poles;
    std::vector<double> knots;
    std::vector<uint32_t> multiplicities;
    std::vector<double> weights;
    bool periodic{};
    double parameter_start{};
    double parameter_end{};
};
// Exact local sketch curves; units are mm and radians. No sampled authority.
struct SketchCurveEvaluation {
    Vec2 point;
    Vec2 first_derivative;
};
enum class SketchCurveIntersectionKind { point, tangent };
struct SketchCurveIntersection {
    SketchCurveIntersectionKind kind{SketchCurveIntersectionKind::point};
    Vec2 point;
    double first_parameter{};
    double second_parameter{};
};
struct SketchCurveOverlap {
    double first_start{};
    double first_end{};
    double second_start{};
    double second_end{};
};
struct SketchCurveIntersections {
    std::vector<SketchCurveIntersection> points;
    std::vector<SketchCurveOverlap> overlaps;
};
[[nodiscard]] ProfileCurveSpec canonicalize_sketch_curve(const ProfileCurveSpec& curve);
[[nodiscard]] SketchCurveEvaluation evaluate_sketch_curve(const ProfileCurveSpec& curve,
                                                          double parameter);
[[nodiscard]] SketchCurveIntersections intersect_sketch_curves(const ProfileCurveSpec& first,
                                                               const ProfileCurveSpec& second,
                                                               double tolerance = 1e-7);
[[nodiscard]] ProfileCurveSpec trim_sketch_curve(const ProfileCurveSpec& curve,
                                                 double start_parameter, double end_parameter);

struct ProfileLoopSpec {
    std::string id;
    std::vector<ProfileCurveSpec> curves;
};
struct ProfileRegionSpec {
    std::string id;
    ProfileLoopSpec outer;
    std::vector<ProfileLoopSpec> holes;
};
[[nodiscard]] std::vector<ProfileRegionSpec>
classify_sketch_profile(const std::vector<ProfileLoopSpec>& loops);
// Locators are valid only in the supplied immutable B-Rep and naming snapshot.
struct FeatureTopologyRef {
    std::string feature_id, output_slot;
    std::vector<std::string> source_ids;
};
struct BodyToolTopology {
    std::string feature_id, output_slot;
    std::vector<std::string> source_ids;
    TopologyType type;
    uint64_t local_id{};
};
struct BodyToolInput {
    std::string body_id, feature_id, geometry_id;
    std::vector<uint8_t> brep;
    std::vector<BodyToolTopology> topology;
};
struct LoftSectionSpec {
    std::string sketch_id, seam_entity_id;
    ProfileRegionSpec region;
    Vec3 origin, normal, u_direction;
    bool reversed{};
    double seam_angle{};
};
struct PatternPlacement {
    std::uint32_t slot{};
    std::array<double, 12> matrix{};
};
struct ProfilePadSpec {
    std::string pattern_source_feature_id, pattern_source_kind;
    std::string pattern_result_mode;
    std::vector<PatternPlacement> pattern_placements;
    std::vector<ProfileRegionSpec> regions;
    double pad_length{};
    std::string plane{"XY"};
    std::string body_operation{"ADD"};
    std::string generator{"LINEAR_EXTRUDE"};
    double revolve_angle{};
    Vec2 axis_start;
    Vec2 axis_end;
    bool reversed{};
    Vec3 plane_origin;
    Vec3 plane_normal;
    Vec3 plane_u_direction;
    // Stable domain identities used by topology naming. They do not affect the
    // generated B-Rep and must never contain OCCT-local topology indices.
    std::string feature_id;
    std::string body_id;
    std::string input_feature_id;
    std::string profile_feature_id;
    std::vector<BodyToolInput> tools;
    std::string extent;
    double second_length{};
    std::vector<FeatureTopologyRef> selections;
    Vec3 neutral_origin, neutral_normal;
    std::vector<LoftSectionSpec> sections;
    bool ruled{};
};

// ---------------------------------------------------------------------------
// Tessellation Result
// ---------------------------------------------------------------------------

struct Triangle {
    uint32_t v0, v1, v2;
};

struct EdgePolyline {
    uint64_t local_id;
    std::vector<Vec3> points;
};

struct TopologyPoint {
    uint64_t local_id;
    Vec3 point;
};

struct VisualStageTimings {
    double meshing_ms = 0, faces_ms = 0, edges_ms = 0, normals_ms = 0;
};

struct TessellationResult {
    std::vector<Vec3> normals;
    VisualStageTimings timings;
    std::vector<Vec3> vertices;
    std::vector<Triangle> triangles;
    std::vector<uint32_t> face_ids;  // triangle -> 1-based frozen face local ID
    std::vector<EdgePolyline> edges;
    std::vector<TopologyPoint> topology_vertices;
    BoundingBox bbox;
};

// ---------------------------------------------------------------------------
// Topology Info
// ---------------------------------------------------------------------------

struct TopologyProperty {
    enum class Kind : uint8_t { NUMBER, INTEGER, BOOLEAN, TEXT, VECTOR };
    std::string name;
    Kind kind{Kind::NUMBER};
    double number_value{0.0};
    int64_t integer_value{0};
    bool bool_value{false};
    std::string text_value;
    Vec3 vector_value;
};

struct FaceInfo {
    uint64_t local_id;
    int surface_type;  // 0=plane, 1=cylinder, 2=cone, 3=sphere, 4=torus, 5=bspline, -1=other
    BoundingBox bbox;
    std::vector<TopologyProperty> properties;
};

struct EdgeInfo {
    uint64_t local_id;
    int curve_type;  // 0=line, 1=circle, 2=ellipse, 3=bspline, -1=other
    BoundingBox bbox;
    std::vector<TopologyProperty> properties;
    std::vector<Vec3> render_points; // Reserved empty: display edges exist only in TessellationResult.
};

struct VertexInfo {
    uint64_t local_id;
    Vec3 point;
    std::vector<TopologyProperty> properties;
};

struct TopologyInfo {
    uint32_t face_count;
    uint32_t edge_count;
    uint32_t vertex_count;
    uint32_t solid_count;
    std::vector<FaceInfo> faces;
    std::vector<EdgeInfo> edges;
    std::vector<VertexInfo> vertices;
};

struct PlacedGeometry {
    GeometryId geometry_id;
    Vec3 translation;
    struct Rotation {
        double x{0.0};
        double y{0.0};
        double z{0.0};
        double w{1.0};
    } rotation;
};

// ---------------------------------------------------------------------------
// Exchange-only graph. IDs are source definition/occurrence identities, never geometry hashes.
struct ExchangeOccurrence {
    std::string id, definition_id, name;
    PlacedGeometry placement;
};
struct ExchangeDefinition {
    std::string id, name, kind; // PART | PRODUCT
    GeometryId geometry_id; // definition-local, PART only
    std::vector<ExchangeOccurrence> children;
};
struct ExchangeGraph {
    std::vector<ExchangeDefinition> definitions;
    std::vector<ExchangeOccurrence> roots;
};

// Abstract Kernel Interface
// ---------------------------------------------------------------------------

class ICadKernel {
public:
    virtual ~ICadKernel() = default;

    // Lifecycle
    virtual GeometryId loadBrepr(const std::vector<uint8_t>& data) = 0;
    virtual GeometryId loadStep(const std::string& path) = 0;
    virtual uint32_t inspectStepRootCount(const std::string& path) = 0;
    virtual GeometryId loadStepRoot(const std::string& path, uint32_t root_index) = 0;
    // Preserve each located Solid occurrence; component ordinal is not a persistent topology ID.
    virtual std::vector<GeometryId> splitSolids(const GeometryId& id) = 0;
    virtual GeometryId repairImportedSolid(const GeometryId& id) = 0;
    virtual GeometryId combine(const std::vector<PlacedGeometry>& components) = 0;
    virtual void unload(const GeometryId& id) = 0;

    // Primitive creation
    virtual GeometryId createBox(double dx, double dy, double dz) = 0;
    virtual GeometryId createRectangularPad(const RectangularPadSpec& spec) = 0;
    virtual GeometryId evaluateRectangularPads(const std::vector<RectangularPadSpec>& specs,
                                               const std::vector<uint8_t>& base_brep = {}) = 0;
    virtual GeometryId evaluateProfilePads(const std::vector<ProfilePadSpec>& specs,
                                           const std::vector<uint8_t>& base_brep = {}) = 0;

    // Queries
    virtual BoundingBox getBoundingBox(const GeometryId& id) = 0;
    virtual const TopologyInfo& getTopology(const GeometryId& id) = 0;
    virtual double getVolume(const GeometryId& id) = 0;

    // Tessellation
    virtual TessellationResult tessellate(const GeometryId& id, double linear_deflection = 0.1,
                                          double angular_deflection = 0.5, bool parallel = true) = 0;

    // Feature operations (return new GeometryId)
    virtual GeometryId chamfer(const GeometryId& id, const std::vector<uint64_t>& edge_local_ids,
                               double distance) = 0;

    virtual GeometryId fillet(const GeometryId& id, const std::vector<uint64_t>& edge_local_ids,
                              double radius) = 0;

    // Serialization
    virtual std::vector<uint8_t> serializeBrepr(const GeometryId& id) = 0;
    virtual GeometryId loadStepData(const std::vector<uint8_t>& data) = 0;
    virtual std::vector<uint8_t> serializeStep(const GeometryId& id) = 0;
};

}  // namespace occccad::kernel

#endif  // OCCCCAD_KERNEL_HPP
