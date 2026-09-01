package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"go.opentelemetry.io/otel/trace"
)

const evaluatorVersion = "part-solid-generators-v8"

var (
	ErrNotFound   = errors.New("document not found")
	ErrValidation = errors.New("invalid workspace command")
)

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
	GeometryKey  string         `json:"geometryKey"`
	GeometryID   string         `json:"geometryId"`
	Kind         string         `json:"kind"`
	LocalID      uint64         `json:"localId"`
	GeometryType string         `json:"geometryType"`
	BBox         map[string]any `json:"bbox,omitempty"`
	Point        *[3]float64    `json:"point,omitempty"`
	Properties   map[string]any `json:"properties"`
	WorkerID     string         `json:"workerId"`
	OCCTVersion  string         `json:"occtVersion"`
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
	Type         string `json:"type"`
	DatumPlaneID string `json:"datumPlaneId"`
	Plane        string `json:"plane"`
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
	SchemaVersion uint32             `json:"schemaVersion"`
	Support       SketchSupport      `json:"support"`
	Entities      []SketchEntity     `json:"entities"`
	Constraints   []SketchConstraint `json:"constraints"`
	Solve         SketchSolveState   `json:"solve"`
}
type SketchOperation struct {
	Type              string             `json:"type"`
	Entity            *SketchEntity      `json:"entity,omitempty"`
	Constraint        *SketchConstraint  `json:"constraint,omitempty"`
	ConstraintID      string             `json:"constraintId,omitempty"`
	LabelPosition     *SketchPoint2      `json:"labelPosition,omitempty"`
	Value             *float64           `json:"value,omitempty"`
	First             *SketchPoint2      `json:"first,omitempty"`
	Second            *SketchPoint2      `json:"second,omitempty"`
	FirstReference    *SketchGeometryRef `json:"firstReference,omitempty"`
	SecondReference   *SketchGeometryRef `json:"secondReference,omitempty"`
	EntityID          string             `json:"entityId,omitempty"`
	Role              string             `json:"role,omitempty"`
	SubElement        string             `json:"subElement,omitempty"`
	ControlPointIndex *int               `json:"controlPointIndex,omitempty"`
	Point             *SketchPoint2      `json:"point,omitempty"`
	Suppressed        *bool              `json:"suppressed,omitempty"`
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
	Units       string                          `json:"units"`
	DatumPlanes []DatumPlane                    `json:"datumPlanes"`
	AxisSystems []AxisSystem                    `json:"axisSystems"`
	DatumAxes   []DatumAxis                     `json:"datumAxes"`
	Features    []Feature                       `json:"features"`
	Parameters  []modelcore.ParameterDefinition `json:"parameters,omitempty"`
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
	InstanceID  string `json:"instanceId"`
	Kind        string `json:"kind"`
	GeometryID  string `json:"geometryId,omitempty"`
	Axis        string `json:"axis,omitempty"`
	GeometryKey string `json:"geometryKey,omitempty"`
	TopologyID  uint64 `json:"topologyId,omitempty"`
}

type AssemblyConstraint struct {
	ID                string               `json:"id"`
	Kind              string               `json:"kind"`
	First             AssemblyGeometryRef  `json:"first"`
	Second            *AssemblyGeometryRef `json:"second,omitempty"`
	Value             float64              `json:"value,omitempty"`
	DirectionRelation string               `json:"directionRelation,omitempty"`
	DistanceRelation  string               `json:"distanceRelation,omitempty"`
	FixedPose         *InstancePose        `json:"fixedPose,omitempty"`
}

type ProductModel struct {
	Instances   []ProductInstance    `json:"instances"`
	Constraints []AssemblyConstraint `json:"constraints,omitempty"`
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
	ID            string                  `json:"id"`
	Kind          string                  `json:"kind"`
	Name          string                  `json:"name"`
	EntityID      string                  `json:"entityId,omitempty"`
	DocumentID    string                  `json:"documentId,omitempty"`
	DocumentType  string                  `json:"documentType,omitempty"`
	VersionID     string                  `json:"versionId,omitempty"`
	Plane         string                  `json:"plane,omitempty"`
	Axis          string                  `json:"axis,omitempty"`
	ReferenceMode string                  `json:"referenceMode,omitempty"`
	InstancePath  *InstancePath           `json:"instancePath,omitempty"`
	OwnerEntityID string                  `json:"ownerEntityId,omitempty"`
	EntityType    string                  `json:"entityType,omitempty"`
	Role          string                  `json:"role,omitempty"`
	Suppressed    bool                    `json:"suppressed,omitempty"`
	Diagnostic    string                  `json:"diagnostic,omitempty"`
	Capabilities  []string                `json:"capabilities,omitempty"`
	Children      []DocumentStructureNode `json:"children,omitempty"`
}

type DocumentView struct {
	Document          DocumentSummary        `json:"document"`
	DatumPlanes       []DatumPlane           `json:"datumPlanes,omitempty"`
	AxisSystems       []AxisSystem           `json:"axisSystems,omitempty"`
	DatumAxes         []DatumAxis            `json:"datumAxes,omitempty"`
	Part              *PartModel             `json:"part,omitempty"`
	Product           *ProductModel          `json:"product,omitempty"`
	Artifact          *Artifact              `json:"artifact,omitempty"`
	Artifacts         map[string]Artifact    `json:"artifacts,omitempty"`
	ResolvedInstances []ResolvedInstance     `json:"resolvedInstances,omitempty"`
	StructureTree     *DocumentStructureNode `json:"structureTree,omitempty"`
}

type CreateDocumentRequest struct {
	RequestID   string  `json:"requestId"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description string  `json:"description,omitempty"`
	FolderID    *string `json:"folderId,omitempty"`
	ActorID     string  `json:"-"`
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
	RequestID            string               `json:"requestId"`
	Type                 string               `json:"type"`
	Plane                string               `json:"plane,omitempty"`
	DatumPlaneID         string               `json:"datumPlaneId,omitempty"`
	SketchID             string               `json:"sketchId,omitempty"`
	Operations           []SketchOperation    `json:"operations,omitempty"`
	Length               float64              `json:"length,omitempty"`
	Angle                float64              `json:"angle,omitempty"`
	Generator            string               `json:"generator,omitempty"`
	Operation            string               `json:"operation,omitempty"`
	AxisEntityID         string               `json:"axisEntityId,omitempty"`
	Reversed             bool                 `json:"reversed,omitempty"`
	Origin               [3]float64           `json:"origin,omitempty"`
	Normal               [3]float64           `json:"normal,omitempty"`
	UDirection           [3]float64           `json:"uDirection,omitempty"`
	Direction            [3]float64           `json:"direction,omitempty"`
	ReferencedDocumentID string               `json:"referencedDocumentId,omitempty"`
	Name                 string               `json:"name,omitempty"`
	InstanceID           string               `json:"instanceId,omitempty"`
	TargetKind           string               `json:"targetKind,omitempty"`
	TargetID             string               `json:"targetId,omitempty"`
	OwnerEntityID        string               `json:"ownerEntityId,omitempty"`
	Targets              []DeleteNodeTarget   `json:"targets,omitempty"`
	Translation          [3]float64           `json:"translation,omitempty"`
	Rotation             [4]float64           `json:"rotation,omitempty"`
	ConstraintKind       string               `json:"constraintKind,omitempty"`
	FirstAssemblyRef     *AssemblyGeometryRef `json:"firstAssemblyRef,omitempty"`
	SecondAssemblyRef    *AssemblyGeometryRef `json:"secondAssemblyRef,omitempty"`
	DirectionRelation    string               `json:"directionRelation,omitempty"`
	DistanceRelation     string               `json:"distanceRelation,omitempty"`
	ReferenceMode        string               `json:"referenceMode,omitempty"`
	GeometryKey          string               `json:"geometryKey,omitempty"`
	FileName             string               `json:"fileName,omitempty"`
	SourceFormat         string               `json:"sourceFormat,omitempty"`
	VersionID            string               `json:"versionId,omitempty"`
	ParameterID          string               `json:"parameterId,omitempty"`
	Expression           string               `json:"expression,omitempty"`
	Value                float64              `json:"value,omitempty"`
	Unit                 string               `json:"unit,omitempty"`
	ActorID              string               `json:"-"`
}

// CommandPreview is a non-persistent evaluation of the same typed command
// used by ApplyCommand. The base revision lets clients reject a response that
// arrived after the workspace head changed.
type CommandPreview struct {
	PreviewID     string    `json:"previewId"`
	BaseVersionID string    `json:"baseVersionId"`
	BaseSequence  uint64    `json:"baseSequence"`
	ModelHash     string    `json:"modelHash"`
	Artifact      *Artifact `json:"artifact,omitempty"`
	InstancePoses []struct {
		InstanceID  string     `json:"instanceId"`
		Translation [3]float64 `json:"translation"`
		Rotation    [4]float64 `json:"rotation"`
	} `json:"instancePoses,omitempty"`
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

func (service *Service) ListWorkspaces(ctx context.Context, documentID string) ([]WorkspaceSummary, error) {
	rows, err := service.database.Query(ctx, `SELECT id::text,name,head_revision_id::text,head_sequence,base_revision_id::text FROM occccad.workspaces WHERE document_id=$1 ORDER BY created_at,id`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WorkspaceSummary{}
	for rows.Next() {
		var item WorkspaceSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.HeadRevisionID, &item.HeadSequence, &item.BaseRevisionID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) BranchWorkspace(ctx context.Context, documentID string, request CreateWorkspaceRequest) (WorkspaceSummary, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" || len(name) > 100 {
		return WorkspaceSummary{}, fmt.Errorf("%w: workspace name is required and limited to 100 characters", ErrValidation)
	}
	var belongs bool
	if err := service.database.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM occccad.document_versions WHERE id=$1 AND document_id=$2)`, request.RevisionID, documentID).Scan(&belongs); err != nil {
		return WorkspaceSummary{}, err
	}
	if !belongs {
		return WorkspaceSummary{}, fmt.Errorf("%w: branch revision does not belong to this document", ErrValidation)
	}
	var result WorkspaceSummary
	err := service.database.QueryRow(ctx, `INSERT INTO occccad.workspaces(document_id,name,head_revision_id,head_sequence,base_revision_id) VALUES($1,$2,$3,0,$3) RETURNING id::text,name,head_revision_id::text,head_sequence,base_revision_id::text`, documentID, name, request.RevisionID).Scan(&result.ID, &result.Name, &result.HeadRevisionID, &result.HeadSequence, &result.BaseRevisionID)
	if isUniqueViolation(err) {
		return WorkspaceSummary{}, fmt.Errorf("%w: workspace name already exists", ErrValidation)
	}
	return result, err
}

type Service struct {
	database           *pgxpool.Pool
	worker             *geometry.Client
	artifacts          *artifactstore.Service
	artifactCacheMu    sync.RWMutex
	artifactCache      map[string]Artifact
	artifactCacheOrder []string
}

func New(database *pgxpool.Pool, worker *geometry.Client) *Service {
	return &Service{database: database, worker: worker, artifactCache: map[string]Artifact{}}
}

func NewWithArtifacts(database *pgxpool.Pool, worker *geometry.Client, artifacts *artifactstore.Service) *Service {
	return &Service{database: database, worker: worker, artifacts: artifacts, artifactCache: map[string]Artifact{}}
}

func newID(prefix string) string {
	buffer := make([]byte, 10)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return prefix + "-" + hex.EncodeToString(buffer)
}

func requestID(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return newID("request")
}

func actorID(value string) string {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err == nil {
		return strings.TrimSpace(value)
	}
	return "00000000-0000-7000-8000-000000000001"
}

func (service *Service) ListDocuments(ctx context.Context, options DocumentListOptions) (DocumentPage, error) {
	scope := strings.ToLower(strings.TrimSpace(options.Scope))
	if scope == "" {
		scope = "active"
	}
	if scope != "active" && scope != "trash" && scope != "all" {
		return DocumentPage{}, fmt.Errorf("%w: document scope must be active, trash or all", ErrValidation)
	}
	documentType := strings.ToUpper(strings.TrimSpace(options.Type))
	if documentType != "" && documentType != "PART" && documentType != "PRODUCT" {
		return DocumentPage{}, fmt.Errorf("%w: document type must be PART or PRODUCT", ErrValidation)
	}
	folderID := strings.TrimSpace(options.FolderID)
	if folderID != "" {
		if _, err := uuid.Parse(folderID); err != nil {
			return DocumentPage{}, fmt.Errorf("%w: folder id is invalid", ErrValidation)
		}
	}
	limit := options.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 || options.Offset < 0 {
		return DocumentPage{}, fmt.Errorf("%w: pagination is outside the supported range", ErrValidation)
	}
	orderBy := "d.updated_at DESC,d.name"
	switch strings.ToLower(strings.TrimSpace(options.Sort)) {
	case "", "updated":
	case "name":
		orderBy = "lower(d.name),d.updated_at DESC"
	case "created":
		orderBy = "d.created_at DESC,d.name"
	case "recent":
		orderBy = "d.last_opened_at DESC NULLS LAST,d.name"
	default:
		return DocumentPage{}, fmt.Errorf("%w: unsupported document sort", ErrValidation)
	}
	query := strings.TrimSpace(options.Query)
	principalID := actorID(options.ActorID)
	var total int
	if err := service.database.QueryRow(ctx, `
		SELECT count(*) FROM occccad.documents d
		WHERE ($1='all' OR ($1='active' AND d.deleted_at IS NULL) OR
		       ($1='trash' AND d.deleted_at IS NOT NULL))
		  AND ($2='' OR d.name ILIKE '%' || $2 || '%' OR d.description ILIKE '%' || $2 || '%')
		  AND ($3='' OR d.document_type=$3)
		  AND ($6 OR (($4='' AND d.folder_id IS NULL) OR ($4<>'' AND d.folder_id=$4::uuid)))
		  AND (NOT $5 OR d.last_opened_at IS NOT NULL)
		  AND occccad.effective_document_role(d.id,$7) >= 10
		  AND (NOT $8 OR occccad.effective_document_role(d.id,$7) < 30)`,
		scope, query, documentType, folderID, options.Recent, options.AllFolders, principalID, options.Shared).Scan(&total); err != nil {
		return DocumentPage{}, err
	}
	rows, err := service.database.Query(ctx, `
		SELECT d.id::text,d.name,d.description,d.document_type,d.head_version_id::text,
		       d.created_at::text,
		       d.updated_at::text,d.deleted_at::text,d.folder_id::text,d.last_opened_at::text,
		       d.copied_from_document_id::text,d.workspace_name,
		       occccad.role_name(occccad.effective_document_role(d.id,$9))
		FROM occccad.documents d
		WHERE ($1='all' OR ($1='active' AND d.deleted_at IS NULL) OR
		       ($1='trash' AND d.deleted_at IS NOT NULL))
		  AND ($2='' OR d.name ILIKE '%' || $2 || '%' OR d.description ILIKE '%' || $2 || '%')
		  AND ($3='' OR d.document_type=$3)
		  AND ($6 OR (($4='' AND d.folder_id IS NULL) OR ($4<>'' AND d.folder_id=$4::uuid)))
		  AND (NOT $5 OR d.last_opened_at IS NOT NULL)
		  AND occccad.effective_document_role(d.id,$9) >= 10
		  AND (NOT $10 OR occccad.effective_document_role(d.id,$9) < 30)
		ORDER BY `+orderBy+`
		LIMIT $7 OFFSET $8`, scope, query, documentType, folderID, options.Recent,
		options.AllFolders, limit, options.Offset, principalID, options.Shared)
	if err != nil {
		return DocumentPage{}, err
	}
	defer rows.Close()
	result := []DocumentSummary{}
	for rows.Next() {
		var item DocumentSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Type, &item.VersionID,
			&item.CreatedAt, &item.LastUpdated,
			&item.DeletedAt, &item.FolderID, &item.LastOpenedAt, &item.CopiedFromID,
			&item.WorkspaceName, &item.Permission); err != nil {
			return DocumentPage{}, err
		}
		item.CanUndo, item.CanRedo, err = service.historyCapabilities(ctx, item.ID, options.ActorID)
		if err != nil {
			return DocumentPage{}, err
		}
		result = append(result, item)
	}
	return DocumentPage{Documents: result, Total: total, Limit: limit, Offset: options.Offset}, rows.Err()
}

func normalizeOptionalUUID(value *string, field string) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(*value))
	if err != nil {
		return nil, fmt.Errorf("%w: %s is invalid", ErrValidation, field)
	}
	normalized := parsed.String()
	return &normalized, nil
}

func validateNameAndDescription(name, description, object string) (string, string, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || len(name) > 120 {
		return "", "", fmt.Errorf("%w: %s name is required and must not exceed 120 characters", ErrValidation, object)
	}
	if len(description) > 500 {
		return "", "", fmt.Errorf("%w: %s description must not exceed 500 characters", ErrValidation, object)
	}
	return name, description, nil
}

func (service *Service) ListFolders(ctx context.Context, parentIDValue, principalID string, shared bool) ([]FolderSummary, error) {
	var parentID *string
	if strings.TrimSpace(parentIDValue) != "" {
		parentID = &parentIDValue
	}
	normalized, err := normalizeOptionalUUID(parentID, "parent folder id")
	if err != nil {
		return nil, err
	}
	rows, err := service.database.Query(ctx, `
		SELECT f.id::text,f.parent_id::text,f.name,f.description,
		       (SELECT count(*) FROM occccad.documents d WHERE d.folder_id=f.id AND d.deleted_at IS NULL),
		       (SELECT count(*) FROM occccad.documents d WHERE d.folder_id=f.id AND d.deleted_at IS NOT NULL),
		       (SELECT count(*) FROM occccad.folders child WHERE child.parent_id=f.id),
		       f.created_at::text,f.updated_at::text,
		       occccad.role_name(occccad.effective_folder_role(f.id,$2))
		FROM occccad.folders f
		WHERE ((NOT $3 AND (($1::text IS NULL AND f.parent_id IS NULL) OR f.parent_id=$1::uuid)) OR
		       ($3 AND occccad.effective_folder_role(f.id,$2) BETWEEN 10 AND 29 AND
		        (f.parent_id IS NULL OR occccad.effective_folder_role(f.parent_id,$2) NOT BETWEEN 10 AND 29)))
		  AND occccad.effective_folder_role(f.id,$2) >= 10
		ORDER BY lower(f.name)`, normalized, actorID(principalID), shared)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FolderSummary{}
	for rows.Next() {
		var item FolderSummary
		if err := rows.Scan(&item.ID, &item.ParentID, &item.Name, &item.Description,
			&item.DocumentCount, &item.TrashCount, &item.ChildCount, &item.CreatedAt, &item.UpdatedAt,
			&item.Permission); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) FolderBreadcrumbs(ctx context.Context, folderID, principalID string) ([]FolderSummary, error) {
	if _, err := uuid.Parse(strings.TrimSpace(folderID)); err != nil {
		return nil, fmt.Errorf("%w: folder id is invalid", ErrValidation)
	}
	rows, err := service.database.Query(ctx, `
		WITH RECURSIVE path AS (
			SELECT f.*,0 AS depth FROM occccad.folders f WHERE f.id=$1
			UNION ALL
			SELECT parent.*,path.depth+1 FROM occccad.folders parent
			JOIN path ON path.parent_id=parent.id
		)
		SELECT id::text,parent_id::text,name,description,0,0,0,created_at::text,updated_at::text,
		       occccad.role_name(occccad.effective_folder_role(id,$2))
		FROM path ORDER BY depth DESC`, folderID, actorID(principalID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FolderSummary{}
	for rows.Next() {
		var item FolderSummary
		if err := rows.Scan(&item.ID, &item.ParentID, &item.Name, &item.Description,
			&item.DocumentCount, &item.TrashCount, &item.ChildCount, &item.CreatedAt, &item.UpdatedAt,
			&item.Permission); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil, ErrNotFound
	}
	return result, rows.Err()
}

func (service *Service) CreateFolder(ctx context.Context, request CreateFolderRequest) (FolderSummary, error) {
	name, description, err := validateNameAndDescription(request.Name, request.Description, "folder")
	if err != nil {
		return FolderSummary{}, err
	}
	parentID, err := normalizeOptionalUUID(request.ParentID, "parent folder id")
	if err != nil {
		return FolderSummary{}, err
	}
	if parentID != nil {
		var exists bool
		if err := service.database.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM occccad.folders WHERE id=$1)`, *parentID).Scan(&exists); err != nil {
			return FolderSummary{}, err
		}
		if !exists {
			return FolderSummary{}, ErrNotFound
		}
	}
	var result FolderSummary
	err = service.database.QueryRow(ctx, `
		INSERT INTO occccad.folders(parent_id,name,description,owner_user_id) VALUES($1,$2,$3,$4)
		RETURNING id::text,parent_id::text,name,description,0,0,0,created_at::text,updated_at::text,'OWNER'`,
		parentID, name, description, actorID(request.ActorID)).Scan(&result.ID, &result.ParentID, &result.Name,
		&result.Description, &result.DocumentCount, &result.TrashCount, &result.ChildCount,
		&result.CreatedAt, &result.UpdatedAt, &result.Permission)
	if isUniqueViolation(err) {
		return FolderSummary{}, fmt.Errorf("%w: a folder with this name already exists here", ErrValidation)
	}
	return result, err
}

func (service *Service) UpdateFolder(
	ctx context.Context, folderID string, request UpdateFolderRequest,
) (FolderSummary, error) {
	name, description, err := validateNameAndDescription(request.Name, request.Description, "folder")
	if err != nil {
		return FolderSummary{}, err
	}
	var result FolderSummary
	err = service.database.QueryRow(ctx, `
		UPDATE occccad.folders SET name=$1,description=$2,updated_at=now() WHERE id=$3
		RETURNING id::text,parent_id::text,name,description,
		  (SELECT count(*) FROM occccad.documents d WHERE d.folder_id=$3 AND d.deleted_at IS NULL),
		  (SELECT count(*) FROM occccad.documents d WHERE d.folder_id=$3 AND d.deleted_at IS NOT NULL),
		  (SELECT count(*) FROM occccad.folders child WHERE child.parent_id=$3),
		  created_at::text,updated_at::text`, name, description, folderID).Scan(
		&result.ID, &result.ParentID, &result.Name, &result.Description,
		&result.DocumentCount, &result.TrashCount, &result.ChildCount, &result.CreatedAt, &result.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return FolderSummary{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return FolderSummary{}, fmt.Errorf("%w: a folder with this name already exists here", ErrValidation)
	}
	return result, err
}

func (service *Service) DeleteFolder(ctx context.Context, folderID string) error {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM occccad.resource_grants
		WHERE resource_type='FOLDER' AND resource_id=$1`, folderID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		DELETE FROM occccad.folders f WHERE f.id=$1
		AND NOT EXISTS(SELECT 1 FROM occccad.folders child WHERE child.parent_id=f.id)
		AND NOT EXISTS(SELECT 1 FROM occccad.documents d WHERE d.folder_id=f.id)`, folderID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		return tx.Commit(ctx)
	}
	_ = tx.Rollback(ctx)
	var exists bool
	if err := service.database.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM occccad.folders WHERE id=$1)`, folderID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return fmt.Errorf("%w: folder must be empty before deletion", ErrValidation)
}

func isUniqueViolation(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == "23505"
}

func (service *Service) UpdateDocument(
	ctx context.Context, documentID string, request UpdateDocumentRequest,
) (DocumentView, error) {
	name := strings.TrimSpace(request.Name)
	description := strings.TrimSpace(request.Description)
	if name == "" || len(name) > 120 {
		return DocumentView{}, fmt.Errorf("%w: name is required and must not exceed 120 characters", ErrValidation)
	}
	if len(description) > 500 {
		return DocumentView{}, fmt.Errorf("%w: description must not exceed 500 characters", ErrValidation)
	}
	if err := service.changeDocumentMetadata(ctx, documentID, request.RequestID,
		"UPDATE_DOCUMENT", request, name, description); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID)
}

func (service *Service) DeleteDocument(ctx context.Context, documentID, requestID string) error {
	return service.changeDocumentMetadata(ctx, documentID, requestID,
		"DELETE_DOCUMENT", map[string]string{"requestId": requestID}, "", "")
}

func (service *Service) RestoreDocument(ctx context.Context, documentID, requestID string) (DocumentView, error) {
	if err := service.changeDocumentMetadata(ctx, documentID, requestID,
		"RESTORE_DOCUMENT", map[string]string{"requestId": requestID}, "", ""); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID)
}

// PurgeDocument permanently removes a document only after it has entered the trash.
// Immutable artifact objects are retained for the artifact garbage collector; all
// document-owned relational state is removed through database cascades.
func (service *Service) PurgeDocument(ctx context.Context, documentID string) error {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM occccad.resource_grants
		WHERE resource_type='DOCUMENT' AND resource_id=$1`, documentID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM occccad.documents WHERE id=$1 AND deleted_at IS NOT NULL`, documentID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		return tx.Commit(ctx)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM occccad.documents WHERE id=$1)`, documentID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return fmt.Errorf("%w: move the document to trash before permanent deletion", ErrValidation)
}

func (service *Service) MoveDocument(
	ctx context.Context, documentID, requestIDValue string, request MoveDocumentRequest,
) (DocumentView, error) {
	folderID, err := normalizeOptionalUUID(request.FolderID, "folder id")
	if err != nil {
		return DocumentView{}, err
	}
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return DocumentView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var headVersion string
	if err := tx.QueryRow(ctx, `
		SELECT head_version_id::text FROM occccad.documents
		WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, documentID).Scan(&headVersion); errors.Is(err, pgx.ErrNoRows) {
		return DocumentView{}, ErrNotFound
	} else if err != nil {
		return DocumentView{}, err
	}
	if folderID != nil {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM occccad.folders WHERE id=$1)`, *folderID).Scan(&exists); err != nil {
			return DocumentView{}, err
		}
		if !exists {
			return DocumentView{}, ErrNotFound
		}
	}
	payload, _ := json.Marshal(request)
	traceID, spanID := traceIDs(ctx)
	var commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,'MOVE_DOCUMENT',$2,$3,'SUCCEEDED',now(),$4,$5) RETURNING id::text`,
		requestID(requestIDValue), documentID, payload, traceID, spanID).Scan(&commandID); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE occccad.documents SET folder_id=$1,updated_at=now() WHERE id=$2`, folderID, documentID); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,'MOVE_DOCUMENT')`, documentID, headVersion, commandID); err != nil {
		return DocumentView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID)
}

func (service *Service) CopyDocument(
	ctx context.Context, sourceDocumentID string, request CopyDocumentRequest,
) (DocumentView, error) {
	name, _, err := validateNameAndDescription(request.Name, "", "document")
	if err != nil {
		return DocumentView{}, err
	}
	var documentType, description string
	var sourceFolderID, sourceGeometryKey *string
	var modelJSON []byte
	if err := service.database.QueryRow(ctx, `
		SELECT d.document_type,d.description,d.folder_id::text,v.model_json,v.geometry_key
		FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id
		WHERE d.id=$1 AND d.deleted_at IS NULL`, sourceDocumentID).Scan(
		&documentType, &description, &sourceFolderID, &modelJSON, &sourceGeometryKey); errors.Is(err, pgx.ErrNoRows) {
		return DocumentView{}, ErrNotFound
	} else if err != nil {
		return DocumentView{}, err
	}
	targetFolderID := sourceFolderID
	if request.FolderID != nil {
		targetFolderID, err = normalizeOptionalUUID(request.FolderID, "folder id")
		if err != nil {
			return DocumentView{}, err
		}
	}
	versionUUID, err := uuid.NewV7()
	if err != nil {
		return DocumentView{}, err
	}
	versionID := versionUUID.String()
	modelJSON, modelHash, initialGraph, initialManifest, dependencyDigest, err := prepareInitialEvaluation(documentType, versionID, modelJSON)
	if err != nil {
		return DocumentView{}, err
	}
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return DocumentView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if targetFolderID != nil {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM occccad.folders WHERE id=$1)`, *targetFolderID).Scan(&exists); err != nil {
			return DocumentView{}, err
		}
		if !exists {
			return DocumentView{}, ErrNotFound
		}
	}
	var documentID, commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.documents(document_type,name,description,folder_id,copied_from_document_id,owner_user_id)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, documentType, name, description,
		targetFolderID, sourceDocumentID, actorID(request.ActorID)).Scan(&documentID); err != nil {
		return DocumentView{}, err
	}
	payload, _ := json.Marshal(map[string]any{"sourceDocumentId": sourceDocumentID, "name": name})
	traceID, spanID := traceIDs(ctx)
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,'COPY_DOCUMENT',$2,$3,'SUCCEEDED',now(),$4,$5) RETURNING id::text`,
		requestID(request.RequestID), documentID, payload, traceID, spanID).Scan(&commandID); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_versions(id,document_id,sequence,model_json,geometry_key,state,created_by_command_id,model_hash)
		VALUES($1,$2,1,$3,$4,'READY',$5,$6)`,
		versionID, documentID, modelJSON, sourceGeometryKey, commandID, modelHash); err != nil {
		return DocumentView{}, err
	}
	if documentType == "PRODUCT" {
		var product ProductModel
		if err := json.Unmarshal(modelJSON, &product); err != nil {
			return DocumentView{}, err
		}
		if err := insertProductInstances(ctx, tx, versionID, product); err != nil {
			return DocumentView{}, err
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE occccad.documents SET head_version_id=$1 WHERE id=$2`, versionID, documentID); err != nil {
		return DocumentView{}, err
	}
	var workspaceID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.workspaces(document_id,name,head_revision_id,head_sequence,base_revision_id)
		VALUES($1,'main',$2,1,$2) RETURNING id::text`, documentID, versionID).Scan(&workspaceID); err != nil {
		return DocumentView{}, err
	}
	if err := persistEvaluationProjection(ctx, tx, versionID, documentType, modelHash, dependencyDigest, initialGraph, initialManifest); err != nil {
		return DocumentView{}, err
	}
	if err := persistInitialTransaction(ctx, tx, workspaceID, documentID, versionID, actorID(request.ActorID), modelHash, "occccad://document/copy", modelJSON); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_history(document_id,position,version_id,command_id)
		VALUES($1,0,$2,$3)`, documentID, versionID, commandID); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,'COPY_DOCUMENT')`, documentID, versionID, commandID); err != nil {
		return DocumentView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID, request.ActorID)
}

func (service *Service) changeDocumentMetadata(
	ctx context.Context, documentID, requestIDValue, changeType string,
	payloadValue any, name, description string,
) error {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var headVersion string
	var deletedAt *string
	if err := tx.QueryRow(ctx, `
		SELECT head_version_id::text,deleted_at::text FROM occccad.documents
		WHERE id=$1 FOR UPDATE`, documentID).Scan(&headVersion, &deletedAt); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if changeType == "DELETE_DOCUMENT" && deletedAt != nil {
		return fmt.Errorf("%w: document is already in trash", ErrValidation)
	}
	if changeType == "RESTORE_DOCUMENT" && deletedAt == nil {
		return fmt.Errorf("%w: document is not in trash", ErrValidation)
	}
	if changeType == "UPDATE_DOCUMENT" && deletedAt != nil {
		return fmt.Errorf("%w: restore the document before editing it", ErrValidation)
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return err
	}
	traceID, spanID := traceIDs(ctx)
	var commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,$2,$3,$4,'SUCCEEDED',now(),$5,$6) RETURNING id::text`,
		requestID(requestIDValue), changeType, documentID, payload, traceID, spanID).Scan(&commandID); err != nil {
		return err
	}
	var updateResult pgconn.CommandTag
	switch changeType {
	case "UPDATE_DOCUMENT":
		updateResult, err = tx.Exec(ctx, `
			UPDATE occccad.documents SET name=$1,description=$2,updated_at=now() WHERE id=$3`,
			name, description, documentID)
	case "DELETE_DOCUMENT":
		updateResult, err = tx.Exec(ctx, `
			UPDATE occccad.documents SET deleted_at=now(),updated_at=now() WHERE id=$1`, documentID)
	case "RESTORE_DOCUMENT":
		updateResult, err = tx.Exec(ctx, `
			UPDATE occccad.documents SET deleted_at=NULL,updated_at=now() WHERE id=$1`, documentID)
	default:
		return fmt.Errorf("%w: unsupported document metadata change", ErrValidation)
	}
	if err != nil {
		return err
	}
	if updateResult.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,$4)`, documentID, headVersion, commandID, changeType); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (service *Service) ListHistory(ctx context.Context, documentID string) ([]HistoryEntry, error) {
	if err := service.requireActiveDocument(ctx, documentID); err != nil {
		return nil, err
	}
	rows, err := service.database.Query(ctx, `
		WITH line AS (
			SELECT dc.*,(row_number() OVER (ORDER BY dc.id)-1)::integer AS position
			FROM occccad.document_changes dc WHERE dc.document_id=$1
		)
		SELECT line.position,line.version_id::text,v.sequence,line.change_type,line.created_at::text,
		       line.id=(SELECT max(dc.id) FROM occccad.document_changes dc WHERE dc.document_id=line.document_id),
		       CASE WHEN line.change_type='CREATE_VERSION' THEN coalesce(v.version_name,'') ELSE '' END
		FROM line
		JOIN occccad.documents d ON d.id=line.document_id
		JOIN occccad.document_versions v ON v.id=line.version_id
		ORDER BY line.id DESC`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []HistoryEntry{}
	for rows.Next() {
		var item HistoryEntry
		if err := rows.Scan(&item.Position, &item.VersionID, &item.Sequence,
			&item.CommandType, &item.CreatedAt, &item.IsHead, &item.VersionName); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) CreateVersion(
	ctx context.Context, documentID string, request CreateVersionRequest,
) ([]HistoryEntry, error) {
	if err := service.requireActiveDocument(ctx, documentID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || len(name) > 120 {
		return nil, fmt.Errorf("%w: version name is required and must not exceed 120 characters", ErrValidation)
	}
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var headVersion string
	if err := tx.QueryRow(ctx,
		`SELECT head_version_id::text FROM occccad.documents WHERE id=$1 FOR UPDATE`, documentID).
		Scan(&headVersion); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	result, err := tx.Exec(ctx, `
		UPDATE occccad.document_versions SET version_name=$1,version_description=$2
		WHERE id=$3 AND version_name IS NULL`, name, strings.TrimSpace(request.Description), headVersion)
	if err != nil {
		return nil, fmt.Errorf("create named version: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil, fmt.Errorf("%w: current history point already has a named version", ErrValidation)
	}
	payload, _ := json.Marshal(request)
	traceID, spanID := traceIDs(ctx)
	var commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,'CREATE_VERSION',$2,$3,'SUCCEEDED',now(),$4,$5) RETURNING id::text`,
		requestID(request.RequestID), documentID, payload, traceID, spanID).Scan(&commandID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,'CREATE_VERSION')`, documentID, headVersion, commandID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return service.ListHistory(ctx, documentID)
}

func (service *Service) CommitImportedPart(ctx context.Context, actor, folderID, reqID, name,
	fileName, format, geometryKey string, evaluation *workerv1.EvaluatePartResponse) (DocumentView, error) {
	if err := service.storeEvaluation(ctx, geometryKey, evaluation, visualizationManifest(newPartModel())); err != nil {
		return DocumentView{}, err
	}
	var folder *string
	if strings.TrimSpace(folderID) != "" {
		folder = &folderID
	}
	view, err := service.CreateDocument(ctx, CreateDocumentRequest{RequestID: reqID + "/document",
		Name: name, Type: "PART", FolderID: folder, ActorID: actor})
	if err != nil {
		return DocumentView{}, err
	}
	return service.ApplyCommand(ctx, view.Document.ID, CommandRequest{RequestID: reqID + "/import",
		Type: "IMPORT_EXCHANGE", GeometryKey: geometryKey, FileName: strings.TrimSpace(fileName),
		SourceFormat: strings.ToUpper(format), ActorID: actor})
}

func (service *Service) CommitImportedProduct(ctx context.Context, actor, folderID, reqID, name string,
	parts []DocumentView) (DocumentView, error) {
	var folder *string
	if strings.TrimSpace(folderID) != "" {
		folder = &folderID
	}
	view, err := service.CreateDocument(ctx, CreateDocumentRequest{RequestID: reqID + "/document",
		Name: name, Type: "PRODUCT", FolderID: folder, ActorID: actor})
	if err != nil {
		return DocumentView{}, err
	}
	for index, part := range parts {
		view, err = service.ApplyCommand(ctx, view.Document.ID, CommandRequest{
			RequestID: reqID + fmt.Sprintf("/instance/%d", index), Type: "INSERT_INSTANCE",
			ReferencedDocumentID: part.Document.ID, Name: part.Document.Name,
			ReferenceMode: "PINNED", ActorID: actor,
		})
		if err != nil {
			return DocumentView{}, err
		}
	}
	return view, nil
}

type ExchangeExportComponent struct {
	Name        string
	BRep        geometry.ArtifactReference
	Translation [3]float64
}

func (service *Service) ExchangeExportComponents(ctx context.Context, documentID string) (string, string, []ExchangeExportComponent, error) {
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return "", "", nil, err
	}
	components := []ExchangeExportComponent{}
	if view.Document.Type == "PART" {
		if view.Artifact == nil || view.Artifact.Volume <= 0 {
			return "", "", nil, fmt.Errorf("%w: Part has no solid geometry to export", ErrValidation)
		}
		reference, err := service.brepArtifactReference(ctx, view.Artifact.GeometryKey)
		if err != nil {
			return "", "", nil, err
		}
		components = append(components, ExchangeExportComponent{Name: view.Document.Name, BRep: reference})
	} else {
		for _, instance := range view.ResolvedInstances {
			reference, err := service.brepArtifactReference(ctx, instance.GeometryKey)
			if err != nil {
				return "", "", nil, err
			}
			components = append(components, ExchangeExportComponent{Name: instance.Name,
				BRep: reference, Translation: instance.Translation})
		}
		if len(components) == 0 {
			return "", "", nil, fmt.Errorf("%w: Product has no resolvable Part geometry to export", ErrValidation)
		}
	}
	return view.Document.Name, view.Document.Type, components, nil
}

func (service *Service) brepArtifactReference(ctx context.Context, geometryKey string) (geometry.ArtifactReference, error) {
	var objectID *string
	var inline []byte
	var backend, objectKey, sha256Value, contentType *string
	var objectSize *int64
	if err := service.database.QueryRow(ctx, `SELECT artifact.brep_object_id::text,artifact.brep_data,
		object.storage_backend,object.object_key,object.sha256,object.size_bytes,object.content_type
		FROM occccad.geometry_artifacts artifact LEFT JOIN occccad.artifact_objects object
		ON object.id=artifact.brep_object_id AND object.state='READY'
		WHERE artifact.geometry_key=$1 AND artifact.volume>0`, geometryKey).
		Scan(&objectID, &inline, &backend, &objectKey, &sha256Value, &objectSize, &contentType); err != nil {
		return geometry.ArtifactReference{}, err
	}
	if objectID != nil && backend != nil && objectKey != nil && sha256Value != nil && objectSize != nil && contentType != nil {
		return geometry.ArtifactReference{Backend: *backend, ObjectKey: *objectKey, SHA256: *sha256Value,
			Size: *objectSize, ContentType: *contentType}, nil
	}
	if objectID == nil {
		if service.artifacts == nil || len(inline) == 0 {
			return geometry.ArtifactReference{}, fmt.Errorf("%w: B-Rep artifact is unavailable", ErrValidation)
		}
		object, err := service.artifacts.Put(ctx, artifactstore.KindBREP,
			"application/vnd.opencascade.brep", bytes.NewReader(inline))
		if err != nil {
			return geometry.ArtifactReference{}, err
		}
		if _, err := service.database.Exec(ctx, `UPDATE occccad.geometry_artifacts
			SET brep_object_id=$2,storage_state=CASE WHEN storage_state='DATABASE' THEN 'DUAL' ELSE storage_state END
			WHERE geometry_key=$1`, geometryKey, object.ID); err != nil {
			return geometry.ArtifactReference{}, err
		}
		return geometry.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key,
			SHA256: object.SHA256, Size: object.Size, ContentType: object.ContentType}, nil
	}
	return geometry.ArtifactReference{}, fmt.Errorf("%w: B-Rep artifact metadata is unavailable", ErrValidation)
}

func (service *Service) CreateDocument(
	ctx context.Context, request CreateDocumentRequest,
) (DocumentView, error) {
	if stableRequestID := strings.TrimSpace(request.RequestID); stableRequestID != "" {
		var existingDocumentID string
		err := service.database.QueryRow(ctx, `SELECT document_id::text FROM occccad.commands
			WHERE request_id=$1 AND command_type='CREATE_DOCUMENT' AND status='SUCCEEDED'`, stableRequestID).
			Scan(&existingDocumentID)
		if err == nil {
			return service.GetDocument(ctx, existingDocumentID, request.ActorID)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return DocumentView{}, err
		}
	}
	documentType := strings.ToUpper(strings.TrimSpace(request.Type))
	if documentType != "PART" && documentType != "PRODUCT" {
		return DocumentView{}, fmt.Errorf("%w: type must be PART or PRODUCT", ErrValidation)
	}
	name, description, err := validateNameAndDescription(request.Name, request.Description, "document")
	if err != nil {
		return DocumentView{}, err
	}
	folderID, err := normalizeOptionalUUID(request.FolderID, "folder id")
	if err != nil {
		return DocumentView{}, err
	}
	if folderID != nil {
		var exists bool
		if err := service.database.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM occccad.folders WHERE id=$1)`, *folderID).Scan(&exists); err != nil {
			return DocumentView{}, err
		}
		if !exists {
			return DocumentView{}, ErrNotFound
		}
	}
	partModel := newPartModel()
	model := any(partModel)
	var initialGeometryKey *string
	if documentType == "PRODUCT" {
		model = ProductModel{Instances: []ProductInstance{}}
	} else {
		key, artifactErr := service.ensureVisualizationArtifact(ctx, partModel)
		if artifactErr != nil {
			return DocumentView{}, artifactErr
		}
		initialGeometryKey = &key
	}
	modelJSON, err := json.Marshal(model)
	if err != nil {
		return DocumentView{}, err
	}
	versionUUID, err := uuid.NewV7()
	if err != nil {
		return DocumentView{}, err
	}
	versionID := versionUUID.String()
	modelJSON, modelHash, initialGraph, initialManifest, dependencyDigest, err := prepareInitialEvaluation(documentType, versionID, modelJSON)
	if err != nil {
		return DocumentView{}, err
	}
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return DocumentView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var documentID, commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.documents(document_type,name,description,folder_id,owner_user_id) VALUES($1,$2,$3,$4,$5)
		RETURNING id::text`, documentType, name, description, folderID,
		actorID(request.ActorID)).Scan(&documentID); err != nil {
		return DocumentView{}, fmt.Errorf("create document: %w", err)
	}
	traceID, spanID := traceIDs(ctx)
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,'CREATE_DOCUMENT',$2,$3,'SUCCEEDED',now(),$4,$5) RETURNING id::text`,
		requestID(request.RequestID), documentID, modelJSON, traceID, spanID).Scan(&commandID); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_versions(id,document_id,sequence,model_json,geometry_key,state,created_by_command_id,model_hash)
		VALUES($1,$2,1,$3,$4,'READY',$5,$6)`,
		versionID, documentID, modelJSON, initialGeometryKey, commandID, modelHash); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`,
		versionID, documentID); err != nil {
		return DocumentView{}, err
	}
	var workspaceID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.workspaces(document_id,name,head_revision_id,head_sequence,base_revision_id)
		VALUES($1,'main',$2,1,$2) RETURNING id::text`, documentID, versionID).Scan(&workspaceID); err != nil {
		return DocumentView{}, err
	}
	if err := persistEvaluationProjection(ctx, tx, versionID, documentType, modelHash, dependencyDigest, initialGraph, initialManifest); err != nil {
		return DocumentView{}, err
	}
	if err := persistInitialTransaction(ctx, tx, workspaceID, documentID, versionID, actorID(request.ActorID), modelHash, "occccad://document/create", modelJSON); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_history(document_id,position,version_id,command_id)
		VALUES($1,0,$2,$3)`, documentID, versionID, commandID); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,'CREATE_DOCUMENT')`, documentID, versionID, commandID); err != nil {
		return DocumentView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID)
}

func (service *Service) GetDocument(ctx context.Context, documentID string, actors ...string) (DocumentView, error) {
	finishModel := perf.Start(ctx, "document-model")
	var summary DocumentSummary
	var modelJSON []byte
	var geometryKey *string
	err := service.database.QueryRow(ctx, `
		SELECT d.id::text,d.name,d.description,d.document_type,d.head_version_id::text,
		       d.created_at::text,
		       d.updated_at::text,d.deleted_at::text,d.folder_id::text,d.last_opened_at::text,
		       d.copied_from_document_id::text,d.workspace_name,
		       v.model_json,v.geometry_key
		FROM occccad.documents d
		JOIN occccad.document_versions v ON v.id=d.head_version_id
		WHERE d.id=$1 AND d.deleted_at IS NULL`, documentID).Scan(
		&summary.ID, &summary.Name, &summary.Description, &summary.Type, &summary.VersionID,
		&summary.CreatedAt, &summary.LastUpdated,
		&summary.DeletedAt, &summary.FolderID, &summary.LastOpenedAt, &summary.CopiedFromID,
		&summary.WorkspaceName,
		&modelJSON, &geometryKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentView{}, ErrNotFound
	}
	if err != nil {
		finishModel()
		return DocumentView{}, err
	}
	finishModel()
	if len(actors) > 0 {
		summary.CanUndo, summary.CanRedo, err = service.historyCapabilities(ctx, documentID, actors[0])
		if err != nil {
			return DocumentView{}, err
		}
	}
	// Opening a document is a read-hot path. Touch recency at most once every
	// five minutes instead of turning every command response and query into a write.
	if summary.LastOpenedAt == nil {
		_, _ = service.database.Exec(ctx, `UPDATE occccad.documents SET last_opened_at=now() WHERE id=$1 AND last_opened_at IS NULL`, documentID)
	}
	view := DocumentView{Document: summary}
	if summary.Type == "PART" {
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return view, err
		}
		normalizePartModel(&model)
		view.Part = &model
		view.DatumPlanes = model.DatumPlanes
		view.AxisSystems = model.AxisSystems
		view.DatumAxes = model.DatumAxes
		if geometryKey == nil {
			key, referenceErr := service.ensureVisualizationArtifact(ctx, model)
			if referenceErr != nil {
				return view, referenceErr
			}
			if _, updateErr := service.database.Exec(ctx,
				`UPDATE occccad.document_versions SET geometry_key=$1 WHERE id=$2 AND geometry_key IS NULL`,
				key, summary.VersionID); updateErr != nil {
				return view, updateErr
			}
			geometryKey = &key
		}
		if geometryKey != nil {
			finishArtifact := perf.Start(ctx, "artifact-load")
			if referenceErr := service.ensureArtifactVisualization(ctx, *geometryKey, model); referenceErr != nil {
				finishArtifact()
				return view, referenceErr
			}
			artifact, err := service.loadArtifact(ctx, *geometryKey)
			finishArtifact()
			if err != nil {
				return view, err
			}
			view.Artifact = &artifact
		}
		finishStructure := perf.Start(ctx, "structure-project")
		structure, err := service.buildDocumentStructure(ctx, summary.VersionID,
			"document:"+summary.ID, summary.Name, InstancePath{RootDocumentID: summary.ID}, map[string]bool{})
		finishStructure()
		if err != nil {
			return view, err
		}
		view.StructureTree = &structure
		return view, nil
	}
	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return view, err
	}
	for index := range model.Instances {
		instance := &model.Instances[index]
		if instance.ReferenceMode == "" {
			instance.ReferenceMode = "FOLLOW_HEAD"
		}
		instance.ResolvedVersionID = instance.ReferencedVersionID
		if instance.ReferenceMode == "FOLLOW_HEAD" {
			if err := service.database.QueryRow(ctx,
				`SELECT head_version_id::text FROM occccad.documents WHERE id=$1`,
				instance.ReferencedDocumentID).Scan(&instance.ResolvedVersionID); err != nil {
				return view, err
			}
			instance.HeadChanged = instance.ResolvedVersionID != instance.ReferencedVersionID
		}
	}
	view.Product = &model
	view.Artifacts = map[string]Artifact{}
	view.ResolvedInstances = []ResolvedInstance{}
	if err := service.resolveProduct(ctx, summary.VersionID, InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, summary.Name,
		InstancePath{RootDocumentID: summary.ID}, "document:"+summary.ID,
		map[string]bool{}, view.Artifacts, &view.ResolvedInstances); err != nil {
		return view, err
	}
	structure, err := service.buildDocumentStructure(ctx, summary.VersionID,
		"document:"+summary.ID, summary.Name, InstancePath{RootDocumentID: summary.ID}, map[string]bool{})
	if err != nil {
		return view, err
	}
	view.StructureTree = &structure
	return view, nil
}

func datumPlanes() []DatumPlane {
	return []DatumPlane{
		{ID: "datum-xy", Name: "XY Plane", Plane: "XY", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, Size: 180},
		{ID: "datum-xz", Name: "XZ Plane", Plane: "XZ", Normal: [3]float64{0, -1, 0}, UDirection: [3]float64{1, 0, 0}, Size: 180},
		{ID: "datum-yz", Name: "YZ Plane", Plane: "YZ", Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}, Size: 180},
	}
}

func defaultAxisSystems() []AxisSystem {
	return []AxisSystem{{ID: "axis-system-default", Name: "Absolute Axis System",
		XDirection: [3]float64{1, 0, 0}, YDirection: [3]float64{0, 1, 0}, ZDirection: [3]float64{0, 0, 1}}}
}

func resolveRevolveAxis(model PartModel, sketch Feature, reference string) ([2]float64, [2]float64, error) {
	var zero [2]float64
	if sketch.Sketch == nil {
		return zero, zero, fmt.Errorf("%w: revolve profile is not a sketch", ErrValidation)
	}
	for _, entity := range sketch.Sketch.Entities {
		if (reference == entity.ID || reference == "SKETCH:"+entity.ID) && entity.Kind == "LINE" && entity.Start != nil && entity.End != nil {
			return [2]float64{entity.Start.X, entity.Start.Y}, [2]float64{entity.End.X, entity.End.Y}, nil
		}
	}
	var origin, direction [3]float64
	found := false
	parts := strings.Split(reference, ":")
	if len(parts) == 3 && parts[0] == "SKETCH_LINE" {
		for _, source := range model.Features {
			if source.ID != parts[1] || source.Sketch == nil {
				continue
			}
			var sourceDatum *DatumPlane
			for index := range model.DatumPlanes {
				if model.DatumPlanes[index].ID == source.Sketch.Support.DatumPlaneID {
					sourceDatum = &model.DatumPlanes[index]
					break
				}
			}
			if sourceDatum == nil {
				break
			}
			cross := func(a, b [3]float64) [3]float64 {
				return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
			}
			v := cross(sourceDatum.Normal, sourceDatum.UDirection)
			toWorld := func(point SketchPoint2) [3]float64 {
				return [3]float64{
					sourceDatum.Origin[0] + sourceDatum.UDirection[0]*point.X + v[0]*point.Y,
					sourceDatum.Origin[1] + sourceDatum.UDirection[1]*point.X + v[1]*point.Y,
					sourceDatum.Origin[2] + sourceDatum.UDirection[2]*point.X + v[2]*point.Y}
			}
			for _, entity := range source.Sketch.Entities {
				if entity.ID == parts[2] && entity.Kind == "LINE" && entity.Start != nil && entity.End != nil {
					origin = toWorld(*entity.Start)
					end := toWorld(*entity.End)
					direction = [3]float64{end[0] - origin[0], end[1] - origin[1], end[2] - origin[2]}
					found = true
					break
				}
			}
			break
		}
	} else if len(parts) == 3 && parts[0] == "AXIS_SYSTEM" {
		for _, system := range model.AxisSystems {
			if system.ID != parts[1] {
				continue
			}
			origin = system.Origin
			switch parts[2] {
			case "X":
				direction = system.XDirection
			case "Y":
				direction = system.YDirection
			case "Z":
				direction = system.ZDirection
			default:
				continue
			}
			found = true
			break
		}
	} else if len(parts) == 2 && parts[0] == "DATUM_AXIS" {
		for _, axis := range model.DatumAxes {
			if axis.ID == parts[1] {
				origin, direction, found = axis.Origin, axis.Direction, true
				break
			}
		}
	}
	if !found {
		return zero, zero, fmt.Errorf("%w: revolve axis %s is missing", ErrValidation, reference)
	}
	var datum *DatumPlane
	for index := range model.DatumPlanes {
		if model.DatumPlanes[index].ID == sketch.Sketch.Support.DatumPlaneID {
			datum = &model.DatumPlanes[index]
			break
		}
	}
	if datum == nil {
		return zero, zero, fmt.Errorf("%w: sketch support plane is missing", ErrValidation)
	}
	dot := func(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
	cross := func(a, b [3]float64) [3]float64 {
		return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
	}
	n, u := datum.Normal, datum.UDirection
	v := cross(n, u)
	rel := [3]float64{origin[0] - datum.Origin[0], origin[1] - datum.Origin[1], origin[2] - datum.Origin[2]}
	if math.Abs(dot(direction, n)) > 1e-8 || math.Abs(dot(rel, n)) > 1e-6 {
		return zero, zero, fmt.Errorf("%w: revolve axis must lie in the sketch support plane", ErrValidation)
	}
	start := [2]float64{dot(rel, u), dot(rel, v)}
	endRel := [3]float64{rel[0] + direction[0], rel[1] + direction[1], rel[2] + direction[2]}
	end := [2]float64{dot(endRel, u), dot(endRel, v)}
	return start, end, nil
}

func newPartModel() PartModel {
	return PartModel{Units: "mm", DatumPlanes: datumPlanes(), AxisSystems: defaultAxisSystems(), DatumAxes: []DatumAxis{}, Features: []Feature{}, Parameters: []modelcore.ParameterDefinition{}}
}

func normalizePartModel(model *PartModel) {
	if model.Units == "" {
		model.Units = "mm"
	}
	if len(model.DatumPlanes) == 0 {
		model.DatumPlanes = datumPlanes()
	}
	for index := range model.DatumPlanes {
		plane := &model.DatumPlanes[index]
		if plane.UDirection != [3]float64{} {
			continue
		}
		switch strings.ToUpper(plane.Plane) {
		case "YZ":
			plane.UDirection = [3]float64{0, 1, 0}
		default:
			plane.UDirection = [3]float64{1, 0, 0}
		}
	}
	if len(model.AxisSystems) == 0 {
		model.AxisSystems = defaultAxisSystems()
	}
	if model.DatumAxes == nil {
		model.DatumAxes = []DatumAxis{}
	}
	if model.Features == nil {
		model.Features = []Feature{}
	}
	if model.Parameters == nil {
		model.Parameters = []modelcore.ParameterDefinition{}
	}
	ensureFeatureParameters(model)
}

func referenceGeometry(model PartModel) ReferenceGeometry {
	normalizePartModel(&model)
	return ReferenceGeometry{DatumPlanes: model.DatumPlanes, AxisSystems: model.AxisSystems, DatumAxes: model.DatumAxes}
}

func visualizationManifest(model PartModel) VisualizationManifest {
	normalizePartModel(&model)
	datumByID := map[string]DatumPlane{}
	for _, datum := range model.DatumPlanes {
		datumByID[datum.ID] = datum
	}
	manifest := VisualizationManifest{
		SchemaVersion:     1,
		ReferenceGeometry: referenceGeometry(model),
		Primitives:        []VisualPrimitive{},
	}
	for _, feature := range model.Features {
		if feature.Sketch == nil {
			continue
		}
		plane := strings.ToUpper(feature.Sketch.Support.Plane)
		if plane == "" {
			plane = strings.ToUpper(feature.Plane)
		}
		if plane == "" {
			plane = "XY"
		}
		datum, hasDatum := datumByID[feature.Sketch.Support.DatumPlaneID]
		toWorld := func(point SketchPoint2) [3]float64 {
			if hasDatum {
				u, n := datum.UDirection, datum.Normal
				v := [3]float64{n[1]*u[2] - n[2]*u[1], n[2]*u[0] - n[0]*u[2], n[0]*u[1] - n[1]*u[0]}
				return [3]float64{datum.Origin[0] + u[0]*point.X + v[0]*point.Y,
					datum.Origin[1] + u[1]*point.X + v[1]*point.Y,
					datum.Origin[2] + u[2]*point.X + v[2]*point.Y}
			}
			switch plane {
			case "XZ":
				return [3]float64{point.X, 0, point.Y}
			case "YZ":
				return [3]float64{0, point.X, point.Y}
			default:
				return [3]float64{point.X, point.Y, 0}
			}
		}
		for _, entity := range feature.Sketch.Entities {
			if entity.Suppressed {
				continue
			}
			primitive := VisualPrimitive{ID: entity.ID, FeatureID: feature.ID,
				EntityType: entity.Kind, Role: entity.Role, Status: feature.Sketch.Solve.Status, Selectable: true}
			switch entity.Kind {
			case "POINT":
				if entity.Point == nil {
					continue
				}
				primitive.Kind = "POINTS"
				primitive.Semantic = "SKETCH_POINT"
				primitive.Positions = [][3]float64{toWorld(*entity.Point)}
			case "LINE":
				if entity.Start == nil || entity.End == nil {
					continue
				}
				primitive.Kind = "POLYLINE"
				primitive.Semantic = "SKETCH_CURVE"
				primitive.Positions = [][3]float64{toWorld(*entity.Start), toWorld(*entity.End)}
			case "CIRCLE", "ARC", "SPLINE":
				points := sampleProfileCurve(profileCurve(entity, false))
				if len(points) < 2 {
					continue
				}
				primitive.Kind, primitive.Semantic = "POLYLINE", "SKETCH_CURVE"
				for _, point := range points {
					primitive.Positions = append(primitive.Positions, toWorld(point))
				}
			default:
				continue
			}
			manifest.Primitives = append(manifest.Primitives, primitive)
			auxiliaryPoints := []struct {
				suffix string
				point  SketchPoint2
			}{}
			if (entity.Kind == "CIRCLE" || entity.Kind == "ARC") && entity.Center != nil {
				auxiliaryPoints = append(auxiliaryPoints, struct {
					suffix string
					point  SketchPoint2
				}{"center", *entity.Center})
			}
			if entity.Kind == "ARC" {
				if first, last, ok := entityProfileEndpoints(entity); ok {
					auxiliaryPoints = append(auxiliaryPoints, struct {
						suffix string
						point  SketchPoint2
					}{"start", first}, struct {
						suffix string
						point  SketchPoint2
					}{"end", last})
				}
			}
			if entity.Kind == "SPLINE" {
				for index, point := range entity.ControlPoints {
					auxiliaryPoints = append(auxiliaryPoints, struct {
						suffix string
						point  SketchPoint2
					}{fmt.Sprintf("fit-%d", index), point})
				}
			}
			for _, auxiliary := range auxiliaryPoints {
				manifest.Primitives = append(manifest.Primitives, VisualPrimitive{ID: entity.ID + ":" + auxiliary.suffix, FeatureID: feature.ID,
					Kind: "POINTS", Semantic: "SKETCH_POINT", EntityType: "REFERENCE_POINT", Role: entity.Role,
					Status: feature.Sketch.Solve.Status, Positions: [][3]float64{toWorld(auxiliary.point)}, Selectable: false})
			}
		}
		entities := make(map[string]SketchEntity, len(feature.Sketch.Entities))
		for _, entity := range feature.Sketch.Entities {
			entities[entity.ID] = entity
		}
		for _, constraint := range feature.Sketch.Constraints {
			visual, ok := constraintVisual(constraint, entities)
			if !ok {
				continue
			}
			primitive := VisualPrimitive{
				ID: constraint.ID, FeatureID: feature.ID, Kind: visual.Kind,
				EntityType: constraint.Kind, Semantic: "SKETCH_CONSTRAINT",
				Status: feature.Sketch.Solve.Status, Label: visual.Label, Selectable: true,
			}
			seenRelated := map[string]bool{}
			for _, reference := range constraint.References {
				if reference.EntityID != "" && !seenRelated[reference.EntityID] {
					primitive.RelatedEntityIDs = append(primitive.RelatedEntityIDs, reference.EntityID)
					seenRelated[reference.EntityID] = true
				}
			}
			for _, point := range visual.Positions {
				primitive.Positions = append(primitive.Positions, toWorld(point))
			}
			if visual.LabelPosition != nil {
				worldPosition := toWorld(*visual.LabelPosition)
				primitive.LabelPosition = &worldPosition
			}
			manifest.Primitives = append(manifest.Primitives, primitive)
		}
	}
	return manifest
}

func (service *Service) ApplyCommand(
	ctx context.Context, documentID string, request CommandRequest,
) (DocumentView, error) {
	if err := service.requireActiveDocument(ctx, documentID); err != nil {
		return DocumentView{}, err
	}
	request.Type = strings.ToUpper(strings.TrimSpace(request.Type))
	if request.Type == "UNDO" || request.Type == "REDO" {
		if err := service.applyCompensatingHistory(ctx, documentID, request); err != nil {
			return DocumentView{}, err
		}
		return service.GetDocument(ctx, documentID, request.ActorID)
	}
	if request.Type == "RESTORE" {
		if err := service.applyRestoreRevision(ctx, documentID, request); err != nil {
			return DocumentView{}, err
		}
		return service.GetDocument(ctx, documentID, request.ActorID)
	}
	if err := service.applyDomainMutation(ctx, documentID, request); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID, request.ActorID)
}

func (service *Service) requireActiveDocument(ctx context.Context, documentID string) error {
	var exists bool
	if err := service.database.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL)`,
		documentID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func mutatePart(model *PartModel, request CommandRequest) error {
	switch request.Type {
	case "CREATE_SKETCH":
		plane := strings.ToUpper(request.Plane)
		datumID := strings.TrimSpace(request.DatumPlaneID)
		if datumID == "" {
			if plane != "XY" && plane != "XZ" && plane != "YZ" {
				return fmt.Errorf("%w: select a datum plane", ErrValidation)
			}
			datumID = "datum-" + strings.ToLower(plane)
		} else {
			found := false
			for _, datum := range model.DatumPlanes {
				if datum.ID == datumID {
					plane, found = datum.Plane, true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: selected datum plane does not exist", ErrValidation)
			}
		}
		model.Features = append(model.Features, Feature{
			ID: newID("sketch"), Type: "SKETCH",
			Name:  numberedFeatureName(model.Features, "SKETCH", "Sketch"),
			Plane: plane, Sketch: &SketchFeature{SchemaVersion: 1, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: datumID, Plane: plane}, Entities: []SketchEntity{}, Constraints: []SketchConstraint{}, Solve: SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY"}},
		})
	case "EDIT_SKETCH":
		for index := range model.Features {
			if model.Features[index].ID == request.SketchID && model.Features[index].Sketch != nil {
				return applySketchOperations(model.Features[index].Sketch, request.Operations)
			}
		}
		return fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
	case "PAD_SKETCH":
		if !positiveFinite(request.Length) {
			return fmt.Errorf("%w: pad length must be a positive finite value", ErrValidation)
		}
		var sketch *Feature
		for index := range model.Features {
			if model.Features[index].ID == request.SketchID &&
				strings.Contains(strings.ToUpper(model.Features[index].Type), "SKETCH") {
				sketch = &model.Features[index]
				break
			}
		}
		if sketch == nil {
			return fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
		}
		model.Features = append(model.Features, Feature{
			ID: newID("extrude"), Type: "PAD",
			Name:    numberedFeatureName(model.Features, "PAD", "Extrude"),
			Profile: sketch.ID, Length: request.Length, Operation: "ADD",
		})
	case "CREATE_SOLID_FEATURE":
		generator := strings.ToUpper(request.Generator)
		operation := strings.ToUpper(request.Operation)
		if generator != "LINEAR_EXTRUDE" && generator != "REVOLVE" {
			return fmt.Errorf("%w: unsupported solid generator", ErrValidation)
		}
		if operation != "NEW_BODY" && operation != "ADD" && operation != "REMOVE" && operation != "INTERSECT" {
			return fmt.Errorf("%w: invalid BodyOperation", ErrValidation)
		}
		var sketch *Feature
		for index := range model.Features {
			if model.Features[index].ID == request.SketchID && model.Features[index].Sketch != nil {
				sketch = &model.Features[index]
				break
			}
		}
		if sketch == nil {
			return fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
		}
		if generator == "REVOLVE" {
			if _, _, err := resolveRevolveAxis(*model, *sketch, request.AxisEntityID); err != nil {
				return err
			}
		}
		label := "Extrude"
		if generator == "REVOLVE" {
			label = "Revolve"
		}
		model.Features = append(model.Features, Feature{ID: newID(strings.ToLower(label)), Type: generator,
			Name: numberedFeatureName(model.Features, generator, label), Profile: sketch.ID,
			Length: request.Length, Angle: request.Angle, Operation: operation,
			AxisEntityID: request.AxisEntityID, Reversed: request.Reversed})
	case "IMPORT_EXCHANGE":
		if strings.TrimSpace(request.GeometryKey) == "" {
			return fmt.Errorf("%w: imported geometry key is required", ErrValidation)
		}
		if len(model.Features) != 0 {
			return fmt.Errorf("%w: STEP import requires an empty Part", ErrValidation)
		}
		name := strings.TrimSpace(request.FileName)
		if name == "" {
			name = "Imported STEP"
		}
		model.Features = append(model.Features, Feature{
			ID: newID("import"), Type: "IMPORT_BODY", Name: "Import " + name,
			GeometryKey: request.GeometryKey, FileName: name, SourceFormat: strings.ToUpper(request.SourceFormat),
		})
	default:
		return fmt.Errorf("%w: command %s is not valid for a Part", ErrValidation, request.Type)
	}
	return nil
}

func numberedFeatureName(features []Feature, featureType, label string) string {
	count := 0
	for _, feature := range features {
		if strings.EqualFold(feature.Type, featureType) ||
			(featureType == "SKETCH" && strings.Contains(strings.ToUpper(feature.Type), "SKETCH")) {
			count++
		}
	}
	return fmt.Sprintf("%s %d", label, count+1)
}

func (service *Service) mutateProduct(
	ctx context.Context, tx pgx.Tx, documentID string, model *ProductModel, request CommandRequest,
) error {
	switch request.Type {
	case "INSERT_INSTANCE":
		var referenceID, versionID, name string
		if err := tx.QueryRow(ctx, `
			SELECT id::text,head_version_id::text,name FROM occccad.documents WHERE id=$1`,
			request.ReferencedDocumentID).Scan(&referenceID, &versionID, &name); errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: referenced document does not exist", ErrValidation)
		} else if err != nil {
			return err
		}
		if referenceID == documentID {
			return fmt.Errorf("%w: a Product cannot contain itself", ErrValidation)
		}
		var createsCycle bool
		if err := tx.QueryRow(ctx, `
			WITH RECURSIVE graph(document_id) AS (
				SELECT $1::uuid UNION
				SELECT pi.referenced_document_id FROM graph g
				JOIN occccad.documents d ON d.id=g.document_id
				JOIN occccad.product_instances pi ON pi.product_version_id=d.head_version_id
			)
			SELECT EXISTS(SELECT 1 FROM graph WHERE document_id=$2::uuid)`,
			referenceID, documentID).Scan(&createsCycle); err != nil {
			return err
		}
		if createsCycle {
			return fmt.Errorf("%w: Product reference would create a cycle", ErrValidation)
		}
		instanceName := nextInstanceName(*model, name)
		model.Instances = append(model.Instances, ProductInstance{
			ID: newID("instance"), Name: instanceName,
			ReferencedDocumentID: referenceID, ReferencedVersionID: versionID,
			Translation:   request.Translation,
			ReferenceMode: "FOLLOW_HEAD",
		})
	case "MOVE_INSTANCE":
		if !finite(request.Translation[0]) || !finite(request.Translation[1]) || !finite(request.Translation[2]) {
			return fmt.Errorf("%w: translation must be finite", ErrValidation)
		}
		found := false
		for index := range model.Instances {
			if model.Instances[index].ID == request.InstanceID {
				model.Instances[index].Translation = request.Translation
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: selected instance does not exist", ErrValidation)
		}
	case "SET_REFERENCE_MODE":
		mode := strings.ToUpper(request.ReferenceMode)
		if mode != "FOLLOW_HEAD" && mode != "PINNED" {
			return fmt.Errorf("%w: reference mode must be FOLLOW_HEAD or PINNED", ErrValidation)
		}
		found := false
		for index := range model.Instances {
			instance := &model.Instances[index]
			if instance.ID != request.InstanceID {
				continue
			}
			instance.ReferenceMode = mode
			if mode == "PINNED" {
				if err := tx.QueryRow(ctx,
					`SELECT head_version_id::text FROM occccad.documents WHERE id=$1`,
					instance.ReferencedDocumentID).Scan(&instance.ReferencedVersionID); err != nil {
					return err
				}
			}
			instance.ResolvedVersionID = ""
			instance.HeadChanged = false
			found = true
			break
		}
		if !found {
			return fmt.Errorf("%w: selected instance does not exist", ErrValidation)
		}
	default:
		return fmt.Errorf("%w: command %s is not valid for a Product", ErrValidation, request.Type)
	}
	return nil
}

func (service *Service) evaluatePart(ctx context.Context, reqID string, model PartModel) (string, error) {
	normalizePartModel(&model)
	sketches := map[string]Feature{}
	solidFeatures := []geometry.ProfilePad{}
	baseKey := ""
	var canonical strings.Builder
	canonical.WriteString(evaluatorVersion)
	visualization := visualizationManifest(model)
	visualizationJSON, _ := json.Marshal(visualization)
	canonical.WriteString("|visualization=" + string(visualizationJSON))
	for _, feature := range model.Features {
		switch strings.ToUpper(feature.Type) {
		case "IMPORT_BODY":
			baseKey = feature.GeometryKey
			canonical.WriteString("|base=" + baseKey)
		case "SKETCH":
			sketches[feature.ID] = feature
		case "PAD", "LINEAR_EXTRUDE", "REVOLVE":
			sketch, exists := sketches[feature.Profile]
			if !exists {
				return "", fmt.Errorf("%w: extrude profile %s is missing or follows the extrude", ErrValidation, feature.Profile)
			}
			regions, err := buildProfileRegions(sketch)
			if err != nil {
				return "", err
			}
			plane := sketch.Plane
			if sketch.Sketch != nil && sketch.Sketch.Support.Plane != "" {
				plane = sketch.Sketch.Support.Plane
			}
			if plane == "" {
				plane = "XY"
			}
			generator := "LINEAR_EXTRUDE"
			angle := 0.0
			axisStart, axisEnd := [2]float64{}, [2]float64{}
			if strings.EqualFold(feature.Type, "REVOLVE") {
				generator = "REVOLVE"
				angle = feature.Angle * math.Pi / 180
				axisStart, axisEnd, err = resolveRevolveAxis(model, sketch, feature.AxisEntityID)
				if err != nil {
					return "", err
				}
			}
			operation := strings.ToUpper(feature.Operation)
			if operation == "" {
				operation = "ADD"
			}
			var planeOrigin, planeNormal, planeU [3]float64
			if sketch.Sketch != nil {
				for _, datum := range model.DatumPlanes {
					if datum.ID == sketch.Sketch.Support.DatumPlaneID {
						planeOrigin, planeNormal, planeU = datum.Origin, datum.Normal, datum.UDirection
						break
					}
				}
			}
			solidFeatures = append(solidFeatures, geometry.ProfilePad{Regions: regions, Length: feature.Length,
				Plane: plane, BodyOperation: operation, Generator: generator, RevolveAngle: angle,
				AxisStart: axisStart, AxisEnd: axisEnd, Reversed: feature.Reversed,
				PlaneOrigin: planeOrigin, PlaneNormal: planeNormal, PlaneUDirection: planeU})
			profileJSON, _ := json.Marshal(regions)
			fmt.Fprintf(&canonical, "|solid=%s,%s,%s,%s,%.9g,%.9g,%v,%v,%t", generator, operation,
				plane, profileJSON, feature.Length, angle, axisStart, axisEnd, feature.Reversed)
		}
	}
	if len(solidFeatures) == 0 {
		if baseKey != "" {
			return service.ensureVisualizationVariant(ctx, baseKey, visualization)
		}
		return service.ensureVisualizationArtifact(ctx, model)
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	key := "sha256:" + hex.EncodeToString(digest[:])
	var exists bool
	if err := service.database.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM occccad.geometry_artifacts WHERE geometry_key=$1)`, key).
		Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return key, nil
	}
	var evaluation *workerv1.EvaluatePartResponse
	var err error
	if service.artifacts != nil {
		var base geometry.ArtifactReference
		if baseKey != "" {
			base, err = service.brepArtifactReference(ctx, baseKey)
			if err != nil {
				return "", err
			}
		}
		evaluation, err = service.worker.EvaluateProfilePartFromArtifact(ctx, reqID, key, solidFeatures, base,
			artifactstore.StagingKey(reqID, "shape.brep"), artifactstore.StagingKey(reqID, "mesh.glb"))
	} else {
		var baseBRep []byte
		if baseKey != "" {
			err = service.database.QueryRow(ctx,
				`SELECT brep_data FROM occccad.geometry_artifacts WHERE geometry_key=$1`, baseKey).Scan(&baseBRep)
		}
		if err == nil {
			evaluation, err = service.worker.EvaluateProfilePart(ctx, reqID, key, solidFeatures, baseBRep)
		}
	}
	if err != nil {
		return "", err
	}
	if err := service.storeEvaluation(ctx, key, evaluation, visualization); err != nil {
		return "", err
	}
	return key, nil
}

func (service *Service) storeEvaluation(
	ctx context.Context, key string, evaluation *workerv1.EvaluatePartResponse, visualization VisualizationManifest,
) error {
	if evaluation.GetTopology().GetSolidCount() == 0 || evaluation.GetVolume() <= 0 {
		return fmt.Errorf("%w: imported/evaluated shape must contain solid geometry", ErrValidation)
	}
	glb := evaluation.GetGlbData()
	if reference := evaluation.GetGlbArtifact(); reference != nil {
		if service.artifacts == nil {
			return fmt.Errorf("GLB artifact response requires an ArtifactStore")
		}
		baseObject, err := service.artifacts.Adopt(ctx, artifactstore.KindGLB,
			"model/gltf-binary", reference.GetObjectKey())
		if err != nil {
			return fmt.Errorf("adopt worker GLB artifact: %w", err)
		}
		_, reader, err := service.artifacts.Open(ctx, baseObject.ID)
		if err != nil {
			return fmt.Errorf("open worker GLB artifact: %w", err)
		}
		glb, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return fmt.Errorf("read worker GLB artifact: %w", err)
		}
		if closeErr != nil {
			return fmt.Errorf("close worker GLB artifact: %w", closeErr)
		}
	}
	var err error
	glb, err = glbWithVisualization(glb, visualization)
	if err != nil {
		return fmt.Errorf("add visualization manifest to GLB: %w", err)
	}
	visualizationJSON, _ := json.Marshal(visualization)
	workerID := service.worker.WorkerFor(key)
	if workerID == "" {
		workerID = "geometry-worker"
		if worker, pingErr := service.worker.Ping(ctx); pingErr == nil && worker.GetWorkerId() != "" {
			workerID = worker.GetWorkerId()
		}
	}
	mesh := meshFromProto(evaluation.GetMesh())
	meshJSON, _ := json.Marshal(mesh)
	bboxJSON, _ := json.Marshal(map[string]any{
		"min": []float64{evaluation.GetBbox().GetMinX(), evaluation.GetBbox().GetMinY(), evaluation.GetBbox().GetMinZ()},
		"max": []float64{evaluation.GetBbox().GetMaxX(), evaluation.GetBbox().GetMaxY(), evaluation.GetBbox().GetMaxZ()},
	})
	topologyJSON, _ := json.Marshal(map[string]any{
		"faces": evaluation.GetTopology().GetFaceCount(), "edges": evaluation.GetTopology().GetEdgeCount(),
		"vertices": evaluation.GetTopology().GetVertexCount(), "solids": evaluation.GetTopology().GetSolidCount(),
	})
	var brepObjectID, glbObjectID *string
	storageState := "DATABASE"
	if service.artifacts != nil {
		var brepObject, glbObject artifactstore.Object
		var err error
		if reference := evaluation.GetBrepArtifact(); reference != nil {
			brepObject, err = service.artifacts.Adopt(ctx, artifactstore.KindBREP,
				"application/vnd.opencascade.brep", reference.GetObjectKey())
		} else {
			brepObject, err = service.artifacts.Put(ctx, artifactstore.KindBREP,
				"application/vnd.opencascade.brep", bytes.NewReader(evaluation.GetBrepData()))
		}
		if err != nil {
			return fmt.Errorf("store B-Rep artifact: %w", err)
		}
		glbObject, err = service.artifacts.Put(ctx, artifactstore.KindGLB,
			"model/gltf-binary", bytes.NewReader(glb))
		if err != nil {
			return fmt.Errorf("store GLB artifact: %w", err)
		}
		brepObjectID, glbObjectID = &brepObject.ID, &glbObject.ID
		if evaluation.GetBrepArtifact() != nil || evaluation.GetGlbArtifact() != nil {
			storageState = "OBJECT"
		} else {
			storageState = "DUAL"
		}
	}
	brepData, glbData := evaluation.GetBrepData(), glb
	if storageState == "OBJECT" {
		brepData, glbData = nil, nil
	}
	if _, err := service.database.Exec(ctx, `
		INSERT INTO occccad.geometry_artifacts(
			geometry_key,geometry_id,evaluator_version,occt_version,units,
			brep_data,glb_data,mesh_json,bbox_json,topology_json,volume,
			brep_object_id,glb_object_id,storage_state,visualization_json,worker_id)
		VALUES($1,$2,$3,$4,'mm',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (geometry_key) DO NOTHING`, key, evaluation.GetGeometryId(), evaluatorVersion,
		evaluation.GetOcctVersion(), brepData, glbData, meshJSON,
		bboxJSON, topologyJSON, evaluation.GetVolume(), brepObjectID, glbObjectID, storageState,
		visualizationJSON, workerID); err != nil {
		return err
	}
	return nil
}

func (service *Service) ensureVisualizationArtifact(ctx context.Context, model PartModel) (string, error) {
	visualization := visualizationManifest(model)
	visualizationJSON, _ := json.Marshal(visualization)
	digest := sha256.Sum256(append([]byte(evaluatorVersion+"|visualization-only|"), visualizationJSON...))
	key := "sha256:" + hex.EncodeToString(digest[:])
	var exists bool
	if err := service.database.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM occccad.geometry_artifacts WHERE geometry_key=$1)`, key).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return key, nil
	}
	glb, err := glbWithVisualization(nil, visualization)
	if err != nil {
		return "", err
	}
	meshJSON, _ := json.Marshal(Mesh{Vertices: [][3]float64{}, Triangles: [][3]uint32{}, FaceIDs: []uint32{}})
	bboxJSON := []byte(`{"min":[-90,-90,-90],"max":[90,90,90]}`)
	topologyJSON := []byte(`{"faces":0,"edges":0,"vertices":0,"solids":0}`)
	var glbObjectID *string
	storageState := "DATABASE"
	if service.artifacts != nil {
		object, putErr := service.artifacts.Put(ctx, artifactstore.KindGLB, "model/gltf-binary", bytes.NewReader(glb))
		if putErr != nil {
			return "", fmt.Errorf("store visualization GLB: %w", putErr)
		}
		glbObjectID, storageState = &object.ID, "DUAL"
	}
	_, err = service.database.Exec(ctx, `
		INSERT INTO occccad.geometry_artifacts(
			geometry_key,geometry_id,evaluator_version,occt_version,units,brep_data,glb_data,
			mesh_json,bbox_json,topology_json,volume,glb_object_id,storage_state,
			visualization_json,worker_id)
		VALUES($1,$2,$3,'none','mm','',$4,$5,$6,$7,0,$8,$9,$10,'metadata-service')
		ON CONFLICT (geometry_key) DO NOTHING`, key, "visualization:"+hex.EncodeToString(digest[:]), evaluatorVersion,
		glb, meshJSON, bboxJSON, topologyJSON, glbObjectID, storageState, visualizationJSON)
	return key, err
}

func (service *Service) ensureVisualizationVariant(
	ctx context.Context, baseKey string, visualization VisualizationManifest,
) (string, error) {
	visualizationJSON, _ := json.Marshal(visualization)
	digest := sha256.Sum256(append([]byte(evaluatorVersion+"|base="+baseKey+"|visualization="), visualizationJSON...))
	key := "sha256:" + hex.EncodeToString(digest[:])
	var exists bool
	if err := service.database.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM occccad.geometry_artifacts WHERE geometry_key=$1)`, key).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return key, nil
	}
	var sourceGLB []byte
	var sourceGLBObjectID *string
	var storageState string
	if err := service.database.QueryRow(ctx, `
		SELECT glb_data,glb_object_id::text,storage_state
		FROM occccad.geometry_artifacts WHERE geometry_key=$1`, baseKey).
		Scan(&sourceGLB, &sourceGLBObjectID, &storageState); err != nil {
		return "", err
	}
	if len(sourceGLB) == 0 {
		if service.artifacts == nil || sourceGLBObjectID == nil {
			return "", fmt.Errorf("%w: base visualization GLB is unavailable", ErrValidation)
		}
		_, reader, err := service.artifacts.Open(ctx, *sourceGLBObjectID)
		if err != nil {
			return "", err
		}
		sourceGLB, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	composed, err := glbWithVisualization(sourceGLB, visualization)
	if err != nil {
		return "", err
	}
	var glbObjectID *string
	if service.artifacts != nil {
		object, putErr := service.artifacts.Put(ctx, artifactstore.KindGLB, "model/gltf-binary", bytes.NewReader(composed))
		if putErr != nil {
			return "", putErr
		}
		glbObjectID = &object.ID
	}
	glbData := composed
	if storageState == "OBJECT" {
		glbData = nil
	}
	_, err = service.database.Exec(ctx, `
		INSERT INTO occccad.geometry_artifacts(
			geometry_key,geometry_id,evaluator_version,occt_version,units,brep_data,glb_data,
			mesh_json,bbox_json,topology_json,volume,evaluation_count,brep_object_id,glb_object_id,
			storage_state,visualization_json,worker_id)
		SELECT $1,geometry_id,$2,occt_version,units,brep_data,$3,mesh_json,bbox_json,topology_json,
		       volume,evaluation_count,brep_object_id,$4,storage_state,$5,worker_id
		FROM occccad.geometry_artifacts WHERE geometry_key=$6
		ON CONFLICT (geometry_key) DO NOTHING`, key, evaluatorVersion, glbData, glbObjectID,
		visualizationJSON, baseKey)
	return key, err
}

func (service *Service) ensureArtifactVisualization(ctx context.Context, key string, model PartModel) error {
	var stored []byte
	if err := service.database.QueryRow(ctx, `
		SELECT visualization_json FROM occccad.geometry_artifacts WHERE geometry_key=$1`, key).
		Scan(&stored); err != nil {
		return err
	}
	var current VisualizationManifest
	if json.Unmarshal(stored, &current) != nil || !reflect.DeepEqual(current, visualizationManifest(model)) {
		return fmt.Errorf("%w: visualization artifact does not match the Part revision", ErrValidation)
	}
	return nil
}

func meshFromProto(source *workerv1.Mesh) Mesh {
	result := Mesh{Vertices: make([][3]float64, 0, len(source.GetVertices())),
		Triangles: make([][3]uint32, 0, len(source.GetTriangles())), FaceIDs: append([]uint32{}, source.GetFaceIds()...),
		Edges: make([]MeshEdge, 0, len(source.GetEdges())), TopologyVertices: make([]TopologyPoint, 0, len(source.GetTopologyVertices()))}
	for _, vertex := range source.GetVertices() {
		result.Vertices = append(result.Vertices, [3]float64{vertex.GetX(), vertex.GetY(), vertex.GetZ()})
	}
	for _, triangle := range source.GetTriangles() {
		result.Triangles = append(result.Triangles, [3]uint32{triangle.GetV0(), triangle.GetV1(), triangle.GetV2()})
	}
	for _, edge := range source.GetEdges() {
		item := MeshEdge{LocalID: edge.GetLocalId(), Points: make([][3]float64, 0, len(edge.GetPoints()))}
		for _, point := range edge.GetPoints() {
			item.Points = append(item.Points, [3]float64{point.GetX(), point.GetY(), point.GetZ()})
		}
		result.Edges = append(result.Edges, item)
	}
	for _, vertex := range source.GetTopologyVertices() {
		point := vertex.GetPoint()
		result.TopologyVertices = append(result.TopologyVertices, TopologyPoint{LocalID: vertex.GetLocalId(),
			Point: [3]float64{point.GetX(), point.GetY(), point.GetZ()}})
	}
	return result
}

func (service *Service) loadArtifact(ctx context.Context, key string) (Artifact, error) {
	service.artifactCacheMu.RLock()
	if cached, ok := service.artifactCache[key]; ok {
		service.artifactCacheMu.RUnlock()
		return cached, nil
	}
	service.artifactCacheMu.RUnlock()
	artifact := Artifact{GeometryKey: key}
	var meshJSON, bboxJSON, topologyJSON []byte
	var visualizationJSON []byte
	if err := service.database.QueryRow(ctx, `
		SELECT geometry_id,mesh_json,bbox_json,topology_json,volume,occt_version,
		       COALESCE(octet_length(glb_data),glb_object.size_bytes,0),
		       COALESCE(octet_length(brep_data),brep_object.size_bytes,0),evaluator_version,worker_id,
		       storage_state,artifact.created_at::text,visualization_json
		FROM occccad.geometry_artifacts artifact
		LEFT JOIN occccad.artifact_objects glb_object ON glb_object.id=artifact.glb_object_id
		LEFT JOIN occccad.artifact_objects brep_object ON brep_object.id=artifact.brep_object_id
		WHERE geometry_key=$1`, key).Scan(
		&artifact.GeometryID, &meshJSON, &bboxJSON, &topologyJSON, &artifact.Volume,
		&artifact.OCCTVersion, &artifact.GLBBytes, &artifact.BRepBytes, &artifact.EvaluatorVersion,
		&artifact.WorkerID, &artifact.StorageState, &artifact.CreatedAt, &visualizationJSON); err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(meshJSON, &artifact.Mesh); err != nil {
		return artifact, err
	}
	if artifact.Volume > 0 && (len(artifact.Mesh.Edges) == 0 || len(artifact.Mesh.TopologyVertices) == 0) {
		var response *workerv1.GetTopologyResponse
		var topologyErr error
		if service.artifacts != nil {
			reference, referenceErr := service.brepArtifactReference(ctx, key)
			if referenceErr != nil {
				return artifact, referenceErr
			}
			response, _, topologyErr = service.worker.GetTopologyFromArtifact(ctx, artifact.GeometryID, reference, "", 0)
		} else {
			var brep []byte
			if err := service.database.QueryRow(ctx,
				`SELECT brep_data FROM occccad.geometry_artifacts WHERE geometry_key=$1`, key).Scan(&brep); err != nil {
				return artifact, err
			}
			response, _, topologyErr = service.worker.GetTopology(ctx, artifact.GeometryID, brep, "", 0)
		}
		if topologyErr != nil {
			return artifact, topologyErr
		}
		artifact.Mesh.Edges = make([]MeshEdge, 0, len(response.GetEdges()))
		for _, edge := range response.GetEdges() {
			item := MeshEdge{LocalID: edge.GetLocalId(), Points: make([][3]float64, 0, len(edge.GetRenderPoints()))}
			for _, point := range edge.GetRenderPoints() {
				item.Points = append(item.Points, [3]float64{point.GetX(), point.GetY(), point.GetZ()})
			}
			artifact.Mesh.Edges = append(artifact.Mesh.Edges, item)
		}
		artifact.Mesh.TopologyVertices = make([]TopologyPoint, 0, len(response.GetVertices()))
		for _, vertex := range response.GetVertices() {
			point := vertex.GetPoint()
			artifact.Mesh.TopologyVertices = append(artifact.Mesh.TopologyVertices,
				TopologyPoint{LocalID: vertex.GetLocalId(), Point: [3]float64{point.GetX(), point.GetY(), point.GetZ()}})
		}
		updatedMesh, _ := json.Marshal(artifact.Mesh)
		if _, err := service.database.Exec(ctx,
			`UPDATE occccad.geometry_artifacts SET mesh_json=$2 WHERE geometry_key=$1`, key, updatedMesh); err != nil {
			return artifact, err
		}
	}
	if err := json.Unmarshal(bboxJSON, &artifact.BBox); err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(topologyJSON, &artifact.Topology); err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(visualizationJSON, &artifact.Visualization); err != nil {
		return artifact, err
	}
	service.artifactCacheMu.Lock()
	if service.artifactCache == nil {
		service.artifactCache = map[string]Artifact{}
	}
	if _, exists := service.artifactCache[key]; !exists {
		service.artifactCache[key] = artifact
		service.artifactCacheOrder = append(service.artifactCacheOrder, key)
		if len(service.artifactCacheOrder) > 32 {
			oldest := service.artifactCacheOrder[0]
			service.artifactCacheOrder = service.artifactCacheOrder[1:]
			delete(service.artifactCache, oldest)
		}
	}
	service.artifactCacheMu.Unlock()
	return artifact, nil
}

func protoBBox(source *workerv1.BoundingBox) map[string]any {
	return map[string]any{"min": []float64{source.GetMinX(), source.GetMinY(), source.GetMinZ()},
		"max": []float64{source.GetMaxX(), source.GetMaxY(), source.GetMaxZ()}}
}

func topologyProperties(source []*workerv1.TopologyProperty) map[string]any {
	result := make(map[string]any, len(source))
	for _, property := range source {
		switch value := property.GetValue().(type) {
		case *workerv1.TopologyProperty_NumberValue:
			result[property.GetName()] = value.NumberValue
		case *workerv1.TopologyProperty_IntegerValue:
			result[property.GetName()] = value.IntegerValue
		case *workerv1.TopologyProperty_BoolValue:
			result[property.GetName()] = value.BoolValue
		case *workerv1.TopologyProperty_TextValue:
			result[property.GetName()] = value.TextValue
		case *workerv1.TopologyProperty_VectorValue:
			result[property.GetName()] = [3]float64{value.VectorValue.GetX(), value.VectorValue.GetY(), value.VectorValue.GetZ()}
		}
	}
	return result
}

func (service *Service) GetTopologyElementProperties(
	ctx context.Context, documentID, geometryKey, kind string, localID uint64,
) (TopologyElementProperties, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if (kind != "FACE" && kind != "EDGE" && kind != "VERTEX") || localID == 0 {
		return TopologyElementProperties{}, fmt.Errorf("%w: kind must be FACE, EDGE, or VERTEX and localId must be positive", ErrValidation)
	}
	finishAuthorize := perf.Start(ctx, "topology-authorize")
	var documentType string
	var allowed bool
	err := service.database.QueryRow(ctx, `SELECT d.document_type,
		EXISTS(SELECT 1 FROM occccad.document_versions v WHERE v.id=d.head_version_id AND v.geometry_key=$2)
		FROM occccad.documents d WHERE d.id=$1 AND d.deleted_at IS NULL`, documentID, geometryKey).Scan(&documentType, &allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		finishAuthorize()
		return TopologyElementProperties{}, ErrNotFound
	}
	if err != nil {
		finishAuthorize()
		return TopologyElementProperties{}, err
	}
	if documentType == "PRODUCT" && !allowed {
		// Product occurrence authorization still needs recursive resolution; keep
		// that colder path correct without penalizing normal Part inspection.
		view, viewErr := service.GetDocument(ctx, documentID)
		if viewErr != nil {
			finishAuthorize()
			return TopologyElementProperties{}, viewErr
		}
		_, allowed = view.Artifacts[geometryKey]
	}
	finishAuthorize()
	if !allowed {
		return TopologyElementProperties{}, ErrNotFound
	}
	var geometryID, workerID, occtVersion string
	if err := service.database.QueryRow(ctx, `
		SELECT geometry_id,worker_id,occt_version FROM occccad.geometry_artifacts WHERE geometry_key=$1`, geometryKey).
		Scan(&geometryID, &workerID, &occtVersion); err != nil {
		return TopologyElementProperties{}, err
	}
	var response *workerv1.GetTopologyResponse
	var servingWorkerID string
	finishWorker := perf.Start(ctx, "topology-worker")
	if service.artifacts != nil {
		reference, referenceErr := service.brepArtifactReference(ctx, geometryKey)
		if referenceErr != nil {
			return TopologyElementProperties{}, referenceErr
		}
		response, servingWorkerID, err = service.worker.GetTopologyFromArtifact(ctx, geometryID, reference, kind, localID)
	} else {
		var brep []byte
		if queryErr := service.database.QueryRow(ctx,
			`SELECT brep_data FROM occccad.geometry_artifacts WHERE geometry_key=$1`, geometryKey).Scan(&brep); queryErr != nil {
			return TopologyElementProperties{}, queryErr
		}
		response, servingWorkerID, err = service.worker.GetTopology(ctx, geometryID, brep, kind, localID)
	}
	if err != nil {
		finishWorker()
		return TopologyElementProperties{}, err
	}
	finishWorker()
	result := TopologyElementProperties{GeometryKey: geometryKey, GeometryID: geometryID, Kind: kind,
		LocalID: localID, Properties: map[string]any{}, WorkerID: workerID, OCCTVersion: occtVersion}
	if servingWorkerID != "" {
		result.WorkerID = servingWorkerID
	}
	surfaceTypes := map[int32]string{0: "PLANE", 1: "CYLINDER", 2: "CONE", 3: "SPHERE", 4: "TORUS",
		5: "BSPLINE_SURFACE", 6: "BEZIER_SURFACE", 7: "EXTRUSION_SURFACE", 8: "REVOLUTION_SURFACE",
		9: "OFFSET_SURFACE", -1: "OTHER_SURFACE"}
	curveTypes := map[int32]string{0: "LINE", 1: "CIRCLE", 2: "ELLIPSE", 3: "BSPLINE_CURVE",
		4: "HYPERBOLA", 5: "PARABOLA", 6: "BEZIER_CURVE", 7: "OFFSET_CURVE", -1: "OTHER_CURVE"}
	switch kind {
	case "FACE":
		if len(response.GetFaces()) == 0 {
			return result, ErrNotFound
		}
		item := response.GetFaces()[0]
		result.GeometryType = surfaceTypes[item.GetSurfaceType()]
		if result.GeometryType == "" {
			result.GeometryType = "OTHER_SURFACE"
		}
		result.BBox, result.Properties = protoBBox(item.GetBbox()), topologyProperties(item.GetProperties())
	case "EDGE":
		if len(response.GetEdges()) == 0 {
			return result, ErrNotFound
		}
		item := response.GetEdges()[0]
		result.GeometryType = curveTypes[item.GetCurveType()]
		if result.GeometryType == "" {
			result.GeometryType = "OTHER_CURVE"
		}
		result.BBox, result.Properties = protoBBox(item.GetBbox()), topologyProperties(item.GetProperties())
	case "VERTEX":
		if len(response.GetVertices()) == 0 {
			return result, ErrNotFound
		}
		item := response.GetVertices()[0]
		result.GeometryType = "POINT"
		point := item.GetPoint()
		value := [3]float64{point.GetX(), point.GetY(), point.GetZ()}
		result.Point, result.Properties = &value, topologyProperties(item.GetProperties())
	}
	return result, nil
}

func insertProductInstances(ctx context.Context, tx pgx.Tx, versionID string, model ProductModel) error {
	for _, instance := range model.Instances {
		if _, err := tx.Exec(ctx, `
			INSERT INTO occccad.product_instances(
				product_version_id,instance_key,display_name,referenced_document_id,
				referenced_version_id,translation_x,translation_y,translation_z)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, versionID, instance.ID, instance.Name,
			instance.ReferencedDocumentID, instance.ReferencedVersionID,
			instance.Translation[0], instance.Translation[1], instance.Translation[2]); err != nil {
			return err
		}
	}
	return nil
}

func featureStructureNode(feature Feature, path, documentID, versionID string, deletable, childrenEditable bool) DocumentStructureNode {
	kind := strings.ToUpper(feature.Type)
	switch {
	case strings.Contains(kind, "SKETCH"):
		kind = "SKETCH"
	case kind == "PAD", kind == "LINEAR_EXTRUDE":
		kind = "PAD"
	case kind == "REVOLVE":
		kind = "REVOLVE"
	case kind == "IMPORT_BODY":
		kind = "IMPORT"
	default:
		kind = "FEATURE"
	}
	node := DocumentStructureNode{ID: path + "/" + strings.ToLower(kind) + ":" + feature.ID,
		Kind: kind, Name: feature.Name, EntityID: feature.ID, EntityType: feature.Type,
		DocumentID: documentID, VersionID: versionID}
	if deletable {
		node.Capabilities = []string{"DELETE"}
	}
	if feature.Sketch != nil {
		node.Children = sketchStructureChildren(*feature.Sketch, node.ID, feature.ID, documentID, versionID, childrenEditable)
	}
	return node
}

func sketchStructureChildren(sketch SketchFeature, path, sketchID, documentID, versionID string, editable bool) []DocumentStructureNode {
	geometry := DocumentStructureNode{ID: path + "/geometry", Kind: "SKETCH_GEOMETRY_SET", Name: "Geometry",
		OwnerEntityID: sketchID, DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
	counts := map[string]int{}
	for _, entity := range sketch.Entities {
		counts[entity.Kind]++
		name := strings.Title(strings.ToLower(entity.Kind)) + " " + fmt.Sprint(counts[entity.Kind])
		node := DocumentStructureNode{ID: geometry.ID + "/entity:" + entity.ID, Kind: "SKETCH_ENTITY", Name: name,
			EntityID: entity.ID, OwnerEntityID: sketchID, EntityType: entity.Kind, Role: entity.Role,
			DocumentID: documentID, VersionID: versionID, Suppressed: entity.Suppressed}
		if editable {
			node.Capabilities = []string{"DELETE", "SUPPRESS"}
		}
		geometry.Children = append(geometry.Children, node)
	}
	constraints := DocumentStructureNode{ID: path + "/constraints", Kind: "SKETCH_CONSTRAINT_SET", Name: "Constraints",
		OwnerEntityID: sketchID, DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
	logical := DocumentStructureNode{ID: constraints.ID + "/logical", Kind: "SKETCH_LOGICAL_CONSTRAINT_SET", Name: "Geometric Constraints",
		OwnerEntityID: sketchID, DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
	dimensions := DocumentStructureNode{ID: constraints.ID + "/dimensions", Kind: "SKETCH_DIMENSION_SET", Name: "Dimensions",
		OwnerEntityID: sketchID, DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
	counts = map[string]int{}
	for _, constraint := range sketch.Constraints {
		counts[constraint.Kind]++
		name := strings.Title(strings.ToLower(strings.ReplaceAll(constraint.Kind, "_", " "))) + " " + fmt.Sprint(counts[constraint.Kind])
		parent := &logical
		if isDimensionalConstraint(constraint.Kind) {
			parent = &dimensions
		}
		node := DocumentStructureNode{ID: parent.ID + "/constraint:" + constraint.ID, Kind: "SKETCH_CONSTRAINT", Name: name,
			EntityID: constraint.ID, OwnerEntityID: sketchID, EntityType: constraint.Kind,
			DocumentID: documentID, VersionID: versionID, Suppressed: constraint.Suppressed}
		if slices.Contains(sketch.Solve.ConflictingConstraintIDs, constraint.ID) {
			node.Diagnostic = "CONFLICTING"
		}
		if slices.Contains(sketch.Solve.RedundantConstraintIDs, constraint.ID) {
			node.Diagnostic = "REDUNDANT"
		}
		if editable {
			node.Capabilities = []string{"DELETE", "SUPPRESS"}
		}
		parent.Children = append(parent.Children, node)
	}
	constraints.Children = append(constraints.Children, logical, dimensions)
	return []DocumentStructureNode{geometry, constraints}
}

func partStructureChildren(model PartModel, path, documentID, versionID string, editable bool) []DocumentStructureNode {
	normalizePartModel(&model)
	planes := model.DatumPlanes
	origin := DocumentStructureNode{ID: path + "/origin", Kind: "ORIGIN", Name: "Origin",
		DocumentID: documentID, VersionID: versionID,
		Children: make([]DocumentStructureNode, 0, len(planes)+len(model.AxisSystems)+len(model.DatumAxes))}
	for _, plane := range planes {
		origin.Children = append(origin.Children, DocumentStructureNode{
			ID: path + "/origin/plane:" + plane.ID, Kind: "PLANE", Name: plane.Name,
			EntityID: plane.ID, DocumentID: documentID, VersionID: versionID, Plane: plane.Plane,
		})
	}
	for _, axis := range model.AxisSystems {
		origin.Children = append(origin.Children, DocumentStructureNode{
			ID: path + "/origin/axis:" + axis.ID, Kind: "AXIS_SYSTEM", Name: axis.Name,
			EntityID: axis.ID, DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{
				{ID: path + "/origin/axis:" + axis.ID + "/x", Kind: "AXIS", Name: "X Axis", EntityID: axis.ID, Axis: "X", DocumentID: documentID, VersionID: versionID},
				{ID: path + "/origin/axis:" + axis.ID + "/y", Kind: "AXIS", Name: "Y Axis", EntityID: axis.ID, Axis: "Y", DocumentID: documentID, VersionID: versionID},
				{ID: path + "/origin/axis:" + axis.ID + "/z", Kind: "AXIS", Name: "Z Axis", EntityID: axis.ID, Axis: "Z", DocumentID: documentID, VersionID: versionID},
			},
		})
	}
	for _, axis := range model.DatumAxes {
		origin.Children = append(origin.Children, DocumentStructureNode{ID: path + "/origin/datum-axis:" + axis.ID,
			Kind: "DATUM_AXIS", Name: axis.Name, EntityID: axis.ID, DocumentID: documentID, VersionID: versionID})
	}
	sketches := make(map[string]Feature)
	consumed := make(map[string]bool)
	dependents := make(map[string]bool)
	for _, feature := range model.Features {
		if strings.Contains(strings.ToUpper(feature.Type), "SKETCH") {
			sketches[feature.ID] = feature
		}
		if isSolidGenerator(feature.Type) && feature.Profile != "" {
			consumed[feature.Profile] = true
			dependents[feature.Profile] = true
		}
	}
	body := DocumentStructureNode{ID: path + "/body", Kind: "BODY", Name: "PartBody",
		DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
	for _, feature := range model.Features {
		if consumed[feature.ID] {
			continue
		}
		node := featureStructureNode(feature, body.ID, documentID, versionID, editable && !dependents[feature.ID], editable)
		if isSolidGenerator(feature.Type) && feature.Profile != "" {
			if sketch, exists := sketches[feature.Profile]; exists {
				node.Children = []DocumentStructureNode{featureStructureNode(sketch, node.ID, documentID, versionID, false, editable)}
			}
		}
		body.Children = append(body.Children, node)
	}
	return []DocumentStructureNode{origin, body}
}

func (service *Service) buildDocumentStructure(
	ctx context.Context, versionID, path, displayName string, occurrenceIdentity InstancePath, visiting map[string]bool,
) (DocumentStructureNode, error) {
	var documentID, documentType, storedName string
	var modelJSON []byte
	if err := service.database.QueryRow(ctx, `
		SELECT d.id::text,d.document_type,d.name,v.model_json
		FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id
		WHERE v.id=$1`, versionID).Scan(&documentID, &documentType, &storedName, &modelJSON); err != nil {
		return DocumentStructureNode{}, err
	}
	if displayName == "" {
		displayName = storedName
	}
	root := DocumentStructureNode{ID: path, Kind: documentType, Name: displayName,
		DocumentID: documentID, DocumentType: documentType, VersionID: versionID}
	if len(occurrenceIdentity.Segments) > 0 {
		root.InstancePath = &occurrenceIdentity
	}
	if visiting[documentID] {
		root.Kind = "REFERENCE_CYCLE"
		return root, nil
	}
	if documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return DocumentStructureNode{}, err
		}
		normalizePartModel(&model)
		// Capabilities describe domain operations, not authorization or the
		// browser's current edit context. The client enables them only after
		// loading the referenced document and proving Editor access.
		root.Children = partStructureChildren(model, path, documentID, versionID, true)
		rootPath := root.InstancePath
		applyInstancePath(root.Children, rootPath)
		return root, nil
	}
	visiting[documentID] = true
	defer delete(visiting, documentID)
	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return DocumentStructureNode{}, err
	}
	root.Children = make([]DocumentStructureNode, 0, len(model.Instances))
	for _, instance := range model.Instances {
		resolvedVersionID := instance.ReferencedVersionID
		mode := strings.ToUpper(instance.ReferenceMode)
		if mode == "" || mode == "FOLLOW_HEAD" {
			mode = "FOLLOW_HEAD"
			if err := service.database.QueryRow(ctx,
				`SELECT head_version_id::text FROM occccad.documents WHERE id=$1`,
				instance.ReferencedDocumentID).Scan(&resolvedVersionID); err != nil {
				return DocumentStructureNode{}, err
			}
		}
		instanceNodePath := path + "/instance:" + instance.ID
		childIdentity := appendInstancePath(occurrenceIdentity, InstancePathSegment{
			OwnerDocumentID: documentID, OwnerVersionID: versionID, InstanceID: instance.ID,
			InstanceName: instance.Name, ReferencedDocumentID: instance.ReferencedDocumentID,
			ResolvedVersionID: resolvedVersionID,
		})
		reference, err := service.buildDocumentStructure(ctx, resolvedVersionID, instanceNodePath+"/reference",
			instance.Name, childIdentity, visiting)
		if err != nil {
			return DocumentStructureNode{}, err
		}
		instanceNode := DocumentStructureNode{
			ID: instanceNodePath, Kind: "INSTANCE", Name: instance.Name, EntityID: instance.ID,
			DocumentID: reference.DocumentID, DocumentType: reference.DocumentType,
			VersionID: resolvedVersionID, ReferenceMode: mode, InstancePath: &childIdentity, Children: reference.Children,
		}
		instanceNode.Capabilities = []string{"DELETE"}
		root.Children = append(root.Children, instanceNode)
	}
	if len(model.Constraints) > 0 {
		group := DocumentStructureNode{ID: path + "/assembly-constraints", Kind: "ASSEMBLY_CONSTRAINT_SET", Name: "约束"}
		names := make(map[string]string, len(model.Instances))
		for _, instance := range model.Instances {
			names[instance.ID] = instance.Name
		}
		counts := map[string]int{}
		labels := map[string]string{"FIX": "固定", "RIGID": "固连", "COINCIDENT": "重合", "CONCENTRIC": "同心", "ANGLE": "角度", "DISTANCE": "距离"}
		for _, constraint := range model.Constraints {
			counts[constraint.Kind]++
			name := fmt.Sprintf("#%s.%d（#%s", labels[constraint.Kind], counts[constraint.Kind], names[constraint.First.InstanceID])
			if constraint.Second != nil {
				name += "，#" + names[constraint.Second.InstanceID]
			}
			name += "）"
			group.Children = append(group.Children, DocumentStructureNode{ID: group.ID + "/constraint:" + constraint.ID,
				Kind: "ASSEMBLY_CONSTRAINT", Name: name, EntityID: constraint.ID, EntityType: constraint.Kind,
				DocumentID: documentID, Capabilities: []string{"DELETE"}})
		}
		root.Children = append(root.Children, group)
	}
	return root, nil
}

func (service *Service) resolveProduct(
	ctx context.Context, versionID string, parent InstancePose, path string, instancePath InstancePath, treePath string, visiting map[string]bool,
	artifacts map[string]Artifact, output *[]ResolvedInstance,
) error {
	var documentID, documentType, name string
	var modelJSON []byte
	var geometryKey *string
	if err := service.database.QueryRow(ctx, `
		SELECT d.id::text,d.document_type,d.name,v.model_json,v.geometry_key
		FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id
		WHERE v.id=$1`, versionID).Scan(&documentID, &documentType, &name, &modelJSON, &geometryKey); err != nil {
		return err
	}
	if visiting[documentID] {
		return fmt.Errorf("product reference cycle detected at %s", name)
	}
	if documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return err
		}
		normalizePartModel(&model)
		if geometryKey == nil {
			key, err := service.ensureVisualizationArtifact(ctx, model)
			if err != nil {
				return err
			}
			if _, err := service.database.Exec(ctx,
				`UPDATE occccad.document_versions SET geometry_key=$1 WHERE id=$2 AND geometry_key IS NULL`, key, versionID); err != nil {
				return err
			}
			geometryKey = &key
		}
		if err := service.ensureArtifactVisualization(ctx, *geometryKey, model); err != nil {
			return err
		}
		if _, exists := artifacts[*geometryKey]; !exists {
			artifact, err := service.loadArtifact(ctx, *geometryKey)
			if err != nil {
				return err
			}
			artifacts[*geometryKey] = artifact
		}
		*output = append(*output, ResolvedInstance{
			ID: path, Name: name, DocumentID: documentID, GeometryKey: *geometryKey, Translation: parent.Translation, Rotation: parent.Rotation,
			OccurrencePath: instancePath.Canonical, InstancePath: instancePath, BodyTreeNodeID: treePath + "/body",
		})
		return nil
	}
	visiting[documentID] = true
	defer delete(visiting, documentID)
	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return err
	}
	for _, instance := range model.Instances {
		childPose := composeInstancePose(parent, InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)})
		resolvedVersionID := instance.ReferencedVersionID
		if instance.ReferenceMode == "" || instance.ReferenceMode == "FOLLOW_HEAD" {
			if err := service.database.QueryRow(ctx,
				`SELECT head_version_id::text FROM occccad.documents WHERE id=$1`,
				instance.ReferencedDocumentID).Scan(&resolvedVersionID); err != nil {
				return err
			}
		}
		childPath := appendInstancePath(instancePath, InstancePathSegment{
			OwnerDocumentID: documentID, OwnerVersionID: versionID, InstanceID: instance.ID,
			InstanceName: instance.Name, ReferencedDocumentID: instance.ReferencedDocumentID,
			ResolvedVersionID: resolvedVersionID,
		})
		if err := service.resolveProduct(ctx, resolvedVersionID, childPose,
			path+"/"+instance.ID,
			childPath,
			treePath+"/instance:"+instance.ID+"/reference", visiting, artifacts, output); err != nil {
			return err
		}
	}
	return nil
}

func composeInstancePose(parent, child InstancePose) InstancePose {
	rotate := func(q [4]float64, v [3]float64) [3]float64 {
		u := [3]float64{q[0], q[1], q[2]}
		dot := u[0]*v[0] + u[1]*v[1] + u[2]*v[2]
		cross := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
		uu := u[0]*u[0] + u[1]*u[1] + u[2]*u[2]
		return [3]float64{2*dot*u[0] + (q[3]*q[3]-uu)*v[0] + 2*q[3]*cross[0], 2*dot*u[1] + (q[3]*q[3]-uu)*v[1] + 2*q[3]*cross[1], 2*dot*u[2] + (q[3]*q[3]-uu)*v[2] + 2*q[3]*cross[2]}
	}
	rotated := rotate(normalizedInstanceRotation(parent.Rotation), child.Translation)
	a, b := normalizedInstanceRotation(parent.Rotation), normalizedInstanceRotation(child.Rotation)
	return InstancePose{Translation: [3]float64{parent.Translation[0] + rotated[0], parent.Translation[1] + rotated[1], parent.Translation[2] + rotated[2]}, Rotation: [4]float64{
		a[3]*b[0] + a[0]*b[3] + a[1]*b[2] - a[2]*b[1], a[3]*b[1] - a[0]*b[2] + a[1]*b[3] + a[2]*b[0], a[3]*b[2] + a[0]*b[1] - a[1]*b[0] + a[2]*b[3], a[3]*b[3] - a[0]*b[0] - a[1]*b[1] - a[2]*b[2]}}
}

func positiveFinite(value float64) bool { return value > 0 && finite(value) }
func finite(value float64) bool         { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func traceIDs(ctx context.Context) (any, any) {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return nil, nil
	}
	return spanContext.TraceID().String(), spanContext.SpanID().String()
}
