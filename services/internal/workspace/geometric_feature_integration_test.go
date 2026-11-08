//go:build integration

package workspace

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
	"math"
	"testing"
)

func TestGeometryFeaturePreviewCommitParameterHistoryAndColdRead(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	line := testGeometryLine()
	line.ID = ""
	req := CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", RequestID: uuid.NewString(), ActorID: f.actor, Feature: &line}
	before, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewCommand(f.ctx, f.documentID, req)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Artifact == nil || preview.Artifact.Result == nil || preview.Artifact.BodyID != "" || preview.Artifact.GeometryDomain != "WIRE_SET" {
		t.Fatalf("incorrect curve preview: %+v", preview)
	}
	unchanged, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil || unchanged.Document.VersionID != before.Document.VersionID {
		t.Fatal("preview changed revision", err)
	}
	req.PreviewID = preview.PreviewID
	view, err := f.service.ApplyCommand(f.ctx, f.documentID, req)
	if err != nil {
		t.Fatal(err)
	}
	curve := view.Part.Features[len(view.Part.Features)-1]
	if curve.Result == nil || curve.BodyID != "" {
		t.Fatal("independent result not persisted")
	}
	surface := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: curve.ID, ResultID: "result"}}, Parameters: GeometryOperationParameters{Direction: [3]float64{0, 0, 1}, Distance: 20}}}
	view = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &surface})
	sheet := view.Part.Features[len(view.Part.Features)-1]
	if sheet.Result == nil || sheet.Result.Domain != "SHEET_SET" {
		t.Fatalf("sheet output missing: %+v", sheet)
	}
	thick := Feature{Type: "THICKEN", Operation: "NEW_BODY", Geometry: &GeometricFeatureDefinition{Kind: "THICKEN", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: sheet.ID, ResultID: "result"}}, Parameters: GeometryOperationParameters{Distance: 2}}}
	view = f.command(CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &thick})
	f.volume(view, 2000)
	a := view.Artifacts[sheet.Result.GeometryKey]
	if a.Result == nil || a.GeometryDomain != "SHEET_SET" || a.BodyID != "" || a.Volume != 0 {
		t.Fatalf("surface artifact lost owner/domain: %+v", a)
	}
	manifest, _, err := f.service.topologyManifestForGeometryKey(f.ctx, sheet.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	tip := manifest.Results[modelcore.GeometryResultReference{FeatureID: sheet.ID, ResultID: "result"}]
	if tip == nil {
		t.Fatal("Naming result owner missing")
	}
	var faceID uint64
	for _, o := range tip.SemanticOutputs {
		if topologyType(o.TopologyType) == modelcore.PersistentTopologyFace {
			faceID = o.LocalId
			break
		}
	}
	selection, err := f.service.BindPersistentSelection(f.ctx, f.documentID, BindPersistentSelectionRequest{SourceVersionID: view.Document.VersionID, GeometryKey: sheet.Result.GeometryKey, Kind: "FACE", LocalID: faceID})
	if err != nil {
		t.Fatal(err)
	}
	if selection.SourceResult == nil || selection.SourceBodyID != "" {
		t.Fatal("sheet pick gained fake Body")
	}
	sourceRevision := view.Document.VersionID
	curve.Geometry.Curve.Points[1].Position[0] = 75
	digest, err := featureDefinitionDigest(*view.Part, curve.ID)
	if err != nil {
		t.Fatal(err)
	}
	view = f.command(CommandRequest{Type: "EDIT_FEATURE", TargetID: curve.ID, ExpectedFeatureDigest: digest, Feature: &curve})
	f.volume(view, 3000)
	resolved, err := f.service.ResolvePersistentSelection(f.ctx, f.documentID, ResolvePersistentSelectionRequest{Selection: selection, SourceVersionID: sourceRevision, TargetVersionID: view.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if err != nil || resolved.Status != modelcore.SelectionResolved {
		t.Fatal("sheet reference did not survive parameter update", resolved, err)
	}
	f.volume(f.command(CommandRequest{Type: "UNDO"}), 2000)
	view = f.command(CommandRequest{Type: "REDO"})
	f.volume(view, 3000)
	cold := NewWithArtifacts(f.service.database, f.service.worker, f.service.artifacts)
	reopened, err := cold.GetDocument(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.Artifacts) != len(view.Artifacts) {
		t.Fatal("cold reopen lost independent geometry")
	}
	candidate := clonePartModel(*reopened.Part)
	if err := cold.evaluatePartBodies(WithColdEvaluation(f.ctx), "cold-geometry/"+uuid.NewString(), &candidate); err != nil {
		t.Fatal(err)
	}
	volume := 0.0
	for _, b := range candidate.Bodies {
		if b.GeometryKey != "" {
			a, err := cold.loadArtifact(f.ctx, b.GeometryKey)
			if err != nil {
				t.Fatal(err)
			}
			volume += a.Volume
		}
	}
	if math.Abs(volume-3000) > 1e-6 {
		t.Fatal("cold evaluation changed material", volume)
	}
	body := view.Part.Bodies[bodyIndex(*view.Part, view.Part.ActiveBodyID)]
	naming, _, err := f.service.topologyManifestForGeometryKey(f.ctx, body.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	var edgeID uint64
	for _, output := range naming.Tips[body.ID].SemanticOutputs {
		if topologyType(output.TopologyType) == modelcore.PersistentTopologyEdge {
			edgeID = output.LocalId
			break
		}
	}
	edge, err := f.service.BindPersistentSelection(f.ctx, f.documentID, BindPersistentSelectionRequest{BodyID: body.ID, SourceVersionID: view.Document.VersionID, GeometryKey: body.GeometryKey, Kind: "EDGE", LocalID: edgeID})
	if err != nil {
		t.Fatal(err)
	}
	modified := f.command(CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &Feature{Type: "FILLET", BodyID: body.ID, Length: .25, Selections: []FeatureSelection{{Selection: edge, SourceVersionID: view.Document.VersionID, SourceFeatureID: body.CreatedByFeatureID}}}})
	modifiedArtifact, err := f.service.loadArtifact(f.ctx, modified.Part.Bodies[bodyIndex(*modified.Part, body.ID)].GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	if modifiedArtifact.Volume <= 0 || modifiedArtifact.Volume >= 3000 {
		t.Fatal("original Body modifier did not consume thickened surface", modifiedArtifact.Volume)
	}
	// A finite planar Sheet also supports a normal associative sketch. The
	// earlier frozen source remains valid after the material modifier.
	supported := f.command(CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", VersionID: sourceRevision, GeometryKey: sheet.Result.GeometryKey, TopologyID: faceID, SketchOwnerKind: "GEOMETRICAL_SET"})
	createdSketch := supported.Part.Features[len(supported.Part.Features)-1]
	if createdSketch.Sketch == nil || createdSketch.Sketch.Support.Status != "CONNECTED" || createdSketch.Sketch.Support.PersistentSelection == nil || createdSketch.Sketch.Support.PersistentSelection.SourceResult == nil {
		t.Fatalf("Sheet sketch support lost its result owner: %+v", createdSketch)
	}
}

func TestGeometryFeatureSketchPointFrameAndSpatialSweepThroughRouter(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	initial, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	f.command(CommandRequest{Type: "DELETE_BODY", BodyID: initial.Part.ActiveBodyID})
	// The first geometry-owned sketch creates its container atomically.
	v := f.command(CommandRequest{Type: "CREATE_SKETCH", Plane: "XY", SketchOwnerKind: "GEOMETRICAL_SET"})
	if len(v.Part.Bodies) != 0 {
		t.Fatal("geometry-only Part acquired a fake Body")
	}
	sketch := v.Part.Features[len(v.Part.Features)-1]
	if sketch.BodyID != "" || sketch.GeometricalSetID == "" {
		t.Fatal("geometrical sketch assigned to material Body")
	}
	point := SketchEntity{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 30}}
	v = f.command(CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []SketchOperation{workflowEntity(point)}})
	surface := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "SKETCH", FeatureID: sketch.ID, EntityIDs: []string{"line"}}}, Parameters: GeometryOperationParameters{Direction: [3]float64{0, 0, 1}, Distance: 10}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &surface})
	sheet := v.Part.Features[len(v.Part.Features)-1]
	if sheet.Result == nil || sheet.Result.Domain != "SHEET_SET" {
		t.Fatal("open sketch did not produce a sheet")
	}
	// Explicit local frame coordinates and exact point results remain associative.
	framed := testGeometryLine()
	framed.ID = ""
	framed.Geometry.Curve.FramePlaneID = "datum-yz"
	framed.Geometry.Curve.Points[1].Position = [3]float64{10, 0, 0}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &framed})
	line := v.Part.Features[len(v.Part.Features)-1]
	onCurve := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "POINT_ON_CURVE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: line.ID, ResultID: "result"}}, Parameters: GeometryOperationParameters{Start: .5}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &onCurve})
	pointFeature := v.Part.Features[len(v.Part.Features)-1]
	link := testGeometryLine()
	link.ID = ""
	link.Geometry.Curve.Points[0].Reference = &GeometricInput{Kind: "RESULT", FeatureID: pointFeature.ID, ResultID: "result"}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &link})
	if v.Part.Features[len(v.Part.Features)-1].Result == nil {
		t.Fatal("point result was not consumed by a line")
	}
	path := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SPLINE", Curve: &GeometryCurveDefinition{Points: []GeometryControlPoint{{ID: "a", Position: [3]float64{}}, {ID: "b", Position: [3]float64{0, 0, 10}}, {ID: "c", Position: [3]float64{10, 10, 20}}, {ID: "d", Position: [3]float64{10, 20, 30}}}, Degree: 3, Knots: []float64{0, 1}, Multiplicities: []uint32{4, 4}, Weights: []float64{1, 1, 1, 1}}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &path})
	curve := v.Part.Features[len(v.Part.Features)-1]
	v = f.command(CommandRequest{Type: "CREATE_SKETCH", Plane: "XY", SketchOwnerKind: "GEOMETRICAL_SET"})
	profile := v.Part.Features[len(v.Part.Features)-1]
	f.command(CommandRequest{Type: "EDIT_SKETCH", SketchID: profile.ID, Operations: []SketchOperation{workflowEntity(SketchEntity{ID: "disk", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 1})}})
	sweep := Feature{Type: "SWEEP", Profile: profile.ID, Operation: "NEW_BODY", Sweep: &SweepDefinition{Path: CurveChainReference{Geometry: &GeometricInput{Kind: "RESULT", FeatureID: curve.ID, ResultID: "result"}}, FrameMode: "CORRECTED_FRENET", CornerMode: "G1_ONLY"}}
	v = f.command(CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &sweep})
	volume := 0.0
	for _, a := range v.Artifacts {
		volume += a.Volume
	}
	if volume <= 30*math.Pi {
		t.Fatal("spatial spline did not produce material", volume)
	}
	curve.Geometry.Curve.Points[3].Position[2] = 40
	digest, err := featureDefinitionDigest(*v.Part, curve.ID)
	if err != nil {
		t.Fatal(err)
	}
	v = f.command(CommandRequest{Type: "EDIT_FEATURE", TargetID: curve.ID, Feature: &curve, ExpectedFeatureDigest: digest})
	after := 0.0
	for _, a := range v.Artifacts {
		after += a.Volume
	}
	if after <= volume {
		t.Fatal("spatial sweep did not follow its curve parameters", after, volume)
	}
}

// The direction is design geometry. Editing its source updates the sheet and
// material without editing either consuming feature, including a cold rebuild.
func TestGeometryAssociativeDirectionPointListsAndColdHistory(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	point := func(name string, position [3]float64) string {
		v := f.command(CommandRequest{Type: "CREATE_REFERENCE_POINT", Name: name, ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: position}})
		return v.Part.DatumPoints[len(v.Part.DatumPoints)-1].ID
	}
	a, b, c := point("Start", [3]float64{0, 0, 0}), point("End", [3]float64{30, 0, 0}), point("Direction endpoint", [3]float64{0, 0, 10})
	ref := func(id string) *GeometricInput {
		return &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: id}}
	}
	line := func(name string, first, last string) Feature {
		return Feature{Type: "GEOMETRY", Name: name, Geometry: &GeometricFeatureDefinition{Kind: "LINE", Curve: &GeometryCurveDefinition{Points: []GeometryControlPoint{{ID: "start", Reference: ref(first)}, {ID: "end", Reference: ref(last)}}}}}
	}
	profile, direction := line("Profile", a, b), line("Direction", a, c)
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &profile})
	profile = v.Part.Features[len(v.Part.Features)-1]
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &direction})
	direction = v.Part.Features[len(v.Part.Features)-1]
	sheet := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: profile.ID, ResultID: "result"}}, Direction: &GeometricInput{Kind: "RESULT", FeatureID: direction.ID, ResultID: "result"}, Parameters: GeometryOperationParameters{Distance: 20}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &sheet})
	sheet = v.Part.Features[len(v.Part.Features)-1]
	definition, _ := featureDefinitionDigest(*v.Part, sheet.ID)
	for _, p := range v.Part.Parameters {
		if p.OwnerFeatureID == sheet.ID && p.PropertySlot != "geometry:distance" {
			t.Fatal("direction reference created unrelated scalar slots", p)
		}
	}
	bounds := func(v DocumentView, id string) [6]float64 {
		t.Helper()
		for _, f := range v.Part.Features {
			if f.ID == id && f.Result != nil {
				a := v.Artifacts[f.Result.GeometryKey]
				encoded, _ := json.Marshal(a.BBox)
				var box struct{ Min, Max [3]float64 }
				if err := json.Unmarshal(encoded, &box); err != nil {
					t.Fatal(err)
				}
				return [6]float64{box.Min[0], box.Min[1], box.Min[2], box.Max[0], box.Max[1], box.Max[2]}
			}
		}
		t.Fatal("missing result", id)
		return [6]float64{}
	}
	assertBox := func(v DocumentView, want [6]float64) {
		t.Helper()
		got := bounds(v, sheet.ID)
		for i := range got {
			if math.Abs(got[i]-want[i]) > 1e-5 {
				t.Fatal("sheet did not follow direction", got, want)
			}
		}
	}
	assertBox(v, [6]float64{0, 0, 0, 30, 0, 20})
	thick := Feature{Type: "THICKEN", Operation: "NEW_BODY", Geometry: &GeometricFeatureDefinition{Kind: "THICKEN", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: sheet.ID, ResultID: "result"}}, Parameters: GeometryOperationParameters{Distance: 2}}}
	v = f.command(CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &thick})
	f.volume(v, 1200)
	// A point edit rotates the referenced line from +Z to +Y. Neither consumer
	// receives new input, and the line's endpoint identities remain unchanged.
	v = f.command(CommandRequest{Type: "EDIT_REFERENCE_POINT", TargetID: c, Name: "Direction endpoint", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{0, 10, 0}}})
	assertBox(v, [6]float64{0, 0, 0, 30, 20, 0})
	f.volume(v, 1200)
	after, _ := featureDefinitionDigest(*v.Part, sheet.ID)
	if after != definition {
		t.Fatal("upstream edit rewrote consumer definition")
	}
	assertBox(f.command(CommandRequest{Type: "UNDO"}), [6]float64{0, 0, 0, 30, 0, 20})
	v = f.command(CommandRequest{Type: "REDO"})
	assertBox(v, [6]float64{0, 0, 0, 30, 20, 0})
	// Reversing an upstream line is also observable; it is not canonicalized to
	// a guessed positive direction based on screen coordinates or labels.
	v = f.command(CommandRequest{Type: "EDIT_REFERENCE_POINT", TargetID: c, Name: "Direction endpoint", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{0, -10, 0}}})
	assertBox(v, [6]float64{0, -20, 0, 30, 0, 0})
	cold := NewWithArtifacts(f.service.database, f.service.worker, f.service.artifacts)
	reopened, err := cold.GetDocument(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	assertBox(reopened, [6]float64{0, -20, 0, 30, 0, 0})
	candidate := clonePartModel(*reopened.Part)
	if err := cold.evaluatePartBodies(WithColdEvaluation(f.ctx), "cold-associative/"+uuid.NewString(), &candidate); err != nil {
		t.Fatal(err)
	}
	for _, feature := range candidate.Features {
		if feature.ID == sheet.ID {
			artifact, err := cold.loadArtifact(f.ctx, feature.Result.GeometryKey)
			if err != nil {
				t.Fatal(err)
			}
			reopened.Artifacts[feature.Result.GeometryKey] = artifact
			reopened.Part = &candidate
			assertBox(reopened, [6]float64{0, -20, 0, 30, 0, 0})
		}
	}
	spline := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SPLINE", Curve: &GeometryCurveDefinition{Points: []GeometryControlPoint{{ID: "a", Reference: ref(a)}, {ID: "b", Reference: ref(b)}, {ID: "c", Reference: ref(c)}}}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &spline})
	spline = v.Part.Features[len(v.Part.Features)-1]
	beforeKey := spline.Result.GeometryKey
	v = f.command(CommandRequest{Type: "EDIT_REFERENCE_POINT", TargetID: b, Name: "End", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{40, 0, 0}}})
	for _, feature := range v.Part.Features {
		if feature.ID == spline.ID && feature.Result.GeometryKey == beforeKey {
			t.Fatal("point list lost associativity")
		}
	}
	f.volume(v, 1600)
	_, err = f.service.ApplyCommand(f.ctx, f.documentID, CommandRequest{Type: "DELETE_NODE", RequestID: uuid.NewString(), ActorID: f.actor, TargetKind: "REFERENCE_POINT", TargetID: b})
	if err == nil {
		t.Fatal("referenced point deletion accepted")
	}
}

func TestGeometryAssociativePlaneAxisAndSketchPointThroughRouter(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	v := f.edit(workflowEntity(SketchEntity{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 5, Y: 7}}))
	c := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "CIRCLE", Curve: &GeometryCurveDefinition{Points: []GeometryControlPoint{{ID: "center", Reference: &GeometricInput{Kind: "POINT", Point: &PatternPointReference{SketchID: f.sketchID, Reference: SketchGeometryRef{Target: "ENTITY", EntityID: "point", SubElement: "POINT"}}}}}, Support: &GeometricInput{Kind: "PLANE", PlaneID: "datum-xy"}, Radius: 3}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &c})
	c = v.Part.Features[len(v.Part.Features)-1]
	axis := f.command(CommandRequest{Type: "CREATE_DATUM_AXIS", ReferenceConstruction: &ReferenceConstruction{Mode: "LINE_COORDINATES", Coordinates: [3]float64{1, 0, 0}, Direction: [3]float64{0, 0, 1}}}).Part.DatumAxes[0]
	rotation := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "ROTATE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: c.ID, ResultID: "result"}}, Axis: &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "AXIS", EntityID: axis.ID}}, Parameters: GeometryOperationParameters{Angle: 90}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &rotation})
	rotated := v.Part.Features[len(v.Part.Features)-1]
	key := rotated.Result.GeometryKey
	v = f.command(CommandRequest{Type: "EDIT_DATUM_AXIS", TargetID: axis.ID, ReferenceConstruction: &ReferenceConstruction{Mode: "LINE_COORDINATES", Coordinates: [3]float64{2, 0, 0}, Direction: [3]float64{0, 0, 1}}})
	if v.Part.Features[len(v.Part.Features)-1].Result.GeometryKey == key {
		t.Fatal("rotation ignored upstream axis origin")
	}
	mirror := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "MIRROR", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: c.ID, ResultID: "result"}}, MirrorPlane: &GeometricInput{Kind: "PLANE", PlaneID: "datum-yz"}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &mirror})
	if result := v.Part.Features[len(v.Part.Features)-1].Result; result == nil || result.Domain != "WIRE_SET" {
		t.Fatal("mirror reference not evaluated")
	}
	// Wrong reference types fail preview without writing a Revision.
	before := v.Document.VersionID
	rotation.ID = ""
	rotation.Geometry.Axis = &GeometricInput{Kind: "PLANE", PlaneID: "datum-xy"}
	_, err := f.service.PreviewCommand(f.ctx, f.documentID, CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", RequestID: uuid.NewString(), ActorID: f.actor, Feature: &rotation})
	if err == nil {
		t.Fatal("plane accepted as rotation axis")
	}
	unchanged, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil || unchanged.Document.VersionID != before {
		t.Fatal("failed preview changed Head", err)
	}
}

func TestGeometryAssociativeExactSupportReferencesThroughRouter(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	line := testGeometryLine()
	line.ID = ""
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	line = v.Part.Features[len(v.Part.Features)-1]
	// A reusable reference axis can be constructed from an independent line's
	// exact edge. This uses the result owner rather than inventing a material Body.
	manifest, _, err := f.service.topologyManifestForGeometryKey(f.ctx, line.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	var edgeID uint64
	for _, locator := range manifest.Locators {
		if topologyType(locator.Output.TopologyType) == modelcore.PersistentTopologyEdge {
			edgeID = locator.Output.LocalId
			break
		}
	}
	selection, err := f.service.BindPersistentSelection(f.ctx, f.documentID, BindPersistentSelectionRequest{SourceVersionID: v.Document.VersionID, GeometryKey: line.Result.GeometryKey, Kind: "EDGE", LocalID: edgeID})
	if err != nil {
		t.Fatal(err)
	}
	v = f.command(CommandRequest{Type: "CREATE_DATUM_AXIS", ReferenceConstruction: &ReferenceConstruction{Mode: "LINE_POINT_DIRECTION", Inputs: []DatumReference{{Kind: "POINT", EntityID: v.Part.AxisSystems[0].ID}, {Kind: "TOPOLOGY", FeatureID: line.ID, SourceVersionID: v.Document.VersionID, Selection: &selection}}}})
	axis := v.Part.DatumAxes[len(v.Part.DatumAxes)-1]
	translated := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "TRANSLATE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: line.ID, ResultID: "result"}}, Direction: &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "AXIS", EntityID: axis.ID}}, Parameters: GeometryOperationParameters{Distance: 5}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &translated})
	translated = v.Part.Features[len(v.Part.Features)-1]
	sheet := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: line.ID, ResultID: "result"}}, Direction: &GeometricInput{Kind: "PLANE", PlaneID: "datum-xy"}, Parameters: GeometryOperationParameters{Distance: 10}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &sheet})
	sheet = v.Part.Features[len(v.Part.Features)-1]
	circle := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "CIRCLE", Curve: &GeometryCurveDefinition{Points: []GeometryControlPoint{{ID: "center", Reference: &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: v.Part.AxisSystems[0].ID}}}}, Support: &GeometricInput{Kind: "RESULT", FeatureID: sheet.ID, ResultID: "result"}, Radius: 2}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &circle})
	circle = v.Part.Features[len(v.Part.Features)-1]
	offset := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "OFFSET", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: line.ID, ResultID: "result"}, {Kind: "RESULT", FeatureID: sheet.ID, ResultID: "result"}}, Parameters: GeometryOperationParameters{Distance: 3}}}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &offset})
	offset = v.Part.Features[len(v.Part.Features)-1]
	keys := map[string]string{}
	for _, feature := range []Feature{translated, circle, offset} {
		keys[feature.ID] = feature.Result.GeometryKey
	}
	line.Geometry.Curve.Points[1].Position = [3]float64{0, 50, 0}
	digest, err := featureDefinitionDigest(*v.Part, line.ID)
	if err != nil {
		t.Fatal(err)
	}
	v = f.command(CommandRequest{Type: "EDIT_FEATURE", TargetID: line.ID, ExpectedFeatureDigest: digest, Feature: &line})
	if math.Abs(v.Part.DatumAxes[len(v.Part.DatumAxes)-1].Direction[1]-1) > 1e-9 {
		t.Fatal("reference axis lost independent result association")
	}
	for _, feature := range v.Part.Features {
		if key, ok := keys[feature.ID]; ok && (feature.Result == nil || feature.Result.GeometryKey == key) {
			t.Fatal("exact support consumer was not rebuilt", feature.ID)
		}
	}
}
