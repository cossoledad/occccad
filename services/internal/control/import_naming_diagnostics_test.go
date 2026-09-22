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
// has a frozen naming definition; the legacy fixture below intentionally omits it.
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
	// Exercise actual STEP normalization as well as BREP import.
	exported, err := client.ExportExchange(t.Context(), requestID+"/step-export", "STEP", artifact.StagingKey(requestID, "fixture.step"), []geometry.ExchangeComponent{{Name: "fixture", BRep: ref}})
	if err != nil {
		t.Fatal(err)
	}
	stepObject, err := artifacts.Adopt(t.Context(), artifact.KindExchangeSource, exported.ContentType, exported.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	stepRef := geometry.ArtifactReference{Backend: stepObject.Backend, ObjectKey: stepObject.Key, SHA256: stepObject.SHA256, Size: stepObject.Size, ContentType: stepObject.ContentType}
	stepEvaluation, err := client.ImportExchange(t.Context(), requestID+"/step", key+"-step", "STEP", stepRef, 1, artifact.StagingKey(requestID, "step-shape.brep"), artifact.StagingKey(requestID, "step-mesh.glb"))
	if err != nil {
		t.Fatal(err)
	}
	stepPart, err := service.CommitImportedPart(t.Context(), p6Actor, "", requestID+"/step", "STEP naming", "fixture.step", "STEP", key+"-step", stepEvaluation, &workspace.ImportSource{ObjectID: stepObject.ID, SHA256: stepObject.SHA256, Format: "STEP", ComponentIndex: 1})
	if err != nil || !stepPart.Artifact.Naming.CanBind {
		t.Fatalf("STEP naming: %v", err)
	}
	if _, err = service.BindPersistentSelection(t.Context(), stepPart.Document.ID, workspace.BindPersistentSelectionRequest{SourceVersionID: stepPart.Document.VersionID, GeometryKey: stepPart.Artifact.GeometryKey, Kind: "FACE", LocalID: 1}); err != nil {
		t.Fatal(err)
	}
	imported, err := service.CommitImportedPart(t.Context(), p6Actor, "", requestID, "Import diagnostics", "fixture.brep", "BREP", key, evaluation, &workspace.ImportSource{ObjectID: object.ID, SHA256: object.SHA256, Format: "BREP", ComponentIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.CommitImportedPart(t.Context(), p6Actor, "", requestID, "Import diagnostics", "fixture.brep", "BREP", key, evaluation)
	if err != nil || repeated.Document.VersionID != imported.Document.VersionID || repeated.Part.Features[0].ImportDefinitionID != imported.Part.Features[0].ImportDefinitionID {
		t.Fatalf("import retry changed identity/head: %v", err)
	}
	verifyImportedNaming(t, db, client, artifacts, imported)
	imported, err = service.GetDocument(t.Context(), imported.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Isolated pre-naming fixture: simulate the historical serialized model and raw
	// artifact. Only this newly created test document is changed, never user data.
	if _, err = db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=jsonb_set(model_json,'{features,0}',jsonb_set((model_json->'features'->0)-'importDefinitionId','{id}',to_jsonb($3::text))),geometry_key=$2 WHERE id=$1`, imported.Document.VersionID, key, "legacy-"+imported.Part.Features[0].ID); err != nil {
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
	repaired, err := cold.ApplyCommand(t.Context(), opened.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: requestID + "/repair", Type: "REPAIR_IMPORT_NAMING", TargetID: opened.Part.Features[0].ID})
	if err != nil || !repaired.Artifact.Naming.CanBind {
		t.Fatalf("legacy naming repair: %v", err)
	}
	repeatedRepair, err := cold.ApplyCommand(t.Context(), opened.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: requestID + "/repair", Type: "REPAIR_IMPORT_NAMING", TargetID: opened.Part.Features[0].ID})
	if err != nil || repeatedRepair.Document.VersionID != repaired.Document.VersionID {
		t.Fatalf("repair retry: %v", err)
	}
	undone, err := cold.ApplyCommand(t.Context(), opened.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: requestID + "/undo-repair", Type: "UNDO"})
	if err != nil || undone.Part.Features[0].ImportDefinitionID != "" {
		t.Fatalf("undo repair: %v", err)
	}
	redone, err := cold.ApplyCommand(t.Context(), opened.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: requestID + "/redo-repair", Type: "REDO"})
	if err != nil || redone.Part.Features[0].ImportDefinitionID != repaired.Part.Features[0].ImportDefinitionID || !redone.Artifact.Naming.CanBind {
		t.Fatalf("redo repair: %v", err)
	}
	t.Log(fmt.Sprintf("import diagnostics: %s; cold open, topology inspection, unchanged rejected Head, datum sketch verified", naming.Status))
}
