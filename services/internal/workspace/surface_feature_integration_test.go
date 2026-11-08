//go:build integration

package workspace

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func surfaceDefinition(kind string, inputs ...GeometricInput) *GeometricFeatureDefinition {
	return &GeometricFeatureDefinition{Kind: kind, Inputs: inputs, Surface: &GeometrySurfaceDefinition{Tolerance: 1e-6, FrameMode: "CORRECTED_FRENET"}}
}
func resultInput(f Feature) GeometricInput {
	return GeometricInput{Kind: "RESULT", FeatureID: f.ID, ResultID: "result"}
}
func surfaceCreate(t *testing.T, f *sketchWorkflowFixture, d *GeometricFeatureDefinition) (DocumentView, Feature) {
	t.Helper()
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &Feature{Type: "GEOMETRY", Geometry: d}})
	feature := v.Part.Features[len(v.Part.Features)-1]
	if feature.Result == nil || feature.EvaluationStatus != "" {
		t.Fatalf("%s failed: %+v", d.Kind, feature)
	}
	if a := v.Artifacts[feature.Result.GeometryKey]; !a.Naming.CanBind || a.BodyID != "" {
		t.Fatalf("%s lost independent naming", d.Kind)
	}
	return v, feature
}

func TestSurfaceEngineeringAssociationsPreviewHistoryAndColdRebuild(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	line := testGeometryLine()
	line.ID = ""
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	base := v.Part.Features[len(v.Part.Features)-1]
	rail := testGeometryLine()
	rail.ID = ""
	rail.Geometry.Curve.Points[1].Position = [3]float64{0, 0, 20}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &rail})
	path := v.Part.Features[len(v.Part.Features)-1]
	sweep := Feature{Type: "GEOMETRY", Geometry: surfaceDefinition("SURFACE_SWEEP", resultInput(base), resultInput(path))}
	request := CommandRequest{ActorID: f.actor, RequestID: uuid.NewString(), Type: "CREATE_GEOMETRY_FEATURE", Feature: &sweep}
	before := v.Document.VersionID
	preview, err := f.service.PreviewCommand(f.ctx, f.documentID, request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Artifact == nil || preview.Artifact.GeometryDomain != "SHEET_SET" {
		t.Fatal("sheet preview missing")
	}
	unchanged, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil || unchanged.Document.VersionID != before {
		t.Fatal("preview mutated revision", err)
	}
	request.PreviewID = preview.PreviewID
	v, err = f.service.ApplyCommand(f.ctx, f.documentID, request)
	if err != nil {
		t.Fatal(err)
	}
	sheet := v.Part.Features[len(v.Part.Features)-1]
	if sheet.Result == nil {
		t.Fatal("sweep did not commit")
	}
	d := surfaceDefinition("SURFACE_OFFSET", resultInput(sheet))
	d.Parameters.Distance = 3
	v, offset := surfaceCreate(t, f, d)
	a := v.Artifacts[offset.Result.GeometryKey]
	if a.Volume != 0 {
		t.Fatal("sheet acquired material volume")
	}
	// The selected extension boundary keeps the exact source-result identity.
	manifest, _, err := f.service.topologyManifestForGeometryKey(f.ctx, sheet.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	var local uint64
	for _, o := range manifest.Results[modelcore.GeometryResultReference{FeatureID: sheet.ID, ResultID: "result"}].SemanticOutputs {
		if topologyType(o.TopologyType) == modelcore.PersistentTopologyEdge {
			local = o.LocalId
			break
		}
	}
	selection, err := f.service.BindPersistentSelection(f.ctx, f.documentID, BindPersistentSelectionRequest{SourceVersionID: v.Document.VersionID, GeometryKey: sheet.Result.GeometryKey, Kind: "EDGE", LocalID: local})
	if err != nil {
		t.Fatal(err)
	}
	sourceRevision := v.Document.VersionID
	d = surfaceDefinition("SURFACE_EXTEND", resultInput(sheet), GeometricInput{Kind: "TOPOLOGY", Selection: &FeatureSelection{Selection: selection, SourceVersionID: sourceRevision, SourceFeatureID: sheet.ID}})
	d.Parameters.Distance = 5
	v, _ = surfaceCreate(t, f, d)
	// A cutter is finite design geometry; the retained side is an upstream point.
	cutterLine := testGeometryLine()
	cutterLine.ID = ""
	cutterLine.Geometry.Curve.Points[0].Position = [3]float64{25, -10, 0}
	cutterLine.Geometry.Curve.Points[1].Position = [3]float64{25, 10, 0}
	v = f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &cutterLine})
	cutterCurve := v.Part.Features[len(v.Part.Features)-1]
	v, cutter := surfaceCreate(t, f, surfaceDefinition("SURFACE_SWEEP", resultInput(cutterCurve), resultInput(path)))
	v = f.command(CommandRequest{Type: "CREATE_REFERENCE_POINT", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{5, 0, 5}}})
	point := v.Part.DatumPoints[len(v.Part.DatumPoints)-1]
	d = surfaceDefinition("SURFACE_TRIM", resultInput(sheet), resultInput(cutter))
	d.KeepPoint = &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: point.ID}}
	v, trimmed := surfaceCreate(t, f, d)
	bounds := v.Artifacts[trimmed.Result.GeometryKey].BBox
	if max, ok := bounds["max"].([]float64); ok && math.Abs(max[0]-25) > 1e-5 {
		t.Fatal("wrong retained region", bounds)
	}
	// Editing the rail length changes sweep, extension, cutter and trim through DAG.
	path.Geometry.Curve.Points[1].Position[2] = 30
	digest, err := featureDefinitionDigest(*v.Part, path.ID)
	if err != nil {
		t.Fatal(err)
	}
	v = f.command(CommandRequest{Type: "EDIT_FEATURE", TargetID: path.ID, ExpectedFeatureDigest: digest, Feature: &path})
	for _, feature := range v.Part.Features {
		if feature.EvaluationStatus != "" {
			t.Fatalf("upstream update: %+v", feature)
		}
	}
	resolved, err := f.service.ResolvePersistentSelection(f.ctx, f.documentID, ResolvePersistentSelectionRequest{Selection: selection, SourceVersionID: sourceRevision, TargetVersionID: v.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if err != nil || resolved.Status != modelcore.SelectionResolved {
		t.Fatal("boundary reference lost", resolved, err)
	}
	f.command(CommandRequest{Type: "UNDO"})
	v = f.command(CommandRequest{Type: "REDO"})
	cold := clonePartModel(*v.Part)
	if err := f.service.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &cold); err != nil {
		t.Fatal(err)
	}
	for _, feature := range cold.Features {
		if feature.EvaluationStatus != "" {
			t.Fatalf("cold rebuild: %+v", feature)
		}
	}
	t.Logf("engineering surface chain document=%s", f.documentID)
}

func TestSurfaceLoftFillSewCloseBodyAndOpenExchange(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	v := f.edit(SketchOperation{Type: "ADD_RECTANGLE", First: &SketchPoint2{}, Second: &SketchPoint2{X: 10, Y: 10}})
	var entities []string
	for _, e := range f.sketch(v).Entities {
		entities = append(entities, e.ID)
	}
	bottom := GeometricInput{Kind: "SKETCH", FeatureID: f.sketchID, EntityIDs: entities}
	v, wire := surfaceCreate(t, f, &GeometricFeatureDefinition{Kind: "EXTRACT", Inputs: []GeometricInput{bottom}})
	translated := &GeometricFeatureDefinition{Kind: "TRANSLATE", Inputs: []GeometricInput{resultInput(wire)}, Parameters: GeometryOperationParameters{Direction: [3]float64{0, 0, 1}, Distance: 10}}
	v, topWire := surfaceCreate(t, f, translated)
	v, walls := surfaceCreate(t, f, surfaceDefinition("SURFACE_LOFT", resultInput(wire), resultInput(topWire)))
	v, bottomFace := surfaceCreate(t, f, surfaceDefinition("SURFACE_FILL", resultInput(wire)))
	v, topFace := surfaceCreate(t, f, surfaceDefinition("SURFACE_FILL", resultInput(topWire)))
	v, _ = surfaceCreate(t, f, surfaceDefinition("SURFACE_JOIN", resultInput(walls), resultInput(bottomFace), resultInput(topFace)))
	v, shell := surfaceCreate(t, f, surfaceDefinition("SURFACE_SEW", resultInput(walls), resultInput(bottomFace), resultInput(topFace)))
	v = f.command(CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &Feature{Type: "CLOSE_SURFACE", Operation: "NEW_BODY", Geometry: surfaceDefinition("CLOSE_SURFACE", resultInput(shell))}})
	f.volume(v, 1000)
	closed := v.Part.Features[len(v.Part.Features)-1]
	if closed.Type != "CLOSE_SURFACE" || closed.EvaluationStatus != "" {
		t.Fatal("close did not create material")
	}
	f.command(CommandRequest{Type: "UNDO"})
	v = f.command(CommandRequest{Type: "REDO"})
	f.volume(v, 1000)
	cold := clonePartModel(*v.Part)
	if err := f.service.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &cold); err != nil {
		t.Fatal(err)
	}
	graph, err := f.service.ExchangeExportGraph(f.ctx, f.documentID, v.Document.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Definitions[0].Geometries) == 0 || len(graph.Definitions[0].Bodies) == 0 {
		t.Fatal("mixed export lost independent geometry or Body")
	}
	// STEP and BREP both preserve an open sheet and acquire frozen persistent IDs.
	ref, err := f.service.brepArtifactReference(f.ctx, walls.Result.GeometryKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"STEP", "BREP"} {
		t.Run(format, func(t *testing.T) {
			request := uuid.NewString()
			source := geometry.ArtifactReference{Backend: ref.Backend, ObjectKey: ref.ObjectKey, SHA256: ref.SHA256, Size: ref.Size, ContentType: ref.ContentType}
			out, err := f.service.worker.ExportExchange(f.ctx, request, format, artifact.StagingKey(request, "open."+format), geometry.SinglePartExchangeGraph("sheet", source))
			if err != nil {
				t.Fatal(err)
			}
			object, err := f.service.artifacts.Adopt(f.ctx, artifact.KindExchangeSource, out.ContentType, out.ObjectKey)
			if err != nil {
				t.Fatal(err)
			}
			source = geometry.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, SHA256: object.SHA256, Size: object.Size, ContentType: object.ContentType}
			key := "open-import-" + request
			evaluation, err := f.service.worker.ImportExchange(f.ctx, request, key, format, source, "", artifact.StagingKey(request, "shape.brep"), artifact.StagingKey(request, "mesh.glb"))
			if err != nil {
				t.Fatal(err)
			}
			imported, err := f.service.CommitImportedPart(f.ctx, f.actor, "", request, "Open "+format, "sheet."+format, format, key, evaluation, &ImportSource{ObjectID: object.ID, SHA256: object.SHA256, Format: format})
			if err != nil {
				t.Fatal(err)
			}
			root := imported.Part.Features[0]
			if root.Type != "GEOMETRY" || root.ImportDefinitionID == "" || root.BodyID != "" || root.Result == nil || root.Result.Domain != "SHEET_SET" {
				t.Fatalf("open import misclassified: %+v", root)
			}
			selection, err := f.service.BindPersistentSelection(f.ctx, imported.Document.ID, BindPersistentSelectionRequest{SourceVersionID: imported.Document.VersionID, GeometryKey: root.Result.GeometryKey, Kind: "FACE", LocalID: 1})
			if err != nil || selection.SourceResult == nil || selection.SourceBodyID != "" {
				t.Fatal("open import naming unavailable", err)
			}
			frozen, _ := json.Marshal(imported.Part)
			cold := NewWithArtifacts(f.service.database, f.service.worker, f.service.artifacts)
			reopened, err := cold.GetDocument(f.ctx, imported.Document.ID)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.Marshal(reopened.Part)
			if string(got) != string(frozen) {
				t.Fatal("reopen changed imported definition")
			}
			candidate := clonePartModel(*reopened.Part)
			if err := cold.evaluatePartBodies(WithColdEvaluation(f.ctx), uuid.NewString(), &candidate); err != nil {
				t.Fatal(err)
			}
			if candidate.Features[0].EvaluationStatus != "" {
				t.Fatal("cold import failed")
			}
		})
	}
}

func TestSurfaceWirePointAndMixedGeometryExchange(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	line := testGeometryLine()
	line.ID = ""
	v := f.command(CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	wire := v.Part.Features[len(v.Part.Features)-1]
	v, point := surfaceCreate(t, f, &GeometricFeatureDefinition{Kind: "POINT_ON_CURVE", Inputs: []GeometricInput{resultInput(wire)}, Parameters: GeometryOperationParameters{Start: .5}})
	v, sheet := surfaceCreate(t, f, &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{resultInput(wire)}, Parameters: GeometryOperationParameters{Direction: [3]float64{0, 0, 1}, Distance: 10}})
	v = f.command(CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &Feature{Type: "THICKEN", Operation: "NEW_BODY", Geometry: &GeometricFeatureDefinition{Kind: "THICKEN", Inputs: []GeometricInput{resultInput(sheet)}, Parameters: GeometryOperationParameters{Distance: 2}}}})
	mixed, err := f.service.ExchangeExportGraph(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name, domain string
		feature      Feature
	}{{"wire", "WIRE_SET", wire}, {"point", "POINT_SET", point}, {"mixed", "MIXED_SET", Feature{}}} {
		for _, format := range []string{"STEP", "BREP"} {
			t.Run(entry.name+"/"+format, func(t *testing.T) {
				request := uuid.NewString()
				graph := mixed
				if entry.feature.ID != "" {
					ref, err := f.service.brepArtifactReference(f.ctx, entry.feature.Result.GeometryKey)
					if err != nil {
						t.Fatal(err)
					}
					graph = geometry.SinglePartExchangeGraph(entry.name, ref)
				}
				out, err := f.service.worker.ExportExchange(f.ctx, request, format, artifact.StagingKey(request, "geometry."+format), graph)
				if err != nil {
					t.Fatal(err)
				}
				object, err := f.service.artifacts.Adopt(f.ctx, artifact.KindExchangeSource, out.ContentType, out.ObjectKey)
				if err != nil {
					t.Fatal(err)
				}
				ref := geometry.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, SHA256: object.SHA256, Size: object.Size, ContentType: object.ContentType}
				key := "geometry-import-" + request
				result, err := f.service.worker.ImportExchange(f.ctx, request, key, format, ref, "", artifact.StagingKey(request, "shape.brep"), artifact.StagingKey(request, "mesh.glb"))
				if err != nil {
					t.Fatal(err)
				}
				if geometryDomain(result) != entry.domain {
					t.Fatalf("lost domain: %s -> %s", entry.domain, geometryDomain(result))
				}
				imported, err := f.service.CommitImportedPart(f.ctx, f.actor, "", request, entry.name, entry.name+"."+format, format, key, result, &ImportSource{ObjectID: object.ID, SHA256: object.SHA256, Format: format})
				if err != nil {
					t.Fatal(err)
				}
				root := imported.Part.Features[0]
				if root.Result == nil || root.Result.Domain != entry.domain || !imported.Artifacts[root.Result.GeometryKey].Naming.CanBind {
					t.Fatal("import did not preserve named independent result")
				}
			})
		}
	}
}
