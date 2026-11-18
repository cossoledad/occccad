#pragma once
#include <internal/occt_kernel.hpp>

#include "naming_artifact.hpp"

namespace occccad::worker {
inline kernel::PartBaseStage restore_part_continuation(v1::PartTopologyManifest& snapshot,
        const v1::PartContinuation& continuation, const std::string& geometry, const v1::TopologyNamingPolicy& policy) {
    if (snapshot.schema_version() != 3 || !snapshot.has_reference_dictionary() || snapshot.transitions_size() ||
        snapshot.coordinate_space() != "PART_LOCAL" || snapshot.length_unit() != "mm" ||
        snapshot.geometry_id() != geometry || snapshot.brep_sha256() != geometry.substr(7) ||
        snapshot.policy_digest() != policy.policy_digest() || snapshot.policy_id() != policy.policy_id() ||
        snapshot.evaluator_version() != policy.evaluator_version() || snapshot.bodies_size() != 1 || snapshot.results_size() ||
        !continuation.stage_count() ||
        (snapshot.history_size() != continuation.stage_count() && snapshot.history_size() != continuation.stage_count() + 1) ||
        continuation.validation_policy() != "occccad.geometry.brep-valid.v1")
        throw std::invalid_argument("PART_CONTINUATION_SNAPSHOT_INVALID");
    const auto& body = snapshot.bodies(0);
    const auto& tail = snapshot.history(snapshot.history_size() - 1);
    if (!body.complete() || body.body_id() != continuation.body_id() || body.tip_feature_id() != continuation.feature_id() ||
        tail.feature_id() != continuation.feature_id() || tail.result_geometry_id() != geometry ||
        !body.tip_size() || body.tip_size() != snapshot.tip_lineage_locators_size() || tail.full_lineage_count() != body.tip_size())
        throw std::invalid_argument("PART_CONTINUATION_TIP_INVALID");
    expand_naming_references(snapshot);
    kernel::PartBaseStage base{geometry, true, {}};
    auto& root = base.root;
    root.feature_id = tail.feature_id();
    root.body_id = body.body_id();
    root.result_geometry_id = geometry;
    root.topology_history_complete = true;
    root.topology_history.feature_id = root.feature_id;
    root.topology_history.result_geometry_id = geometry;
    root.topology_history.policy_digest = policy.policy_digest();
    for (const auto& locator : body.tip()) {
        if (!locator.semantic_ref() || locator.semantic_ref() > snapshot.semantic_refs_size() ||
            !locator.evidence() || locator.evidence() > snapshot.evidence_size() ||
            locator.topology_type() < v1::PERSISTENT_TOPOLOGY_TYPE_FACE || locator.topology_type() > v1::PERSISTENT_TOPOLOGY_TYPE_VERTEX)
            throw std::invalid_argument("PART_CONTINUATION_LOCATOR_INVALID");
        const auto& ref = snapshot.semantic_refs(locator.semantic_ref() - 1);
        root.semantic_outputs.push_back({{ref.feature_id(), ref.output_slot(), {ref.source_ids().begin(), ref.source_ids().end()}},
            static_cast<kernel::PersistentTopologyType>(locator.topology_type()), locator.local_id(),
            read_naming_evidence(snapshot.evidence(locator.evidence() - 1), snapshot)});
    }
    std::vector<bool> seen(root.semantic_outputs.size());
    for (const auto ordinal : snapshot.tip_lineage_locators()) {
        if (!ordinal || ordinal > seen.size() || seen[ordinal - 1])
            throw std::invalid_argument("PART_CONTINUATION_LINEAGE_ORDER_INVALID");
        seen[ordinal - 1] = true;
        const auto& output = root.semantic_outputs[ordinal - 1];
        root.topology_history.lineage.push_back({{output.semantic_ref}, output.semantic_ref,
            kernel::TopologyLineageKind::unchanged, output.evidence});
    }
    return base;
}
}  // namespace occccad::worker
