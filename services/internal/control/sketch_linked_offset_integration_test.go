//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestLinkedOffsetParametricSolidAndHistoryThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Linked offset shell"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		sequence++
		request.ActorID = p6Actor
		request.RequestID = fmt.Sprintf("%s-offset-%d", view.Document.ID, sequence)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "wall", Value: 2, Unit: "mm"})
	wall := view.Part.Parameters[0].ParameterID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketchID := view.Part.Features[0].ID
	radius := 10.0
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "outer", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: radius}}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "radius", Kind: "RADIUS", Value: &radius, Unit: "mm", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "outer", SubElement: "WHOLE"}}}}}})
	source := "wall"
	request := workspace.CommandRequest{Type: "EDIT_SKETCH", ActorID: p6Actor, RequestID: view.Document.ID + "-offset-preview", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "CREATE_OFFSET", Offset: &workspace.SketchOffset{ID: "inner", EntityIDs: []string{"outer"}, Distance: 1, Join: "MITER"}, ParameterSource: &source}}}
	head := view.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
	if e != nil || preview.PreviewID == "" {
		t.Fatal("offset preview", e)
	}
	read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("preview changed Head", e)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	if len(view.SketchDerivedEntities[sketchID]) != 1 || view.SketchDerivedEntities[sketchID][0].Radius != 8 {
		t.Fatal("accepted derived output", view.SketchDerivedEntities)
	}
	outputID := view.SketchDerivedEntities[sketchID][0].ID
	region := view.SketchAnalyses[sketchID].RegionIDs[0]
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketchID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{region}}})
	padID := view.Part.Features[len(view.Part.Features)-1].ID
	volume := func(want float64) {
		t.Helper()
		if math.Abs(activeBodyArtifact(t, view).Volume-want) > 1e-5 {
			t.Fatal(activeBodyArtifact(t, view).Volume, want)
		}
	}
	volume(5 * math.Pi * 36)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: wall, Value: 3, Unit: "mm"})
	volume(5 * math.Pi * 51)
	if view.SketchDerivedEntities[sketchID][0].ID != outputID || view.SketchAnalyses[sketchID].RegionIDs[0] != region {
		t.Fatal("distance edit replaced formal identity")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	volume(5 * math.Pi * 36)
	apply(workspace.CommandRequest{Type: "REDO"})
	volume(5 * math.Pi * 51)
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "DETACH_OFFSET", OffsetID: "inner"}}})
	if len(view.Part.Features[0].Sketch.Offsets) != 0 || len(view.SketchDerivedEntities[sketchID]) != 0 {
		t.Fatal("detachment retained derived definition")
	}
	volume(5 * math.Pi * 51)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: wall, Value: 4, Unit: "mm"})
	volume(5 * math.Pi * 51)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, err = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	final := activeBodyArtifact(t, view)
	var feature workspace.Feature
	for _, f := range view.Part.Features {
		if f.ID == padID {
			feature = f
		}
	}
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-offset-cold", TargetID: padID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, padID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold offset rebuild", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold result differs", role)
		}
	}
}
