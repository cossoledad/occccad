#pragma once
#include <occccad/kernel/topology_naming_values.hpp>
#include <occccad/kernel/topology_value_key.hpp>

#include <limits>
#include <mutex>
#include <unordered_map>

namespace occccad::kernel {

// Lossless encoding plan. Offsets only address rows of the verified predecessor
// or this record's literal table; topology identity remains the complete ref.
struct FeatureHistorySpan {
    bool carry = false;
    std::size_t offset = 0, count = 0;
};

// For evaluators that emit exactly one output per final lineage row in row order.
struct LineageOutputLocator {
    PersistentTopologyType type;
    std::uint64_t local_id;
};

class FrozenFeatureResult {
    FeatureResult header_;
    ImmutableSequence<TopologyLineage> lineage_;
    std::vector<FeatureHistorySpan> layout_;
    std::vector<std::size_t> literal_rows_;
    std::vector<std::uint8_t> output_bindings_;
    std::vector<SemanticTopologyOutput> exceptional_outputs_;
    std::size_t output_count_ = 0, charged_bytes_ = 0, previous_rows_ = 0;
    mutable std::mutex materialization_mutex_;
    mutable std::weak_ptr<const FeatureResult> materialized_;

    using ChargedSources = std::unordered_set<const void*>;
    static std::size_t ref_bytes(const SemanticTopologyRef& r, ChargedSources& charged) {
        std::size_t n = r.feature_id.capacity() + r.output_slot.capacity();
        if (r.source_ids.storage_identity() &&
            charged.insert(r.source_ids.storage_identity()).second) {
            n += r.source_ids.storage_bytes();
            for (const auto& s : r.source_ids)
                n += s.capacity();
        }
        return n;
    }
    static std::size_t evidence_bytes(const SelectionEvidence& e, ChargedSources& charged) {
        std::size_t n = e.geometry_type.capacity() + e.measure_dimension.capacity() +
                        e.endpoint_role.capacity() + e.evidence_digest.capacity() +
                        e.adjacent.capacity() * sizeof(SemanticTopologyRef);
        for (const auto& r : e.adjacent)
            n += ref_bytes(r, charged);
        return n;
    }
    static std::size_t row_bytes(const TopologyLineage& r, ChargedSources& charged) {
        std::size_t n = sizeof(TopologyLineage) + 2 * sizeof(void*) + ref_bytes(r.result, charged) +
                        evidence_bytes(r.evidence, charged) +
                        r.sources.capacity() * sizeof(SemanticTopologyRef);
        for (const auto& source : r.sources)
            n += ref_bytes(source, charged);
        return n;
    }
    static bool same_row(const TopologyLineage& a, const TopologyLineage& b) {
        return a.kind == b.kind && detail::equal_ref(a.result, b.result) &&
               a.sources.size() == b.sources.size() &&
               std::equal(a.sources.begin(), a.sources.end(), b.sources.begin(),
                          detail::equal_ref) &&
               detail::EvidenceEqual{}(&a.evidence, &b.evidence);
    }
    static void varint(std::vector<std::uint8_t>& out, std::uint64_t n) {
        while (n >= 128) {
            out.push_back(static_cast<std::uint8_t>(n) | 128);
            n >>= 7;
        }
        out.push_back(static_cast<std::uint8_t>(n));
    }
    std::uint64_t varint(std::size_t& position) const {
        std::uint64_t value = 0;
        for (unsigned shift = 0; shift < 64; shift += 7) {
            if (position >= output_bindings_.size())
                throw std::logic_error("FEATURE_BINDING_TRUNCATED");
            const auto byte = output_bindings_[position++];
            if (shift == 63 && byte > 1)
                throw std::logic_error("FEATURE_BINDING_OVERFLOW");
            value |= std::uint64_t(byte & 127) << shift;
            if (!(byte & 128))
                return value;
        }
        throw std::logic_error("FEATURE_BINDING_OVERFLOW");
    }

public:
    using handle_type = std::shared_ptr<const FrozenFeatureResult>;
    struct OutputView {
        const SemanticTopologyRef& semantic_ref;
        PersistentTopologyType topology_type;
        std::uint64_t local_id;
        const SelectionEvidence& evidence;
    };
    class OutputRange {
        const FrozenFeatureResult* owner_;

    public:
        class iterator {
            const FrozenFeatureResult* owner_;
            std::size_t index_, cursor_ = 0, extra_ = 0;

        public:
            iterator(const FrozenFeatureResult* owner, std::size_t index)
                : owner_(owner), index_(index) {}
            OutputView operator*() const {
                auto cursor = cursor_;
                const auto type = static_cast<PersistentTopologyType>(owner_->varint(cursor));
                const auto local = owner_->varint(cursor), row = owner_->varint(cursor);
                if (row) {
                    const auto& source = owner_->lineage_[row - 1];
                    return {source.result, type, local, source.evidence};
                }
                const auto& source = owner_->exceptional_outputs_[extra_];
                return {source.semantic_ref, type, local, source.evidence};
            }
            iterator& operator++() {
                owner_->varint(cursor_);
                owner_->varint(cursor_);
                if (!owner_->varint(cursor_))
                    ++extra_;
                ++index_;
                return *this;
            }
            bool operator!=(const iterator& other) const {
                return owner_ != other.owner_ || index_ != other.index_;
            }
        };
        explicit OutputRange(const FrozenFeatureResult* owner) : owner_(owner) {}
        iterator begin() const { return {owner_, 0}; }
        iterator end() const { return {owner_, owner_->output_count_}; }
    };
    FrozenFeatureResult(FeatureResult value, const FrozenFeatureResult* previous,
                        const std::vector<LineageOutputLocator>* aligned = nullptr) {
        if (aligned && (!value.semantic_outputs.empty() || aligned->size() != value.topology_history.lineage.size()))
            throw std::logic_error("FEATURE_LINEAGE_OUTPUT_COUNT");
        if (previous &&
            (previous->header_.body_id != value.body_id ||
             previous->header_.result_id != value.result_id ||
             (!value.result_id.empty() && previous->header_.feature_id != value.feature_id)))
            previous = nullptr;
        // Share complete ordered source lists within this immutable record.
        // Value equality qualifies sharing; copy-on-write preserves mutations
        // in independent working values and reader materializations.
        struct SourceHash {
            std::size_t operator()(const SharedVector<std::string>& ids) const {
                std::size_t h = ids.size();
                for (const auto& id : ids)
                    h ^= std::hash<std::string>{}(id) + 0x9e3779b9 + (h << 6) + (h >> 2);
                return h;
            }
        };
        std::unordered_map<SharedVector<std::string>, bool, SourceHash> source_lists;
        ChargedSources charged_sources, canonical_sources;
        const auto seed_ref = [&](const SemanticTopologyRef& ref) {
            const auto identity = ref.source_ids.storage_identity();
            if (identity && charged_sources.insert(identity).second) {
                source_lists.emplace(ref.source_ids, true);
                canonical_sources.insert(identity);
            }
        };
        const auto seed_evidence = [&](const SelectionEvidence& evidence) {
            for (const auto& ref : evidence.adjacent)
                seed_ref(ref);
        };
        if (previous) {
            for (const auto& row : previous->lineage_) {
                seed_ref(row.result);
                for (const auto& ref : row.sources)
                    seed_ref(ref);
                seed_evidence(row.evidence);
            }
            for (const auto& output : previous->exceptional_outputs_) {
                seed_ref(output.semantic_ref);
                seed_evidence(output.evidence);
            }
            for (const auto& row : previous->header_.topology_history.deleted) {
                seed_ref(row.source);
                seed_evidence(row.evidence);
            }
            for (const auto& row : previous->header_.topology_history.ambiguous) {
                for (const auto& ref : row.sources)
                    seed_ref(ref);
                for (const auto& ref : row.candidates)
                    seed_ref(ref);
            }
        }
        const auto share_ref = [&](SemanticTopologyRef& ref) {
            if (ref.source_ids.empty()) {
                ref.source_ids = SharedVector<std::string>{};
                return;
            }
            if (canonical_sources.count(ref.source_ids.storage_identity()))
                return;
            auto entry = source_lists.find(ref.source_ids);
            if (entry == source_lists.end())
                entry = source_lists.emplace(ref.source_ids, true).first;
            ref.source_ids = entry->first;
            canonical_sources.insert(ref.source_ids.storage_identity());
        };
        const auto share_evidence = [&](SelectionEvidence& evidence) {
            for (auto& ref : evidence.adjacent)
                share_ref(ref);
        };
        for (auto& row : value.topology_history.lineage) {
            share_ref(row.result);
            for (auto& ref : row.sources)
                share_ref(ref);
            share_evidence(row.evidence);
        }
        for (auto& output : value.semantic_outputs) {
            share_ref(output.semantic_ref);
            share_evidence(output.evidence);
        }
        for (auto& row : value.topology_history.deleted) {
            share_ref(row.source);
            share_evidence(row.evidence);
        }
        for (auto& row : value.topology_history.ambiguous) {
            for (auto& ref : row.sources)
                share_ref(ref);
            for (auto& ref : row.candidates)
                share_ref(ref);
        }
        using Index = std::unordered_map<const SemanticTopologyRef*, std::size_t, detail::RefHash,
                                         detail::RefEqual>;
        Index prior;
        if (previous) {
            previous_rows_ = previous->lineage_.size();
            prior.reserve(previous_rows_);
            for (std::size_t i = 0; i < previous_rows_; ++i)
                prior.emplace(&previous->lineage_[i].result, i);
        }
        std::vector<typename ImmutableSequence<TopologyLineage>::handle_type> literals;
        std::size_t shared_start = 0, shared_count = 0;
        const auto flush_literals = [&]() {
            if (literals.empty())
                return;
            lineage_.append(ImmutableSequence<TopologyLineage>(std::move(literals)));
            literals.clear();
        };
        const auto flush_shared = [&]() {
            if (!shared_count)
                return;
            lineage_.append(previous->lineage_.subrange(shared_start, shared_count));
            shared_count = 0;
        };
        for (std::size_t i = 0; i < value.topology_history.lineage.size(); ++i) {
            auto& row = value.topology_history.lineage[i];
            const auto found = prior.find(&row.result);
            const bool known = found != prior.end();
            const auto* old = known ? &previous->lineage_[found->second] : nullptr;
            const bool carry = known && row.kind == TopologyLineageKind::unchanged &&
                               row.sources.size() == 1 &&
                               detail::equal_ref(row.sources.front(), row.result) &&
                               detail::EvidenceEqual{}(&row.evidence, &old->evidence);
            const auto offset = carry ? found->second : literal_rows_.size();
            if (!carry)
                literal_rows_.push_back(i);
            if (!layout_.empty() && layout_.back().carry == carry &&
                layout_.back().offset + layout_.back().count == offset)
                ++layout_.back().count;
            else
                layout_.push_back({carry, offset, 1});
            if (known && same_row(row, *old)) {
                flush_literals();
                if (shared_count && shared_start + shared_count != found->second)
                    flush_shared();
                if (!shared_count)
                    shared_start = found->second;
                ++shared_count;
            } else {
                flush_shared();
                charged_bytes_ += row_bytes(row, charged_sources);
                literals.push_back(std::make_shared<const TopologyLineage>(std::move(row)));
            }
        }
        flush_literals();
        flush_shared();
        charged_bytes_ +=
            lineage_.incremental_storage_bytes(previous ? &previous->lineage_ : nullptr);
        if (aligned) {
            output_count_ = aligned->size();
            for (std::size_t i = 0; i < aligned->size(); ++i) {
                varint(output_bindings_, static_cast<unsigned>((*aligned)[i].type));
                varint(output_bindings_, (*aligned)[i].local_id);
                varint(output_bindings_, i + 1);
            }
        } else {
            Index current;
            current.reserve(lineage_.size());
            for (std::size_t i = 0; i < lineage_.size(); ++i) {
                const auto [it, added] = current.emplace(&lineage_[i].result, i);
                if (!added)
                    it->second = std::numeric_limits<std::size_t>::max();
            }
            output_count_ = value.semantic_outputs.size();
            for (auto& output : value.semantic_outputs) {
                const auto found = current.find(&output.semantic_ref);
                const bool indexed =
                    found != current.end() &&
                    found->second != std::numeric_limits<std::size_t>::max() &&
                    detail::EvidenceEqual{}(&output.evidence, &lineage_[found->second].evidence);
                varint(output_bindings_, static_cast<unsigned>(output.topology_type));
                varint(output_bindings_, output.local_id);
                varint(output_bindings_, indexed ? found->second + 1 : 0);
                if (!indexed) {
                    charged_bytes_ += ref_bytes(output.semantic_ref, charged_sources) +
                                      evidence_bytes(output.evidence, charged_sources);
                    exceptional_outputs_.push_back(std::move(output));
                }
            }
        }
        std::vector<TopologyLineage>().swap(value.topology_history.lineage);
        std::vector<SemanticTopologyOutput>().swap(value.semantic_outputs);
        header_ = std::move(value);
        charged_bytes_ += sizeof(FrozenFeatureResult) + 2 * sizeof(void*) +
                          layout_.capacity() * sizeof(FeatureHistorySpan) +
                          literal_rows_.capacity() * sizeof(std::size_t) +
                          output_bindings_.capacity() +
                          exceptional_outputs_.capacity() * sizeof(SemanticTopologyOutput);
        for (const auto* s :
             {&header_.feature_id, &header_.body_id, &header_.input_feature_id,
              &header_.profile_feature_id, &header_.result_geometry_id, &header_.result_id,
              &header_.topology_history.feature_id, &header_.topology_history.input_geometry_id,
              &header_.topology_history.result_geometry_id,
              &header_.topology_history.evidence_digest, &header_.topology_history.policy_digest})
            charged_bytes_ += s->capacity();
        charged_bytes_ += header_.diagnostics.capacity() * sizeof(std::string);
        for (const auto& d : header_.diagnostics)
            charged_bytes_ += d.capacity();
        charged_bytes_ += header_.topology_history.deleted.capacity() * sizeof(TopologyTombstone) +
                          header_.topology_history.ambiguous.capacity() * sizeof(AmbiguousLineage);
        for (const auto& d : header_.topology_history.deleted)
            charged_bytes_ += ref_bytes(d.source, charged_sources) +
                              evidence_bytes(d.evidence, charged_sources) + d.reason.capacity();
        for (const auto& a : header_.topology_history.ambiguous) {
            charged_bytes_ += a.sources.capacity() * sizeof(SemanticTopologyRef) +
                              a.candidates.capacity() * sizeof(SemanticTopologyRef) +
                              a.diagnostic_code.capacity();
            for (const auto& r : a.sources)
                charged_bytes_ += ref_bytes(r, charged_sources);
            for (const auto& r : a.candidates)
                charged_bytes_ += ref_bytes(r, charged_sources);
        }
    }
    const FeatureResult& header() const { return header_; }
    const ImmutableSequence<TopologyLineage>& lineage() const { return lineage_; }
    const std::vector<FeatureHistorySpan>& layout() const { return layout_; }
    const std::vector<std::size_t>& literal_rows() const { return literal_rows_; }
    std::size_t previous_rows() const { return previous_rows_; }
    std::size_t output_count() const { return output_count_; }
    std::size_t charged_bytes() const { return charged_bytes_; }
    OutputRange outputs() const { return OutputRange(this); }
    std::vector<std::uint32_t> tip_lineage_locators() const {
        if (output_count_ != lineage_.size()) return {};
        std::vector<std::uint32_t> locators(lineage_.size());
        std::size_t cursor = 0;
        for (std::size_t output = 0; output < output_count_; ++output) {
            varint(cursor); // topology type
            varint(cursor); // frozen local ID
            const auto row = varint(cursor);
            if (!row || row > locators.size() || locators[row - 1]) return {};
            locators[row - 1] = static_cast<std::uint32_t>(output + 1);
        }
        return locators;
    }
    template <class Visit>
    void visit_outputs(Visit visit) const {
        std::size_t cursor = 0, extra = 0;
        for (std::size_t i = 0; i < output_count_; ++i) {
            const auto type = static_cast<PersistentTopologyType>(varint(cursor));
            const auto local = varint(cursor), row = varint(cursor);
            if (row) {
                const auto& source = lineage_[row - 1];
                visit(type, local, source.result, source.evidence);
            } else {
                const auto& source = exceptional_outputs_[extra++];
                visit(type, local, source.semantic_ref, source.evidence);
            }
        }
    }
    std::shared_ptr<const FeatureResult> materialize() const {
        std::lock_guard lock(materialization_mutex_);
        if (auto value = materialized_.lock())
            return value;
        auto value = std::make_shared<FeatureResult>(header_);
        value->topology_history.lineage.reserve(lineage_.size());
        for (const auto& row : lineage_)
            value->topology_history.lineage.push_back(row);
        value->semantic_outputs.reserve(output_count_);
        visit_outputs([&](auto type, auto local, const auto& ref, const auto& evidence) {
            value->semantic_outputs.push_back({ref, type, local, evidence});
        });
        materialized_ = value;
        return value;
    }
};

// Reader leases own optional full materializations. Checkpoints retain only
// compact immutable records, so reading a result cannot enlarge their ownership.
class FeatureResultSequence {
    std::vector<FrozenFeatureResult::handle_type> values_;
    struct Leases {
        std::mutex mutex;
        std::vector<std::shared_ptr<const FeatureResult>> values;
    };
    std::shared_ptr<Leases> leases_ = std::make_shared<Leases>();

public:
    class const_iterator {
        const FeatureResultSequence* owner_ = nullptr;
        std::size_t index_ = 0;

    public:
        using iterator_category = std::forward_iterator_tag;
        using value_type = FeatureResult;
        using difference_type = std::ptrdiff_t;
        using pointer = const FeatureResult*;
        using reference = const FeatureResult&;
        const_iterator() = default;
        const_iterator(const FeatureResultSequence* owner, std::size_t index)
            : owner_(owner), index_(index) {}
        reference operator*() const { return (*owner_)[index_]; }
        pointer operator->() const { return &**this; }
        const_iterator& operator++() {
            ++index_;
            return *this;
        }
        const_iterator operator++(int) {
            auto old = *this;
            ++*this;
            return old;
        }
        bool operator==(const const_iterator& other) const {
            return owner_ == other.owner_ && index_ == other.index_;
        }
        bool operator!=(const const_iterator& other) const { return !(*this == other); }
    };
    FeatureResultSequence() = default;
    explicit FeatureResultSequence(std::vector<FrozenFeatureResult::handle_type> values)
        : values_(std::move(values)) {
        leases_->values.resize(values_.size());
    }
    FeatureResultSequence(const FeatureResultSequence& other) : values_(other.values_) {
        if (other.leases_) {
            std::lock_guard lock(other.leases_->mutex);
            leases_->values = other.leases_->values;
        }
    }
    FeatureResultSequence& operator=(const FeatureResultSequence& other) {
        if (this != &other) {
            FeatureResultSequence copy(other);
            *this = std::move(copy);
        }
        return *this;
    }
    FeatureResultSequence(FeatureResultSequence&&) = default;
    FeatureResultSequence& operator=(FeatureResultSequence&&) = default;
    void push_back(FeatureResult value, const std::vector<LineageOutputLocator>* aligned = nullptr) {
        if (!leases_)
            leases_ = std::make_shared<Leases>();
        values_.push_back(std::make_shared<const FrozenFeatureResult>(
            std::move(value), empty() ? nullptr : values_.back().get(), aligned));
        leases_->values.push_back({});
    }
    void reserve(std::size_t n) {
        if (!leases_)
            leases_ = std::make_shared<Leases>();
        values_.reserve(n);
        leases_->values.reserve(n);
    }
    std::size_t size() const { return values_.size(); }
    bool empty() const { return values_.empty(); }
    const FrozenFeatureResult::handle_type& frozen(std::size_t i) const { return values_[i]; }
    const std::shared_ptr<const FeatureResult>& shared(std::size_t i) const {
        std::lock_guard lock(leases_->mutex);
        auto& value = leases_->values[i];
        if (!value)
            value = values_[i]->materialize();
        return value;
    }
    const FeatureResult& operator[](std::size_t i) const { return *shared(i); }
    const FeatureResult& front() const { return (*this)[0]; }
    const FeatureResult& back() const { return (*this)[size() - 1]; }
    const_iterator begin() const { return const_iterator(this, 0); }
    const_iterator end() const { return const_iterator(this, size()); }
};
}  // namespace occccad::kernel
