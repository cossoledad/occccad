#pragma once
#include <occccad/kernel/topology_naming.hpp>

#include "naming_artifact.hpp"

namespace occccad::worker {
inline kernel::FrozenFeatureResult::handle_type restore_part_continuation(const v1::PartTopologyManifest& snapshot,
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
    std::vector<kernel::SemanticTopologyRef> refs;
    refs.reserve(snapshot.reference_dictionary().rows_size());
    visit_naming_dictionary(snapshot, [&](const auto& feature, const auto& slot, const auto& sources) {
        refs.push_back({feature, slot, sources});
    });
    const auto resolve = [&](uint32_t id) -> const kernel::SemanticTopologyRef& {
        if (!id || id > refs.size()) throw std::invalid_argument("NAMING_EVIDENCE_REF_INVALID");
        return refs[id - 1];
    };
    kernel::FeatureResult root;
    root.feature_id = tail.feature_id();
    root.body_id = body.body_id();
    root.result_geometry_id = geometry;
    root.topology_history_complete = true;
    root.topology_history.feature_id = root.feature_id;
    root.topology_history.result_geometry_id = geometry;
    root.topology_history.policy_digest = policy.policy_digest();
    // The persisted permutation is lineage -> tip. Bind each output directly
    // to its immutable row so that its adjacency evidence is decoded once.
    std::vector<kernel::LineageOutputLocator> bindings(body.tip_size());
    root.topology_history.lineage.reserve(bindings.size());
    for (const auto ordinal : snapshot.tip_lineage_locators()) {
        if (!ordinal || ordinal > bindings.size() || bindings[ordinal - 1].lineage_ordinal)
            throw std::invalid_argument("PART_CONTINUATION_LINEAGE_ORDER_INVALID");
        const auto& locator = body.tip(ordinal - 1);
        if (!locator.semantic_ref() || locator.semantic_ref() > refs.size() ||
            !locator.evidence() || locator.evidence() > static_cast<unsigned>(snapshot.evidence_size()) ||
            locator.topology_type() < v1::PERSISTENT_TOPOLOGY_TYPE_FACE || locator.topology_type() > v1::PERSISTENT_TOPOLOGY_TYPE_VERTEX)
            throw std::invalid_argument("PART_CONTINUATION_LOCATOR_INVALID");
        const auto& ref = resolve(locator.semantic_ref());
        root.topology_history.lineage.push_back({{ref}, ref, kernel::TopologyLineageKind::unchanged,
            decode_naming_evidence(snapshot.evidence(locator.evidence() - 1), resolve)});
        bindings[ordinal - 1] = {static_cast<kernel::PersistentTopologyType>(locator.topology_type()),
                                 locator.local_id(), root.topology_history.lineage.size()};
    }
    return std::make_shared<const kernel::FrozenFeatureResult>(std::move(root), nullptr, &bindings);
}
}  // namespace occccad::worker
