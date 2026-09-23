package workspace

import (
	"context"
	"errors"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssemblyReadCacheScopeAndFailures(t *testing.T) {
	ctx := withAssemblyReadCache(context.Background())
	calls := 0
	read := func() (int, error) { calls++; return calls, nil }
	key := assemblyReadKey{"manifest", "part", "v1"}
	for i := 0; i < 12; i++ {
		if _, err := assemblyRead(ctx, key, read); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("repeated endpoints loaded %d times", calls)
	}
	for _, other := range []assemblyReadKey{{"manifest", "part", "v2"}, {"manifest", "other", "v1"}} {
		_, _ = assemblyRead(ctx, other, read)
	}
	_, _ = assemblyRead(withAssemblyReadCache(ctx), key, read)
	if calls != 4 {
		t.Fatalf("document/revision/operation isolation: %d reads", calls)
	}
	failure := errors.New("temporary storage failure")
	failedKey := assemblyReadKey{"manifest", "part", "retry"}
	_, err := assemblyRead(ctx, failedKey, func() (int, error) { return 0, failure })
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	value, err := assemblyRead(ctx, failedKey, read)
	if err != nil || value != 5 {
		t.Fatalf("failure cached: %v %v", value, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := assemblyRead(cancelled, key, read); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached read ignored cancellation: %v", err)
	}
}

func TestAssemblyResolvedTopologyUsesValidatedRevision(t *testing.T) {
	ctx := withAssemblyReadCache(context.Background())
	_, _ = assemblyRead(ctx, assemblyReadKey{"live-document", "part-1", ""}, func() (bool, error) { return true, nil })
	selection := testSelection()
	ref := testRef(selection.Anchor.FeatureID, selection.Anchor.OutputSlot)
	seed := func(version, geometry string, localID uint64) {
		manifest := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{FeatureId: "extrude-1", BodyId: "body-main", ResultGeometryId: geometry, TopologyHistoryComplete: true, SemanticOutputs: []*workerv1.SemanticTopologyOutput{testOutput(ref, localID, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE)}}}}
		_, err := assemblyRead(ctx, assemblyReadKey{"manifest", "part-1", version}, func() (assemblyManifestRead, error) {
			return assemblyManifestRead{manifest, geometry, "digest-" + version}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = assemblyRead(ctx, assemblyTopologyKey{geometry, "FACE", localID}, func() (TopologyElementProperties, error) {
			return TopologyElementProperties{GeometryKey: geometry, Kind: "FACE", LocalID: localID}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seed("v1", "geometry-1", 1)
	seed("v2", "geometry-2", 6)
	// No database or worker: any redundant inspection authorization/rebinding or
	// uncached artifact fetch would fail. The real persistent resolver still runs.
	service := &Service{}
	request := ResolvePersistentSelectionRequest{Selection: selection, SourceVersionID: "v1", TargetVersionID: "v2", PolicyDigest: modelcore.TopologyNamingPolicyDigest}
	for i := 0; i < 3; i++ {
		got, err := service.resolvedAssemblyTopologyProperties(ctx, "part-1", request)
		if err != nil || got.Properties == nil || got.Properties.GeometryKey != "geometry-2" || got.Properties.LocalID != 6 {
			t.Fatalf("target revision: %+v %v", got, err)
		}
	}
	request.ManifestDigest = "wrong"
	got, err := service.resolvedAssemblyTopologyProperties(ctx, "part-1", request)
	if err != nil || got.Properties != nil || got.Resolution.Status != modelcore.SelectionContractMismatch {
		t.Fatalf("manifest mismatch bypassed: %+v %v", got, err)
	}
	request.ManifestDigest = ""
	request.Selection.CreationEvidence.EvidenceDigest = "wrong"
	got, err = service.resolvedAssemblyTopologyProperties(ctx, "part-1", request)
	if err != nil || got.Properties != nil || got.Resolution.Status != modelcore.SelectionContractMismatch {
		t.Fatalf("source evidence bypassed: %+v %v", got, err)
	}
	if _, err = service.resolvedAssemblyTopologyProperties(ctx, "other-document", request); !errors.Is(err, ErrValidation) {
		t.Fatalf("document validation bypassed: %v", err)
	}
}
