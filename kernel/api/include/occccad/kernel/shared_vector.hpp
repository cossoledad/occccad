#pragma once

#include <initializer_list>
#include <memory>
#include <utility>
#include <vector>

namespace occccad::kernel {

// Value semantics with shared read-only storage. Mutating access detaches;
// storage addresses are private allocation evidence, never CAD identity.
template <class T>
class SharedVector {
    struct Storage {
        std::vector<T> values;
        bool mutable_borrowed = false;
        explicit Storage(std::vector<T> source) : values(std::move(source)) {}
    };
    std::shared_ptr<Storage> values_;
    const std::vector<T>& read() const {
        static const std::vector<T> empty;
        return values_ ? values_->values : empty;
    }
    std::vector<T>& write(bool borrow = false) {
        if (!values_)
            values_ = std::make_shared<Storage>(std::vector<T>{});
        else if (!values_.unique())
            values_ = std::make_shared<Storage>(read());
        if (borrow)
            values_->mutable_borrowed = true;
        return values_->values;
    }

public:
    using value_type = T;
    using iterator = typename std::vector<T>::iterator;
    using const_iterator = typename std::vector<T>::const_iterator;
    SharedVector() = default;
    SharedVector(const SharedVector& other)
        : values_(other.values_ && other.values_->mutable_borrowed
                      ? std::make_shared<Storage>(other.read())
                      : other.values_) {}
    SharedVector& operator=(const SharedVector& other) {
        if (this != &other) {
            SharedVector copy(other);
            values_ = std::move(copy.values_);
        }
        return *this;
    }
    SharedVector(SharedVector&&) = default;
    SharedVector& operator=(SharedVector&&) = default;
    // A moved vector may have existing mutable iterators. Preserve isolation
    // when this value is subsequently copied, just as for our own write views.
    SharedVector(std::vector<T> values) : values_(std::make_shared<Storage>(std::move(values))) {
        values_->mutable_borrowed = true;
    }
    SharedVector(std::initializer_list<T> values)
        : values_(std::make_shared<Storage>(std::vector<T>(values))) {}
    template <class Iterator>
    SharedVector(Iterator first, Iterator last)
        : values_(std::make_shared<Storage>(std::vector<T>(first, last))) {}
    SharedVector& operator=(std::initializer_list<T> values) {
        values_ = std::make_shared<Storage>(std::vector<T>(values));
        return *this;
    }
    operator const std::vector<T>&() const { return read(); }
    std::size_t size() const { return read().size(); }
    std::size_t capacity() const { return read().capacity(); }
    bool empty() const { return read().empty(); }
    const T& operator[](std::size_t i) const { return read()[i]; }
    T& operator[](std::size_t i) { return write(true)[i]; }
    const T& front() const { return read().front(); }
    const T& back() const { return read().back(); }
    T& front() { return write(true).front(); }
    T& back() { return write(true).back(); }
    const_iterator begin() const { return read().begin(); }
    const_iterator end() const { return read().end(); }
    iterator begin() { return write(true).begin(); }
    iterator end() { return write(true).end(); }
    void clear() { values_.reset(); }
    void reserve(std::size_t n) { write().reserve(n); }
    void push_back(T value) { write().push_back(std::move(value)); }
    template <class Iterator>
    iterator insert(const_iterator position, Iterator first, Iterator last) {
        const auto offset = position - read().begin();
        auto& target = write(true);
        return target.insert(target.begin() + offset, first, last);
    }
    iterator erase(const_iterator first, const_iterator last) {
        const auto begin_offset = first - read().begin(), end_offset = last - read().begin();
        auto& target = write(true);
        return target.erase(target.begin() + begin_offset, target.begin() + end_offset);
    }
    friend bool operator==(const SharedVector& a, const SharedVector& b) {
        return a.values_ == b.values_ || a.read() == b.read();
    }
    friend bool operator!=(const SharedVector& a, const SharedVector& b) { return !(a == b); }
    friend bool operator<(const SharedVector& a, const SharedVector& b) {
        return a.read() < b.read();
    }
    friend bool operator==(const SharedVector& a, const std::vector<T>& b) { return a.read() == b; }
    friend bool operator==(const std::vector<T>& a, const SharedVector& b) { return a == b.read(); }
    friend bool operator!=(const SharedVector& a, const std::vector<T>& b) { return !(a == b); }
    friend bool operator!=(const std::vector<T>& a, const SharedVector& b) { return !(a == b); }
    const void* storage_identity() const { return values_.get(); }
    std::size_t storage_bytes() const {
        std::size_t bytes =
            values_ ? sizeof(Storage) + 2 * sizeof(void*) + read().capacity() * sizeof(T) : 0;
        return bytes;
    }
};
}  // namespace occccad::kernel
