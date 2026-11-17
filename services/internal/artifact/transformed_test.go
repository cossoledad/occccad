package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/database"
)

func TestTransformedPublicationVerifiesDiscardedSourceBeforeReady(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "artifact.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, store, store)
	input := []byte("mesh-prefix obsolete-metadata")
	staged, err := store.Put(t.Context(), KindGLB, "model/gltf-binary", bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	transform := func(source io.Reader, size int64) (io.Reader, error) {
		return io.MultiReader(strings.NewReader("new:"), io.LimitReader(source, 11), strings.NewReader(":metadata")), nil
	}
	for _, c := range []struct {
		name, digest string
		size         int64
		cancel       bool
	}{
		{"bad-digest", strings.Repeat("0", 64), int64(len(input)), false},
		{"truncated", staged.SHA256, int64(len(input) + 1), false},
		{"extra-bytes", staged.SHA256, int64(len(input) - 1), false},
		{"cancel", staged.SHA256, int64(len(input)), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if c.cancel {
				cancel()
			}
			if _, err := service.AdoptTransformed(ctx, KindGLB, "model/gltf-binary", staged.Key, c.digest, c.size, transform); err == nil {
				t.Fatal("unverified source became publishable")
			}
			var count int
			if err := db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.artifact_objects WHERE state='READY'`).Scan(&count); err != nil || count != 0 {
				t.Fatal("corrupt result marked READY", count, err)
			}
		})
	}
	object, err := service.AdoptTransformed(t.Context(), KindGLB, "model/gltf-binary", staged.Key, staged.SHA256, staged.Size, transform)
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err := service.Open(t.Context(), object.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte("new:mesh-prefix:metadata")
	hash := sha256.Sum256(expected)
	if !bytes.Equal(data, expected) || object.SHA256 != hex.EncodeToString(hash[:]) {
		t.Fatal("transformed bytes or final digest changed", string(data))
	}
}
