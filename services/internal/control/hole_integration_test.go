package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestHoleLifecycleThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Hole lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-hole-%d", view.Document.ID, seq)
		}
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	saved := func(id string) workspace.Feature {
		t.Helper()
		for _, f := range view.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("missing Feature")
		return workspace.Feature{}
	}
	edit := func(f workspace.Feature) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	stockSketch := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: stockSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stockSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
	sourceBase := view.Part.Features[len(view.Part.Features)-1]
	sourceBody := view.Part.ActiveBodyID
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	targetSketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: targetSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: targetSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
	body := view.Part.ActiveBodyID
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Top", Origin: [3]float64{0, 0, 30}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	top := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
	apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: sourceBody})
	sourceArtifact := activeBodyArtifact(t, view)
	face := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == sourceBase.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: body})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", BodyID: body, GeometryKey: face.GeometryKey, TopologyID: face.LocalID, VersionID: view.Document.VersionID})
	location := view.Part.Features[len(view.Part.Features)-1].ID
	addPoint := func(id string, x float64) workspace.SketchOperation {
		return workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: x}}}
	}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{addPoint("p", 0), addPoint("q", 5)}})
	ref := func(id string) *workspace.PatternPointReference {
		return &workspace.PatternPointReference{SketchID: location, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}}
	}
	definition := workspace.Feature{Type: "HOLE", BodyID: body, Operation: "REMOVE", Extent: "BLIND", HolePoint: ref("p"), Diameter: 4, Depth: 10, Reversed: true}
	create := workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-preview", Type: "CREATE_SOLID_FEATURE", Feature: &definition, ParameterExpressions: map[string]string{"diameter": "2 * 2 mm", "depth": "5 mm + 5 mm"}}
	head := view.Document.VersionID
	count := len(view.Part.Features)
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, create)
	if e != nil || preview.Artifact == nil {
		t.Fatal("preview", e)
	}
	unchanged, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || unchanged.Document.VersionID != head {
		t.Fatal("preview/cancel wrote Head", e)
	}
	stale := create
	wrong := definition
	wrong.Diameter = 8
	stale.Feature = &wrong
	stale.ParameterExpressions = nil
	stale.PreviewID = preview.PreviewID
	if _, e = service.ApplyCommand(t.Context(), view.Document.ID, stale); e == nil {
		t.Fatal("stale preview accepted")
	}
	create.PreviewID = preview.PreviewID
	apply(create)
	hole := view.Part.Features[len(view.Part.Features)-1]
	if len(view.Part.Features) != count+1 || hole.Type != "HOLE" || hole.Profile != "" {
		t.Fatal("hidden modeling definitions created")
	}
	checkVolume := func(want float64) {
		t.Helper()
		input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: hole.ID, ResultStage: true})
		if e != nil || math.Abs(input.Artifact.Volume-want) > 1e-5 {
			t.Fatal("hole volume", input.Artifact.Volume, want, e)
		}
	}
	checkVolume(12000 - 40*math.Pi)
	params := func(slot string) modelcore.ParameterDefinition {
		t.Helper()
		for _, p := range view.Part.Parameters {
			if p.OwnerFeatureID == hole.ID && p.PropertySlot == slot {
				return p
			}
		}
		t.Fatal("parameter missing", slot)
		return modelcore.ParameterDefinition{}
	}
	diameterID, depthID := params("diameter").ParameterID, params("depth").ParameterID
	if params("diameter").Source.Expression == nil || params("depth").Source.Expression == nil {
		t.Fatal("expressions lost")
	}
	mouth := findP7TopologyPick(t, service, view, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == hole.ID && strings.HasPrefix(s.Anchor.OutputSlot, "START_BOUNDARY_FROM_PROFILE_EDGE/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: body, Length: .3, Selections: []workspace.FeatureSelection{{Selection: mouth.Selection, SourceVersionID: mouth.SourceVersionID}}}})
	chamfer := view.Part.Features[len(view.Part.Features)-1]
	changed := saved(hole.ID)
	changed.Diameter = 6
	changed.Depth = 8
	edit(changed)
	checkVolume(12000 - 72*math.Pi)
	if params("diameter").ParameterID != diameterID || params("depth").ParameterID != depthID || saved(chamfer.ID).EvaluationStatus != "" {
		t.Fatal("parameter or mouth identity lost")
	}
	for _, b := range view.Part.Bodies {
		if b.ID == sourceBody && view.Artifacts[b.GeometryKey].GeometryID != sourceArtifact.GeometryID {
			t.Fatal("Hole cut a different Body")
		}
	}
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: sourceBase.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, sourceBase.ID), Suppressed: &suppressed})
	if saved(location).Sketch.Support.Status != "FAILED_SUPPORT" || saved(hole.ID).EvaluationStatus != "BLOCKED" || saved(chamfer.ID).EvaluationStatus != "BLOCKED" {
		t.Fatal("support failure not isolated")
	}
	apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", SketchID: location, VersionID: view.Document.VersionID, DatumPlaneID: top.ID})
	if saved(hole.ID).EvaluationStatus != "" || saved(chamfer.ID).EvaluationStatus != "" {
		t.Fatal("support repair did not restore Hole")
	}
	changedChamfer := saved(chamfer.ID)
	changedChamfer.Length = .4
	edit(changedChamfer)
	value := 2.0
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "point-x", Kind: "HORIZONTAL_DISTANCE", Unit: "mm", Value: &value, References: []workspace.SketchGeometryRef{{Target: "SKETCH_ORIGIN", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "p", SubElement: "POINT"}}}}}})
	value = 3
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: "point-x", Value: &value}}})
	for _, entity := range saved(location).Sketch.Entities {
		if entity.ID == "p" && math.Abs(entity.Point.X-3) > 1e-6 {
			t.Fatal("Hole position did not consume solved dimension", entity.Point)
		}
	}
	if saved(chamfer.ID).EvaluationStatus != "" {
		t.Fatal("point move lost mouth reference")
	}
	pointStage, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: chamfer.ID})
	if e != nil {
		t.Fatal(e)
	}
	foundWall := false
	for local := uint64(1); local <= p6TopologyCount(t, pointStage.Artifact.Topology, "faces"); local++ {
		r, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: chamfer.ID, GeometryKey: pointStage.Artifact.GeometryKey, Kind: "FACE", LocalID: local})
		if e == nil && r.Selection.Selection.CreationEvidence.GeometryType == "CYLINDER" {
			foundWall = true
			if math.Abs(r.Selection.Selection.CreationEvidence.Origin[0]-3) > 1e-6 {
				t.Fatal("Hole wall did not follow solved point", r.Selection.Selection.CreationEvidence.Origin)
			}
		}
	}
	if !foundWall {
		t.Fatal("Hole wall missing")
	}
	// Reassign support with the same local design and oriented axis; no point coordinates are copied into Hole.
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Inner", Origin: [3]float64{0, 0, 20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	inner := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
	apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", SketchID: location, VersionID: view.Document.VersionID, DatumPlaneID: inner.ID})
	checkVolume(12000 - 72*math.Pi)
	chamferOff := saved(chamfer.ID)
	chamferOff.Suppressed = true
	edit(chamferOff)
	// The starting plane is inside stock. Through cuts only the selected half.
	for _, reverse := range []bool{false, true} {
		through := saved(hole.ID)
		through.Extent = "THROUGH_ALL"
		through.Reversed = reverse
		r := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-through-%v", view.Document.ID, reverse), Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &through, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, hole.ID)}
		candidate, e := service.PreviewCommand(t.Context(), view.Document.ID, r)
		length := 10.0
		if reverse {
			length = 20
		}
		if e != nil || candidate.Artifact == nil || math.Abs(candidate.Artifact.Volume-(12000-9*math.Pi*length)) > 1e-5 {
			t.Fatal("directional through", reverse, e)
		}
	}
	through := saved(hole.ID)
	through.Extent = "THROUGH_ALL"
	through.Reversed = false
	edit(through)
	for _, p := range view.Part.Parameters {
		if p.OwnerFeatureID == hole.ID && p.PropertySlot == "depth" {
			t.Fatal("inactive through depth parameter retained")
		}
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if params("depth").ParameterID != depthID {
		t.Fatal("undo lost depth identity")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	apply(workspace.CommandRequest{Type: "UNDO"})
	chamferOff = saved(chamfer.ID)
	chamferOff.Suppressed = false
	edit(chamferOff)
	// Missing point retains the old definition, and only its downstream dependants block.
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"p"}}}})
	if saved(hole.ID).EvaluationStatus != "FAILED" || !strings.Contains(saved(hole.ID).Diagnostic, "HOLE_POINT_MISSING") || saved(chamfer.ID).EvaluationStatus != "BLOCKED" || saved(hole.ID).HolePoint.Reference.EntityID != "p" {
		t.Fatal("point loss not retained", saved(hole.ID))
	}
	failedHead := view.Document.VersionID
	input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: hole.ID})
	if e != nil || input.PointPick == nil || input.PointPick.Status != modelcore.SelectionMissing || math.Abs(input.Artifact.Volume-12000) > 1e-6 {
		t.Fatal("failed point blocks valid upstream", e)
	}
	broken := saved(hole.ID)
	if _, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-unhandled", Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &broken, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, hole.ID)}); e == nil {
		t.Fatal("unhandled point saved")
	}
	broken.HolePoint = ref("q")
	edit(broken)
	if saved(hole.ID).ID != hole.ID || saved(hole.ID).BodyID != hole.BodyID || saved(hole.ID).Name != hole.Name || saved(hole.ID).Order != hole.Order {
		t.Fatal("repair changed identity")
	}
	// Replacement has a new formal point identity: repair the downstream mouth explicitly too.
	replacementInput, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: chamfer.ID})
	if e != nil {
		t.Fatal(e)
	}
	var rebound *workspace.FeatureSelection
	for local := uint64(1); local <= p6TopologyCount(t, replacementInput.Artifact.Topology, "edges"); local++ {
		r, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: chamfer.ID, GeometryKey: replacementInput.Artifact.GeometryKey, Kind: "EDGE", LocalID: local})
		if e == nil && r.Selection.Selection.Anchor.FeatureID == hole.ID && strings.HasPrefix(r.Selection.Selection.Anchor.OutputSlot, "START_BOUNDARY_FROM_PROFILE_EDGE/") {
			rebound = r.Selection
			break
		}
	}
	if rebound == nil {
		t.Fatal("repaired mouth unavailable")
	}
	changedChamfer = saved(chamfer.ID)
	changedChamfer.Selections = []workspace.FeatureSelection{*rebound}
	edit(changedChamfer)
	final := activeBodyArtifact(t, view)
	if saved(chamfer.ID).EvaluationStatus != "" {
		t.Fatal("downstream not restored")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	apply(workspace.CommandRequest{Type: "UNDO"})
	if saved(hole.ID).EvaluationStatus != "FAILED" {
		t.Fatal("undo did not restore failed point")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	apply(workspace.CommandRequest{Type: "REDO"})
	if activeBodyArtifact(t, view).GeometryID != final.GeometryID {
		t.Fatal("redo differs")
	}
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	opened, e := fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || activeBodyArtifact(t, opened).GeometryID != final.GeometryID {
		t.Fatal("reopen", e)
	}
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-cold", Type: "EDIT_FEATURE", TargetID: chamfer.ID, Feature: &changedChamfer, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, chamfer.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold mismatch", role)
		}
	}
	if !reflect.DeepEqual(saved(hole.ID).HolePoint, ref("q")) {
		t.Fatal("point binding not persisted")
	}
}

func TestHoleAdmissionAndReplicationThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Hole admission"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-admission-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	stock := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: stock, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stock, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
	base := view.Part.Features[len(view.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "p", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "q", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 4}}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "far", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 100}}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "CONSTRUCTION", Center: &workspace.SketchPoint2{}, Radius: 2}}}})
	point := func(id string) *workspace.PatternPointReference {
		return &workspace.PatternPointReference{SketchID: sketch, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}}
	}
	valid := workspace.Feature{Type: "HOLE", BodyID: base.BodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 5, HolePoint: point("p")}
	cases := map[string]workspace.Feature{}
	f := valid
	f.HolePoint = nil
	cases["select-point"] = f
	f = valid
	f.HolePoint = point("circle")
	cases["nonpoint"] = f
	f = valid
	f.HolePoint = point("absent")
	cases["missing-point"] = f
	f = valid
	f.Diameter = -2
	cases["diameter"] = f
	f = valid
	f.Depth = 0
	cases["depth"] = f
	f = valid
	f.HolePoint = point("far")
	cases["no-intersection"] = f
	f = valid
	f.Reversed = true
	f.Extent = "THROUGH_ALL"
	cases["wrong-half"] = f
	f = valid
	f.Extent = "TO_PLANE"
	cases["unsupported-extent"] = f
	head := view.Document.VersionID
	for name, f := range cases {
		if _, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-bad-" + name, Type: "CREATE_SOLID_FEATURE", Feature: &f}); e == nil {
			t.Fatal("invalid Hole accepted", name)
		}
	}
	inactive := valid
	inactive.Extent = "THROUGH_ALL"
	if _, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-inactive-depth", Type: "CREATE_SOLID_FEATURE", Feature: &inactive, ParameterExpressions: map[string]string{"depth": "5 mm"}}); e == nil || !strings.Contains(e.Error(), "HOLE_PARAMETER_UNSUPPORTED") {
		t.Fatal("inactive depth expression accepted", e)
	}
	current, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || current.Document.VersionID != head {
		t.Fatal("rejected definition wrote Head", e)
	}
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &valid})
	hole := view.Part.Features[len(view.Part.Features)-1]
	if caps := view.ReplicationSources[hole.ID]; caps.Tool || caps.RangeStart || caps.RangeModifier || !caps.BodyStage {
		t.Fatal("wrong Hole replication capabilities", caps)
	}
	for _, kind := range []string{"GENERATOR_TOOL", "FEATURE_DELTA", "BODY_STAGE"} {
		pattern := &workspace.Feature{Type: "SOLID_PATTERN", BodyID: base.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 2, Spacing: 30, Direction: [3]float64{1, 0, 0}}, Source: workspace.FeatureStageRef{BodyID: base.BodyID, FeatureID: hole.ID}, SourceKind: kind, ResultMode: "COMBINE"}}
		if kind == "FEATURE_DELTA" {
			pattern.Pattern.StartFeatureID = hole.ID
		}
		candidate, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-pattern-" + kind, Type: "CREATE_PATTERN", Feature: pattern})
		if kind == "BODY_STAGE" {
			if e != nil || candidate.Artifact == nil || math.Abs(candidate.Artifact.Volume-2*(12000-20*math.Pi)) > 1e-5 {
				t.Fatal("Body stage copy", e)
			}
		} else if e == nil || !strings.Contains(e.Error(), map[string]string{"GENERATOR_TOOL": "PATTERN_REEXECUTION_UNSUPPORTED", "FEATURE_DELTA": "PATTERN_RANGE_INVALID"}[kind]) {
			t.Fatal("Hole tool replication accepted", kind, e)
		}
	}
	if _, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-delete-location", Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: sketch}); e == nil {
		t.Fatal("referenced sketch deletion accepted")
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	later := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: later, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "later-point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
	hole.HolePoint = &workspace.PatternPointReference{SketchID: later, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "later-point", SubElement: "POINT"}}
	if _, e = service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-downstream", Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &hole, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, hole.ID)}); e == nil {
		t.Fatal("downstream point accepted")
	}
	// A different standard support rotates the evaluated axis and retains local point identity.
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "UPDATE_ENTITY_POINT", EntityID: "p", SubElement: "POINT", Point: &workspace.SketchPoint2{Y: 15}}}})
	apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", SketchID: sketch, VersionID: view.Document.VersionID, DatumPlaneID: "datum-yz"})
	if math.Abs(activeBodyArtifact(t, view).Volume-(12000-20*math.Pi)) > 1e-5 {
		t.Fatal("rotated Hole volume")
	}
	wall := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == hole.ID && s.CreationEvidence.GeometryType == "CYLINDER"
	})
	if math.Abs(math.Abs(wall.Selection.CreationEvidence.Direction[0])-1) > 1e-6 || math.Abs(wall.Selection.CreationEvidence.Origin[2]-15) > 1e-6 {
		t.Fatal("Hole did not follow the new solved support frame", wall.Selection.CreationEvidence)
	}
}

func TestEngineeringHoleLifecycleThroughRouter(t *testing.T) {
	service, store, db, client, _ := featureAssociationRuntimeService(t)
	v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Engineering Hole"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-engineering-%d", v.Document.ID, seq)
		}
		v, err = service.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	saved := func(id string) workspace.Feature {
		t.Helper()
		for _, f := range v.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("missing feature", id)
		return workspace.Feature{}
	}
	edit := func(f workspace.Feature) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, f.ID)})
	}
	parameter := func(id, slot string) modelcore.ParameterDefinition {
		t.Helper()
		for _, p := range v.Part.Parameters {
			if p.OwnerFeatureID == id && p.PropertySlot == slot {
				return p
			}
		}
		t.Fatal("missing parameter", slot)
		return modelcore.ParameterDefinition{}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := v.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
	base := v.Part.Features[len(v.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 30}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID})
	location := v.Part.Features[len(v.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "p", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
	definition := workspace.Feature{Type: "HOLE", BodyID: base.BodyID, Operation: "REMOVE", Extent: "BLIND", HoleKind: "COUNTERBORE", Diameter: 4, Depth: 10, RecessDiameter: 8, RecessDepth: 3, Reversed: true, HolePoint: &workspace.PatternPointReference{SketchID: location, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "p", SubElement: "POINT"}}}
	create := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: v.Document.ID + "-counterbore", Feature: &definition, ParameterExpressions: map[string]string{"diameter": "2 mm + 2 mm", "recessDiameter": "2 * 4 mm", "recessDepth": "6 mm / 2"}}
	before := v.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), v.Document.ID, create)
	if e != nil || preview.Artifact == nil {
		t.Fatal("counterbore preview", e)
	}
	untouched, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || untouched.Document.VersionID != before {
		t.Fatal("preview wrote", e)
	}
	create.PreviewID = preview.PreviewID
	apply(create)
	hole := v.Part.Features[len(v.Part.Features)-1]
	checkVolume := func(extra float64) {
		t.Helper()
		stage, e := service.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: hole.ID, ResultStage: true})
		if e != nil || math.Abs(stage.Artifact.Volume-(12000-math.Pi*(40+extra))) > 1e-5 {
			t.Fatal("volume", stage.Artifact.Volume, extra, e)
		}
	}
	checkVolume(36)
	mainID, recessID := parameter(hole.ID, "diameter").ParameterID, parameter(hole.ID, "recessDiameter").ParameterID
	// A stable expression dependency on the parent pad changes both managed seat dimensions.
	diameter := parameter(base.ID, "length")
	source := fmt.Sprintf("%s / 5", diameter.Key)
	changed := saved(hole.ID)
	request := workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, hole.ID), ParameterExpressions: map[string]string{"recessDiameter": source}}
	apply(request)
	if saved(hole.ID).RecessDiameter != 6 || parameter(hole.ID, "recessDiameter").ParameterID != recessID {
		t.Fatal("expression or parameter identity lost")
	}
	base.Length = 40
	edit(base)
	if saved(hole.ID).RecessDiameter != 8 {
		t.Fatal("expression dependency not recomputed", saved(hole.ID).RecessDiameter)
	}
	base.Length = 30
	edit(base)
	// Restore the fixed seat size before checking downstream topology continuity.
	changed = saved(hole.ID)
	changed.RecessDiameter = 8
	edit(changed)
	mouth := findP7TopologyPick(t, service, v, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == hole.ID && strings.HasPrefix(s.Anchor.OutputSlot, "HOLE_COUNTERBORE_MOUTH")
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: hole.BodyID, Length: .2, Selections: []workspace.FeatureSelection{{Selection: mouth.Selection, SourceVersionID: mouth.SourceVersionID}}}})
	chamfer := v.Part.Features[len(v.Part.Features)-1]
	changed = saved(hole.ID)
	changed.RecessDiameter = 9
	edit(changed)
	if saved(chamfer.ID).EvaluationStatus != "" {
		t.Fatal("mouth lost after size edit", saved(chamfer.ID).Diagnostic)
	}
	shoulder := findP7TopologyPick(t, service, v, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == hole.ID && s.Anchor.OutputSlot == "HOLE_COUNTERBORE_SHOULDER"
	})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", BodyID: hole.BodyID, VersionID: v.Document.VersionID, GeometryKey: shoulder.GeometryKey, TopologyID: shoulder.LocalID})
	shoulderSketch := v.Part.Features[len(v.Part.Features)-1]
	// Removing the counterbore role must preserve the failed downstream definition.
	changed = saved(hole.ID)
	changed.HoleKind = "COUNTERSINK"
	changed.CountersinkAngle = 90
	changed.RecessDepth = 0
	switchRequest := workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-switch-preview", Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, hole.ID)}
	switchPreview, e := service.PreviewCommand(t.Context(), v.Document.ID, switchRequest)
	if e != nil || switchPreview.Artifact == nil || len(switchPreview.RepairRequired) != 1 || switchPreview.RepairRequired[0].FeatureID != chamfer.ID {
		t.Fatal("downstream repair preview", switchPreview.RepairRequired, e)
	}
	switchRequest.PreviewID = switchPreview.PreviewID
	apply(switchRequest)
	if saved(chamfer.ID).EvaluationStatus != "FAILED" {
		t.Fatal("vanished role silently rebound", saved(chamfer.ID))
	}
	if saved(shoulderSketch.ID).Sketch.Support.Status != "BLOCKED" {
		t.Fatal("failed predecessor did not block support", saved(shoulderSketch.ID).Sketch.Support)
	}
	if saved(hole.ID).ID != hole.ID || saved(hole.ID).BodyID != hole.BodyID || saved(hole.ID).Name != hole.Name || parameter(hole.ID, "diameter").ParameterID != mainID || parameter(hole.ID, "recessDiameter").ParameterID != recessID {
		t.Fatal("switch changed stable identity")
	}
	for _, p := range v.Part.Parameters {
		if p.OwnerFeatureID == hole.ID && p.PropertySlot == "recessDepth" {
			t.Fatal("inactive parameter retained")
		}
	}
	// Rebind explicitly to the cone mouth using the legal input prefix of the failed chamfer.
	input, e := service.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: chamfer.ID})
	if e != nil {
		t.Fatal(e)
	}
	var replacement *workspace.FeatureSelection
	for local := uint64(1); local <= p6TopologyCount(t, input.Artifact.Topology, "edges"); local++ {
		p, e := service.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: chamfer.ID, GeometryKey: input.Artifact.GeometryKey, Kind: "EDGE", LocalID: local})
		if e != nil {
			t.Fatal(e)
		}
		if p.Selection != nil && strings.HasPrefix(p.Selection.Selection.Anchor.OutputSlot, "HOLE_COUNTERSINK_MOUTH") {
			replacement = p.Selection
			break
		}
	}
	if replacement == nil {
		t.Fatal("cone mouth not named")
	}
	child := saved(chamfer.ID)
	child.Selections = []workspace.FeatureSelection{*replacement}
	edit(child)
	if saved(chamfer.ID).EvaluationStatus != "" {
		t.Fatal("explicit repair failed")
	}
	if saved(shoulderSketch.ID).Sketch.Support.Status != "FAILED_SUPPORT" {
		t.Fatal("vanished shoulder did not fail support", saved(shoulderSketch.ID).Sketch.Support)
	}
	apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", VersionID: v.Document.VersionID, SketchID: shoulderSketch.ID, DatumPlaneID: v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID})
	if saved(shoulderSketch.ID).Sketch.Support.Status != "CONNECTED" {
		t.Fatal("support repair failed")
	}
	changed = saved(hole.ID)
	changed.CountersinkAngle = 100
	request = workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, hole.ID), ParameterExpressions: map[string]string{"countersinkAngle": "50 deg + 50 deg"}}
	apply(request)
	angleID := parameter(hole.ID, "countersinkAngle").ParameterID
	invalid := []workspace.Feature{saved(hole.ID), saved(hole.ID), saved(hole.ID)}
	invalid[0].RecessDiameter = 3
	invalid[1].CountersinkAngle = 180
	invalid[2].Depth = .1
	head := v.Document.VersionID
	for i, f := range invalid {
		_, e := service.PreviewCommand(t.Context(), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-invalid-%d", head, i), Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, hole.ID)})
		if e == nil {
			t.Fatal("invalid hole accepted", i)
		}
	}
	current, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || current.Document.VersionID != head {
		t.Fatal("invalid preview advanced Head", e)
	}
	final := activeBodyArtifact(t, v)
	apply(workspace.CommandRequest{Type: "UNDO"})
	apply(workspace.CommandRequest{Type: "REDO"})
	if activeBodyArtifact(t, v).GeometryID != final.GeometryID || parameter(hole.ID, "countersinkAngle").ParameterID != angleID {
		t.Fatal("history differs")
	}
	fresh := workspace.NewWithArtifacts(db, client, store)
	opened, e := fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || activeBodyArtifact(t, opened).GeometryID != final.GeometryID {
		t.Fatal("reopen", e)
	}
	child = saved(chamfer.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: head + "-cold", Type: "EDIT_FEATURE", TargetID: child.ID, Feature: &child, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, child.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("cold digest differs", role)
		}
	}
}
