package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

func TestStaticInterferenceViewerEntryIsReadOnly(t *testing.T) {
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "static-entry.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	domain := workspace.New(db, nil)
	v, e := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PRODUCT", Name: "Read-only static scope"})
	if e != nil {
		t.Fatal(e)
	}
	viewer := uuid.NewString()
	if _, e = db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name,status,platform_role) VALUES($1,$2,'Static viewer','ACTIVE','MEMBER')`, viewer, viewer+"@example.test"); e != nil {
		t.Fatal(e)
	}
	acl := access.New(db)
	if _, e = acl.UpsertGrant(t.Context(), "DOCUMENT", v.Document.ID, access.DefaultUserID, access.ShareRequest{SubjectType: "USER", SubjectID: viewer, Role: access.RoleViewer}); e != nil {
		t.Fatal(e)
	}
	server := &Server{database: db, workspace: domain, access: acl}
	raw, _ := json.Marshal(workspace.StaticInterferenceRequest{RevisionID: v.Document.VersionID, Scope: "ALL"})
	request := httptest.NewRequest("POST", "/api/documents/"+v.Document.ID+"/static-interference", bytes.NewReader(raw))
	request.SetPathValue("documentID", v.Document.ID)
	request = request.WithContext(access.WithPrincipal(request.Context(), access.User{ID: viewer}))
	response := httptest.NewRecorder()
	server.analyzeStaticInterference(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var out workspace.StaticInterferenceReport
	if e = json.Unmarshal(response.Body.Bytes(), &out); e != nil || out.Complete || len(out.Pairs) != 0 {
		t.Fatal("empty scope cannot pass", out, e)
	}
	var head string
	if e = db.QueryRow(t.Context(), `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, v.Document.ID).Scan(&head); e != nil || head != v.Document.VersionID {
		t.Fatal("Viewer analysis wrote a Revision", e)
	}
	for _, extra := range []string{`,"geometry":[]}`, `,"pose":{"translation":[1,2,3]}}`} {
		forged := httptest.NewRequest("POST", "/static-interference", bytes.NewReader(append(append([]byte(nil), raw[:len(raw)-1]...), []byte(extra)...)))
		forged.SetPathValue("documentID", v.Document.ID)
		forged = forged.WithContext(access.WithPrincipal(forged.Context(), access.User{ID: viewer}))
		w := httptest.NewRecorder()
		server.analyzeStaticInterference(w, forged)
		if w.Code != 400 {
			t.Fatal("client geometry/pose accepted", w.Code, w.Body.String())
		}
	}
	request = request.WithContext(access.WithPrincipal(request.Context(), access.User{ID: uuid.NewString()}))
	response = httptest.NewRecorder()
	server.analyzeStaticInterference(response, request)
	if response.Code != 403 && response.Code != 404 {
		t.Fatal("unshared analysis allowed", response.Code)
	}
}
