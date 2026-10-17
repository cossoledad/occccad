package api

import (
	"bytes"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/thumbnail"
	"github.com/occccad/occccad/internal/workspace"
	"image/png"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestDefaultThumbnailResponseIsPNG(t *testing.T) {
	response := httptest.NewRecorder()
	writeThumbnail(response, thumbnail.DefaultForType("PART"), "default")
	if response.Header().Get("Content-Type") != "image/png" || response.Header().Get("X-OCCCCAD-Thumbnail-State") != "default" {
		t.Fatal(response.Header())
	}
	config, err := png.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
	if err != nil || config.Width != thumbnail.Width || config.Height != thumbnail.Height {
		t.Fatalf("invalid default PNG: %v %v", config, err)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("pending placeholder must not become a permanently cached image")
	}
}

// Internal Workspace writers bypass HTTP mutation callbacks. Reading a missing
// derived preview must repair it without opening or editing the document.
func TestMissingThumbnailRepairsWithoutOpeningDocument(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "previews.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifact.NewService(db, local)
	domain := workspace.NewWithArtifacts(db, nil, artifacts)
	server := &Server{database: db, workspace: domain, access: access.New(db), artifacts: artifacts, jobs: jobs.New(db)}
	for _, kind := range []string{"PART", "PRODUCT"} {
		t.Run(kind, func(t *testing.T) {
			view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: kind, Name: "preview repair"})
			if err != nil {
				t.Fatal(err)
			}
			get := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest("GET", "/preview", nil)
				r.SetPathValue("documentID", view.Document.ID)
				r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: access.DefaultUserID}))
				w := httptest.NewRecorder()
				server.documentPreview(w, r)
				return w
			}
			for i := 0; i < 2; i++ {
				w := get()
				if w.Code != 200 || w.Header().Get("X-OCCCCAD-Thumbnail-State") != "pending" {
					t.Fatalf("missing preview not pending: %d %v", w.Code, w.Header())
				}
				config, err := png.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
				if err != nil || config.Width != thumbnail.Width {
					t.Fatal("pending image", err)
				}
			}
			var count int
			if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.jobs WHERE document_id=$1 AND job_type='THUMBNAIL_RENDER'`, view.Document.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("non-idempotent repair: %d %v", count, err)
			}
			if err = server.enqueueDocumentPreviews(t.Context(), view, access.DefaultUserID); err != nil {
				t.Fatal("mutation scheduling", err)
			}
			object, err := artifacts.Put(t.Context(), artifact.KindThumbnail, thumbnail.ContentType, bytes.NewReader(thumbnail.Default(view)))
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(t.Context(), `INSERT INTO occccad.document_previews(document_id,version_id,preview_identity,renderer_version,object_id,state) VALUES($1,$2,$3,$4,$5,'READY')`, view.Document.ID, view.Document.VersionID, "test-ready", thumbnail.RendererVersion, object.ID)
			if err != nil {
				t.Fatal(err)
			}
			w := get()
			if w.Header().Get("X-OCCCCAD-Thumbnail-State") != "ready" || w.Header().Get("ETag") == "" {
				t.Fatal("completed preview not served", w.Header())
			}
			current, err := domain.GetDocument(t.Context(), view.Document.ID)
			if err != nil || current.Document.VersionID != view.Document.VersionID {
				t.Fatal("preview changed document head", err)
			}
		})
	}
}
