//go:build integration

package workspace

import (
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
	"math"
	"testing"
)

func controlLine(t *testing.T, f *sketchWorkflowFixture, a, b [3]float64) (DocumentView, Feature) {
	t.Helper()
	line := testGeometryLine()
	line.ID = ""
	line.Geometry.Curve.Points[0].Position = a
	line.Geometry.Curve.Points[1].Position = b
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	return v, v.Part.Features[len(v.Part.Features)-1]
}
func controlPoint(t *testing.T, f *sketchWorkflowFixture, p [3]float64) GeometricInput {
	t.Helper()
	v := f.command(CommandRequest{Type: "CREATE_REFERENCE_POINT", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: p}})
	return GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: v.Part.DatumPoints[len(v.Part.DatumPoints)-1].ID}}
}
func TestSurfaceShapeControlPreviewParametersQualityHistoryAndCold(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	_, a := controlLine(t, f, [3]float64{0, 0, 0}, [3]float64{10, 0, 0})
	v, b := controlLine(t, f, [3]float64{0, 10, 0}, [3]float64{10, 10, 0})
	plane := GeometricInput{Kind: "PLANE", PlaneID: "datum-xy"}
	id := uuid.NewString()
	d := surfaceDefinition("SURFACE_BLEND", resultInput(a), resultInput(b))
	d.Surface.Boundaries = []GeometryBoundaryControl{{ID: id, Boundary: resultInput(a), Support: &plane, Continuity: "G2", Tension: 1}, {ID: uuid.NewString(), Boundary: resultInput(b), Support: &plane, Continuity: "G1", Tension: 1}}
	request := CommandRequest{ActorID: f.actor, RequestID: uuid.NewString(), Type: "CREATE_GEOMETRY_FEATURE", Feature: &Feature{Type: "GEOMETRY", Geometry: d}}
	before := v.Document.VersionID
	preview, err := f.service.PreviewCommand(f.ctx, f.documentID, request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Artifact == nil || preview.Artifact.GeometryDomain != "SHEET_SET" {
		t.Fatal("missing patch preview")
	}
	unchanged, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil || unchanged.Document.VersionID != before {
		t.Fatal("preview mutated head", err)
	}
	request.PreviewID = preview.PreviewID
	v, err = f.service.ApplyCommand(f.ctx, f.documentID, request)
	if err != nil {
		t.Fatal(err)
	}
	blend := v.Part.Features[len(v.Part.Features)-1]
	query := ShapeQualityRequest{RevisionID: v.Document.VersionID, Input: resultInput(blend), SampleCount: 64}
	report, err := f.service.AnalyzeShapeQuality(f.ctx, f.documentID, query)
	if err != nil || !report.Result.Complete || len(report.Result.Samples) != 64 {
		t.Fatal("real routed curvature", report, err)
	}
	for _, sample := range report.Result.Samples {
		if math.Abs(sample.Maximum) > 1e-8 || math.Abs(sample.Minimum) > 1e-8 {
			t.Fatal("flat patch curvature", sample)
		}
	}
	unchanged, err = f.service.GetDocument(f.ctx, f.documentID)
	if err != nil || unchanged.Document.VersionID != v.Document.VersionID {
		t.Fatal("analysis mutated revision", err)
	}
	// Face binding is the same formal owner/result identity used by feature picks.
	manifest, _, err := f.service.topologyManifestForGeometryKey(f.ctx, blend.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	var local uint64
	for _, o := range manifest.Results[modelcore.GeometryResultReference{FeatureID: blend.ID, ResultID: "result"}].SemanticOutputs {
		if topologyType(o.TopologyType) == modelcore.PersistentTopologyFace {
			local = o.LocalId
			break
		}
	}
	bound, err := f.service.BindPersistentSelection(f.ctx, f.documentID, BindPersistentSelectionRequest{SourceVersionID: v.Document.VersionID, GeometryKey: blend.Result.GeometryKey, Kind: "FACE", LocalID: local})
	if err != nil {
		t.Fatal(err)
	}
	query.Input = GeometricInput{Kind: "TOPOLOGY", Selection: &FeatureSelection{Selection: bound, SourceVersionID: v.Document.VersionID, SourceFeatureID: blend.ID}}
	if _, err = f.service.AnalyzeShapeQuality(f.ctx, f.documentID, query); err != nil {
		t.Fatal("face analysis", err)
	}
	thickness := Feature{Type: "THICKEN", Operation: "NEW_BODY", Geometry: &GeometricFeatureDefinition{Kind: "THICKEN", Inputs: []GeometricInput{resultInput(blend)}, Parameters: GeometryOperationParameters{Distance: 2}}}
	v = f.command(CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &thickness})
	solid := v.Part.Features[len(v.Part.Features)-1]
	if math.Abs(v.Artifacts[v.Part.Bodies[len(v.Part.Bodies)-1].GeometryKey].Volume-200) > 1e-4 {
		t.Fatal("controlled surface did not thicken", solid)
	}
	b.Geometry.Curve.Points[0].Position[1] = 12
	b.Geometry.Curve.Points[1].Position[1] = 12
	digest, err := featureDefinitionDigest(*v.Part, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	v = f.command(CommandRequest{Type: "EDIT_FEATURE", TargetID: b.ID, ExpectedFeatureDigest: digest, Feature: &b})
	for _, feature := range v.Part.Features {
		if feature.EvaluationStatus != "" {
			t.Fatal("upstream update", feature)
		}
	}
	if _, err = f.service.AnalyzeShapeQuality(f.ctx, f.documentID, query); err == nil {
		t.Fatal("accepted stale analysis")
	}
	if math.Abs(v.Artifacts[v.Part.Bodies[len(v.Part.Bodies)-1].GeometryKey].Volume-240) > 1e-4 {
		t.Fatal("surface/solid association lost")
	}
	f.command(CommandRequest{Type: "UNDO"})
	v = f.command(CommandRequest{Type: "REDO"})
	cold := clonePartModel(*v.Part)
	if err = f.service.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &cold); err != nil {
		t.Fatal(err)
	}
	for _, feature := range cold.Features {
		if feature.EvaluationStatus != "" {
			t.Fatal("cold control rebuild", feature)
		}
	}
	found := false
	for _, p := range v.Part.Parameters {
		if p.OwnerFeatureID == blend.ID && p.PropertySlot == "geometry:tension:"+id {
			found = true
		}
	}
	if !found {
		t.Fatal("stable tension slot lost")
	}
}
func TestSurfaceShapeControlSpineGuidesCouplingsAndBoundary(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	_, a := controlLine(t, f, [3]float64{0, 0, 0}, [3]float64{10, 0, 0})
	_, b := controlLine(t, f, [3]float64{0, 0, 20}, [3]float64{20, 0, 20})
	_, spine := controlLine(t, f, [3]float64{0, 0, 0}, [3]float64{0, 0, 20})
	_, guide := controlLine(t, f, [3]float64{10, 0, 0}, [3]float64{20, 0, 20})
	stationA, stationB := controlPoint(t, f, [3]float64{0, 0, 0}), controlPoint(t, f, [3]float64{0, 0, 20})
	pointA, pointB := controlPoint(t, f, [3]float64{4, 0, 0}), controlPoint(t, f, [3]float64{12, 0, 20})
	id := uuid.NewString()
	d := surfaceDefinition("SURFACE_LOFT", resultInput(a), resultInput(b))
	path, orientation := resultInput(spine), resultInput(guide)
	d.Surface.Spine = &path
	d.Surface.Guide = &orientation
	d.Surface.FrameMode = "GUIDE_FRAME"
	d.Surface.Sections = []GeometrySectionControl{{ID: uuid.NewString(), Section: resultInput(a), StationPoint: &stationA, Couplings: []GeometryCouplingPoint{{ID: id, Point: &pointA}}}, {ID: uuid.NewString(), Section: resultInput(b), StationPoint: &stationB, Couplings: []GeometryCouplingPoint{{ID: id, Point: &pointB}}}}
	v, loft := surfaceCreate(t, f, d)
	manifest, _, err := f.service.topologyManifestForGeometryKey(f.ctx, loft.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	faces := 0
	for _, o := range manifest.Results[modelcore.GeometryResultReference{FeatureID: loft.ID, ResultID: "result"}].SemanticOutputs {
		if topologyType(o.TopologyType) == modelcore.PersistentTopologyFace {
			faces++
		}
	}
	if faces < 2 {
		t.Fatal("correspondence spans missing")
	}
	f.command(CommandRequest{Type: "UNDO"})
	v = f.command(CommandRequest{Type: "REDO"})
	cold := clonePartModel(*v.Part)
	if err := f.service.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &cold); err != nil {
		t.Fatal(err)
	}
	// A separate four-edge engineering patch, with per-edge full polynomial jets.
	_, bottom := controlLine(t, f, [3]float64{30, 0, 0}, [3]float64{40, 0, 0})
	_, right := controlLine(t, f, [3]float64{40, 0, 0}, [3]float64{40, 10, 0})
	_, top := controlLine(t, f, [3]float64{40, 10, 0}, [3]float64{30, 10, 0})
	_, left := controlLine(t, f, [3]float64{30, 10, 0}, [3]float64{30, 0, 0})
	plane := GeometricInput{Kind: "PLANE", PlaneID: "datum-xy"}
	d = surfaceDefinition("SURFACE_BOUNDARY", resultInput(bottom), resultInput(right), resultInput(top), resultInput(left))
	d.Surface.Boundaries = []GeometryBoundaryControl{{ID: uuid.NewString(), Boundary: resultInput(bottom), Continuity: "G2", Support: &plane, Tension: 1}}
	v, patch := surfaceCreate(t, f, d)
	if v.Artifacts[patch.Result.GeometryKey].GeometryDomain != "SHEET_SET" {
		t.Fatal("quad result domain")
	}
	d.Kind = "SURFACE_FILL"
	surfaceCreate(t, f, d)
}
func TestSurfaceShapeControlBorderGuideAssociation(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	_, profile := controlLine(t, f, [3]float64{0, 0, 0}, [3]float64{10, 0, 0})
	_, path := controlLine(t, f, [3]float64{0, 0, 0}, [3]float64{0, 0, 20})
	curve := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SPLINE", Curve: &GeometryCurveDefinition{Points: []GeometryControlPoint{{ID: "a", Position: [3]float64{10, 0, 0}}, {ID: "b", Position: [3]float64{15, 4, 5}}, {ID: "c", Position: [3]float64{8, 4, 15}}, {ID: "d", Position: [3]float64{20, 0, 20}}}, Degree: 3, Knots: []float64{0, 1}, Multiplicities: []uint32{4, 4}, Weights: []float64{1, 1, 1, 1}}}}
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &curve})
	rail := v.Part.Features[len(v.Part.Features)-1]
	guide := resultInput(rail)
	d := surfaceDefinition("SURFACE_SWEEP", resultInput(profile), resultInput(path))
	d.Surface.Guide = &guide
	d.Surface.FrameMode = "GUIDE_BORDER"
	v, surface := surfaceCreate(t, f, d)
	before := surface.Result.GeometryKey
	rail.Geometry.Curve.Points[1].Position[0] = 18
	digest, err := featureDefinitionDigest(*v.Part, rail.ID)
	if err != nil {
		t.Fatal(err)
	}
	v = f.command(CommandRequest{Type: "EDIT_FEATURE", TargetID: rail.ID, ExpectedFeatureDigest: digest, Feature: &rail})
	updated := v.Part.Features[len(v.Part.Features)-1]
	if updated.EvaluationStatus != "" || updated.Result.GeometryKey == before {
		t.Fatal("boundary guide did not drive surface", updated)
	}
	f.command(CommandRequest{Type: "UNDO"})
	v = f.command(CommandRequest{Type: "REDO"})
	cold := clonePartModel(*v.Part)
	if err = f.service.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &cold); err != nil {
		t.Fatal(err)
	}
}
