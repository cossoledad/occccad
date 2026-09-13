#include <internal/occt_kernel.hpp>
#include <occccad/kernel/topology_naming.hpp>

#include <gtest/gtest.h>

#include <algorithm>
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

    ProfilePadSpec pad;
    pad.regions = {region};
    pad.pad_length = 12.0;
    pad.plane = "XY";
    const auto evaluation = kernel.evaluateProfilePadsWithHistory({pad});
    const auto id = evaluation.geometry_id;

    EXPECT_NEAR(kernel.getVolume(id), 3.14159265358979323846 * (400.0 - 64.0) * 12.0, 1.0e-5);
    EXPECT_GT(kernel.getTopology(id).solid_count, 0U);
    ASSERT_EQ(evaluation.feature_results.size(), 1U);
    const auto& feature = evaluation.feature_results.front();
    EXPECT_TRUE(feature.topology_history_complete);
    EXPECT_EQ(feature.semantic_outputs.size(), kernel.getTopology(id).face_count +
                                                   kernel.getTopology(id).edge_count +
                                                   kernel.getTopology(id).vertex_count);
    EXPECT_TRUE(std::any_of(feature.semantic_outputs.begin(), feature.semantic_outputs.end(),
                            [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::vertex &&
                                       output.evidence.endpoint_role.find("CAP_VERTEX") !=
                                           std::string::npos;
                            }));
}

TEST(GeometryExchange, NamingRejectsProfileEdgesBelowPolicyTolerance) {
    OcctKernel kernel;
    ProfilePadSpec pad;
    pad.feature_id = "short-edge-pad";
    pad.body_id = "body-main";
    pad.profile_feature_id = "short-edge-sketch";
    ProfileRegionSpec region;
    region.id = "short";
    region.outer.id = "short-outer";
    const std::vector<Vec2> points{{0, 0}, {0.00001, 0}, {0.00001, 10}, {0, 10}};
    for (std::size_t index = 0; index < points.size(); ++index) {
        ProfileCurveSpec line;
        line.entity_id = "short-edge-" + std::to_string(index);
        line.kind = "LINE";
        line.start = points[index];
        line.end = points[(index + 1) % points.size()];
        region.outer.curves.push_back(line);
    }
    pad.regions = {region};
    pad.pad_length = 5;
    pad.body_operation = "NEW_BODY";

    try {
        (void)kernel.evaluateProfilePadsWithHistory({pad});
        FAIL() << "profile edge below naming tolerance must be rejected";
    } catch (const std::invalid_argument& error) {
        EXPECT_NE(std::string(error.what()).find("DEGENERATE_PROFILE_EDGE"), std::string::npos);
    }
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

    ProfilePadSpec pad;
    pad.regions = {region};
    pad.pad_length = 7.0;
    pad.plane = "XZ";
    const auto id = kernel.evaluateProfilePads({pad});

    EXPECT_NEAR(kernel.getVolume(id), 0.5 * 3.14159265358979323846 * 100.0 * 7.0, 1.0e-5);
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

    ProfilePadSpec pad;
    pad.regions = {region};
    pad.pad_length = 5.0;
    pad.plane = "YZ";
    const auto id = kernel.evaluateProfilePads({pad});

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

std::size_t faces_on_z(const TopologyInfo& topology, const double z) {
    return static_cast<std::size_t>(
        std::count_if(topology.faces.begin(), topology.faces.end(), [z](const FaceInfo& face) {
            return std::abs(face.bbox.min.z - z) < 1.0e-6 && std::abs(face.bbox.max.z - z) < 1.0e-6;
        }));
}

std::size_t faces_on_y(const TopologyInfo& topology, const double y) {
    return static_cast<std::size_t>(
        std::count_if(topology.faces.begin(), topology.faces.end(), [y](const FaceInfo& face) {
            return std::abs(face.bbox.min.y - y) < 1.0e-6 && std::abs(face.bbox.max.y - y) < 1.0e-6;
        }));
}

TEST(GeometryExchange, NamingFixtureExtrudeLengthChangesGeometryButKeepsDomainIdentity) {
    OcctKernel kernel;
    ProfilePadSpec twenty;
    twenty.feature_id = "extrude-1";
    twenty.body_id = "body-main";
    twenty.profile_feature_id = "sketch-1";
    twenty.regions = {rectangular_region("region-1", 0, 0, 20, 10)};
    twenty.pad_length = 20;
    twenty.body_operation = "NEW_BODY";
    ProfilePadSpec forty = twenty;
    forty.pad_length = 40;

    const auto twenty_id = kernel.evaluateProfilePads({twenty});
    const auto forty_id = kernel.evaluateProfilePads({forty});

    EXPECT_NE(twenty_id, forty_id);
    EXPECT_NEAR(kernel.getBoundingBox(twenty_id).max.z, 20.0, 1.0e-6);
    EXPECT_NEAR(kernel.getBoundingBox(forty_id).max.z, 40.0, 1.0e-6);
    EXPECT_EQ(twenty.feature_id, forty.feature_id);
    EXPECT_EQ(twenty.profile_feature_id, forty.profile_feature_id);
    EXPECT_EQ(faces_on_z(kernel.getTopology(twenty_id), 20.0), 1U);
    EXPECT_EQ(faces_on_z(kernel.getTopology(forty_id), 40.0), 1U);
}

TEST(GeometryExchange, TopologyHistoryKeepsExtrudeSemanticOutputsAcrossLengthEdit) {
    OcctKernel kernel;
    ProfilePadSpec twenty;
    twenty.feature_id = "extrude-1";
    twenty.body_id = "body-main";
    twenty.profile_feature_id = "sketch-1";
    twenty.regions = {rectangular_region("region-1", 0, 0, 20, 10)};
    twenty.pad_length = 20;
    twenty.body_operation = "NEW_BODY";
    auto forty = twenty;
    forty.pad_length = 40;

    const auto first = kernel.evaluateProfilePadsWithHistory({twenty});
    const auto second = kernel.evaluateProfilePadsWithHistory({forty});
    const auto repeated = kernel.evaluateProfilePadsWithHistory({twenty});
    ASSERT_EQ(first.feature_results.size(), 1U);
    ASSERT_EQ(second.feature_results.size(), 1U);
    const auto slots = [](const FeatureResult& feature) {
        std::vector<std::string> result;
        for (const auto& output : feature.semantic_outputs)
            result.push_back(output.semantic_ref.output_slot);
        std::sort(result.begin(), result.end());
        return result;
    };
    EXPECT_EQ(slots(first.feature_results.front()), slots(second.feature_results.front()));
    EXPECT_EQ(first.geometry_id, repeated.geometry_id);
    EXPECT_EQ(slots(first.feature_results.front()), slots(repeated.feature_results.front()));
    EXPECT_EQ(first.feature_results.front().topology_history.evidence_digest,
              repeated.feature_results.front().topology_history.evidence_digest);
    const auto& first_feature = first.feature_results.front();
    const auto& first_topology = kernel.getTopology(first.geometry_id);
    EXPECT_EQ(first_feature.semantic_outputs.size(), 26U);
    EXPECT_EQ(std::count_if(first_feature.semantic_outputs.begin(),
                            first_feature.semantic_outputs.end(), [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::face;
                            }),
              6);
    EXPECT_EQ(std::count_if(first_feature.semantic_outputs.begin(),
                            first_feature.semantic_outputs.end(), [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::edge;
                            }),
              12);
    EXPECT_EQ(std::count_if(first_feature.semantic_outputs.begin(),
                            first_feature.semantic_outputs.end(), [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::vertex;
                            }),
              8);
    EXPECT_EQ(first_feature.semantic_outputs.size(), first_topology.face_count +
                                                         first_topology.edge_count +
                                                         first_topology.vertex_count);
    EXPECT_TRUE(first.feature_results.front().topology_history_complete);
    EXPECT_FALSE(first.feature_results.front().topology_history.evidence_digest.empty());
    for (const auto& output : second.feature_results.front().semantic_outputs) {
        EXPECT_GE(output.local_id, 1U);
        const auto& topology = kernel.getTopology(second.geometry_id);
        if (output.topology_type == PersistentTopologyType::face)
            EXPECT_LE(output.local_id, topology.face_count);
        else if (output.topology_type == PersistentTopologyType::edge)
            EXPECT_LE(output.local_id, topology.edge_count);
        else if (output.topology_type == PersistentTopologyType::vertex)
            EXPECT_LE(output.local_id, topology.vertex_count);
        else
            ADD_FAILURE() << "semantic output has unspecified topology type";
        EXPECT_FALSE(output.evidence.evidence_digest.empty());
        EXPECT_FALSE(output.evidence.adjacent.empty());
    }
    const auto contains_slot = [&](const std::string& prefix) {
        return std::any_of(first_feature.semantic_outputs.begin(),
                           first_feature.semantic_outputs.end(), [&](const auto& output) {
                               return output.semantic_ref.output_slot.rfind(prefix, 0) == 0;
                           });
    };
    EXPECT_TRUE(contains_slot("START_BOUNDARY_FROM_PROFILE_EDGE/"));
    EXPECT_TRUE(contains_slot("END_BOUNDARY_FROM_PROFILE_EDGE/"));
    EXPECT_TRUE(contains_slot("VERTICAL_FROM_PROFILE_ENDPOINTS/"));
    EXPECT_TRUE(contains_slot("START_VERTEX_FROM_PROFILE_ENDPOINTS/"));
    EXPECT_TRUE(contains_slot("END_VERTEX_FROM_PROFILE_ENDPOINTS/"));
}

TEST(GeometryExchange, TopologyHistoryComposesBooleanAndSameDomainHistory) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-base";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec add = base;
    add.feature_id = "extrude-add";
    add.input_feature_id = base.feature_id;
    add.profile_feature_id = "sketch-add";
    add.regions = {rectangular_region("add", 10, 0, 30, 20)};
    add.body_operation = "ADD";
    ProfilePadSpec remove = base;
    remove.feature_id = "cut-1";
    remove.input_feature_id = add.feature_id;
    remove.profile_feature_id = "sketch-cut";
    remove.regions = {rectangular_region("cut", 12, 5, 18, 15)};
    remove.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, add, remove});
    const auto repeated = kernel.evaluateProfilePadsWithHistory({base, add, remove});
    ASSERT_EQ(evaluation.feature_results.size(), 3U);
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(evaluation.geometry_id), 5400.0, 1.0e-6);
    const auto& add_history = evaluation.feature_results[1].topology_history;
    const auto& cut_history = evaluation.feature_results[2].topology_history;
    EXPECT_TRUE(std::any_of(
        add_history.lineage.begin(), add_history.lineage.end(),
        [](const TopologyLineage& value) { return value.kind == TopologyLineageKind::merged; }));
    EXPECT_TRUE(std::any_of(cut_history.lineage.begin(), cut_history.lineage.end(),
                            [](const TopologyLineage& value) {
                                return value.kind == TopologyLineageKind::modified ||
                                       value.kind == TopologyLineageKind::unchanged;
                            }));
    EXPECT_FALSE(cut_history.deleted.empty());
    EXPECT_TRUE(evaluation.feature_results[2].topology_history_complete);
    const auto& final_topology = kernel.getTopology(evaluation.geometry_id);
    const auto& final_outputs = evaluation.feature_results[2].semantic_outputs;
    EXPECT_EQ(final_outputs.size(), final_topology.face_count + final_topology.edge_count +
                                        final_topology.vertex_count);
    EXPECT_EQ(std::count_if(final_outputs.begin(), final_outputs.end(), [](const auto& output) {
                  return output.topology_type == PersistentTopologyType::edge;
              }),
              final_topology.edge_count);
    EXPECT_EQ(std::count_if(final_outputs.begin(), final_outputs.end(), [](const auto& output) {
                  return output.topology_type == PersistentTopologyType::vertex;
              }),
              final_topology.vertex_count);
    const auto merged_edge = std::find_if(
        evaluation.feature_results[1].topology_history.lineage.begin(),
        evaluation.feature_results[1].topology_history.lineage.end(), [&](const auto& lineage) {
            if (lineage.kind != TopologyLineageKind::merged)
                return false;
            return std::any_of(evaluation.feature_results[1].semantic_outputs.begin(),
                               evaluation.feature_results[1].semantic_outputs.end(),
                               [&](const auto& output) {
                                   return output.topology_type == PersistentTopologyType::edge &&
                                          output.semantic_ref.feature_id == lineage.result.feature_id &&
                                          output.semantic_ref.output_slot == lineage.result.output_slot;
                               });
        });
    EXPECT_NE(merged_edge, evaluation.feature_results[1].topology_history.lineage.end());
    EXPECT_EQ(evaluation.feature_results[1].topology_history.evidence_digest,
              repeated.feature_results[1].topology_history.evidence_digest);
    ASSERT_EQ(evaluation.feature_results[1].semantic_outputs.size(),
              repeated.feature_results[1].semantic_outputs.size());
    for (std::size_t index = 0; index < evaluation.feature_results[1].semantic_outputs.size(); ++index)
        EXPECT_EQ(evaluation.feature_results[1].semantic_outputs[index].semantic_ref.output_slot,
                  repeated.feature_results[1].semantic_outputs[index].semantic_ref.output_slot);
    for (const auto& tombstone : cut_history.deleted) {
        EXPECT_FALSE(
            std::any_of(evaluation.feature_results[2].semantic_outputs.begin(),
                        evaluation.feature_results[2].semantic_outputs.end(),
                        [&](const SemanticTopologyOutput& output) {
                            return output.semantic_ref.feature_id == tombstone.source.feature_id &&
                                   output.semantic_ref.output_slot == tombstone.source.output_slot;
                        }));
    }
}

TEST(GeometryExchange, RevolveReportsTopologyHistoryAsIncompleteUntilItsNamingPhase) {
    OcctKernel kernel;
    ProfilePadSpec revolve;
    revolve.feature_id = "revolve-1";
    revolve.body_id = "body-main";
    revolve.profile_feature_id = "sketch-revolve";
    revolve.regions = {rectangular_region("section", 5, 0, 10, 5)};
    revolve.generator = "REVOLVE";
    revolve.revolve_angle = 2.0 * 3.14159265358979323846;
    revolve.axis_start = {0, -10};
    revolve.axis_end = {0, 10};
    revolve.body_operation = "NEW_BODY";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({revolve});
    ASSERT_EQ(evaluation.feature_results.size(), 1U);
    EXPECT_FALSE(evaluation.feature_results.front().topology_history_complete);
    EXPECT_NE(std::find(evaluation.feature_results.front().diagnostics.begin(),
                        evaluation.feature_results.front().diagnostics.end(),
                        "TOPOLOGY_HISTORY_UNSUPPORTED_GENERATOR:REVOLVE"),
              evaluation.feature_results.front().diagnostics.end());
}

TEST(GeometryExchange, NamingFixtureCutRetainsModifiedTopFace) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-1";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec hole;
    hole.feature_id = "cut-1";
    hole.body_id = "body-main";
    hole.input_feature_id = base.feature_id;
    hole.profile_feature_id = "sketch-hole";
    hole.regions = {rectangular_region("hole", 5, 5, 15, 15)};
    hole.pad_length = 10;
    hole.body_operation = "REMOVE";

    const auto result = kernel.evaluateProfilePads({base, hole});
    EXPECT_EQ(kernel.getTopology(result).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(result), 3000.0, 1.0e-6);
    EXPECT_EQ(faces_on_z(kernel.getTopology(result), 10.0), 1U);
}

TEST(GeometryExchange, NamingFixtureCutDeletesOriginalTopFace) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec remove_top;
    remove_top.regions = {rectangular_region("top-half", 0, 0, 20, 20)};
    remove_top.pad_length = 5;
    remove_top.body_operation = "REMOVE";
    remove_top.plane_origin = {0, 0, 5};
    remove_top.plane_normal = {0, 0, 1};
    remove_top.plane_u_direction = {1, 0, 0};

    base.feature_id = "extrude-1";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    remove_top.feature_id = "cut-top";
    remove_top.body_id = "body-main";
    remove_top.input_feature_id = base.feature_id;
    remove_top.profile_feature_id = "sketch-cut";
    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, remove_top});
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(evaluation.geometry_id), 2000.0, 1.0e-6);
    EXPECT_EQ(faces_on_z(kernel.getTopology(evaluation.geometry_id), 10.0), 0U);
    EXPECT_EQ(faces_on_z(kernel.getTopology(evaluation.geometry_id), 5.0), 1U);
    const auto& deleted = evaluation.feature_results.back().topology_history.deleted;
    EXPECT_TRUE(std::any_of(deleted.begin(), deleted.end(), [](const auto& tombstone) {
        return tombstone.source.output_slot.rfind("END_VERTEX_FROM_PROFILE_ENDPOINTS/", 0) == 0;
    }));
}

TEST(GeometryExchange, NamingFixtureSideOpeningCreatesTwoAmbiguousFaceCandidates) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-1";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec notch;
    notch.feature_id = "cut-1";
    notch.body_id = "body-main";
    notch.input_feature_id = base.feature_id;
    notch.profile_feature_id = "sketch-notch";
    notch.regions = {rectangular_region("notch", 8, 0, 12, 5)};
    notch.pad_length = 10;
    notch.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, notch});
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).solid_count, 1U);
    EXPECT_EQ(faces_on_y(kernel.getTopology(evaluation.geometry_id), 0.0), 2U);
    ASSERT_EQ(evaluation.feature_results.size(), 2U);
    const auto& cut = evaluation.feature_results.back();
    EXPECT_TRUE(cut.topology_history_complete);
    const auto ambiguity = std::find_if(
        cut.topology_history.ambiguous.begin(), cut.topology_history.ambiguous.end(),
        [](const AmbiguousLineage& value) {
            return value.sources.size() == 1U &&
                   value.sources.front().output_slot ==
                       "SIDE_FROM_PROFILE_EDGE/base-edge-0";
        });
    ASSERT_NE(ambiguity, cut.topology_history.ambiguous.end());
    EXPECT_EQ(ambiguity->diagnostic_code, "TOPOLOGY_SPLIT_AMBIGUOUS");
    EXPECT_EQ(ambiguity->candidates.size(), 2U);
    EXPECT_TRUE(std::any_of(cut.topology_history.ambiguous.begin(),
                            cut.topology_history.ambiguous.end(), [](const auto& value) {
                                return !value.sources.empty() &&
                                       (value.sources.front().output_slot.find("BOUNDARY_") !=
                                            std::string::npos ||
                                        value.sources.front().output_slot.find("VERTICAL_") !=
                                            std::string::npos);
                            }));
}

TEST(GeometryExchange, NamingFixtureXZThroughCutKeepsSixBaseFacesAndAddsFourHoleFaces) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-base";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";

    ProfilePadSpec hole;
    hole.feature_id = "cut-hole";
    hole.body_id = "body-main";
    hole.input_feature_id = base.feature_id;
    hole.profile_feature_id = "sketch-hole-xz";
    hole.regions = {rectangular_region("hole", 5, 2, 15, 8)};
    hole.pad_length = 20;
    hole.plane = "XZ";
    hole.reversed = true;
    hole.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, hole});
    ASSERT_EQ(evaluation.feature_results.size(), 2U);
    const auto& result = evaluation.feature_results.back();
    EXPECT_TRUE(result.topology_history_complete);
    EXPECT_TRUE(result.diagnostics.empty());
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).face_count, 10U);
    const auto& topology = kernel.getTopology(evaluation.geometry_id);
    ASSERT_EQ(result.semantic_outputs.size(), topology.face_count + topology.edge_count +
                                                  topology.vertex_count);
    std::vector<std::uint64_t> local_ids;
    std::size_t base_faces = 0;
    std::size_t hole_faces = 0;
    std::size_t named_edges = 0;
    std::size_t named_vertices = 0;
    for (const auto& output : result.semantic_outputs) {
        local_ids.push_back((static_cast<std::uint64_t>(output.topology_type) << 56U) |
                            output.local_id);
        if (output.topology_type == PersistentTopologyType::face) {
            base_faces += output.semantic_ref.feature_id == base.feature_id ? 1U : 0U;
            hole_faces += output.semantic_ref.feature_id == hole.feature_id ? 1U : 0U;
        } else if (output.topology_type == PersistentTopologyType::edge) {
            ++named_edges;
        } else if (output.topology_type == PersistentTopologyType::vertex) {
            ++named_vertices;
        }
    }
    std::sort(local_ids.begin(), local_ids.end());
    EXPECT_EQ(std::adjacent_find(local_ids.begin(), local_ids.end()), local_ids.end());
    EXPECT_EQ(base_faces, 6U);
    EXPECT_EQ(hole_faces, 4U);
    EXPECT_EQ(named_edges, topology.edge_count);
    EXPECT_EQ(named_vertices, topology.vertex_count);
    EXPECT_TRUE(std::any_of(result.semantic_outputs.begin(), result.semantic_outputs.end(),
                            [&](const auto& output) {
                                return output.topology_type == PersistentTopologyType::edge &&
                                       output.semantic_ref.feature_id == hole.feature_id;
                            }));
    EXPECT_TRUE(std::any_of(result.semantic_outputs.begin(), result.semantic_outputs.end(),
                            [&](const auto& output) {
                                return output.topology_type == PersistentTopologyType::vertex &&
                                       output.semantic_ref.feature_id == hole.feature_id;
                            }));
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
    // Equal-height overlapping pads form one 30 x 20 x 10 box.  A raw OCCT
    // Fuse has the correct volume but retains section edges and exposes the top
    // as three faces; a Body result must merge those same-domain regions.
    EXPECT_EQ(kernel.getTopology(fused).face_count, 6U);
    EXPECT_EQ(kernel.getTopology(fused).edge_count, 12U);
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
