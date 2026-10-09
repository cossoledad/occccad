package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssemblyDiagnosticPermissionsAndReadOnlySnapshot(t *testing.T) {
	db := testsupport.OpenTestDatabase(t)
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, nil, artifact.NewService(db, local))
	part, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PART", Name: "Source"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PRODUCT", Name: "Assembly"})
	if err != nil {
		t.Fatal(err)
	}
	product, err = domain.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: access.DefaultUserID, Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{database: db, workspace: domain, access: access.New(db)}
	get := func(user, doc string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/assembly-diagnostic", nil)
		request.SetPathValue("documentID", doc)
		request = request.WithContext(access.WithPrincipal(request.Context(), access.User{ID: user}))
		response := httptest.NewRecorder()
		server.downloadAssemblyDiagnostic(response, request)
		return response
	}
	response := get(access.DefaultUserID, product.Document.ID)
	if response.Code != 200 || !strings.Contains(response.Body.String(), product.Document.VersionID) || !strings.Contains(response.Body.String(), part.Document.VersionID) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response.Code, response.Body.String())
	}
	if response = get("00000000-0000-7000-8000-000000000099", product.Document.ID); response.Code != 403 {
		t.Fatal("unauthorized assembly export", response.Code)
	}
	if response = get(access.DefaultUserID, part.Document.ID); response.Code != 400 {
		t.Fatal("Part accepted by assembly endpoint", response.Code)
	}
	reopened, err := domain.GetDocument(t.Context(), product.Document.ID)
	if err != nil || reopened.Document.VersionID != product.Document.VersionID {
		t.Fatal("diagnostic export changed head", err)
	}
}
