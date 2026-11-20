#include "../src/naming_history.hpp"
#include "../src/part_continuation.hpp"
#include "naming_legacy_reference.hpp"
#include "../src/cancellation.hpp"

#include <gtest/gtest.h>

#include <cstdlib>
#include <fstream>
#include <occccad/kernel/geometry_id.hpp>

namespace api = occccad::worker::v1;

namespace {
api::SemanticTopologyRef stored_ref(const api::PartTopologyManifest& values,uint32_t id) {return values.semantic_refs(id-1);}
api::SelectionEvidence stored_evidence(const api::PartTopologyManifest& values,uint32_t id,
                                       const api::TopologyHistory* predecessor = nullptr) {
    api::SelectionEvidence out;
    auto evidence = values.evidence(id-1);
    if (predecessor) evidence.clear_adjacent();
    occccad::worker::naming_detail::encode_evidence(occccad::worker::read_naming_evidence(evidence,values),&out);
    if (predecessor) {
        for (const auto encoded : values.evidence(id-1).adjacent()) {
            const auto index = encoded >> 1;
            *out.add_adjacent() = encoded & 1 ? predecessor->lineage(index-1).result()
                                             : stored_ref(values, index);
        }
    }
    return out;
}
std::vector<api::FeatureResult> expanded_history(const occccad::worker::EncodedNamingHistory& encoded) {
    std::vector<api::FeatureResult> result;
    for(const auto& payload:encoded.blocks) {
        api::NamingHistoryBlock block;if(!block.ParseFromArray(payload->bytes.data(),static_cast<int>(payload->bytes.size())))throw std::runtime_error("invalid test block");
        occccad::worker::expand_naming_references(*block.mutable_values());
        const auto& values=block.values();const auto& source=values.transitions(0);
        const auto* previous = block.schema_version() == 2 ? &result.back().topology_history() : nullptr;
        api::FeatureResult feature;feature.set_feature_id(source.feature_id());feature.set_body_id(source.body_id());
        if(source.has_result())*feature.mutable_result()=source.result();
        feature.set_input_feature_id(source.input_feature_id());feature.set_profile_feature_id(source.profile_feature_id());feature.set_result_geometry_id(source.result_geometry_id());feature.set_topology_history_complete(source.complete());
        for(const auto& diagnostic:source.diagnostics())feature.add_diagnostics(diagnostic);
        auto* history=feature.mutable_topology_history();history->set_input_geometry_id(source.input_geometry_id());history->set_evidence_digest(source.evidence_digest());
        for(const auto& span:block.layout())for(uint64_t i=0;i<span.count();++i) {
            auto* row=history->add_lineage();
            if(span.carry()) {
                const auto& old=result.back().topology_history().lineage(static_cast<int>(span.offset()+i));
                *row->mutable_result()=old.result();*row->add_sources()=old.result();*row->mutable_evidence()=old.evidence();row->set_kind(api::TOPOLOGY_LINEAGE_UNCHANGED);
            } else {
                const auto& old=source.lineage(static_cast<int>(span.offset()+i));
                *row->mutable_result()=stored_ref(values,old.result());for(auto ref:old.sources())*row->add_sources()=stored_ref(values,ref);
                *row->mutable_evidence()=stored_evidence(values,old.evidence(),previous);row->set_kind(old.kind());
            }
        }
        for(const auto& d:source.deleted()){auto* row=history->add_deleted();*row->mutable_source()=stored_ref(values,d.source());*row->mutable_evidence()=stored_evidence(values,d.evidence(),previous);row->set_reason(d.reason());}
        for(const auto& a:source.ambiguous()){auto* row=history->add_ambiguous();for(auto ref:a.sources())*row->add_sources()=stored_ref(values,ref);for(auto ref:a.candidates())*row->add_candidates()=stored_ref(values,ref);row->set_diagnostic_code(a.diagnostic_code());}
        result.push_back(std::move(feature));
    }
    auto root=encoded.root;
    occccad::worker::expand_naming_references(root);
    const auto& tip=root.bodies(0).tip();
    for(const auto& binding:tip){auto* row=result.back().add_semantic_outputs();*row->mutable_semantic_ref()=stored_ref(root,binding.semantic_ref());*row->mutable_evidence()=stored_evidence(root,binding.evidence());row->set_local_id(binding.local_id());row->set_topology_type(binding.topology_type());}
    return result;
}
}

TEST(NamingArtifact, CheckpointKeepsTipAndLineagePermutations) {
    namespace native = occccad::kernel;
    native::FeatureResult f;
    f.body_id = "body";
    f.feature_id = "base";
    f.result_geometry_id = "sha256:geometry";
    f.topology_history_complete = true;
    for (uint64_t i = 1; i <= 3; ++i) {
        native::SemanticTopologyRef ref{"base", "face/" + std::to_string(i), {"source"}};
        native::SelectionEvidence evidence;
        evidence.geometry_type = "PLANE";
        evidence.adjacent = {ref};
        f.semantic_outputs.push_back({ref, native::PersistentTopologyType::face, i, evidence});
        f.topology_history.lineage.push_back({{}, ref, native::TopologyLineageKind::generated, evidence});
    }
    std::reverse(f.semantic_outputs.begin(), f.semantic_outputs.end());
    native::FeatureResultSequence features;
    features.push_back(std::move(f));
    api::TopologyNamingPolicy policy;
    policy.set_policy_id("policy");
    policy.set_policy_digest("digest");
    policy.set_evaluator_version("evaluator");
    auto snapshot = occccad::worker::NamingHistoryEncoder().encode(features, policy, "sha256:geometry", "geometry").root;
    api::PartContinuation c;
    c.set_feature_id("base");
    c.set_body_id("body");
    c.set_stage_count(1);
    c.set_validation_policy("occccad.geometry.brep-valid.v1");
    auto broken = snapshot;
    broken.set_tip_lineage_locators(1, broken.tip_lineage_locators(0));
    EXPECT_THROW(occccad::worker::restore_part_continuation(broken, c, "sha256:geometry", policy), std::invalid_argument);
    auto restored = occccad::worker::restore_part_continuation(snapshot, c, "sha256:geometry", policy);
    ASSERT_EQ(restored->tip_lineage_locators(), (std::vector<uint32_t>{3, 2, 1}));
    uint64_t ordinal = 0;
    for (const auto output : restored->outputs()) {
        EXPECT_EQ(output.local_id, 3 - ordinal++);
        EXPECT_EQ(output.semantic_ref.output_slot, "face/" + std::to_string(output.local_id));
        EXPECT_EQ(&output.evidence, &restored->lineage()[output.local_id - 1].evidence);
        ASSERT_EQ(output.evidence.adjacent.size(), 1U);
        EXPECT_EQ(output.semantic_ref.output_slot, output.evidence.adjacent.front().output_slot);
    }
}

TEST(NamingArtifact, HistoryBlocksKeepLegacyBytesAndSurviveEvictionAndColdRebuild) {
    namespace native=occccad::kernel;
    native::FeatureResult source;source.feature_id="base";source.body_id="body";source.topology_history_complete=true;
    for(int i=0;i<30;++i){
        native::SemanticTopologyRef ref{"base","face/"+std::to_string(i),{"a/b","a\nb"}};
        native::SelectionEvidence evidence;evidence.geometry_type=i%2?"PLANE":"CIRCLE";evidence.centroid={double(i),2,3};evidence.direction={0,0,1};evidence.parameter_start=-0.;evidence.parameter_end=2.7;evidence.radius_mm=4;evidence.adjacent={ref};evidence.evidence_digest="evidence";
        source.topology_history.lineage.push_back({{},ref,native::TopologyLineageKind::generated,evidence});
        source.semantic_outputs.push_back({ref,native::PersistentTopologyType::face,uint64_t(i+1),evidence});
    }
    native::FeatureResultSequence features;
    api::TopologyNamingPolicy policy;policy.set_policy_id("policy");policy.set_policy_digest("policy-digest");policy.set_evaluator_version("evaluator");
    occccad::worker::NamingHistoryEncoder encoder;
    occccad::worker::EncodedNamingHistory encoded;
    for(int step=0;step<4;++step){
        source.feature_id="step-"+std::to_string(step);source.result_geometry_id="geometry-"+std::to_string(step);source.topology_history.evidence_digest="history-"+std::to_string(step);
        if(step)for(auto& row:source.topology_history.lineage){row.kind=native::TopologyLineageKind::unchanged;row.sources={row.result};}
        if(step==2){source.topology_history.lineage[0].evidence.radius_mm=5;source.semantic_outputs[0].evidence.radius_mm=5;}
        features.push_back(source);
        encoded=encoder.encode(features,policy,source.result_geometry_id,"brep");
        EXPECT_EQ(encoded.computed,1U);EXPECT_EQ(encoded.reused,static_cast<unsigned>(step));
        auto restored=expanded_history(encoded);
        api::PartTopologyManifest expected,actual;
        occccad::worker::legacy_test_reference::pack_naming(features.size(),[&](size_t i)->const native::FeatureResult&{return features[i];},expected);
        occccad::worker::legacy_test_reference::pack_naming(restored,actual);
        EXPECT_EQ(expected.SerializeAsString(),actual.SerializeAsString());
    }
    EXPECT_GT(encoded.blocks.back()->carries,0U);
    api::NamingHistoryBlock changed;
    ASSERT_TRUE(changed.ParseFromArray(encoded.blocks[2]->bytes.data(), static_cast<int>(encoded.blocks[2]->bytes.size())));
    EXPECT_EQ(changed.schema_version(), 2U);
    EXPECT_EQ(changed.values().evidence(0).adjacent(0), 3U);
    // The full prefix does not fit this memo. Rebuilding missing early blocks
    // must not evict the tail before the same request can reuse it.
    occccad::worker::NamingHistoryEncoder bounded(1024);
    const auto first_bounded=bounded.encode(features,policy,source.result_geometry_id,"brep");
    ASSERT_GT(bounded.bytes(),0U);ASSERT_LE(bounded.bytes(),1024U);
    const auto replay_bounded=bounded.encode(features,policy,source.result_geometry_id,"brep");
    EXPECT_GT(replay_bounded.reused,0U);
    EXPECT_EQ(replay_bounded.computed+replay_bounded.reused,4U);
    EXPECT_EQ(first_bounded.root.SerializeAsString(),replay_bounded.root.SerializeAsString());
    for(size_t i=0;i<encoded.blocks.size();++i)EXPECT_EQ(encoded.blocks[i]->bytes,replay_bounded.blocks[i]->bytes);
    const auto root_bytes=encoded.root.SerializeAsString();encoder.clear();EXPECT_EQ(encoder.bytes(),0U);
    const auto cold=encoder.encode(features,policy,source.result_geometry_id,"brep",true);
    EXPECT_EQ(cold.root.SerializeAsString(),root_bytes);EXPECT_EQ(cold.computed,4U);EXPECT_EQ(cold.reused,0U);
    for(size_t i=0;i<cold.blocks.size();++i)EXPECT_EQ(cold.blocks[i]->bytes,encoded.blocks[i]->bytes);
    EXPECT_THROW(encoder.encode(features,policy,source.result_geometry_id,"brep",false,[]{return true;}),std::runtime_error);
}

TEST(NamingArtifact, NativeImmutableRecordsRetainExactWireInterningAndOwners) {
    namespace native = occccad::kernel;
    namespace detail = occccad::worker::naming_detail;
    native::FeatureResult first;
    first.feature_id="first";first.body_id="body";first.result_geometry_id="geometry-first";
    first.topology_history_complete=true;first.diagnostics={"diagnostic"};
    first.topology_history.input_geometry_id="input";first.topology_history.evidence_digest="history";
    native::SelectionEvidence seed;
    seed.geometry_type="LINE";seed.evidence_digest="same-digest";seed.origin={1,2,3};seed.direction={0,0,1};
    seed.centroid={4,5,6};seed.parameter_start=-0.0;seed.parameter_end=2.75;seed.radius_mm=3;
    seed.minor_radius_mm=2;seed.half_angle_radians=.3;seed.cone_leaf=-1;seed.material_side=-1;
    seed.x_direction=native::Vec3{1,0,0};seed.measure_si=.001;seed.measure_dimension="LENGTH";seed.endpoint_role="START";
    seed.adjacent={{"adjacent","edge",{"a/b","a\nb"}}};
    std::vector<native::SelectionEvidence> variants(12,seed);
    // Same digest is insufficient for equality. Optional presence, bitwise
    // doubles, ordered adjacency and every geometry family remain significant.
    variants[2].parameter_start=0.0;variants[3].radius_mm.reset();
    variants[4].adjacent[0].source_ids={"a","b\na/b"};variants[5].geometry_type="POINT";
    variants[6]=variants[5];variants[6].direction={9,8,7}; // ignored by compact POINT: wire fallback must still intern
    variants[7].geometry_type="PLANE";variants[8]=variants[7];variants[8].parameter_start=42;
    variants[9].geometry_type="CYLINDER";variants[10].geometry_type="BSPLINE";variants[11].geometry_type="TORUS";
    for(size_t i=0;i<variants.size();++i){
        native::SemanticTopologyOutput output;
        output.semantic_ref={first.feature_id,"face/"+std::to_string(i),{"a/b","a\nb"}};
        output.local_id=i+1;output.topology_type=native::PersistentTopologyType::face;output.evidence=variants[i];
        first.semantic_outputs.push_back(output);
        first.topology_history.lineage.push_back({{},output.semantic_ref,native::TopologyLineageKind::generated,output.evidence});
    }
    auto last=first;last.feature_id="last";last.input_feature_id=first.feature_id;last.profile_feature_id="profile";
    last.result_geometry_id="geometry-last";
    for(auto& lineage:last.topology_history.lineage){lineage.sources={lineage.result};lineage.kind=native::TopologyLineageKind::unchanged;}
    last.topology_history.deleted.push_back({first.semantic_outputs[0].semantic_ref,"removed",seed});
    last.topology_history.ambiguous.push_back({{first.semantic_outputs[1].semantic_ref},{first.semantic_outputs[2].semantic_ref},"ambiguous"});
    for(bool geometric:{false,true}){
        if(geometric){first.body_id.clear();last.body_id.clear();first.result_id="result";last.result_id="result";last.feature_id=first.feature_id;}
        native::ImmutableSequence<native::FeatureResult> records;records.push_back(first);records.push_back(last);
        std::vector<api::FeatureResult> converted;
        for(const auto& source:records){
            auto& target=converted.emplace_back();target.set_feature_id(source.feature_id);target.set_body_id(source.body_id);
            if(geometric){target.mutable_result()->set_feature_id(source.feature_id);target.mutable_result()->set_result_id(source.result_id);}
            target.set_input_feature_id(source.input_feature_id);target.set_profile_feature_id(source.profile_feature_id);
            target.set_result_geometry_id(source.result_geometry_id);target.set_topology_history_complete(source.topology_history_complete);
            for(const auto& diagnostic:source.diagnostics)target.add_diagnostics(diagnostic);
            auto* history=target.mutable_topology_history();history->set_input_geometry_id(source.topology_history.input_geometry_id);history->set_evidence_digest(source.topology_history.evidence_digest);
            for(const auto& lineage:source.topology_history.lineage){
                auto* row=history->add_lineage();for(const auto& ref:lineage.sources)detail::encode_ref(ref,row->add_sources());
                detail::encode_ref(lineage.result,row->mutable_result());row->set_kind(detail::encoded_kind(lineage.kind));detail::encode_evidence(lineage.evidence,row->mutable_evidence());
            }
            for(const auto& deleted:source.topology_history.deleted){auto* row=history->add_deleted();detail::encode_ref(deleted.source,row->mutable_source());row->set_reason(deleted.reason);detail::encode_evidence(deleted.evidence,row->mutable_evidence());}
            for(const auto& ambiguous:source.topology_history.ambiguous){auto* row=history->add_ambiguous();for(const auto& ref:ambiguous.sources)detail::encode_ref(ref,row->add_sources());for(const auto& ref:ambiguous.candidates)detail::encode_ref(ref,row->add_candidates());row->set_diagnostic_code(ambiguous.diagnostic_code);}
            for(const auto& output:source.semantic_outputs){auto* row=target.add_semantic_outputs();row->set_local_id(output.local_id);row->set_topology_type(detail::encoded_type(output.topology_type));detail::encode_ref(output.semantic_ref,row->mutable_semantic_ref());detail::encode_evidence(output.evidence,row->mutable_evidence());}
        }
        api::PartTopologyManifest expected,actual;
        occccad::worker::pack_naming(converted,expected);
        occccad::worker::pack_naming(records.size(),[&](size_t i)->const native::FeatureResult&{return records[i];},actual);
        EXPECT_EQ(actual.SerializeAsString(),expected.SerializeAsString());
        EXPECT_LT(actual.evidence_size(),variants.size());
        if(geometric){EXPECT_EQ(actual.bodies_size(),0);ASSERT_EQ(actual.results_size(),1);}
        else {ASSERT_EQ(actual.bodies_size(),1);EXPECT_EQ(actual.results_size(),0);}
    }
}

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
    auto legacy_projection=manifest;legacy_projection.set_schema_version(2);
    EXPECT_EQ(occccad::kernel::make_geometry_id(legacy_projection.SerializeAsString()),
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
    EXPECT_EQ(packed.schema_version(), 3);
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
