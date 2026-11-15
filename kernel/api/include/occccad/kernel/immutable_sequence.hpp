#pragma once
#include <cstddef>
#include <iterator>
#include <memory>
#include <utility>
#include <vector>

namespace occccad::kernel {

// Evaluator working storage. Records are frozen once and shared by returned
// views and checkpoints; indexes/addresses never become persistent identities.
template<class T> class ImmutableSequence {
public:
    using handle_type = std::shared_ptr<const T>;
    class const_iterator {
        using Base = typename std::vector<handle_type>::const_iterator;
        Base current_;
    public:
        using iterator_category = std::forward_iterator_tag;
        using value_type = T;
        using difference_type = std::ptrdiff_t;
        using pointer = const T*;
        using reference = const T&;
        const_iterator() = default;
        explicit const_iterator(Base value) : current_(value) {}
        reference operator*() const { return **current_; }
        pointer operator->() const { return current_->get(); }
        const_iterator& operator++() { ++current_; return *this; }
        const_iterator operator++(int) { auto old = *this; ++*this; return old; }
        bool operator==(const const_iterator& other) const { return current_ == other.current_; }
        bool operator!=(const const_iterator& other) const { return !(*this == other); }
    };
    ImmutableSequence() = default;
    explicit ImmutableSequence(std::vector<handle_type> values) : values_(std::move(values)) {}
    void push_back(T value) { values_.push_back(std::make_shared<const T>(std::move(value))); }
    void reserve(std::size_t count) { values_.reserve(count); }
    std::size_t size() const { return values_.size(); }
    bool empty() const { return values_.empty(); }
    const T& operator[](std::size_t index) const { return *values_[index]; }
    const T& front() const { return *values_.front(); }
    const T& back() const { return *values_.back(); }
    const handle_type& shared(std::size_t index) const { return values_[index]; }
    const_iterator begin() const { return const_iterator(values_.begin()); }
    const_iterator end() const { return const_iterator(values_.end()); }
private:
    std::vector<handle_type> values_;
};
}
