package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

func TestRepresentationDownloadAuthorizationAndSnapshot(t *testing.T) {
	url := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires isolated test database")
	}
	db, err := database.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	objects := artifact.NewService(db, local)
	domain := workspace.NewWithArtifacts(db, nil, objects)
	actor := "00000000-0000-7000-8000-000000000001"
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PART", Name: fmt.Sprintf("artifact HTTP %d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := objects.Put(t.Context(), artifact.KindGLB, "model/gltf-binary", bytes.NewReader([]byte("unreferenced")))
	if err != nil {
		t.Fatal(err)
	}
	ref := view.Artifacts[view.Part.Bodies[0].GeometryKey].Representations["VISUAL"]
	server := &Server{database: db, workspace: domain, access: access.New(db), artifacts: objects}
	bodyScope := ""
	get := func(user, object, version, etag string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/representation?versionId="+version+"&bodyId="+bodyScope, nil)
		request.SetPathValue("documentID", view.Document.ID)
		request.SetPathValue("objectID", object)
		request = request.WithContext(access.WithPrincipal(request.Context(), access.User{ID: user}))
		request.Header.Set("If-None-Match", etag)
		response := httptest.NewRecorder()
		server.downloadRepresentation(response, request)
		return response
	}
	result := get(actor, ref.ObjectID, view.Document.VersionID, "")
	if result.Code != 200 || result.Body.Len() != int(ref.Size) || result.Header().Get("ETag") != `"`+ref.Digest+`"` {
		t.Fatalf("download status=%d body=%s", result.Code, result.Body.String())
	}
	if cached := get(actor, ref.ObjectID, view.Document.VersionID, result.Header().Get("ETag")); cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatal("ETag not honored")
	}
	if result := get(actor, unrelated.ID, view.Document.VersionID, ""); result.Code != 404 {
		t.Fatalf("unrelated object exposed: %d", result.Code)
	}
	if result := get("00000000-0000-7000-8000-000000000099", ref.ObjectID, view.Document.VersionID, ""); result.Code != 403 {
		t.Fatalf("unauthorized object exposed: %d", result.Code)
	}
	if result := get(actor, ref.ObjectID, "00000000-0000-7000-8000-000000000099", ""); result.Code != 404 {
		t.Fatalf("foreign revision accepted: %d", result.Code)
	}

	view, err = domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: view.Document.ID + "-body", Type: "CREATE_BODY"})
	if err != nil {
		t.Fatal(err)
	}
	authorityIDs := make(map[string]string)
	for _, role := range []string{"BREP", "NAMING"} {
		kind := artifact.KindBREP
		contentType := "application/vnd.opencascade.brep"
		if role == "NAMING" {
			kind = artifact.KindTopologyManifest
			contentType = "application/x-protobuf"
		}
		data := []byte("authorization fixture " + role)
		object, e := objects.Put(t.Context(), kind, contentType, bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		authorityIDs[role] = object.ID
		_, e = db.Exec(t.Context(), `INSERT INTO occccad.geometry_representations(geometry_key,role,schema_version,object_id) VALUES($1,$2,2,$3)`, view.Part.Bodies[1].GeometryKey, role, object.ID)
		if e != nil {
			t.Fatal(e)
		}
		bodyScope = view.Part.Bodies[1].ID
		if response := get(actor, object.ID, view.Document.VersionID, ""); response.Code != 200 || !bytes.Equal(response.Body.Bytes(), data) {
			t.Fatalf("%s download: %d %s", role, response.Code, response.Body.String())
		}
		bodyScope = view.Part.Bodies[0].ID
		if response := get(actor, object.ID, view.Document.VersionID, ""); response.Code != 404 {
			t.Fatalf("cross Body %s download accepted: %d", role, response.Code)
		}
	}
	bodyScope = ""
	partSnapshot := view
	// Follow nested frozen snapshots, including after the root Head changes.
	child := view
	for depth := 0; depth < 2; depth++ {
		product, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PRODUCT", Name: fmt.Sprintf("artifact parent %d", depth)})
		if err != nil {
			t.Fatal(err)
		}
		view, err = domain.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: "insert-" + product.Document.ID, Type: "INSERT_INSTANCE", ReferencedDocumentID: child.Document.ID})
		if err != nil {
			t.Fatal(err)
		}
		if response := get(actor, ref.ObjectID, view.Document.VersionID, ""); response.Code != 200 {
			t.Fatalf("nested depth %d download: %d %s", depth, response.Code, response.Body.String())
		}
		child = view
	}
	historical := view.Document.VersionID
	if _, err := domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: "undo-" + view.Document.ID, Type: "UNDO"}); err != nil {
		t.Fatal(err)
	}
	if response := get(actor, ref.ObjectID, historical, ""); response.Code != 200 {
		t.Fatalf("historical download: %d", response.Code)
	}
	if response := get(actor, ref.ObjectID, "", ""); response.Code != 404 {
		t.Fatalf("removed occurrence still exposes artifact: %d", response.Code)
	}
	// Seed the derived failure display in this test's own Part snapshot. Its
	// successful source representations remain stored, but only Visual is readable.
	view = partSnapshot
	body := view.Part.Bodies[1]
	// Mutate this test's own snapshot with the shared JSON model, so the
	// authorization fixture runs on both supported database dialects.
	model := *view.Part
	model.Bodies = append([]workspace.PartBody(nil), model.Bodies...)
	model.Bodies[1].DisplayFallback = &workspace.BodyDisplayFallback{GeometryKey: body.GeometryKey, SourceVersionID: view.Document.VersionID}
	model.Bodies[1].GeometryKey = ""
	raw, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$2 WHERE id=$1`, view.Document.VersionID, raw); err != nil {
		t.Fatal(err)
	}
	bodyScope = body.ID
	visual := view.Artifacts[body.GeometryKey].Representations["VISUAL"]
	if response := get(actor, visual.ObjectID, view.Document.VersionID, ""); response.Code != 200 {
		t.Fatalf("failure display unavailable: %d %s", response.Code, response.Body.String())
	}
	for role, objectID := range authorityIDs {
		if response := get(actor, objectID, view.Document.VersionID, ""); response.Code != 404 {
			t.Fatalf("fallback exposed %s: %d", role, response.Code)
		}
	}
}
