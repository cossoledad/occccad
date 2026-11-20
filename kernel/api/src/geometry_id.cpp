#include <occccad/kernel/geometry_id.hpp>
#include <openssl/evp.h>
#include <array>
#include <stdexcept>
#include <memory>
#include <ostream>

namespace occccad::kernel {

namespace {
using Digest = std::array<unsigned char, 32>;
std::string format_id(const Digest& digest) {
    constexpr char hex[] = "0123456789abcdef";
    std::string result = "sha256:";
    result.reserve(result.size() + 2 * digest.size());
    for (const auto byte : digest) {
        result.push_back(hex[byte >> 4]);
        result.push_back(hex[byte & 15]);
    }
    return result;
}

class DigestBuffer final : public std::streambuf {
public:
    explicit DigestBuffer(EVP_MD_CTX* ctx) : ctx_(ctx) { setp(buffer_.data(), buffer_.data()+buffer_.size()); }
private:
    int sync() override {
        if (EVP_DigestUpdate(ctx_, pbase(), static_cast<size_t>(pptr()-pbase())) != 1) return -1;
        setp(buffer_.data(), buffer_.data()+buffer_.size());
        return 0;
    }
    int_type overflow(int_type ch) override {
        if (sync() != 0) return traits_type::eof();
        if (!traits_type::eq_int_type(ch, traits_type::eof())) { *pptr() = traits_type::to_char_type(ch); pbump(1); }
        return traits_type::not_eof(ch);
    }
    EVP_MD_CTX* ctx_;
    std::array<char, 16384> buffer_;
};
}

std::string make_geometry_id(const void* data, size_t size) {
    Digest digest{};
    unsigned int written = 0;
    if (EVP_Digest(data, size, digest.data(), &written, EVP_sha256(), nullptr) != 1 || written != digest.size())
        throw std::runtime_error("SHA256_DIGEST_FAILED");
    return format_id(digest);
}

std::string streamed_geometry_id(const std::function<void(std::ostream&)>& write) {
    std::unique_ptr<EVP_MD_CTX, decltype(&EVP_MD_CTX_free)> ctx(EVP_MD_CTX_new(), EVP_MD_CTX_free);
    if (!ctx || EVP_DigestInit_ex(ctx.get(), EVP_sha256(), nullptr) != 1) throw std::runtime_error("SHA256_DIGEST_FAILED");
    DigestBuffer buffer(ctx.get());
    std::ostream stream(&buffer);
    stream.exceptions(std::ios::failbit | std::ios::badbit);
    write(stream);
    stream.flush();
    Digest digest{};
    unsigned int written = 0;
    if (EVP_DigestFinal_ex(ctx.get(), digest.data(), &written) != 1 || written != digest.size())
        throw std::runtime_error("SHA256_DIGEST_FAILED");
    return format_id(digest);
}

}  // namespace occccad::kernel
