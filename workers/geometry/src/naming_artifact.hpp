#pragma once
#include <occccad/worker/v1/geometry_worker.pb.h>

#include <map>
#include <stdexcept>
#include <string>
#include <vector>

namespace occccad::worker {
// FeatureResult is evaluator working memory. Only this interned v2 representation is persisted.
inline void pack_naming(const std::vector<v1::FeatureResult>& features,
                        v1::PartTopologyManifest& out) {
    if (features.empty() || features.front().body_id().empty()) throw std::invalid_argument("NAMING_REQUIRES_ONE_BODY");
    for (const auto& feature : features)
        if (feature.body_id()!=features.front().body_id()) throw std::invalid_argument("NAMING_CROSS_BODY_TRANSITION");
    out.set_schema_version(2);
    out.set_coordinate_space("PART_LOCAL");
    out.set_length_unit("mm");
    std::map<std::string, uint32_t> refs, evidence, bodies;
    auto ref = [&](const v1::SemanticTopologyRef& value) {
        const auto key = value.SerializeAsString();
        auto [it, added] = refs.emplace(key, refs.size() + 1);
        if (added)
            *out.add_semantic_refs() = value;
        return it->second;
    };
    auto ev = [&](const v1::SelectionEvidence& value) {
        v1::NamingEvidence packed;
        *packed.mutable_centroid() = value.centroid();
        if (value.has_measure_si())
            packed.set_measure_si(value.measure_si());
        packed.set_measure_dimension(value.measure_dimension());
        packed.set_evidence_digest(value.evidence_digest());
        packed.set_endpoint_role(value.endpoint_role());
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
    // Tips are selected per Body, never by the last global FeatureResult.
    std::map<std::string, size_t> tips;
    for (size_t i = 0; i < features.size(); ++i)
        tips[features[i].body_id()] = i;
    for (size_t i = 0; i < features.size(); ++i) {
        const auto& f = features[i];
        const auto& h = f.topology_history();
        auto [it, added] = bodies.emplace(f.body_id(), bodies.size());
        if (added)
            out.add_bodies()->set_body_id(f.body_id());
        auto* body = out.mutable_bodies(it->second);
        auto* t = out.add_transitions();
        body->add_transitions(out.transitions_size());
        t->set_feature_id(f.feature_id());
        t->set_body_id(f.body_id());
        t->set_input_feature_id(f.input_feature_id());
        t->set_profile_feature_id(f.profile_feature_id());
        t->set_input_geometry_id(h.input_geometry_id());
        t->set_result_geometry_id(f.result_geometry_id());
        t->set_evidence_digest(h.evidence_digest());
        t->set_complete(f.topology_history_complete());
        for (const auto& d : f.diagnostics())
            t->add_diagnostics(d);
        for (const auto& l : h.lineage()) {
            auto* p = t->add_lineage();
            for (const auto& s : l.sources())
                p->add_sources(ref(s));
            p->set_result(ref(l.result()));
            p->set_kind(l.kind());
            p->set_evidence(ev(l.evidence()));
        }
        for (const auto& d : h.deleted()) {
            auto* p = t->add_deleted();
            p->set_source(ref(d.source()));
            p->set_reason(d.reason());
            p->set_evidence(ev(d.evidence()));
        }
        for (const auto& a : h.ambiguous()) {
            auto* p = t->add_ambiguous();
            for (const auto& s : a.sources())
                p->add_sources(ref(s));
            for (const auto& c : a.candidates())
                p->add_candidates(ref(c));
            p->set_diagnostic_code(a.diagnostic_code());
        }
        if (tips.at(f.body_id()) == i) {
            body->set_tip_transition(out.transitions_size());
            for (const auto& o : f.semantic_outputs()) {
                auto* p = body->add_tip();
                p->set_topology_type(o.topology_type());
                p->set_local_id(o.local_id());
                p->set_semantic_ref(ref(o.semantic_ref()));
                p->set_evidence(ev(o.evidence()));
            }
        }
    }
}
}  // namespace occccad::worker
