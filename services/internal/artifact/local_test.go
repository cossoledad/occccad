package artifact

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalStoreRoundTripAndDeduplication(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Put(context.Background(), KindExchangeSource, "application/step", strings.NewReader("STEP"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(context.Background(), KindExchangeSource, "application/step", strings.NewReader("STEP"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key || first.SHA256 != second.SHA256 {
		t.Fatalf("expected content-addressed deduplication: %#v %#v", first, second)
	}
	reader, err := store.Open(context.Background(), first.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	value, _ := io.ReadAll(reader)
	if string(value) != "STEP" {
		t.Fatalf("unexpected content %q", value)
	}
}

func TestLocalStoreRejectsTraversal(t *testing.T) {
	store, _ := NewLocalStore(t.TempDir())
	if _, err := store.Open(context.Background(), "../secret"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}
