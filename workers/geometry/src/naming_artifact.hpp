#pragma once
#include <occccad/worker/v1/geometry_worker.pb.h>
#include <occccad/kernel/topology_naming.hpp>

#include <unordered_map>
#include <stdexcept>
#include <string>
#include <vector>
#include "naming_value_access.hpp"

namespace occccad::worker {
using NativeRefTable = std::unordered_map<const kernel::SemanticTopologyRef*, uint32_t, naming_detail::RefHash, naming_detail::RefEqual>;

template<class Visit>
inline void visit_naming_dictionary(const v1::PartTopologyManifest& manifest, Visit visit) {
    if (manifest.semantic_refs_size()) throw std::invalid_argument("NAMING_REFERENCE_ENCODING_AMBIGUOUS");
    const auto& table = manifest.reference_dictionary();
    const auto symbol = [&](uint32_t id) -> const std::string& {
        if (!id || id > static_cast<uint32_t>(table.symbols_size()))
            throw std::invalid_argument("NAMING_REFERENCE_SYMBOL_INVALID");
        return table.symbols(id - 1);
    };
    std::vector<kernel::SharedVector<std::string>> sources(table.source_lists_size() + 1);
    for (int i = 0; i < table.source_lists_size(); ++i) {
        const auto& ids = table.source_lists(i).symbols();
        if (ids.empty()) throw std::invalid_argument("NAMING_REFERENCE_LIST_EMPTY");
        auto& list = sources[i + 1];
        list.reserve(ids.size());
        for (const auto id : ids) list.push_back(symbol(id));
    }
    for (const auto& row : table.rows()) {
        if (row.sources() >= sources.size()) throw std::invalid_argument("NAMING_REFERENCE_LIST_INVALID");
        visit(symbol(row.feature()), symbol(row.slot()), sources[row.sources()]);
    }
}

inline void expand_naming_references(v1::PartTopologyManifest& manifest) {
    if (!manifest.has_reference_dictionary()) return;
    visit_naming_dictionary(manifest, [&](const auto& feature, const auto& slot, const auto& sources) {
        auto* ref = manifest.add_semantic_refs();
        ref->set_feature_id(feature);
        ref->set_output_slot(slot);
        for (const auto& source : sources) ref->add_source_ids(source);
    });
    manifest.clear_reference_dictionary();
}

// Restore typed geometry evidence from the immutable v2 artifact. This is the
// inverse of pack_naming, not a reconstruction from display coordinates.
template<class ResolveRef>
inline kernel::SelectionEvidence decode_naming_evidence(const v1::NamingEvidence& source, ResolveRef resolve) {
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
    out.adjacent.reserve(source.adjacent_size());
    for (auto id : source.adjacent()) out.adjacent.push_back(resolve(id));
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

inline kernel::SelectionEvidence read_naming_evidence(const v1::NamingEvidence& source,
                                                      const v1::PartTopologyManifest& manifest) {
    return decode_naming_evidence(source, [&](uint32_t id) -> kernel::SemanticTopologyRef {
        if (!id || id > static_cast<unsigned>(manifest.semantic_refs_size()))
            throw std::invalid_argument("NAMING_EVIDENCE_REF_INVALID");
        const auto& ref = manifest.semantic_refs(id - 1);
        return {ref.feature_id(), ref.output_slot(), {ref.source_ids().begin(), ref.source_ids().end()}};
    });
}

// Protobuf readers may reuse scratch. Native readers must borrow records that
// remain immutable for this whole call. Only the compact v2
// representation is retained; IDs still follow first occurrence, never hash
// table iteration. Full wire keys retain exact equality, including unknowns.
template<class ReadFeature>
inline void pack_naming(size_t count, ReadFeature read, v1::PartTopologyManifest& out, bool compact = false,
                        const NativeRefTable* previous_refs = nullptr) {
    if (!count) throw std::invalid_argument("NAMING_OWNER_REQUIRED");
    const auto& first=read(0);
    const bool geometric=naming_detail::has_result(first);
    if(geometric ? (!naming_detail::body_id(first).empty() || naming_detail::owner_result(first).feature_id().empty() || naming_detail::owner_result(first).result_id().empty()) : naming_detail::body_id(first).empty())
        throw std::invalid_argument("NAMING_OWNER_INVALID");
    const auto body_id=naming_detail::body_id(first);
    const auto result_owner=naming_detail::owner_result(first);
    const auto result_identity=result_owner.SerializeAsString();
    out.set_schema_version(3);
    out.set_coordinate_space("PART_LOCAL");
    out.set_length_unit("mm");
    std::unordered_map<std::string, uint32_t> refs, evidence;
    auto* dictionary = compact ? out.mutable_reference_dictionary() : nullptr;
    std::unordered_map<std::string_view, uint32_t> symbols;
    std::unordered_map<std::string, uint32_t> source_lists;
    const auto symbol = [&](const std::string& value) {
        if (const auto found = symbols.find(value); found != symbols.end()) return found->second;
        const auto id = static_cast<uint32_t>(symbols.size() + 1);
        auto& stored = *dictionary->add_symbols();
        stored = value;
        symbols.emplace(stored, id);
        return id;
    };
    auto dictionary_row = [&](const std::string& feature, const std::string& slot, const auto& ids) {
        auto& row = *dictionary->add_rows();
        row.set_feature(symbol(feature));
        row.set_slot(symbol(slot));
        if (!ids.empty()) {
            v1::NamingSourceList sources;
            for (const auto& id : ids) sources.add_symbols(symbol(id));
            const auto [list, inserted] = source_lists.emplace(sources.SerializeAsString(), source_lists.size() + 1);
            if (inserted) dictionary->add_source_lists()->Swap(&sources);
            row.set_sources(list->second);
        }
    };
    NativeRefTable native_refs;
    std::unordered_map<const kernel::SelectionEvidence*,uint32_t,naming_detail::EvidenceHash,naming_detail::EvidenceEqual> native_evidence;
    const auto ref = [&](const auto& value) {
        if constexpr(std::is_same_v<std::decay_t<decltype(value)>,kernel::SemanticTopologyRef>) {
            const auto [entry, added] = native_refs.emplace(&value, native_refs.size() + 1);
            if (added) {
                if (dictionary) dictionary_row(value.feature_id, value.output_slot, value.source_ids);
                else naming_detail::encode_ref(value, out.add_semantic_refs());
            }
            return entry->second;
        } else {
            // Wire values can carry unknown fields; include those in their identity.
            const auto [entry, added] = refs.emplace(value.SerializeAsString(), refs.size() + 1);
            if (added) {
                if (dictionary) dictionary_row(value.feature_id(), value.output_slot(), value.source_ids());
                else *out.add_semantic_refs() = value;
            }
            return entry->second;
        }
    };
    auto wire_ev = [&](const v1::SelectionEvidence& value, const auto& adjacency) {
        v1::NamingEvidence packed;
        if(value.has_centroid()) *packed.mutable_centroid() = value.centroid();
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
        for (const auto& adjacent : adjacency) {
            if constexpr (std::is_same_v<std::decay_t<decltype(adjacent)>, kernel::SemanticTopologyRef>) {
                if (previous_refs) {
                    const auto previous = previous_refs->find(&adjacent);
                    if (previous != previous_refs->end()) {
                        packed.add_adjacent((previous->second << 1) | 1);
                        continue;
                    }
                }
            }
            const auto local = ref(adjacent);
            if (previous_refs && local > UINT32_MAX / 2)
                throw std::invalid_argument("NAMING_LOCAL_REFERENCE_LIMIT");
            packed.add_adjacent(previous_refs ? local << 1 : local);
        }
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
            out.add_evidence()->Swap(&packed);
        return it->second;
    };
    const auto ev = [&](const auto& value) {
        if constexpr(std::is_same_v<std::decay_t<decltype(value)>,kernel::SelectionEvidence>) {
            const auto found=native_evidence.find(&value);if(found!=native_evidence.end())return found->second;
            v1::SelectionEvidence converted;naming_detail::encode_evidence(value,&converted,false);
            const auto id=wire_ev(converted,value.adjacent);native_evidence.emplace(&value,id);return id;
        } else return wire_ev(value,value.adjacent());
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
