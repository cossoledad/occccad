package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

func TestDocumentListMetadataPreservesSummaryAndDefaultHistory(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "document-list.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	domain := workspace.New(db, nil)
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PRODUCT", Name: "Metadata listing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(t.Context(), `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status)
		SELECT $1,id,2,$2,$1,$1,'DOMAIN','COMMITTED' FROM occccad.workspaces WHERE document_id=$3 AND name='main'`, uuid.NewString(), access.DefaultUserID, view.Document.ID); err != nil {
		t.Fatal(err)
	}
	server := &Server{database: db, workspace: domain}
	list := func(query string) workspace.DocumentPage {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/documents?q=Metadata+listing"+query, nil)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: access.DefaultUserID}))
		w := httptest.NewRecorder()
		server.listDocuments(w, r)
		if w.Code != 200 {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		var result workspace.DocumentPage
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Documents) != 1 {
			t.Fatalf("unexpected page: %+v", result)
		}
		return result
	}
	full := list("")
	if !full.Documents[0].CanUndo {
		t.Fatal("default list must retain real history capabilities")
	}
	metadata := list("&includeHistory=false")
	if metadata.Documents[0].CanUndo || metadata.Documents[0].CanRedo {
		t.Fatal("metadata requested a history fold")
	}
	full.Documents[0].CanUndo, full.Documents[0].CanRedo = false, false
	if !reflect.DeepEqual(full, metadata) {
		t.Fatal("metadata listing changed identities, permissions, versions or pagination")
	}
	if !list("&includeHistory=true").Documents[0].CanUndo {
		t.Fatal("metadata request changed subsequent history reads")
	}

	reader := uuid.NewString()
	if _, err = db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name,status,platform_role) VALUES($1,$2,'Library reader','ACTIVE','MEMBER')`, reader, reader+"@example.test"); err != nil {
		t.Fatal(err)
	}
	folder, err := domain.CreateFolder(t.Context(), workspace.CreateFolderRequest{ActorID: access.DefaultUserID, Name: "ACL parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := domain.CreateFolder(t.Context(), workspace.CreateFolderRequest{ActorID: access.DefaultUserID, Name: "ACL child", ParentID: &folder.ID})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{}
	for _, item := range []struct {
		name, owner, permission string
		folder                  *string
	}{
		{"ACL listing owned", reader, "OWNER", nil},
		{"ACL listing direct", access.DefaultUserID, "VIEWER", nil},
		{"ACL listing inherited", access.DefaultUserID, "EDITOR", &child.ID},
		{"ACL listing private", access.DefaultUserID, "", nil},
	} {
		created, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: item.owner, Type: "PRODUCT", Name: item.name, FolderID: item.folder})
		if err != nil {
			t.Fatal(err)
		}
		if item.permission != "" {
			expected[created.Document.ID] = item.permission
		}
		if item.permission == "VIEWER" {
			if _, err = access.New(db).UpsertGrant(t.Context(), "DOCUMENT", created.Document.ID, access.DefaultUserID, access.ShareRequest{SubjectType: "USER", SubjectID: reader, Role: access.RoleViewer}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err = access.New(db).UpsertGrant(t.Context(), "FOLDER", folder.ID, access.DefaultUserID, access.ShareRequest{SubjectType: "USER", SubjectID: reader, Role: access.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	for _, shared := range []bool{false, true} {
		page, err := domain.ListDocuments(t.Context(), workspace.DocumentListOptions{ActorID: reader, AllFolders: true, Query: "ACL listing", OmitHistory: true, Shared: shared})
		if err != nil {
			t.Fatal(err)
		}
		want := 3
		if shared {
			want = 2
		}
		if len(page.Documents) != want || page.Total != want {
			t.Fatalf("shared=%v changed ACL filtering: %+v", shared, page)
		}
		for _, item := range page.Documents {
			if expected[item.ID] != item.Permission || (shared && item.Permission == "OWNER") {
				t.Fatalf("changed permission: %+v", item)
			}
		}
	}
}
