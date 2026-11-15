#include <internal/occt_kernel.hpp>
#include <internal/naming_digest.hpp>
#include <internal/retained_prefix_budget.hpp>
#include <set>
#include <sstream>
#include <occccad/kernel/geometry_id.hpp>
#include <gtest/gtest.h>

#include <chrono>
#include <cmath>
#include <cstdlib>
#include <iostream>
#include <sys/resource.h>

namespace {
using namespace occccad::kernel;

std::string legacy_history_digest(const TopologyHistory& history) {
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << "schema=" << history.schema_version << "|feature=" << history.feature_id
              << "|input=" << history.input_geometry_id << "|result="
              << history.result_geometry_id << "|policy=" << history.policy_digest;
    auto lineage = history.lineage;
    std::sort(lineage.begin(), lineage.end(), [](const auto& left, const auto& right) {
        return detail::canonical_ref_key(left.result) < detail::canonical_ref_key(right.result);
    });
    for (auto& item : lineage) {
        std::sort(item.sources.begin(),item.sources.end(),[](const auto& a,const auto& b){return detail::canonical_ref_key(a)<detail::canonical_ref_key(b);});
        canonical << "|lineage=" << static_cast<int>(item.kind) << ':' << detail::canonical_ref_key(item.result)
                  << ':' << item.evidence.evidence_digest;
        for (const auto& source : item.sources)
            canonical << ":source=" << detail::canonical_ref_key(source);
        auto adjacent = item.evidence.adjacent;
        std::sort(adjacent.begin(),adjacent.end(),[](const auto& a,const auto& b){return detail::canonical_ref_key(a)<detail::canonical_ref_key(b);});
        for (const auto& ref : adjacent)
            canonical << ":adjacent=" << detail::canonical_ref_key(ref);
    }
    auto deleted = history.deleted;
    std::sort(deleted.begin(), deleted.end(), [](const auto& left, const auto& right) {
        return detail::canonical_ref_key(left.source) < detail::canonical_ref_key(right.source);
    });
    for (const auto& item : deleted)
        canonical << "|deleted=" << detail::canonical_ref_key(item.source) << ':' << item.reason << ':'
                  << item.evidence.evidence_digest;
    auto ambiguous = history.ambiguous;
    std::sort(ambiguous.begin(), ambiguous.end(), [](const auto& left, const auto& right) {
        return (left.sources.empty() ? std::string{} : detail::canonical_ref_key(left.sources.front())) <
               (right.sources.empty() ? std::string{} : detail::canonical_ref_key(right.sources.front()));
    });
    for (auto& item : ambiguous) {
        std::sort(item.sources.begin(),item.sources.end(),[](const auto& a,const auto& b){return detail::canonical_ref_key(a)<detail::canonical_ref_key(b);});
        std::sort(item.candidates.begin(),item.candidates.end(),[](const auto& a,const auto& b){return detail::canonical_ref_key(a)<detail::canonical_ref_key(b);});
        canonical << "|ambiguous=" << item.diagnostic_code;
        for (const auto& source : item.sources)
            canonical << ":source=" << detail::canonical_ref_key(source);
        for (const auto& candidate : item.candidates)
            canonical << ":candidate=" << detail::canonical_ref_key(candidate);
    }
    return make_geometry_id(canonical.str());
}

TEST(GeometryIncrementalHistory, RetainedPrefixBudgetCountsSharedBranchesOnce) {
    using Prefix = detail::ImmutablePrefix<int>;
    detail::RetainedPrefixBudget<int> budget;
    std::shared_ptr<const Prefix> common;
    for (int i=0;i<100;++i) common=Prefix::append(common,i,16);
    auto first=Prefix::append(common,100,32),second=Prefix::append(common,101,64);
    std::vector<std::shared_ptr<const Prefix>> roots;
    const auto expected=[&](const auto& additional){
        std::set<const Prefix*> seen;
        for(const auto& root:roots)for(auto node=root.get();node;node=node->previous.get())seen.insert(node);
        const auto before=seen;
        for(auto node=additional.get();node;node=node->previous.get())seen.insert(node);
        size_t bytes=0,added=0;
        for(const auto* node:seen){const auto own=node->bytes-(node->previous?node->previous->bytes:0);bytes+=own;if(!before.count(node))added+=own;}
        EXPECT_EQ(budget.additional(additional),added);
        return bytes;
    };
    for(const auto& root:{first,second,common,first}) {
        const auto total=expected(root);budget.retain(root);roots.push_back(root);EXPECT_EQ(budget.bytes(),total);
    }
    while(!roots.empty()) {
        const auto root=roots.back();roots.pop_back();budget.release(root);
        EXPECT_EQ(budget.bytes(),expected(std::shared_ptr<const Prefix>{}));
    }
    EXPECT_EQ(budget.bytes(),0U);
    budget.retain(second);budget.clear();EXPECT_EQ(budget.bytes(),0U);
}


ProfilePadSpec growth_pad(int ordinal, bool base = false) {
    ProfilePadSpec spec;
    spec.feature_id = base ? "growth-base" : "growth-boss-" + std::to_string(ordinal);
    spec.body_id = "growth-body";
    spec.profile_feature_id = "sketch-" + spec.feature_id;
    spec.pad_length = base ? 3 : 7 + (ordinal % 9) * .1;
    const double x = base ? 0 : 2 + (ordinal % 12) * 4;
    const double y = base ? 0 : 2 + (ordinal / 12) * 4;
    const double width = base ? 56 : 1, height = base ? 72 : 1;
    ProfileRegionSpec region;
    region.id = "region-" + spec.feature_id;
    region.outer.id = "outer-" + spec.feature_id;
    if (!base && ordinal % 2) {
        ProfileCurveSpec curve;
        curve.entity_id = "circle-" + spec.feature_id;
        curve.kind = "CIRCLE";
        curve.center = {x + .5, y + .5};
        curve.radius = .45;
        region.outer.curves.push_back(std::move(curve));
    } else {
        const Vec2 points[]{{x,y},{x+width,y},{x+width,y+height},{x,y+height}};
        for (int edge = 0; edge < 4; ++edge) {
            ProfileCurveSpec curve;
            curve.entity_id = "edge-" + std::to_string(edge) + "-" + spec.feature_id;
            curve.kind = "LINE";
            curve.start = points[edge];
            curve.end = points[(edge+1)%4];
            region.outer.curves.push_back(std::move(curve));
        }
    }
    spec.regions.push_back(std::move(region));
    return spec;
}
}

// Default is a small deterministic correctness case. The explicit performance
// recipe uses OCCCCAD_TEST_NAMING_STEPS=120; every step adds distinct topology.
TEST(GeometryIncrementalHistory, GrowingTopologyKeepsCompleteHistoryAndColdResult) {
    int count = 12;
    if (const auto value = std::getenv("OCCCCAD_TEST_NAMING_STEPS")) count = std::stoi(value);
    ASSERT_GE(count, 1);
    ASSERT_LE(count, 200);
    OcctKernel kernel;
    PartRuntimeStats stats;
    PartRuntimeOptions options;
    options.stats = &stats;
    std::vector<ProfilePadSpec> specs{growth_pad(0,true)};
    options.stage_keys = {"naming-growth-v1/base"};
    auto result = kernel.evaluateProfilePadsWithHistory(specs,{},nullptr,options);
    double expected_volume = 56*72*3;
    for (int step = 1; step <= count; ++step) {
        auto spec = growth_pad(step);
        spec.input_feature_id = specs.back().feature_id;
        expected_volume += (step%2 ? std::acos(-1.)*.45*.45 : 1) * (spec.pad_length-3);
        specs.push_back(std::move(spec));
        options.stage_keys.push_back("naming-growth-v1/"+specs.back().feature_id);
        const auto start = std::chrono::steady_clock::now();
        result = kernel.evaluateProfilePadsWithHistory(specs,{},nullptr,options);
        const double elapsed = std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-start).count();
        ASSERT_EQ(result.feature_results.size(),static_cast<size_t>(step+1));
        EXPECT_NEAR(kernel.getVolume(result.geometry_id),expected_volume,1e-5);
        size_t outputs = 0, lineages = 0;
        for (const auto& feature : result.feature_results) {
            ASSERT_TRUE(feature.topology_history_complete);
            outputs += feature.semantic_outputs.size();
            lineages += feature.topology_history.lineage.size();
        }
        ASSERT_GE(result.feature_results.back().semantic_outputs.size(),26U+static_cast<size_t>(step)*6);
        EXPECT_LE(stats.stage_cache_bytes,128U*1024*1024);
        if (step == 1 || step == 20 || step == 50 || step == 100 || step == count) {
            rusage usage{};getrusage(RUSAGE_SELF,&usage);
            std::cout << "naming_growth steps=" << step << " elapsed_ms=" << elapsed
                      << " base_ms=" << stats.base_ms << " exact_ms=" << stats.exact_ms
                      << " naming_ms=" << stats.naming_ms << " stages_executed=" << stats.stages_executed
                      << " stages_reused=" << stats.stages_reused << " history_outputs=" << outputs
                      << " history_lineages=" << lineages << " tip_outputs=" << result.feature_results.back().semantic_outputs.size()
                      << " stage_bytes=" << stats.stage_cache_bytes << " peak_rss_bytes=" << usage.ru_maxrss*1024
                      << " geometry=" << result.geometry_id << " history_digest=" << result.feature_results.back().topology_history.evidence_digest << '\n';
        }
    }
    const auto accepted = result;
    // A changed tip is a new operation, not a result-cache hit.
    specs.back().pad_length += .25;
    options.stage_keys.back() += "/edit";
    const auto start = std::chrono::steady_clock::now();
    result = kernel.evaluateProfilePadsWithHistory(specs,{},nullptr,options);
    std::cout << "naming_growth_tip_edit steps=" << count
              << " elapsed_ms=" << std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-start).count()
              << " base_ms=" << stats.base_ms << " exact_ms=" << stats.exact_ms << " naming_ms=" << stats.naming_ms
              << " stages_executed=" << stats.stages_executed << " stages_reused=" << stats.stages_reused << '\n';
    EXPECT_EQ(stats.stages_executed,1U);
    EXPECT_EQ(stats.stages_reused,static_cast<size_t>(count));
    for (size_t i=0;i<static_cast<size_t>(count);++i) EXPECT_EQ(&accepted.feature_results[i],&result.feature_results[i]);
    kernel.clear_runtime_cache();
    EXPECT_EQ(kernel.runtime_cache_bytes(),0U);
    EXPECT_TRUE(accepted.feature_results.back().topology_history_complete);
    OcctKernel cold;
    options.force_cold = true;
    const auto rebuilt = cold.evaluateProfilePadsWithHistory(specs,{},nullptr,options);
    EXPECT_EQ(result.geometry_id,rebuilt.geometry_id);
    ASSERT_EQ(result.feature_results.size(),rebuilt.feature_results.size());
    for (size_t i = 0; i < result.feature_results.size(); ++i) {
        EXPECT_EQ(result.feature_results[i].topology_history.evidence_digest,rebuilt.feature_results[i].topology_history.evidence_digest);
        EXPECT_EQ(result.feature_results[i].semantic_outputs.size(),rebuilt.feature_results[i].semantic_outputs.size());
    }
}

TEST(GeometryIncrementalHistory, CanonicalDigestRetainsLegacyByteOrder) {
    TopologyHistory history;
    history.feature_id="feature";
    history.input_geometry_id="source";
    history.result_geometry_id="result";
    history.policy_digest=std::string(topology_naming_policy_digest);
    EXPECT_EQ(detail::topology_history_digest(history),legacy_history_digest(history));
    for(int i=0;i<37;++i) {
        SemanticTopologyRef ref{"f-"+std::to_string(i%7),"side",{"source\npart",std::to_string(37-i)}};
        SelectionEvidence evidence;
        evidence.evidence_digest="evidence-"+std::to_string(i);
        evidence.adjacent={ref,{"other","vertex",{}},ref};
        history.lineage.push_back({{{"support","edge",{"b","a"}},ref},ref,TopologyLineageKind::modified,evidence});
        history.deleted.push_back({ref,"removed",evidence});
        history.ambiguous.push_back({{ref,{"support","edge",{}}},{ref,{"fork","side",{}}},"ambiguous"});
    }
    EXPECT_EQ(detail::topology_history_digest(history),legacy_history_digest(history));
    std::reverse(history.lineage.begin(),history.lineage.end());
    EXPECT_EQ(detail::topology_history_digest(history),legacy_history_digest(history));
}
