//go:build integration

package thumbnail

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

// Explicitly selected development revisions use the production loader and renderer.
func TestDocumentRenderMeasurements(t *testing.T) {
	ids := os.Getenv("OCCCCAD_THUMBNAIL_DOCUMENT_IDS")
	if ids == "" {
		t.Skip("set OCCCCAD_THUMBNAIL_DOCUMENT_IDS to measure stored documents")
	}
	cfg := testsupport.Environment(t)
	db := testsupport.OpenDevelopmentDatabase(t)
	store, staging, err := artifact.OpenConfigured(t.Context(), cfg.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	ws := workspace.NewWithArtifacts(db, nil, artifact.NewService(db, store, staging))
	for _, id := range strings.Split(ids, ",") {
		start := time.Now()
		view, err := ws.GetDocument(t.Context(), strings.TrimSpace(id))
		if err != nil {
			t.Fatal(err)
		}
		load := estimateScene(view)
		t.Logf("document=%s revision=%s load=%s instances=%d geometries=%d placed_triangles=%d", view.Document.Name, view.Document.VersionID, time.Since(start), load.instances, len(load.uses), load.triangles)
		for i := 0; i < 3; i++ {
			ctx, timing := performance.WithRecorder(t.Context())
			start := time.Now()
			png, overview, err := RenderDisplay(ctx, view, cfg.ThumbnailRenderTimeout, ws.HydrateArtifact)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("document=%s sample=%d overview=%t render=%s phases_ms=%v sha=%x", view.Document.Name, i+1, overview, time.Since(start), timing.SnapshotMilliseconds(), sha256.Sum256(png))
			if dir := os.Getenv("OCCCCAD_THUMBNAIL_GALLERY"); dir != "" && i == 0 {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, view.Document.ID+".png"), png, 0644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
