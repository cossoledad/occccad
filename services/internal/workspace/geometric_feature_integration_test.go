//go:build integration

package workspace

import (
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
