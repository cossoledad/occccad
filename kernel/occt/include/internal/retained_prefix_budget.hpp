#pragma once
#include <internal/immutable_prefix.hpp>
#include <stdexcept>
#include <unordered_map>

namespace occccad::kernel::detail {

// Account the union retained by cached roots, including forks. A shared prefix
// is charged once. Each node owns one edge to its predecessor; roots add leases.
// Pointer keys are private memory ownership, never naming/geometry identities.
template<class T> class RetainedPrefixBudget {
    using Node = ImmutablePrefix<T>;
    std::unordered_map<const Node*, std::size_t> owners_;
    std::size_t bytes_ = 0;
    static std::size_t own_bytes(const Node* node) {
        return node->bytes - (node->previous ? node->previous->bytes : 0);
    }
public:
    std::size_t bytes() const { return bytes_; }
    std::size_t additional(const std::shared_ptr<const Node>& root) const {
        std::size_t result = 0;
        for (auto node = root.get(); node && !owners_.count(node); node = node->previous.get())
            result += own_bytes(node);
        return result;
    }
    void retain(const std::shared_ptr<const Node>& root) {
        for (auto node = root.get(); node; node = node->previous.get()) {
            auto& count = owners_[node];
            if (count++) break;
            bytes_ += own_bytes(node);
        }
    }
    void release(const std::shared_ptr<const Node>& root) {
        for (auto node = root.get(); node; node = node->previous.get()) {
            const auto found = owners_.find(node);
            if (found == owners_.end()) throw std::logic_error("PREFIX_BUDGET_OWNER_MISSING");
            if (--found->second) break;
            bytes_ -= own_bytes(node);
            owners_.erase(found);
        }
    }
    void clear() { owners_.clear(); bytes_ = 0; }
};
}
