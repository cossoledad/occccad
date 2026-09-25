package api

import (
	"bytes"
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
	ref := view.Artifact.Representations["VISUAL"]
	server := &Server{database: db, workspace: domain, access: access.New(db), artifacts: objects}
	get := func(user, object, version, etag string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/representation?versionId="+version, nil)
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

}
