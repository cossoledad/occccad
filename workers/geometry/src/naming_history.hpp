#pragma once
#include <occccad/kernel/geometry_id.hpp>

#include <functional>
#include <unordered_set>

#include "naming_artifact.hpp"

namespace occccad::worker {

enum class NamingProjection { full, literals, tip };
class NamingRows {
    const kernel::FrozenFeatureResult* source_;
    NamingProjection mode_;

public:
    class iterator {
        const kernel::FrozenFeatureResult* source_;
        NamingProjection mode_;
        std::size_t index_;

    public:
        iterator(const kernel::FrozenFeatureResult* source, NamingProjection mode,
                 std::size_t index)
            : source_(source), mode_(mode), index_(index) {}
        const kernel::TopologyLineage& operator*() const {
            return source_
                ->lineage()[mode_ == NamingProjection::literals ? source_->literal_rows()[index_]
                                                                : index_];
        }
        iterator& operator++() {
            ++index_;
            return *this;
        }
        bool operator!=(const iterator& other) const { return index_ != other.index_; }
    };
    NamingRows(const kernel::FrozenFeatureResult& source, NamingProjection mode)
        : source_(&source), mode_(mode) {}
    iterator begin() const { return {source_, mode_, 0}; }
    iterator end() const {
        return {source_, mode_,
                mode_ == NamingProjection::tip
                    ? 0
                    : (mode_ == NamingProjection::literals ? source_->literal_rows().size()
                                                           : source_->lineage().size())};
    }
};
class NamingOutputs {
    kernel::FrozenFeatureResult::OutputRange values_;
    bool include_;

public:
    NamingOutputs(const kernel::FrozenFeatureResult& source, bool include)
        : values_(source.outputs()), include_(include) {}
    auto begin() const { return include_ ? values_.begin() : values_.end(); }
    auto end() const { return values_.end(); }
};
struct NamingHistoryView {
    const std::string& input_geometry_id;
    const std::string& evidence_digest;
    NamingRows lineage;
    const std::vector<kernel::TopologyTombstone>& deleted;
    const std::vector<kernel::AmbiguousLineage>& ambiguous;
};
struct NamingFeatureView {
    const std::string &feature_id, &body_id, &input_feature_id, &profile_feature_id,
        &result_geometry_id, &result_id;
    bool topology_history_complete;
    const std::vector<std::string>& diagnostics;
    NamingHistoryView topology_history;
    NamingOutputs semantic_outputs;
};
inline NamingFeatureView naming_projection(const kernel::FrozenFeatureResult& source,
                                           NamingProjection mode) {
    static const std::vector<kernel::TopologyTombstone> no_deleted;
    static const std::vector<kernel::AmbiguousLineage> no_ambiguous;
    const auto& h = source.header();
    return {h.feature_id,
            h.body_id,
            h.input_feature_id,
            h.profile_feature_id,
            h.result_geometry_id,
            h.result_id,
            h.topology_history_complete,
            h.diagnostics,
            {h.topology_history.input_geometry_id, h.topology_history.evidence_digest,
             NamingRows(source, mode),
             mode == NamingProjection::tip ? no_deleted : h.topology_history.deleted,
             mode == NamingProjection::tip ? no_ambiguous : h.topology_history.ambiguous},
            NamingOutputs(source, mode != NamingProjection::literals)};
}

struct EncodedNamingBlock {
    v1::NamingHistoryReference reference;
    std::vector<uint8_t> bytes;
    std::uint64_t literals = 0, carries = 0;
};
struct EncodedNamingHistory {
    v1::PartTopologyManifest root;
    std::vector<std::shared_ptr<const EncodedNamingBlock>> blocks;
    std::uint64_t computed = 0, reused = 0;
};

// Memoization follows immutable native records, qualified by predecessor bytes
// and complete policy. It neither owns database truth nor pins native history.
class NamingHistoryEncoder {
    struct Entry {
        std::weak_ptr<const kernel::FrozenFeatureResult> source;
        std::string previous, policy;
        std::shared_ptr<const EncodedNamingBlock> block;
        std::size_t bytes = 0;
        std::uint64_t used = 0;
    };
    std::unordered_map<const kernel::FrozenFeatureResult*, Entry> entries_;
    std::size_t bytes_ = 0, budget_;
    std::uint64_t clock_ = 0;
    static void policy(const v1::TopologyNamingPolicy& source, v1::PartTopologyManifest& out) {
        out.set_policy_id(source.policy_id());
        out.set_evaluator_version(source.evaluator_version());
        out.set_policy_digest(source.policy_digest());
    }

public:
    explicit NamingHistoryEncoder(std::size_t budget = 32 * 1024 * 1024) : budget_(budget) {}
    void clear() {
        entries_.clear();
        bytes_ = 0;
    }
    std::size_t bytes() const { return bytes_; }
    EncodedNamingHistory encode(const kernel::FeatureResultSequence& features,
                                const v1::TopologyNamingPolicy& p, const std::string& geometry,
                                const std::string& brep, bool cold = false,
                                const std::function<bool()>& cancelled = {},
                                const v1::PartTopologyManifest* accepted = nullptr) {
        if (features.empty())
            throw std::invalid_argument("NAMING_OWNER_REQUIRED");
        EncodedNamingHistory out;
        const auto key = p.SerializeAsString();
        const auto& owner = features.frozen(0)->header();
        // Lease the entries relevant to this request before filling misses.
        // A history longer than the memo budget must not evict its cached tail
        // while rebuilding the missing head, causing a cascading full rescan.
        std::unordered_map<const kernel::FrozenFeatureResult*, Entry> leases;
        if (!cold) {
            for (std::size_t i = 0; i < features.size(); ++i) {
                const auto& source = features.frozen(i);
                const auto found = entries_.find(source.get());
                if (found != entries_.end() && found->second.source.lock() == source &&
                    found->second.policy == key)
                    leases.emplace(source.get(), found->second);
            }
        }
        std::string previous;
        std::size_t previous_rows = 0;
        bool complete = true;
        if (accepted) {
            *out.root.mutable_history() = accepted->history();
            const auto& predecessor = out.root.history(out.root.history_size() - 1);
            previous = predecessor.sha256();
            previous_rows = predecessor.full_lineage_count();
            out.reused = out.root.history_size();
        }
        for (std::size_t i = accepted ? 1 : 0; i < features.size(); ++i) {
            if (cancelled && cancelled())
                throw std::runtime_error("EVALUATION_CANCELLED");
            const auto& source = features.frozen(i);
            const auto& h = source->header();
            complete = complete && h.topology_history_complete;
            if (h.body_id != owner.body_id || h.result_id != owner.result_id ||
                (!h.result_id.empty() && h.feature_id != owner.feature_id))
                throw std::invalid_argument("NAMING_CROSS_OWNER_TRANSITION");
            if (source->previous_rows() != previous_rows)
                throw std::invalid_argument("NAMING_HISTORY_PREDECESSOR_MISMATCH");
            std::shared_ptr<const EncodedNamingBlock> encoded;
            const auto found = entries_.find(source.get());
            const auto leased = leases.find(source.get());
            if (leased != leases.end() && leased->second.previous == previous) {
                encoded = leased->second.block;
                if (found != entries_.end())
                    found->second.used = ++clock_;
                ++out.reused;
            } else {
                v1::NamingHistoryBlock block;
                block.set_schema_version(1);
                block.set_previous_sha256(previous);
                block.set_previous_lineage_count(previous_rows);
                block.set_full_lineage_count(source->lineage().size());
                auto* values = block.mutable_values();
                policy(p, *values);
                pack_naming(
                    1,
                    [&](std::size_t) {
                        return naming_projection(*source, NamingProjection::literals);
                    },
                    *values, true);
                values->set_geometry_id(h.result_geometry_id);
                for (const auto& span : source->layout()) {
                    auto* row = block.add_layout();
                    row->set_carry(span.carry);
                    row->set_offset(span.offset);
                    row->set_count(span.count);
                }
                auto payload = std::make_shared<EncodedNamingBlock>();
                const auto raw = block.SerializeAsString();
                payload->bytes.assign(raw.begin(), raw.end());
                const auto digest = kernel::make_geometry_id(raw);
                payload->reference.set_sha256(digest.substr(7));
                payload->reference.set_size_bytes(raw.size());
                payload->reference.set_feature_id(h.feature_id);
                payload->reference.set_result_geometry_id(h.result_geometry_id);
                payload->reference.set_full_lineage_count(source->lineage().size());
                payload->literals = source->literal_rows().size();
                payload->carries = source->lineage().size() - payload->literals;
                encoded = std::move(payload);
                ++out.computed;
                if (cancelled && cancelled())
                    throw std::runtime_error("EVALUATION_CANCELLED");
                const auto charge = sizeof(Entry) + sizeof(EncodedNamingBlock) +
                                    encoded->bytes.capacity() + encoded->reference.ByteSizeLong() +
                                    key.capacity() + previous.capacity();
                if (!cold && charge <= budget_) {
                    if (found != entries_.end()) {
                        bytes_ -= found->second.bytes;
                        entries_.erase(found);
                    }
                    while (!entries_.empty() && bytes_ + charge > budget_) {
                        const auto oldest = std::min_element(
                            entries_.begin(), entries_.end(), [](const auto& a, const auto& b) {
                                return a.second.used < b.second.used;
                            });
                        bytes_ -= oldest->second.bytes;
                        entries_.erase(oldest);
                    }
                    bytes_ += charge;
                    entries_.emplace(source.get(),
                                     Entry{source, previous, key, encoded, charge, ++clock_});
                }
            }
            previous = encoded->reference.sha256();
            previous_rows = source->lineage().size();
            out.blocks.push_back(encoded);
            *out.root.add_history() = encoded->reference;
        }
        if (cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
        policy(p, out.root);
        const auto& tip = *features.frozen(features.size() - 1);
        pack_naming(
            1, [&](std::size_t) { return naming_projection(tip, NamingProjection::tip); },
            out.root, true);
        out.root.clear_transitions();
        out.root.set_geometry_id(geometry);
        out.root.set_brep_sha256(brep);
        for (auto& body : *out.root.mutable_bodies()) {
            body.clear_transitions();
            body.clear_tip_transition();
            body.set_tip_feature_id(tip.header().feature_id);
            body.set_complete(complete);
            if (complete) {
                for (const auto row : tip.tip_lineage_locators()) out.root.add_tip_lineage_locators(row);
            }
        }
        for (auto& result : *out.root.mutable_results()) {
            result.clear_transitions();
            result.clear_tip_transition();
            result.set_tip_feature_id(tip.header().feature_id);
            result.set_complete(complete);
        }
        return out;
    }
};

}  // namespace occccad::worker
