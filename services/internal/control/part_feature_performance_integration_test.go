//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/workspace"
)

// Generated mechanical STEP, not the unavailable user's B-pillar. Controls
// source complexity independently of the number of subsequent Feature steps.
func TestDenseSTEPFaceSketchAndFirstCutThroughRouter(t *testing.T) {
	service, _, db, client, pool := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Dense STEP source plate"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	var lastElapsed time.Duration
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-dense-%d", view.Document.ID, seq)
		}
		r.VersionID = view.Document.VersionID
		started := time.Now()
		view, err = service.ApplyCommand(ctx, view.Document.ID, r)
		lastElapsed = time.Since(started)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[0].ID
	ops := []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 300, Y: 150}}}
	for i := 0; i < 225; i++ {
		ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: fmt.Sprintf("hole-%d", i), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 5 + 10*float64(i%15), Y: 5 + 10*float64(i/15)}, Radius: 1}})
	}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: ops})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	secondProfile := view.Part.Features[len(view.Part.Features)-1].ID
	var secondHoles []workspace.SketchOperation
	for i := 0; i < 225; i++ {
		secondHoles = append(secondHoles, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: fmt.Sprintf("hole-%d", i), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 155 + 10*float64(i%15), Y: 5 + 10*float64(i/15)}, Radius: 1}})
	}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: secondProfile, Operations: secondHoles})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: secondProfile, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 10})
	graph, e := service.ExchangeExportGraph(ctx, view.Document.ID, workspace.ExchangeExportOptions{})
	if e != nil {
		t.Fatal(e)
	}
	prefix := view.Document.ID + "-dense-step"
	source, e := client.ExportExchange(ctx, prefix, "STEP", artifactstore.StagingKey(prefix, "source.step"), graph)
	if e != nil {
		t.Fatal(e)
	}
	key := "sha256:" + source.SHA256
	evaluation, e := client.ImportExchange(ctx, prefix, key, "STEP", source, "", artifactstore.StagingKey(prefix, "shape.brep"), artifactstore.StagingKey(prefix, "mesh.glb"))
	if e != nil {
		t.Fatal(e)
	}
	view, e = service.CommitImportedPart(ctx, p6Actor, "", prefix, "Dense imported plate", "plate.step", "STEP", key, evaluation)
	if e != nil {
		t.Fatal(e)
	}
	base := activeBodyArtifact(t, view)
	if len(view.Part.Features) != 1 || view.Part.Features[0].Type != "IMPORT_BODY" {
		t.Fatal("fixture did not cross the real STEP import boundary")
	}
	if _, e = pool.UnloadGeometry(ctx, &workerv1.UnloadGeometryRequest{GeometryId: base.GeometryID}); e != nil {
		t.Fatal(e)
	}
	t.Logf("dense STEP bytes=%d BREP bytes=%d topology=%v triangles=%d", source.Size, base.BRepBytes, base.Topology, base.TriangleCount)
	frameCtx, frameTimes := perf.WithRecorder(ctx)
	if _, e = service.GetTopologyElementPropertiesAtVersion(frameCtx, view.Document.ID, view.Document.VersionID, base.GeometryKey, "FACE", 1); e != nil {
		t.Fatal(e)
	}
	pickStarted := time.Now()
	pick := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		n, ok := p.Properties["normal"].([3]float64)
		return ok && n[2] > .99
	})
	t.Logf("support-first-read=%v; remaining-support-pick elapsed_ms=%.3f (artifact/input caches not flushed)", frameTimes.SnapshotMilliseconds(), float64(time.Since(pickStarted))/float64(time.Millisecond))
	// The deliberate support read and creation use the same frozen artifact.
	_, e = service.GetTopologyElementPropertiesAtVersion(frameCtx, view.Document.ID, view.Document.VersionID, pick.GeometryKey, "FACE", pick.LocalID)
	if e != nil {
		t.Fatal(e)
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: pick.GeometryKey, TopologyID: pick.LocalID})
	sketch := view.Part.Features[len(view.Part.Features)-1]
	frame := sketch.Sketch.Support
	u, n := frame.XDirection, frame.Normal
	v := [3]float64{n[1]*u[2] - n[2]*u[1], n[2]*u[0] - n[0]*u[2], n[0]*u[1] - n[1]*u[0]}
	local := func(x, y float64) *workspace.SketchPoint2 {
		d := [3]float64{x - frame.Origin[0], y - frame.Origin[1], 10 - frame.Origin[2]}
		return &workspace.SketchPoint2{X: d[0]*u[0] + d[1]*u[1] + d[2]*u[2], Y: d[0]*v[0] + d[1]*v[1] + d[2]*v[2]}
	}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: local(1, 1), Second: local(3, 3)}}})
	if activeBodyArtifact(t, view).GeometryKey != base.GeometryKey {
		t.Fatal("drawing on dense STEP regenerated Body")
	}
	r := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: prefix + "-first-cut", VersionID: view.Document.VersionID, SketchID: sketch.ID, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 10, Reversed: true}
	start := time.Now()
	preview, e := service.PreviewCommand(ctx, view.Document.ID, r)
	if e != nil {
		t.Fatal(e)
	}
	if preview.Artifact == nil || preview.Runtime == nil || math.Abs(preview.Artifact.Volume-(base.Volume-40)) > 1e-4 {
		t.Fatalf("first STEP cut invalid: %v", preview.Artifact)
	}
	var calls uint32
	for _, run := range preview.Runtime.WorkerRuns {
		calls += run.GeneratorCalls
		t.Logf("first-cut worker=%s", mustRuntimeJSON(run))
	}
	if calls != 1 {
		t.Fatalf("first cut must execute exactly one new generator, got %d", calls)
	}
	t.Logf("first-cut elapsed_ms=%.3f support-read=%v", float64(time.Since(start))/float64(time.Millisecond), frameTimes.SnapshotMilliseconds())
	r.PreviewID = preview.PreviewID
	apply(r)
	readRuntime := func() workspace.EvaluationRuntime {
		t.Helper()
		var raw []byte
		if e := db.QueryRow(ctx, `SELECT evaluation_manifest FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var m struct {
			Runtime workspace.EvaluationRuntime `json:"runtime"`
		}
		if e := json.Unmarshal(raw, &m); e != nil {
			t.Fatal(e)
		}
		return m.Runtime
	}
	if runtime := readRuntime(); len(runtime.WorkerRuns) != 0 {
		t.Fatal("preview commit repeated dense Body evaluation", mustRuntimeJSON(runtime))
	}
	feature := view.Part.Features[len(view.Part.Features)-1]
	feature.Length = 9
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
	t.Logf("different-depth elapsed_ms=%.3f runtime=%s", float64(lastElapsed)/float64(time.Millisecond), mustRuntimeJSON(readRuntime()))
	accepted := activeBodyArtifact(t, view)
	if math.Abs(accepted.Volume-(base.Volume-36)) > 1e-4 {
		t.Fatal("different-depth cut volume", accepted.Volume)
	}
	cold, e := service.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: prefix + "-cold", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("dense STEP cold replay", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != accepted.Representations[role].Digest {
			t.Fatalf("dense STEP cold %s mismatch", role)
		}
	}
}

func mustRuntimeJSON(value any) string { data, _ := json.Marshal(value); return string(data) }

// Hold the exact result at six faces while growing real Feature history. This
// separates scheduling/history cost from the dense STEP's increasing topology.
func TestLongPartChainWithFixedTopologyThroughRouter(t *testing.T) {
	service, _, db, _, pool := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Fixed topology long chain"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(request workspace.CommandRequest) time.Duration {
		t.Helper()
		sequence++
		request.ActorID, request.VersionID = p6Actor, view.Document.VersionID
		request.RequestID = fmt.Sprintf("%s-fixed-topology-%d", view.Document.ID, sequence)
		started := time.Now()
		view, err = service.ApplyCommand(ctx, view.Document.ID, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
		return time.Since(started)
	}
	readRun := func() *workerv1.PartRuntimeStats {
		t.Helper()
		var raw []byte
		if e := db.QueryRow(ctx, `SELECT evaluation_manifest FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var manifest struct {
			Runtime workspace.EvaluationRuntime `json:"runtime"`
		}
		if e := json.Unmarshal(raw, &manifest); e != nil {
			t.Fatal(e)
		}
		if len(manifest.Runtime.WorkerRuns) != 1 {
			t.Fatal("expected one Body execution", mustRuntimeJSON(manifest.Runtime))
		}
		return manifest.Runtime.WorkerRuns[0]
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 100, Y: 100}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 200})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	cutProfile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: cutProfile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 110, Y: 110}}}})
	checkCut := func(count int, volume float64) *workerv1.PartRuntimeStats {
		t.Helper()
		artifact := activeBodyArtifact(t, view)
		if math.Abs(artifact.Volume-volume) > 1e-5 || p6TopologyCount(t, artifact.Topology, "faces") != 6 || p6TopologyCount(t, artifact.Topology, "edges") != 12 || p6TopologyCount(t, artifact.Topology, "vertices") != 8 || !artifact.Naming.CanBind {
			t.Fatalf("cut %d changed fixed topology/volume: %+v", count, artifact)
		}
		run := readRun()
		if run.GeneratorCalls != 1 || run.StagesExecuted != 1 || run.StagesReused < uint32(count) {
			t.Fatal("long chain repeated its unchanged prefix", mustRuntimeJSON(run))
		}
		return run
	}
	for count := 1; count <= 100; count++ {
		elapsed := apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: cutProfile, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: float64(count)})
		run := checkCut(count, float64(200-count)*10000)
		if count == 1 || count == 20 || count == 50 || count == 100 {
			pool.mu.Lock()
			client := pool.workers[0].client
			pool.mu.Unlock()
			health, e := client.Ping(ctx, &workerv1.PingRequest{})
			if e != nil {
				t.Fatal(e)
			}
			t.Logf("fixed-topology cuts=%d features=%d elapsed_ms=%.3f rss_bytes=%d worker=%s", count, len(view.Part.Features), float64(elapsed)/float64(time.Millisecond), health.GetResidentMemoryBytes(), mustRuntimeJSON(run))
		}
	}
	tip := view.Part.Features[len(view.Part.Features)-1]
	tip.Length = 100.5
	elapsed := apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)})
	run := checkCut(100, 995000)
	t.Logf("fixed-topology tip-edit elapsed_ms=%.3f worker=%s", float64(elapsed)/float64(time.Millisecond), mustRuntimeJSON(run))
	accepted := activeBodyArtifact(t, view)
	cold, e := service.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-fixed-topology-cold", TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("long chain cold replay", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if accepted.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatalf("long chain cold %s differs", role)
		}
	}
}

func TestPartSketchEditingIsolationThroughRouter(t *testing.T) {
	service, _, db, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Sketch execution isolation"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-isolation-%d", view.Document.ID, seq)
		r.VersionID = view.Document.VersionID
		measured, recorder := perf.WithRecorder(ctx)
		started := time.Now()
		view, err = service.ApplyCommand(measured, view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
		t.Logf("operation=%s features=%d elapsed_ms=%.3f phases=%v", r.Type, len(view.Part.Features), float64(time.Since(started))/float64(time.Millisecond), recorder.SnapshotMilliseconds())
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 100, Y: 100}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	accepted := activeBodyArtifact(t, view)
	// Deliberate selection warms exact properties. A later request must retain
	// typed vectors and the immutable support frame through the preparation cache.
	properties, e := service.GetTopologyElementPropertiesAtVersion(ctx, view.Document.ID, view.Document.VersionID, accepted.GeometryKey, "FACE", 1)
	if e != nil {
		t.Fatal(e)
	}
	cached, e := service.GetTopologyElementPropertiesAtVersion(ctx, view.Document.ID, view.Document.VersionID, accepted.GeometryKey, "FACE", 1)
	if e != nil {
		t.Fatal(e)
	}
	if normal, ok := cached.Properties["normal"].([3]float64); !ok || normal != properties.Properties["normal"] {
		t.Fatal("cached exact frame lost typed vector", cached)
	}
	for i := 0; i < 20; i++ {
		if i == 0 {
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: accepted.GeometryKey, TopologyID: 1, BodyID: view.Part.ActiveBodyID})
		} else {
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		}
		id := view.Part.Features[len(view.Part.Features)-1].ID
		ops := []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: float64(i + 1), Y: 10}}}}
		preview, e := service.PreviewCommand(ctx, view.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", ActorID: p6Actor, RequestID: fmt.Sprintf("%s-preview-%d", view.Document.ID, i), VersionID: view.Document.VersionID, SketchID: id, Operations: ops})
		if e != nil {
			t.Fatal(e)
		}
		if len(preview.SketchCandidates) != 1 || preview.PreviewID == "" || preview.Artifact != nil || preview.Runtime == nil || len(preview.Runtime.WorkerRuns) != 0 {
			t.Fatalf("sketch preview entered Body display: %+v", preview)
		}
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: ops, PreviewID: preview.PreviewID})
		current := activeBodyArtifact(t, view)
		if current.GeometryKey != accepted.GeometryKey {
			t.Fatalf("unused sketch changed Body result: %s -> %s", accepted.GeometryKey, current.GeometryKey)
		}
		for _, role := range []string{"BREP", "NAMING", "VISUAL"} {
			if current.Representations[role] != accepted.Representations[role] {
				t.Fatalf("unused sketch replaced %s", role)
			}
		}
		var raw []byte
		if e = db.QueryRow(ctx, `SELECT evaluation_manifest FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var manifest struct {
			Runtime workspace.EvaluationRuntime `json:"runtime"`
		}
		if e = json.Unmarshal(raw, &manifest); e != nil {
			t.Fatal(e)
		}
		if len(manifest.Runtime.WorkerRuns) != 0 {
			t.Fatal("unused sketch invoked Body Worker", string(raw))
		}
		for key, node := range manifest.Runtime.Nodes {
			if !strings.HasPrefix(string(key), "parameter:") && key != modelcore.DependencyKey("feature:"+id) && node.Executions > 0 {
				t.Fatalf("unrelated preparation %s: %+v", key, node)
			}
		}
		if view.SketchVisualization == nil || len(view.SketchVisualization.Primitives) == 0 {
			t.Fatal("independent sketch display missing")
		}
	}
	// A real profile consumer still updates geometry and has the original gates.
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "hole", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 50, Y: 50}, Radius: 5}}}})
	if got := activeBodyArtifact(t, view); got.GeometryKey == accepted.GeometryKey || math.Abs(got.Volume-(200000-500*math.Pi)) > 1e-5 || !got.Naming.CanBind {
		t.Fatalf("profile consumers were skipped: %+v", got)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if got := activeBodyArtifact(t, view); math.Abs(got.Volume-accepted.Volume) > 1e-5 {
		t.Fatal("undo lost the Body", got.Volume)
	}
}

// Uses real Domain Commands, configured storage and the production Router.
// Execution counts are the regression contract; wall-clock capacity is not.
func TestPartFeatureChainIncrementalAndColdThroughRouter(t *testing.T) {
	service, _, db, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Part incremental feature chain"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		sequence++
		request.ActorID = p6Actor
		request.RequestID = fmt.Sprintf("%s-part-chain-%d", view.Document.ID, sequence)
		request.VersionID = view.Document.VersionID
		var e error
		view, e = service.ApplyCommand(ctx, view.Document.ID, request)
		if e != nil {
			t.Fatalf("%s: %v", request.Type, e)
		}
		for _, feature := range view.Part.Features {
			if feature.EvaluationStatus == "FAILED" || feature.EvaluationStatus == "BLOCKED" {
				t.Fatalf("%s: %s", feature.ID, feature.Diagnostic)
			}
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 100, Y: 100}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	var cuts []string
	for i := 0; i < 8; i++ {
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 5 + 10*float64(i), Y: 5}, Radius: 1}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: id, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 20})
		cuts = append(cuts, view.Part.Features[len(view.Part.Features)-1].ID)
	}
	checkVolume := func(want float64) {
		t.Helper()
		a := activeBodyArtifact(t, view)
		if math.Abs(a.Volume-want) > 1e-5 || !a.Naming.CanBind {
			t.Fatalf("volume/naming: %g != %g, %+v", a.Volume, want, a.Naming)
		}
	}
	checkRuns := func(minReused uint32, maxExecuted uint32) {
		t.Helper()
		var raw []byte
		if e := db.QueryRow(ctx, `SELECT evaluation_manifest FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var m struct {
			Runtime struct {
				WorkerRuns []*workerv1.PartRuntimeStats `json:"workerRuns"`
			} `json:"runtime"`
		}
		if e := json.Unmarshal(raw, &m); e != nil {
			t.Fatal(e)
		}
		found := false
		for _, r := range m.Runtime.WorkerRuns {
			if r.GeneratorCalls > 0 {
				found = true
				if r.StagesReused < minReused || r.StagesExecuted > maxExecuted {
					t.Fatalf("unrelated prefix recomputed: %+v", r)
				}
			}
		}
		if !found {
			t.Fatal("missing real Worker execution evidence")
		}
	}
	checkVolume(200000 - 8*20*math.Pi)
	checkRuns(8, 1)
	var edited workspace.Feature
	for _, f := range view.Part.Features {
		if f.ID == cuts[2] {
			edited = f
		}
	}
	edited.Length = 15
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: edited.ID, Feature: &edited, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, edited.ID)})
	checkVolume(200000 - (7*20+15)*math.Pi)
	checkRuns(3, 6)
	accepted := activeBodyArtifact(t, view)
	head := view.Document.VersionID
	cold, e := service.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-part-chain-cold", TargetID: edited.ID, Feature: &edited, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, edited.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatalf("cold replay: %v", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if accepted.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatalf("cold %s differs from incrementally evaluated result", role)
		}
	}
	if cold.Runtime == nil {
		t.Fatal("missing cold runtime")
	}
	for _, run := range cold.Runtime.WorkerRuns {
		if run.GeneratorCalls > 0 && run.StagesReused != 0 {
			t.Fatal("cold path borrowed a stage")
		}
	}
	reopened, e := service.GetDocument(ctx, view.Document.ID, p6Actor)
	if e != nil || reopened.Document.VersionID != head {
		t.Fatalf("preview changed Head: %v", e)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	checkVolume(200000 - 8*20*math.Pi)
	apply(workspace.CommandRequest{Type: "REDO"})
	checkVolume(200000 - (7*20+15)*math.Pi)
}
