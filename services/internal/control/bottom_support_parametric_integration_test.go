package control

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// Actual Domain Command -> Router -> Worker -> Profile -> artifacts; independent
// temporary storage is owned by the existing integration fixture, never dev data.
func TestBottomSupportParametricDatumLifecycleThroughRouter(t *testing.T) {
	service, artifacts, db, client := featureAssociationTestService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Bottom Support parameter chain"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-bottom-%d", view.Document.ID, seq)
		}
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatalf("%s: %v", r.Type, e)
		}
		view = v
		for _, f := range view.Part.Features {
			if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
				t.Fatalf("%s: %s", f.Type, f.Diagnostic)
			}
		}
	}
	parameterID := func(key string) string {
		t.Helper()
		for _, p := range view.Part.Parameters {
			if p.Key == key {
				return p.ParameterID
			}
		}
		t.Fatalf("parameter %s absent", key)
		return ""
	}
	for _, p := range []struct {
		key, unit string
		value     float64
	}{{"tilt", "deg", 30}, {"height", "mm", 20}, {"r_1", "mm", 5}, {"r_2", "mm", 10}} {
		apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: p.key, Unit: p.unit, Value: p.value})
	}
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_EXPRESSION", ParameterID: parameterID("r_2"), Expression: "r_1 + 5"})
	system := view.Part.AxisSystems[0].ID
	x := workspace.DatumReference{Kind: "AXIS_SYSTEM", EntityID: system, Axis: "X"}
	z := workspace.DatumReference{Kind: "AXIS_SYSTEM", EntityID: system, Axis: "Z"}
	datumRequest := workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Support slope", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "PLANE", EntityID: "datum-xy"}, RotationAxis: &x, TranslationDirection: &z}, DatumAngleExpression: "tilt", DatumDistanceExpression: "height"}
	apply(datumRequest)
	planeID := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_AXIS", Name: "Support normal", Direction: [3]float64{0, 0, 1}, DatumDefinition: &workspace.DatumTransform{Source: z, RotationAxis: &x, TranslationDirection: &z}, DatumAngleExpression: "tilt", DatumDistanceExpression: "height"})
	normalID := view.Part.DatumAxes[len(view.Part.DatumAxes)-1].ID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: planeID})
	sketchID := view.Part.Features[len(view.Part.Features)-1].ID
	refs := []workspace.SketchOperation{}
	for _, p := range []struct {
		id  string
		ref workspace.DatumReference
	}{{"world-origin", workspace.DatumReference{Kind: "POINT", EntityID: system}}, {"world-x", x}, {"support-normal", workspace.DatumReference{Kind: "AXIS", EntityID: normalID}}} {
		ref := p.ref
		refs = append(refs, workspace.SketchOperation{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: p.id, SourceVersionID: view.Document.VersionID, DatumReference: &ref})
	}
	refs = append(refs, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "support-circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 10}})
	radiusSource := "r_2"
	radiusValue := 10.0
	refs = append(refs, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "support-radius", Kind: "RADIUS", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "support-circle", SubElement: "WHOLE"}}, Value: &radiusValue, Unit: "mm"}, ParameterSource: &radiusSource, ParameterKey: "support_radius"})
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: refs})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketchID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", LengthExpression: "height / 4"})
	check := func(angle, height, radius float64) {
		t.Helper()
		p := view.Part.DatumPlanes[slices.IndexFunc(view.Part.DatumPlanes, func(p workspace.DatumPlane) bool { return p.ID == planeID })]
		a := angle * math.Pi / 180
		if math.Abs(p.Origin[2]-height) > 1e-8 || math.Abs(p.Normal[1]+math.Sin(a)) > 1e-8 || math.Abs(p.Normal[2]-math.Cos(a)) > 1e-8 {
			t.Fatalf("datum not recomputed %+v", p)
		}
		sketch := view.Part.Features[slices.IndexFunc(view.Part.Features, func(f workspace.Feature) bool { return f.ID == sketchID })].Sketch
		if math.Abs(sketch.Support.Normal[1]-p.Normal[1]) > 1e-8 || math.Abs(sketch.Support.Origin[2]-height) > 1e-8 {
			t.Fatal("sketch retained old support", sketch.Support)
		}
		for _, e := range sketch.ExternalGeometry {
			if e.Status != "CONNECTED" || e.DatumReference == nil || e.ResolvedSourceDigest == "" {
				t.Fatal("projection reference lost", e)
			}
			expected := "POINT"
			if e.ID == "world-x" {
				expected = "LINE"
			}
			if e.GeometryKind != expected {
				t.Fatal("axis projection kind", e)
			}
			if e.ID == "world-origin" && math.Abs(e.Snapshot.Point.Y+height*math.Sin(a)) > 1e-8 {
				t.Fatal("world origin projection stale", e.Snapshot)
			}
		}
		want := math.Pi * radius * radius * height / 4
		if math.Abs(activeBodyArtifact(t, view).Volume-want) > 1e-5 {
			t.Fatalf("profile/solid did not recompute: %g want %g", activeBodyArtifact(t, view).Volume, want)
		}
	}
	check(30, 20, 10)
	// Authority preview leaves Head and artifacts unchanged; promotion commits once.
	seq++
	request := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-bottom-%d", view.Document.ID, seq), Type: "EDIT_PARAMETER", ParameterID: parameterID("tilt"), Name: "tilt", Unit: "deg", Value: 60}
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
	if e != nil || preview.PreviewID == "" {
		t.Fatal("parameter preview", e)
	}
	found := false
	for _, p := range preview.ParameterCandidates {
		if p.ParameterID == request.ParameterID && p.EvaluatedValue != nil {
			found = math.Abs(p.EvaluatedValue.SIValue-math.Pi/3) < 1e-8
		}
	}
	if !found {
		t.Fatal("preview omitted calculated value")
	}
	current, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || current.Document.VersionID != view.Document.VersionID {
		t.Fatal("preview wrote Head", e)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	check(60, 20, 10)
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(30, 20, 10)
	apply(workspace.CommandRequest{Type: "REDO"})
	check(60, 20, 10)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameterID("height"), Unit: "mm", Value: 40})
	check(60, 40, 10)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameterID("r_1"), Unit: "mm", Value: 7})
	check(60, 40, 12)
	tiltID := parameterID("tilt")
	apply(workspace.CommandRequest{Type: "RENAME_PARAMETER", ParameterID: tiltID, Name: "support_tilt"})
	datumRequest.Type = "EDIT_DATUM_PLANE"
	datumRequest.TargetID = planeID
	datumRequest.Name = "Support slope edited"
	datumRequest.DatumAngleExpression = "support_tilt"
	apply(datumRequest)
	check(60, 40, 12)
	if view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID != planeID {
		t.Fatal("edit changed persistent datum identity")
	}
	// Stale helper selection must fail before projection; failure does not mutate Head.
	before := view.Document.VersionID
	_, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: "stale-datum-projection", Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "stale", SourceVersionID: current.Document.VersionID, DatumReference: &x}}})
	if e == nil {
		t.Fatal("accepted stale projection")
	}
	cold := workspace.NewWithArtifacts(db, client, artifacts)
	reopened, e := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	view = reopened
	if view.Document.VersionID != before {
		t.Fatal("failed projection mutated Head")
	}
	check(60, 40, 12)
	// Resolve a planar face through real Naming at the explicitly selected stage.
	stage := view.Part.Features[len(view.Part.Features)-1]
	face := findP6FacePick(t, service, view, func(p modelcore.PersistentSelection) bool { return p.CreationEvidence.GeometryType == "PLANE" })
	source := workspace.DatumReference{Kind: "TOPOLOGY", FeatureID: stage.ID, Selection: &face.Selection, SourceVersionID: view.Document.VersionID}
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Face datum", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: source, Distance: 3}})
	faceDatumID := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: faceDatumID})
	faceSketch := view.Part.Features[len(view.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: faceSketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "direction-line", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{X: 0, Y: 1}, End: &workspace.SketchPoint2{X: 2, Y: 1}}}}})
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_AXIS", Name: "Line datum", Direction: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "SKETCH_LINE", FeatureID: faceSketch.ID, EntityID: "direction-line"}, Distance: 2}})
	axis := view.Part.DatumAxes[len(view.Part.DatumAxes)-1]
	if math.Abs(axis.Direction[0])+math.Abs(axis.Direction[1])+math.Abs(axis.Direction[2]) < 0.99 {
		t.Fatal("sketch direction not solved", axis)
	}
	// Indirect self-use via a supported sketch must fail and preserve Head.
	_, e = service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: "datum-cycle", Type: "EDIT_DATUM_PLANE", TargetID: faceDatumID, Name: "Face datum", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: source, RotationAxis: &workspace.DatumReference{Kind: "SKETCH_LINE", FeatureID: faceSketch.ID, EntityID: "direction-line"}, Angle: 5}})
	if e == nil {
		t.Fatal("accepted datum/sketch cycle")
	}
	cold = workspace.NewWithArtifacts(db, client, artifacts)
	reopened, e = cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.Part.DatumAxes[len(reopened.Part.DatumAxes)-1].Definition.Source.FeatureID != faceSketch.ID {
		t.Fatal("cold read lost sketch line definition")
	}

	// A later face/line-derived datum must follow upstream parameter changes too.
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameterID("height"), Unit: "mm", Value: 50})
	check(60, 50, 12)
	support := view.Part.Features[len(view.Part.Features)-1].Sketch.Support
	plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
	if support.Origin != plane.Origin || support.Normal != plane.Normal {
		t.Fatal("topology datum/line consumer retained old frame")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(60, 40, 12)
	apply(workspace.CommandRequest{Type: "REDO"})
	check(60, 50, 12)

	// Persistent display attributes must retain the exact Body result, datum
	// references and parameter chain, including compensating history/reopen.
	key := view.Part.Bodies[0].GeometryKey
	apply(workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "ORIGIN", TargetID: "origin", Visible: false})
	if view.Part.OriginVisible == nil || *view.Part.OriginVisible || view.Part.Bodies[0].GeometryKey != key {
		t.Fatal("Origin visibility changed geometry or was not saved")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if view.Part.OriginVisible != nil && !*view.Part.OriginVisible {
		t.Fatal("Origin Undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if view.Part.OriginVisible == nil || *view.Part.OriginVisible {
		t.Fatal("Origin Redo")
	}
	apply(workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "AXIS", TargetID: system, Axis: "X", Visible: false})
	if view.Part.AxisSystems[0].AxisVisibility["X"] {
		t.Fatal("X visibility")
	}
	cold = workspace.NewWithArtifacts(db, client, artifacts)
	reopened, e = cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || reopened.Part.OriginVisible == nil || *reopened.Part.OriginVisible || reopened.Part.AxisSystems[0].AxisVisibility["X"] {
		t.Fatal("cold display state", e)
	}
	check(60, 50, 12)

}
