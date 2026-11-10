#pragma once
#include <cstdint>
#include <atomic>
#include <chrono>
#include <functional>

namespace occccad::worker {
// Algorithms observe a local monotonic token. The transport probe can involve
// a completion queue; it must not run on every topology/mesh iteration.
class CancellationPolling {
public:
    using Clock=std::function<int64_t()>;
    explicit CancellationPolling(std::function<bool()> provider,Clock clock=[](){return std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::steady_clock::now().time_since_epoch()).count();})
        :provider_(std::move(provider)),clock_(std::move(clock)){}
    bool poll() {
        if(cancelled_.load(std::memory_order_acquire))return true;
        const auto now=clock_();auto next=next_.load(std::memory_order_relaxed);
        if(now>=next&&next_.compare_exchange_strong(next,now+16000,std::memory_order_relaxed))return check();
        return cancelled_.load(std::memory_order_acquire);
    }
    // Admission/publication boundaries always check the authoritative source.
    bool check(){if(cancelled_.load(std::memory_order_acquire))return true;const auto began=clock_();const bool value=provider_&&provider_();polls_.fetch_add(1,std::memory_order_relaxed);micros_.fetch_add(clock_()-began,std::memory_order_relaxed);if(value)cancelled_.store(true,std::memory_order_release);return value;}
    uint64_t polls()const{return polls_.load();}
    double milliseconds()const{return double(micros_.load())/1000.;}
private:
    std::function<bool()> provider_;Clock clock_;
    std::atomic<bool> cancelled_{false};std::atomic<int64_t> next_{0},micros_{0};std::atomic<uint64_t> polls_{0};
};
}
