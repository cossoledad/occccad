package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/visual"
)

func (s *Service) representationObject(ctx context.Context, key, role string) (artifactstore.Object, error) {
	var id string
	err := s.database.QueryRow(ctx, `SELECT object_id::text FROM occccad.geometry_representations WHERE geometry_key=$1 AND role=$2`, key, role).Scan(&id)
	if err != nil {
		return artifactstore.Object{}, err
	}
	if s.artifacts == nil {
		return artifactstore.Object{}, fmt.Errorf("ArtifactStore required")
	}
	return s.artifacts.Get(ctx, id)
}
func (s *Service) readRepresentation(ctx context.Context, key, role string) ([]byte, error) {
	o, err := s.representationObject(ctx, key, role)
	if err != nil {
		return nil, err
	}
	_, r, err := s.artifacts.Open(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != o.SHA256 {
		return nil, fmt.Errorf("%s artifact digest mismatch", role)
	}
	return data, nil
}
func (s *Service) adoptEvaluationObject(ctx context.Context, ref *workerv1.ArtifactReference, kind artifactstore.Kind) (artifactstore.Object, error) {
	if s.artifacts == nil || ref == nil || ref.ObjectKey == "" {
		return artifactstore.Object{}, fmt.Errorf("persistent result requires %s ArtifactReference", kind)
	}
	o, err := s.artifacts.Adopt(ctx, kind, ref.ContentType, ref.ObjectKey)
	if err == nil && (o.SHA256 != ref.Sha256 || uint64(o.Size) != ref.SizeBytes) {
		err = fmt.Errorf("%s output integrity mismatch", kind)
	}
	return o, err
}
func (s *Service) persistGeometry(ctx context.Context, key string, a Artifact, objects map[string]artifactstore.Object) error {
	bbox, _ := json.Marshal(a.BBox)
	topology, _ := json.Marshal(a.Topology)
	// Reference geometry is business metadata; large display primitives live only in GLB.
	metadata := a.Visualization
	metadata.Primitives = nil
	visual, _ := json.Marshal(metadata)
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO occccad.geometry_artifacts(geometry_key,geometry_id,evaluator_version,occt_version,units,bbox_json,topology_json,volume,triangle_count,display_vertex_count,reference_geometry_json,worker_id) VALUES($1,$2,$3,$4,'mm',$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(geometry_key) DO NOTHING`, key, a.GeometryID, evaluatorVersion, a.OCCTVersion, bbox, topology, a.Volume, a.TriangleCount, a.DisplayVertexCount, visual, a.WorkerID)
	if err != nil {
		return err
	}
	for role, o := range objects {
		_, err = tx.Exec(ctx, `INSERT INTO occccad.geometry_representations(geometry_key,role,schema_version,object_id) VALUES($1,$2,1,$3) ON CONFLICT(geometry_key,role) DO NOTHING`, key, role, o.ID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Service) storeEvaluation(ctx context.Context, key string, e *workerv1.EvaluatePartResponse, v VisualizationManifest) error {
	if e.GetRepresentationKind() != "PERSISTENT" || e.GetTopology().GetSolidCount() == 0 || e.GetVolume() <= 0 {
		return fmt.Errorf("%w: persistent solid result required", ErrValidation)
	}
	brep, err := s.adoptEvaluationObject(ctx, e.BrepArtifact, artifactstore.KindBREP)
	if err != nil {
		return err
	}
	ref := e.GetGlbArtifact()
	if ref == nil || ref.GetSizeBytes() > uint64(^uint64(0)>>1) {
		return fmt.Errorf("visual ArtifactReference required")
	}
	visual, err := s.artifacts.AdoptTransformed(ctx, artifactstore.KindGLB, "model/gltf-binary", ref.ObjectKey, ref.Sha256, int64(ref.SizeBytes), func(data []byte) ([]byte, error) { return glbWithVisualization(data, v) })
	if err != nil {
		return err
	}
	objects := map[string]artifactstore.Object{"BREP": brep, "VISUAL": visual}
	if m := e.EvaluationManifest; m != nil {
		if m.SchemaVersion != modelcore.TopologyNamingSchemaVersion || m.TopologyPolicyId != modelcore.TopologyNamingPolicyID || m.TopologyEvaluatorVersion != modelcore.TopologyNamingEvaluator || m.TopologyPolicyDigest != modelcore.TopologyNamingPolicyDigest {
			return fmt.Errorf("topology manifest contract mismatch")
		}
		naming, err := s.adoptEvaluationObject(ctx, m.TopologyManifestArtifact, artifactstore.KindTopologyManifest)
		if err != nil {
			return err
		}
		if err := validateTopologyManifestDigests(m.TopologyManifestDigest, m.GetTopologyManifestArtifact().GetSha256(), naming.SHA256); err != nil {
			return err
		}
		objects["NAMING"] = naming
	}
	worker := s.worker.WorkerFor(key)
	return s.persistGeometry(ctx, key, Artifact{GeometryID: e.GeometryId, OCCTVersion: e.OcctVersion, WorkerID: worker, Volume: e.Volume, BBox: protoBBox(e.Bbox), Topology: map[string]any{"faces": e.Topology.FaceCount, "edges": e.Topology.EdgeCount, "vertices": e.Topology.VertexCount, "solids": e.Topology.SolidCount}, TriangleCount: e.TriangleCount, DisplayVertexCount: e.DisplayVertexCount, Visualization: v}, objects)
}
func (s *Service) ensureVisualizationArtifact(ctx context.Context, model PartModel) (string, error) {
	v := visualizationManifest(model)
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(append([]byte(evaluatorVersion), raw...))
	key := "sha256:" + hex.EncodeToString(sum[:])
	glb, err := glbWithVisualization(nil, v)
	if err != nil {
		return "", err
	}
	if s.artifacts == nil {
		return "", fmt.Errorf("ArtifactStore required")
	}
	object, err := s.artifacts.Put(ctx, artifactstore.KindGLB, "model/gltf-binary", bytes.NewReader(glb))
	if err != nil {
		return "", err
	}
	err = s.persistGeometry(ctx, key, Artifact{GeometryID: key, OCCTVersion: "none", WorkerID: "metadata-service", BBox: map[string]any{"min": []float64{-90, -90, -90}, "max": []float64{90, 90, 90}}, Topology: map[string]any{"faces": 0, "edges": 0, "vertices": 0, "solids": 0}, Visualization: v}, map[string]artifactstore.Object{"VISUAL": object})
	return key, err
}
func (s *Service) ensureVisualizationVariant(ctx context.Context, base string, v VisualizationManifest) (string, error) {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(append([]byte(evaluatorVersion+"|base="+base), raw...))
	key := "sha256:" + hex.EncodeToString(sum[:])
	glb, err := s.readRepresentation(ctx, base, "VISUAL")
	if err != nil {
		return "", err
	}
	glb, err = glbWithVisualization(glb, v)
	if err != nil {
		return "", err
	}
	a, err := s.loadArtifact(ctx, base)
	if err != nil {
		return "", err
	}
	objects := map[string]artifactstore.Object{}
	for role := range a.Representations {
		o, err := s.representationObject(ctx, base, role)
		if err != nil {
			return "", err
		}
		objects[role] = o
	}
	objects["VISUAL"], err = s.artifacts.Put(ctx, artifactstore.KindGLB, "model/gltf-binary", bytes.NewReader(glb))
	if err != nil {
		return "", err
	}
	a.Visualization = v
	return key, s.persistGeometry(ctx, key, a, objects)
}
func (s *Service) loadArtifact(ctx context.Context, key string) (Artifact, error) {
	s.artifactCacheMu.RLock()
	cached, ok := s.artifactCache[key]
	s.artifactCacheMu.RUnlock()
	if ok {
		return cached, nil
	}
	a := Artifact{GeometryKey: key, RepresentationKind: "PERSISTENT", StorageState: "OBJECT", Representations: map[string]Representation{}}
	var bbox, topology, visual []byte
	err := s.database.QueryRow(ctx, `SELECT geometry_id,bbox_json,topology_json,volume,occt_version,evaluator_version,worker_id,created_at::text,reference_geometry_json,triangle_count,display_vertex_count FROM occccad.geometry_artifacts WHERE geometry_key=$1`, key).Scan(&a.GeometryID, &bbox, &topology, &a.Volume, &a.OCCTVersion, &a.EvaluatorVersion, &a.WorkerID, &a.CreatedAt, &visual, &a.TriangleCount, &a.DisplayVertexCount)
	if err != nil {
		return a, err
	}
	for _, item := range []struct {
		data   []byte
		target any
	}{{bbox, &a.BBox}, {topology, &a.Topology}, {visual, &a.Visualization}} {
		if err = json.Unmarshal(item.data, item.target); err != nil {
			return a, err
		}
	}
	rows, err := s.database.Query(ctx, `SELECT r.role,r.schema_version,o.id::text,o.sha256,o.size_bytes,o.content_type FROM occccad.geometry_representations r JOIN occccad.artifact_objects o ON o.id=r.object_id AND o.state='READY' WHERE r.geometry_key=$1`, key)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var r Representation
		if err = rows.Scan(&role, &r.SchemaVersion, &r.ObjectID, &r.Digest, &r.Size, &r.ContentType); err != nil {
			return a, err
		}
		a.Representations[role] = r
	}
	a.GLBBytes = int(a.Representations["VISUAL"].Size)
	a.BRepBytes = int(a.Representations["BREP"].Size)
	a.Naming = NamingAvailability{Status: "UNAVAILABLE", CanBind: false, DiagnosticCode: "TOPOLOGY_NAMING_UNAVAILABLE", Diagnostic: "当前几何没有拓扑命名；需要先建立导入命名。"}
	if _, ok := a.Representations["NAMING"]; ok {
		a.Naming = NamingAvailability{Status: "READY", CanBind: true}
	}
	if err := rows.Err(); err != nil {
		return a, err
	}
	s.artifactCacheMu.Lock()
	if s.artifactCache == nil {
		s.artifactCache = map[string]Artifact{}
	}
	if _, exists := s.artifactCache[key]; !exists {
		s.artifactCache[key] = a
		s.artifactCacheOrder = append(s.artifactCacheOrder, key)
	}
	if len(s.artifactCacheOrder) > 1024 {
		delete(s.artifactCache, s.artifactCacheOrder[0])
		s.artifactCacheOrder = s.artifactCacheOrder[1:]
	}
	s.artifactCacheMu.Unlock()
	return a, nil
}

// Naming stays lazy: opening a document never downloads the full topology graph.
func (s *Service) namingReference(ctx context.Context, key string) (*string, *string, error) {
	var id, digest string
	err := s.database.QueryRow(ctx, `SELECT o.id::text,o.sha256 FROM occccad.geometry_representations r JOIN occccad.artifact_objects o ON o.id=r.object_id WHERE r.geometry_key=$1 AND r.role='NAMING'`, key).Scan(&id, &digest)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &id, &digest, nil
}

// HydrateDisplay is explicitly consumer-local (thumbnail/preview), never called
// by GetDocument. All solid display geometry is decoded from the same GLB.
func (s *Service) HydrateDisplay(ctx context.Context, view *DocumentView) error {
	hydrate := func(a *Artifact) error {
		data, err := s.readRepresentation(ctx, a.GeometryKey, "VISUAL")
		if err != nil {
			return err
		}
		mesh, metadata, err := visual.Decode(data)
		if err != nil {
			return err
		}
		a.Mesh = mesh
		if len(metadata) > 0 {
			if err = json.Unmarshal(metadata, &a.Visualization); err != nil {
				return err
			}
		}
		return nil
	}
	if view.Artifact != nil {
		copy := *view.Artifact
		view.Artifact = &copy
		if err := hydrate(view.Artifact); err != nil {
			return err
		}
	}
	hydrated := make(map[string]Artifact, len(view.Artifacts))
	for key, a := range view.Artifacts {
		if err := hydrate(&a); err != nil {
			return err
		}
		hydrated[key] = a
	}
	view.Artifacts = hydrated
	return nil
}
