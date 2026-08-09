#ifndef OCCCCAD_KERNEL_MESH_GLB_HPP
#define OCCCCAD_KERNEL_MESH_GLB_HPP

#include <occccad/kernel/kernel.hpp>

#include <cstdint>
#include <vector>

namespace occccad::kernel {

[[nodiscard]] std::vector<uint8_t> make_glb(const TessellationResult& mesh);

}  // namespace occccad::kernel

#endif  // OCCCCAD_KERNEL_MESH_GLB_HPP
