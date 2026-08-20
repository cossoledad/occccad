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
