#pragma once
#include <occccad/kernel/topology_naming.hpp>
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
    encode_vec(source.centroid,target->mutable_centroid());encode_vec(source.origin,target->mutable_origin());encode_vec(source.direction,target->mutable_direction());
    for(const auto& adjacent:source.adjacent)encode_ref(adjacent,target->add_adjacent());
    target->set_evidence_digest(source.evidence_digest);
}

inline uint64_t bits(double value){uint64_t result;std::memcpy(&result,&value,sizeof(result));return result;}
inline bool equal_vec(const kernel::Vec3& a,const kernel::Vec3& b){return bits(a.x)==bits(b.x)&&bits(a.y)==bits(b.y)&&bits(a.z)==bits(b.z);}
inline bool equal_real(const std::optional<double>& a,const std::optional<double>& b){return a.has_value()==b.has_value()&&(!a||bits(*a)==bits(*b));}
inline bool equal_ref(const kernel::SemanticTopologyRef& a,const kernel::SemanticTopologyRef& b){return a.feature_id==b.feature_id&&a.output_slot==b.output_slot&&a.source_ids==b.source_ids;}
struct RefHash {
    size_t operator()(const kernel::SemanticTopologyRef* ref)const {
        size_t result=std::hash<std::string>{}(ref->feature_id);
        const auto add=[&](const auto& value){result^=std::hash<std::decay_t<decltype(value)>>{}(value)+0x9e3779b9+(result<<6)+(result>>2);};
        add(ref->output_slot);add(ref->source_ids.size());for(const auto& id:ref->source_ids)add(id);return result;
    }
};
struct RefEqual {bool operator()(const kernel::SemanticTopologyRef* a,const kernel::SemanticTopologyRef* b)const{return equal_ref(*a,*b);}};
struct EvidenceHash {
    size_t operator()(const kernel::SelectionEvidence* e)const {
        size_t result=0;
        const auto add=[&](const auto& value){result^=std::hash<std::decay_t<decltype(value)>>{}(value)+0x9e3779b9+(result<<6)+(result>>2);};
        const auto vec=[&](const auto& value){add(bits(value.x));add(bits(value.y));add(bits(value.z));};
        const auto real=[&](const auto& value){add(value.has_value());if(value)add(bits(*value));};
        add(e->geometry_type);add(e->measure_dimension);add(e->endpoint_role);add(e->evidence_digest);
        vec(e->centroid);vec(e->origin);vec(e->direction);real(e->measure_si);real(e->parameter_start);real(e->parameter_end);real(e->radius_mm);real(e->minor_radius_mm);real(e->half_angle_radians);
        add(e->cone_leaf.has_value());if(e->cone_leaf)add(*e->cone_leaf);add(e->material_side.has_value());if(e->material_side)add(*e->material_side);
        add(e->x_direction.has_value());if(e->x_direction)vec(*e->x_direction);
        add(e->adjacent.size());for(const auto& adjacent:e->adjacent)add(RefHash{}(&adjacent));return result;
    }
};
struct EvidenceEqual {
    bool operator()(const kernel::SelectionEvidence* a,const kernel::SelectionEvidence* b)const {
        return a->geometry_type==b->geometry_type&&a->measure_dimension==b->measure_dimension&&a->endpoint_role==b->endpoint_role&&a->evidence_digest==b->evidence_digest&&
            equal_vec(a->centroid,b->centroid)&&equal_vec(a->origin,b->origin)&&equal_vec(a->direction,b->direction)&&
            equal_real(a->measure_si,b->measure_si)&&equal_real(a->parameter_start,b->parameter_start)&&equal_real(a->parameter_end,b->parameter_end)&&
            equal_real(a->radius_mm,b->radius_mm)&&equal_real(a->minor_radius_mm,b->minor_radius_mm)&&equal_real(a->half_angle_radians,b->half_angle_radians)&&
            a->cone_leaf==b->cone_leaf&&a->material_side==b->material_side&&a->x_direction.has_value()==b->x_direction.has_value()&&(!a->x_direction||equal_vec(*a->x_direction,*b->x_direction))&&
            a->adjacent.size()==b->adjacent.size()&&std::equal(a->adjacent.begin(),a->adjacent.end(),b->adjacent.begin(),equal_ref);
    }
};
}
