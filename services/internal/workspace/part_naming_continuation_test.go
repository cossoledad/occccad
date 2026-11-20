package workspace

import (
	"bytes"
	"path/filepath"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"google.golang.org/protobuf/proto"
)

func checkExactNamingSuffix(t *testing.T, root *workerv1.PartTopologyManifest, payloads map[string][]byte) {
	t.Helper()
	ctx := t.Context()
	db, err := database.Open(ctx, "sqlite:"+filepath.Join(t.TempDir(), "suffix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	local, err := artifactstore.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifactstore.NewService(db, local, local)
	s := &Service{database: db, artifacts: artifacts}
	put := func(kind artifactstore.Kind, data []byte) artifactstore.Object {
		t.Helper()
		o, err := artifacts.Put(ctx, kind, "application/octet-stream", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	storeRoot := func(r *workerv1.PartTopologyManifest) artifactstore.Object {
		t.Helper()
		data, err := proto.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		return put(artifactstore.KindTopologyManifest, data)
	}
	before := proto.CloneOf(root)
	before.History = before.History[:1]
	predecessor := before.History[0]
	before.GeometryId, before.Bodies[0].TipFeatureId = predecessor.ResultGeometryId, predecessor.FeatureId
	parent := storeRoot(before)
	base := put(artifactstore.KindNamingHistory, payloads[predecessor.Sha256])
	if err = artifacts.RecordNamingClosure(ctx, parent, []artifactstore.Object{base}); err != nil {
		t.Fatal(err)
	}
	checkpoint := partCheckpoint{object: parent, naming: before, continuation: &workerv1.PartContinuation{BodyId: before.Bodies[0].BodyId}}
	current := storeRoot(root)
	last := root.History[1]
	suffix := put(artifactstore.KindNamingHistory, payloads[last.Sha256])
	objects := map[string]artifactstore.Object{last.Sha256: suffix}
	if display, err := s.adoptNamingContinuation(ctx, checkpoint, current, root, objects, true); err != nil || display != nil {
		t.Fatal("missing display state must use the full contribution reader", err)
	}
	view, err := s.adoptNamingContinuation(ctx, checkpoint, current, root, objects, false)
	if err != nil || view == nil || view.associations != nil || len(view.Tips) != 1 {
		t.Fatal("exact stage required visual state", err)
	}
	children, err := artifacts.NamingHistorySuffix(ctx, current.ID, predecessor.Sha256)
	if err != nil || len(children) != 1 || children[0].ID != suffix.ID {
		t.Fatal("unpublished exact-stage successor", children, err)
	}
	// Force a cache hit on the verified block while changing its advertised
	// header. Sharing a SHA cannot authorize a different row count.
	forged := proto.CloneOf(root)
	forged.History[1].FullLineageCount++
	if _, err = s.adoptNamingContinuation(ctx, checkpoint, current, forged, objects, false); err == nil {
		t.Fatal("accepted inconsistent cached block header")
	}
}
