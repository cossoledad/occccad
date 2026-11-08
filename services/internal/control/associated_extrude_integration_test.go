//go:build integration

package control

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssociatedExtrudeLifecycleThroughRouter(t *testing.T) {
	for _, extent := range []string{"TO_NEXT", "TO_SURFACE"} {
		t.Run(extent, func(t *testing.T) {
			service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: extent + " offset lifecycle"})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				if r.RequestID == "" {
					r.RequestID = fmt.Sprintf("%s-associated-%d", view.Document.ID, seq)
				}
				view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			rectangle := func(x, y, w, h float64) string {
				t.Helper()
				apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				id := view.Part.Features[len(view.Part.Features)-1].ID
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + w, Y: y + h}}}})
				return id
			}
			volume := func(want float64) {
				t.Helper()
				if got := activeBodyArtifact(t, view).Volume; math.Abs(got-want) > 1e-5 {
					t.Fatal("volume", got, want)
				}
			}
			apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "allowance", Value: 1, Unit: "mm"})
			allowance := view.Part.Parameters[0].ParameterID
			stockSketch := rectangle(-10, -10, 20, 20)
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stockSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
			stock := view.Part.Features[len(view.Part.Features)-1]
			var surface *workspace.FeatureSelection
			if extent == "TO_SURFACE" {
				face := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
					return p.Anchor.FeatureID == stock.ID && strings.HasPrefix(p.Anchor.OutputSlot, "END_CAP/")
				})
				surface = &workspace.FeatureSelection{Selection: face.Selection, SourceVersionID: face.SourceVersionID}
			}
			profile := rectangle(-1, -1, 2, 2)
			apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Start", Origin: [3]float64{0, 0, 5}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", SketchID: profile, DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID, VersionID: view.Document.VersionID})
			request := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-associated-preview", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Extent: extent, TerminationSurface: surface, ParameterExpressions: map[string]string{"startOffset": "2*allowance", "endOffset": "-3*allowance"}}
			head := view.Document.VersionID
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
			if e != nil || preview.Artifact == nil {
				t.Fatal("preview", e)
			}
			read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || read.Document.VersionID != head {
				t.Fatal("preview advanced Head", e)
			}
			request.PreviewID = preview.PreviewID
			apply(request)
			cut := view.Part.Features[len(view.Part.Features)-1]
			volume(7960)
			if cut.StartOffset != 2 || cut.EndOffset != -3 || extent == "TO_SURFACE" && (cut.TerminationSurface == nil || cut.TerminationSurface.SourceFeatureID != stock.ID) {
				t.Fatal("definition not qualified", cut)
			}
			startID, endID := "", ""
			for _, p := range view.Part.Parameters {
				if p.OwnerFeatureID == cut.ID {
					if p.PropertySlot == "startOffset" {
						startID = p.ParameterID
					}
					if p.PropertySlot == "endOffset" {
						endID = p.ParameterID
					}
					if p.PropertySlot == "length" {
						t.Fatal("inactive driving length")
					}
				}
			}
			if startID == "" || endID == "" {
				t.Fatal("offset parameters missing")
			}
			apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: allowance, Value: 2, Unit: "mm"})
			volume(7980)
			stock.Length = 25
			apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: stock.ID, Feature: &stock, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, stock.ID)})
			volume(9960)
			apply(workspace.CommandRequest{Type: "UNDO"})
			volume(7980)
			apply(workspace.CommandRequest{Type: "REDO"})
			volume(9960)
			if extent == "TO_SURFACE" {
				input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: cut.ID})
				if e != nil || input.TerminationPick == nil || input.TerminationPick.Status != modelcore.SelectionResolved {
					t.Fatal("surface input repair view", e, input.TerminationPick)
				}
			}
			fresh := workspace.NewWithArtifacts(db, client, artifacts)
			view, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			volume(9960)
			for _, f := range view.Part.Features {
				if f.ID == cut.ID {
					cut = f
				}
			}
			if cut.StartOffset != 4 || cut.EndOffset != -6 {
				t.Fatal("formula not reevaluated", cut)
			}
			final := activeBodyArtifact(t, view)
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-associated-cold", TargetID: cut.ID, Feature: &cut, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, cut.ID)})
			if e != nil || cold.Artifact == nil {
				t.Fatal("cold", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
					t.Fatal("cold mismatch", role)
				}
			}
		})
	}
}

func TestCurvedTerminationUpdatesAndRequiresExplicitRepairThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Curved termination repair"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-curved-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	saved := func(id string) workspace.Feature {
		for _, f := range view.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("missing feature", id)
		return workspace.Feature{}
	}
	rectangle := func(a, b workspace.SketchPoint2) string {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &a, Second: &b}}})
		return id
	}
	cylinder := func() (workspace.Feature, string) {
		t.Helper()
		sketch := rectangle(workspace.SketchPoint2{}, workspace.SketchPoint2{X: 20, Y: 10})
		bottom, side := "", ""
		for _, e := range saved(sketch).Sketch.Entities {
			if e.Start != nil && e.End != nil {
				if e.Start.Y == 0 && e.End.Y == 0 {
					bottom = e.ID
				}
				if e.Start.X == e.End.X {
					side = e.ID
				}
			}
		}
		radius := 10.0
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "axis-fixed", Kind: "FIXED", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: bottom, SubElement: "WHOLE"}}}}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "radius", Kind: "LENGTH", Value: &radius, Unit: "mm", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: side, SubElement: "WHOLE"}}}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "REVOLVE", Operation: "ADD", Angle: 360, AxisEntityID: "SKETCH_LINE:" + sketch + ":" + bottom})
		return view.Part.Features[len(view.Part.Features)-1], "parameter:" + sketch + ":constraint:radius:value"
	}
	source, radiusID := cylinder()
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	spare, _ := cylinder()
	pick := func(feature workspace.Feature) *workspace.FeatureSelection {
		t.Helper()
		pickView := view
		pickModel := *view.Part
		pickModel.ActiveBodyID = feature.BodyID
		pickView.Part = &pickModel
		p := findP7TopologyPick(t, service, pickView, "FACE", func(property workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
			return selection.SourceBodyID == feature.BodyID && property.GeometryType == "CYLINDER"
		})
		return &workspace.FeatureSelection{Selection: p.Selection, SourceVersionID: p.SourceVersionID, SourceFeatureID: feature.ID}
	}
	original, replacement := pick(source), pick(spare)
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	profile := rectangle(workspace.SketchPoint2{X: 2, Y: -2}, workspace.SketchPoint2{X: 4, Y: 2})
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Start", Origin: [3]float64{0, 0, -20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", SketchID: profile, DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID, VersionID: view.Document.VersionID})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Extent: "TO_SURFACE", TerminationSurface: original})
	consumer := view.Part.Features[len(view.Part.Features)-1]
	volume := func(radius float64) {
		t.Helper()
		want := 160 - 2*(2*math.Sqrt(radius*radius-4)+radius*radius*math.Asin(2/radius))
		if got := activeBodyArtifact(t, view).Volume; math.Abs(got-want) > 1e-5 {
			t.Fatal("curved volume", got, want)
		}
	}
	volume(10)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: radiusID, Value: 12, Unit: "mm"})
	volume(12)
	off := true
	apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: source.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, source.ID), Suppressed: &off})
	consumer = saved(consumer.ID)
	if consumer.EvaluationStatus != "FAILED" || consumer.TerminationSurface.Selection.Anchor.FeatureID != source.ID {
		t.Fatal("missing surface guessed a replacement", consumer)
	}
	head := view.Document.VersionID
	input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: head, FeatureID: consumer.ID})
	if e != nil || input.TerminationPick == nil || input.TerminationPick.Status == modelcore.SelectionResolved {
		t.Fatal("failed input not inspectable", e, input.TerminationPick)
	}
	_, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-unresolved-save", TargetID: consumer.ID, Feature: &consumer, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, consumer.ID)})
	if e == nil {
		t.Fatal("unresolved definition accepted as repair")
	}
	read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("failed repair moved Head", e)
	}
	consumer.TerminationSurface = replacement
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: consumer.ID, Feature: &consumer, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, consumer.ID)})
	volume(10)
	final := activeBodyArtifact(t, view)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	consumer = saved(consumer.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-curved-cold", TargetID: consumer.ID, Feature: &consumer, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, consumer.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal(e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("cold mismatch", role)
		}
	}
}
