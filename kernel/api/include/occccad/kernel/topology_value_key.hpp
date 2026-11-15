#pragma once
#include <occccad/kernel/topology_naming_values.hpp>

#include <algorithm>
#include <cstring>
#include <functional>
#include <type_traits>

namespace occccad::kernel::detail {
// Exact working-value equality. Addresses and hashes are never persistent IDs.
inline uint64_t bits(double value) {
    uint64_t result;
    std::memcpy(&result, &value, sizeof(result));
    return result;
}
inline bool equal_vec(const Vec3& a, const Vec3& b) {
    return bits(a.x) == bits(b.x) && bits(a.y) == bits(b.y) && bits(a.z) == bits(b.z);
}
inline bool equal_real(const std::optional<double>& a, const std::optional<double>& b) {
    return a.has_value() == b.has_value() && (!a || bits(*a) == bits(*b));
}
inline bool equal_ref(const SemanticTopologyRef& a, const SemanticTopologyRef& b) {
    return a.feature_id == b.feature_id && a.output_slot == b.output_slot &&
           a.source_ids == b.source_ids;
}
struct RefHash {
    size_t operator()(const SemanticTopologyRef* ref) const {
        size_t result = std::hash<std::string>{}(ref->feature_id);
        const auto add = [&](const auto& value) {
            result ^= std::hash<std::decay_t<decltype(value)>>{}(value) + 0x9e3779b9 +
                      (result << 6) + (result >> 2);
        };
        add(ref->output_slot);
        add(ref->source_ids.size());
        for (const auto& id : ref->source_ids)
            add(id);
        return result;
    }
};
struct RefEqual {
    bool operator()(const SemanticTopologyRef* a, const SemanticTopologyRef* b) const {
        return equal_ref(*a, *b);
    }
};
struct EvidenceHash {
    size_t operator()(const SelectionEvidence* e) const {
        size_t result = 0;
        const auto add = [&](const auto& value) {
            result ^= std::hash<std::decay_t<decltype(value)>>{}(value) + 0x9e3779b9 +
                      (result << 6) + (result >> 2);
        };
        const auto vec = [&](const auto& value) {
            add(bits(value.x));
            add(bits(value.y));
            add(bits(value.z));
        };
        const auto real = [&](const auto& value) {
            add(value.has_value());
            if (value)
                add(bits(*value));
        };
        add(e->geometry_type);
        add(e->measure_dimension);
        add(e->endpoint_role);
        add(e->evidence_digest);
        vec(e->centroid);
        vec(e->origin);
        vec(e->direction);
        real(e->measure_si);
        real(e->parameter_start);
        real(e->parameter_end);
        real(e->radius_mm);
        real(e->minor_radius_mm);
        real(e->half_angle_radians);
        add(e->cone_leaf.has_value());
        if (e->cone_leaf)
            add(*e->cone_leaf);
        add(e->material_side.has_value());
        if (e->material_side)
            add(*e->material_side);
        add(e->x_direction.has_value());
        if (e->x_direction)
            vec(*e->x_direction);
        add(e->adjacent.size());
        for (const auto& adjacent : e->adjacent)
            add(RefHash{}(&adjacent));
        return result;
    }
};
struct EvidenceEqual {
    bool operator()(const SelectionEvidence* a, const SelectionEvidence* b) const {
        return a->geometry_type == b->geometry_type &&
               a->measure_dimension == b->measure_dimension &&
               a->endpoint_role == b->endpoint_role && a->evidence_digest == b->evidence_digest &&
               equal_vec(a->centroid, b->centroid) && equal_vec(a->origin, b->origin) &&
               equal_vec(a->direction, b->direction) && equal_real(a->measure_si, b->measure_si) &&
               equal_real(a->parameter_start, b->parameter_start) &&
               equal_real(a->parameter_end, b->parameter_end) &&
               equal_real(a->radius_mm, b->radius_mm) &&
               equal_real(a->minor_radius_mm, b->minor_radius_mm) &&
               equal_real(a->half_angle_radians, b->half_angle_radians) &&
               a->cone_leaf == b->cone_leaf && a->material_side == b->material_side &&
               a->x_direction.has_value() == b->x_direction.has_value() &&
               (!a->x_direction || equal_vec(*a->x_direction, *b->x_direction)) &&
               a->adjacent.size() == b->adjacent.size() &&
               std::equal(a->adjacent.begin(), a->adjacent.end(), b->adjacent.begin(), equal_ref);
    }
};
}  // namespace occccad::kernel::detail
