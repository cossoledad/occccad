#include "../src/naming_artifact.hpp"
#include "../src/cancellation.hpp"

#include <gtest/gtest.h>

#include <cstdlib>
#include <fstream>
#include <occccad/kernel/geometry_id.hpp>

namespace api = occccad::worker::v1;

TEST(NamingArtifact, StoredLayoutRetainsTypedEvidenceAndFirstOccurrenceOrder) {
    const std::vector<std::string> kinds{"PLANE", "CYLINDER", "POINT", "LINE", "CIRCLE", "BSPLINE", "BEZIER", "ELLIPSE", "TORUS"};
    std::vector<api::FeatureResult> features(3);
    for(size_t i=0;i<features.size();++i){
        auto& feature=features[i];feature.set_feature_id("feature-"+std::to_string(i));feature.set_body_id("body");
        feature.set_input_feature_id(i?features[i-1].feature_id():"");feature.set_profile_feature_id("profile");
        feature.set_result_geometry_id("geometry-"+std::to_string(i));feature.set_topology_history_complete(true);
        feature.add_diagnostics("frozen diagnostic");auto* history=feature.mutable_topology_history();
        history->set_input_geometry_id(i?features[i-1].result_geometry_id():"");history->set_evidence_digest("history-"+std::to_string(i));
        for(size_t j=0;j<kinds.size();++j){
            auto* output=feature.add_semantic_outputs();output->set_local_id(j+1);output->set_topology_type(api::PERSISTENT_TOPOLOGY_TYPE_FACE);
            auto* ref=output->mutable_semantic_ref();ref->set_feature_id(feature.feature_id());ref->set_output_slot(kinds[j]);
            ref->add_source_ids("a/b");ref->add_source_ids("a\nb");
            auto* evidence=output->mutable_evidence();evidence->set_geometry_type(kinds[j]);evidence->set_evidence_digest("evidence-"+std::to_string(j));
            evidence->set_measure_si(.001);evidence->set_measure_dimension("AREA");evidence->set_parameter_start(-0.0);evidence->set_parameter_end(2.75);
            evidence->set_endpoint_role("START");evidence->set_radius_mm(3);evidence->set_minor_radius_mm(2);evidence->set_half_angle_radians(.3);
            evidence->set_cone_leaf(-1);evidence->set_material_side(-1);evidence->mutable_x_direction()->set_x(1);
            evidence->mutable_origin()->set_x(12);evidence->mutable_direction()->set_z(1);evidence->mutable_centroid()->set_y(4);
            *evidence->add_adjacent()=*ref;
            auto* lineage=history->add_lineage();*lineage->mutable_result()=*ref;*lineage->mutable_evidence()=*evidence;
            lineage->set_kind(i?api::TOPOLOGY_LINEAGE_MODIFIED:api::TOPOLOGY_LINEAGE_GENERATED);
            if(i)*lineage->add_sources()=features[i-1].semantic_outputs(j).semantic_ref();
        }
        auto* deleted=history->add_deleted();*deleted->mutable_source()=feature.semantic_outputs(0).semantic_ref();
        deleted->set_reason("removed");*deleted->mutable_evidence()=feature.semantic_outputs(0).evidence();
        auto* ambiguous=history->add_ambiguous();*ambiguous->add_sources()=feature.semantic_outputs(1).semantic_ref();
        *ambiguous->add_candidates()=feature.semantic_outputs(2).semantic_ref();ambiguous->set_diagnostic_code("candidate");
    }
    api::PartTopologyManifest manifest;manifest.set_policy_digest("policy");manifest.set_geometry_id("geometry");manifest.set_brep_sha256("brep");
    occccad::worker::pack_naming(features,manifest);
    ASSERT_EQ(manifest.transitions_size(),3);ASSERT_EQ(manifest.bodies(0).tip_size(),kinds.size());
    // Frozen with the original vector/tree-map encoder before the streaming
    // implementation: this checks the stored layout, not just two new paths.
    EXPECT_EQ(occccad::kernel::make_geometry_id(manifest.SerializeAsString()),
              "sha256:0bb66a9d6524445feea778e8e15c03e42ba5e1b8d9fd6c336069c0d8186f0791");
    api::PartTopologyManifest streamed;streamed.set_policy_digest("policy");streamed.set_geometry_id("geometry");streamed.set_brep_sha256("brep");
    api::FeatureResult scratch;
    occccad::worker::pack_naming(features.size(),[&](size_t i)->const api::FeatureResult&{scratch=features[i];return scratch;},streamed);
    EXPECT_EQ(streamed.SerializeAsString(),manifest.SerializeAsString());
}
TEST(WorkerCancellation, PollingIsBoundedAndAdmissionIsFresh) {
    int calls=0;int64_t now=0;bool cancelled=false;
    occccad::worker::CancellationPolling token([&](){++calls;return cancelled;},[&](){return now;});
    for(int i=0;i<10000;i++)EXPECT_FALSE(token.poll());
    EXPECT_EQ(calls,1);cancelled=true;EXPECT_FALSE(token.poll());
    EXPECT_TRUE(token.check());EXPECT_EQ(calls,2);
    for(int i=0;i<10000;i++)EXPECT_TRUE(token.poll());
    EXPECT_EQ(calls,2);
    occccad::worker::CancellationPolling later([&](){++calls;return cancelled;},[&](){return now;});
    cancelled=false;EXPECT_FALSE(later.poll());cancelled=true;now+=16000;EXPECT_TRUE(later.poll());
}
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
    for(size_t i=0;i<kinds.size();++i) {
        const auto restored=occccad::worker::read_naming_evidence(m.evidence(static_cast<int>(i)),m);
        EXPECT_EQ(restored.geometry_type,kinds[i]);EXPECT_DOUBLE_EQ(restored.origin.x,12);
        EXPECT_DOUBLE_EQ(restored.centroid.x,12);
        if(i>=2&&kinds[i]!="POINT") {ASSERT_TRUE(restored.parameter_start);ASSERT_TRUE(restored.parameter_end);EXPECT_DOUBLE_EQ(*restored.parameter_start,.25);EXPECT_DOUBLE_EQ(*restored.parameter_end,2.75);}
    }
}

TEST(NamingArtifact, RejectsCrossBodyArtifact) {
 std::vector<api::FeatureResult> features(2);features[0].set_body_id("a");features[1].set_body_id("b");
 api::PartTopologyManifest out;
 EXPECT_THROW(occccad::worker::pack_naming(features,out),std::invalid_argument);
}

TEST(NamingArtifact, IndependentFeatureResultsNeverAcquireBodyIdentity) {
    api::FeatureResult f;f.set_feature_id("curve");f.mutable_result()->set_feature_id("curve");f.mutable_result()->set_result_id("result");f.set_topology_history_complete(true);
    auto* o=f.add_semantic_outputs();o->set_local_id(1);o->set_topology_type(api::PERSISTENT_TOPOLOGY_TYPE_EDGE);o->mutable_semantic_ref()->set_feature_id("curve");o->mutable_semantic_ref()->set_output_slot("curve");o->mutable_evidence()->set_geometry_type("LINE");
    api::PartTopologyManifest packed;occccad::worker::pack_naming({f},packed);EXPECT_EQ(packed.bodies_size(),0);ASSERT_EQ(packed.results_size(),1);EXPECT_EQ(packed.results(0).reference().feature_id(),"curve");EXPECT_EQ(packed.results(0).tip_size(),1);EXPECT_TRUE(packed.transitions(0).body_id().empty());EXPECT_EQ(packed.transitions(0).result().result_id(),"result");
    f.set_body_id("body");api::PartTopologyManifest ambiguous;EXPECT_THROW(occccad::worker::pack_naming({f},ambiguous),std::invalid_argument);
}
