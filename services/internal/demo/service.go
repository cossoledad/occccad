package demo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
)

const evaluatorVersion = "demo01-rectangle-pad-v1"

type Mesh struct {
	Vertices  [][3]float64 `json:"vertices"`
	Triangles [][3]uint32  `json:"triangles"`
	FaceIDs   []uint32     `json:"faceIds"`
}

type Artifact struct {
	GeometryKey string         `json:"geometryKey"`
	GeometryID  string         `json:"geometryId"`
	Mesh        Mesh           `json:"mesh"`
	BBox        map[string]any `json:"bbox"`
	Topology    map[string]any `json:"topology"`
	Volume      float64        `json:"volume"`
	OCCTVersion string         `json:"occtVersion"`
	GLBBytes    int            `json:"glbBytes"`
}

type DocumentRef struct {
	ID        string `json:"id"`
	VersionID string `json:"versionId"`
	Name      string `json:"name"`
	Type      string `json:"type"`
}

type SceneInstance struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	GeometryKey string     `json:"geometryKey"`
	Translation [3]float64 `json:"translation"`
}

type TreeNode struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Children []TreeNode `json:"children,omitempty"`
}

type Result struct {
	Part      DocumentRef     `json:"part"`
	Module    DocumentRef     `json:"module"`
	Machine   DocumentRef     `json:"machine"`
	Artifact  Artifact        `json:"artifact"`
	Instances []SceneInstance `json:"instances"`
	Tree      TreeNode        `json:"tree"`
	Metrics   map[string]int  `json:"metrics"`
}

type Service struct {
	database *pgxpool.Pool
	worker   *geometry.Client
}

func New(database *pgxpool.Pool, worker *geometry.Client) *Service {
	return &Service{database: database, worker: worker}
}

func geometryKey(width, height, padLength float64) string {
	canonical := fmt.Sprintf(
		"%s|units=mm|origin=0,0|width=%.9g|height=%.9g|pad=%.9g",
		evaluatorVersion, width, height, padLength)
	digest := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func meshFromProto(source *workerv1.Mesh) Mesh {
	result := Mesh{
		Vertices:  make([][3]float64, 0, len(source.GetVertices())),
		Triangles: make([][3]uint32, 0, len(source.GetTriangles())),
		FaceIDs:   append([]uint32(nil), source.GetFaceIds()...),
	}
	for _, vertex := range source.GetVertices() {
		result.Vertices = append(result.Vertices, [3]float64{vertex.GetX(), vertex.GetY(), vertex.GetZ()})
	}
	for _, triangle := range source.GetTriangles() {
		result.Triangles = append(result.Triangles, [3]uint32{
			triangle.GetV0(), triangle.GetV1(), triangle.GetV2(),
		})
	}
	return result
}

func (service *Service) ensureArtifact(ctx context.Context) (Artifact, error) {
	key := geometryKey(100, 60, 40)
	var artifact Artifact
	artifact.GeometryKey = key
	var meshJSON, bboxJSON, topologyJSON []byte
	err := service.database.QueryRow(ctx, `
		SELECT geometry_id, mesh_json, bbox_json, topology_json, volume, occt_version,
		       octet_length(glb_data)
		FROM occccad.geometry_artifacts WHERE geometry_key=$1`, key).Scan(
		&artifact.GeometryID, &meshJSON, &bboxJSON, &topologyJSON,
		&artifact.Volume, &artifact.OCCTVersion, &artifact.GLBBytes)
	if err == nil {
		if err := json.Unmarshal(meshJSON, &artifact.Mesh); err != nil {
			return artifact, err
		}
		if err := json.Unmarshal(bboxJSON, &artifact.BBox); err != nil {
			return artifact, err
		}
		if err := json.Unmarshal(topologyJSON, &artifact.Topology); err != nil {
			return artifact, err
		}
		return artifact, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return artifact, fmt.Errorf("query geometry artifact: %w", err)
	}

	evaluation, err := service.worker.EvaluateRectangularPad(
		ctx, "demo01-evaluate-box", key, 0, 0, 100, 60, 40, "XY")
	if err != nil {
		return artifact, err
	}
	artifact.GeometryID = evaluation.GetGeometryId()
	artifact.Mesh = meshFromProto(evaluation.GetMesh())
	artifact.Volume = evaluation.GetVolume()
	artifact.OCCTVersion = evaluation.GetOcctVersion()
	artifact.GLBBytes = len(evaluation.GetGlbData())
	artifact.BBox = map[string]any{
		"min": []float64{evaluation.GetBbox().GetMinX(), evaluation.GetBbox().GetMinY(), evaluation.GetBbox().GetMinZ()},
		"max": []float64{evaluation.GetBbox().GetMaxX(), evaluation.GetBbox().GetMaxY(), evaluation.GetBbox().GetMaxZ()},
	}
	artifact.Topology = map[string]any{
		"faces":    evaluation.GetTopology().GetFaceCount(),
		"edges":    evaluation.GetTopology().GetEdgeCount(),
		"vertices": evaluation.GetTopology().GetVertexCount(),
		"solids":   evaluation.GetTopology().GetSolidCount(),
	}
	meshJSON, _ = json.Marshal(artifact.Mesh)
	bboxJSON, _ = json.Marshal(artifact.BBox)
	topologyJSON, _ = json.Marshal(artifact.Topology)
	_, err = service.database.Exec(ctx, `
		INSERT INTO occccad.geometry_artifacts(
			geometry_key, geometry_id, evaluator_version, occt_version, units,
			brep_data, glb_data, mesh_json, bbox_json, topology_json, volume)
		VALUES($1,$2,$3,$4,'mm',$5,$6,$7,$8,$9,$10)
		ON CONFLICT (geometry_key) DO NOTHING`,
		key, artifact.GeometryID, evaluatorVersion, artifact.OCCTVersion,
		evaluation.GetBrepData(), evaluation.GetGlbData(), meshJSON, bboxJSON, topologyJSON, artifact.Volume)
	if err != nil {
		return artifact, fmt.Errorf("store geometry artifact: %w", err)
	}
	return artifact, nil
}

func (service *Service) ensurePart(ctx context.Context, artifact Artifact) (DocumentRef, error) {
	if existing, err := service.findDocument(ctx, "PART", "Demo Box"); err == nil {
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return DocumentRef{}, err
	}

	model := map[string]any{
		"units": "mm",
		"features": []any{
			map[string]any{"id": "sketch-1", "type": "rectangle_sketch", "origin": []float64{0, 0}, "width": 100, "height": 60},
			map[string]any{"id": "pad-1", "type": "pad", "profile": "sketch-1", "length": 40},
		},
	}
	return service.insertDocument(ctx, "PART", "Demo Box", "demo01-create-part",
		"CREATE_RECTANGULAR_PAD", model, artifact.GeometryKey, nil)
}

type productInstance struct {
	Key         string
	Name        string
	Reference   DocumentRef
	Translation [3]float64
}

func (service *Service) ensureProduct(
	ctx context.Context,
	name string,
	requestID string,
	instances []productInstance,
) (DocumentRef, error) {
	if existing, err := service.findDocument(ctx, "PRODUCT", name); err == nil {
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return DocumentRef{}, err
	}
	modelInstances := make([]map[string]any, 0, len(instances))
	for _, instance := range instances {
		modelInstances = append(modelInstances, map[string]any{
			"id": instance.Key, "name": instance.Name,
			"documentId": instance.Reference.ID, "versionId": instance.Reference.VersionID,
			"translation": instance.Translation,
		})
	}
	model := map[string]any{"instances": modelInstances}
	return service.insertDocument(ctx, "PRODUCT", name, requestID,
		"CREATE_PRODUCT", model, "", instances)
}

func (service *Service) insertDocument(
	ctx context.Context,
	documentType, name, requestID, commandType string,
	model any,
	geometryKey string,
	instances []productInstance,
) (DocumentRef, error) {
	modelJSON, _ := json.Marshal(model)
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return DocumentRef{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result DocumentRef
	result.Name, result.Type = name, documentType
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.documents(document_type,name) VALUES($1,$2)
		RETURNING id::text`, documentType, name).Scan(&result.ID); err != nil {
		return result, fmt.Errorf("insert document: %w", err)
	}
	var commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at)
		VALUES($1,$2,$3,$4,'SUCCEEDED',now()) RETURNING id::text`,
		requestID, commandType, result.ID, modelJSON).Scan(&commandID); err != nil {
		return result, fmt.Errorf("insert command: %w", err)
	}
	var nullableGeometry any
	if geometryKey != "" {
		nullableGeometry = geometryKey
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO occccad.document_versions(
			document_id,sequence,model_json,geometry_key,state,created_by_command_id)
		VALUES($1,1,$2,$3,'READY',$4) RETURNING id::text`,
		result.ID, modelJSON, nullableGeometry, commandID).Scan(&result.VersionID); err != nil {
		return result, fmt.Errorf("insert document version: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`,
		result.VersionID, result.ID); err != nil {
		return result, err
	}
	for _, instance := range instances {
		var createsCycle bool
		if err := tx.QueryRow(ctx, `
			WITH RECURSIVE reference_graph(document_id) AS (
				SELECT $1::uuid
				UNION
				SELECT pi.referenced_document_id
				FROM reference_graph r
				JOIN occccad.documents d ON d.id=r.document_id
				JOIN occccad.document_versions dv ON dv.id=d.head_version_id
				JOIN occccad.product_instances pi ON pi.product_version_id=dv.id
			)
			SELECT EXISTS(SELECT 1 FROM reference_graph WHERE document_id=$2::uuid)`,
			instance.Reference.ID, result.ID).Scan(&createsCycle); err != nil {
			return result, fmt.Errorf("check product reference cycle: %w", err)
		}
		if createsCycle {
			return result, fmt.Errorf("product reference %s creates a cycle", instance.Name)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO occccad.product_instances(
				product_version_id,instance_key,display_name,
				referenced_document_id,referenced_version_id,
				translation_x,translation_y,translation_z)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			result.VersionID, instance.Key, instance.Name,
			instance.Reference.ID, instance.Reference.VersionID,
			instance.Translation[0], instance.Translation[1], instance.Translation[2])
		if err != nil {
			return result, fmt.Errorf("insert product instance: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func (service *Service) findDocument(ctx context.Context, documentType, name string) (DocumentRef, error) {
	var result DocumentRef
	err := service.database.QueryRow(ctx, `
		SELECT d.id::text,d.head_version_id::text,d.name,d.document_type
		FROM occccad.documents d WHERE d.document_type=$1 AND d.name=$2`,
		documentType, name).Scan(&result.ID, &result.VersionID, &result.Name, &result.Type)
	return result, err
}

func (service *Service) Seed(ctx context.Context) (Result, error) {
	artifact, err := service.ensureArtifact(ctx)
	if err != nil {
		return Result{}, err
	}
	part, err := service.ensurePart(ctx, artifact)
	if err != nil {
		return Result{}, err
	}
	module, err := service.ensureProduct(ctx, "Demo Module", "demo01-create-module", []productInstance{
		{Key: "box-1", Name: "Box-1", Reference: part, Translation: [3]float64{0, 0, 0}},
		{Key: "box-2", Name: "Box-2", Reference: part, Translation: [3]float64{140, 0, 0}},
	})
	if err != nil {
		return Result{}, err
	}
	machine, err := service.ensureProduct(ctx, "Demo Machine", "demo01-create-machine", []productInstance{
		{Key: "module-1", Name: "Module-1", Reference: module, Translation: [3]float64{0, 0, 0}},
		{Key: "module-2", Name: "Module-2", Reference: module, Translation: [3]float64{0, 120, 0}},
	})
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Part: part, Module: module, Machine: machine, Artifact: artifact,
		Instances: []SceneInstance{
			{ID: "module-1/box-1", Name: "Box-1", GeometryKey: artifact.GeometryKey, Translation: [3]float64{0, 0, 0}},
			{ID: "module-1/box-2", Name: "Box-2", GeometryKey: artifact.GeometryKey, Translation: [3]float64{140, 0, 0}},
			{ID: "module-2/box-1", Name: "Box-1", GeometryKey: artifact.GeometryKey, Translation: [3]float64{0, 120, 0}},
			{ID: "module-2/box-2", Name: "Box-2", GeometryKey: artifact.GeometryKey, Translation: [3]float64{140, 120, 0}},
		},
		Tree: TreeNode{Name: "Demo Machine", Type: "PRODUCT", Children: []TreeNode{
			{Name: "Module-1", Type: "PRODUCT_INSTANCE", Children: []TreeNode{{Name: "Box-1", Type: "PART_INSTANCE"}, {Name: "Box-2", Type: "PART_INSTANCE"}}},
			{Name: "Module-2", Type: "PRODUCT_INSTANCE", Children: []TreeNode{{Name: "Box-1", Type: "PART_INSTANCE"}, {Name: "Box-2", Type: "PART_INSTANCE"}}},
		}},
		Metrics: map[string]int{},
	}
	for table, key := range map[string]string{
		"documents": "documents", "document_versions": "versions",
		"commands": "commands", "geometry_artifacts": "artifacts",
	} {
		var count int
		if err := service.database.QueryRow(ctx,
			"SELECT count(*) FROM occccad."+table).Scan(&count); err != nil {
			return Result{}, err
		}
		result.Metrics[key] = count
	}
	result.Metrics["visibleInstances"] = len(result.Instances)
	result.Metrics["uniqueGeometry"] = 1
	return result, nil
}
