#include <occccad/kernel/mesh_glb.hpp>

#include <algorithm>
#include <array>
#include <cstring>
#include <sstream>
#include <stdexcept>
#include <string>

namespace occccad::kernel {
namespace {

void append_u32(std::vector<uint8_t>& output, const uint32_t value) {
    output.push_back(static_cast<uint8_t>(value));
    output.push_back(static_cast<uint8_t>(value >> 8U));
    output.push_back(static_cast<uint8_t>(value >> 16U));
    output.push_back(static_cast<uint8_t>(value >> 24U));
}

void append_float(std::vector<uint8_t>& output, const float value) {
    static_assert(sizeof(float) == sizeof(uint32_t));
    uint32_t bits = 0;
    std::memcpy(&bits, &value, sizeof(bits));
    append_u32(output, bits);
}

}  // namespace

std::vector<uint8_t> make_glb(const TessellationResult& mesh) {
    if (mesh.vertices.empty() || mesh.triangles.empty()) {
        throw std::invalid_argument("cannot encode an empty mesh as GLB");
    }

    std::vector<uint8_t> binary;
    binary.reserve(mesh.vertices.size() * 12U + mesh.triangles.size() * 12U);
    std::array<double, 3> minimum = {mesh.vertices[0].x, mesh.vertices[0].y, mesh.vertices[0].z};
    std::array<double, 3> maximum = minimum;
    for (const Vec3& vertex : mesh.vertices) {
        minimum[0] = std::min(minimum[0], vertex.x);
        minimum[1] = std::min(minimum[1], vertex.y);
        minimum[2] = std::min(minimum[2], vertex.z);
        maximum[0] = std::max(maximum[0], vertex.x);
        maximum[1] = std::max(maximum[1], vertex.y);
        maximum[2] = std::max(maximum[2], vertex.z);
        append_float(binary, static_cast<float>(vertex.x));
        append_float(binary, static_cast<float>(vertex.y));
        append_float(binary, static_cast<float>(vertex.z));
    }
    const size_t index_offset = binary.size();
    for (const Triangle& triangle : mesh.triangles) {
        append_u32(binary, triangle.v0);
        append_u32(binary, triangle.v1);
        append_u32(binary, triangle.v2);
    }
    while ((binary.size() % 4U) != 0U) binary.push_back(0U);

    std::ostringstream json_stream;
    json_stream << "{\"asset\":{\"version\":\"2.0\",\"generator\":\"occccad Geometry Worker\"},"
                << "\"buffers\":[{\"byteLength\":" << binary.size() << "}],"
                << "\"bufferViews\":["
                << "{\"buffer\":0,\"byteOffset\":0,\"byteLength\":" << index_offset
                << ",\"target\":34962},"
                << "{\"buffer\":0,\"byteOffset\":" << index_offset
                << ",\"byteLength\":" << mesh.triangles.size() * 12U
                << ",\"target\":34963}],"
                << "\"accessors\":["
                << "{\"bufferView\":0,\"componentType\":5126,\"count\":" << mesh.vertices.size()
                << ",\"type\":\"VEC3\",\"min\":[" << minimum[0] << ',' << minimum[1] << ',' << minimum[2]
                << "],\"max\":[" << maximum[0] << ',' << maximum[1] << ',' << maximum[2] << "]},"
                << "{\"bufferView\":1,\"componentType\":5125,\"count\":"
                << mesh.triangles.size() * 3U << ",\"type\":\"SCALAR\"}],"
                << "\"meshes\":[{\"primitives\":[{\"attributes\":{\"POSITION\":0},\"indices\":1}]}],"
                << "\"nodes\":[{\"mesh\":0}],\"scenes\":[{\"nodes\":[0]}],\"scene\":0}";
    std::string json = json_stream.str();
    while ((json.size() % 4U) != 0U) json.push_back(' ');

    constexpr uint32_t kGlbHeaderSize = 12U;
    constexpr uint32_t kChunkHeaderSize = 8U;
    const auto total_size = static_cast<uint32_t>(
        kGlbHeaderSize + kChunkHeaderSize + json.size() + kChunkHeaderSize + binary.size());
    std::vector<uint8_t> output;
    output.reserve(total_size);
    append_u32(output, 0x46546c67U);
    append_u32(output, 2U);
    append_u32(output, total_size);
    append_u32(output, static_cast<uint32_t>(json.size()));
    append_u32(output, 0x4e4f534aU);
    output.insert(output.end(), json.begin(), json.end());
    append_u32(output, static_cast<uint32_t>(binary.size()));
    append_u32(output, 0x004e4942U);
    output.insert(output.end(), binary.begin(), binary.end());
    return output;
}

}  // namespace occccad::kernel
