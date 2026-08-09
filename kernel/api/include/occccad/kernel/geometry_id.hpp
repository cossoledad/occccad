// occccad Kernel — Geometry ID helpers

#ifndef OCCCCAD_KERNEL_GEOMETRY_ID_HPP
#define OCCCCAD_KERNEL_GEOMETRY_ID_HPP

#include <string>
#include <string_view>

namespace occccad::kernel {

/// Generate a GeometryId from raw content bytes.
/// Uses SHA-256 for content-addressable identity.
[[nodiscard]] std::string make_geometry_id(const void* data, size_t size);

[[nodiscard]] inline std::string make_geometry_id(const std::string_view& data) {
    return make_geometry_id(data.data(), data.size());
}

}  // namespace occccad::kernel

#endif  // OCCCCAD_KERNEL_GEOMETRY_ID_HPP
