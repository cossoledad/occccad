// occccad persistent topology naming value contract.
//
// These types deliberately contain no OCCT labels, TopoDS shapes, or traversal
// indices. OCCT adapters produce this evidence; Revision and Artifact layers
// decide how it is persisted.

#ifndef OCCCCAD_TOPOLOGY_NAMING_HPP
#define OCCCCAD_TOPOLOGY_NAMING_HPP

#include <cstdint>
#include <optional>
#include <string>
#include <string_view>
#include <vector>

#include <occccad/kernel/kernel.hpp>

namespace occccad::kernel {

inline constexpr std::uint32_t topology_naming_schema_version = 1;
inline constexpr std::string_view topology_naming_policy_id =
    "occccad.topology.naming.v1";
inline constexpr std::string_view topology_evaluator_version =
    "occccad.topology.contract.v1";
inline constexpr double topology_linear_tolerance_meters = 1.0e-7;
inline constexpr double topology_angular_tolerance_radians = 1.0e-9;

enum class PersistentTopologyType : std::uint8_t { unspecified, face, edge, vertex };
enum class SelectionRecipeKind : std::uint8_t {
    unspecified,
    direct_semantic_output,
    lineage_descendant,
    intersection_of,
    adjacent_to,
    owned_by_region_boundary,
};
enum class TopologyLineageKind : std::uint8_t {
    unspecified,
    generated,
    modified,
    split,
    merged,
    unchanged,
};
enum class SelectionResolutionStatus : std::uint8_t {
    unspecified,
    resolved,
    missing,
    ambiguous,
    type_mismatch,
    source_unavailable,
    contract_mismatch,
    outside_current_tip,
};
enum class SupportingElementStatus : std::uint8_t { unspecified, connected, not_connected };
enum class AssemblyConstraintEvaluationStatus : std::uint8_t {
    unspecified,
    not_updated,
    broken,
    impossible,
    verified,
};

struct SemanticTopologyRef {
    std::string feature_id;
    std::string output_slot;
    std::vector<std::string> source_ids;
};

struct SelectionEvidence {
    std::string geometry_type;
    std::optional<double> measure_si;
    std::string measure_dimension;
    Vec3 centroid;
    Vec3 origin;
    Vec3 direction;
    std::vector<SemanticTopologyRef> adjacent;
    std::string evidence_digest;
};

struct PersistentSelection {
    std::uint32_t schema_version{1};
    std::string source_document_id;
    std::string source_body_id;
    SemanticTopologyRef anchor;
    PersistentTopologyType expected_type{PersistentTopologyType::unspecified};
    SelectionRecipeKind recipe{SelectionRecipeKind::unspecified};
    std::vector<SemanticTopologyRef> operands;
    SelectionEvidence creation_evidence;
};

struct ResolvedTopologyElement {
    GeometryId geometry_id;
    std::string geometry_key;
    PersistentTopologyType topology_type{PersistentTopologyType::unspecified};
    std::uint64_t local_id{};  // Valid only inside geometry_id/geometry_key.
    SemanticTopologyRef semantic_ref;
    SelectionEvidence evidence;
};

struct SelectionResolution {
    SelectionResolutionStatus status{SelectionResolutionStatus::unspecified};
    SupportingElementStatus supporting_element_status{SupportingElementStatus::unspecified};
    std::vector<ResolvedTopologyElement> candidates;
    std::string diagnostic_code;
    std::string diagnostic;
    std::string evidence_digest;
};

struct TopologyLineage {
    std::vector<SemanticTopologyRef> sources;
    SemanticTopologyRef result;
    TopologyLineageKind kind{TopologyLineageKind::unspecified};
    SelectionEvidence evidence;
};

struct TopologyTombstone {
    SemanticTopologyRef source;
    std::string reason;
    SelectionEvidence evidence;
};

struct AmbiguousLineage {
    std::vector<SemanticTopologyRef> sources;
    std::vector<SemanticTopologyRef> candidates;
    std::string diagnostic_code;
};

struct TopologyHistory {
    std::uint32_t schema_version{1};
    std::string feature_id;
    GeometryId input_geometry_id;
    GeometryId result_geometry_id;
    std::vector<TopologyLineage> lineage;
    std::vector<TopologyTombstone> deleted;
    std::vector<AmbiguousLineage> ambiguous;
    std::string evidence_digest;
    std::string policy_digest;
};

}  // namespace occccad::kernel

#endif  // OCCCCAD_TOPOLOGY_NAMING_HPP
