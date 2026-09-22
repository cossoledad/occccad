package control

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

// Called by the real Router fixture after producing a native BREP. ImportExchange
// intentionally has no naming yet; the diagnostics task must not invent any.
func verifyImportNamingDiagnostics(t *testing.T, db *pgxpool.Pool, client *geometry.Client, artifacts *artifact.Service, source workspace.DocumentView) {
	t.Helper()
	if source.Artifact.Naming.Status != "READY" || !source.Artifact.Naming.CanBind {
		t.Fatalf("native capability regressed: %+v", source.Artifact.Naming)
	}
	var objectID string
	if err := db.QueryRow(t.Context(), `SELECT brep_object_id::text FROM occccad.geometry_artifacts WHERE geometry_key=$1`, source.Artifact.GeometryKey).Scan(&objectID); err != nil {
		t.Fatal(err)
	}
	object, err := artifacts.Get(t.Context(), objectID)
	if err != nil {
		t.Fatal(err)
	}
	ref := geometry.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, SHA256: object.SHA256, Size: object.Size, ContentType: object.ContentType}
	requestID := "import-diagnostics-" + source.Document.ID
	key := "diagnostics-" + source.Artifact.GeometryKey
	evaluation, err := client.ImportExchange(t.Context(), requestID, key, "BREP", ref, 1, artifact.StagingKey(requestID, "shape.brep"), artifact.StagingKey(requestID, "mesh.glb"))
	if err != nil {
		t.Fatal(err)
	}
	service := workspace.NewWithArtifacts(db, client, artifacts)
	imported, err := service.CommitImportedPart(t.Context(), p6Actor, "", requestID, "Import diagnostics", "fixture.brep", "BREP", key, evaluation)
	if err != nil {
		t.Fatal(err)
	}
	// A fresh service must read the nullable digest from PostgreSQL, not a cached
	// projection retained from the import transaction.
	cold := workspace.NewWithArtifacts(db, client, artifacts)
	opened, err := cold.GetDocument(t.Context(), imported.Document.ID)
	if err != nil || opened.Artifact == nil {
		t.Fatalf("open imported part: %v", err)
	}
	key = opened.Artifact.GeometryKey // ImportBody evaluation may wrap the source with visualization.
	naming := opened.Artifact.Naming
	if naming.Status != "UNAVAILABLE" || naming.CanBind || naming.DiagnosticCode != "TOPOLOGY_NAMING_UNAVAILABLE" {
		t.Fatalf("import capability: %+v", naming)
	}
	properties, err := cold.GetTopologyElementPropertiesAtVersion(t.Context(), opened.Document.ID, opened.Document.VersionID, key, "FACE", 1)
	if err != nil || properties.NamingStatus != "UNAVAILABLE" || properties.PersistentSelection != nil || properties.NamingDiagnostic == nil || len(properties.Properties) == 0 {
		t.Fatalf("non-persistent inspection must work without fake naming: %+v %v", properties, err)
	}
	_, err = cold.BindPersistentSelection(t.Context(), opened.Document.ID, workspace.BindPersistentSelectionRequest{SourceVersionID: opened.Document.VersionID, GeometryKey: key, Kind: "FACE", LocalID: 1})
	if !errors.Is(err, workspace.ErrValidation) || !strings.Contains(err.Error(), "TOPOLOGY_NAMING_UNAVAILABLE") {
		t.Fatalf("bind diagnostic: %v", err)
	}
	_, err = cold.ApplyCommand(t.Context(), opened.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: requestID + "/face", Type: "CREATE_SKETCH", TargetKind: "FACE", VersionID: opened.Document.VersionID, GeometryKey: key, TopologyID: 1})
	if !errors.Is(err, workspace.ErrValidation) || !strings.Contains(err.Error(), "TOPOLOGY_NAMING_UNAVAILABLE") {
		t.Fatalf("face sketch diagnostic: %v", err)
	}
	after, err := cold.GetDocument(t.Context(), opened.Document.ID)
	if err != nil || after.Document.VersionID != opened.Document.VersionID {
		t.Fatalf("failed binding changed Head: %+v %v", after.Document, err)
	}
	datum, err := cold.ApplyCommand(t.Context(), opened.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: requestID + "/datum", Type: "CREATE_SKETCH", Plane: "XY"})
	if err != nil || len(datum.Part.Features) != len(opened.Part.Features)+1 {
		t.Fatalf("datum sketch incorrectly blocked: %v", err)
	}
	t.Log(fmt.Sprintf("import diagnostics: %s; cold open, topology inspection, unchanged rejected Head, datum sketch verified", naming.Status))
}
