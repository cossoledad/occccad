package debugartifact

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStoreKeepsRecentLocalArtifactsAndFiltersRequest(t *testing.T) {
	store, err := NewStore(t.TempDir(), 2, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 9, 7, 1, 2, 3, 0, time.UTC) }
	first, err := store.Save(context.Background(), "doc-1", "preview/one", []byte(`{"result":{"status":"CONVERGED"}}`))
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 9, 7, 1, 2, 4, 0, time.UTC) }
	_, _ = store.Save(context.Background(), "doc-1", "preview/two", []byte(`{"result":{"status":"MAX_ITERATIONS"}}`))
	store.now = func() time.Time { return time.Date(2026, 9, 7, 1, 2, 5, 0, time.UTC) }
	latest, _ := store.Save(context.Background(), "doc-1", "commit", []byte(`{"result":{"status":"CONVERGED"}}`))
	items, err := store.List(context.Background(), "doc-1", 50)
	if err != nil || len(items) != 2 || items[0].ID != latest.ID {
		t.Fatalf("items = %#v, err = %v", items, err)
	}
	if _, _, err = store.Read(context.Background(), "doc-1", first.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pruned read err = %v", err)
	}
	item, data, err := store.Read(context.Background(), "doc-1", "latest", "preview/two")
	if err != nil || item.RequestID != "preview/two" || len(data) == 0 {
		t.Fatalf("read = %#v %q %v", item, data, err)
	}
}

func TestStoreJSONSurvivesReopenAndExpiresWithoutAnotherWrite(t *testing.T) {
	root := t.TempDir()
	s, err := NewStore(root, 2, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	item, err := s.SaveJSON(t.Context(), "doc-1", "preview/failed", []byte(`{"failure":"bad geometry"}`))
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(root, 2, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = func() time.Time { return now.Add(time.Minute) }
	if _, _, err = reopened.Read(t.Context(), "doc-1", item.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err = reopened.Read(t.Context(), "other-doc", item.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross document read", err)
	}
	reopened.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, _, err = reopened.Read(t.Context(), "doc-1", item.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired diagnostic retained", err)
	}
}
