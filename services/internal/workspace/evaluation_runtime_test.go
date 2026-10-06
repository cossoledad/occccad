package workspace

import (
	"context"
	"errors"
	"github.com/occccad/occccad/internal/modelcore"
	"sync/atomic"
	"testing"
)

func TestPreparedRuntimeOwnershipColdAndFailure(t *testing.T) {
	cache := &preparedCache{}
	var calls atomic.Int32
	compute := func() ([]int, error) { calls.Add(1); return []int{1, 2}, nil }
	a, err := preparedResult(context.Background(), cache, "profile:one", "same", compute)
	if err != nil {
		t.Fatal(err)
	}
	a[0] = 99
	b, err := preparedResult(context.Background(), cache, "profile:two", "same", compute)
	if err != nil || b[0] != 1 || calls.Load() != 1 {
		t.Fatal("shared preparation leaked mutable ownership", b, err, calls.Load())
	}
	_, err = preparedResult(WithColdEvaluation(context.Background()), cache, "profile:two", "same", compute)
	if err != nil || calls.Load() != 2 {
		t.Fatal("cold preparation used hot cache")
	}
	failure := errors.New("failed input")
	_, err = preparedResult(context.Background(), cache, "profile:one", "failure", func() ([]int, error) { return nil, failure })
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	_, err = preparedResult(context.Background(), cache, "profile:one", "failure", compute)
	if err != nil || calls.Load() != 3 {
		t.Fatal("failed preparation poisoned recovery")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = preparedResult(ctx, cache, "profile:one", "cancelled", compute)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPreparedRuntimeColdRequestAndSingleFlight(t *testing.T) {
	cache := &preparedCache{}
	var calls atomic.Int32
	compute := func() (int, error) { return int(calls.Add(1)), nil }
	cold := withEvaluationRuntime(WithColdEvaluation(context.Background()))
	for range 2 {
		if _, err := preparedResult(cold, cache, "profile:section", "same", compute); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cold request prepared identical frozen input twice")
	}
	cold = withEvaluationRuntime(WithColdEvaluation(context.Background()))
	if _, err := preparedResult(cold, cache, "profile:section", "same", compute); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("cold request reused previous request")
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 2)
	blocked := func() (int, error) { calls.Add(1); close(entered); <-release; return 7, nil }
	go func() {
		_, err := preparedResult(context.Background(), cache, "profile:one", "concurrent", blocked)
		done <- err
	}()
	<-entered
	go func() {
		_, err := preparedResult(context.Background(), cache, "profile:two", "concurrent", blocked)
		done <- err
	}()
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("shared frozen preparation executed twice")
	}
}

func TestEvaluationProjectionUsesSketchScopeAndContextDependency(t *testing.T) {
	m := newPartModel()
	m.ContextReferences = []ContextReference{{ID: "source-reference", Name: "source", OwningWorkspace: "main", SourceDocumentID: "source-part", ResolvedRevisionID: "source-revision", ReferenceMode: "FOLLOW_HEAD", Publication: PublicationRef{PublicationID: "source-publication", ExpectedType: "CURVE", CompatibilityVersion: "1.0.0"}}}
	for i, id := range []string{"sketch-one", "sketch-two"} {
		sketch := testRectangleSketch(id, "XY")
		sketch.Order = i + 1
		sketch.BodyID = m.ActiveBodyID
		sketch.Sketch.ExternalGeometry = []SketchExternalGeometry{{ID: "shared-local-id", ContextReferenceID: "source-reference", ProjectionKind: "ORTHOGONAL", SourceVersionID: "source-revision", Status: "PENDING", PersistentSelection: modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: "source-part", SourceBodyID: "source-body", Anchor: modelcore.SemanticTopologyRef{FeatureID: "source-feature", OutputSlot: "SIDE/profile-edge"}, ExpectedType: modelcore.PersistentTopologyEdge, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}}}}
		m.Features = append(m.Features, sketch)
	}
	graph, _, err := buildPartEvaluation(m, "revision", "model", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, key := range graph.TopologicalOrder() {
		found[string(key)] = true
	}
	for _, id := range []string{"sketch-one", "sketch-two"} {
		if !found["projection:"+id+"/shared-local-id"] {
			t.Fatal("projection identity lost sketch owner")
		}
	}
}
