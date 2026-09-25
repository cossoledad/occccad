package control

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/thumbnail"
	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func verifyGeometryRepresentations(t *testing.T, db *database.Pool, service *workspace.Service, store *artifact.Service, view workspace.DocumentView) {
	t.Helper()
	if len(view.Artifact.Representations) != 3 {
		t.Fatalf("expected BREP/VISUAL/NAMING: %+v", view.Artifact.Representations)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(raw, &envelope)
	var descriptor map[string]json.RawMessage
	_ = json.Unmarshal(envelope["artifact"], &descriptor)
	if _, ok := descriptor["mesh"]; ok {
		t.Fatal("DocumentView serialized full mesh")
	}
	var forbidden int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns WHERE table_schema='occccad' AND table_name='geometry_artifacts' AND column_name IN ('mesh_json','brep_data','glb_data','topology_manifest_data')`).Scan(&forbidden); err != nil || forbidden != 0 {
		t.Fatalf("large database columns remain: %d %v", forbidden, err)
	}
	ref := view.Artifact.Representations["VISUAL"]
	_, reader, err := store.Open(t.Context(), ref.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	mesh, _, err := visual.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(mesh.Triangles)) != view.Artifact.TriangleCount || len(mesh.FaceIDs) != len(mesh.Triangles) || len(mesh.Edges) != 12 || len(mesh.TopologyVertices) != 8 || len(mesh.StableIDs) != 26 {
		t.Fatalf("display mapping lost: triangles=%d edges=%d vertices=%d anchors=%d", len(mesh.Triangles), len(mesh.Edges), len(mesh.TopologyVertices), len(mesh.StableIDs))
	}
	for _, kind := range []string{"FACE", "EDGE", "VERTEX"} {
		if _, err := service.BindPersistentSelection(t.Context(), view.Document.ID, workspace.BindPersistentSelectionRequest{SourceVersionID: view.Document.VersionID, GeometryKey: view.Artifact.GeometryKey, Kind: kind, LocalID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if path := os.Getenv("OCCCCAD_TEST_GLB_FIXTURE"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.HydrateDisplay(t.Context(), &view); err != nil {
		t.Fatal(err)
	}
	if payload, err := thumbnail.Render(view); err != nil || len(payload) == 0 {
		t.Fatalf("GLB thumbnail: %v", err)
	}
	// Even a locally hydrated renderer working set must never leak through JSON.
	raw, _ = json.Marshal(view)
	_ = json.Unmarshal(raw, &envelope)
	_ = json.Unmarshal(envelope["artifact"], &descriptor)
	if _, ok := descriptor["mesh"]; ok {
		t.Fatal("hydrated display leaked into DocumentView")
	}
}
