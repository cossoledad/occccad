//go:build integration

package control

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestHoleToPlaneLifecycleThroughRouter(t *testing.T) {
	s, store, db, client, _ := featureAssociationRuntimeService(t)
	for _, kind := range []string{"SIMPLE", "COUNTERBORE", "COUNTERSINK"} {
		t.Run(kind, func(t *testing.T) {
			v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Hole to plane " + kind})
			if e != nil {
				t.Fatal(e)
			}
			seq := 0
			run := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				if r.RequestID == "" {
					r.RequestID = fmt.Sprintf("%s-hole-plane-%d", v.Document.ID, seq)
				}
				v, e = s.ApplyCommand(t.Context(), v.Document.ID, r)
				if e != nil {
					t.Fatal(r.Type, e)
				}
			}
			saved := func(id string) workspace.Feature {
				t.Helper()
				for _, f := range v.Part.Features {
					if f.ID == id {
						return f
					}
				}
				t.Fatal("missing feature", id)
				return workspace.Feature{}
			}
			edit := func(f workspace.Feature) {
				t.Helper()
				run(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, f.ID)})
			}
			run(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			stockSketch := v.Part.Features[0].ID
			run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: stockSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
			run(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: stockSketch, Length: 30})
			stock := v.Part.Features[len(v.Part.Features)-1]
			// A real upstream stage pick, not a semantic anchor substituted for its stage.
			bottom := findP7TopologyPick(t, s, v, "FACE", func(p workspace.TopologyElementProperties, ref modelcore.PersistentSelection) bool {
				return ref.Anchor.FeatureID == stock.ID && strings.HasPrefix(ref.Anchor.OutputSlot, "START_CAP/")
			})
			run(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Start", Origin: [3]float64{0, 0, 15}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			start := v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID
			run(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "End", Origin: [3]float64{0, 0, 23}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			end := v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1]
			run(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: start})
			sketch := v.Part.Features[len(v.Part.Features)-1].ID
			run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "p", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
			var sourcePlane *workspace.FeatureSelection
			var sourceFeature workspace.Feature
			if kind == "SIMPLE" {
				run(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				otherSketch := v.Part.Features[len(v.Part.Features)-1].ID
				run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: otherSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 30, Y: -10}, Second: &workspace.SketchPoint2{X: 40, Y: 10}}}})
				run(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", SketchID: otherSketch, Length: 23})
				sourceFeature = v.Part.Features[len(v.Part.Features)-1]
				pick := findP7TopologyPick(t, s, v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
					return p.Anchor.FeatureID == sourceFeature.ID && strings.HasPrefix(p.Anchor.OutputSlot, "END_CAP/")
				})
				sourcePlane = &workspace.FeatureSelection{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID, SourceFeatureID: sourceFeature.ID}
			}

			run(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: stock.BodyID})
			f := workspace.Feature{Type: "HOLE", BodyID: stock.BodyID, Operation: "REMOVE", Extent: "TO_PLANE", HoleKind: kind, Diameter: 4, RecessDiameter: 8, RecessDepth: 2, CountersinkAngle: 90, TerminationPlaneID: end.ID, HolePoint: &workspace.PatternPointReference{SketchID: sketch, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "p", SubElement: "POINT"}}}
			req := workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-create-preview", Type: "CREATE_SOLID_FEATURE", Feature: &f, ParameterExpressions: map[string]string{"diameter": "2 mm * 2"}}
			head := v.Document.VersionID
			preview, e := s.PreviewCommand(t.Context(), v.Document.ID, req)
			if e != nil || preview.Artifact == nil {
				t.Fatal("create preview", e)
			}
			s.DiscardPreview(v.Document.ID, p6Actor, preview.PreviewID)
			reopened, e := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
			if e != nil || reopened.Document.VersionID != head {
				t.Fatal("cancel wrote Head", e)
			}
			preview, e = s.PreviewCommand(t.Context(), v.Document.ID, req)
			if e != nil {
				t.Fatal(e)
			}
			req.PreviewID = preview.PreviewID
			run(req)
			hole := v.Part.Features[len(v.Part.Features)-1]
			diameterID := ""
			for _, p := range v.Part.Parameters {
				if p.OwnerFeatureID == hole.ID {
					if p.PropertySlot == "depth" {
						t.Fatal("to-plane created measured depth parameter")
					}
					if p.PropertySlot == "diameter" {
						diameterID = p.ParameterID
						if p.Source.Expression == nil {
							t.Fatal("expression lost")
						}
					}
				}
			}
			extra := 0.
			if kind == "COUNTERBORE" {
				extra = 24
			}
			if kind == "COUNTERSINK" {
				extra = 32. / 3
			}
			check := func(depth float64) {
				t.Helper()
				a, e := s.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: hole.ID, ResultStage: true})
				if e != nil {
					t.Fatal(e)
				}
				want := 12000 - math.Pi*(4*depth+extra)
				if math.Abs(a.Artifact.Volume-want) > 1e-5 {
					t.Fatal("exact volume", a.Artifact.Volume, want)
				}
				got := saved(hole.ID)
				if got.ID != hole.ID || got.BodyID != hole.BodyID || got.Name != hole.Name || got.Order != hole.Order || got.EvaluationStatus != "" {
					t.Fatal("Hole identity/status", got)
				}
			}
			check(8)
			cap := findP7TopologyPick(t, s, v, "FACE", func(_ workspace.TopologyElementProperties, r modelcore.PersistentSelection) bool {
				return r.Anchor.FeatureID == hole.ID && strings.HasPrefix(r.Anchor.OutputSlot, "END_CAP/")
			})
			_ = cap
			mouth := findP7TopologyPick(t, s, v, "EDGE", func(_ workspace.TopologyElementProperties, r modelcore.PersistentSelection) bool {
				return r.Anchor.FeatureID == hole.ID && (strings.HasPrefix(r.Anchor.OutputSlot, "START_BOUNDARY_FROM_PROFILE_EDGE/") || strings.HasSuffix(r.Anchor.OutputSlot, "_MOUTH"))
			})
			run(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: hole.BodyID, Length: .15, Selections: []workspace.FeatureSelection{{Selection: mouth.Selection, SourceVersionID: mouth.SourceVersionID}}}})
			chamfer := v.Part.Features[len(v.Part.Features)-1].ID
			change := workspace.CommandRequest{Type: "EDIT_DATUM_PLANE", TargetID: end.ID, Origin: [3]float64{0, 0, 24}, Normal: [3]float64{-.5, 0, 1}, UDirection: [3]float64{1, 0, .5}}
			run(change)
			check(9)
			if saved(chamfer).EvaluationStatus != "" {
				t.Fatal("mouth lost on tilted update", saved(chamfer))
			}
			// Full candidate rejection, including the wide recess crossing an inclined target.
			invalid := saved(hole.ID)
			invalid.TerminationPlaneID = "datum-xz"
			for _, bad := range []workspace.Feature{invalid, func() workspace.Feature { x := saved(hole.ID); x.Reversed = true; return x }()} {
				seq++
				h := v.Document.VersionID
				_, err := s.PreviewCommand(t.Context(), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-invalid-%d", v.Document.ID, seq), Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &bad, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, hole.ID)})
				if err == nil || !strings.Contains(err.Error(), "TO_PLANE_") {
					t.Fatal("invalid target accepted", err)
				}
				now, err := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
				if err != nil || now.Document.VersionID != h {
					t.Fatal("failed preview wrote history", err)
				}
			}
			// Upstream legal edit retains failed definition and blocks only its dependants.
			badPlane := change
			badPlane.Normal = [3]float64{1, 0, 0}
			badPlane.UDirection = [3]float64{0, 1, 0}
			run(badPlane)
			if saved(hole.ID).EvaluationStatus != "FAILED" || saved(chamfer).EvaluationStatus != "BLOCKED" {
				t.Fatal("failure contract", saved(hole.ID), saved(chamfer))
			}
			input, e := s.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: hole.ID})
			if e != nil || input.TerminationPick == nil {
				t.Fatal("repair input", e)
			}
			fixed := saved(hole.ID)
			fixed.Reversed = true
			fixed.TerminationPlaneID = ""
			fixed.TerminationPlane = &workspace.FeatureSelection{Selection: bottom.Selection, SourceVersionID: bottom.SourceVersionID, SourceFeatureID: stock.ID}
			edit(fixed)
			check(15)
			if saved(chamfer).EvaluationStatus != "" {
				t.Fatal("reselection lost downstream", saved(chamfer))
			}
			run(workspace.CommandRequest{Type: "UNDO"})
			if saved(hole.ID).EvaluationStatus != "FAILED" {
				t.Fatal("undo did not restore failure")
			}
			run(workspace.CommandRequest{Type: "REDO"})
			check(15)
			// Switching modes never persists a one-time measured depth. Parameter identity
			// is retained for dimensions that remain active, including the expression.
			switched := saved(hole.ID)
			switched.Extent = "BLIND"
			switched.Depth = 7
			switched.TerminationPlane = nil
			edit(switched)
			check(7)
			switched = saved(hole.ID)
			switched.Extent = "TO_PLANE"
			switched.TerminationPlane = &workspace.FeatureSelection{Selection: bottom.Selection, SourceVersionID: bottom.SourceVersionID, SourceFeatureID: stock.ID}
			edit(switched)
			check(15)
			for _, p := range v.Part.Parameters {
				if p.OwnerFeatureID == hole.ID && p.PropertySlot == "diameter" && p.ParameterID != diameterID {
					t.Fatal("diameter identity changed")
				}
				if p.OwnerFeatureID == hole.ID && p.PropertySlot == "depth" {
					t.Fatal("inactive depth retained")
				}
			}

			if sourcePlane != nil {
				cross := saved(hole.ID)
				cross.TerminationPlaneID = ""
				cross.TerminationPlane = sourcePlane
				cross.Reversed = false
				edit(cross)
				check(8)
				off := true
				run(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: sourceFeature.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, sourceFeature.ID), Suppressed: &off})
				if saved(hole.ID).EvaluationStatus != "FAILED" || saved(chamfer).EvaluationStatus != "BLOCKED" {
					t.Fatal("lost upstream face not retained", saved(hole.ID))
				}
				broken, e := s.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: hole.ID})
				if e != nil || broken.TerminationPick == nil || broken.TerminationPick.Status == modelcore.SelectionResolved {
					t.Fatal("missing target not repairable", e)
				}
				fixed := saved(hole.ID)
				fixed.TerminationPlane = &workspace.FeatureSelection{Selection: bottom.Selection, SourceVersionID: bottom.SourceVersionID, SourceFeatureID: stock.ID}
				fixed.Reversed = true
				edit(fixed)
				check(15)
				if saved(chamfer).EvaluationStatus != "" {
					t.Fatal("face repair lost downstream")
				}
			}
			finalStage, e := s.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: chamfer, ResultStage: true})
			if e != nil {
				t.Fatal(e)
			}
			final := finalStage.Artifact
			reopened, e = s.GetDocument(t.Context(), v.Document.ID, p6Actor)
			if e != nil || !reflect.DeepEqual(reopened.Part, v.Part) {
				t.Fatal("reopen", e)
			}
			coldService := workspace.NewWithArtifacts(db, client, store)
			last := saved(chamfer)
			cold, e := coldService.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-cold", Type: "EDIT_FEATURE", TargetID: chamfer, Feature: &last, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, chamfer)})
			if e != nil || cold.Artifact == nil {
				t.Fatal("cold", e)
			}
			coldArt := *cold.Artifact
			for _, role := range []string{"BREP", "NAMING"} {
				if coldArt.Representations[role].Digest != final.Representations[role].Digest {
					t.Fatal("cold mismatch", role)
				}
			}
		})
	}
}

func TestHoleToPlaneAcceptedContextInputMaintenanceThroughRouter(t *testing.T) {
	s, store, db, client, _ := featureAssociationRuntimeService(t)
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: kind, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-plane-input-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = n
	}
	read := func(v *workspace.DocumentView) {
		t.Helper()
		n, e := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
		if e != nil {
			t.Fatal(e)
		}
		*v = n
	}
	path := func(v workspace.DocumentView, i int) workspace.InstancePath {
		t.Helper()
		p, e := s.GetProductDesignSession(t.Context(), v.Document.ID, v.Product.Instances[i].ID)
		if e != nil {
			t.Fatal(e)
		}
		return *p.ActiveInstancePath
	}
	layout := create("PART", "Termination layout")
	run(&layout, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Floor", Origin: [3]float64{0, 0, 10}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	plane := layout.Part.DatumPlanes[len(layout.Part.DatumPlanes)-1]
	run(&layout, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Hole end", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: plane.ID})
	pub := layout.Part.Publications[0]
	plate := create("PART", "Context terminated plate")
	run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := plate.Part.Features[0].ID
	run(&plate, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	run(&plate, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: profile, Length: 30})
	run(&plate, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Local floor", Origin: [3]float64{0, 0, 6}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	end := plate.Part.DatumPlanes[len(plate.Part.DatumPlanes)-1].ID
	run(&plate, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: "Hole end", TargetKind: "DATUM", PublicationType: "PLANE", TargetID: end})
	input := plate.Part.ContextInputs[0].ID
	run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := plate.Part.Features[len(plate.Part.Features)-1].ID
	run(&plate, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "p", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
	run(&plate, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: plate.Part.ActiveBodyID, Operation: "REMOVE", Extent: "TO_PLANE", TerminationPlaneID: end, Diameter: 4, HolePoint: &workspace.PatternPointReference{SketchID: sketch, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "p", SubElement: "POINT"}}}})
	hole := plate.Part.Features[len(plate.Part.Features)-1]
	root := create("PRODUCT", "Accepted plane assembly")
	for _, v := range []workspace.DocumentView{layout, plate, plate} {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	run(&root, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: root.Product.Instances[0].ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[1].ID, Translation: [3]float64{5, 0, 2}, Rotation: [4]float64{0, math.Sin(.1), 0, math.Cos(.1)}})
	bind := func() {
		t.Helper()
		next, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-bind-%d", root.Document.ID, seq), ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: path(root, 1), SourceInstancePath: path(root, 0), PublicationID: pub.ID, PublicationType: "PLANE", ContextInputID: input, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
		seq++
		if e != nil {
			t.Fatal(e)
		}
		root = next
	}
	bind()
	binding := root.Product.ContextBindings[0].ID
	check := func(wantDepth float64) {
		t.Helper()
		for i, instance := range root.Product.Instances[1:] {
			found := false
			for _, cv := range root.ContextVariants {
				if cv.OwningInstancePath.Segments[0].InstanceID != instance.ID {
					continue
				}
				found = true
				if cv.Status != "READY" || cv.Part == nil {
					t.Fatal("invalid plane variant", cv.Diagnostic)
				}
				want := 6.
				if i == 0 {
					want = wantDepth
				}
				a := root.Artifacts[cv.Bodies[0].GeometryKey]
				if math.Abs(a.Volume-(12000-4*math.Pi*want)) > 1e-5 {
					t.Fatal("precise accepted hole depth", i, a.Volume, want)
				}
				for _, f := range cv.Part.Features {
					if f.ID == hole.ID && (f.EvaluationStatus != "" || f.HolePoint.Reference.EntityID != "p") {
						t.Fatal("original hole lost", f)
					}
				}
			}
			if !found {
				t.Fatal("variant missing")
			}
		}
	}
	accepted := 8 / math.Cos(.2)
	check(accepted)
	run(&layout, workspace.CommandRequest{Type: "EDIT_DATUM_PLANE", TargetID: plane.ID, Origin: [3]float64{0, 0, 12}, Normal: plane.Normal, UDirection: plane.UDirection})
	baseline := root.Document.VersionID
	read(&root)
	check(accepted)
	if root.Document.VersionID != baseline {
		t.Fatal("unaccepted skeleton plane adopted")
	}
	run(&root, workspace.CommandRequest{Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: binding})
	check(accepted)
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check(accepted)
	plan, e := s.GetProductUpdatePlan(t.Context(), root.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	accepted = 10 / math.Cos(.2)
	check(accepted)
	run(&root, workspace.CommandRequest{Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: binding})
	check(accepted)
	run(&root, workspace.CommandRequest{Type: "RESTORE_CONTEXT_INPUT_DEFAULT", VersionID: root.Document.VersionID, ContextBindingID: binding})
	check(6)
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check(accepted)
	run(&root, workspace.CommandRequest{Type: "REDO"})
	check(6)
	bind()
	binding = root.Product.ContextBindings[0].ID
	check(accepted)
	// An invalid pending plane must not replace the accepted geometry.
	run(&layout, workspace.CommandRequest{Type: "EDIT_DATUM_PLANE", TargetID: plane.ID, Origin: [3]float64{0, 0, -2}, Normal: plane.Normal, UDirection: plane.UDirection})
	read(&root)
	check(accepted)
	run(&root, workspace.CommandRequest{Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: binding})
	check(accepted)
	// The local terminal plane has no source dependency, including after reopening
	// and rebuilding its precise Hole with the source occurrence removed.
	run(&root, workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "INSTANCE", TargetID: root.Product.Instances[0].ID})
	if e := s.DeleteDocument(t.Context(), layout.Document.ID, root.Document.ID+"-remove-layout"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(t.Context(), `DELETE FROM occccad.product_context_variants WHERE base_document_id=$1`, plate.Document.ID); e != nil {
		t.Fatal(e)
	}
	fresh := workspace.NewWithArtifacts(db, client, store)
	cold, e := fresh.GetDocument(workspace.WithColdEvaluation(t.Context()), root.Document.ID, p6Actor)
	if e != nil {
		t.Fatal("local termination rebuild", e)
	}
	if len(cold.ContextVariants) != 2 {
		t.Fatal("missing cold variants")
	}
	for _, cv := range cold.ContextVariants {
		if cv.Status != "READY" || cv.Part == nil {
			t.Fatal("cold variant failed", cv.Diagnostic)
		}
		want := 6.
		if cv.OwningInstancePath.Segments[0].InstanceID == root.Product.Instances[0].ID {
			want = accepted
		}
		a := cold.Artifacts[cv.Bodies[0].GeometryKey]
		if math.Abs(a.Volume-(12000-4*math.Pi*want)) > 1e-5 {
			t.Fatal("local terminal plane changed on cold rebuild", a.Volume, want)
		}
	}
	read(&root)
}
