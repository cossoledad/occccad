#pragma once
#include <occccad/kernel/topology_naming.hpp>
#include <occccad/kernel/topology_value_key.hpp>
#include <occccad/worker/v1/geometry_worker.pb.h>
#include <algorithm>
#include <cstring>
#include <functional>
#include <type_traits>

namespace occccad::worker::naming_detail {
// One compact encoder accepts immutable evaluator records and protobuf records.
// These adapters borrow fields; they do not build a second FeatureResult graph.
#define OCCCCAD_NAMING_FIELD(name) template<class T> decltype(auto) name(const T& value) { \
    if constexpr(std::is_base_of_v<google::protobuf::MessageLite,T>) return value.name(); \
    else return (value.name); }
OCCCCAD_NAMING_FIELD(feature_id)
OCCCCAD_NAMING_FIELD(body_id)
OCCCCAD_NAMING_FIELD(input_feature_id)
OCCCCAD_NAMING_FIELD(profile_feature_id)
OCCCCAD_NAMING_FIELD(result_geometry_id)
OCCCCAD_NAMING_FIELD(topology_history)
OCCCCAD_NAMING_FIELD(topology_history_complete)
OCCCCAD_NAMING_FIELD(diagnostics)
OCCCCAD_NAMING_FIELD(semantic_outputs)
OCCCCAD_NAMING_FIELD(input_geometry_id)
OCCCCAD_NAMING_FIELD(evidence_digest)
OCCCCAD_NAMING_FIELD(lineage)
OCCCCAD_NAMING_FIELD(deleted)
OCCCCAD_NAMING_FIELD(ambiguous)
OCCCCAD_NAMING_FIELD(sources)
OCCCCAD_NAMING_FIELD(result)
OCCCCAD_NAMING_FIELD(kind)
OCCCCAD_NAMING_FIELD(evidence)
OCCCCAD_NAMING_FIELD(source)
OCCCCAD_NAMING_FIELD(reason)
OCCCCAD_NAMING_FIELD(candidates)
OCCCCAD_NAMING_FIELD(diagnostic_code)
OCCCCAD_NAMING_FIELD(topology_type)
OCCCCAD_NAMING_FIELD(local_id)
OCCCCAD_NAMING_FIELD(semantic_ref)
#undef OCCCCAD_NAMING_FIELD

// A frozen history projects borrowed rows and bindings without materializing
// every old FeatureResult. Both storage encoders use the same value adapters.
#define OCCCCAD_FROZEN_HEADER(name) inline decltype(auto) name(const kernel::FrozenFeatureResult& v){return (v.header().name);}
OCCCCAD_FROZEN_HEADER(feature_id)
OCCCCAD_FROZEN_HEADER(body_id)
OCCCCAD_FROZEN_HEADER(input_feature_id)
OCCCCAD_FROZEN_HEADER(profile_feature_id)
OCCCCAD_FROZEN_HEADER(result_geometry_id)
OCCCCAD_FROZEN_HEADER(topology_history_complete)
OCCCCAD_FROZEN_HEADER(diagnostics)
#undef OCCCCAD_FROZEN_HEADER
#define OCCCCAD_FROZEN_HISTORY(name) inline decltype(auto) name(const kernel::FrozenFeatureResult& v){return (v.header().topology_history.name);}
OCCCCAD_FROZEN_HISTORY(input_geometry_id)
OCCCCAD_FROZEN_HISTORY(evidence_digest)
OCCCCAD_FROZEN_HISTORY(deleted)
OCCCCAD_FROZEN_HISTORY(ambiguous)
#undef OCCCCAD_FROZEN_HISTORY
inline const auto& topology_history(const kernel::FrozenFeatureResult& v){return v;}
inline const auto& lineage(const kernel::FrozenFeatureResult& v){return v.lineage();}
inline auto semantic_outputs(const kernel::FrozenFeatureResult& v){return v.outputs();}
inline bool has_result(const kernel::FrozenFeatureResult& v){return !v.header().result_id.empty();}
inline v1::GeometryResultReference owner_result(const kernel::FrozenFeatureResult& v){v1::GeometryResultReference r;if(has_result(v)){r.set_feature_id(v.header().feature_id);r.set_result_id(v.header().result_id);}return r;}

template<class T> bool has_result(const T& value) {
    if constexpr(std::is_base_of_v<google::protobuf::MessageLite,T>) return value.has_result();
    else return !value.result_id.empty();
}
template<class T> v1::GeometryResultReference owner_result(const T& value) {
    if constexpr(std::is_base_of_v<google::protobuf::MessageLite,T>) return value.result();
    else { v1::GeometryResultReference result;if(!value.result_id.empty()){result.set_feature_id(value.feature_id);result.set_result_id(value.result_id);}return result; }
}
inline auto encoded_kind(v1::TopologyLineageKind kind){return kind;}
inline v1::TopologyLineageKind encoded_kind(kernel::TopologyLineageKind kind) {
    switch(kind) {
        case kernel::TopologyLineageKind::generated:return v1::TOPOLOGY_LINEAGE_GENERATED;
        case kernel::TopologyLineageKind::modified:return v1::TOPOLOGY_LINEAGE_MODIFIED;
        case kernel::TopologyLineageKind::split:return v1::TOPOLOGY_LINEAGE_SPLIT;
        case kernel::TopologyLineageKind::merged:return v1::TOPOLOGY_LINEAGE_MERGED;
        case kernel::TopologyLineageKind::unchanged:return v1::TOPOLOGY_LINEAGE_UNCHANGED;
        default:return v1::TOPOLOGY_LINEAGE_UNSPECIFIED;
    }
}
inline auto encoded_type(v1::PersistentTopologyType type){return type;}
inline v1::PersistentTopologyType encoded_type(kernel::PersistentTopologyType type) {
    switch(type) {
        case kernel::PersistentTopologyType::face:return v1::PERSISTENT_TOPOLOGY_TYPE_FACE;
        case kernel::PersistentTopologyType::edge:return v1::PERSISTENT_TOPOLOGY_TYPE_EDGE;
        case kernel::PersistentTopologyType::vertex:return v1::PERSISTENT_TOPOLOGY_TYPE_VERTEX;
        default:return v1::PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED;
    }
}
inline void encode_ref(const kernel::SemanticTopologyRef& source,v1::SemanticTopologyRef* target) {
    target->set_feature_id(source.feature_id);target->set_output_slot(source.output_slot);
    for(const auto& id:source.source_ids)target->add_source_ids(id);
}
inline void encode_vec(const kernel::Vec3& source,v1::Vec3* target) {target->set_x(source.x);target->set_y(source.y);target->set_z(source.z);}
inline void encode_evidence(const kernel::SelectionEvidence& source,v1::SelectionEvidence* target) {
    target->set_geometry_type(source.geometry_type);
    if(source.radius_mm)target->set_radius_mm(*source.radius_mm);
    if(source.minor_radius_mm)target->set_minor_radius_mm(*source.minor_radius_mm);
    if(source.half_angle_radians)target->set_half_angle_radians(*source.half_angle_radians);
    if(source.cone_leaf)target->set_cone_leaf(*source.cone_leaf);
    if(source.material_side)target->set_material_side(*source.material_side);
    if(source.x_direction)encode_vec(*source.x_direction,target->mutable_x_direction());
    if(source.measure_si)target->set_measure_si(*source.measure_si);
    target->set_measure_dimension(source.measure_dimension);
    if(source.parameter_start)target->set_parameter_start(*source.parameter_start);
    if(source.parameter_end)target->set_parameter_end(*source.parameter_end);
    target->set_endpoint_role(source.endpoint_role);
    if(source.measure_si || source.centroid.x!=0 || source.centroid.y!=0 || source.centroid.z!=0)
        encode_vec(source.centroid,target->mutable_centroid());
    encode_vec(source.origin,target->mutable_origin());encode_vec(source.direction,target->mutable_direction());
    for(const auto& adjacent:source.adjacent)encode_ref(adjacent,target->add_adjacent());
    target->set_evidence_digest(source.evidence_digest);
}

using kernel::detail::RefHash;
using kernel::detail::RefEqual;
using kernel::detail::EvidenceHash;
using kernel::detail::EvidenceEqual;

}
