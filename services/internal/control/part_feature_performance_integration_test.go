//go:build integration

package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Time the command boundary separately from fixture setup, measurement queries and
// response assertions. A preview used in these scenarios must be promotable.
func engineeringFeaturePreview(t *testing.T, service *workspace.Service, documentID string, req workspace.CommandRequest, operation string) workspace.CommandPreview {
	t.Helper()
	ctx, rec := perf.WithRecorder(t.Context())
	started := time.Now()
	preview, err := service.PreviewCommand(ctx, documentID, req)
	elapsed := time.Since(started)
	if err != nil || preview.PreviewID == "" || preview.Artifact == nil {
		t.Fatalf("%s preview: %v", operation, err)
	}
	t.Logf("engineering_preview=%s elapsed_ms=%.3f stages=%v", operation, float64(elapsed)/float64(time.Millisecond), rec.SnapshotMilliseconds())
	return preview
}

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
		timed, rec := perf.WithRecorder(ctx)
		started := time.Now()
		view, err = service.ApplyCommand(timed, view.Document.ID, r)
		lastElapsed = time.Since(started)
		t.Logf("step_command=%s elapsed_ms=%.3f stages=%v", r.Type, float64(lastElapsed)/float64(time.Millisecond), rec.SnapshotMilliseconds())
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
	preview := engineeringFeaturePreview(t, service, view.Document.ID, r, "step-cut")
	firstElapsed := time.Since(start)
	if preview.Artifact == nil || preview.Runtime == nil || math.Abs(measuredArtifactVolume(t, preview.Artifact)-(measuredArtifactVolume(t, base)-40)) > 1e-4 {
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
	t.Logf("first-cut elapsed_ms=%.3f support-read=%v", float64(firstElapsed)/float64(time.Millisecond), frameTimes.SnapshotMilliseconds())
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
	if math.Abs(measuredArtifactVolume(t, accepted)-(measuredArtifactVolume(t, base)-36)) > 1e-4 {
		t.Fatal("different-depth cut volume", measuredArtifactVolume(t, accepted))
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

// Distinct rectangle/circle bosses continuously add faces, edges and vertices.
// This is not a fixed-topology cut chain or a single pattern Feature.
func TestGrowingTopologyPartFeaturesAndNamingThroughRouter(t *testing.T) {
	count := 120
	if value := os.Getenv("OCCCCAD_TEST_NAMING_STEPS"); value != "" {
		var err error
		count, err = strconv.Atoi(value)
		if err != nil || count < 1 || count > 1000 {
			t.Fatal("invalid naming growth step count", value)
		}
	}
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	const fixtureName = "Growing topology and naming"
	resume := os.Getenv("OCCCCAD_TEST_NAMING_RESUME_DOCUMENT")
	var view workspace.DocumentView
	var err error
	if resume == "" {
		view, err = service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: fixtureName})
	} else {
		view, err = service.GetDocument(ctx, resume)
	}
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(request workspace.CommandRequest) time.Duration {
		t.Helper()
		sequence++
		request.ActorID = p6Actor
		request.VersionID = view.Document.VersionID
		request.RequestID = fmt.Sprintf("%s-growing-%d", view.Document.ID, sequence)
		start := time.Now()
		view, err = service.ApplyCommand(ctx, view.Document.ID, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
		return time.Since(start)
	}
	readRun := func() *workerv1.PartRuntimeStats {
		t.Helper()
		var raw []byte
		if err := db.QueryRow(ctx, `SELECT evaluation_manifest FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Runtime workspace.EvaluationRuntime `json:"runtime"`
		}
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		if len(manifest.Runtime.WorkerRuns) != 1 {
			t.Fatal("expected one Body run", mustRuntimeJSON(manifest.Runtime))
		}
		return manifest.Runtime.WorkerRuns[0]
	}
	height := max(72, 6+count/12*4)
	completed := 0
	expectedVolume := float64(56 * height * 3)
	if resume == "" {
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		baseProfile := view.Part.Features[0].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: baseProfile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 56, Y: float64(height)}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: baseProfile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 3})
	} else {
		if view.Document.Name != fixtureName || view.Part == nil || len(view.Part.Features) < 4 || len(view.Part.Features)%2 != 0 {
			t.Fatal("resume requires a completed growing-fixture step", resume)
		}
		completed = len(view.Part.Features)/2 - 1
		if completed >= count {
			t.Fatal("resume fixture is already beyond the requested starting stage", completed)
		}
		for step := 1; step <= completed; step++ {
			area := 1.0
			if step%2 == 1 {
				area = math.Pi * .45 * .45
			}
			expectedVolume += area * (4 + float64(step%9)*.1)
		}
		if math.Abs(measuredArtifactVolume(t, activeBodyArtifact(t, view))-expectedVolume) > 1e-5 {
			t.Fatal("resume fixture material volume differs from the requested recipe")
		}
		sequence = 3 + 3*completed
		t.Logf("resume at %d accepted steps from a fresh Router", completed)
	}
	sourceVersion := view.Document.VersionID
	var picks []p7TopologyPick
	for _, axis := range []int{0, 1, 2} {
		picks = append(picks, findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
			n, ok := p.Properties["normal"].([3]float64)
			return ok && n[axis] > .99
		}))
	}
	lastArea := 0.0
	for step := completed + 1; step <= count; step++ {
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		profile := view.Part.Features[len(view.Part.Features)-1].ID
		x, y := float64(2+(step%12)*4), float64(2+(step/12)*4)
		operations := []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + 1, Y: y + 1}}}
		lastArea = 1
		if step%2 == 1 {
			operations = []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: fmt.Sprintf("circle-%d", step), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: x + .5, Y: y + .5}, Radius: .45}}}
			lastArea = math.Pi * .45 * .45
		}
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: operations})
		length := 7 + float64(step%9)*.1
		expectedVolume += lastArea * (length - 3)
		elapsed := apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: length})
		artifact := activeBodyArtifact(t, view)
		run := readRun()
		if !artifact.Naming.CanBind || p6TopologyCount(t, artifact.Topology, "faces") < uint64(6+step*2) {
			t.Fatalf("growing solid %d invalid: %+v", step, artifact)
		}
		if run.StagesExecuted != 1 || run.StagesReused < uint32(step) {
			t.Fatal("growing feature repeated the prefix", step, mustRuntimeJSON(run))
		}
		if run.NamingBlocksComputed+run.NamingBlocksReused != uint64(step+1) || run.NamingBlockBytesWritten == 0 || run.NamingRootBytes != uint64(artifact.Representations["NAMING"].Size) {
			t.Fatal("invalid naming block accounting", step, mustRuntimeJSON(run))
		}
		if step == 20 || step == 50 || step == 100 || step == 500 || step == 1000 || step == count {
			if step >= 20 && (run.MeshReusedFaces == 0 || run.EdgeProfilesReused == 0) {
				t.Fatal("accepted BREP reload discarded every unaffected face mesh or edge profile", step, mustRuntimeJSON(run))
			}
			if math.Abs(measuredArtifactVolume(t, artifact)-expectedVolume) > 1e-5 {
				t.Fatal("sampled growing stage has different material volume", step)
			}
			t.Logf("growing-router steps=%d features=%d elapsed_ms=%.3f naming_bytes=%d topology=%v worker=%s", step, len(view.Part.Features), float64(elapsed)/float64(time.Millisecond), artifact.Representations["NAMING"].Size, artifact.Topology, mustRuntimeJSON(run))
		}
	}
	tip := view.Part.Features[len(view.Part.Features)-1]
	tip.Length += .25
	elapsed := apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)})
	run := readRun()
	if run.StagesExecuted != 1 || run.StagesReused < uint32(count) {
		t.Fatal("tip edit lost its predecessor", mustRuntimeJSON(run))
	}
	t.Logf("growing-router tip-edit steps=%d elapsed_ms=%.3f worker=%s", count, float64(elapsed)/float64(time.Millisecond), mustRuntimeJSON(run))
	edited := activeBodyArtifact(t, view)
	if math.Abs(measuredArtifactVolume(t, edited)-(expectedVolume+lastArea*.25)) > 1e-5 {
		t.Fatal("edited volume", measuredArtifactVolume(t, edited))
	}
	for _, pick := range picks {
		start := time.Now()
		request := workspace.ResolvePersistentSelectionRequest{Selection: pick.Selection, SourceVersionID: sourceVersion, TargetVersionID: view.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest}
		resolution, err := service.ResolvePersistentSelection(ctx, view.Document.ID, request)
		if err != nil || resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			t.Fatal("old face did not resolve", resolution, err)
		}
		t.Logf("growing-router selection elapsed_ms=%.3f anchor=%s", float64(time.Since(start))/float64(time.Millisecond), pick.Selection.Anchor.OutputSlot)
		request.ManifestDigest = "wrong"
		if rejected, err := service.ResolvePersistentSelection(ctx, view.Document.ID, request); err != nil || rejected.Status != modelcore.SelectionContractMismatch {
			t.Fatal("index bypassed target snapshot qualification", rejected, err)
		}
	}
	// Reopening has no parsed root/block/index cache. Resolve from the durable
	// dependency closure rather than reusing the evaluator's in-memory records.
	reopened := workspace.NewWithArtifacts(db, client, artifacts)
	for _, pick := range picks {
		start := time.Now()
		resolution, err := reopened.ResolvePersistentSelection(ctx, view.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: pick.Selection, SourceVersionID: sourceVersion, TargetVersionID: view.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		if err != nil || resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			t.Fatal("reopened naming closure did not resolve old face", resolution, err)
		}
		t.Logf("growing-router reopened-selection elapsed_ms=%.3f anchor=%s", float64(time.Since(start))/float64(time.Millisecond), pick.Selection.Anchor.OutputSlot)
	}
	root := edited.Representations["NAMING"]
	children, err := artifacts.NamingHistoryObjects(ctx, root.ObjectID, "")
	if err != nil || len(children) != count+1 {
		t.Fatal("incomplete durable naming closure", len(children), err)
	}
	_, reader, err := artifacts.Open(ctx, root.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	rootBytes, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	naming := &workerv1.PartTopologyManifest{}
	if err = proto.Unmarshal(rootBytes, naming); err != nil {
		t.Fatal(err)
	}
	if len(naming.History) != count+1 || run.NamingBlockBytesWritten != naming.History[count].SizeBytes {
		t.Fatal("tip edit republished durable prefix blocks", mustRuntimeJSON(run))
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if math.Abs(measuredArtifactVolume(t, activeBodyArtifact(t, view))-expectedVolume) > 1e-5 {
		t.Fatal("undo changed the accepted shape")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	accepted := activeBodyArtifact(t, view)
	revision := view.Document.VersionID
	cold, err := service.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-growing-cold", TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)})
	if err != nil || cold.Artifact == nil {
		t.Fatal("cold growing chain", err)
	}
	assertExactPartReplay(t, artifacts, accepted, *cold.Artifact)
	current, err := service.GetDocument(ctx, view.Document.ID)
	if err != nil || current.Document.VersionID != revision {
		t.Fatal("read-only cold preview moved Head", err)
	}
}

// OCCT can serialize the zero components of a restored circle's axis with the
// opposite sign. This changes the BREP hash and thus dependent Naming hashes.
// Compare the exact BREP tokens and every typed Naming table in that case.
func assertExactPartReplay(t *testing.T, artifacts *artifactstore.Service, accepted, cold workspace.Artifact) {
	t.Helper()
	sameBRep := accepted.Representations["BREP"].Digest == cold.Representations["BREP"].Digest
	if sameBRep && accepted.Representations["NAMING"].Digest == cold.Representations["NAMING"].Digest {
		return
	}
	ctx := t.Context()
	readArtifact := func(rep workspace.Representation) []byte { return readPartReplayArtifact(t, artifacts, rep) }
	if !sameBRep {
		left := bytes.Fields(readArtifact(accepted.Representations["BREP"]))
		right := bytes.Fields(readArtifact(cold.Representations["BREP"]))
		if len(left) != len(right) {
			t.Fatal("cold BREP token count differs")
		}
		isZero := func(token []byte) bool { return bytes.Equal(token, []byte("-0")) || bytes.Equal(token, []byte("0")) }
		for i, token := range left {
			if !bytes.Equal(token, right[i]) && (!isZero(token) || !isZero(right[i])) {
				t.Fatalf("cold BREP token %d differs: %s / %s", i, token, right[i])
			}
		}
	}
	normalize := func(m *workerv1.PartTopologyManifest) {
		m.GeometryId, m.BrepSha256 = "", ""
		for _, row := range m.Evidence {
			row.EvidenceDigest = ""
		}
		for _, transition := range m.Transitions {
			transition.InputGeometryId, transition.ResultGeometryId, transition.EvidenceDigest = "", "", ""
		}
	}
	var roots [2]*workerv1.PartTopologyManifest
	var histories [2][]*workerv1.NamingHistoryReference
	var objects [2]map[string]artifactstore.Object
	for i, artifact := range []workspace.Artifact{accepted, cold} {
		naming := artifact.Representations["NAMING"]
		root := &workerv1.PartTopologyManifest{}
		if err := proto.Unmarshal(readArtifact(naming), root); err != nil {
			t.Fatal(err)
		}
		if root.GeometryId != artifact.GeometryID || root.BrepSha256 != artifact.Representations["BREP"].Digest {
			t.Fatal("replay Naming is bound to a different BREP")
		}
		members, err := artifacts.NamingHistoryObjects(ctx, naming.ObjectID, "")
		if err != nil || len(members) != len(root.History) {
			t.Fatal("replay Naming closure is incomplete", err)
		}
		objects[i] = make(map[string]artifactstore.Object, len(members))
		for _, member := range members {
			objects[i][member.SHA256] = member
		}
		roots[i], histories[i] = root, root.History
		root.History = nil
		normalize(root)
	}
	if !proto.Equal(roots[0], roots[1]) || len(histories[0]) != len(histories[1]) {
		t.Fatal("cold Naming type, reference or tip locator differs")
	}
	sharedRefs := map[string]uint32{}
	var previousRows [2][]uint32
	differentCodec := false
	for n, ref := range histories[0] {
		if ref.FeatureId != histories[1][n].FeatureId || ref.FullLineageCount != histories[1][n].FullLineageCount {
			t.Fatalf("cold Naming history differs at stage %d", n+1)
		}
		var blocks [2]*workerv1.NamingHistoryBlock
		for i, history := range histories {
			ref := history[n]
			object := objects[i][ref.Sha256]
			block := &workerv1.NamingHistoryBlock{}
			if err := proto.Unmarshal(readArtifact(workspace.Representation{ObjectID: object.ID, Digest: ref.Sha256, Size: int64(ref.SizeBytes)}), block); err != nil {
				t.Fatal(err)
			}
			previous := ""
			if n > 0 {
				previous = history[n-1].Sha256
			}
			if block.PreviousSha256 != previous || block.GetValues().GetGeometryId() != ref.ResultGeometryId {
				t.Fatalf("replay Naming block integrity differs at stage %d", n+1)
			}
			block.PreviousSha256 = ""
			normalize(block.Values)
			previousRows[i] = canonicalReplayNamingRefs(t, block, previousRows[i], sharedRefs)
			blocks[i] = block
		}
		differentCodec = differentCodec || blocks[0].SchemaVersion != blocks[1].SchemaVersion
		blocks[0].SchemaVersion, blocks[1].SchemaVersion = 0, 0
		if !proto.Equal(blocks[0], blocks[1]) {
			t.Fatalf("cold Naming typed lineage or evidence differs at stage %d", n+1)
		}
	}
	if sameBRep && !differentCodec {
		t.Fatal("cold Naming digest differs for the same BREP and codec")
	}
	t.Logf("replay matched BREP (identical digest=%t) and %d typed Naming stages (different codec=%t)", sameBRep, len(histories[0]), differentCodec)
}

func readPartReplayArtifact(t *testing.T, artifacts *artifactstore.Service, rep workspace.Representation) []byte {
	t.Helper()
	object, reader, err := artifacts.Open(t.Context(), rep.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || int64(len(data)) != rep.Size ||
		object.SHA256 != rep.Digest || fmt.Sprintf("%x", sha256.Sum256(data)) != rep.Digest {
		t.Fatal("replay artifact failed its own integrity check", rep.ObjectID, err, closeErr)
	}
	return data
}

// The replay oracle compares typed references and ordered adjacency across
// codecs. Temporary ordinals represent exact decoded tuples on both sides.
func canonicalReplayNamingRefs(t *testing.T, block *workerv1.NamingHistoryBlock, previous []uint32, shared map[string]uint32) []uint32 {
	t.Helper()
	values := block.Values
	if (block.SchemaVersion != 1 && block.SchemaVersion != 2) || uint64(len(previous)) != block.PreviousLineageCount {
		t.Fatal("invalid replay block codec or predecessor")
	}
	intern := func(ref *workerv1.SemanticTopologyRef) uint32 {
		raw, err := proto.Marshal(ref)
		if err != nil {
			t.Fatal(err)
		}
		key := string(raw)
		if shared[key] == 0 {
			shared[key] = uint32(len(shared) + 1)
		}
		return shared[key]
	}
	var local []uint32
	if dictionary := values.ReferenceDictionary; dictionary != nil {
		symbol := func(id uint32) string { return dictionary.Symbols[id-1] }
		sourceLists := make([][]string, len(dictionary.SourceLists)+1)
		for i, list := range dictionary.SourceLists {
			for _, id := range list.Symbols {
				sourceLists[i+1] = append(sourceLists[i+1], symbol(id))
			}
		}
		local = make([]uint32, len(dictionary.Rows))
		for i, row := range dictionary.Rows {
			local[i] = intern(&workerv1.SemanticTopologyRef{FeatureId: symbol(row.Feature), OutputSlot: symbol(row.Slot), SourceIds: sourceLists[row.Sources]})
		}
	} else {
		for _, ref := range values.SemanticRefs {
			local = append(local, intern(ref))
		}
	}
	lookup := func(ids []uint32, id uint32) uint32 {
		if id == 0 || uint64(id) > uint64(len(ids)) {
			t.Fatalf("invalid replay reference %d of %d", id, len(ids))
		}
		return ids[id-1]
	}
	remap := func(ids []uint32) {
		for i, id := range ids {
			ids[i] = lookup(local, id)
		}
	}
	for _, evidence := range values.Evidence {
		for i, token := range evidence.Adjacent {
			switch {
			case block.SchemaVersion == 1:
				evidence.Adjacent[i] = lookup(local, token)
			case token&1 == 0:
				evidence.Adjacent[i] = lookup(local, token>>1)
			default:
				evidence.Adjacent[i] = lookup(previous, token>>1)
			}
		}
	}
	values.ReferenceDictionary, values.SemanticRefs = nil, nil
	stage := values.Transitions[0]
	literals := make([]uint32, len(stage.Lineage))
	for i, row := range stage.Lineage {
		row.Result = lookup(local, row.Result)
		literals[i] = row.Result
		remap(row.Sources)
	}
	for _, row := range stage.Deleted {
		row.Source = lookup(local, row.Source)
	}
	for _, row := range stage.Ambiguous {
		remap(row.Sources)
		remap(row.Candidates)
	}
	expanded := make([]uint32, 0, block.FullLineageCount)
	for _, span := range block.Layout {
		rows := literals
		if span.Carry {
			rows = previous
		}
		if span.Count == 0 || span.Offset > uint64(len(rows)) || span.Count > uint64(len(rows))-span.Offset {
			t.Fatal("invalid replay lineage layout")
		}
		expanded = append(expanded, rows[span.Offset:span.Offset+span.Count]...)
	}
	if uint64(len(expanded)) != block.FullLineageCount {
		t.Fatal("incomplete replay lineage layout")
	}
	return expanded
}

// Optional cross-build replay uses an existing development document so that
// Feature identities, input BREP and stored artifact bytes are identical. The
// ordinary growth scenario above also checks cached versus cold evaluation.
func TestStoredGrowingTopologyNamingReplayThroughRouter(t *testing.T) {
	documentID := os.Getenv("OCCCCAD_TEST_NAMING_REPLAY_DOCUMENT")
	if documentID == "" {
		t.Skip("requires a frozen growing-topology document from a prior build")
	}
	service, artifacts, _, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		t.Fatal(err)
	}
	accepted := activeBodyArtifact(t, view)
	tip := view.Part.Features[len(view.Part.Features)-1]
	coldCtx, timings := perf.WithRecorder(workspace.WithColdEvaluation(ctx))
	start := time.Now()
	cold, err := service.PreviewCommand(coldCtx, documentID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: documentID + "-cross-build-replay", TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)})
	elapsed := time.Since(start)
	if err != nil || cold.Artifact == nil {
		t.Fatal("cold stored chain", err)
	}
	assertExactPartReplay(t, artifacts, accepted, *cold.Artifact)
	current, err := service.GetDocument(ctx, documentID)
	if err != nil || current.Document.VersionID != view.Document.VersionID {
		t.Fatal("cross-build preview moved Head", err)
	}
	if cold.Runtime == nil || len(cold.Runtime.WorkerRuns) != 1 {
		t.Fatal("missing cold Worker run")
	}
	run := cold.Runtime.WorkerRuns[0]
	if run.StagesExecuted == 0 || run.StagesReused != 0 || run.FrozenPrefixStages != 0 {
		t.Fatal("cold replay reused a stage", mustRuntimeJSON(run))
	}
	t.Logf("stored-growing-replay features=%d elapsed_ms=%.3f stages=%v executed=%d blocks=%d brep=%s naming=%s visual=%s exact_geometry_and_typed_naming_unchanged=true",
		len(view.Part.Features), float64(elapsed)/float64(time.Millisecond), timings.SnapshotMilliseconds(), run.StagesExecuted, run.NamingBlocksComputed, cold.Artifact.Representations["BREP"].Digest,
		cold.Artifact.Representations["NAMING"].Digest, cold.Artifact.Representations["VISUAL"].Digest)
}

func TestStoredFixedPartContinuationThroughFreshRouter(t *testing.T) {
	id := os.Getenv("OCCCCAD_TEST_FIXED_REPLAY_DOCUMENT")
	if id == "" {
		t.Skip("requires an accepted fixed-topology performance fixture")
	}
	service, artifacts, _, _, _ := featureAssociationRuntimeService(t)
	ctx, timings := perf.WithRecorder(t.Context())
	view, err := service.GetDocument(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	previous := activeBodyArtifact(t, view)
	tip := view.Part.Features[len(view.Part.Features)-1]
	tip.Length += .125
	req := workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: id + "-restart-" + strconv.FormatInt(time.Now().UnixNano(), 10), TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)}
	start := time.Now()
	preview, err := service.PreviewCommand(ctx, id, req)
	elapsed := time.Since(start)
	if err != nil || preview.Artifact == nil || preview.Runtime == nil || len(preview.Runtime.WorkerRuns) != 1 {
		t.Fatal("restart preview unavailable", err)
	}
	requestTimes := timings.SnapshotMilliseconds()
	run := preview.Runtime.WorkerRuns[0]
	if run.FrozenPrefixStages == 0 || run.StagesExecuted != 1 || preview.Artifact.GeometryID == previous.GeometryID {
		t.Fatal("fresh worker failed to restore the accepted predecessor", mustRuntimeJSON(run))
	}
	coldRequest := req
	coldRequest.RequestID += "-fully-cold"
	start = time.Now()
	cold, err := service.PreviewCommand(workspace.WithColdEvaluation(t.Context()), id, coldRequest)
	coldElapsed := time.Since(start)
	if err != nil || cold.Artifact == nil {
		t.Fatal("full-chain preview unavailable", err)
	}
	assertExactPartReplay(t, artifacts, *preview.Artifact, *cold.Artifact)
	warmNaming, coldNaming := preview.Artifact.Representations["NAMING"].Digest, cold.Artifact.Representations["NAMING"].Digest
	warmVisual, coldVisual := preview.Artifact.Representations["VISUAL"], cold.Artifact.Representations["VISUAL"]
	if warmVisual.Digest != coldVisual.Digest {
		// Old immutable prefix blocks produce different artifact digests even
		// when every typed Naming value is identical. The display still has
		// to match byte-for-byte, except for its verified geometry association.
		warm, cold := readPartReplayArtifact(t, artifacts, warmVisual), readPartReplayArtifact(t, artifacts, coldVisual)
		binding := func(digest string) []byte { return []byte(`"namingDigest":"` + digest + `"`) }
		if bytes.Count(warm, binding(warmNaming)) != 1 || bytes.Count(cold, binding(coldNaming)) != 1 ||
			!bytes.Equal(bytes.Replace(warm, binding(warmNaming), binding(coldNaming), 1), cold) {
			t.Fatal("checkpoint display differs from fully cold evaluation")
		}
	}
	commit, commitTimes := perf.WithRecorder(t.Context())
	req.PreviewID = preview.PreviewID
	start = time.Now()
	accepted, err := service.ApplyCommand(commit, id, req)
	commitElapsed := time.Since(start)
	if err != nil || activeBodyArtifact(t, accepted).GeometryID != preview.Artifact.GeometryID {
		t.Fatal("could not promote fresh-worker preview", err)
	}
	t.Logf("fresh-router preview_ms=%.3f fully_cold_ms=%.3f commit_ms=%.3f preview=%v commit=%v worker=%s", float64(elapsed)/float64(time.Millisecond), float64(coldElapsed)/float64(time.Millisecond), float64(commitElapsed)/float64(time.Millisecond), requestTimes, commitTimes.SnapshotMilliseconds(), mustRuntimeJSON(run))
}

func TestMissingPartContinuationRecoversThroughRouter(t *testing.T) {
	service, artifacts, db, _, pool := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Missing Body checkpoint"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		sequence++
		request.ActorID, request.VersionID = p6Actor, view.Document.VersionID
		request.RequestID = fmt.Sprintf("%s-missing-checkpoint-%d", view.Document.ID, sequence)
		view, err = service.ApplyCommand(ctx, view.Document.ID, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
	}
	rectangle := func(from, to float64) workspace.SketchOperation {
		return workspace.SketchOperation{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: from, Y: from}, Second: &workspace.SketchPoint2{X: to, Y: to}}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	base := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: base, Operations: []workspace.SketchOperation{rectangle(0, 12)}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: base, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 2})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{rectangle(1, 3)}})

	// Intercept only the derived checkpoint reference. The real Router and
	// Worker observe the missing file, without changing any persisted object.
	var rejected bool
	client, err := openDevelopmentGeometry(t, pool, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, request, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		evaluate, ok := request.(*workerv1.EvaluatePartRequest)
		if ok && evaluate.Continuation != nil {
			fault := proto.CloneOf(evaluate)
			fault.BaseBrepArtifact.ObjectKey = artifactstore.StagingKey(view.Document.ID+"/missing-checkpoint", "shape.brep")
			err := invoke(ctx, method, fault, reply, conn, options...)
			rejected = status.Code(err) == codes.FailedPrecondition
			return err
		}
		return invoke(ctx, method, request, reply, conn, options...)
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	reopened := workspace.NewWithArtifacts(db, client, artifacts)
	request := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-checkpoint-unavailable", VersionID: view.Document.VersionID, SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 3}
	preview, err := reopened.PreviewCommand(ctx, view.Document.ID, request)
	if err != nil || preview.Artifact == nil || preview.Runtime == nil || len(preview.Runtime.WorkerRuns) != 1 || !rejected {
		t.Fatal("missing checkpoint did not recover through the original evaluator", rejected, err)
	}
	if run := preview.Runtime.WorkerRuns[0]; run.FrozenPrefixStages != 0 || run.StagesExecuted+run.StagesReused != 2 {
		t.Fatal("recovery did not evaluate the complete chain", mustRuntimeJSON(run))
	}
	cold, err := reopened.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID, request)
	if err != nil || cold.Artifact == nil || cold.Runtime == nil || len(cold.Runtime.WorkerRuns) != 1 || cold.Runtime.WorkerRuns[0].StagesExecuted != 2 {
		t.Fatal("missing checkpoint cold comparison", err)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if preview.Artifact.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatalf("recovery %s differs from cold evaluation", role)
		}
	}
}

// Reproduce three same-predecessor previews without mutating the frozen Part.
// Supply a new starting offset for uncached timings on a previously used fixture.
func TestStoredNamingTipPreviewTimingThroughRouter(t *testing.T) {
	id := os.Getenv("OCCCCAD_TEST_NAMING_TIMING_DOCUMENT")
	if id == "" {
		t.Skip("requires an accepted growing-topology fixture")
	}
	delta := .1875
	if value := os.Getenv("OCCCCAD_TEST_NAMING_OFFSET_MM"); value != "" {
		var err error
		delta, err = strconv.ParseFloat(value, 64)
		if err != nil || delta <= 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
			t.Fatal("invalid feature length increment", value)
		}
	}
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	view, err := service.GetDocument(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	original := view.Part.Features[len(view.Part.Features)-1]
	for i := 0; i < 3; i++ {
		tip := original
		add := delta + .375*float64(i)
		tip.Length += add
		ctx, rec := perf.WithRecorder(t.Context())
		start := time.Now()
		candidate, err := service.PreviewCommand(ctx, id, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: fmt.Sprintf("%s-naming-timing-%d", id, start.UnixNano()), TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)})
		elapsed := time.Since(start)
		if err != nil || candidate.Artifact == nil || candidate.Runtime == nil {
			t.Fatal("tip preview", err)
		}
		if len(candidate.Runtime.WorkerRuns) == 0 {
			t.Fatalf("delta %.9g mm already has an accepted artifact; use a fresh OCCCCAD_TEST_NAMING_OFFSET_MM for an uncached measurement", add)
		}
		run := candidate.Runtime.WorkerRuns[0]
		if len(candidate.Runtime.WorkerRuns) != 1 || run.StagesExecuted != 1 || run.FrozenPrefixStages == 0 || run.NamingBlocksComputed != 1 {
			t.Fatal("expected one suffix stage", mustRuntimeJSON(run))
		}
		rep := candidate.Artifact.Representations
		t.Logf("tip_work delta_mm=%.9g base_ms=%.3f input_prepare_ms=%.3f naming_kernel_ms=%.3f naming_pack_ms=%.3f adjacency_ms=%.3f face_evidence_reused=%d face_evidence_computed=%d rows_carried=%d rows_literal=%d block_bytes=%d root_bytes=%d display_ms=%.3f mesh_ms=%.3f faces_reused=%d faces_computed=%d profiles_reused=%d profiles_computed=%d glb_bytes=%d peak_rss=%d",
			add, run.BaseMs, run.InputPrepareMs, run.NamingKernelMs, run.NamingArtifactMs, run.AdjacencyMs, run.FaceEvidenceReused, run.FaceEvidenceComputed, run.NamingCarriedLineageRows, run.NamingLiteralLineageRows, run.NamingBlockBytesWritten, run.NamingRootBytes, run.DisplayMs, run.MeshMs, run.MeshReusedFaces, run.MeshComputedFaces, run.EdgeProfilesReused, run.EdgeProfilesComputed, rep["VISUAL"].Size, run.PeakRssBytes)
		t.Logf("delta_mm=%.9g exact_ms=%.3f boolean_ms=%.3f witness_ms=%.3f brep_check_ms=%.3f solid_material_ms=%.3f same_domain_ms=%.3f snapshot_ms=%.3f correspondence_ms=%.3f", add, run.ExactMs, run.BooleanBuildMs, run.MaterialWitnessMs, run.BrepCheckMs, run.SolidMaterialMs, run.SameDomainMs, run.BrepSnapshotMs, run.CorrespondenceMs)
		t.Logf("delta_mm=%.9g elapsed_ms=%.3f stages=%v prefix=%d brep=%s naming=%s visual=%s", add, float64(elapsed)/float64(time.Millisecond), rec.SnapshotMilliseconds(), run.FrozenPrefixStages, rep["BREP"].Digest, rep["NAMING"].Digest, rep["VISUAL"].Digest)
	}
	current, err := service.GetDocument(t.Context(), id)
	if err != nil || current.Document.VersionID != view.Document.VersionID {
		t.Fatal("preview changed Head", err)
	}
}

func TestStoredGrowingNamingQueriesThroughRouter(t *testing.T) {
	id := os.Getenv("OCCCCAD_TEST_NAMING_REPLAY_DOCUMENT")
	if id == "" {
		t.Skip("requires an accepted growing-topology fixture")
	}
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.GetDocument(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var sourceVersion string
	var raw []byte
	if err = db.QueryRow(ctx, `SELECT id::text,model_json FROM occccad.document_versions WHERE document_id=$1 AND sequence=4`, id).Scan(&sourceVersion, &raw); err != nil {
		t.Fatal(err)
	}
	var source workspace.PartModel
	if err = json.Unmarshal(raw, &source); err != nil || len(source.Bodies) != 1 || source.Bodies[0].GeometryKey == "" {
		t.Fatal("missing starting solid stage", err)
	}
	for _, localID := range []uint64{1, 2, 3} {
		selection, err := service.BindPersistentSelection(ctx, id, workspace.BindPersistentSelectionRequest{SourceVersionID: sourceVersion, BodyID: source.Bodies[0].ID, GeometryKey: source.Bodies[0].GeometryKey, Kind: "FACE", LocalID: localID})
		if err != nil {
			t.Fatal(err)
		}
		query, recorder := perf.WithRecorder(ctx)
		start := time.Now()
		resolved, err := service.ResolvePersistentSelection(query, id, workspace.ResolvePersistentSelectionRequest{Selection: selection, SourceVersionID: sourceVersion, TargetVersionID: view.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		elapsed := time.Since(start)
		if err != nil || resolved.Status != modelcore.SelectionResolved || len(resolved.Candidates) != 1 {
			t.Fatal("historical face did not resolve", resolved, err)
		}
		t.Logf("stored-growing-query features=%d local_id=%d elapsed_ms=%.3f", len(view.Part.Features), localID, float64(elapsed)/float64(time.Millisecond))
		if localID > 1 && len(view.Part.Features) <= 2002 {
			if _, rebuilt := recorder.SnapshotMilliseconds()["naming-view-decode"]; rebuilt {
				t.Fatal("decoded the same frozen root again for another early face", localID)
			}
		}
	}
	// Rebind an old survivor on the accepted predecessor (before the final
	// sketch and solid). A fresh service must reach the same current member.
	var predecessorVersion string
	if err = db.QueryRow(ctx, `SELECT id::text, model_json FROM occccad.document_versions WHERE document_id=$1 AND sequence=(SELECT MAX(sequence) FROM occccad.document_versions WHERE document_id=$1)-4`, id).Scan(&predecessorVersion, &raw); err != nil {
		t.Fatal(err)
	}
	var previous workspace.PartModel
	if err = json.Unmarshal(raw, &previous); err != nil || len(previous.Bodies) != 1 {
		t.Fatal("missing predecessor", err)
	}
	first, err := service.BindPersistentSelection(ctx, id, workspace.BindPersistentSelectionRequest{SourceVersionID: sourceVersion, BodyID: source.Bodies[0].ID, GeometryKey: source.Bodies[0].GeometryKey, Kind: "FACE", LocalID: 1})
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(s *workspace.Service, selection modelcore.PersistentSelection, from, to string) modelcore.ResolvedTopologyElement {
		t.Helper()
		r, err := s.ResolvePersistentSelection(ctx, id, workspace.ResolvePersistentSelectionRequest{Selection: selection, SourceVersionID: from, TargetVersionID: to, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		if err != nil || r.Status != modelcore.SelectionResolved || len(r.Candidates) != 1 {
			t.Fatal(r, err)
		}
		return r.Candidates[0]
	}
	old := lookup(service, first, sourceVersion, predecessorVersion)
	picked, err := service.BindPersistentSelection(ctx, id, workspace.BindPersistentSelectionRequest{SourceVersionID: predecessorVersion, BodyID: previous.Bodies[0].ID, GeometryKey: old.GeometryKey, Kind: "FACE", LocalID: old.LocalID})
	if err != nil {
		t.Fatal(err)
	}
	want := lookup(service, first, sourceVersion, view.Document.VersionID)
	reopened := workspace.NewWithArtifacts(db, client, artifacts)
	start := time.Now()
	got := lookup(reopened, picked, predecessorVersion, view.Document.VersionID)
	elapsed := time.Since(start)
	if got.LocalID != want.LocalID || got.GeometryKey != want.GeometryKey || got.Evidence.EvidenceDigest != want.Evidence.EvidenceDigest {
		t.Fatal("adjacent revision rebound to a different face", got, want)
	}
	t.Logf("stored-adjacent-query features=%d source_features=%d elapsed_ms=%.3f", len(view.Part.Features), len(previous.Features), float64(elapsed)/float64(time.Millisecond))

}

// Hold the exact result at six faces while growing real Feature history. This
// separates scheduling/history cost from the dense STEP's increasing topology.
func TestLongPartChainWithFixedTopologyThroughRouter(t *testing.T) {
	steps := 100
	if value := os.Getenv("OCCCCAD_TEST_FIXED_SOLID_STEPS"); value != "" {
		var err error
		steps, err = strconv.Atoi(value)
		if err != nil || steps < 100 || steps > 1000 {
			t.Fatal("invalid fixed topology step count", value)
		}
	}
	height := max(200, steps+100)
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
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: float64(height)})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	cutProfile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: cutProfile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 110, Y: 110}}}})
	checkCut := func(count int, volume float64) *workerv1.PartRuntimeStats {
		t.Helper()
		artifact := activeBodyArtifact(t, view)
		if math.Abs(measuredArtifactVolume(t, artifact)-volume) > 1e-5 || p6TopologyCount(t, artifact.Topology, "faces") != 6 || p6TopologyCount(t, artifact.Topology, "edges") != 12 || p6TopologyCount(t, artifact.Topology, "vertices") != 8 || !artifact.Naming.CanBind {
			t.Fatalf("cut %d changed fixed topology/volume: %+v", count, artifact)
		}
		run := readRun()
		if run.GeneratorCalls != 1 || run.StagesExecuted != 1 || run.StagesReused < uint32(count) {
			t.Fatal("long chain repeated its unchanged prefix", mustRuntimeJSON(run))
		}
		return run
	}
	for count := 1; count <= steps; count++ {
		elapsed := apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: cutProfile, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: float64(count)})
		if count == 1 || count == 20 || count == 50 || count == 100 || count == 500 || count == 1000 || count == steps {
			run := checkCut(count, float64(height-count)*10000)
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
	tip.Length = float64(steps) + .5
	elapsed := apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: tip.ID, Feature: &tip, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)})
	run := checkCut(steps, (float64(height-steps)-.5)*10000)
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
	var originalSupport workspace.SketchSupport
	for i := 0; i < 20; i++ {
		if i == 0 {
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: accepted.GeometryKey, TopologyID: 1, BodyID: view.Part.ActiveBodyID})
			originalSupport = view.Part.Features[len(view.Part.Features)-1].Sketch.Support
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
			ModelHash string                      `json:"modelHash"`
			Runtime   workspace.EvaluationRuntime `json:"runtime"`
		}
		if e = json.Unmarshal(raw, &manifest); e != nil {
			t.Fatal(e)
		}
		if preview.ModelHash == "" || preview.ModelHash != manifest.ModelHash {
			t.Fatal("sketch candidate digest differs from promoted model", preview.ModelHash, manifest.ModelHash)
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
	if got := activeBodyArtifact(t, view); got.GeometryKey == accepted.GeometryKey || math.Abs(measuredArtifactVolume(t, got)-(200000-500*math.Pi)) > 1e-5 || !got.Naming.CanBind {
		t.Fatalf("profile consumers were skipped: %+v", got)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if got := activeBodyArtifact(t, view); math.Abs(measuredArtifactVolume(t, got)-measuredArtifactVolume(t, accepted)) > 1e-5 {
		t.Fatal("undo lost the Body", measuredArtifactVolume(t, got))
	}
	if got := view.Part.Features[2].Sketch.Support.DependencySnapshot; got == nil || *got != *originalSupport.DependencySnapshot {
		t.Fatalf("restored exact source changed its frozen support: original=%+v restored=%+v", originalSupport.DependencySnapshot, got)
	}
	// Restoring an upstream shape can choose a different display artifact for
	// the same exact BREP/Naming. It must not invalidate earlier drawing history.
	for i := 0; i < 40; i++ {
		apply(workspace.CommandRequest{Type: "UNDO"})
	}
	if len(view.Part.Features) != 2 {
		t.Fatal("sketch creation/drawing compensation left extra features")
	}
	for i := 0; i < 40; i++ {
		apply(workspace.CommandRequest{Type: "REDO"})
	}
	if len(view.Part.Features) != 22 || math.Abs(measuredArtifactVolume(t, activeBodyArtifact(t, view))-measuredArtifactVolume(t, accepted)) > 1e-5 {
		t.Fatal("drawing replay lost the accepted Body or sketches")
	}
}

// Uses real Domain Commands, configured storage and the production Router.
// Execution counts are the regression contract; wall-clock capacity is not.
func TestPartFeatureChainIncrementalAndColdThroughRouter(t *testing.T) {
	service, artifacts, db, _, _ := featureAssociationRuntimeService(t)
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
		if math.Abs(measuredArtifactVolume(t, a)-want) > 1e-5 || !a.Naming.CanBind {
			t.Fatalf("volume/naming: %g != %g, %+v", measuredArtifactVolume(t, a), want, a.Naming)
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
	beforeEdit := view.Document.VersionID
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
	var owner string
	var stored, logical, previous, differences int
	if err := db.QueryRow(ctx, `SELECT COALESCE(dependency_edge_owner::text,'') FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&owner); err != nil || owner != beforeEdit {
		t.Fatalf("edited revision did not reuse the same dependency endpoints: %q / %q (%v)", owner, beforeEdit, err)
	}
	if err := db.QueryRow(ctx, `
SELECT (SELECT count(*) FROM occccad.dependency_edge_rows WHERE revision_id=$1),
       (SELECT count(*) FROM occccad.dependency_edges WHERE revision_id=$1),
       (SELECT count(*) FROM occccad.dependency_edges WHERE revision_id=$2),
       (SELECT count(*) FROM (
           SELECT source_key,target_key,edge_kind FROM occccad.dependency_edges WHERE revision_id=$2
           EXCEPT
           SELECT source_key,target_key,edge_kind FROM occccad.dependency_edges WHERE revision_id=$1
       ) AS difference)`, view.Document.VersionID, beforeEdit).Scan(&stored, &logical, &previous, &differences); err != nil ||
		stored != 0 || logical == 0 || logical != previous || differences != 0 {
		t.Fatalf("dependency projection changed: stored=%d logical=%d previous=%d differences=%d (%v)", stored, logical, previous, differences, err)
	}
	accepted := activeBodyArtifact(t, view)
	head := view.Document.VersionID
	cold, e := service.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-part-chain-cold", TargetID: edited.ID, Feature: &edited, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, edited.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatalf("cold replay: %v", e)
	}
	assertExactPartReplay(t, artifacts, accepted, *cold.Artifact)
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
