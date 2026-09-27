package control

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/thumbnail"
	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/protobuf/proto"
)

func verifyGeometryRepresentations(t *testing.T, db *database.Pool, service *workspace.Service, store *artifact.Service, view workspace.DocumentView) {
	t.Helper()
	if len(activeBodyArtifact(t, view).Representations) != 3 {
		t.Fatalf("expected BREP/VISUAL/NAMING: %+v", activeBodyArtifact(t, view).Representations)
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
	ref := activeBodyArtifact(t, view).Representations["VISUAL"]
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
	if uint64(len(mesh.Triangles)) != activeBodyArtifact(t, view).TriangleCount || len(mesh.FaceIDs) != len(mesh.Triangles) || len(mesh.Edges) != 12 || len(mesh.TopologyVertices) != 8 {
		t.Fatalf("display mapping lost: triangles=%d edges=%d vertices=%d", len(mesh.Triangles), len(mesh.Edges), len(mesh.TopologyVertices))
	}
	if mesh.Association.GeometryID != activeBodyArtifact(t, view).GeometryID || mesh.Association.NamingDigest != activeBodyArtifact(t, view).Representations["NAMING"].Digest {
		t.Fatal("Visual/Naming association missing or mismatched")
	}
	_, namingReader, err := store.Open(t.Context(), activeBodyArtifact(t, view).Representations["NAMING"].ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	namingBytes, err := io.ReadAll(namingReader)
	namingReader.Close()
	if err != nil {
		t.Fatal(err)
	}
	naming := &workerv1.PartTopologyManifest{}
	if err = proto.Unmarshal(namingBytes, naming); err != nil {
		t.Fatal(err)
	}
	if naming.SchemaVersion != 2 || naming.GeometryId != activeBodyArtifact(t, view).GeometryID {
		t.Fatal("Naming geometry/version association")
	}
	locators := map[workerv1.PersistentTopologyType]map[uint64]bool{1: {}, 2: {}, 3: {}}
	for _, body := range naming.Bodies {
		for _, o := range body.Tip {
			locators[o.TopologyType][o.LocalId] = true
		}
	}
	for _, id := range mesh.FaceIDs {
		if !locators[1][uint64(id)] {
			t.Fatalf("visual face %d absent from Naming", id)
		}
	}
	for _, edge := range mesh.Edges {
		if !locators[2][edge.LocalID] {
			t.Fatalf("visual edge %d absent from Naming", edge.LocalID)
		}
	}
	for _, point := range mesh.TopologyVertices {
		if !locators[3][point.LocalID] {
			t.Fatalf("visual vertex %d absent from Naming", point.LocalID)
		}
	}
	for _, kind := range []string{"FACE", "EDGE", "VERTEX"} {
		if _, err := service.BindPersistentSelection(t.Context(), view.Document.ID, workspace.BindPersistentSelectionRequest{SourceVersionID: view.Document.VersionID, GeometryKey: activeBodyArtifact(t, view).GeometryKey, Kind: kind, LocalID: 1}); err != nil {
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

func activeBodyArtifact(t *testing.T, view workspace.DocumentView) workspace.Artifact {
	t.Helper()
	if view.Part != nil {
		for _, b := range view.Part.Bodies {
			if b.ID == view.Part.ActiveBodyID {
				if a, ok := view.Artifacts[b.GeometryKey]; ok {
					return a
				}
			}
		}
	}
	t.Fatal("active body artifact missing")
	return workspace.Artifact{}
}
