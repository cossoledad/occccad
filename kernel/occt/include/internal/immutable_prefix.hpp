#pragma once
#include <cstddef>
#include <memory>
#include <vector>

namespace occccad::kernel::detail {
// Checkpoints share completed values. Extending a prefix copies only its new
// item; the public flat result is assembled once when a checkpoint is restored.
template<class T> struct ImmutablePrefix {
    T value;
    std::shared_ptr<const ImmutablePrefix> previous;
    size_t size;
    static std::shared_ptr<const ImmutablePrefix> append(std::shared_ptr<const ImmutablePrefix> prefix, const T& item) {
        const size_t count=prefix?prefix->size:0;
        return std::make_shared<const ImmutablePrefix>(ImmutablePrefix{item,std::move(prefix),count+1});
    }
    std::vector<T> materialize() const {
        std::vector<const T*> reverse; reverse.reserve(size);
        for(auto node=this;node;node=node->previous.get())reverse.push_back(&node->value);
        std::vector<T> result;result.reserve(size);
        for(auto it=reverse.rbegin();it!=reverse.rend();++it)result.push_back(**it);
        return result;
    }
};
}
