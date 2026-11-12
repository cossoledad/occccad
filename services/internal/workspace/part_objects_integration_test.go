//go:build integration

package workspace

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
)

func lifecycleFeature(t *testing.T, v DocumentView, id string) Feature {
	t.Helper()
	for _, f := range v.Part.Features {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("feature %s missing", id)
	return Feature{}
}

func TestPartObjectsRouterDeletionRepairCompensationAndContainers(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	line := testGeometryLine()
	line.ID = ""
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	base := v.Part.Features[len(v.Part.Features)-1]
	line.Geometry.Curve.Points[1].Position[0] = 30
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	replacement := v.Part.Features[len(v.Part.Features)-1]
	d := &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{resultInput(base)}, Parameters: GeometryOperationParameters{Distance: 20, Direction: [3]float64{0, 0, 1}}}
	v, sheet := surfaceCreate(t, f, d)
	v = f.command(CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &Feature{Type: "THICKEN", Operation: "NEW_BODY", Geometry: &GeometricFeatureDefinition{Kind: "THICKEN", Inputs: []GeometricInput{resultInput(sheet)}, Parameters: GeometryOperationParameters{Distance: 2}}}})
	solid := v.Part.Features[len(v.Part.Features)-1]
	if v.Part.Bodies[len(v.Part.Bodies)-1].GeometryKey == "" {
		t.Fatal("thickening has no exact body")
	}
	targets := []DeleteNodeTarget{{TargetKind: "FEATURE", TargetID: base.ID}}
	plan, err := f.service.GetPartDeletePlan(f.ctx, f.documentID, PartDeletePlanRequest{VersionID: v.Document.VersionID, Targets: targets})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Affected) < 2 {
		t.Fatal("downstream impact missing", plan)
	}
	unchanged, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil || unchanged.Document.VersionID != v.Document.VersionID {
		t.Fatal("planning mutated Head", err)
	}
	v = f.command(CommandRequest{Type: "DELETE_NODES", VersionID: plan.VersionID, Targets: targets, DeletePlanDigest: plan.Digest})
	if lifecycleFeature(t, v, sheet.ID).EvaluationStatus != "FAILED" || lifecycleFeature(t, v, solid.ID).EvaluationStatus != "BLOCKED" {
		t.Fatal("missing/blocked states were not preserved")
	}
	if lifecycleFeature(t, v, sheet.ID).Result != nil {
		t.Fatal("unavailable sheet retained an exact result")
	}
	digest, err := featureDefinitionDigest(*v.Part, sheet.ID)
	if err != nil {
		t.Fatal(err)
	}
	repaired := lifecycleFeature(t, v, sheet.ID)
	repaired.Geometry.Inputs = []GeometricInput{resultInput(replacement)}
	v = f.command(CommandRequest{Type: "EDIT_FEATURE", TargetID: sheet.ID, ExpectedFeatureDigest: digest, Feature: &repaired})
	if lifecycleFeature(t, v, solid.ID).EvaluationStatus != "" || len(v.Part.RetiredReferences) != 0 {
		t.Fatal("input repair did not recover the body or prune missing descriptors")
	}
	v = f.command(CommandRequest{Type: "UNDO"})
	if lifecycleFeature(t, v, sheet.ID).EvaluationStatus != "FAILED" {
		t.Fatal("Undo repair did not restore broken definition")
	}
	v = f.command(CommandRequest{Type: "UNDO"})
	if lifecycleFeature(t, v, sheet.ID).EvaluationStatus != "" || lifecycleFeature(t, v, base.ID).Result == nil {
		t.Fatal("Undo deletion did not restore stable identities")
	}
	f.command(CommandRequest{Type: "REDO"})
	v = f.command(CommandRequest{Type: "REDO"})
	if lifecycleFeature(t, v, solid.ID).EvaluationStatus != "" {
		t.Fatal("Redo repair failed")
	}
	// Organization and styling keep exact identities and downstream references.
	key := lifecycleFeature(t, v, replacement.ID).Result.GeometryKey
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRICAL_SET", Name: "Delivery destination"})
	set := v.Part.GeometricalSets[len(v.Part.GeometricalSets)-1]
	v = f.command(CommandRequest{Type: "MOVE_TO_GEOMETRICAL_SET", GeometricalSetID: set.ID, Targets: []DeleteNodeTarget{{TargetKind: "FEATURE", TargetID: replacement.ID}}})
	color := json.RawMessage(`"#12ab34"`)
	v = f.command(CommandRequest{Type: "SET_PROPERTIES", VersionID: v.Document.VersionID, Properties: &PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "GEOMETRICAL_SET", ID: set.ID}}, Appearance: map[string]json.RawMessage{"color": color, "edgeWidth": json.RawMessage(`3`)}}})
	if lifecycleFeature(t, v, replacement.ID).Result.GeometryKey != key || v.ResultAppearances[replacement.ID].Color != "#12ab34" {
		t.Fatal("move/style changed geometry or lost inheritance")
	}
	sheet = lifecycleFeature(t, v, sheet.ID)
	manifest, _, err := f.service.topologyManifestForGeometryKey(f.ctx, sheet.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	var faceID uint64
	resultRef := modelcore.GeometryResultReference{FeatureID: sheet.ID, ResultID: "result"}
	for _, out := range manifest.Results[resultRef].SemanticOutputs {
		if topologyType(out.TopologyType) == modelcore.PersistentTopologyFace {
			faceID = out.LocalId
			break
		}
	}
	if faceID == 0 {
		t.Fatal("surface face identity missing")
	}
	v = f.command(CommandRequest{Type: "SET_PROPERTIES", VersionID: v.Document.VersionID, Properties: &PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "FACE", ID: "selected-face", GeometryKey: sheet.Result.GeometryKey, LocalID: faceID, SourceResult: &resultRef}}, Appearance: map[string]json.RawMessage{"color": json.RawMessage(`"#ff2200"`)}}})
	if v.ResultAppearances[sheet.ID].Faces[faceID] != "#ff2200" || lifecycleFeature(t, v, sheet.ID).Result.GeometryKey != sheet.Result.GeometryKey {
		t.Fatal("independent face appearance lost owner or exact identity")
	}

	parent := lifecycleFeature(t, v, sheet.ID).GeometricalSetID
	v = f.command(CommandRequest{Type: "DISSOLVE_GEOMETRICAL_SET", TargetID: set.ID, GeometricalSetID: parent})
	if lifecycleFeature(t, v, replacement.ID).GeometricalSetID != parent || lifecycleFeature(t, v, replacement.ID).Result.GeometryKey != key {
		t.Fatal("dissolve failed to retain geometry and membership")
	}
	v = f.command(CommandRequest{Type: "UNDO"})
	if lifecycleFeature(t, v, replacement.ID).GeometricalSetID != set.ID {
		t.Fatal("dissolve compensation failed")
	}
	// The same plan is rejected after an intervening Revision, even for metadata.
	_, err = f.service.GetPartDeletePlan(f.ctx, f.documentID, PartDeletePlanRequest{VersionID: plan.VersionID, Targets: targets})
	if err == nil {
		t.Fatal("stale impact plan accepted")
	}
	// A plan binds Revision as well as identical model content after Undo/Redo.
	survivorTargets := []DeleteNodeTarget{{TargetKind: "FEATURE", TargetID: replacement.ID}}
	frozen, err := f.service.GetPartDeletePlan(f.ctx, f.documentID, PartDeletePlanRequest{VersionID: v.Document.VersionID, Targets: survivorTargets})
	if err != nil {
		t.Fatal(err)
	}
	f.command(CommandRequest{Type: "REDO"})
	v = f.command(CommandRequest{Type: "UNDO"})
	_, err = f.service.ApplyCommand(f.ctx, f.documentID, CommandRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: "DELETE_NODES", VersionID: frozen.VersionID, Targets: survivorTargets, DeletePlanDigest: frozen.Digest})
	if err == nil {
		t.Fatal("stale plan reused across identical-model Revision")
	}
	bodyID := lifecycleFeature(t, v, solid.ID).BodyID
	v = f.command(CommandRequest{Type: "DELETE_BODY", BodyID: bodyID})
	for _, feature := range v.Part.Features {
		if feature.ID == solid.ID {
			t.Fatal("Body deletion retained owned material feature")
		}
	}
	if lifecycleFeature(t, v, sheet.ID).Result == nil {
		t.Fatal("Body deletion removed independent source")
	}
	v = f.command(CommandRequest{Type: "UNDO"})
	if lifecycleFeature(t, v, solid.ID).BodyID != bodyID {
		t.Fatal("Body Undo changed identity")
	}

	cold := clonePartModel(*v.Part)
	if err = f.service.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &cold); err != nil {
		t.Fatal(err)
	}
	for _, feature := range cold.Features {
		if feature.EvaluationStatus != "" {
			t.Fatalf("cold rebuild: %+v", feature)
		}
	}
	t.Logf("Part lifecycle acceptance document=%s", f.documentID)
}

func TestSurfaceRevolveCurveSplitMutualTrimAndBoundaryQualityRouter(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	line := testGeometryLine()
	line.ID = ""
	line.Geometry.Curve.Points[0].Position = [3]float64{10, 0, 0}
	line.Geometry.Curve.Points[1].Position = [3]float64{10, 0, 20}
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	profile := v.Part.Features[len(v.Part.Features)-1]
	v = f.command(CommandRequest{Type: "CREATE_DATUM_AXIS", ReferenceConstruction: &ReferenceConstruction{Mode: "LINE_COORDINATES", Direction: [3]float64{0, 0, 1}}})
	axis := v.Part.DatumAxes[len(v.Part.DatumAxes)-1]
	d := surfaceDefinition("SURFACE_REVOLVE", resultInput(profile))
	d.Surface.Tolerance = .01
	d.Parameters.Angle = 360
	d.Axis = &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "AXIS", EntityID: axis.ID}}
	request := CommandRequest{ActorID: f.actor, RequestID: uuid.NewString(), Type: "CREATE_GEOMETRY_FEATURE", Feature: &Feature{Type: "GEOMETRY", Geometry: d}}
	preview, err := f.service.PreviewCommand(f.ctx, f.documentID, request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Artifact == nil || preview.Artifact.GeometryDomain != "SHEET_SET" {
		t.Fatal("revolve preview absent")
	}
	request.PreviewID = preview.PreviewID
	v, err = f.service.ApplyCommand(f.ctx, f.documentID, request)
	if err != nil {
		t.Fatal(err)
	}
	revolved := v.Part.Features[len(v.Part.Features)-1]
	report, err := f.service.AnalyzeShapeQuality(f.ctx, f.documentID, ShapeQualityRequest{RevisionID: v.Document.VersionID, Input: resultInput(revolved), SampleCount: 64})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Result.Complete || !report.Result.BoundaryComplete || len(report.Result.BoundaryEdges) != 2 {
		t.Fatal("cylinder free edges include seam or are missing", report.Result)
	}
	for _, s := range report.Result.Samples {
		if !s.Surface || math.Abs(math.Abs(s.Mean)-.05) > 1e-6 {
			t.Fatal("incorrect exact cylinder curvature", s)
		}
	}
	// A plane sheet split by a curve lying on it, then mutual sheet trimming.
	line.Geometry.Curve.Points[0].Position = [3]float64{0, 0, 0}
	line.Geometry.Curve.Points[1].Position = [3]float64{50, 0, 0}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	base := v.Part.Features[len(v.Part.Features)-1]
	extrude := func(source Feature, direction [3]float64) Feature {
		d := &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{resultInput(source)}, Direction: &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "AXIS", EntityID: axis.ID}}, Parameters: GeometryOperationParameters{Distance: 20, Direction: direction}}
		var result Feature
		v, result = surfaceCreate(t, f, d)
		return result
	}
	sheet := extrude(base, [3]float64{0, 0, 1})
	line.Geometry.Curve.Points[0].Position = [3]float64{25, 0, 0}
	line.Geometry.Curve.Points[1].Position = [3]float64{25, 0, 20}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	cut := v.Part.Features[len(v.Part.Features)-1]
	v = f.command(CommandRequest{Type: "CREATE_REFERENCE_POINT", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{5, 0, 5}}})
	keep := v.Part.DatumPoints[len(v.Part.DatumPoints)-1]
	d = surfaceDefinition("SURFACE_TRIM", resultInput(sheet), resultInput(cut))
	d.KeepPoint = &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: keep.ID}}
	v, split := surfaceCreate(t, f, d)
	manifest, _, err := f.service.topologyManifestForGeometryKey(f.ctx, split.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Results[modelcore.GeometryResultReference{FeatureID: split.ID, ResultID: "result"}].SemanticOutputs) == 0 {
		t.Fatal("split naming missing")
	}
	line.Geometry.Curve.Points[0].Position = [3]float64{0, -10, 10}
	line.Geometry.Curve.Points[1].Position = [3]float64{50, -10, 10}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	crossBase := v.Part.Features[len(v.Part.Features)-1]
	v = f.command(CommandRequest{Type: "CREATE_DATUM_AXIS", ReferenceConstruction: &ReferenceConstruction{Mode: "LINE_COORDINATES", Direction: [3]float64{0, 1, 0}}})
	crossAxis := v.Part.DatumAxes[len(v.Part.DatumAxes)-1]
	d = &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{resultInput(crossBase)}, Direction: &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "AXIS", EntityID: crossAxis.ID}}, Parameters: GeometryOperationParameters{Distance: 20}}
	v, other := surfaceCreate(t, f, d)
	v = f.command(CommandRequest{Type: "CREATE_REFERENCE_POINT", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{5, 5, 10}}})
	otherKeep := v.Part.DatumPoints[len(v.Part.DatumPoints)-1]
	d = surfaceDefinition("SURFACE_MUTUAL_TRIM", resultInput(sheet), resultInput(other))
	d.KeepPoint = &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: keep.ID}}
	d.SecondKeepPoint = &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: otherKeep.ID}}
	v, mutual := surfaceCreate(t, f, d)
	if mutual.Result.Domain != "SHEET_SET" {
		t.Fatal("mutual trim domain")
	}
	cold := clonePartModel(*v.Part)
	if err = f.service.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &cold); err != nil {
		t.Fatal(err)
	}
	for _, feature := range cold.Features {
		if feature.EvaluationStatus != "" {
			t.Fatal("cold rebuild failed", feature.Diagnostic)
		}
	}
	t.Logf("Surface delivery acceptance document=%s", f.documentID)
}
