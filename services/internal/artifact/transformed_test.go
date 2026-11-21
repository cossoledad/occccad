package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
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

func TestVerifiedSetPublishesOnlyExactStagedPayload(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "artifact.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	stage, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dest, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, dest, stage)
	input := []byte("immutable topology root")
	staged, err := stage.Put(t.Context(), KindTopologyManifest, "application/octet-stream", bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		digest string
		size   int64
	}{
		{strings.Repeat("0", 64), staged.Size}, {staged.SHA256, staged.Size - 1}, {staged.SHA256, staged.Size + 1},
	} {
		_, data, err := service.PrepareVerified(t.Context(), KindTopologyManifest, staged.ContentType, staged.Key, tc.digest, tc.size, true)
		if err == nil || data != nil {
			t.Fatal("accepted invalid staged data")
		}
	}
	var count int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.artifact_objects WHERE state='READY'`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	existing, err := dest.Put(t.Context(), KindTopologyManifest, staged.ContentType, bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	// The durable copy must match the bytes decoded by admission even when
	// an earlier file at that key has suffered same-size corruption.
	if err := os.WriteFile(filepath.Join(dest.Root(), existing.Key), bytes.Repeat([]byte{'x'}, len(input)), 0600); err != nil {
		t.Fatal(err)
	}
	root, data, err := service.PrepareVerified(t.Context(), KindTopologyManifest, staged.ContentType, staged.Key, staged.SHA256, staged.Size, true)
	if err != nil || !bytes.Equal(data, input) {
		t.Fatal(data, err)
	}
	member, err := stage.Put(t.Context(), KindNamingHistory, "application/octet-stream", strings.NewReader("history block"))
	if err != nil {
		t.Fatal(err)
	}
	block, raw, err := service.PrepareVerified(t.Context(), KindNamingHistory, member.ContentType, member.Key, member.SHA256, member.Size, false)
	if err != nil || raw != nil {
		t.Fatal(raw, err)
	}
	// Force the later upsert to fail, after the block insert in the same transaction.
	badRoot := root
	badRoot.stored.Size = -1
	if _, err = service.PublishPrepared(t.Context(), []PreparedObject{badRoot, block}); err == nil {
		t.Fatal("accepted invalid object set")
	}
	if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.artifact_objects`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial registration", count, err)
	}
	for _, key := range []string{member.Key, staged.Key} {
		r, err := stage.Open(t.Context(), key)
		if err != nil {
			t.Fatal("lost staged source on rollback", err)
		}
		r.Close()
	}
	objects, err := service.PublishPrepared(t.Context(), []PreparedObject{root, block})
	if err != nil || len(objects) != 2 || objects[0].SHA256 != staged.SHA256 || objects[1].SHA256 != member.SHA256 {
		t.Fatal(objects, err)
	}
	retried, err := service.PublishPrepared(t.Context(), []PreparedObject{block, root})
	if err != nil || len(retried) != 2 || retried[0].ID != objects[1].ID || retried[1].ID != objects[0].ID {
		t.Fatal("retry changed verified object identities", retried, err)
	}
	o := objects[0]
	reader, err := dest.Open(t.Context(), o.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	saved, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(saved, data) {
		t.Fatal(saved, err)
	}
	for _, key := range []string{staged.Key, member.Key} {
		if reader, err := stage.Open(t.Context(), key); err == nil {
			reader.Close()
			t.Fatal("accepted stage was retained", key)
		}
	}
}
