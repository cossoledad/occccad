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

func TestFeatureInputRepairThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	for _, tc := range []struct {
		kind    string
		partial bool
	}{
		{"FILLET", true}, {"FILLET", false}, {"CHAMFER", true}, {"CHAMFER", false},
		{"SHELL", false}, {"DRAFT", true},
	} {
		t.Run(fmt.Sprintf("%s/partial=%v", tc.kind, tc.partial), func(t *testing.T) {
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Topology repair " + tc.kind})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				r.RequestID = fmt.Sprintf("%s-repair-%d", view.Document.ID, seq)
				view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			pad := func(x, y, width, height, length float64) workspace.Feature {
				t.Helper()
				apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				id := view.Part.Features[len(view.Part.Features)-1].ID
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + width, Y: y + height}}}})
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: id, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: length})
				return view.Part.Features[len(view.Part.Features)-1]
			}
			base := pad(-20, -20, 40, 40, 10)
			stem := pad(-5, -5, 10, 10, 20)
			pick := func(id, kind, slot string) workspace.FeatureSelection {
				t.Helper()
				p := findP7TopologyPick(t, service, view, kind, func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
					return s.Anchor.FeatureID == id && strings.HasPrefix(s.Anchor.OutputSlot, slot)
				})
				return workspace.FeatureSelection{Selection: p.Selection, SourceVersionID: p.SourceVersionID}
			}
			kind, slot := "EDGE", "END_BOUNDARY_FROM_PROFILE_EDGE/"
			if tc.kind == "SHELL" {
				kind, slot = "FACE", "END_CAP/"
			}
			if tc.kind == "DRAFT" {
				kind, slot = "FACE", "SIDE_FROM_PROFILE_EDGE/"
			}
			kept, lost := pick(base.ID, kind, slot), pick(stem.ID, kind, slot)
			selections := []workspace.FeatureSelection{lost}
			if !tc.partial && kind == "EDGE" {
				other := findP7TopologyPick(t, service, view, kind, func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
					return s.Anchor.FeatureID == stem.ID && strings.HasPrefix(s.Anchor.OutputSlot, slot) && !reflect.DeepEqual(s.Anchor, lost.Selection.Anchor)
				})
				selections = append(selections, workspace.FeatureSelection{Selection: other.Selection, SourceVersionID: other.SourceVersionID})
			}
			if tc.partial {
				selections = []workspace.FeatureSelection{kept, lost}
			}
			modifier := workspace.Feature{Type: tc.kind, BodyID: base.BodyID, Length: .5, Angle: 2, Selections: selections}
			var neutralSource *workspace.Feature
			if tc.kind == "DRAFT" {
				apply(workspace.CommandRequest{Type: "CREATE_BODY"})
				source := pad(-5, -5, 10, 10, 20)
				neutralSource = &source
				neutral := pick(source.ID, "FACE", "END_CAP/")
				modifier.NeutralPlane = &neutral
				modifier.Selections = []workspace.FeatureSelection{kept}
				apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: base.BodyID})
			}
			apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &modifier})
			feature := view.Part.Features[len(view.Part.Features)-1]
			downstream := pad(-2, -2, 4, 4, 25)
			suppressed := true
			apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: stem.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, stem.ID), Suppressed: &suppressed})
			if neutralSource != nil {
				apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: neutralSource.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, neutralSource.ID), Suppressed: &suppressed})
			}
			saved := func(id string) workspace.Feature {
				t.Helper()
				for _, f := range view.Part.Features {
					if f.ID == id {
						return f
					}
				}
				t.Fatal("feature missing", id)
				return workspace.Feature{}
			}
			if saved(feature.ID).EvaluationStatus != "FAILED" || saved(downstream.ID).EvaluationStatus != "BLOCKED" {
				t.Fatalf("topology removal did not fail modifier and block descendant: %+v", view.Part.Features)
			}
			failedHead := view.Document.VersionID
			input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: feature.ID})
			if e != nil {
				t.Fatalf("valid upstream inaccessible after saved topology vanished: %v", e)
			}
			if math.Abs(input.Artifact.Volume-16000) > 1e-6 || input.Artifact.Naming.Status != "READY" || input.SourceFeatureID != base.ID {
				t.Fatalf("not a valid upstream: %+v", input)
			}
			if len(input.Picks) != len(feature.Selections) {
				t.Fatal("input query discarded unresolved definitions")
			}
			for i, p := range input.Picks {
				expected := modelcore.SelectionOutsideCurrentTip
				if tc.kind == "DRAFT" || tc.partial && i == 0 {
					expected = modelcore.SelectionResolved
				}
				if p.Index != i || p.Status != expected || (p.Status != modelcore.SelectionResolved && p.LocalID != 0) {
					t.Fatalf("input %d: %+v want %s", i, p, expected)
				}
			}
			if tc.kind == "DRAFT" && (input.NeutralPick == nil || input.NeutralPick.Status == modelcore.SelectionResolved || input.NeutralPick.LocalID != 0) {
				t.Fatal("missing neutral plane was hidden", input.NeutralPick)
			}
			request := workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-unresolved", TargetID: feature.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID), Feature: &feature}
			if _, e = service.PreviewCommand(t.Context(), view.Document.ID, request); e == nil {
				t.Fatal("unresolved inputs preview succeeded")
			}
			if _, e = service.ApplyCommand(t.Context(), view.Document.ID, request); e == nil {
				t.Fatal("unresolved required input committed a failed definition as a repair")
			}
			// Current fallback/downstream geometry is never legal replacement evidence.
			fallbackKey := ""
			for _, body := range view.Part.Bodies {
				if body.ID == base.BodyID && body.DisplayFallback != nil {
					fallbackKey = body.DisplayFallback.GeometryKey
				}
			}
			if fallbackKey == "" {
				t.Fatal("failure did not retain display fallback")
			}
			for _, key := range []string{"foreign", fallbackKey} {
				if _, e = service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: feature.ID, GeometryKey: key, Kind: kind, LocalID: 1}); e == nil {
					t.Fatal("accepted geometry outside the legal prefix")
				}
			}
			// Bind a distinct, exact object on the reconstructed prefix through the same endpoint.
			var rebound *workspace.FeatureSelection
			count := p6TopologyCount(t, input.Artifact.Topology, map[string]string{"EDGE": "edges", "FACE": "faces"}[kind])
			for local := uint64(1); local <= count; local++ {
				if tc.kind != "DRAFT" && tc.partial && local == input.Picks[0].LocalID {
					continue
				}
				result, bindErr := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: feature.ID, GeometryKey: input.Artifact.GeometryKey, Kind: kind, LocalID: local})
				if bindErr != nil {
					t.Fatal(bindErr)
				}
				if tc.kind == "DRAFT" && !strings.HasPrefix(result.Selection.Selection.Anchor.OutputSlot, "END_CAP/") {
					continue
				}
				rebound = result.Selection
				break
			}
			if rebound == nil || rebound.SourceVersionID != failedHead || rebound.SourceFeatureID != base.ID {
				t.Fatal("replacement lacks exact prefix source", rebound)
			}
			repaired := feature
			if tc.kind == "DRAFT" {
				repaired.NeutralPlane = rebound
			} else if tc.partial {
				repaired.Selections = []workspace.FeatureSelection{feature.Selections[0], *rebound}
			} else {
				repaired.Selections = []workspace.FeatureSelection{*rebound}
			}
			if !tc.partial && len(feature.Selections) > 1 {
				pending := feature
				pending.Selections = append([]workspace.FeatureSelection{*rebound}, feature.Selections[1:]...)
				incomplete := request
				incomplete.RequestID = view.Document.ID + "-partly-repaired"
				incomplete.Feature = &pending
				if _, e = service.PreviewCommand(t.Context(), view.Document.ID, incomplete); e == nil {
					t.Fatal("remaining unresolved input was silently dropped")
				}
				// The next definition explicitly removes the remaining invalid row.
			}
			request.RequestID = view.Document.ID + "-replacement"
			request.Feature = &repaired
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
			if e != nil {
				t.Fatal("repair preview", e)
			}
			reopened, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || reopened.Document.VersionID != failedHead {
				t.Fatal("preview/read/cancel changed Head", e)
			}
			// Discarding the candidate is cancellation: a later input must not use its stale preview.
			if tc.partial && tc.kind == "CHAMFER" {
				removed := feature
				removed.Selections = []workspace.FeatureSelection{feature.Selections[0]}
				stale := request
				stale.RequestID = view.Document.ID + "-stale"
				stale.Feature = &removed
				stale.PreviewID = preview.PreviewID
				if _, e = service.ApplyCommand(t.Context(), view.Document.ID, stale); e == nil {
					t.Fatal("changed inputs adopted old preview")
				}
				request.RequestID = view.Document.ID + "-remove"
				request.Feature = &removed
				preview, e = service.PreviewCommand(t.Context(), view.Document.ID, request)
				if e != nil {
					t.Fatal("explicit removal preview", e)
				}
				repaired = removed
			}
			request.PreviewID = preview.PreviewID
			apply(request)
			result := saved(feature.ID)
			if result.ID != feature.ID || result.Name != feature.Name || result.Order != feature.Order || result.BodyID != feature.BodyID || result.EvaluationStatus == "FAILED" || saved(downstream.ID).EvaluationStatus == "BLOCKED" {
				t.Fatal("repair changed identity or failed to rebuild descendants", result)
			}
			if tc.partial && !reflect.DeepEqual(result.Selections[0], feature.Selections[0]) {
				t.Fatal("valid original input was rewritten")
			}
			repairedHead := view.Document.VersionID
			final := activeBodyArtifact(t, view)
			apply(workspace.CommandRequest{Type: "UNDO"})
			if view.Document.VersionID == repairedHead || saved(feature.ID).EvaluationStatus != "FAILED" {
				t.Fatal("Undo did not restore failed definition")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			if saved(feature.ID).EvaluationStatus == "FAILED" {
				t.Fatal("Redo failed")
			}
			fresh := workspace.NewWithArtifacts(db, client, artifacts)
			reopened, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || activeBodyArtifact(t, reopened).GeometryID != final.GeometryID {
				t.Fatal("reopen changed geometry", e)
			}
			request.Feature = &repaired
			request.RequestID = view.Document.ID + "-cold"
			request.ExpectedFeatureDigest = findP6FeatureDigest(t, view.StructureTree, feature.ID)
			request.PreviewID = ""
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, request)
			if e != nil || cold.Artifact == nil || cold.Artifact.GeometryID != final.GeometryID {
				t.Fatal("cold rebuild differs", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
					t.Fatalf("cold %s differs", role)
				}
			}
			after, e := fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || after.Document.VersionID != view.Document.VersionID {
				t.Fatal("cold preview changed Head", e)
			}
		})
	}
}

func TestFeatureInputUpstreamFailureThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Upstream blocker location"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-upstream-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	pick := func(slot string) workspace.FeatureSelection {
		p := findP7TopologyPick(t, service, view, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
			return strings.HasPrefix(s.Anchor.OutputSlot, slot)
		})
		return workspace.FeatureSelection{Selection: p.Selection, SourceVersionID: p.SourceVersionID}
	}
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: view.Part.ActiveBodyID, Length: 1, Selections: []workspace.FeatureSelection{pick("END_BOUNDARY_FROM_PROFILE_EDGE/")}}})
	upstream := view.Part.Features[len(view.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: view.Part.ActiveBodyID, Length: .5, Selections: []workspace.FeatureSelection{pick("START_BOUNDARY_FROM_PROFILE_EDGE/")}}})
	target := view.Part.Features[len(view.Part.Features)-1]
	upstream.Length = 1000
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: upstream.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, upstream.ID), Feature: &upstream})
	head := view.Document.VersionID
	_, err = service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: head, FeatureID: target.ID})
	if err == nil || !strings.Contains(err.Error(), "FEATURE_INPUT_UPSTREAM["+upstream.ID+"]") || !strings.Contains(err.Error(), upstream.Name) {
		t.Fatal("upstream evaluation failure was lost or converted to a missing reference", err)
	}
	reopened, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || reopened.Document.VersionID != head {
		t.Fatal("blocked input query changed Head", e)
	}
}
