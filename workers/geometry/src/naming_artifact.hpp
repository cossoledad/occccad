#pragma once
#include <occccad/worker/v1/geometry_worker.pb.h>
#include <occccad/kernel/topology_naming.hpp>

#include <unordered_map>
#include <stdexcept>
#include <string>
#include <vector>
#include "naming_value_access.hpp"

namespace occccad::worker {
// Restore typed geometry evidence from the immutable v2 artifact. This is the
// inverse of pack_naming, not a reconstruction from display coordinates.
inline kernel::SelectionEvidence read_naming_evidence(const v1::NamingEvidence& source,
                                                     const v1::PartTopologyManifest& manifest) {
    kernel::SelectionEvidence out;
    const auto vec=[](const v1::Vec3& v){return kernel::Vec3{v.x(),v.y(),v.z()};};
    out.centroid=vec(source.centroid());out.evidence_digest=source.evidence_digest();
    out.measure_dimension=source.measure_dimension();out.endpoint_role=source.endpoint_role();
    if(source.has_measure_si())out.measure_si=source.measure_si();
    if(source.has_radius_mm())out.radius_mm=source.radius_mm();
    if(source.has_minor_radius_mm())out.minor_radius_mm=source.minor_radius_mm();
    if(source.has_half_angle_radians())out.half_angle_radians=source.half_angle_radians();
    if(source.has_cone_leaf())out.cone_leaf=source.cone_leaf();
    if(source.has_material_side())out.material_side=source.material_side();
    if(source.has_x_direction())out.x_direction=vec(source.x_direction());
    for(auto id:source.adjacent()) {
        if(!id||id>static_cast<unsigned>(manifest.semantic_refs_size()))throw std::invalid_argument("NAMING_EVIDENCE_REF_INVALID");
        const auto& r=manifest.semantic_refs(id-1);
        out.adjacent.push_back({r.feature_id(),r.output_slot(),{r.source_ids().begin(),r.source_ids().end()}});
    }
    const v1::NamingFrame* frame=nullptr;const v1::NamingCurveRange* range=nullptr;
    switch(source.geometry_case()) {
        case v1::NamingEvidence::kPlane:out.geometry_type="PLANE";frame=&source.plane();break;
        case v1::NamingEvidence::kCylinder:out.geometry_type="CYLINDER";frame=&source.cylinder();break;
        case v1::NamingEvidence::kPoint:out.geometry_type="POINT";out.origin=vec(source.point().position());break;
        case v1::NamingEvidence::kLine:out.geometry_type=source.line().family();frame=&source.line().frame();range=&source.line().range();break;
        case v1::NamingEvidence::kCircle:out.geometry_type=source.circle().family();frame=&source.circle().frame();range=&source.circle().range();break;
        case v1::NamingEvidence::kSpline:out.geometry_type=source.spline().family();frame=&source.spline().frame();range=&source.spline().range();break;
        case v1::NamingEvidence::kOther:out.geometry_type=source.other().family();frame=&source.other().frame();range=&source.other().range();break;
        default:throw std::invalid_argument("NAMING_EVIDENCE_GEOMETRY_MISSING");
    }
    if(frame){out.origin=vec(frame->origin());out.direction=vec(frame->direction());}
    if(range){if(range->has_start())out.parameter_start=range->start();if(range->has_end())out.parameter_end=range->end();}
    return out;
}

// Protobuf readers may reuse scratch. Native readers must borrow records that
// remain immutable for this whole call. Only the compact v2
// representation is retained; IDs still follow first occurrence, never hash
// table iteration. Full wire keys retain exact equality, including unknowns.
template<class ReadFeature>
inline void pack_naming(size_t count, ReadFeature read, v1::PartTopologyManifest& out) {
    if (!count) throw std::invalid_argument("NAMING_OWNER_REQUIRED");
    const auto& first=read(0);
    const bool geometric=naming_detail::has_result(first);
    if(geometric ? (!naming_detail::body_id(first).empty() || naming_detail::owner_result(first).feature_id().empty() || naming_detail::owner_result(first).result_id().empty()) : naming_detail::body_id(first).empty())
        throw std::invalid_argument("NAMING_OWNER_INVALID");
    const auto body_id=naming_detail::body_id(first);
    const auto result_owner=naming_detail::owner_result(first);
    const auto result_identity=result_owner.SerializeAsString();
    out.set_schema_version(2);
    out.set_coordinate_space("PART_LOCAL");
    out.set_length_unit("mm");
    std::unordered_map<std::string, uint32_t> refs, evidence;
    auto wire_ref = [&](const v1::SemanticTopologyRef& value) {
        const auto key = value.SerializeAsString();
        auto [it, added] = refs.emplace(key, refs.size() + 1);
        if (added)
            *out.add_semantic_refs() = value;
        return it->second;
    };
    // Native keys borrow immutable records, but hash/compare complete values.
    // A hit only skips conversion; full protobuf wire keys remain authoritative
    // on a miss (including ignored fields, signed zero and optional presence).
    std::unordered_map<const kernel::SemanticTopologyRef*,uint32_t,naming_detail::RefHash,naming_detail::RefEqual> native_refs;
    std::unordered_map<const kernel::SelectionEvidence*,uint32_t,naming_detail::EvidenceHash,naming_detail::EvidenceEqual> native_evidence;
    const auto ref = [&](const auto& value) {
        if constexpr(std::is_same_v<std::decay_t<decltype(value)>,kernel::SemanticTopologyRef>) {
            const auto found=native_refs.find(&value);if(found!=native_refs.end())return found->second;
            v1::SemanticTopologyRef converted;naming_detail::encode_ref(value,&converted);
            const auto id=wire_ref(converted);native_refs.emplace(&value,id);return id;
        } else return wire_ref(value);
    };
    auto wire_ev = [&](const v1::SelectionEvidence& value) {
        v1::NamingEvidence packed;
        *packed.mutable_centroid() = value.centroid();
        if (value.has_measure_si())
            packed.set_measure_si(value.measure_si());
        packed.set_measure_dimension(value.measure_dimension());
        packed.set_evidence_digest(value.evidence_digest());
        packed.set_endpoint_role(value.endpoint_role());
        if (value.has_radius_mm()) packed.set_radius_mm(value.radius_mm());
        if (value.has_minor_radius_mm()) packed.set_minor_radius_mm(value.minor_radius_mm());
        if (value.has_half_angle_radians()) packed.set_half_angle_radians(value.half_angle_radians());
        if (value.has_cone_leaf()) packed.set_cone_leaf(value.cone_leaf());
        if (value.has_material_side()) packed.set_material_side(value.material_side());
        if (value.has_x_direction()) *packed.mutable_x_direction() = value.x_direction();
        for (const auto& adjacent : value.adjacent())
            packed.add_adjacent(ref(adjacent));
        v1::NamingFrame* frame = nullptr;
        v1::NamingCurveRange* range = nullptr;
        const auto& kind = value.geometry_type();
        if (kind == "PLANE")
            frame = packed.mutable_plane();
        else if (kind == "CYLINDER")
            frame = packed.mutable_cylinder();
        else if (kind == "POINT")
            *packed.mutable_point()->mutable_position() = value.origin();
        else if (kind == "LINE" || kind == "CIRCLE" || kind == "BSPLINE" || kind == "BEZIER") {
            auto* curve = kind == "LINE"     ? packed.mutable_line()
                          : kind == "CIRCLE" ? packed.mutable_circle()
                                             : packed.mutable_spline();
            curve->set_family(kind);
            frame = curve->mutable_frame();
            range = curve->mutable_range();
        } else {
            auto* other = packed.mutable_other();
            other->set_family(kind);
            frame = other->mutable_frame();
            range = other->mutable_range();
        }
        if (frame) {
            *frame->mutable_origin() = value.origin();
            *frame->mutable_direction() = value.direction();
        }
        if (range) {
            if (value.has_parameter_start())
                range->set_start(value.parameter_start());
            if (value.has_parameter_end())
                range->set_end(value.parameter_end());
            range->set_convention(kind == "LINE" ? "LINE_MM"
                                  : (kind == "CIRCLE" || kind == "ELLIPSE")
                                      ? "ANGLE_RADIANS"
                                      : "NATIVE_CURVE_PARAMETER");
        }
        auto [it, added] = evidence.emplace(packed.SerializeAsString(), evidence.size() + 1);
        if (added)
            *out.add_evidence() = packed;
        return it->second;
    };
    const auto ev = [&](const auto& value) {
        if constexpr(std::is_same_v<std::decay_t<decltype(value)>,kernel::SelectionEvidence>) {
            const auto found=native_evidence.find(&value);if(found!=native_evidence.end())return found->second;
            v1::SelectionEvidence converted;naming_detail::encode_evidence(value,&converted);
            const auto id=wire_ev(converted);native_evidence.emplace(&value,id);return id;
        } else return wire_ev(value);
    };
    const auto write_snapshot = [&](auto* body) {
    for (size_t i = 0; i < count; ++i) {
        const auto& f = read(i);
        if (naming_detail::body_id(f)!=body_id || naming_detail::has_result(f)!=geometric || naming_detail::owner_result(f).SerializeAsString()!=result_identity)
            throw std::invalid_argument("NAMING_CROSS_OWNER_TRANSITION");
        const auto& h = naming_detail::topology_history(f);
        auto* t = out.add_transitions();
        body->add_transitions(out.transitions_size());
        t->set_feature_id(naming_detail::feature_id(f));
        t->set_body_id(naming_detail::body_id(f));
        if(naming_detail::has_result(f))*t->mutable_result()=naming_detail::owner_result(f);
        t->set_input_feature_id(naming_detail::input_feature_id(f));
        t->set_profile_feature_id(naming_detail::profile_feature_id(f));
        t->set_input_geometry_id(naming_detail::input_geometry_id(h));
        t->set_result_geometry_id(naming_detail::result_geometry_id(f));
        t->set_evidence_digest(naming_detail::evidence_digest(h));
        t->set_complete(naming_detail::topology_history_complete(f));
        for (const auto& d : naming_detail::diagnostics(f))
            t->add_diagnostics(d);
        for (const auto& l : naming_detail::lineage(h)) {
            auto* p = t->add_lineage();
            for (const auto& s : naming_detail::sources(l))
                p->add_sources(ref(s));
            p->set_result(ref(naming_detail::result(l)));
            p->set_kind(naming_detail::encoded_kind(naming_detail::kind(l)));
            p->set_evidence(ev(naming_detail::evidence(l)));
        }
        for (const auto& d : naming_detail::deleted(h)) {
            auto* p = t->add_deleted();
            p->set_source(ref(naming_detail::source(d)));
            p->set_reason(naming_detail::reason(d));
            p->set_evidence(ev(naming_detail::evidence(d)));
        }
        for (const auto& a : naming_detail::ambiguous(h)) {
            auto* p = t->add_ambiguous();
            for (const auto& s : naming_detail::sources(a))
                p->add_sources(ref(s));
            for (const auto& c : naming_detail::candidates(a))
                p->add_candidates(ref(c));
            p->set_diagnostic_code(naming_detail::diagnostic_code(a));
        }
        if (i+1 == count) {
            body->set_tip_transition(out.transitions_size());
            for (const auto& o : naming_detail::semantic_outputs(f)) {
                auto* p = body->add_tip();
                p->set_topology_type(naming_detail::encoded_type(naming_detail::topology_type(o)));
                p->set_local_id(naming_detail::local_id(o));
                p->set_semantic_ref(ref(naming_detail::semantic_ref(o)));
                p->set_evidence(ev(naming_detail::evidence(o)));
            }
        }
    }
    };
    if(geometric){auto* result=out.add_results();*result->mutable_reference()=result_owner;write_snapshot(result);}
    else {auto* body=out.add_bodies();body->set_body_id(body_id);write_snapshot(body);}
}

inline void pack_naming(const std::vector<v1::FeatureResult>& features, v1::PartTopologyManifest& out) {
    pack_naming(features.size(),[&](size_t i)->const v1::FeatureResult&{return features[i];},out);
}
}  // namespace occccad::worker
