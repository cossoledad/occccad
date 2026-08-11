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
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"go.opentelemetry.io/otel/trace"
)

const evaluatorVersion = "demo03-feature-chain-v1"

var (
	ErrNotFound   = errors.New("document not found")
	ErrValidation = errors.New("invalid workspace command")
)

type Mesh struct {
	Vertices  [][3]float64 `json:"vertices"`
	Triangles [][3]uint32  `json:"triangles"`
	FaceIDs   []uint32     `json:"faceIds"`
}

type Artifact struct {
	GeometryKey       string            `json:"geometryKey"`
	GeometryID        string            `json:"geometryId"`
	Mesh              Mesh              `json:"mesh"`
	BBox              map[string]any    `json:"bbox"`
	Topology          map[string]any    `json:"topology"`
	Volume            float64           `json:"volume"`
	OCCTVersion       string            `json:"occtVersion"`
	GLBBytes          int               `json:"glbBytes"`
	BRepBytes         int               `json:"brepBytes"`
	EvaluatorVersion  string            `json:"evaluatorVersion"`
	WorkerID          string            `json:"workerId"`
	StorageState      string            `json:"storageState"`
	CreatedAt         string            `json:"createdAt"`
	ReferenceGeometry ReferenceGeometry `json:"referenceGeometry"`
}

type DatumPlane struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Plane  string     `json:"plane"`
	Origin [3]float64 `json:"origin"`
	Normal [3]float64 `json:"normal"`
	Size   float64    `json:"size"`
}

type AxisSystem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Origin     [3]float64 `json:"origin"`
	XDirection [3]float64 `json:"xDirection"`
	YDirection [3]float64 `json:"yDirection"`
	ZDirection [3]float64 `json:"zDirection"`
}

type ReferenceGeometry struct {
	DatumPlanes []DatumPlane `json:"datumPlanes"`
	AxisSystems []AxisSystem `json:"axisSystems"`
}

type Rectangle struct {
	Origin [2]float64 `json:"origin"`
	Width  float64    `json:"width"`
	Height float64    `json:"height"`
}

type Feature struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Name        string     `json:"name"`
	Plane       string     `json:"plane,omitempty"`
	Rectangle   *Rectangle `json:"rectangle,omitempty"`
	Profile     string     `json:"profile,omitempty"`
	Length      float64    `json:"length,omitempty"`
	Operation   string     `json:"operation,omitempty"`
	GeometryKey string     `json:"geometryKey,omitempty"`
	FileName    string     `json:"fileName,omitempty"`

	// Legacy snapshots stored before the nested Rectangle model are still readable.
	Origin *[2]float64 `json:"origin,omitempty"`
	Width  float64     `json:"width,omitempty"`
	Height float64     `json:"height,omitempty"`
}

type PartModel struct {
	Units       string       `json:"units"`
	DatumPlanes []DatumPlane `json:"datumPlanes"`
	AxisSystems []AxisSystem `json:"axisSystems"`
	Features    []Feature    `json:"features"`
}

type ProductInstance struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	ReferencedDocumentID string     `json:"documentId"`
	ReferencedVersionID  string     `json:"versionId"`
	Translation          [3]float64 `json:"translation"`
	ReferenceMode        string     `json:"referenceMode,omitempty"`
	ResolvedVersionID    string     `json:"resolvedVersionId,omitempty"`
	HeadChanged          bool       `json:"headChanged,omitempty"`
}

type ProductModel struct {
	Instances []ProductInstance `json:"instances"`
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
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	DocumentID  string     `json:"documentId"`
	GeometryKey string     `json:"geometryKey"`
	Translation [3]float64 `json:"translation"`
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
	ReferenceMode string                  `json:"referenceMode,omitempty"`
	Children      []DocumentStructureNode `json:"children,omitempty"`
}

type DocumentView struct {
	Document          DocumentSummary        `json:"document"`
	DatumPlanes       []DatumPlane           `json:"datumPlanes,omitempty"`
	AxisSystems       []AxisSystem           `json:"axisSystems,omitempty"`
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

type CommandRequest struct {
	RequestID            string     `json:"requestId"`
	Type                 string     `json:"type"`
	Plane                string     `json:"plane,omitempty"`
	Origin               [2]float64 `json:"origin,omitempty"`
	Width                float64    `json:"width,omitempty"`
	Height               float64    `json:"height,omitempty"`
	SketchID             string     `json:"sketchId,omitempty"`
	Length               float64    `json:"length,omitempty"`
	ReferencedDocumentID string     `json:"referencedDocumentId,omitempty"`
	Name                 string     `json:"name,omitempty"`
	InstanceID           string     `json:"instanceId,omitempty"`
	Translation          [3]float64 `json:"translation,omitempty"`
	ReferenceMode        string     `json:"referenceMode,omitempty"`
	GeometryKey          string     `json:"geometryKey,omitempty"`
	FileName             string     `json:"fileName,omitempty"`
	VersionID            string     `json:"versionId,omitempty"`
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

type Service struct {
	database  *pgxpool.Pool
	worker    *geometry.Client
	artifacts *artifactstore.Service
}

func New(database *pgxpool.Pool, worker *geometry.Client) *Service {
	return &Service{database: database, worker: worker}
}

func NewWithArtifacts(database *pgxpool.Pool, worker *geometry.Client, artifacts *artifactstore.Service) *Service {
	return &Service{database: database, worker: worker, artifacts: artifacts}
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
		       d.history_cursor>0,d.history_cursor<d.history_tip,d.created_at::text,
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
			&item.CanUndo, &item.CanRedo, &item.CreatedAt, &item.LastUpdated,
			&item.DeletedAt, &item.FolderID, &item.LastOpenedAt, &item.CopiedFromID,
			&item.WorkspaceName, &item.Permission); err != nil {
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
		if isUniqueViolation(err) {
			return DocumentView{}, fmt.Errorf("%w: a document with this name and type already exists in the target folder", ErrValidation)
		}
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
	var documentID, commandID, versionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.documents(document_type,name,description,folder_id,copied_from_document_id,owner_user_id)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, documentType, name, description,
		targetFolderID, sourceDocumentID, actorID(request.ActorID)).Scan(&documentID); err != nil {
		if isUniqueViolation(err) {
			return DocumentView{}, fmt.Errorf("%w: a copied document with this name already exists here", ErrValidation)
		}
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
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.document_versions(document_id,sequence,model_json,geometry_key,state,created_by_command_id)
		VALUES($1,1,$2,$3,'READY',$4) RETURNING id::text`,
		documentID, modelJSON, sourceGeometryKey, commandID).Scan(&versionID); err != nil {
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
	return service.GetDocument(ctx, documentID)
}

func (service *Service) changeDocumentMetadata(
	ctx context.Context, documentID, requestIDValue, changeType string,
	payloadValue any, name, description string,
) error {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: a document with this name and type already exists here", ErrValidation)
		}
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

func (service *Service) ImportStep(
	ctx context.Context, documentID, reqID, fileName string, data []byte,
) (DocumentView, error) {
	var documentType string
	if err := service.database.QueryRow(ctx,
		`SELECT document_type FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, documentID).
		Scan(&documentType); errors.Is(err, pgx.ErrNoRows) {
		return DocumentView{}, ErrNotFound
	} else if err != nil {
		return DocumentView{}, err
	}
	if documentType != "PART" {
		return DocumentView{}, fmt.Errorf("%w: STEP can only be imported into a Part", ErrValidation)
	}
	if len(data) == 0 {
		return DocumentView{}, fmt.Errorf("%w: STEP file is empty", ErrValidation)
	}
	digest := sha256.Sum256(append([]byte("demo03-step-import-v1\x00"), data...))
	key := "sha256:" + hex.EncodeToString(digest[:])
	var exists bool
	if err := service.database.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM occccad.geometry_artifacts WHERE geometry_key=$1)`, key).
		Scan(&exists); err != nil {
		return DocumentView{}, err
	}
	if !exists {
		evaluation, err := service.worker.ImportStep(ctx, requestID(reqID), key, fileName, data)
		if err != nil {
			return DocumentView{}, err
		}
		if err := service.storeEvaluation(ctx, key, evaluation, referenceGeometry(newPartModel())); err != nil {
			return DocumentView{}, err
		}
	}
	request := CommandRequest{
		RequestID: reqID, Type: "IMPORT_STEP", GeometryKey: key,
		FileName: strings.TrimSpace(fileName),
	}
	if err := service.applyMutation(ctx, documentID, request); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID)
}

func (service *Service) ExportStep(
	ctx context.Context, documentID, reqID string,
) ([]byte, string, error) {
	var name, documentType string
	var geometryKey *string
	if err := service.database.QueryRow(ctx, `
		SELECT d.name,d.document_type,v.geometry_key
		FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id
		WHERE d.id=$1 AND d.deleted_at IS NULL`, documentID).Scan(&name, &documentType, &geometryKey); errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	} else if err != nil {
		return nil, "", err
	}
	if documentType != "PART" || geometryKey == nil {
		return nil, "", fmt.Errorf("%w: Part has no solid geometry to export", ErrValidation)
	}
	var brep []byte
	var volume float64
	if err := service.database.QueryRow(ctx,
		`SELECT brep_data,volume FROM occccad.geometry_artifacts WHERE geometry_key=$1`, *geometryKey).
		Scan(&brep, &volume); err != nil {
		return nil, "", err
	}
	if volume <= 0 || len(brep) == 0 {
		return nil, "", fmt.Errorf("%w: Part has no solid geometry to export", ErrValidation)
	}
	data, err := service.worker.ExportStep(ctx, requestID(reqID), brep)
	if err != nil {
		return nil, "", err
	}
	return data, name + ".step", nil
}

func (service *Service) CreateDocument(
	ctx context.Context, request CreateDocumentRequest,
) (DocumentView, error) {
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
		key, artifactErr := service.ensureReferenceArtifact(ctx, partModel)
		if artifactErr != nil {
			return DocumentView{}, artifactErr
		}
		initialGeometryKey = &key
	}
	modelJSON, err := json.Marshal(model)
	if err != nil {
		return DocumentView{}, err
	}
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return DocumentView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var documentID, commandID, versionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.documents(document_type,name,description,folder_id,owner_user_id) VALUES($1,$2,$3,$4,$5)
		RETURNING id::text`, documentType, name, description, folderID,
		actorID(request.ActorID)).Scan(&documentID); err != nil {
		if isUniqueViolation(err) {
			return DocumentView{}, fmt.Errorf("%w: a %s document with this name already exists here", ErrValidation, documentType)
		}
		return DocumentView{}, fmt.Errorf("create document: %w", err)
	}
	traceID, spanID := traceIDs(ctx)
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,'CREATE_DOCUMENT',$2,$3,'SUCCEEDED',now(),$4,$5) RETURNING id::text`,
		requestID(request.RequestID), documentID, modelJSON, traceID, spanID).Scan(&commandID); err != nil {
		return DocumentView{}, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.document_versions(document_id,sequence,model_json,geometry_key,state,created_by_command_id)
		VALUES($1,1,$2,$3,'READY',$4) RETURNING id::text`,
		documentID, modelJSON, initialGeometryKey, commandID).Scan(&versionID); err != nil {
		return DocumentView{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`,
		versionID, documentID); err != nil {
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

func (service *Service) GetDocument(ctx context.Context, documentID string) (DocumentView, error) {
	var summary DocumentSummary
	var modelJSON []byte
	var geometryKey *string
	err := service.database.QueryRow(ctx, `
		SELECT d.id::text,d.name,d.description,d.document_type,d.head_version_id::text,
		       d.history_cursor>0,d.history_cursor<d.history_tip,d.created_at::text,
		       d.updated_at::text,d.deleted_at::text,d.folder_id::text,d.last_opened_at::text,
		       d.copied_from_document_id::text,d.workspace_name,
		       v.model_json,v.geometry_key
		FROM occccad.documents d
		JOIN occccad.document_versions v ON v.id=d.head_version_id
		WHERE d.id=$1 AND d.deleted_at IS NULL`, documentID).Scan(
		&summary.ID, &summary.Name, &summary.Description, &summary.Type, &summary.VersionID,
		&summary.CanUndo, &summary.CanRedo, &summary.CreatedAt, &summary.LastUpdated,
		&summary.DeletedAt, &summary.FolderID, &summary.LastOpenedAt, &summary.CopiedFromID,
		&summary.WorkspaceName,
		&modelJSON, &geometryKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentView{}, ErrNotFound
	}
	if err != nil {
		return DocumentView{}, err
	}
	var lastOpened string
	if err := service.database.QueryRow(ctx,
		`UPDATE occccad.documents SET last_opened_at=now() WHERE id=$1 RETURNING last_opened_at::text`,
		documentID).Scan(&lastOpened); err != nil {
		return DocumentView{}, err
	}
	summary.LastOpenedAt = &lastOpened
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
		if geometryKey == nil {
			key, referenceErr := service.ensureReferenceArtifact(ctx, model)
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
			if referenceErr := service.ensureArtifactReferenceGeometry(ctx, *geometryKey, model); referenceErr != nil {
				return view, referenceErr
			}
			artifact, err := service.loadArtifact(ctx, *geometryKey)
			if err != nil {
				return view, err
			}
			view.Artifact = &artifact
		}
		structure, err := service.buildDocumentStructure(ctx, summary.VersionID,
			"document:"+summary.ID, summary.Name, map[string]bool{})
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
	if err := service.resolveProduct(ctx, summary.VersionID, [3]float64{}, summary.Name,
		map[string]bool{}, view.Artifacts, &view.ResolvedInstances); err != nil {
		return view, err
	}
	structure, err := service.buildDocumentStructure(ctx, summary.VersionID,
		"document:"+summary.ID, summary.Name, map[string]bool{})
	if err != nil {
		return view, err
	}
	view.StructureTree = &structure
	return view, nil
}

func datumPlanes() []DatumPlane {
	return []DatumPlane{
		{ID: "datum-xy", Name: "XY Plane", Plane: "XY", Normal: [3]float64{0, 0, 1}, Size: 180},
		{ID: "datum-xz", Name: "XZ Plane", Plane: "XZ", Normal: [3]float64{0, 1, 0}, Size: 180},
		{ID: "datum-yz", Name: "YZ Plane", Plane: "YZ", Normal: [3]float64{1, 0, 0}, Size: 180},
	}
}

func defaultAxisSystems() []AxisSystem {
	return []AxisSystem{{ID: "axis-system-default", Name: "Absolute Axis System",
		XDirection: [3]float64{1, 0, 0}, YDirection: [3]float64{0, 1, 0}, ZDirection: [3]float64{0, 0, 1}}}
}

func newPartModel() PartModel {
	return PartModel{Units: "mm", DatumPlanes: datumPlanes(), AxisSystems: defaultAxisSystems(), Features: []Feature{}}
}

func normalizePartModel(model *PartModel) {
	if model.Units == "" {
		model.Units = "mm"
	}
	if len(model.DatumPlanes) == 0 {
		model.DatumPlanes = datumPlanes()
	}
	if len(model.AxisSystems) == 0 {
		model.AxisSystems = defaultAxisSystems()
	}
	if model.Features == nil {
		model.Features = []Feature{}
	}
}

func referenceGeometry(model PartModel) ReferenceGeometry {
	normalizePartModel(&model)
	return ReferenceGeometry{DatumPlanes: model.DatumPlanes, AxisSystems: model.AxisSystems}
}

func (service *Service) ApplyCommand(
	ctx context.Context, documentID string, request CommandRequest,
) (DocumentView, error) {
	if err := service.requireActiveDocument(ctx, documentID); err != nil {
		return DocumentView{}, err
	}
	request.Type = strings.ToUpper(strings.TrimSpace(request.Type))
	if request.Type == "UNDO" || request.Type == "REDO" {
		if err := service.moveHistory(ctx, documentID, request); err != nil {
			return DocumentView{}, err
		}
		return service.GetDocument(ctx, documentID)
	}
	if request.Type == "RESTORE" {
		if err := service.restoreHistory(ctx, documentID, request); err != nil {
			return DocumentView{}, err
		}
		return service.GetDocument(ctx, documentID)
	}
	if err := service.applyMutation(ctx, documentID, request); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, documentID)
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

func (service *Service) restoreHistory(
	ctx context.Context, documentID string, request CommandRequest,
) error {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var documentType, headVersion string
	var cursor int
	if err := tx.QueryRow(ctx, `
		SELECT document_type,head_version_id::text,history_cursor
		FROM occccad.documents WHERE id=$1 FOR UPDATE`, documentID).
		Scan(&documentType, &headVersion, &cursor); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var modelJSON []byte
	var geometryKey *string
	if err := tx.QueryRow(ctx, `
		SELECT model_json,geometry_key FROM occccad.document_versions
		WHERE id=$1 AND document_id=$2`, request.VersionID, documentID).
		Scan(&modelJSON, &geometryKey); errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: restore point does not belong to this document", ErrValidation)
	} else if err != nil {
		return err
	}
	payload, _ := json.Marshal(request)
	traceID, spanID := traceIDs(ctx)
	var commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,'RESTORE',$2,$3,'SUCCEEDED',now(),$4,$5) RETURNING id::text`,
		requestID(request.RequestID), documentID, payload, traceID, spanID).Scan(&commandID); err != nil {
		return err
	}
	var sequence int
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(max(sequence),0)+1 FROM occccad.document_versions WHERE document_id=$1`,
		documentID).Scan(&sequence); err != nil {
		return err
	}
	var versionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.document_versions(
			document_id,parent_version_id,sequence,model_json,geometry_key,state,created_by_command_id)
		VALUES($1,$2,$3,$4,$5,'READY',$6) RETURNING id::text`,
		documentID, headVersion, sequence, modelJSON, geometryKey, commandID).Scan(&versionID); err != nil {
		return err
	}
	nextPosition := cursor + 1
	if _, err := tx.Exec(ctx,
		`DELETE FROM occccad.document_history WHERE document_id=$1 AND position>$2`, documentID, cursor); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_history(document_id,position,version_id,command_id)
		VALUES($1,$2,$3,$4)`, documentID, nextPosition, versionID, commandID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,$4)`, documentID, versionID, commandID, request.Type); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE occccad.documents SET head_version_id=$1,history_cursor=$2,history_tip=$2,updated_at=now()
		WHERE id=$3`, versionID, nextPosition, documentID); err != nil {
		return err
	}
	if documentType == "PRODUCT" {
		var model ProductModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return err
		}
		if err := insertProductInstances(ctx, tx, versionID, model); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (service *Service) applyMutation(ctx context.Context, documentID string, request CommandRequest) error {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var documentType, headVersion string
	var cursor int
	if err := tx.QueryRow(ctx, `
		SELECT document_type,head_version_id::text,history_cursor
		FROM occccad.documents WHERE id=$1 FOR UPDATE`, documentID).
		Scan(&documentType, &headVersion, &cursor); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var modelJSON []byte
	if err := tx.QueryRow(ctx,
		`SELECT model_json FROM occccad.document_versions WHERE id=$1`, headVersion).
		Scan(&modelJSON); err != nil {
		return err
	}
	var nextModel any
	var geometryKey string
	if documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return err
		}
		normalizePartModel(&model)
		if err := mutatePart(&model, request); err != nil {
			return err
		}
		nextModel = model
		key, err := service.evaluatePart(ctx, requestID(request.RequestID), model)
		if err != nil {
			return err
		}
		geometryKey = key
	} else {
		var model ProductModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return err
		}
		if err := service.mutateProduct(ctx, tx, documentID, &model, request); err != nil {
			return err
		}
		nextModel = model
	}
	nextJSON, err := json.Marshal(nextModel)
	if err != nil {
		return err
	}
	var commandID string
	payloadJSON, err := json.Marshal(request)
	if err != nil {
		return err
	}
	traceID, spanID := traceIDs(ctx)
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,$2,$3,$4,'SUCCEEDED',now(),$5,$6) RETURNING id::text`,
		requestID(request.RequestID), request.Type, documentID, payloadJSON, traceID, spanID).Scan(&commandID); err != nil {
		return err
	}
	var sequence int
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(max(sequence),0)+1 FROM occccad.document_versions WHERE document_id=$1`,
		documentID).Scan(&sequence); err != nil {
		return err
	}
	var versionID string
	var nullableGeometry any
	if geometryKey != "" {
		nullableGeometry = geometryKey
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.document_versions(
			document_id,parent_version_id,sequence,model_json,geometry_key,state,created_by_command_id)
		VALUES($1,$2,$3,$4,$5,'READY',$6) RETURNING id::text`,
		documentID, headVersion, sequence, nextJSON, nullableGeometry, commandID).Scan(&versionID); err != nil {
		return err
	}
	nextPosition := cursor + 1
	if _, err := tx.Exec(ctx,
		`DELETE FROM occccad.document_history WHERE document_id=$1 AND position>$2`,
		documentID, cursor); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_history(document_id,position,version_id,command_id)
		VALUES($1,$2,$3,$4)`, documentID, nextPosition, versionID, commandID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,$4)`, documentID, versionID, commandID, request.Type); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE occccad.documents SET head_version_id=$1,history_cursor=$2,history_tip=$2,updated_at=now()
		WHERE id=$3`, versionID, nextPosition, documentID); err != nil {
		return err
	}
	if documentType == "PRODUCT" {
		if err := insertProductInstances(ctx, tx, versionID, nextModel.(ProductModel)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func mutatePart(model *PartModel, request CommandRequest) error {
	switch request.Type {
	case "CREATE_RECTANGLE_SKETCH":
		plane := strings.ToUpper(request.Plane)
		if plane != "XY" && plane != "XZ" && plane != "YZ" {
			return fmt.Errorf("%w: select XY, XZ, or YZ plane", ErrValidation)
		}
		if !positiveFinite(request.Width) || !positiveFinite(request.Height) ||
			!finite(request.Origin[0]) || !finite(request.Origin[1]) {
			return fmt.Errorf("%w: rectangle dimensions must be positive finite values", ErrValidation)
		}
		model.Features = append(model.Features, Feature{
			ID: newID("sketch"), Type: "RECTANGLE_SKETCH",
			Name:  numberedFeatureName(model.Features, "SKETCH", "Sketch"),
			Plane: plane, Rectangle: &Rectangle{Origin: request.Origin, Width: request.Width, Height: request.Height},
		})
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
	case "IMPORT_STEP":
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
			ID: newID("import"), Type: "IMPORT_STEP", Name: "Import " + name,
			GeometryKey: request.GeometryKey, FileName: name,
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
		instanceName := strings.TrimSpace(request.Name)
		if instanceName == "" {
			instanceName = name
		}
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

func (service *Service) moveHistory(ctx context.Context, documentID string, request CommandRequest) error {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var cursor, tip int
	if err := tx.QueryRow(ctx, `
		SELECT history_cursor,history_tip FROM occccad.documents WHERE id=$1 FOR UPDATE`, documentID).
		Scan(&cursor, &tip); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	target := cursor - 1
	if request.Type == "REDO" {
		target = cursor + 1
	}
	if target < 0 || target > tip {
		return fmt.Errorf("%w: nothing to %s", ErrValidation, strings.ToLower(request.Type))
	}
	var versionID string
	if err := tx.QueryRow(ctx, `
		SELECT version_id::text FROM occccad.document_history
		WHERE document_id=$1 AND position=$2`, documentID, target).Scan(&versionID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"from": cursor, "to": target, "versionId": versionID})
	traceID, spanID := traceIDs(ctx)
	var commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,$2,$3,$4,'SUCCEEDED',now(),$5,$6) RETURNING id::text`,
		requestID(request.RequestID), request.Type, documentID, payload, traceID, spanID).Scan(&commandID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,$4)`, documentID, versionID, commandID, request.Type); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE occccad.documents SET head_version_id=$1,history_cursor=$2,updated_at=now() WHERE id=$3`,
		versionID, target, documentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (service *Service) evaluatePart(ctx context.Context, reqID string, model PartModel) (string, error) {
	normalizePartModel(&model)
	sketches := map[string]Feature{}
	pads := []geometry.RectangularPad{}
	baseKey := ""
	var canonical strings.Builder
	canonical.WriteString(evaluatorVersion)
	referenceJSON, _ := json.Marshal(referenceGeometry(model))
	canonical.WriteString("|reference=" + string(referenceJSON))
	for _, feature := range model.Features {
		switch strings.ToUpper(feature.Type) {
		case "IMPORT_STEP":
			baseKey = feature.GeometryKey
			canonical.WriteString("|base=" + baseKey)
		case "RECTANGLE_SKETCH":
			sketches[feature.ID] = feature
		case "PAD":
			sketch, exists := sketches[feature.Profile]
			if !exists {
				return "", fmt.Errorf("%w: extrude profile %s is missing or follows the extrude", ErrValidation, feature.Profile)
			}
			rectangle := sketch.Rectangle
			if rectangle == nil {
				origin := [2]float64{}
				if sketch.Origin != nil {
					origin = *sketch.Origin
				}
				rectangle = &Rectangle{Origin: origin, Width: sketch.Width, Height: sketch.Height}
			}
			plane := sketch.Plane
			if plane == "" {
				plane = "XY"
			}
			pads = append(pads, geometry.RectangularPad{
				OriginX: rectangle.Origin[0], OriginY: rectangle.Origin[1],
				Width: rectangle.Width, Height: rectangle.Height, Length: feature.Length, Plane: plane,
			})
			fmt.Fprintf(&canonical, "|pad=%s,%.9g,%.9g,%.9g,%.9g,%.9g",
				plane, rectangle.Origin[0], rectangle.Origin[1], rectangle.Width, rectangle.Height, feature.Length)
		}
	}
	if len(pads) == 0 {
		if baseKey != "" {
			return baseKey, nil
		}
		return service.ensureReferenceArtifact(ctx, model)
	}
	var baseBRep []byte
	if baseKey != "" {
		if err := service.database.QueryRow(ctx,
			`SELECT brep_data FROM occccad.geometry_artifacts WHERE geometry_key=$1`, baseKey).
			Scan(&baseBRep); err != nil {
			return "", err
		}
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
	evaluation, err := service.worker.EvaluatePart(ctx, reqID, key, pads, baseBRep)
	if err != nil {
		return "", err
	}
	if err := service.storeEvaluation(ctx, key, evaluation, referenceGeometry(model)); err != nil {
		return "", err
	}
	return key, nil
}

func (service *Service) storeEvaluation(
	ctx context.Context, key string, evaluation *workerv1.EvaluatePartResponse, reference ReferenceGeometry,
) error {
	if evaluation.GetVolume() < 0 {
		return fmt.Errorf("%w: imported/evaluated STEP must contain solid geometry", ErrValidation)
	}
	glb, err := glbWithReferenceGeometry(evaluation.GetGlbData(), reference)
	if err != nil {
		return fmt.Errorf("add reference geometry to GLB: %w", err)
	}
	referenceJSON, _ := json.Marshal(reference)
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
		brepObject, err := service.artifacts.Put(ctx, artifactstore.KindBREP,
			"application/vnd.opencascade.brep", bytes.NewReader(evaluation.GetBrepData()))
		if err != nil {
			return fmt.Errorf("store B-Rep artifact: %w", err)
		}
		glbObject, err := service.artifacts.Put(ctx, artifactstore.KindGLB,
			"model/gltf-binary", bytes.NewReader(glb))
		if err != nil {
			return fmt.Errorf("store GLB artifact: %w", err)
		}
		brepObjectID, glbObjectID, storageState = &brepObject.ID, &glbObject.ID, "DUAL"
	}
	if _, err := service.database.Exec(ctx, `
		INSERT INTO occccad.geometry_artifacts(
			geometry_key,geometry_id,evaluator_version,occt_version,units,
			brep_data,glb_data,mesh_json,bbox_json,topology_json,volume,
			brep_object_id,glb_object_id,storage_state,reference_geometry_json,worker_id)
		VALUES($1,$2,$3,$4,'mm',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (geometry_key) DO NOTHING`, key, evaluation.GetGeometryId(), evaluatorVersion,
		evaluation.GetOcctVersion(), evaluation.GetBrepData(), glb, meshJSON,
		bboxJSON, topologyJSON, evaluation.GetVolume(), brepObjectID, glbObjectID, storageState,
		referenceJSON, workerID); err != nil {
		return err
	}
	return nil
}

func (service *Service) ensureReferenceArtifact(ctx context.Context, model PartModel) (string, error) {
	reference := referenceGeometry(model)
	referenceJSON, _ := json.Marshal(reference)
	digest := sha256.Sum256(append([]byte(evaluatorVersion+"|reference-only|"), referenceJSON...))
	key := "sha256:" + hex.EncodeToString(digest[:])
	var exists bool
	if err := service.database.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM occccad.geometry_artifacts WHERE geometry_key=$1)`, key).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return key, nil
	}
	glb, err := glbWithReferenceGeometry(nil, reference)
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
			return "", fmt.Errorf("store reference GLB: %w", putErr)
		}
		glbObjectID, storageState = &object.ID, "DUAL"
	}
	_, err = service.database.Exec(ctx, `
		INSERT INTO occccad.geometry_artifacts(
			geometry_key,geometry_id,evaluator_version,occt_version,units,brep_data,glb_data,
			mesh_json,bbox_json,topology_json,volume,glb_object_id,storage_state,
			reference_geometry_json,worker_id)
		VALUES($1,$2,$3,'none','mm','',$4,$5,$6,$7,0,$8,$9,$10,'metadata-service')
		ON CONFLICT (geometry_key) DO NOTHING`, key, "reference:"+hex.EncodeToString(digest[:]), evaluatorVersion,
		glb, meshJSON, bboxJSON, topologyJSON, glbObjectID, storageState, referenceJSON)
	return key, err
}

func (service *Service) ensureArtifactReferenceGeometry(ctx context.Context, key string, model PartModel) error {
	var glb, storedReference []byte
	if err := service.database.QueryRow(ctx, `
		SELECT glb_data,reference_geometry_json FROM occccad.geometry_artifacts WHERE geometry_key=$1`, key).
		Scan(&glb, &storedReference); err != nil {
		return err
	}
	var current ReferenceGeometry
	if json.Unmarshal(storedReference, &current) == nil &&
		(len(current.DatumPlanes) > 0 || len(current.AxisSystems) > 0) {
		return nil
	}
	reference := referenceGeometry(model)
	updatedGLB, err := glbWithReferenceGeometry(glb, reference)
	if err != nil {
		return err
	}
	referenceJSON, _ := json.Marshal(reference)
	var objectID *string
	if service.artifacts != nil {
		object, putErr := service.artifacts.Put(ctx, artifactstore.KindGLB,
			"model/gltf-binary", bytes.NewReader(updatedGLB))
		if putErr != nil {
			return fmt.Errorf("backfill reference GLB: %w", putErr)
		}
		objectID = &object.ID
	}
	_, err = service.database.Exec(ctx, `
		UPDATE occccad.geometry_artifacts
		SET glb_data=$2,reference_geometry_json=$3,
		    glb_object_id=COALESCE($4,glb_object_id),
		    storage_state=CASE WHEN $4::uuid IS NULL THEN storage_state ELSE 'DUAL' END
		WHERE geometry_key=$1`, key, updatedGLB, referenceJSON, objectID)
	return err
}

func meshFromProto(source *workerv1.Mesh) Mesh {
	result := Mesh{Vertices: make([][3]float64, 0, len(source.GetVertices())),
		Triangles: make([][3]uint32, 0, len(source.GetTriangles())), FaceIDs: append([]uint32{}, source.GetFaceIds()...)}
	for _, vertex := range source.GetVertices() {
		result.Vertices = append(result.Vertices, [3]float64{vertex.GetX(), vertex.GetY(), vertex.GetZ()})
	}
	for _, triangle := range source.GetTriangles() {
		result.Triangles = append(result.Triangles, [3]uint32{triangle.GetV0(), triangle.GetV1(), triangle.GetV2()})
	}
	return result
}

func (service *Service) loadArtifact(ctx context.Context, key string) (Artifact, error) {
	artifact := Artifact{GeometryKey: key}
	var meshJSON, bboxJSON, topologyJSON []byte
	var referenceJSON []byte
	if err := service.database.QueryRow(ctx, `
		SELECT geometry_id,mesh_json,bbox_json,topology_json,volume,occt_version,
		       octet_length(glb_data),octet_length(brep_data),evaluator_version,worker_id,
		       storage_state,created_at::text,reference_geometry_json
		FROM occccad.geometry_artifacts WHERE geometry_key=$1`, key).Scan(
		&artifact.GeometryID, &meshJSON, &bboxJSON, &topologyJSON, &artifact.Volume,
		&artifact.OCCTVersion, &artifact.GLBBytes, &artifact.BRepBytes, &artifact.EvaluatorVersion,
		&artifact.WorkerID, &artifact.StorageState, &artifact.CreatedAt, &referenceJSON); err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(meshJSON, &artifact.Mesh); err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(bboxJSON, &artifact.BBox); err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(topologyJSON, &artifact.Topology); err != nil {
		return artifact, err
	}
	if err := json.Unmarshal(referenceJSON, &artifact.ReferenceGeometry); err != nil {
		return artifact, err
	}
	return artifact, nil
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

func featureStructureNode(feature Feature, path, documentID, versionID string) DocumentStructureNode {
	kind := strings.ToUpper(feature.Type)
	switch {
	case strings.Contains(kind, "SKETCH"):
		kind = "SKETCH"
	case kind == "PAD":
		kind = "PAD"
	case kind == "IMPORT_STEP":
		kind = "IMPORT"
	default:
		kind = "FEATURE"
	}
	return DocumentStructureNode{ID: path + "/" + strings.ToLower(kind) + ":" + feature.ID,
		Kind: kind, Name: feature.Name, EntityID: feature.ID, DocumentID: documentID, VersionID: versionID}
}

func partStructureChildren(model PartModel, path, documentID, versionID string) []DocumentStructureNode {
	normalizePartModel(&model)
	planes := model.DatumPlanes
	origin := DocumentStructureNode{ID: path + "/origin", Kind: "ORIGIN", Name: "Origin",
		DocumentID: documentID, VersionID: versionID,
		Children: make([]DocumentStructureNode, 0, len(planes)+len(model.AxisSystems))}
	for _, plane := range planes {
		origin.Children = append(origin.Children, DocumentStructureNode{
			ID: path + "/origin/plane:" + plane.ID, Kind: "PLANE", Name: plane.Name,
			EntityID: plane.ID, DocumentID: documentID, VersionID: versionID, Plane: plane.Plane,
		})
	}
	for _, axis := range model.AxisSystems {
		origin.Children = append(origin.Children, DocumentStructureNode{
			ID: path + "/origin/axis:" + axis.ID, Kind: "AXIS_SYSTEM", Name: axis.Name,
			EntityID: axis.ID, DocumentID: documentID, VersionID: versionID,
		})
	}
	sketches := make(map[string]Feature)
	consumed := make(map[string]bool)
	for _, feature := range model.Features {
		if strings.Contains(strings.ToUpper(feature.Type), "SKETCH") {
			sketches[feature.ID] = feature
		}
		if strings.EqualFold(feature.Type, "PAD") && feature.Profile != "" {
			consumed[feature.Profile] = true
		}
	}
	body := DocumentStructureNode{ID: path + "/body", Kind: "BODY", Name: "PartBody",
		DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
	for _, feature := range model.Features {
		if consumed[feature.ID] {
			continue
		}
		node := featureStructureNode(feature, body.ID, documentID, versionID)
		if strings.EqualFold(feature.Type, "PAD") && feature.Profile != "" {
			if sketch, exists := sketches[feature.Profile]; exists {
				node.Children = []DocumentStructureNode{featureStructureNode(sketch, node.ID, documentID, versionID)}
			}
		}
		body.Children = append(body.Children, node)
	}
	return []DocumentStructureNode{origin, body}
}

func (service *Service) buildDocumentStructure(
	ctx context.Context, versionID, path, displayName string, visiting map[string]bool,
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
		root.Children = partStructureChildren(model, path, documentID, versionID)
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
		instancePath := path + "/instance:" + instance.ID
		reference, err := service.buildDocumentStructure(ctx, resolvedVersionID, instancePath+"/reference",
			instance.Name, visiting)
		if err != nil {
			return DocumentStructureNode{}, err
		}
		root.Children = append(root.Children, DocumentStructureNode{
			ID: instancePath, Kind: "INSTANCE", Name: instance.Name, EntityID: instance.ID,
			DocumentID: reference.DocumentID, DocumentType: reference.DocumentType,
			VersionID: resolvedVersionID, ReferenceMode: mode, Children: reference.Children,
		})
	}
	return root, nil
}

func (service *Service) resolveProduct(
	ctx context.Context, versionID string, parent [3]float64, path string, visiting map[string]bool,
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
			key, err := service.ensureReferenceArtifact(ctx, model)
			if err != nil {
				return err
			}
			if _, err := service.database.Exec(ctx,
				`UPDATE occccad.document_versions SET geometry_key=$1 WHERE id=$2 AND geometry_key IS NULL`, key, versionID); err != nil {
				return err
			}
			geometryKey = &key
		}
		if err := service.ensureArtifactReferenceGeometry(ctx, *geometryKey, model); err != nil {
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
			ID: path, Name: name, DocumentID: documentID, GeometryKey: *geometryKey, Translation: parent,
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
		offset := [3]float64{parent[0] + instance.Translation[0], parent[1] + instance.Translation[1], parent[2] + instance.Translation[2]}
		resolvedVersionID := instance.ReferencedVersionID
		if instance.ReferenceMode == "" || instance.ReferenceMode == "FOLLOW_HEAD" {
			if err := service.database.QueryRow(ctx,
				`SELECT head_version_id::text FROM occccad.documents WHERE id=$1`,
				instance.ReferencedDocumentID).Scan(&resolvedVersionID); err != nil {
				return err
			}
		}
		if err := service.resolveProduct(ctx, resolvedVersionID, offset,
			path+"/"+instance.ID, visiting, artifacts, output); err != nil {
			return err
		}
	}
	return nil
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
