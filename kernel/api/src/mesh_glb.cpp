#include <occccad/kernel/mesh_glb.hpp>

#include <algorithm>
#include <array>
#include <cstring>
#include <limits>
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

std::vector<uint8_t> make_glb(const TessellationResult& mesh,
                              const std::string& topology_ids_json) {
    if (mesh.vertices.empty() || mesh.triangles.empty()) {
        throw std::invalid_argument("cannot encode an empty mesh as GLB");
    }

    std::vector<uint8_t> binary;
    std::vector<std::string> views, accessors;
    auto accessor = [&](size_t offset, size_t count, const char* type, int component,
                        const std::string& bounds = "") {
        const auto id = accessors.size();
        views.push_back("{\"buffer\":0,\"byteOffset\":" + std::to_string(offset) +
                        ",\"byteLength\":" + std::to_string(binary.size() - offset) + "}");
        accessors.push_back("{\"bufferView\":" + std::to_string(id) + ",\"componentType\":" +
                            std::to_string(component) + ",\"count\":" + std::to_string(count) +
                            ",\"type\":\"" + type + "\"" + bounds + "}");
        return id;
    };
    auto points = [&](const auto& vertices) {
        size_t offset = binary.size();
        std::array<double, 3> lo{vertices[0].x, vertices[0].y, vertices[0].z}, hi = lo;
        for (const auto& v : vertices) {
            const double values[]{v.x, v.y, v.z};
            for (int i = 0; i < 3; ++i) {
                lo[i] = std::min(lo[i], values[i]);
                hi[i] = std::max(hi[i], values[i]);
                append_float(binary, static_cast<float>(values[i]));
            }
        }
        std::ostringstream bounds;
        bounds << ",\"min\":[" << lo[0] << ',' << lo[1] << ',' << lo[2] << "],\"max\":[" << hi[0]
               << ',' << hi[1] << ',' << hi[2] << ']';
        return accessor(offset, vertices.size(), "VEC3", 5126, bounds.str());
    };
    const auto positions = points(mesh.vertices);
    size_t offset = binary.size();
    for (const auto& t : mesh.triangles) {
        append_u32(binary, t.v0);
        append_u32(binary, t.v1);
        append_u32(binary, t.v2);
    }
    const auto indices = accessor(offset, mesh.triangles.size() * 3, "SCALAR", 5125);
    if (mesh.face_ids.size() != mesh.triangles.size())
        throw std::invalid_argument("GLB face mapping count mismatch");
    offset = binary.size();
    for (auto id : mesh.face_ids)
        append_u32(binary, id);
    const auto faces = accessor(offset, mesh.face_ids.size(), "SCALAR", 5125);
    std::ostringstream cad;
    cad << "{\"schemaVersion\":1,\"units\":\"mm\",\"coordinateSpace\":\"PART_LOCAL\",\"faceIds\":"
        << faces << ",\"edges\":[";
    bool first = true;
    for (const auto& edge : mesh.edges) {
        if (edge.points.empty())
            continue;
        if (!first)
            cad << ',';
        first = false;
        cad << "{\"localId\":" << edge.local_id << ",\"positions\":" << points(edge.points) << '}';
    }
    cad << "],\"vertices\":[";
    first = true;
    // Vertex coordinates are small and independent of tessellation vertices.
    for (const auto& vertex : mesh.topology_vertices) {
        if (!first)
            cad << ',';
        first = false;
        cad << "{\"localId\":" << vertex.local_id << ",\"point\":[" << vertex.point.x << ','
            << vertex.point.y << ',' << vertex.point.z << "]}";
    }
    cad << "],\"stableIds\":" << topology_ids_json << "}";
    std::ostringstream json_stream;
    json_stream << "{\"asset\":{\"version\":\"2.0\",\"generator\":\"occccad\"},\"extensionsUsed\":["
                   "\"OCCCCAD_cad\"],\"extensions\":{\"OCCCCAD_cad\":"
                << cad.str() << "},\"buffers\":[{\"byteLength\":" << binary.size()
                << "}],\"bufferViews\":[";
    for (size_t i = 0; i < views.size(); ++i) {
        if (i)
            json_stream << ',';
        json_stream << views[i];
    }
    json_stream << "],\"accessors\":[";
    for (size_t i = 0; i < accessors.size(); ++i) {
        if (i)
            json_stream << ',';
        json_stream << accessors[i];
    }
    json_stream << "],\"meshes\":[{\"primitives\":[{\"attributes\":{\"POSITION\":" << positions
                << "},\"indices\":" << indices
                << "}]}],\"nodes\":[{\"mesh\":0}],\"scenes\":[{\"nodes\":[0]}],\"scene\":0}";
    std::string json = json_stream.str();
    while ((json.size() % 4U) != 0U)
        json.push_back(' ');

    constexpr uint32_t kGlbHeaderSize = 12U;
    constexpr uint32_t kChunkHeaderSize = 8U;
    if (json.size() + binary.size() > std::numeric_limits<uint32_t>::max() - 28ULL)
        throw std::length_error("GLB exceeds its 32-bit container limit");
    const auto total_size = static_cast<uint32_t>(kGlbHeaderSize + kChunkHeaderSize + json.size() +
                                                  kChunkHeaderSize + binary.size());
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
