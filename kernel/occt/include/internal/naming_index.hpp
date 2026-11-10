#pragma once
#include <occccad/kernel/topology_naming.hpp>
#include <functional>
#include <unordered_map>
#include <vector>

namespace occccad::kernel::detail {
struct SemanticRefEqual {
    bool operator()(const SemanticTopologyRef& a,const SemanticTopologyRef& b)const noexcept {
        return a.feature_id==b.feature_id&&a.output_slot==b.output_slot&&a.source_ids==b.source_ids;
    }
};
struct SemanticRefHash {
    size_t operator()(const SemanticTopologyRef& ref)const noexcept {
        size_t result=0;const auto add=[&](size_t value){result^=value+0x9e3779b9+(result<<6)+(result>>2);};
        add(std::hash<std::string>{}(ref.feature_id));add(std::hash<std::string>{}(ref.output_slot));add(ref.source_ids.size());
        for(const auto& id:ref.source_ids)add(std::hash<std::string>{}(id));return result;
    }
};

// An evaluation-local incidence index over stable semantic references. Values
// borrow a range which must remain immutable for the index's lifetime. Duplicate
// references retain input order; callers choose first/all explicitly.
template<class T>class SemanticRefIndex {
public:
    template<class Range,class Reference>SemanticRefIndex(const Range& values,Reference reference) {
        entries_.reserve(values.size());for(const auto& value:values)entries_[reference(value)].push_back(&value);
    }
    const std::vector<const T*>& all(const SemanticTopologyRef& reference)const {
        static const std::vector<const T*> empty;const auto found=entries_.find(reference);return found==entries_.end()?empty:found->second;
    }
    const T* first(const SemanticTopologyRef& reference)const {const auto& values=all(reference);return values.empty()?nullptr:values.front();}
private:
    std::unordered_map<SemanticTopologyRef,std::vector<const T*>,SemanticRefHash,SemanticRefEqual> entries_;
};
}
