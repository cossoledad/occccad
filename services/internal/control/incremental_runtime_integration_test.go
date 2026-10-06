package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestIncrementalRuntimeProductionThroughRouter(t *testing.T) {
	runIncrementalRuntimeProduction(t, false)
}
func TestIncrementalRuntimeEvictionThroughRouter(t *testing.T) {
	t.Setenv("OCCCCAD_STAGE_CACHE_BYTES", "1")
	runIncrementalRuntimeProduction(t, true)
}
func runIncrementalRuntimeProduction(t *testing.T, cacheDisabled bool) {
	service, artifacts, _, _, pool := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Incremental production runtime"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-runtime-%d", view.Document.ID, seq)
		}
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		view = v
	}
	rectangle := func(x, y, width, height float64) string {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + width, Y: y + height}}}})
		return id
	}
	baseSketch := rectangle(-20, -20, 40, 40)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: baseSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5})
	base := view.Part.Features[len(view.Part.Features)-1]
	stemSketch := rectangle(-8, -8, 16, 16)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stemSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 15})
	stem := view.Part.Features[len(view.Part.Features)-1]
	tailSketch := rectangle(-3, -3, 6, 6)
	preview := func(ctx context.Context, r workspace.CommandRequest) workspace.CommandPreview {
		t.Helper()
		r.ActorID = p6Actor
		start := time.Now()
		p, e := service.PreviewCommand(ctx, view.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		if p.Runtime == nil || p.Artifact == nil {
			t.Fatal("production runtime evidence missing")
		}
		var executed, reused uint32
		var exact, naming, mesh, encoding, queue, ioMS float64
		var peak uint64
		for _, w := range p.Runtime.WorkerRuns {
			executed += w.StagesExecuted
			reused += w.StagesReused
			exact += w.ExactMs
			naming += w.NamingMs
			mesh += w.MeshMs
			encoding += w.EncodingMs
			queue += w.QueueMs
			ioMS += w.ArtifactIoMs
			peak = max(peak, w.PeakRssBytes)
		}
		var profiles, correspondence int
		for key, r := range p.Runtime.Nodes {
			if strings.HasPrefix(string(key), "profile:") {
				profiles += r.Executions
			}
			if strings.HasPrefix(string(key), "loft-correspondence:") {
				correspondence += r.Executions
			}
		}
		t.Logf("preparation profiles=%d correspondence=%d", profiles, correspondence)
		t.Logf("runtime request=%s cold=%t elapsed_ms=%.2f executed=%d reused=%d exact_ms=%.2f naming_ms=%.2f mesh_ms=%.2f encoding_ms=%.2f queue_ms=%.2f io_ms=%.2f peak_rss=%d", r.RequestID, ctx != t.Context(), float64(time.Since(start))/float64(time.Millisecond), executed, reused, exact, naming, mesh, encoding, queue, ioMS, peak)
		if cacheDisabled && reused != 0 {
			t.Fatal("evicted stage was reported reused")
		}
		return p
	}
	noUpstream := func(p workspace.CommandPreview, ids ...string) {
		t.Helper()
		if cacheDisabled {
			return
		}
		for _, id := range ids {
			if r := p.Runtime.Nodes[modelcore.DependencyKey("feature:"+id)]; r.Executions != 0 {
				t.Fatalf("unchanged upstream executed: %s %+v", id, r)
			}
		}
	}
	representation := func(p workspace.CommandPreview, role string) []byte {
		t.Helper()
		_, r, e := artifacts.Open(t.Context(), p.Artifact.Representations[role].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Close()
		b, e := io.ReadAll(r)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	equivalent := func(a, b workspace.CommandPreview) {
		t.Helper()
		if a.Artifact.GeometryID != b.Artifact.GeometryID || string(representation(a, "NAMING")) != string(representation(b, "NAMING")) {
			t.Fatal("cold/incremental exact shape or real Naming differs")
		}
		if math.Abs(a.Artifact.Volume-b.Artifact.Volume) > 1e-8 {
			t.Fatal("volume differs")
		}
		if len(representation(a, "BREP")) == 0 || len(representation(b, "BREP")) == 0 {
			t.Fatal("missing exact shape")
		}
		if fmt.Sprint(a.Artifact.BBox) != fmt.Sprint(b.Artifact.BBox) {
			t.Fatal("dimensions differ")
		}
	}
	appendRequest := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", RequestID: "runtime-append-tail", SketchID: tailSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20}
	hot := preview(t.Context(), appendRequest)
	noUpstream(hot, base.ID, stem.ID)
	if r := hot.Runtime.Nodes[modelcore.DependencyKey("feature:"+workspaceCommandFeatureID(t, hot, base.ID, stem.ID))]; r.Executions != 1 {
		t.Fatalf("append did not execute precisely its new stage: %+v", hot.Runtime)
	}
	cold := preview(workspace.WithColdEvaluation(t.Context()), appendRequest)
	equivalent(hot, cold)
	// Promotion preserves the candidate geometry and compare-and-swap base.
	appendRequest.PreviewID = hot.PreviewID
	apply(appendRequest)
	tail := view.Part.Features[len(view.Part.Features)-1]
	stem.Length = 16
	var digest string
	var visit func(workspace.DocumentStructureNode)
	visit = func(n workspace.DocumentStructureNode) {
		if n.EntityID == stem.ID && n.DefinitionDigest != "" {
			digest = n.DefinitionDigest
		}
		for _, c := range n.Children {
			visit(c)
		}
	}
	visit(*view.StructureTree)
	editRequest := workspace.CommandRequest{Type: "EDIT_FEATURE", RequestID: "runtime-edit-middle", TargetID: stem.ID, ExpectedFeatureDigest: digest, Feature: &stem}
	edited := preview(t.Context(), editRequest)
	noUpstream(edited, base.ID)
	if edited.Runtime.Nodes[modelcore.DependencyKey("feature:"+stem.ID)].Executions != 1 || edited.Runtime.Nodes[modelcore.DependencyKey("feature:"+tail.ID)].Executions != 1 {
		t.Fatal("middle edit did not rebuild only its descendants", edited.Runtime)
	}
	equivalent(edited, preview(workspace.WithColdEvaluation(t.Context()), editRequest))
	renamed := preview(t.Context(), workspace.CommandRequest{Type: "RENAME_FEATURE", RequestID: "runtime-rename", TargetID: tail.ID, Name: "renamed tail"})
	for _, w := range renamed.Runtime.WorkerRuns {
		if w.GeneratorCalls+w.ModifierCalls+w.BodyOperationCalls != 0 {
			t.Fatal("name change rebuilt geometry")
		}
	}
	if renamed.Artifact.GeometryID != hot.Artifact.GeometryID {
		t.Fatal("name change altered exact shape")
	}
	invisible := false
	hidden := preview(t.Context(), workspace.CommandRequest{Type: "SET_BODY_VISIBILITY", RequestID: "runtime-display", BodyID: base.BodyID, Visible: invisible})
	for _, w := range hidden.Runtime.WorkerRuns {
		if w.GeneratorCalls+w.ModifierCalls+w.BodyOperationCalls != 0 {
			t.Fatal("display change rebuilt geometry")
		}
	}
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.PreviewCommand(cancelledCtx, view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, Type: "CREATE_SOLID_FEATURE", RequestID: "runtime-cancelled", SketchID: tailSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 21}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled evaluation did not stop", err)
	}
	// Continuous loft preview: section preparation / correspondence and fixed prefix stay warm.
	sections := []workspace.LoftSection{}
	for i, z := range []float64{5, 25} {
		apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: fmt.Sprintf("loft section %d", i), Origin: [3]float64{0, 0, z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
		plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: fmt.Sprintf("circle-%d", i), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: float64(6 - i*2)}}}})
		sections = append(sections, workspace.LoftSection{SketchID: id})
	}
	loft := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", RequestID: "runtime-loft-session", Feature: &workspace.Feature{Type: "LOFT", Operation: "ADD", BodyID: base.BodyID, Sections: sections}}
	p0 := preview(t.Context(), loft)
	noUpstream(p0, base.ID, stem.ID, tail.ID)
	loft.Feature.Ruled = true
	p1 := preview(t.Context(), loft)
	noUpstream(p1, base.ID, stem.ID, tail.ID)
	for _, s := range sections {
		if p1.Runtime.Nodes[modelcore.DependencyKey("profile:"+s.SketchID)].Executions != 0 {
			t.Fatal("fixed loft section profile rebuilt")
		}
	}
	for key, r := range p1.Runtime.Nodes {
		if strings.HasPrefix(string(key), "loft-correspondence:") && r.Executions != 0 {
			t.Fatal("fixed correspondence rebuilt")
		}
	}
	equivalent(p1, preview(workspace.WithColdEvaluation(t.Context()), loft))
	// A real process restart loses every native stage. The persisted model and input
	// artifacts recover it; rebuilding only a Go Service would not prove this.
	pool.mu.Lock()
	worker := pool.workers[0]
	pool.mu.Unlock()
	pool.removeFailedWorker(worker, errors.New("isolated runtime restart test"))
	loft.Feature.Ruled = false
	loft.RequestID = "runtime-after-worker-restart"
	restarted := preview(t.Context(), loft)
	for _, id := range []string{base.ID, stem.ID, tail.ID} {
		if restarted.Runtime.Nodes[modelcore.DependencyKey("feature:"+id)].Executions != 1 {
			t.Fatal("fresh Worker falsely reported stage reuse", restarted.Runtime)
		}
	}
	equivalent(restarted, preview(workspace.WithColdEvaluation(t.Context()), loft))
}

// New feature identity is obtained from executed runtime records, not display names.
func workspaceCommandFeatureID(t *testing.T, p workspace.CommandPreview, upstream ...string) string {
	t.Helper()
	for _, w := range p.Runtime.WorkerRuns {
		for _, id := range w.ExecutedFeatureIds {
			skip := false
			for _, u := range upstream {
				skip = skip || id == u
			}
			if !skip {
				return id
			}
		}
	}
	t.Fatal("new stage not recorded")
	return ""
}
