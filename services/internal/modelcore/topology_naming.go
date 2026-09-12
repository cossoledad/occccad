package modelcore

import "fmt"

const (
	TopologyNamingPolicyID                = "occccad.topology.naming.v1"
	TopologyNamingEvaluator               = "occccad.topology.contract.v1"
	TopologyNamingSchemaVersion           = uint32(1)
	TopologyNamingLinearToleranceMeters   = 1e-7
	TopologyNamingAngularToleranceRadians = 1e-9
)

type PersistentTopologyType string

const (
	PersistentTopologyFace   PersistentTopologyType = "FACE"
	PersistentTopologyEdge   PersistentTopologyType = "EDGE"
	PersistentTopologyVertex PersistentTopologyType = "VERTEX"
)

type SelectionRecipeKind string

const (
	SelectionDirectSemanticOutput SelectionRecipeKind = "DIRECT_SEMANTIC_OUTPUT"
	SelectionLineageDescendant    SelectionRecipeKind = "LINEAGE_DESCENDANT"
	SelectionIntersectionOf       SelectionRecipeKind = "INTERSECTION_OF"
	SelectionAdjacentTo           SelectionRecipeKind = "ADJACENT_TO"
	SelectionOwnedByBoundary      SelectionRecipeKind = "OWNED_BY_REGION_BOUNDARY"
)

type TopologyLineageKind string

const (
	TopologyGenerated TopologyLineageKind = "GENERATED"
	TopologyModified  TopologyLineageKind = "MODIFIED"
	TopologySplit     TopologyLineageKind = "SPLIT"
	TopologyMerged    TopologyLineageKind = "MERGED"
	TopologyUnchanged TopologyLineageKind = "UNCHANGED"
)

type SelectionResolutionStatus string

const (
	SelectionResolved          SelectionResolutionStatus = "RESOLVED"
	SelectionMissing           SelectionResolutionStatus = "MISSING"
	SelectionAmbiguous         SelectionResolutionStatus = "AMBIGUOUS"
	SelectionTypeMismatch      SelectionResolutionStatus = "TYPE_MISMATCH"
	SelectionSourceUnavailable SelectionResolutionStatus = "SOURCE_UNAVAILABLE"
	SelectionContractMismatch  SelectionResolutionStatus = "CONTRACT_MISMATCH"
	SelectionOutsideCurrentTip SelectionResolutionStatus = "OUTSIDE_CURRENT_TIP"
)

type SupportingElementStatus string

const (
	SupportingElementConnected    SupportingElementStatus = "CONNECTED"
	SupportingElementNotConnected SupportingElementStatus = "NOT_CONNECTED"
)

type AssemblyConstraintEvaluationStatus string

const (
	AssemblyConstraintNotUpdated AssemblyConstraintEvaluationStatus = "NOT_UPDATED"
	AssemblyConstraintBroken     AssemblyConstraintEvaluationStatus = "BROKEN"
	AssemblyConstraintImpossible AssemblyConstraintEvaluationStatus = "IMPOSSIBLE"
	AssemblyConstraintVerified   AssemblyConstraintEvaluationStatus = "VERIFIED"
)

type SemanticTopologyRef struct {
	FeatureID  string   `json:"featureId"`
	OutputSlot string   `json:"outputSlot"`
	SourceIDs  []string `json:"sourceIds,omitempty"`
}

type SelectionRecipe struct {
	Kind     SelectionRecipeKind   `json:"kind"`
	Operands []SemanticTopologyRef `json:"operands,omitempty"`
}

type TopologySelectionEvidence struct {
	GeometryType     string                `json:"geometryType,omitempty"`
	MeasureSI        *float64              `json:"measureSI,omitempty"`
	MeasureDimension string                `json:"measureDimension,omitempty"`
	Centroid         [3]float64            `json:"centroid,omitempty"`
	Origin           [3]float64            `json:"origin,omitempty"`
	Direction        [3]float64            `json:"direction,omitempty"`
	Adjacent         []SemanticTopologyRef `json:"adjacent,omitempty"`
	EvidenceDigest   string                `json:"evidenceDigest,omitempty"`
}

type PersistentSelection struct {
	SchemaVersion    uint32                    `json:"schemaVersion"`
	SourceDocumentID string                    `json:"sourceDocumentId"`
	SourceBodyID     string                    `json:"sourceBodyId"`
	Anchor           SemanticTopologyRef       `json:"anchor"`
	ExpectedType     PersistentTopologyType    `json:"expectedType"`
	Selector         SelectionRecipe           `json:"selector"`
	CreationEvidence TopologySelectionEvidence `json:"creationEvidence"`
}

func (selection PersistentSelection) Validate() error {
	if selection.SchemaVersion != TopologyNamingSchemaVersion {
		return fmt.Errorf("unsupported persistent selection schema version %d", selection.SchemaVersion)
	}
	if selection.SourceDocumentID == "" || selection.SourceBodyID == "" {
		return fmt.Errorf("persistent selection source document and body are required")
	}
	if selection.Anchor.FeatureID == "" || selection.Anchor.OutputSlot == "" {
		return fmt.Errorf("persistent selection semantic anchor is required")
	}
	if selection.ExpectedType != PersistentTopologyFace && selection.ExpectedType != PersistentTopologyEdge && selection.ExpectedType != PersistentTopologyVertex {
		return fmt.Errorf("unsupported persistent topology type %q", selection.ExpectedType)
	}
	if selection.Selector.Kind == "" {
		return fmt.Errorf("persistent selection recipe is required")
	}
	return nil
}

type ResolvedTopologyElement struct {
	GeometryID  string                    `json:"geometryId"`
	GeometryKey string                    `json:"geometryKey"`
	Type        PersistentTopologyType    `json:"type"`
	LocalID     uint64                    `json:"localId"`
	SemanticRef SemanticTopologyRef       `json:"semanticRef"`
	Evidence    TopologySelectionEvidence `json:"evidence"`
}

type SelectionResolution struct {
	Status                  SelectionResolutionStatus `json:"status"`
	SupportingElementStatus SupportingElementStatus   `json:"supportingElementStatus"`
	Candidates              []ResolvedTopologyElement `json:"candidates,omitempty"`
	DiagnosticCode          string                    `json:"diagnosticCode,omitempty"`
	Diagnostic              string                    `json:"diagnostic,omitempty"`
	EvidenceDigest          string                    `json:"evidenceDigest,omitempty"`
}

func SupportingStatusForResolution(status SelectionResolutionStatus) SupportingElementStatus {
	if status == SelectionResolved {
		return SupportingElementConnected
	}
	return SupportingElementNotConnected
}

type TopologyLineage struct {
	Sources  []SemanticTopologyRef     `json:"sources"`
	Result   SemanticTopologyRef       `json:"result"`
	Kind     TopologyLineageKind       `json:"kind"`
	Evidence TopologySelectionEvidence `json:"evidence"`
}

type TopologyTombstone struct {
	Source   SemanticTopologyRef       `json:"source"`
	Reason   string                    `json:"reason"`
	Evidence TopologySelectionEvidence `json:"evidence"`
}

type AmbiguousTopologyLineage struct {
	Sources        []SemanticTopologyRef `json:"sources"`
	Candidates     []SemanticTopologyRef `json:"candidates"`
	DiagnosticCode string                `json:"diagnosticCode"`
}

type TopologyHistory struct {
	SchemaVersion    uint32                     `json:"schemaVersion"`
	FeatureID        string                     `json:"featureId"`
	InputGeometryID  string                     `json:"inputGeometryId,omitempty"`
	ResultGeometryID string                     `json:"resultGeometryId"`
	Lineage          []TopologyLineage          `json:"lineage,omitempty"`
	Deleted          []TopologyTombstone        `json:"deleted,omitempty"`
	Ambiguous        []AmbiguousTopologyLineage `json:"ambiguous,omitempty"`
	EvidenceDigest   string                     `json:"evidenceDigest"`
	PolicyDigest     string                     `json:"policyDigest"`
}
