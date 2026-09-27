#include <occccad/kernel/mesh_glb.hpp>

#include <algorithm>
#include <array>
#include <cmath>
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

std::vector<uint8_t> make_glb(const TessellationResult& mesh, const std::string& association_json) {
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
                if (!std::isfinite(values[i]) || !std::isfinite(static_cast<float>(values[i])))
                    throw std::invalid_argument("nonfinite GLB position");
                lo[i] = std::min(lo[i], values[i]);
                hi[i] = std::max(hi[i], values[i]);
                append_float(binary, static_cast<float>(values[i]));
            }
        }
        std::ostringstream bounds;
        bounds.precision(std::numeric_limits<double>::max_digits10);
        bounds << ",\"min\":[" << lo[0] << ',' << lo[1] << ',' << lo[2] << "],\"max\":[" << hi[0]
               << ',' << hi[1] << ',' << hi[2] << ']';
        return accessor(offset, vertices.size(), "VEC3", 5126, bounds.str());
    };
    const auto positions = points(mesh.vertices);
    std::vector<Vec3> normals(mesh.vertices.size());
    for (const auto& t : mesh.triangles) {
        if (t.v0 >= mesh.vertices.size() || t.v1 >= mesh.vertices.size() ||
            t.v2 >= mesh.vertices.size())
            throw std::invalid_argument("GLB triangle index");
        const auto &a = mesh.vertices[t.v0], &b = mesh.vertices[t.v1], &c = mesh.vertices[t.v2];
        const Vec3 u{b.x - a.x, b.y - a.y, b.z - a.z}, v{c.x - a.x, c.y - a.y, c.z - a.z};
        const Vec3 n{u.y * v.z - u.z * v.y, u.z * v.x - u.x * v.z, u.x * v.y - u.y * v.x};
        for (auto id : {t.v0, t.v1, t.v2}) {
            normals[id].x += n.x;
            normals[id].y += n.y;
            normals[id].z += n.z;
        }
    }
    for (auto& n : normals) {
        const auto length = std::sqrt(n.x * n.x + n.y * n.y + n.z * n.z);
        if (length > 0) {
            n.x /= length;
            n.y /= length;
            n.z /= length;
        } else {
            n = {0, 0, 1};
        }
    }
    const auto normal_offset = binary.size();
    for (const auto& n : normals) {
        append_float(binary, n.x);
        append_float(binary, n.y);
        append_float(binary, n.z);
    }
    const auto normal_accessor = accessor(normal_offset, normals.size(), "VEC3", 5126);
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
    for (auto id : mesh.face_ids) {
        if (id == 0)
            throw std::invalid_argument("zero face locator");
        append_u32(binary, id);
    }
    const auto faces = accessor(offset, mesh.face_ids.size(), "SCALAR", 5125);
    std::ostringstream primitives;
    primitives << "{\"attributes\":{\"POSITION\":" << positions << ",\"NORMAL\":" << normal_accessor
               << "},\"indices\":" << indices
               << ",\"mode\":4,\"extensions\":{\"OCCCCAD_cad\":{\"kind\":\"FACE\",\"localIds\":"
               << faces << "}}}";
    // One indexed LINES primitive keeps JSON/accessor overhead independent of edge count.
    std::vector<Vec3> edge_points;
    std::vector<uint32_t> edge_indices, edge_ids;
    for (const auto& edge : mesh.edges) {
        if (edge.local_id == 0 || edge.local_id > std::numeric_limits<uint32_t>::max())
            throw std::invalid_argument("GLB edge locator");
        if (edge.points.size() < 2)
            continue;
        if (edge.points.size() > std::numeric_limits<uint32_t>::max() - edge_points.size())
            throw std::length_error("GLB edge vertex limit");
        const auto base = static_cast<uint32_t>(edge_points.size());
        edge_points.insert(edge_points.end(), edge.points.begin(), edge.points.end());
        for (size_t i = 1; i < edge.points.size(); ++i) {
            edge_indices.push_back(base + i - 1);
            edge_indices.push_back(base + i);
            edge_ids.push_back(edge.local_id);
        }
    }
    if (!edge_points.empty()) {
        const auto position = points(edge_points);
        offset = binary.size();
        for (auto index : edge_indices)
            append_u32(binary, index);
        const auto line_indices = accessor(offset, edge_indices.size(), "SCALAR", 5125);
        offset = binary.size();
        for (auto id : edge_ids)
            append_u32(binary, id);
        const auto ids = accessor(offset, edge_ids.size(), "SCALAR", 5125);
        primitives << ",{\"attributes\":{\"POSITION\":" << position
                   << "},\"indices\":" << line_indices
                   << ",\"mode\":1,\"extensions\":{\"OCCCCAD_cad\":{\"kind\":\"EDGE\",\"localIds\":"
                   << ids << "}}}";
    }
    if (!mesh.topology_vertices.empty()) {
        std::vector<Vec3> vertices;
        for (const auto& vertex : mesh.topology_vertices)
            vertices.push_back(vertex.point);
        const auto position = points(vertices);
        offset = binary.size();
        for (const auto& vertex : mesh.topology_vertices) {
            if (vertex.local_id == 0 || vertex.local_id > std::numeric_limits<uint32_t>::max())
                throw std::invalid_argument("GLB vertex locator");
            append_u32(binary, vertex.local_id);
        }
        const auto ids = accessor(offset, vertices.size(), "SCALAR", 5125);
        primitives
            << ",{\"attributes\":{\"POSITION\":" << position
            << "},\"mode\":0,\"extensions\":{\"OCCCCAD_cad\":{\"kind\":\"VERTEX\",\"localIds\":"
            << ids << "}}}";
    }
    const auto cad = std::string(
                         "{\"schemaVersion\":2,\"units\":\"mm\",\"coordinateSpace\":\"PART_LOCAL\","
                         "\"association\":") +
                     association_json + "}";
    std::ostringstream json_stream;
    json_stream << "{\"asset\":{\"version\":\"2.0\",\"generator\":\"occccad\"},\"extensionsUsed\":["
                   "\"OCCCCAD_cad\"],\"extensions\":{\"OCCCCAD_cad\":"
                << cad << "},\"buffers\":[{\"byteLength\":" << binary.size()
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
    json_stream << "],\"meshes\":[{\"primitives\":[" << primitives.str()
                << "]}],\"nodes\":[{\"mesh\":0}],\"scenes\":[{\"nodes\":[0]}],\"scene\":0}";
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
