#pragma once
#include <occccad/kernel/geometry_id.hpp>
#include <occccad/kernel/topology_naming.hpp>
#include <algorithm>
#include <sstream>

namespace occccad::kernel::detail {

inline std::string canonical_ref_key(const SemanticTopologyRef& ref) {
    size_t size = ref.feature_id.size()+1+ref.output_slot.size();
    for (const auto& id : ref.source_ids) size += 1+id.size();
    std::string result;
    result.reserve(size);
    result.append(ref.feature_id).push_back('\n');
    result.append(ref.output_slot);
    for (const auto& id : ref.source_ids) { result.push_back('\n'); result.append(id); }
    return result;
}

// Decorate once, sort keys, borrow values. In particular this never copies the
// history's evidence/adjacency graph merely to establish canonical wire order.
template<class T, class Key> auto canonical_view(const std::vector<T>& values, Key key) {
    std::vector<std::pair<std::string,const T*>> view;
    view.reserve(values.size());
    for (const auto& value : values) view.emplace_back(key(value),&value);
    std::sort(view.begin(),view.end(),[](const auto& a,const auto& b){return a.first<b.first;});
    return view;
}

inline std::string topology_history_digest(const TopologyHistory& history) {
    return streamed_geometry_id([&](std::ostream& canonical) {
        canonical.precision(17);
        canonical << "schema=" << history.schema_version << "|feature=" << history.feature_id
                  << "|input=" << history.input_geometry_id << "|result=" << history.result_geometry_id
                  << "|policy=" << history.policy_digest;
        for (const auto& entry : canonical_view(history.lineage,[](const auto& value){return canonical_ref_key(value.result);})) {
            const auto& item = *entry.second;
            canonical << "|lineage=" << static_cast<int>(item.kind) << ':' << entry.first << ':' << item.evidence.evidence_digest;
            for (const auto& source : canonical_view(item.sources,canonical_ref_key)) canonical << ":source=" << source.first;
            for (const auto& adjacent : canonical_view(item.evidence.adjacent,canonical_ref_key)) canonical << ":adjacent=" << adjacent.first;
        }
        for (const auto& entry : canonical_view(history.deleted,[](const auto& value){return canonical_ref_key(value.source);})) {
            const auto& item = *entry.second;
            canonical << "|deleted=" << entry.first << ':' << item.reason << ':' << item.evidence.evidence_digest;
        }
        for (const auto& entry : canonical_view(history.ambiguous,[](const auto& value){
            return value.sources.empty()?std::string{}:canonical_ref_key(value.sources.front());
        })) {
            const auto& item = *entry.second;
            canonical << "|ambiguous=" << item.diagnostic_code;
            for (const auto& source : canonical_view(item.sources,canonical_ref_key)) canonical << ":source=" << source.first;
            for (const auto& candidate : canonical_view(item.candidates,canonical_ref_key)) canonical << ":candidate=" << candidate.first;
        }
    });
}
}
