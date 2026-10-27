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

func TestSketchSupportReassignmentThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Sketch support repair"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-support-%d", view.Document.ID, seq)
		if r.Type == "REASSIGN_SKETCH_SUPPORT" {
			r.VersionID = view.Document.VersionID
		}
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	saved := func(id string) workspace.Feature {
		t.Helper()
		for _, f := range view.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("feature missing")
		return workspace.Feature{}
	}
	pad := func(x, y, w, h, length float64) workspace.Feature {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		s := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: s, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + w, Y: y + h}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: s, Generator: "LINEAR_EXTRUDE", Length: length, Operation: "ADD"})
		return view.Part.Features[len(view.Part.Features)-1]
	}
	base := pad(-20, -20, 40, 40, 10)
	tip := pad(-5, -5, 10, 10, 20)
	face := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == base.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	targetBody := view.Part.ActiveBodyID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: face.GeometryKey, TopologyID: face.LocalID, VersionID: view.Document.VersionID, BodyID: targetBody})
	sketchID := view.Part.Features[len(view.Part.Features)-1].ID
	if support := saved(sketchID).Sketch.Support; support.SourceFeatureID != tip.ID || support.SourceFeatureID == support.PersistentSelection.Anchor.FeatureID {
		t.Fatal("source stage was replaced by semantic anchor", support)
	}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 2, Y: 2}, Second: &workspace.SketchPoint2{X: 6, Y: 5}}, {Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "origin-projection", DatumReference: &workspace.DatumReference{Kind: "POINT", EntityID: "axis-system-default"}, SourceVersionID: view.Document.VersionID}}})
	original := saved(sketchID)
	originalParameters := append([]modelcore.ParameterDefinition(nil), view.Part.Parameters...)
	regionIDs := append([]string(nil), view.SketchAnalyses[sketchID].RegionIDs...)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketchID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", BodyID: targetBody, Length: 3, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: regionIDs}})
	child := view.Part.Features[len(view.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	unrelated := pad(80, 0, 10, 10, 2)
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
	independent := artifact(unrelated.BodyID)
	// A legal upstream suppression removes the saved face, while its Body still evaluates.
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: base.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, base.ID), Suppressed: &suppressed})
	if s := saved(sketchID); s.Sketch.Support.Status != "FAILED_SUPPORT" || s.EvaluationStatus != "FAILED" || saved(child.ID).EvaluationStatus != "BLOCKED" {
		t.Fatal("support loss was not retained", s, saved(child.ID))
	}
	if artifact(unrelated.BodyID).GeometryID != independent.GeometryID || saved(unrelated.ID).EvaluationStatus != "" {
		t.Fatal("unrelated body blocked")
	}
	failedHead := view.Document.VersionID
	if !reflect.DeepEqual(saved(sketchID).Sketch.Entities, original.Sketch.Entities) || saved(sketchID).Sketch.ExternalGeometry[0].Snapshot != nil {
		t.Fatal("failed support changed design or retained old projection")
	}
	// Existing delete protection remains in force, even for a failed reference.
	if _, e := service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: base.ID, RequestID: view.Document.ID + "-delete"}); e == nil {
		t.Fatal("support deletion protection bypassed")
	}
	input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: sketchID, BodyID: base.BodyID})
	if e != nil || input.SourceFeatureID != tip.ID || math.Abs(input.Artifact.Volume-2000) > 1e-6 {
		t.Fatal("failed support blocked valid input stage", e, input)
	}
	var replacement uint64
	for local := uint64(1); local <= p6TopologyCount(t, input.Artifact.Topology, "faces"); local++ {
		r, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: sketchID, BodyID: base.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "FACE", LocalID: local})
		if e == nil && strings.HasPrefix(r.Selection.Selection.Anchor.OutputSlot, "END_CAP/") {
			replacement = local
			break
		}
	}
	if replacement == 0 {
		t.Fatal("replacement plane missing")
	}
	// Downstream/fallback geometry is not admissible evidence.
	if _, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-bad-source", Type: "REASSIGN_SKETCH_SUPPORT", VersionID: failedHead, SketchID: sketchID, TargetKind: "FACE", BodyID: targetBody, GeometryKey: independent.GeometryKey, TopologyID: 1}); e == nil {
		t.Fatal("downstream geometry accepted")
	}
	datum := workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-datum", Type: "REASSIGN_SKETCH_SUPPORT", VersionID: failedHead, SketchID: sketchID, DatumPlaneID: "datum-xz"}
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, datum)
	if e != nil || preview.Artifact == nil || len(preview.ReferenceGeometry.DatumPlanes) != 1 {
		t.Fatal("datum preview", e)
	}
	reopened, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || reopened.Document.VersionID != failedHead {
		t.Fatal("preview/cancel advanced Head", e)
	}
	stale := datum
	stale.RequestID = view.Document.ID + "-stale"
	stale.DatumPlaneID = "datum-yz"
	stale.PreviewID = preview.PreviewID
	if _, e = service.ApplyCommand(t.Context(), view.Document.ID, stale); e == nil {
		t.Fatal("changed support adopted stale preview")
	}
	datum.PreviewID = preview.PreviewID
	apply(datum)
	if saved(sketchID).Sketch.Support.Status != "CONNECTED" || saved(child.ID).EvaluationStatus != "" {
		t.Fatal("datum repair did not restore downstream")
	}
	if !reflect.DeepEqual(saved(sketchID).Sketch.Entities, original.Sketch.Entities) || !reflect.DeepEqual(saved(sketchID).Sketch.Constraints, original.Sketch.Constraints) {
		t.Fatal("local design definitions changed")
	}
	if !reflect.DeepEqual(saved(child.ID).ProfileSelection.RegionIDs, regionIDs) {
		t.Fatal("region identity changed")
	}
	for _, old := range originalParameters {
		for _, p := range view.Part.Parameters {
			if p.ParameterID == old.ParameterID && (!reflect.DeepEqual(p.Source, old.Source) || p.OwnerFeatureID != old.OwnerFeatureID) {
				t.Fatal("parameter definition changed")
			}
		}
	}
	ext := saved(sketchID).Sketch.ExternalGeometry[0]
	if ext.ID != "origin-projection" || ext.Status != "CONNECTED" || ext.Snapshot == nil {
		t.Fatal("external projection not rebuilt", ext)
	}
	// Switch from datum to an exact plane from a FAILED source Revision's legal prefix.
	repair := workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-face", Type: "REASSIGN_SKETCH_SUPPORT", VersionID: view.Document.VersionID, SketchID: sketchID, TargetKind: "FACE", BodyID: base.BodyID, GeometryKey: input.Artifact.GeometryKey, TopologyID: replacement}
	preview, e = service.PreviewCommand(t.Context(), view.Document.ID, repair)
	if e != nil {
		t.Fatal("plane replacement", e)
	}
	repair.PreviewID = preview.PreviewID
	apply(repair)
	final := artifact(targetBody)
	s := saved(sketchID)
	if s.ID != original.ID || s.BodyID != original.BodyID || s.Order != original.Order || s.Name != original.Name || s.Sketch.Support.SourceFeatureID != tip.ID {
		t.Fatal("identity or source stage changed")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if saved(sketchID).Sketch.Support.DatumPlaneID != "datum-xz" {
		t.Fatal("undo face")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if saved(sketchID).Sketch.Support.Status != "FAILED_SUPPORT" {
		t.Fatal("undo lost failed definition")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	apply(workspace.CommandRequest{Type: "REDO"})
	if artifact(targetBody).GeometryID != final.GeometryID {
		t.Fatal("redo differs")
	}
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	reopened, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.Document.VersionID != view.Document.VersionID {
		t.Fatal("reopen changed Head")
	}
	repair.VersionID = view.Document.VersionID
	repair.PreviewID = ""
	repair.RequestID = view.Document.ID + "-cold"
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, repair)
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold rebuild", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold differs", role)
		}
	}
}

func TestSketchSupportProjectionAndIllegalReplacementThroughRouter(t *testing.T) {
	service, _, _, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Support projection gates"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-projection-%d", view.Document.ID, seq)
		r.VersionID = view.Document.VersionID
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	source := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: source, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "source-circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: source, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	sourceBody := view.Part.ActiveBodyID
	cylinder := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return p.GeometryType == "CYLINDER"
	})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 20}, Radius: 2}}, {Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "projected-axis", SourceVersionID: view.Document.VersionID, DatumReference: &workspace.DatumReference{Kind: "AXIS_SYSTEM", EntityID: "axis-system-default", Axis: "X"}}}})
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "parallel-line", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{Y: 1}, End: &workspace.SketchPoint2{X: 10, Y: 1}}},
		{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "parallel-external", Kind: "PARALLEL", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "parallel-line", SubElement: "WHOLE"}, {Target: "EXTERNAL", EntityID: "projected-axis", SubElement: "WHOLE"}}}},
	}})
	original := view.Part.Features[len(view.Part.Features)-1]
	reject := func(r workspace.CommandRequest, diagnostic string) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-invalid-%d", view.Document.ID, seq)
		r.Type = "REASSIGN_SKETCH_SUPPORT"
		r.VersionID = view.Document.VersionID
		r.SketchID = sketch
		head := view.Document.VersionID
		if _, e := service.PreviewCommand(t.Context(), view.Document.ID, r); e == nil || !strings.Contains(e.Error(), diagnostic) {
			t.Fatal("invalid preview", e)
		}
		if _, e := service.ApplyCommand(t.Context(), view.Document.ID, r); e == nil || !strings.Contains(e.Error(), diagnostic) {
			t.Fatal("invalid commit", e)
		}
		v, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
		if e != nil || v.Document.VersionID != head {
			t.Fatal("rejected replacement advanced Head", e)
		}
	}
	reject(workspace.CommandRequest{TargetKind: "FACE", BodyID: sourceBody, GeometryKey: cylinder.GeometryKey, TopologyID: cylinder.LocalID}, "SUPPORT_TYPE_MISMATCH")
	reject(workspace.CommandRequest{DatumPlaneID: "datum-yz"}, "") // Projection becomes a point, incompatible with the retained Parallel constraint.
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{7, 2, 3}, Normal: [3]float64{0, -1, 0}, UDirection: [3]float64{1, 0, 0}})
	datum := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
	apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", SketchID: sketch, DatumPlaneID: datum.ID})
	var changed workspace.Feature
	for _, f := range view.Part.Features {
		if f.ID == sketch {
			changed = f
		}
	}
	if changed.Sketch.Support.Origin != datum.Origin || changed.Sketch.Support.Normal != datum.Normal || !reflect.DeepEqual(changed.Sketch.Entities, original.Sketch.Entities) || !reflect.DeepEqual(changed.Sketch.Constraints, original.Sketch.Constraints) {
		t.Fatal("custom plane lost local design", changed)
	}
	old, projected := original.Sketch.ExternalGeometry[0], changed.Sketch.ExternalGeometry[0]
	if projected.ID != old.ID || projected.Status != "CONNECTED" || reflect.DeepEqual(projected.Snapshot, old.Snapshot) {
		t.Fatal("external identity or reprojection wrong", projected)
	}
	// A datum depending on this sketch's downstream result creates a real graph cycle.
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 3})
	child := view.Part.Features[len(view.Part.Features)-1]
	p := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == child.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "TOPOLOGY", FeatureID: child.ID, SourceVersionID: view.Document.VersionID, Selection: &p.Selection}}})
	cycle := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
	reject(workspace.CommandRequest{DatumPlaneID: cycle.ID}, "DEPENDENCY_CYCLE")
	// Fault injection closes only this test's client. Service errors stay atomic.
	head := view.Document.VersionID
	_ = client.Close()
	_, e := service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-worker-unavailable", Type: "REASSIGN_SKETCH_SUPPORT", VersionID: head, SketchID: sketch, DatumPlaneID: "datum-xy"})
	if e == nil || strings.Contains(e.Error(), "FAILED_SUPPORT") {
		t.Fatal("worker error swallowed as support loss", e)
	}
	reopened, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || reopened.Document.VersionID != head {
		t.Fatal("infrastructure error advanced Head", e)
	}

}

func TestSketchSupportUpstreamGeometryFailureThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Support upstream blocker"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-blocker-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	source := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: source, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: source, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	p := findP7TopologyPick(t, service, view, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return strings.HasPrefix(s.Anchor.OutputSlot, "END_BOUNDARY_FROM_PROFILE_EDGE/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: view.Part.ActiveBodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: p.Selection, SourceVersionID: p.SourceVersionID}}}})
	fillet := view.Part.Features[len(view.Part.Features)-1]
	face := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return p.GeometryType == "PLANE" && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", VersionID: view.Document.VersionID, GeometryKey: face.GeometryKey, TopologyID: face.LocalID})
	sketch := view.Part.Features[len(view.Part.Features)-1]
	broken := fillet
	broken.Length = 1000
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: fillet.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, fillet.ID), Feature: &broken})
	for _, f := range view.Part.Features {
		if f.ID == fillet.ID && f.EvaluationStatus != "FAILED" {
			t.Fatal("actual source not failed")
		}
		if f.ID == sketch.ID && (f.EvaluationStatus != "BLOCKED" || f.Sketch.Support.Status != "BLOCKED" || !strings.Contains(f.Sketch.Support.Diagnostic, fillet.ID)) {
			t.Fatal("upstream failure misreported as missing support", f)
		}
	}
	if _, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: sketch.ID, BodyID: fillet.BodyID}); e == nil || !strings.Contains(e.Error(), fillet.ID) {
		t.Fatal("actual input-stage blocker lost", e)
	}
}
