//go:build integration

package control

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestDeferredBodyPropertiesThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Demand-only exact properties"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		sequence++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-properties-%d", view.Document.ID, sequence)
		r.VersionID = view.Document.VersionID
		view, err = service.ApplyCommand(ctx, view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 10, Y: 20}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
	feature := view.Part.Features[len(view.Part.Features)-1]
	bodyID := feature.BodyID
	key := view.Part.Bodies[0].GeometryKey
	revision := view.Document.VersionID
	properties, err := service.GetTopologyElementPropertiesAtVersion(ctx, view.Document.ID, revision, key, "BODY", 0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(properties.Properties["volume"].(float64)-6000) > 1e-7 || math.Abs(properties.Properties["area"].(float64)-2200) > 1e-7 {
		t.Fatal(properties.Properties)
	}
	feature.Length = 40
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID), Feature: &feature})
	currentRevision := view.Document.VersionID
	historical, err := service.GetTopologyElementPropertiesAtVersion(ctx, view.Document.ID, revision, key, "BODY", 0)
	if err != nil || math.Abs(historical.Properties["volume"].(float64)-6000) > 1e-7 {
		t.Fatal("immutable historical query", historical, err)
	}
	if _, err = service.GetTopologyElementPropertiesAtVersion(ctx, view.Document.ID, currentRevision, key, "BODY", 0); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatal("mismatched snapshot accepted", err)
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	cutSketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: cutSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 2, Y: 2}, Second: &workspace.SketchPoint2{X: 4, Y: 6}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: cutSketch, BodyID: bodyID, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 10})
	cut := view.Part.Features[len(view.Part.Features)-1]
	before := view.Document.VersionID
	request := workspace.FeatureInputRequest{VersionID: before, FeatureID: cut.ID}
	input, err := service.GetFeatureInput(ctx, view.Document.ID, request)
	if err != nil || input.Properties != nil {
		t.Fatal("normal input must not request mass properties", err)
	}
	request.IncludeProperties = true
	input, err = service.GetFeatureInput(ctx, view.Document.ID, request)
	if err != nil || input.Properties == nil || math.Abs(input.Properties.Properties["volume"].(float64)-8000) > 1e-7 {
		t.Fatal("explicit prefix query", input.Properties, err)
	}
	request.Kind, request.LocalID, request.GeometryKey = "FACE", 1, input.Artifact.GeometryKey
	faceInput, err := service.GetFeatureInput(ctx, view.Document.ID, request)
	if err != nil || faceInput.Properties == nil || faceInput.Properties.Kind != "FACE" {
		t.Fatal("explicit upstream face query", faceInput.Properties, err)
	}
	_ = queriedTopologyCenter(t, *faceInput.Properties)
	after, err := service.GetDocument(ctx, view.Document.ID)
	if err != nil || after.Document.VersionID != before || len(after.Part.Measurements) != 0 {
		t.Fatal("query changed business state", err)
	}
	current, err := service.GetTopologyElementPropertiesAtVersion(ctx, view.Document.ID, before, after.Part.Bodies[0].GeometryKey, "BODY", 0)
	if err != nil || math.Abs(current.Properties["volume"].(float64)-7920) > 1e-7 {
		t.Fatal(current.Properties, err)
	}
}
