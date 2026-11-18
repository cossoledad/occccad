package artifact

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/database"
)

func TestNamingArtifactClosureSharesPrefixesAndProtectsReachableBlocks(t *testing.T) {
	ctx := t.Context()
	db, err := database.Open(ctx, "sqlite:"+filepath.Join(t.TempDir(), "artifacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, store, store)
	put := func(kind Kind, text string) Object {
		t.Helper()
		o, err := service.Put(ctx, kind, "application/octet-stream", strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	a, b, c := put(KindNamingHistory, "a"), put(KindNamingHistory, "b"), put(KindNamingHistory, "c")
	first, second := put(KindTopologyManifest, "first"), put(KindTopologyManifest, "second")
	if err = service.RecordNamingClosure(ctx, first, []Object{a, b}); err != nil {
		t.Fatal(err)
	}
	if err = service.RecordNamingClosure(ctx, second, []Object{a, b, c}); err != nil {
		t.Fatal(err)
	}
	if err = service.RecordNamingClosure(ctx, second, []Object{a, b, c}); err != nil {
		t.Fatal("idempotent closure", err)
	}
	if err = service.RecordNamingClosure(ctx, second, []Object{a, c}); err == nil {
		t.Fatal("immutable closure changed")
	}
	if _, err = db.Exec(ctx, `DELETE FROM occccad.artifact_objects WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	dependencies, err := service.Dependencies(ctx, second.ID)
	if err != nil || len(dependencies) != 1 || dependencies[0].ID != c.ID {
		t.Fatal(dependencies, err)
	}
	reachable, err := service.ReachableObjects(ctx, []string{second.ID})
	if err != nil || len(reachable) != 4 {
		t.Fatal(reachable, err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM occccad.artifact_objects WHERE id=$1`, a.ID); err == nil {
		t.Fatal("reachable shared block was deleted")
	}
	objects, err := service.ReadyByDigests(ctx, KindNamingHistory, []string{a.SHA256, b.SHA256, c.SHA256, second.SHA256})
	if err != nil || len(objects) != 3 {
		t.Fatal(objects, err)
	}
}
