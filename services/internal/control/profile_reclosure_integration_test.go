//go:build integration

package control

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestProfileReclosureAfterDeletedBoundaryThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Profile reclosure"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-reclose-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[0].ID
	rectangle := func(x, y, w, h float64) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + w, Y: y + h}}}})
	}
	rectangle(-20, -20, 40, 40)
	region := view.SketchAnalyses[sketch].RegionIDs[0]
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{region}}})
	feature := view.Part.Features[len(view.Part.Features)-1]
	saved := func() workspace.Feature {
		t.Helper()
		for _, f := range view.Part.Features {
			if f.ID == feature.ID {
				return f
			}
		}
		t.Fatal("feature missing")
		return workspace.Feature{}
	}
	volume := func(want float64) {
		t.Helper()
		f := saved()
		if f.EvaluationStatus != "" {
			t.Fatal("feature did not recover", f.Diagnostic)
		}
		for _, b := range view.Part.Bodies {
			if b.ID == feature.BodyID {
				if math.Abs(view.Artifacts[b.GeometryKey].Volume-want) > 1e-6 {
					t.Fatal("wrong restored volume", view.Artifacts[b.GeometryKey].Volume, want)
				}
				return
			}
		}
		t.Fatal("body missing")
	}
	rectangle(-5, -5, 10, 10)
	volume(7500)
	replace := func(edge workspace.SketchEntity, prior workspace.SketchFeature, newID string) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{edge.ID}}}})
		if saved().EvaluationStatus != "FAILED" || view.SketchAnalyses[sketch].Status != "OPEN" {
			t.Fatal("deletion did not open profile and fail extrusion")
		}
		copy := edge
		copy.ID = newID
		copy.StartPointID = ""
		copy.EndPointID = ""
		ops := []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &copy}}
		for _, constraint := range prior.Constraints {
			if constraint.Kind != "COINCIDENT" {
				continue
			}
			touches := false
			for _, r := range constraint.References {
				touches = touches || r.EntityID == edge.ID
			}
			if !touches {
				continue
			}
			c := constraint
			c.ID = fmt.Sprintf("reclose-%s-%d", newID, len(ops))
			c.References = append([]workspace.SketchGeometryRef(nil), c.References...)
			for i := range c.References {
				if c.References[i].EntityID == edge.ID {
					c.References[i].EntityID = newID
					c.References[i].PointID = ""
				}
			}
			ops = append(ops, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &c})
		}
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
		if a := view.SketchAnalyses[sketch]; a.Status != "CLOSED" || !a.GeometryVerified {
			t.Fatal("replacement did not create an exact closed profile", a)
		}
	}
	prior := *view.Part.Features[0].Sketch
	replace(prior.Entities[4], prior, "restored-hole-edge")
	volume(7500) // The selected outer boundary is unchanged: recovery is automatic.
	if saved().ProfileSelection.RegionIDs[0] != region {
		t.Fatal("hole repair changed selected region")
	}
	prior = *view.Part.Features[0].Sketch
	replace(prior.Entities[0], prior, "restored-outer-edge")
	if f := saved(); f.EvaluationStatus != "FAILED" || !strings.Contains(f.Diagnostic, "PROFILE_REGION_MISSING") {
		t.Fatal("new outer boundary silently inherited a deleted selection", f)
	}
	replacement := saved()
	replacement.ProfileSelection = &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: view.SketchAnalyses[sketch].RegionIDs}
	r := workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-reselect", Type: "EDIT_FEATURE", TargetID: feature.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID), Feature: &replacement}
	failedHead := view.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, r)
	if e != nil {
		t.Fatal("closed profile reselect", e)
	}
	current, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || current.Document.VersionID != failedHead {
		t.Fatal("repair preview advanced Head", e)
	}
	r.PreviewID = preview.PreviewID
	apply(r)
	volume(7500)
	if f := saved(); f.ID != feature.ID || f.BodyID != feature.BodyID || f.Name != feature.Name || f.Order != feature.Order {
		t.Fatal("repair changed feature identity")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if saved().EvaluationStatus != "FAILED" {
		t.Fatal("undo lost unresolved region")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	volume(7500)
}
