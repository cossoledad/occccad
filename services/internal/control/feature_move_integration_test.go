package control

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// Uses the production Router/Worker, persistence and the same commands as the tree UI.
func TestMoveFeatureLifecycleThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Constrained move lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-move-%d", v.Document.ID, seq)
		}
		next, e := service.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		v = next
	}
	saved := func(id string) workspace.Feature {
		t.Helper()
		for _, f := range v.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("missing", id)
		return workspace.Feature{}
	}
	edit := func(f workspace.Feature) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, f.ID)})
	}
	move := func(id, anchor, placement string) workspace.CommandRequest {
		seq++
		return workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-intent-%d", v.Document.ID, seq), Type: "MOVE_FEATURE", TargetID: id, AnchorFeatureID: anchor, Placement: placement, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, id)}
	}
	ids := func() []string {
		var a []string
		for i, f := range v.Part.Features {
			if f.Order != i+1 {
				t.Fatal("noncanonical Order")
			}
			a = append(a, f.ID)
		}
		return a
	}
	rejected := func(r workspace.CommandRequest, code string) {
		t.Helper()
		head := v.Document.VersionID
		if _, e := service.PreviewCommand(t.Context(), v.Document.ID, r); e == nil || !strings.Contains(e.Error(), code) {
			t.Fatal("preview rejection", code, e)
		}
		if _, e := service.ApplyCommand(t.Context(), v.Document.ID, r); e == nil || !strings.Contains(e.Error(), code) {
			t.Fatal("commit rejection", code, e)
		}
		now, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
		if e != nil || now.Document.VersionID != head {
			t.Fatal("rejection wrote Head", e)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := v.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: v.SketchAnalyses[profile].RegionIDs}})
	base := v.Part.Features[len(v.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID})
	location := v.Part.Features[len(v.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "center", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
	edge := findP7TopologyPick(t, service, v, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == base.ID && strings.HasPrefix(s.Anchor.OutputSlot, "VERTICAL_FROM_PROFILE_ENDPOINTS/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
	round := v.Part.Features[len(v.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: base.BodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 5, Reversed: true, HolePoint: &workspace.PatternPointReference{SketchID: location, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "center", SubElement: "POINT"}}}})
	hole := v.Part.Features[len(v.Part.Features)-1]
	original := ids()
	params := append([]modelcore.ParameterDefinition(nil), v.Part.Parameters...)
	originalGeometry := activeBodyArtifact(t, v)
	r := move(hole.ID, round.ID, "BEFORE")
	head := v.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), v.Document.ID, r)
	if e != nil || preview.Artifact == nil {
		t.Fatal("move preview", e)
	}
	now, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || now.Document.VersionID != head {
		t.Fatal("cancel changed Head", e)
	}
	stale := r
	stale.AnchorFeatureID = location
	stale.Placement = "AFTER"
	stale.PreviewID = preview.PreviewID
	if _, e = service.ApplyCommand(t.Context(), v.Document.ID, stale); e == nil {
		t.Fatal("changed intent reused preview")
	}
	r.PreviewID = preview.PreviewID
	apply(r)
	first := ids()
	movedGeometry := activeBodyArtifact(t, v)
	if !slices.Equal(first, []string{profile, base.ID, location, hole.ID, round.ID}) || !reflect.DeepEqual(params, v.Part.Parameters) || math.Abs(originalGeometry.Volume-movedGeometry.Volume) > 1e-6 {
		t.Fatal("move changed identities, parameters or final volume")
	}
	if saved(hole.ID).Name != hole.Name || saved(hole.ID).BodyID != hole.BodyID || !reflect.DeepEqual(saved(hole.ID).HolePoint, hole.HolePoint) || !reflect.DeepEqual(saved(round.ID).Selections, round.Selections) {
		t.Fatal("definitions rebound")
	}
	retry, e := service.ApplyCommand(t.Context(), v.Document.ID, r)
	if e != nil || retry.Document.VersionID != v.Document.VersionID {
		t.Fatal("idempotent commit", e)
	}
	stage, e := service.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: hole.ID, ResultStage: true})
	if e != nil || math.Abs(stage.Artifact.Volume-(8000-20*math.Pi)) > 1e-5 {
		t.Fatal("Hole stage still included fillet", e, stage.Artifact.Volume)
	}
	apply(move(hole.ID, round.ID, "AFTER"))
	second := ids()
	if !slices.Equal(second, original) {
		t.Fatal("second move")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if !slices.Equal(ids(), first) {
		t.Fatal("undo second")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if !slices.Equal(ids(), original) {
		t.Fatal("undo first")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if !slices.Equal(ids(), first) {
		t.Fatal("redo first")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if !slices.Equal(ids(), second) {
		t.Fatal("redo second")
	}
	changed := saved(hole.ID)
	changed.Diameter = 6
	edit(changed)
	changed = saved(round.ID)
	changed.Length = 1.3
	edit(changed)
	if saved(hole.ID).EvaluationStatus != "" || saved(round.ID).EvaluationStatus != "" {
		t.Fatal("post-move editing failed")
	}
	// Local modifications use the same single-item move, then retain editable inputs.
	apply(move(round.ID, hole.ID, "AFTER"))
	changed = saved(round.ID)
	changed.Length = 1.2
	edit(changed)
	apply(move(round.ID, hole.ID, "BEFORE"))
	// Reopen and compare an identical candidate with incremental reuse disabled.
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	opened, e := fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || !reflect.DeepEqual(opened.Part.Features, v.Part.Features) {
		t.Fatal("reopen", e)
	}
	probe := move(hole.ID, round.ID, "BEFORE")
	warm, e := service.PreviewCommand(t.Context(), v.Document.ID, probe)
	if e != nil {
		t.Fatal(e)
	}
	probe.RequestID += "-cold"
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, probe)
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if warm.Artifact.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("cold mismatch", role)
		}
	}
	// Semantic anchor and actual source stage are separate constraints.
	mouth := findP7TopologyPick(t, service, v, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == hole.ID && strings.HasPrefix(s.Anchor.OutputSlot, "START_BOUNDARY_FROM_PROFILE_EDGE/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: base.BodyID, Length: .2, Selections: []workspace.FeatureSelection{{Selection: mouth.Selection, SourceVersionID: mouth.SourceVersionID}}}})
	chamfer := v.Part.Features[len(v.Part.Features)-1]
	rejected(move(chamfer.ID, hole.ID, "BEFORE"), chamfer.ID)
	outer := findP7TopologyPick(t, service, v, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == base.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_BOUNDARY_FROM_PROFILE_EDGE/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: base.BodyID, Length: .15, Selections: []workspace.FeatureSelection{{Selection: outer.Selection, SourceVersionID: outer.SourceVersionID}}}})
	outerChamfer := v.Part.Features[len(v.Part.Features)-1]
	if outerChamfer.Selections[0].SourceFeatureID != "" {
		t.Fatal("test must exercise historical tip inference")
	}
	rejected(move(outerChamfer.ID, hole.ID, "BEFORE"), "SOURCE_STAGE "+chamfer.ID)
	changed = saved(outerChamfer.ID)
	changed.Selections[0].SourceFeatureID = chamfer.ID
	edit(changed)
	rejected(move(outerChamfer.ID, hole.ID, "BEFORE"), chamfer.ID)

	rejected(move(base.ID, hole.ID, "AFTER"), "fixed feature")
	// A real FEATURE_DELTA range remains ordered and closed under single-item moves.
	apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: base.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Direction: [3]float64{1, 0, 0}, Count: 2, Spacing: 30}, SourceKind: "FEATURE_DELTA", StartFeatureID: base.ID, Source: workspace.FeatureStageRef{BodyID: base.BodyID, FeatureID: round.ID}, ResultMode: "COMBINE"}}})
	pattern := v.Part.Features[len(v.Part.Features)-1]
	rejected(move(hole.ID, round.ID, "BEFORE"), "FEATURE_DELTA")
	rejected(move(outerChamfer.ID, pattern.ID, "AFTER"), "fixed feature cannot be crossed")
	rejected(move(pattern.ID, base.ID, "AFTER"), "fixed feature cannot move")
}

func TestMoveFeatureRejectsCrossBodyGeometryFailureThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Move cross Body stage"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-cross-%d", v.Document.ID, seq)
		next, e := service.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		v = next
	}
	rectangle := func(id string, a, b float64) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: a, Y: a}, Second: &workspace.SketchPoint2{X: b, Y: b}}}})
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := v.Part.Features[0].ID
	rectangle(profile, -10, 10)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	base := v.Part.Features[len(v.Part.Features)-1]
	bodyA := base.BodyID
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID})
	location := v.Part.Features[len(v.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "center", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	cutProfile := v.Part.Features[len(v.Part.Features)-1].ID
	rectangle(cutProfile, -4, 4)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: bodyA, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 5, Reversed: true, HolePoint: &workspace.PatternPointReference{SketchID: location, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "center", SubElement: "POINT"}}}})
	hole := v.Part.Features[len(v.Part.Features)-1]
	bottom := findP7TopologyPick(t, service, v, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == hole.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	bodyB := v.Part.ActiveBodyID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", BodyID: bodyB, GeometryKey: bottom.GeometryKey, TopologyID: bottom.LocalID, VersionID: v.Document.VersionID})
	consumer := v.Part.Features[len(v.Part.Features)-1]
	rectangle(consumer.ID, -.5, .5)
	if consumer.Sketch.Support.SourceFeatureID != hole.ID {
		t.Fatal("not bound to Hole result stage")
	}
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: bodyB, SketchID: consumer.ID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 2})
	child := v.Part.Features[len(v.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: bodyA})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: bodyA, SketchID: cutProfile, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Extent: "THROUGH_ALL", Length: 20})
	cut := v.Part.Features[len(v.Part.Features)-1]
	head := v.Document.VersionID
	for _, f := range v.Part.Features {
		if f.EvaluationStatus != "" {
			t.Fatal("fixture already failed", f.Name, f.Diagnostic)
		}
	}
	reject := func(id, anchor, placement, blocker string) {
		t.Helper()
		seq++
		r := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-rejected-%d", v.Document.ID, seq), Type: "MOVE_FEATURE", TargetID: id, AnchorFeatureID: anchor, Placement: placement, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, id)}
		if _, e := service.PreviewCommand(t.Context(), v.Document.ID, r); e == nil || !strings.Contains(e.Error(), blocker) {
			t.Fatal("preview accepted invalid consumer", blocker, e)
		}
		if _, e := service.ApplyCommand(t.Context(), v.Document.ID, r); e == nil || !strings.Contains(e.Error(), blocker) {
			t.Fatal("commit accepted invalid consumer", blocker, e)
		}
		now, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
		if e != nil || now.Document.VersionID != head || !reflect.DeepEqual(now.Part.Features, v.Part.Features) {
			t.Fatal("invalid move was not atomic", e)
		}
	}
	// All declared references precede consumers, but changing A's prefix before
	// the support sketch removes Hole's bottom face. A itself remains valid.
	reject(cut.ID, hole.ID, "AFTER", consumer.ID)
	// The dependency graph must also check the semantic anchor and explicit stage
	// of consumers in another Body, not just the moved Body's own inputs.
	reject(hole.ID, cut.ID, "AFTER", consumer.ID)
	reject(cut.ID, child.ID, "BEFORE", "anchor must belong to the same Body")
}
