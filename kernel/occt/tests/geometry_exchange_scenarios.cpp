#include <internal/occt_kernel.hpp>

#include <gtest/gtest.h>

#include <chrono>
#include <cmath>
#include <filesystem>
#include <fstream>
#include <stdexcept>
#include <vector>

namespace occccad::kernel {
namespace {

class TemporaryStepFile {
public:
    explicit TemporaryStepFile(const std::vector<uint8_t>& data)
        : path_(std::filesystem::temp_directory_path() /
                ("occccad-exchange-test-" +
                 std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()) +
                 ".step")) {
        std::ofstream output(path_, std::ios::binary);
        output.write(reinterpret_cast<const char*>(data.data()),
                     static_cast<std::streamsize>(data.size()));
        if (!output)
            throw std::runtime_error("cannot write STEP test fixture");
    }

    ~TemporaryStepFile() { std::filesystem::remove(path_); }
    const std::filesystem::path& path() const { return path_; }

private:
    std::filesystem::path path_;
};

TEST(GeometryExchange, RectanglePadProducesSelectableTopologyAndRoundTrips) {
    OcctKernel kernel;
    const auto id = kernel.createRectangularPad({0.0, 0.0, 100.0, 60.0, 40.0, "XY"});
    const auto topology = kernel.getTopology(id);
    const auto mesh = kernel.tessellate(id);

    EXPECT_EQ(topology.faces.size(), 6U);
    EXPECT_EQ(topology.edges.size(), 12U);
    EXPECT_EQ(topology.vertices.size(), 8U);
    ASSERT_FALSE(topology.faces.empty());
    ASSERT_FALSE(topology.edges.empty());
    EXPECT_FALSE(topology.faces.front().properties.empty());
    EXPECT_FALSE(topology.edges.front().properties.empty());
    EXPECT_EQ(mesh.edges.size(), 12U);
    EXPECT_EQ(mesh.topology_vertices.size(), 8U);

    TemporaryStepFile step(kernel.serializeStep(id));
    ASSERT_EQ(kernel.inspectStepRootCount(step.path().string()), 1U);
    const auto imported = kernel.loadStepRoot(step.path().string(), 1U);
    const auto brep = kernel.serializeBrepr(imported);
    const auto brep_round_trip = kernel.loadBrepr(brep);
    EXPECT_FALSE(brep.empty());
    EXPECT_NEAR(kernel.getVolume(imported), kernel.getVolume(id), 1e-6);
    EXPECT_NEAR(kernel.getVolume(brep_round_trip), kernel.getVolume(id), 1e-6);
}

TEST(GeometryExchange, ReusesTopologyAnalysisForEveryElementOfTheSameBody) {
    OcctKernel kernel;
    const auto id = kernel.createRectangularPad({0.0, 0.0, 20.0, 10.0, 5.0, "XY"});

    const auto& first = kernel.getTopology(id);
    const auto& second = kernel.getTopology(id);

    EXPECT_EQ(&first, &second);
    EXPECT_EQ(first.faces.size(), 6U);
}

TEST(GeometryExchange, ProfilePadSupportsCircularOuterLoopAndHole) {
    OcctKernel kernel;
    ProfileCurveSpec outer;
    outer.entity_id = "outer";
    outer.kind = "CIRCLE";
    outer.center = {0.0, 0.0};
    outer.radius = 20.0;
    ProfileCurveSpec hole;
    hole.entity_id = "hole";
    hole.kind = "CIRCLE";
    hole.reversed = true;
    hole.center = {0.0, 0.0};
    hole.radius = 8.0;
    ProfileRegionSpec region;
    region.id = "annulus";
    region.outer = {"outer-loop", {outer}};
    region.holes = {{"hole-loop", {hole}}};

    const auto id = kernel.evaluateProfilePads({{{region}, 12.0, "XY"}});

    EXPECT_NEAR(kernel.getVolume(id), 3.14159265358979323846 * (400.0 - 64.0) * 12.0, 1.0e-5);
    EXPECT_GT(kernel.getTopology(id).solid_count, 0U);
}

TEST(GeometryExchange, ProfilePadKeepsArcAnglesInTheSketchPlane) {
    OcctKernel kernel;
    ProfileCurveSpec arc;
    arc.entity_id = "arc";
    arc.kind = "ARC";
    arc.center = {0.0, 0.0};
    arc.radius = 10.0;
    arc.start_angle = 0.0;
    arc.end_angle = 3.14159265358979323846;
    ProfileCurveSpec diameter;
    diameter.entity_id = "diameter";
    diameter.kind = "LINE";
    diameter.start = {-10.0, 0.0};
    diameter.end = {10.0, 0.0};
    ProfileRegionSpec region;
    region.id = "semicircle";
    region.outer = {"semicircle-loop", {arc, diameter}};

    const auto id = kernel.evaluateProfilePads({{{region}, 7.0, "XZ"}});

    EXPECT_NEAR(kernel.getVolume(id), 0.5 * 3.14159265358979323846 * 100.0 * 7.0,
                1.0e-5);
}

TEST(GeometryExchange, ProfilePadBuildsClosedSplineWire) {
    OcctKernel kernel;
    ProfileCurveSpec spline;
    spline.entity_id = "spline";
    spline.kind = "SPLINE";
    spline.control_points = {{0.0, 0.0}, {20.0, 0.0}, {20.0, 20.0}, {0.0, 20.0}};
    spline.degree = 3;
    spline.closed = true;
    ProfileRegionSpec region;
    region.id = "spline-region";
    region.outer = {"spline-loop", {spline}};

    const auto id = kernel.evaluateProfilePads({{{region}, 5.0, "YZ"}});

    EXPECT_GT(kernel.getVolume(id), 0.0);
    EXPECT_GT(kernel.getTopology(id).solid_count, 0U);
}

ProfileRegionSpec rectangular_region(const std::string& id, double x0, double y0, double x1,
                                     double y1) {
    ProfileRegionSpec region;
    region.id = id;
    region.outer.id = id + "-outer";
    const std::vector<Vec2> points{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}};
    for (std::size_t index = 0; index < points.size(); ++index) {
        ProfileCurveSpec line;
        line.entity_id = id + "-edge-" + std::to_string(index);
        line.kind = "LINE";
        line.start = points[index];
        line.end = points[(index + 1) % points.size()];
        region.outer.curves.push_back(line);
    }
    return region;
}

TEST(GeometryExchange, SolidFeatureChainFusesAndCutsOneBody) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec add;
    add.regions = {rectangular_region("add", 10, 0, 30, 20)};
    add.pad_length = 10;
    add.body_operation = "ADD";
    ProfilePadSpec remove;
    remove.regions = {rectangular_region("cut", 12, 5, 18, 15)};
    remove.pad_length = 10;
    remove.body_operation = "REMOVE";

    const auto fused = kernel.evaluateProfilePads({base, add});
    EXPECT_EQ(kernel.getTopology(fused).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(fused), 6000.0, 1.0e-6);
    const auto cut = kernel.evaluateProfilePads({base, add, remove});
    EXPECT_EQ(kernel.getTopology(cut).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(cut), 5400.0, 1.0e-6);
}

TEST(GeometryExchange, RevolveBuildsSolidAroundConstructionAxis) {
    OcctKernel kernel;
    ProfilePadSpec revolve;
    revolve.regions = {rectangular_region("profile", 5, -2, 10, 2)};
    revolve.generator = "REVOLVE";
    revolve.body_operation = "NEW_BODY";
    revolve.revolve_angle = 2.0 * 3.14159265358979323846;
    revolve.axis_start = {0, -10};
    revolve.axis_end = {0, 10};
    const auto result = kernel.evaluateProfilePads({revolve});
    EXPECT_EQ(kernel.getTopology(result).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(result), 300.0 * 3.14159265358979323846, 1.0e-5);
}

TEST(GeometryExchange, ExplicitDatumFramePlacesExtrudeOffTheDefaultPlanes) {
    OcctKernel kernel;
    ProfilePadSpec extrude;
    extrude.regions = {rectangular_region("offset-profile", 0, 0, 10, 5)};
    extrude.pad_length = 8;
    extrude.body_operation = "NEW_BODY";
    extrude.plane_origin = {100, 20, 30};
    extrude.plane_normal = {1, 0, 0};
    extrude.plane_u_direction = {0, 1, 0};
    const auto result = kernel.evaluateProfilePads({extrude});
    const auto bounds = kernel.getBoundingBox(result);
    EXPECT_NEAR(bounds.min.x, 100, 1.0e-6);
    EXPECT_NEAR(bounds.max.x, 108, 1.0e-6);
    EXPECT_NEAR(bounds.min.y, 20, 1.0e-6);
    EXPECT_NEAR(bounds.max.y, 30, 1.0e-6);
    EXPECT_NEAR(bounds.min.z, 30, 1.0e-6);
    EXPECT_NEAR(bounds.max.z, 35, 1.0e-6);
}

TEST(GeometryExchange, ProductStepKeepsOneTransferableRootPerOccurrence) {
    OcctKernel kernel;
    const auto id = kernel.createRectangularPad({0.0, 0.0, 20.0, 10.0, 5.0, "XY"});
    const std::vector<PlacedGeometry> components = {
        {id, {0.0, 0.0, 0.0}},
        {id, {50.0, 0.0, 0.0}},
    };

    TemporaryStepFile step(kernel.serializeStepComponents(components));
    ASSERT_EQ(kernel.inspectStepRootCount(step.path().string()), 2U);
    const auto first = kernel.loadStepRoot(step.path().string(), 1U);
    const auto second = kernel.loadStepRoot(step.path().string(), 2U);
    EXPECT_NEAR(kernel.getVolume(first), kernel.getVolume(id), 1e-6);
    EXPECT_NEAR(kernel.getVolume(second), kernel.getVolume(id), 1e-6);
    EXPECT_NEAR(kernel.getBoundingBox(second).min.x - kernel.getBoundingBox(first).min.x, 50.0,
                1e-6);
}

TEST(GeometryExchange, RepositoryStepFixturesContainImportableSolidGeometry) {
    OcctKernel kernel;
    const std::vector<std::filesystem::path> fixtures = {
        std::filesystem::path(OCCCCAD_MODEL_FIXTURE_DIR) / "Bottom Support - Bottom Support.step",
        std::filesystem::path(OCCCCAD_MODEL_FIXTURE_DIR) /
            "Windmill Head Cover - Windmill Head Cover.step",
    };
    for (const auto& fixture : fixtures) {
        SCOPED_TRACE(fixture.string());
        ASSERT_TRUE(std::filesystem::is_regular_file(fixture));
        const auto roots = kernel.inspectStepRootCount(fixture.string());
        ASSERT_GT(roots, 0U);
        for (uint32_t index = 1; index <= roots; ++index) {
            const auto imported = kernel.loadStepRoot(fixture.string(), index);
            EXPECT_GT(kernel.getVolume(imported), 0.0);
            EXPECT_GT(kernel.getTopology(imported).solid_count, 0U);
        }
    }
}

}  // namespace
}  // namespace occccad::kernel
