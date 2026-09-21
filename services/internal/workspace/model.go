package workspace

import (
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const SketchSchemaVersion = uint32(2)

type Mesh struct {
	Vertices         [][3]float64    `json:"vertices"`
	Triangles        [][3]uint32     `json:"triangles"`
	FaceIDs          []uint32        `json:"faceIds"`
	Edges            []MeshEdge      `json:"edges"`
	TopologyVertices []TopologyPoint `json:"topologyVertices"`
}

type MeshEdge struct {
	LocalID uint64       `json:"localId"`
	Points  [][3]float64 `json:"points"`
}
type TopologyPoint struct {
	LocalID uint64     `json:"localId"`
	Point   [3]float64 `json:"point"`
}

type TopologyElementProperties struct {
	GeometryKey         string                         `json:"geometryKey"`
	GeometryID          string                         `json:"geometryId"`
	Kind                string                         `json:"kind"`
	LocalID             uint64                         `json:"localId"`
	GeometryType        string                         `json:"geometryType"`
	BBox                map[string]any                 `json:"bbox,omitempty"`
	Point               *[3]float64                    `json:"point,omitempty"`
	Properties          map[string]any                 `json:"properties"`
	WorkerID            string                         `json:"workerId"`
	OCCTVersion         string                         `json:"occtVersion"`
	NamingStatus        string                         `json:"namingStatus"`
	PersistentSelection *modelcore.PersistentSelection `json:"persistentSelection,omitempty"`
	NamingResolution    *modelcore.SelectionResolution `json:"namingResolution,omitempty"`
}

type Artifact struct {
	GeometryKey      string                `json:"geometryKey"`
	GeometryID       string                `json:"geometryId"`
	Mesh             Mesh                  `json:"mesh"`
	BBox             map[string]any        `json:"bbox"`
	Topology         map[string]any        `json:"topology"`
	Volume           float64               `json:"volume"`
	OCCTVersion      string                `json:"occtVersion"`
	GLBBytes         int                   `json:"glbBytes"`
	BRepBytes        int                   `json:"brepBytes"`
	EvaluatorVersion string                `json:"evaluatorVersion"`
	WorkerID         string                `json:"workerId"`
	StorageState     string                `json:"storageState"`
	CreatedAt        string                `json:"createdAt"`
	Visualization    VisualizationManifest `json:"visualization"`
}

type DatumPlane struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Plane      string     `json:"plane"`
	Origin     [3]float64 `json:"origin"`
	Normal     [3]float64 `json:"normal"`
	UDirection [3]float64 `json:"uDirection"`
	Size       float64    `json:"size"`
}

type AxisSystem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Origin     [3]float64 `json:"origin"`
	XDirection [3]float64 `json:"xDirection"`
	YDirection [3]float64 `json:"yDirection"`
	ZDirection [3]float64 `json:"zDirection"`
}

type DatumAxis struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Origin    [3]float64 `json:"origin"`
	Direction [3]float64 `json:"direction"`
}

type PublicationTarget struct {
	Kind                string                         `json:"kind"` // DATUM | TOPOLOGY | FEATURE_OUTPUT | PARAMETER
	DatumID             string                         `json:"datumId,omitempty"`
	Axis                string                         `json:"axis,omitempty"`
	PersistentSelection *modelcore.PersistentSelection `json:"persistentSelection,omitempty"`
	SourceVersionID     string                         `json:"sourceVersionId,omitempty"`
	FeatureID           string                         `json:"featureId,omitempty"`
	OutputSlot          string                         `json:"outputSlot,omitempty"`
	ParameterID         string                         `json:"parameterId,omitempty"`
}

type PublicationContract struct {
	GeometryKind string               `json:"geometryKind,omitempty"`
	ValueType    modelcore.ValueType  `json:"valueType,omitempty"`
	Dimension    *modelcore.Dimension `json:"dimension,omitempty"`
	UnitPolicy   string               `json:"unitPolicy,omitempty"`
	Bounds       *PublicationBounds   `json:"bounds,omitempty"`
	Symmetry     string               `json:"symmetry,omitempty"`
}

type PublicationBounds struct {
	Minimum *modelcore.Quantity `json:"minimum,omitempty"`
	Maximum *modelcore.Quantity `json:"maximum,omitempty"`
}

type PublicationResolution struct {
	Status             string              `json:"status"` // CONNECTED | BROKEN_PUBLICATION
	DiagnosticCode     string              `json:"diagnosticCode,omitempty"`
	Diagnostic         string              `json:"diagnostic,omitempty"`
	ResolvedVersionID  string              `json:"resolvedVersionId,omitempty"`
	GeometryKey        string              `json:"geometryKey,omitempty"`
	GeometryID         string              `json:"geometryId,omitempty"`
	TopologyKind       string              `json:"topologyKind,omitempty"`
	GeometryKind       string              `json:"geometryKind,omitempty"`
	Symmetry           string              `json:"symmetry,omitempty"`
	LocalID            uint64              `json:"localId,omitempty"`
	Origin             [3]float64          `json:"origin,omitempty"`
	XDirection         [3]float64          `json:"xDirection,omitempty"`
	YDirection         [3]float64          `json:"yDirection,omitempty"`
	ZDirection         [3]float64          `json:"zDirection,omitempty"`
	Radius             float64             `json:"radius,omitempty"`
	SourceDigest       string              `json:"sourceDigest,omitempty"`
	Value              *modelcore.Quantity `json:"value,omitempty"`
	ValueDigest        string              `json:"valueDigest,omitempty"`
	ManifestDigest     string              `json:"manifestDigest,omitempty"`
	NamingPolicyDigest string              `json:"namingPolicyDigest,omitempty"`
	EvaluatorVersion   string              `json:"evaluatorVersion,omitempty"`
	WorkerID           string              `json:"workerId,omitempty"`
	OCCTVersion        string              `json:"occtVersion,omitempty"`
}

type Publication struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Type                 string                `json:"type"`
	SemanticPurpose      string                `json:"semanticPurpose,omitempty"`
	CompatibilityVersion string                `json:"compatibilityVersion"`
	Target               PublicationTarget     `json:"target"`
	Contract             PublicationContract   `json:"contract"`
	Resolution           PublicationResolution `json:"resolution"`
}

type ReferenceGeometry struct {
	DatumPlanes []DatumPlane `json:"datumPlanes"`
	AxisSystems []AxisSystem `json:"axisSystems"`
	DatumAxes   []DatumAxis  `json:"datumAxes"`
}

// VisualizationManifest is the immutable display contract shared by a Part
// and every Product occurrence that references it. Positions are always in
// Part coordinates; occurrence transforms are applied only by the consumer.
type VisualizationManifest struct {
	SchemaVersion     uint32            `json:"schemaVersion"`
	ReferenceGeometry ReferenceGeometry `json:"referenceGeometry"`
	Primitives        []VisualPrimitive `json:"primitives"`
}

// VisualPrimitive represents selectable non-solid geometry. POINTS,
// POLYLINE, and TRIANGLES cover sketches and form the extension boundary for
// future wire/curve/surface modules without leaking OCCT types.
type VisualPrimitive struct {
	ID               string       `json:"id"`
	FeatureID        string       `json:"featureId"`
	Kind             string       `json:"kind"`
	Semantic         string       `json:"semantic"`
	EntityType       string       `json:"entityType,omitempty"`
	Role             string       `json:"role,omitempty"`
	Status           string       `json:"status,omitempty"`
	Positions        [][3]float64 `json:"positions"`
	Label            string       `json:"label,omitempty"`
	LabelPosition    *[3]float64  `json:"labelPosition,omitempty"`
	RelatedEntityIDs []string     `json:"relatedEntityIds,omitempty"`
	Indices          []uint32     `json:"indices,omitempty"`
	Selectable       bool         `json:"selectable"`
}

type SketchPoint2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type SketchSupport struct {
	Type                string                           `json:"type"`
	DatumPlaneID        string                           `json:"datumPlaneId,omitempty"`
	Plane               string                           `json:"plane"`
	PersistentSelection *modelcore.PersistentSelection   `json:"persistentSelection,omitempty"`
	SourceVersionID     string                           `json:"sourceVersionId,omitempty"`
	Origin              [3]float64                       `json:"origin,omitempty"`
	XDirection          [3]float64                       `json:"xDirection,omitempty"`
	Normal              [3]float64                       `json:"normal,omitempty"`
	OrientationRule     string                           `json:"orientationRule,omitempty"`
	Status              string                           `json:"status,omitempty"`
	DiagnosticCode      string                           `json:"diagnosticCode,omitempty"`
	Diagnostic          string                           `json:"diagnostic,omitempty"`
	DependencySnapshot  *SketchSupportDependencySnapshot `json:"dependencySnapshot,omitempty"`
}
type SketchSupportDependencySnapshot struct {
	GeometryKey    string `json:"geometryKey"`
	ManifestDigest string `json:"manifestDigest"`
	PolicyDigest   string `json:"policyDigest"`
	EvidenceDigest string `json:"evidenceDigest,omitempty"`
}
type SketchEntity struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	Role          string         `json:"role"`
	Point         *SketchPoint2  `json:"point,omitempty"`
	Start         *SketchPoint2  `json:"start,omitempty"`
	End           *SketchPoint2  `json:"end,omitempty"`
	Center        *SketchPoint2  `json:"center,omitempty"`
	Radius        float64        `json:"radius,omitempty"`
	StartAngle    float64        `json:"startAngle,omitempty"`
	EndAngle      float64        `json:"endAngle,omitempty"`
	ControlPoints []SketchPoint2 `json:"controlPoints,omitempty"`
	Degree        uint32         `json:"degree,omitempty"`
	Closed        bool           `json:"closed,omitempty"`
	Suppressed    bool           `json:"suppressed,omitempty"`
}
type SketchExternalGeometrySnapshot struct {
	Kind   string        `json:"kind"`
	Point  *SketchPoint2 `json:"point,omitempty"`
	Start  *SketchPoint2 `json:"start,omitempty"`
	End    *SketchPoint2 `json:"end,omitempty"`
	Center *SketchPoint2 `json:"center,omitempty"`
	Radius float64       `json:"radius,omitempty"`
}
type SketchExternalGeometry struct {
	ID                       string                           `json:"id"`
	ProjectionKind           string                           `json:"projectionKind"`
	GeometryKind             string                           `json:"geometryKind,omitempty"`
	PersistentSelection      modelcore.PersistentSelection    `json:"persistentSelection"`
	SourceVersionID          string                           `json:"sourceVersionId"`
	SourceDocumentID         string                           `json:"sourceDocumentId,omitempty"`
	ContextReferenceID       string                           `json:"contextReferenceId,omitempty"`
	Status                   string                           `json:"status"`
	DiagnosticCode           string                           `json:"diagnosticCode,omitempty"`
	Diagnostic               string                           `json:"diagnostic,omitempty"`
	ResolvedSourceDigest     string                           `json:"resolvedSourceDigest,omitempty"`
	Snapshot                 *SketchExternalGeometrySnapshot  `json:"snapshot,omitempty"`
	DependencySnapshot       *SketchSupportDependencySnapshot `json:"dependencySnapshot,omitempty"`
	AffectedConstraintIDs    []string                         `json:"affectedConstraintIds,omitempty"`
	AffectedProfileRegionIDs []string                         `json:"affectedProfileRegionIds,omitempty"`
	DownstreamFeatureIDs     []string                         `json:"downstreamFeatureIds,omitempty"`
}
type SketchGeometryRef struct {
	Target            string `json:"target"`
	EntityID          string `json:"entityId,omitempty"`
	SubElement        string `json:"subElement"`
	ControlPointIndex *int   `json:"controlPointIndex,omitempty"`
}
type SketchConstraint struct {
	ID            string              `json:"id"`
	Kind          string              `json:"kind"`
	References    []SketchGeometryRef `json:"references"`
	FixedPoint    *SketchPoint2       `json:"fixedPoint,omitempty"`
	Value         *float64            `json:"value,omitempty"`
	Unit          string              `json:"unit,omitempty"`
	ParameterID   string              `json:"parameterId,omitempty"`
	LabelPosition *SketchPoint2       `json:"labelPosition,omitempty"`
	Internal      bool                `json:"internal,omitempty"`
	Suppressed    bool                `json:"suppressed,omitempty"`
}
type SketchSolveState struct {
	Status                   string                 `json:"status"`
	DefinitionStatus         string                 `json:"definitionStatus"`
	DegreesOfFreedom         int                    `json:"degreesOfFreedom"`
	Diagnostic               string                 `json:"diagnostic,omitempty"`
	ConflictingConstraintIDs []string               `json:"conflictingConstraintIds,omitempty"`
	RedundantConstraintIDs   []string               `json:"redundantConstraintIds,omitempty"`
	Components               []SketchSolveComponent `json:"components,omitempty"`
}
type SketchSolveComponent struct {
	EntityIDs        []string `json:"entityIds"`
	ConstraintIDs    []string `json:"constraintIds"`
	Status           string   `json:"status"`
	DefinitionStatus string   `json:"definitionStatus"`
	DegreesOfFreedom int      `json:"degreesOfFreedom"`
}
type SketchFeature struct {
	SchemaVersion    uint32                   `json:"schemaVersion"`
	Support          SketchSupport            `json:"support"`
	Entities         []SketchEntity           `json:"entities"`
	ExternalGeometry []SketchExternalGeometry `json:"externalGeometry,omitempty"`
	Constraints      []SketchConstraint       `json:"constraints"`
	Solve            SketchSolveState         `json:"solve"`
}
type SketchOperation struct {
	Type              string                  `json:"type"`
	Entity            *SketchEntity           `json:"entity,omitempty"`
	Constraint        *SketchConstraint       `json:"constraint,omitempty"`
	ConstraintID      string                  `json:"constraintId,omitempty"`
	LabelPosition     *SketchPoint2           `json:"labelPosition,omitempty"`
	Value             *float64                `json:"value,omitempty"`
	First             *SketchPoint2           `json:"first,omitempty"`
	Second            *SketchPoint2           `json:"second,omitempty"`
	FirstReference    *SketchGeometryRef      `json:"firstReference,omitempty"`
	SecondReference   *SketchGeometryRef      `json:"secondReference,omitempty"`
	EntityID          string                  `json:"entityId,omitempty"`
	Role              string                  `json:"role,omitempty"`
	SubElement        string                  `json:"subElement,omitempty"`
	ControlPointIndex *int                    `json:"controlPointIndex,omitempty"`
	Point             *SketchPoint2           `json:"point,omitempty"`
	Suppressed        *bool                   `json:"suppressed,omitempty"`
	ExternalGeometry  *SketchExternalGeometry `json:"externalGeometry,omitempty"`
	ExternalID        string                  `json:"externalId,omitempty"`
	GeometryKey       string                  `json:"geometryKey,omitempty"`
	TopologyID        uint64                  `json:"topologyId,omitempty"`
	TopologyKind      string                  `json:"topologyKind,omitempty"`
	SourceVersionID   string                  `json:"sourceVersionId,omitempty"`
}

type Feature struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Name         string         `json:"name"`
	Plane        string         `json:"plane,omitempty"`
	Sketch       *SketchFeature `json:"sketch,omitempty"`
	Profile      string         `json:"profile,omitempty"`
	Length       float64        `json:"length,omitempty"`
	Angle        float64        `json:"angle,omitempty"`
	Operation    string         `json:"operation,omitempty"`
	AxisEntityID string         `json:"axisEntityId,omitempty"`
	Reversed     bool           `json:"reversed,omitempty"`
	GeometryKey  string         `json:"geometryKey,omitempty"`
	FileName     string         `json:"fileName,omitempty"`
	SourceFormat string         `json:"sourceFormat,omitempty"`
}

type PartModel struct {
	Units             string                          `json:"units"`
	DatumPlanes       []DatumPlane                    `json:"datumPlanes"`
	AxisSystems       []AxisSystem                    `json:"axisSystems"`
	DatumAxes         []DatumAxis                     `json:"datumAxes"`
	Features          []Feature                       `json:"features"`
	Parameters        []modelcore.ParameterDefinition `json:"parameters,omitempty"`
	Publications      []Publication                   `json:"publications,omitempty"`
	ContextInputs     []ContextInput                  `json:"contextInputs,omitempty"`
	ContextReferences []ContextReference              `json:"contextReferences,omitempty"`
}

type ContextInputTarget struct {
	Kind     string `json:"kind"` // PARAMETER | DATUM | SKETCH_EXTERNAL_GEOMETRY | FEATURE_INPUT
	TargetID string `json:"targetId"`
}

// ContextInput is a reusable, source-independent input port owned by a Part.
// The Product that instantiates the Part owns the source occurrence wiring.
type ContextInput struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Type     string              `json:"type"`
	Required bool                `json:"required,omitempty"`
	Target   ContextInputTarget  `json:"target"`
	Contract PublicationContract `json:"contract"`
}

type PublicationRef struct {
	PublicationID            string                         `json:"publicationId"`
	ExpectedType             string                         `json:"expectedType"`
	CompatibilityVersion     string                         `json:"compatibilityVersion"`
	PersistentSelection      *modelcore.PersistentSelection `json:"persistentSelection,omitempty"`
	SelectionSourceVersionID string                         `json:"selectionSourceVersionId,omitempty"`
}

type ProductPublicationTarget struct {
	InstancePath  InstancePath `json:"instancePath"`
	PublicationID string       `json:"publicationId"`
}

type ProductPublication struct {
	ID                   string                   `json:"id"`
	Name                 string                   `json:"name"`
	Type                 string                   `json:"type"`
	SemanticPurpose      string                   `json:"semanticPurpose,omitempty"`
	CompatibilityVersion string                   `json:"compatibilityVersion"`
	Target               ProductPublicationTarget `json:"target"`
	Contract             PublicationContract      `json:"contract"`
	Resolution           PublicationResolution    `json:"resolution"`
}

type ContextBindingResolutionSnapshot struct {
	RootProductRevisionID string `json:"rootProductRevisionId"`
	SourceRevisionID      string `json:"sourceRevisionId"`
	OwningRevisionID      string `json:"owningRevisionId"`
	ContractDigest        string `json:"contractDigest"`
	SourceDigest          string `json:"sourceDigest"`
	Status                string `json:"status"`
}

// ContextBinding is Product-owned wiring between two occurrences. It never
// uses display names as identity and does not mutate either referenced Part.
type ContextBinding struct {
	ID                 string                           `json:"id"`
	Name               string                           `json:"name"`
	OwningInstancePath InstancePath                     `json:"owningInstancePath"`
	ContextInputID     string                           `json:"contextInputId"`
	SourceInstancePath InstancePath                     `json:"sourceInstancePath"`
	Publication        PublicationRef                   `json:"publication"`
	ReferenceMode      string                           `json:"referenceMode"`
	Transform          InstancePose                     `json:"transform"`
	Resolution         PublicationResolution            `json:"resolution"`
	Accepted           ContextBindingResolutionSnapshot `json:"accepted"`
}

type ContextReference struct {
	ID                        string                `json:"id"`
	Name                      string                `json:"name"`
	OwningWorkspace           string                `json:"owningWorkspace"`
	SourceDocumentID          string                `json:"sourceDocumentId"`
	ReferenceMode             string                `json:"referenceMode"`
	RootProductDocumentID     string                `json:"rootProductDocumentId,omitempty"`
	RootProductRevisionID     string                `json:"rootProductRevisionId,omitempty"`
	RootProductSnapshotDigest string                `json:"rootProductSnapshotDigest,omitempty"`
	SourceInstancePath        *InstancePath         `json:"sourceInstancePath,omitempty"`
	OwningInstancePath        *InstancePath         `json:"owningInstancePath,omitempty"`
	ContextVariantID          string                `json:"contextVariantId,omitempty"`
	Publication               PublicationRef        `json:"publication"`
	Transform                 InstancePose          `json:"transform"`
	ResolvedRevisionID        string                `json:"resolvedRevisionId"`
	Resolution                PublicationResolution `json:"resolution"`
	LocalTargetID             string                `json:"localTargetId,omitempty"`
}

type ProductInstance struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	ReferencedDocumentID string     `json:"documentId"`
	ReferencedVersionID  string     `json:"versionId"`
	Translation          [3]float64 `json:"translation"`
	Rotation             [4]float64 `json:"rotation,omitempty"`
	ReferenceMode        string     `json:"referenceMode,omitempty"`
	ResolvedVersionID    string     `json:"resolvedVersionId,omitempty"`
	HeadChanged          bool       `json:"headChanged,omitempty"`
}

type InstancePose struct {
	Translation [3]float64 `json:"translation"`
	Rotation    [4]float64 `json:"rotation"`
}

type AssemblyGeometryRef struct {
	InstanceID            string                         `json:"instanceId"`
	Kind                  string                         `json:"kind"`
	GeometryID            string                         `json:"geometryId,omitempty"`
	Axis                  string                         `json:"axis,omitempty"`
	PersistentSelection   *modelcore.PersistentSelection `json:"persistentSelection,omitempty"`
	SourceVersionID       string                         `json:"sourceVersionId,omitempty"`
	Resolution            *ResolutionSnapshot            `json:"resolution,omitempty"`
	GeometryKey           string                         `json:"geometryKey,omitempty"` // transient pick evidence
	TopologyID            uint64                         `json:"topologyId,omitempty"`  // transient pick evidence
	PublicationRef        *PublicationRef                `json:"publicationRef,omitempty"`
	PublicationResolution *PublicationResolution         `json:"publicationResolution,omitempty"`
}

type ResolutionSnapshot struct {
	SourceVersionID string                        `json:"sourceVersionId"`
	TargetVersionID string                        `json:"targetVersionId"`
	ManifestDigest  string                        `json:"manifestDigest"`
	PolicyDigest    string                        `json:"policyDigest"`
	Result          modelcore.SelectionResolution `json:"result"`
}

type AssemblyConstraint struct {
	FixMode                 string                                       `json:"fixMode,omitempty"`
	AngleRelation           string                                       `json:"angleRelation,omitempty"`
	MeasuredValue           *float64                                     `json:"measuredValue,omitempty"`
	Suppressed              bool                                         `json:"suppressed,omitempty"`
	ID                      string                                       `json:"id"`
	ConnectionID            string                                       `json:"connectionId,omitempty"`
	Kind                    string                                       `json:"kind"`
	Mode                    string                                       `json:"mode,omitempty"`
	First                   AssemblyGeometryRef                          `json:"first"`
	Second                  *AssemblyGeometryRef                         `json:"second,omitempty"`
	Value                   float64                                      `json:"value,omitempty"`
	DirectionRelation       string                                       `json:"directionRelation,omitempty"`
	DistanceRelation        string                                       `json:"distanceRelation,omitempty"`
	AngleAxis               *AssemblyGeometryRef                         `json:"angleAxis,omitempty"`
	ReverseAngleAxis        bool                                         `json:"reverseAngleAxis,omitempty"`
	AngleReferenceDirection *[3]float64                                  `json:"angleReferenceDirection,omitempty"`
	FixedPose               *InstancePose                                `json:"fixedPose,omitempty"`
	EvaluationStatus        modelcore.AssemblyConstraintEvaluationStatus `json:"evaluationStatus"`
	EvaluationSummary       string                                       `json:"evaluationSummary,omitempty"`
}

type ProductModel struct {
	Instances       []ProductInstance    `json:"instances"`
	Constraints     []AssemblyConstraint `json:"constraints,omitempty"`
	Publications    []ProductPublication `json:"publications,omitempty"`
	ContextBindings []ContextBinding     `json:"contextBindings,omitempty"`
}

type ReferenceUpdate struct {
	ConsumerKind        string `json:"consumerKind"`
	ConsumerID          string `json:"consumerId"`
	SourceDocumentID    string `json:"sourceDocumentId"`
	AcceptedRevisionID  string `json:"acceptedRevisionId"`
	CandidateRevisionID string `json:"candidateRevisionId,omitempty"`
	Status              string `json:"status"`
	DiagnosticCode      string `json:"diagnosticCode,omitempty"`
	Diagnostic          string `json:"diagnostic,omitempty"`
}

type ContextVariantSnapshot struct {
	VariantKey               string        `json:"variantKey"`
	OwningInstancePath       InstancePath  `json:"owningInstancePath"`
	BaseDocumentID           string        `json:"baseDocumentId"`
	BaseRevisionID           string        `json:"baseRevisionId"`
	BindingIDs               []string      `json:"bindingIds"`
	BindingDigest            string        `json:"bindingDigest"`
	EvaluationManifestDigest string        `json:"evaluationManifestDigest,omitempty"`
	GeometryKey              string        `json:"geometryKey,omitempty"`
	Publications             []Publication `json:"publications,omitempty"`
	Status                   string        `json:"status"`
	DiagnosticCode           string        `json:"diagnosticCode,omitempty"`
	Diagnostic               string        `json:"diagnostic,omitempty"`
}

type ProductUpdatePlanEntry struct {
	Kind                string `json:"kind"`
	BindingID           string `json:"bindingId"`
	Name                string `json:"name"`
	SourceDisplayPath   string `json:"sourceDisplayPath"`
	OwningDisplayPath   string `json:"owningDisplayPath"`
	AcceptedRevisionID  string `json:"acceptedRevisionId"`
	CandidateRevisionID string `json:"candidateRevisionId,omitempty"`
	Connection          string `json:"connection"`
	Currency            string `json:"currency"`
	Evaluation          string `json:"evaluation"`
	DiagnosticCode      string `json:"diagnosticCode,omitempty"`
	Diagnostic          string `json:"diagnostic,omitempty"`
}

type ProductUpdatePlan struct {
	RootProductDocumentID string                   `json:"rootProductDocumentId"`
	RootProductRevisionID string                   `json:"rootProductRevisionId"`
	Digest                string                   `json:"digest"`
	CanAccept             bool                     `json:"canAccept"`
	HasUpdates            bool                     `json:"hasUpdates"`
	Entries               []ProductUpdatePlanEntry `json:"entries"`
	ContextVariants       []ContextVariantSnapshot `json:"contextVariants"`
	AffectedConstraintIDs []string                 `json:"affectedConstraintIds,omitempty"`
}

type CreateProductReleaseRequest struct {
	RequestID string `json:"requestId"`
	Name      string `json:"name"`
	ActorID   string `json:"-"`
}

type ProductReleaseGate struct {
	Code       string `json:"code"`
	Status     string `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type ProductReleaseOccurrence struct {
	InstancePath             InstancePath `json:"instancePath"`
	DocumentID               string       `json:"documentId"`
	DocumentType             string       `json:"documentType"`
	RevisionID               string       `json:"revisionId"`
	Pose                     InstancePose `json:"pose"`
	GeometryKey              string       `json:"geometryKey,omitempty"`
	EvaluationManifestDigest string       `json:"evaluationManifestDigest"`
}

type ProductReleaseManifest struct {
	SchemaVersion         int                        `json:"schemaVersion"`
	Digest                string                     `json:"digest"`
	RootProductDocumentID string                     `json:"rootProductDocumentId"`
	RootProductRevisionID string                     `json:"rootProductRevisionId"`
	RootSnapshotDigest    string                     `json:"rootSnapshotDigest"`
	Occurrences           []ProductReleaseOccurrence `json:"occurrences"`
	ContextBindings       []ContextBinding           `json:"contextBindings,omitempty"`
	ContextVariants       []ContextVariantSnapshot   `json:"contextVariants,omitempty"`
	ProductPublications   []ProductPublication       `json:"productPublications,omitempty"`
	AssemblySolveManifest string                     `json:"assemblySolveManifestDigest,omitempty"`
	EvaluatorVersion      string                     `json:"evaluatorVersion"`
	NamingPolicyDigest    string                     `json:"namingPolicyDigest"`
	Gates                 []ProductReleaseGate       `json:"gates"`
}

type ProductRelease struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	CreatedAt string                 `json:"createdAt"`
	Manifest  ProductReleaseManifest `json:"manifest"`
}

type ProductReleaseReplay struct {
	ReleaseID      string                       `json:"releaseId"`
	ManifestDigest string                       `json:"manifestDigest"`
	Status         string                       `json:"status"`
	Assembly       *AssemblySolveManifestResult `json:"assembly,omitempty"`
}

// InstancePath is the stable occurrence identity from an opened root Product
// to one referenced document. Names are presentation only; identity is the
// ordered owner/InstanceId chain.
type InstancePathSegment struct {
	OwnerDocumentID      string `json:"ownerDocumentId"`
	OwnerVersionID       string `json:"ownerVersionId"`
	InstanceID           string `json:"instanceId"`
	InstanceName         string `json:"instanceName"`
	ReferencedDocumentID string `json:"referencedDocumentId"`
	ResolvedVersionID    string `json:"resolvedVersionId"`
}

type InstancePath struct {
	RootDocumentID string                `json:"rootDocumentId"`
	Segments       []InstancePathSegment `json:"segments"`
	Canonical      string                `json:"canonical"`
	Display        string                `json:"display"`
}

func appendInstancePath(path InstancePath, segment InstancePathSegment) InstancePath {
	segments := append(append([]InstancePathSegment{}, path.Segments...), segment)
	ids, names := make([]string, 0, len(segments)), make([]string, 0, len(segments))
	for _, item := range segments {
		ids = append(ids, item.InstanceID)
		names = append(names, item.InstanceName)
	}
	path.Segments = segments
	path.Canonical = strings.Join(ids, "/")
	path.Display = strings.Join(names, "/")
	return path
}

func nextInstanceName(model ProductModel, referenceName string) string {
	base := strings.TrimSpace(referenceName)
	if base == "" {
		base = "Component"
	}
	used := make(map[string]bool, len(model.Instances))
	for _, instance := range model.Instances {
		used[strings.ToLower(strings.TrimSpace(instance.Name))] = true
	}
	for ordinal := 1; ; ordinal++ {
		candidate := fmt.Sprintf("%s.%d", base, ordinal)
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

func applyInstancePath(nodes []DocumentStructureNode, path *InstancePath) {
	for index := range nodes {
		if path != nil && nodes[index].InstancePath == nil {
			copy := *path
			nodes[index].InstancePath = &copy
		}
		applyInstancePath(nodes[index].Children, path)
	}
}

type DocumentSummary struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Type          string  `json:"type"`
	VersionID     string  `json:"versionId"`
	CanUndo       bool    `json:"canUndo"`
	CanRedo       bool    `json:"canRedo"`
	CreatedAt     string  `json:"createdAt"`
	LastUpdated   string  `json:"lastUpdated"`
	DeletedAt     *string `json:"deletedAt,omitempty"`
	FolderID      *string `json:"folderId,omitempty"`
	LastOpenedAt  *string `json:"lastOpenedAt,omitempty"`
	CopiedFromID  *string `json:"copiedFromDocumentId,omitempty"`
	WorkspaceName string  `json:"workspaceName"`
	Permission    string  `json:"permission"`
}

type DocumentListOptions struct {
	Scope, Query, Type, FolderID, Sort, ActorID string
	Recent, AllFolders, Shared                  bool
	Limit, Offset                               int
}

type DocumentPage struct {
	Documents []DocumentSummary `json:"documents"`
	Total     int               `json:"total"`
	Limit     int               `json:"limit"`
	Offset    int               `json:"offset"`
}

type FolderSummary struct {
	ID            string  `json:"id"`
	ParentID      *string `json:"parentId,omitempty"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	DocumentCount int     `json:"documentCount"`
	TrashCount    int     `json:"trashCount"`
	ChildCount    int     `json:"childCount"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
	Permission    string  `json:"permission"`
}

type CreateFolderRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	ParentID    *string `json:"parentId,omitempty"`
	ActorID     string  `json:"-"`
}

type UpdateFolderRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type MoveDocumentRequest struct {
	FolderID *string `json:"folderId"`
}

type CopyDocumentRequest struct {
	RequestID string  `json:"requestId"`
	Name      string  `json:"name"`
	FolderID  *string `json:"folderId,omitempty"`
	ActorID   string  `json:"-"`
}

type ResolvedInstance struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	DocumentID     string       `json:"documentId"`
	GeometryKey    string       `json:"geometryKey"`
	Translation    [3]float64   `json:"translation"`
	Rotation       [4]float64   `json:"rotation"`
	OccurrencePath string       `json:"occurrencePath"`
	InstancePath   InstancePath `json:"instancePath"`
	BodyTreeNodeID string       `json:"bodyTreeNodeId"`
}

// DocumentStructureNode is the UI-independent specification tree contract.
// IDs are path-stable within one DocumentView while EntityID preserves the
// domain object identity used by commands and selection.
type DocumentStructureNode struct {
	ID                 string                  `json:"id"`
	Kind               string                  `json:"kind"`
	Name               string                  `json:"name"`
	ReferenceName      string                  `json:"referenceName,omitempty"`
	InstanceName       string                  `json:"instanceName,omitempty"`
	EntityID           string                  `json:"entityId,omitempty"`
	DocumentID         string                  `json:"documentId,omitempty"`
	DocumentType       string                  `json:"documentType,omitempty"`
	VersionID          string                  `json:"versionId,omitempty"`
	Plane              string                  `json:"plane,omitempty"`
	Axis               string                  `json:"axis,omitempty"`
	GeometryKey        string                  `json:"geometryKey,omitempty"`
	TopologyID         uint64                  `json:"topologyId,omitempty"`
	ReferenceMode      string                  `json:"referenceMode,omitempty"`
	InstancePath       *InstancePath           `json:"instancePath,omitempty"`
	OwnerEntityID      string                  `json:"ownerEntityId,omitempty"`
	EntityType         string                  `json:"entityType,omitempty"`
	Role               string                  `json:"role,omitempty"`
	Suppressed         bool                    `json:"suppressed,omitempty"`
	Diagnostic         string                  `json:"diagnostic,omitempty"`
	Capabilities       []string                `json:"capabilities,omitempty"`
	DefinitionDigest   string                  `json:"definitionDigest,omitempty"`
	Publication        *Publication            `json:"publication,omitempty"`
	ProductPublication *ProductPublication     `json:"productPublication,omitempty"`
	ContextInput       *ContextInput           `json:"contextInput,omitempty"`
	ContextBinding     *ContextBinding         `json:"contextBinding,omitempty"`
	Children           []DocumentStructureNode `json:"children,omitempty"`
}

type DocumentView struct {
	Document          DocumentSummary          `json:"document"`
	DatumPlanes       []DatumPlane             `json:"datumPlanes,omitempty"`
	AxisSystems       []AxisSystem             `json:"axisSystems,omitempty"`
	DatumAxes         []DatumAxis              `json:"datumAxes,omitempty"`
	Part              *PartModel               `json:"part,omitempty"`
	Product           *ProductModel            `json:"product,omitempty"`
	Artifact          *Artifact                `json:"artifact,omitempty"`
	Artifacts         map[string]Artifact      `json:"artifacts,omitempty"`
	ResolvedInstances []ResolvedInstance       `json:"resolvedInstances,omitempty"`
	StructureTree     *DocumentStructureNode   `json:"structureTree,omitempty"`
	ReferenceUpdates  []ReferenceUpdate        `json:"referenceUpdates,omitempty"`
	DesignSession     *ProductDesignSession    `json:"designSession,omitempty"`
	ContextVariants   []ContextVariantSnapshot `json:"contextVariants,omitempty"`
}

type ProductDesignSession struct {
	RootProductDocumentID string        `json:"rootProductDocumentId"`
	RootProductRevisionID string        `json:"rootProductRevisionId"`
	RootSnapshotDigest    string        `json:"rootSnapshotDigest"`
	ActiveInstancePath    *InstancePath `json:"activeInstancePath,omitempty"`
	ActiveDocumentID      string        `json:"activeDocumentId"`
	ActiveRevisionID      string        `json:"activeRevisionId"`
	ContextCatalogDigest  string        `json:"contextCatalogDigest"`
}

type ContextCatalogPublication struct {
	InstancePath InstancePath `json:"instancePath"`
	DocumentID   string       `json:"documentId"`
	RevisionID   string       `json:"revisionId"`
	Publication  Publication  `json:"publication"`
	DisplayPath  string       `json:"displayPath"`
	Selectable   bool         `json:"selectable"`
	Diagnostic   string       `json:"diagnostic,omitempty"`
}

type ContextCatalog struct {
	RootProductDocumentID string                      `json:"rootProductDocumentId"`
	RootProductRevisionID string                      `json:"rootProductRevisionId"`
	ActiveInstancePath    *InstancePath               `json:"activeInstancePath,omitempty"`
	ExpectedType          string                      `json:"expectedType,omitempty"`
	Digest                string                      `json:"digest"`
	Publications          []ContextCatalogPublication `json:"publications"`
}

type CreateDocumentRequest struct {
	RequestID   string  `json:"requestId"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description string  `json:"description,omitempty"`
	FolderID    *string `json:"folderId,omitempty"`
	ActorID     string  `json:"-"`
}

// CreatePartComponentRequest models the Product-scoped "new Part" action.
// A nil TargetProductInstancePath addresses the root Product. A non-empty path
// addresses a nested Product occurrence inside that immutable root snapshot.
type CreatePartComponentRequest struct {
	RequestID                 string        `json:"requestId"`
	Name                      string        `json:"name,omitempty"`
	Description               string        `json:"description,omitempty"`
	PlacementMode             string        `json:"placementMode,omitempty"`
	TargetProductInstancePath *InstancePath `json:"targetProductInstancePath,omitempty"`
	ActorID                   string        `json:"-"`
}

type UpdateDocumentRequest struct {
	RequestID   string `json:"requestId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type DeleteNodeTarget struct {
	TargetKind    string `json:"targetKind"`
	TargetID      string `json:"targetId"`
	OwnerEntityID string `json:"ownerEntityId,omitempty"`
}

type CommandRequest struct {
	FixMode                 string               `json:"fixMode,omitempty"`
	FixedPose               *InstancePose        `json:"fixedPose,omitempty"`
	AngleRelation           string               `json:"angleRelation,omitempty"`
	ConstraintIDs           []string             `json:"constraintIds,omitempty"`
	Suppressed              *bool                `json:"suppressed,omitempty"`
	ConstraintMode          *string              `json:"constraintMode,omitempty"`
	RequestID               string               `json:"requestId"`
	InteractionID           string               `json:"interactionId,omitempty"`
	PreviewSequence         uint64               `json:"previewSequence,omitempty"`
	PreviewID               string               `json:"previewId,omitempty"`
	Type                    string               `json:"type"`
	Plane                   string               `json:"plane,omitempty"`
	DatumPlaneID            string               `json:"datumPlaneId,omitempty"`
	TopologyID              uint64               `json:"topologyId,omitempty"`
	SketchID                string               `json:"sketchId,omitempty"`
	Operations              []SketchOperation    `json:"operations,omitempty"`
	Length                  float64              `json:"length,omitempty"`
	Angle                   float64              `json:"angle,omitempty"`
	Generator               string               `json:"generator,omitempty"`
	Operation               string               `json:"operation,omitempty"`
	AxisEntityID            string               `json:"axisEntityId,omitempty"`
	Reversed                bool                 `json:"reversed,omitempty"`
	Origin                  [3]float64           `json:"origin,omitempty"`
	Normal                  [3]float64           `json:"normal,omitempty"`
	UDirection              [3]float64           `json:"uDirection,omitempty"`
	Direction               [3]float64           `json:"direction,omitempty"`
	ReferencedDocumentID    string               `json:"referencedDocumentId,omitempty"`
	Name                    string               `json:"name,omitempty"`
	InstanceID              string               `json:"instanceId,omitempty"`
	TargetKind              string               `json:"targetKind,omitempty"`
	TargetID                string               `json:"targetId,omitempty"`
	OwnerEntityID           string               `json:"ownerEntityId,omitempty"`
	Targets                 []DeleteNodeTarget   `json:"targets,omitempty"`
	Translation             [3]float64           `json:"translation,omitempty"`
	Rotation                [4]float64           `json:"rotation,omitempty"`
	ConstraintKind          string               `json:"constraintKind,omitempty"`
	FirstAssemblyRef        *AssemblyGeometryRef `json:"firstAssemblyRef,omitempty"`
	SecondAssemblyRef       *AssemblyGeometryRef `json:"secondAssemblyRef,omitempty"`
	DirectionRelation       string               `json:"directionRelation,omitempty"`
	DistanceRelation        string               `json:"distanceRelation,omitempty"`
	AngleAxis               *AssemblyGeometryRef `json:"angleAxis,omitempty"`
	ReverseAngleAxis        *bool                `json:"reverseAngleAxis,omitempty"`
	AngleReferenceDirection *[3]float64          `json:"angleReferenceDirection,omitempty"`
	ReferenceMode           string               `json:"referenceMode,omitempty"`
	GeometryKey             string               `json:"geometryKey,omitempty"`
	FileName                string               `json:"fileName,omitempty"`
	SourceFormat            string               `json:"sourceFormat,omitempty"`
	VersionID               string               `json:"versionId,omitempty"`
	ParameterID             string               `json:"parameterId,omitempty"`
	ExpectedFeatureDigest   string               `json:"expectedFeatureDigest,omitempty"`
	LengthExpression        string               `json:"lengthExpression,omitempty"`
	Expression              string               `json:"expression,omitempty"`
	Value                   float64              `json:"value,omitempty"`
	Unit                    string               `json:"unit,omitempty"`
	PublicationID           string               `json:"publicationId,omitempty"`
	PublicationType         string               `json:"publicationType,omitempty"`
	SemanticPurpose         string               `json:"semanticPurpose,omitempty"`
	CompatibilityVersion    string               `json:"compatibilityVersion,omitempty"`
	Axis                    string               `json:"axis,omitempty"`
	SourceDocumentID        string               `json:"sourceDocumentId,omitempty"`
	ContextReferenceID      string               `json:"contextReferenceId,omitempty"`
	ContextBindingID        string               `json:"contextBindingId,omitempty"`
	ContextVariantID        string               `json:"contextVariantId,omitempty"`
	UpdatePlanDigest        string               `json:"updatePlanDigest,omitempty"`
	RootProductDocumentID   string               `json:"rootProductDocumentId,omitempty"`
	InstancePath            *InstancePath        `json:"instancePath,omitempty"`
	OwningInstancePath      *InstancePath        `json:"owningInstancePath,omitempty"`
	SourceInstancePath      *InstancePath        `json:"sourceInstancePath,omitempty"`
	ContextInputID          string               `json:"contextInputId,omitempty"`
	Required                bool                 `json:"required,omitempty"`
	ActorID                 string               `json:"-"`
}

// CommandPreview is a non-persistent evaluation of the same typed command
// used by ApplyCommand. The base revision lets clients reject a response that
// arrived after the workspace head changed.
type CommandPreview struct {
	AssemblySolverBuild  string                               `json:"assemblySolverBuild,omitempty"`
	AssemblyComponents   []geometry.AssemblyComponentDof      `json:"assemblyComponents,omitempty"`
	PreviewID            string                               `json:"previewId"`
	BaseVersionID        string                               `json:"baseVersionId"`
	BaseSequence         uint64                               `json:"baseSequence"`
	ModelHash            string                               `json:"modelHash"`
	Artifact             *Artifact                            `json:"artifact,omitempty"`
	ConstraintLimited    bool                                 `json:"constraintLimited,omitempty"`
	ConstraintEvaluation *AssemblyConstraintPreviewEvaluation `json:"constraintEvaluation,omitempty"`
	InstancePoses        []struct {
		InstanceID  string     `json:"instanceId"`
		Translation [3]float64 `json:"translation"`
		Rotation    [4]float64 `json:"rotation"`
	} `json:"instancePoses,omitempty"`
}

type AssemblySupportPreviewEvaluation struct {
	Status         modelcore.SupportingElementStatus `json:"status"`
	DiagnosticCode string                            `json:"diagnosticCode,omitempty"`
	Diagnostic     string                            `json:"diagnostic,omitempty"`
}

type AssemblyConstraintPreviewEvaluation struct {
	ConstraintID string                                       `json:"constraintId"`
	Status       modelcore.AssemblyConstraintEvaluationStatus `json:"status"`
	Summary      string                                       `json:"summary,omitempty"`
	First        AssemblySupportPreviewEvaluation             `json:"first"`
	Second       *AssemblySupportPreviewEvaluation            `json:"second,omitempty"`
}

type HistoryEntry struct {
	Position    int    `json:"position"`
	VersionID   string `json:"versionId"`
	Sequence    int    `json:"sequence"`
	CommandType string `json:"commandType"`
	CreatedAt   string `json:"createdAt"`
	IsHead      bool   `json:"isHead"`
	VersionName string `json:"versionName,omitempty"`
}

type CreateVersionRequest struct {
	RequestID   string `json:"requestId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type CreateWorkspaceRequest struct {
	Name       string `json:"name"`
	RevisionID string `json:"revisionId"`
}

type WorkspaceSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	HeadRevisionID string `json:"headRevisionId"`
	HeadSequence   uint64 `json:"headSequence"`
	BaseRevisionID string `json:"baseRevisionId"`
}
