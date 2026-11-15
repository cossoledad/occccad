#pragma once
#include <algorithm>
#include <cstddef>
#include <iterator>
#include <memory>
#include <stdexcept>
#include <unordered_set>
#include <utility>
#include <vector>

namespace occccad::kernel {

// Persistent AVL sequence of immutable payloads. Slices share leaves and joins
// copy boundary paths. Positions address this sequence, never CAD identity.
template <class T>
class ImmutableSequence {
public:
    using handle_type = std::shared_ptr<const T>;

private:
    struct Node;
    using Root = std::shared_ptr<const Node>;
    struct Node {
        Root left, right;
        std::shared_ptr<const std::vector<handle_type>> values;
        std::size_t offset = 0, count = 0;
        int height = 1;
    };
    Root root_;
    static int height(const Root& n) { return n ? n->height : 0; }
    static std::size_t count(const Root& n) { return n ? n->count : 0; }
    static Root leaf(std::shared_ptr<const std::vector<handle_type>> data, std::size_t offset,
                     std::size_t size, std::size_t* charge) {
        if (!size)
            return {};
        auto n = std::make_shared<Node>();
        n->values = std::move(data);
        n->offset = offset;
        n->count = size;
        if (charge)
            *charge += sizeof(Node) + 2 * sizeof(void*);
        return n;
    }
    static Root branch(Root a, Root b, std::size_t* charge) {
        if (!a)
            return b;
        if (!b)
            return a;
        auto n = std::make_shared<Node>();
        n->count = a->count + b->count;
        n->height = 1 + std::max(a->height, b->height);
        n->left = std::move(a);
        n->right = std::move(b);
        if (charge)
            *charge += sizeof(Node) + 2 * sizeof(void*);
        return n;
    }
    static Root balance(Root a, Root b, std::size_t* charge) {
        if (height(a) > height(b) + 1) {
            if (height(a->left) >= height(a->right))
                return branch(a->left, branch(a->right, b, charge), charge);
            const auto mid = a->right;
            return branch(branch(a->left, mid->left, charge), branch(mid->right, b, charge),
                          charge);
        }
        if (height(b) > height(a) + 1) {
            if (height(b->right) >= height(b->left))
                return branch(branch(a, b->left, charge), b->right, charge);
            const auto mid = b->left;
            return branch(branch(a, mid->left, charge), branch(mid->right, b->right, charge),
                          charge);
        }
        return branch(std::move(a), std::move(b), charge);
    }
    static Root join(const Root& a, const Root& b, std::size_t* charge) {
        if (!a)
            return b;
        if (!b)
            return a;
        if (height(a) > height(b) + 1)
            return balance(a->left, join(a->right, b, charge), charge);
        if (height(b) > height(a) + 1)
            return balance(join(a, b->left, charge), b->right, charge);
        return branch(a, b, charge);
    }
    static Root slice(const Root& n, std::size_t start, std::size_t size, std::size_t* charge) {
        if (!size)
            return {};
        if (!start && size == n->count)
            return n;
        if (n->values)
            return leaf(n->values, n->offset + start, size, charge);
        const auto middle = n->left->count;
        if (start >= middle)
            return slice(n->right, start - middle, size, charge);
        const auto left_size = std::min(size, middle - start);
        return join(slice(n->left, start, left_size, charge),
                    slice(n->right, 0, size - left_size, charge), charge);
    }
    static Root build(const std::shared_ptr<const std::vector<handle_type>>& data,
                      std::size_t start, std::size_t size, std::size_t* charge) {
        if (size <= 32)
            return leaf(data, start, size, charge);
        const auto half = size / 2;
        return branch(build(data, start, half, charge),
                      build(data, start + half, size - half, charge), charge);
    }
    explicit ImmutableSequence(Root value) : root_(std::move(value)) {}

public:
    class const_iterator {
        const ImmutableSequence* owner_ = nullptr;
        std::size_t index_ = 0;

    public:
        using iterator_category = std::forward_iterator_tag;
        using value_type = T;
        using difference_type = std::ptrdiff_t;
        using pointer = const T*;
        using reference = const T&;
        const_iterator() = default;
        const_iterator(const ImmutableSequence* owner, std::size_t index)
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
    ImmutableSequence() = default;
    explicit ImmutableSequence(std::vector<handle_type> values, std::size_t* charge = nullptr) {
        if (values.empty())
            return;
        if (charge)
            *charge += sizeof(std::vector<handle_type>) + 2 * sizeof(void*) +
                       values.capacity() * sizeof(handle_type);
        const auto data = std::make_shared<const std::vector<handle_type>>(std::move(values));
        root_ = build(data, 0, data->size(), charge);
    }
    ImmutableSequence subrange(std::size_t start, std::size_t size,
                               std::size_t* charge = nullptr) const {
        if (start > this->size() || size > this->size() - start)
            throw std::out_of_range("IMMUTABLE_SEQUENCE_RANGE");
        return ImmutableSequence(slice(root_, start, size, charge));
    }
    void append(const ImmutableSequence& other, std::size_t* charge = nullptr) {
        root_ = join(root_, other.root_, charge);
    }
    void push_back(T value) {
        append(ImmutableSequence(
            std::vector<handle_type>{std::make_shared<const T>(std::move(value))}));
    }
    void reserve(std::size_t) {}  // Leaves are shared, not one mutable flat array.
    std::size_t size() const { return count(root_); }
    bool empty() const { return !root_; }
    int depth() const { return height(root_); }
    // Retained new tree/vector ownership relative to a live predecessor. Tree
    // construction may allocate temporary rotation/join nodes; charging those
    // forever would evict valid checkpoints despite their being destroyed.
    std::size_t incremental_storage_bytes(const ImmutableSequence* previous = nullptr) const {
        std::unordered_set<const Node*> old_nodes, visited;
        std::unordered_set<const std::vector<handle_type>*> vectors;
        std::vector<const Node*> pending;
        if (previous && previous->root_)
            pending.push_back(previous->root_.get());
        while (!pending.empty()) {
            const auto* n = pending.back();
            pending.pop_back();
            if (!old_nodes.insert(n).second)
                continue;
            if (n->values)
                vectors.insert(n->values.get());
            else {
                pending.push_back(n->left.get());
                pending.push_back(n->right.get());
            }
        }
        std::size_t bytes = 0;
        if (root_)
            pending.push_back(root_.get());
        while (!pending.empty()) {
            const auto* n = pending.back();
            pending.pop_back();
            if (old_nodes.count(n) || !visited.insert(n).second)
                continue;
            bytes += sizeof(Node) + 2 * sizeof(void*);
            if (n->values) {
                if (vectors.insert(n->values.get()).second)
                    bytes += sizeof(std::vector<handle_type>) + 2 * sizeof(void*) +
                             n->values->capacity() * sizeof(handle_type);
            } else {
                pending.push_back(n->left.get());
                pending.push_back(n->right.get());
            }
        }
        return bytes;
    }
    const handle_type& shared(std::size_t index) const {
        if (index >= size())
            throw std::out_of_range("IMMUTABLE_SEQUENCE_INDEX");
        auto n = root_.get();
        while (!n->values) {
            if (index < n->left->count)
                n = n->left.get();
            else {
                index -= n->left->count;
                n = n->right.get();
            }
        }
        return (*n->values)[n->offset + index];
    }
    const T& operator[](std::size_t index) const { return *shared(index); }
    const T& front() const { return (*this)[0]; }
    const T& back() const { return (*this)[size() - 1]; }
    const_iterator begin() const { return const_iterator(this, 0); }
    const_iterator end() const { return const_iterator(this, size()); }
};
}  // namespace occccad::kernel
