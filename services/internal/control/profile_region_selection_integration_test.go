package control

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestExplicitProfileRegionsThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	for _, generator := range []string{"LINEAR_EXTRUDE", "REVOLVE"} {
		t.Run(generator, func(t *testing.T) {
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Explicit regions " + generator})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				r.RequestID = fmt.Sprintf("%s-regions-%d", view.Document.ID, seq)
				view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			circle := func(id string, x, y, r float64) workspace.SketchOperation {
				return workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: x, Y: y}, Radius: r}}
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			sketch := view.Part.Features[0].ID
			radius := 4.0
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{circle("outer", 0, 15, 4), circle("hole", 0, 15, 2), circle("island", 0, 15, 1), circle("other", 20, 15, 3), {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "outer-radius", Kind: "RADIUS", Unit: "mm", Value: &radius, References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "outer", SubElement: "WHOLE"}}}}}})
			// Display projections retain the exact Builder's hole/island classification.
			analysis := view.SketchAnalyses[sketch]
			if !analysis.GeometryVerified || len(analysis.RegionViews) != 3 {
				t.Fatalf("exact analysis: %+v", analysis)
			}
			regionFor := func(x float64, hole bool) string {
				t.Helper()
				for _, r := range view.SketchAnalyses[sketch].RegionViews {
					if (len(r.Holes) > 0) == hole && math.Abs(r.Outer[0].X-x) < 1e-5 {
						return r.ID
					}
				}
				t.Fatal("region missing", x, hole)
				return ""
			}
			annulus, other, island := regionFor(4, true), regionFor(23, false), regionFor(1, false)
			selected := func(ids ...string) *workspace.ProfileRegionSelection {
				return &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: ids}
			}
			create := func(selection *workspace.ProfileRegionSelection) workspace.Feature {
				t.Helper()
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: generator, Operation: "NEW_BODY", Length: 5, Angle: 360, AxisEntityID: "AXIS_SYSTEM:axis-system-default:X", ProfileSelection: selection})
				return view.Part.Features[len(view.Part.Features)-1]
			}
			a, b := create(selected(annulus)), create(selected(other))
			multi := create(selected(annulus, other, island))
			legacy := create(nil)
			saved := func(id string) workspace.Feature {
				t.Helper()
				for _, f := range view.Part.Features {
					if f.ID == id {
						return f
					}
				}
				t.Fatal("missing feature")
				return workspace.Feature{}
			}
			artifact := func(body string) workspace.Artifact {
				t.Helper()
				for _, b := range view.Part.Bodies {
					if b.ID == body {
						return view.Artifacts[b.GeometryKey]
					}
				}
				t.Fatal("body missing")
				return workspace.Artifact{}
			}
			factor := 5.0
			if generator == "REVOLVE" {
				factor = 2 * math.Pi * 15
			}
			volume := func(body string, area float64) {
				t.Helper()
				actual := artifact(body).Volume
				if math.Abs(actual-area*factor) > 1e-5 {
					t.Fatalf("volume %g want %g", actual, area*factor)
				}
			}
			// Cut uses the same exact subset; downstream operations recover after region repair.
			if generator == "LINEAR_EXTRUDE" {
				base := create(nil)
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: generator, BodyID: base.BodyID, Operation: "ADD", Length: 10, ProfileSelection: selected(annulus, other, island)})
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: generator, BodyID: base.BodyID, Operation: "REMOVE", Length: 5, ProfileSelection: selected(annulus)})
				if math.Abs(artifact(base.BodyID).Volume-160*math.Pi) > 1e-5 {
					t.Fatal("cut ignored explicit profile", artifact(base.BodyID).Volume)
				}
			}
			volume(a.BodyID, 12*math.Pi)
			volume(b.BodyID, 9*math.Pi)
			volume(multi.BodyID, 22*math.Pi)
			beforeA, beforeB := artifact(a.BodyID), artifact(b.BodyID)
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{circle("third", 40, 15, 2)}})
			for _, pair := range []struct {
				f   workspace.Feature
				old workspace.Artifact
			}{{a, beforeA}, {b, beforeB}} {
				now := artifact(pair.f.BodyID)
				if now.Representations["BREP"].Digest != pair.old.Representations["BREP"].Digest || now.Representations["NAMING"].Digest != pair.old.Representations["NAMING"].Digest {
					t.Fatal("unrelated new region changed explicit geometry or Naming")
				}
				if !reflect.DeepEqual(saved(pair.f.ID).ProfileSelection, pair.f.ProfileSelection) {
					t.Fatal("new region absorbed")
				}
			}
			volume(legacy.BodyID, 26*math.Pi) // legacy whole-sketch definition deliberately includes new region
			// Ordinary dimension edit keeps formal region identity and updates the selected solid.
			radius = 5
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: "outer-radius", Value: &radius}}})
			volume(a.BodyID, 21*math.Pi)
			if saved(a.ID).ProfileSelection.RegionIDs[0] != annulus {
				t.Fatal("dimension edit changed region identity")
			}
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: generator, BodyID: b.BodyID, Operation: "ADD", Length: 5, Angle: 360, AxisEntityID: "AXIS_SYSTEM:axis-system-default:X", ProfileSelection: selected(annulus)})
			descendant := view.Part.Features[len(view.Part.Features)-1]
			// Delete one selected region while the remaining exact upstream sketch is valid.
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"other"}}}})
			if saved(b.ID).EvaluationStatus != "FAILED" || saved(multi.ID).EvaluationStatus != "FAILED" || saved(a.ID).EvaluationStatus != "" {
				t.Fatal("missing region not isolated", view.Part.Features)
			}
			if !strings.Contains(saved(b.ID).Diagnostic, "PROFILE_REGION_MISSING") {
				t.Fatal("missing diagnostic", saved(b.ID))
			}
			if saved(descendant.ID).EvaluationStatus != "BLOCKED" {
				t.Fatal("missing upstream did not block downstream")
			}
			failedHead := view.Document.VersionID
			broken := saved(multi.ID)
			request := workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-unresolved", TargetID: multi.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, multi.ID), Feature: &broken}
			if _, err = service.PreviewCommand(t.Context(), view.Document.ID, request); err == nil {
				t.Fatal("unresolved region preview accepted")
			}
			if _, err = service.ApplyCommand(t.Context(), view.Document.ID, request); err == nil {
				t.Fatal("unresolved region committed")
			}
			// Explicitly remove just the missing item, preserving the two valid inputs.
			broken.ProfileSelection = selected(annulus, island)
			request.RequestID = view.Document.ID + "-remove"
			request.Feature = &broken
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
			if e != nil {
				t.Fatal(e)
			}
			reopened, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || reopened.Document.VersionID != failedHead {
				t.Fatal("preview/cancel advanced Head", e)
			}
			// Different set cannot adopt the cancelled candidate.
			replacement := saved(b.ID)
			replacement.ProfileSelection = selected(regionFor(42, false))
			stale := request
			stale.RequestID = view.Document.ID + "-stale"
			stale.TargetID = b.ID
			stale.ExpectedFeatureDigest = findP6FeatureDigest(t, view.StructureTree, b.ID)
			stale.Feature = &replacement
			stale.PreviewID = preview.PreviewID
			if _, e = service.ApplyCommand(t.Context(), view.Document.ID, stale); e == nil {
				t.Fatal("adopted stale region preview")
			}
			request.PreviewID = preview.PreviewID
			apply(request)
			repair := workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-replace", TargetID: b.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, b.ID), Feature: &replacement}
			preview, e = service.PreviewCommand(t.Context(), view.Document.ID, repair)
			if e != nil {
				t.Fatal(e)
			}
			repair.PreviewID = preview.PreviewID
			apply(repair)
			if saved(descendant.ID).EvaluationStatus != "" {
				t.Fatal("repair failed to restore downstream")
			}
			final := artifact(b.BodyID)
			volume(b.BodyID, 25*math.Pi)
			result := saved(b.ID)
			if result.ID != b.ID || result.BodyID != b.BodyID || result.Name != b.Name || result.Order != b.Order {
				t.Fatal("repair changed identity")
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			if saved(b.ID).EvaluationStatus != "FAILED" {
				t.Fatal("undo lost failed region definition")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			volume(b.BodyID, 25*math.Pi)
			fresh := workspace.NewWithArtifacts(db, client, artifacts)
			reopened, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			for _, f := range reopened.Part.Features {
				if f.ID == b.ID && !reflect.DeepEqual(f.ProfileSelection, replacement.ProfileSelection) {
					t.Fatal("reopen lost selection")
				}
			}
			repair.PreviewID = ""
			repair.RequestID = view.Document.ID + "-cold"
			repair.ExpectedFeatureDigest = findP6FeatureDigest(t, view.StructureTree, b.ID)
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, repair)
			if e != nil || cold.Artifact == nil {
				t.Fatal("cold", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
					t.Fatal("cold differs", role)
				}
			}
		})
	}
}
