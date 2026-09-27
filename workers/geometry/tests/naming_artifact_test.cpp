#include "../src/naming_artifact.hpp"

#include <gtest/gtest.h>

#include <cstdlib>
#include <fstream>

namespace api = occccad::worker::v1;
TEST(NamingArtifact, InternsEvidenceAndKeepsExplicitBodyTips) {
    api::SelectionEvidence evidence;
    evidence.set_geometry_type("PLANE");
    evidence.set_measure_dimension("AREA");
    evidence.set_measure_si(0.001);
    evidence.set_evidence_digest("evidence");
    evidence.mutable_direction()->set_z(1);
    evidence.mutable_adjacent()->Add()->set_feature_id("adjacency-feature");
    evidence.mutable_adjacent(0)->set_output_slot("edge");
    std::vector<api::FeatureResult> features(2);
    for (size_t i = 0; i < features.size(); ++i) {
        auto& f = features[i];
        f.set_body_id("body-a");
        f.set_feature_id("feature-" + std::to_string(i));
        f.set_result_geometry_id("geometry");
        f.set_topology_history_complete(true);
        auto* o = f.add_semantic_outputs();
        o->set_local_id(i + 1);
        o->set_topology_type(api::PERSISTENT_TOPOLOGY_TYPE_FACE);
        o->mutable_semantic_ref()->set_feature_id(f.feature_id());
        o->mutable_semantic_ref()->set_output_slot("face");
        *o->mutable_evidence() = evidence;
        auto* l = f.mutable_topology_history()->add_lineage();
        *l->mutable_result() = o->semantic_ref();
        *l->mutable_evidence() = evidence;
        l->set_kind(api::TOPOLOGY_LINEAGE_GENERATED);
        if (i == 1) {
            *l->add_sources() = features[0].semantic_outputs(0).semantic_ref();
            l->set_kind(api::TOPOLOGY_LINEAGE_MODIFIED);
        }
    }
    api::PartTopologyManifest packed;
    packed.set_geometry_id("geometry");
    occccad::worker::pack_naming(features, packed);
    EXPECT_EQ(packed.schema_version(), 2);
    EXPECT_EQ(packed.evidence_size(), 1);
    ASSERT_EQ(packed.bodies_size(), 1);
    EXPECT_EQ(packed.bodies(0).tip_transition(), 2);
    EXPECT_EQ(packed.bodies(0).tip_size(), 1);
    EXPECT_EQ(packed.bodies(0).tip(0).local_id(), 2);
    EXPECT_EQ(packed.semantic_refs_size(), 3);  // 2 feature identities + shared adjacency
    for (const auto& t : packed.transitions())
        EXPECT_EQ(t.lineage(0).evidence(), 1);
    api::PartTopologyManifest decoded;
    ASSERT_TRUE(decoded.ParseFromString(packed.SerializeAsString()));
    EXPECT_EQ(decoded.SerializeAsString(), packed.SerializeAsString());
    if (const auto* path = std::getenv("OCCCCAD_TEST_NAMING_FIXTURE")) {
        std::ofstream file(path, std::ios::binary);
        const auto bytes = packed.SerializeAsString();
        file.write(bytes.data(), bytes.size());
        ASSERT_TRUE(file.good());
    }
}

TEST(NamingArtifact, TypedEvidenceAndExplicitCurveParameters) {
    api::FeatureResult feature;
    feature.set_feature_id("feature");
    feature.set_body_id("body");
    const std::vector<std::string> kinds{"PLANE", "CYLINDER", "LINE",   "CIRCLE",
                                         "POINT", "BSPLINE",  "BEZIER", "ELLIPSE"};
    for (size_t i = 0; i < kinds.size(); ++i) {
        auto* output = feature.add_semantic_outputs();
        output->set_local_id(i + 1);
        output->set_topology_type(api::PERSISTENT_TOPOLOGY_TYPE_EDGE);
        output->mutable_semantic_ref()->set_feature_id("feature");
        output->mutable_semantic_ref()->set_output_slot(kinds[i]);
        auto* e = output->mutable_evidence();
        e->set_geometry_type(kinds[i]);
        e->mutable_origin()->set_x(12);
        e->mutable_centroid()->set_x(12);
        if (kinds[i] != "POINT")
            e->mutable_direction()->set_z(1);
        if (i >= 2 && kinds[i] != "POINT") {
            e->set_parameter_start(0.25);
            e->set_parameter_end(2.75);
        }
    }
    api::PartTopologyManifest m;
    occccad::worker::pack_naming({feature}, m);
    EXPECT_EQ(m.coordinate_space(), "PART_LOCAL");
    EXPECT_EQ(m.length_unit(), "mm");
    ASSERT_EQ(m.evidence_size(), 8);
    EXPECT_TRUE(m.evidence(0).has_plane());
    EXPECT_TRUE(m.evidence(1).has_cylinder());
    EXPECT_TRUE(m.evidence(4).has_point());
    EXPECT_EQ(m.evidence(2).line().range().convention(), "LINE_MM");
    EXPECT_DOUBLE_EQ(m.evidence(2).line().range().start(), 0.25);
    EXPECT_EQ(m.evidence(3).circle().range().convention(), "ANGLE_RADIANS");
    EXPECT_EQ(m.evidence(5).spline().range().convention(), "NATIVE_CURVE_PARAMETER");
    EXPECT_EQ(m.evidence(6).spline().family(), "BEZIER");
    EXPECT_EQ(m.evidence(7).other().range().convention(), "ANGLE_RADIANS");
}

TEST(NamingArtifact, RejectsCrossBodyArtifact) {
 std::vector<api::FeatureResult> features(2);features[0].set_body_id("a");features[1].set_body_id("b");
 api::PartTopologyManifest out;
 EXPECT_THROW(occccad::worker::pack_naming(features,out),std::invalid_argument);
}
