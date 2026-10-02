package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
	"strings"
	"testing"
)

func TestAssemblyInteractionMetadataOnlySnapshotAfterDelete(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "snapshot reuse"})
	if err != nil {
		t.Fatal(err)
	}
	serial := 0
	apply := func(command workspace.CommandRequest) {
		t.Helper()
		serial++
		command.ActorID = f.actor
		command.RequestID = fmt.Sprintf("%s-snapshot-%d", p.Document.ID, serial)
		p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, command)
		if err != nil {
			t.Fatal(err)
		}
	}
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	selected := p.Product.Instances[0].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: selected, Kind: "BODY"}, FixMode: "SPACE"})
	constraint := p.Product.Constraints[0].ID
	engineering, err := f.service.GetAssemblyEngineeringEvidence(t.Context(), p.Document.ID, p.Document.VersionID)
	if err != nil || !engineering.Available || len(engineering.Components) == 0 || engineering.Components[0].RelativeDof != 0 {
		t.Fatal("exact current Revision engineering evidence", engineering, err)
	}
	snapshot := func() workspace.InstancePath {
		t.Helper()
		p, err = f.service.GetDocument(t.Context(), p.Document.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range p.ResolvedInstances {
			if r.OccurrencePath == selected {
				return r.InstancePath
			}
		}
		t.Fatal("missing authoritative occurrence")
		return workspace.InstancePath{}
	}
	old := snapshot()
	geometryKey := p.ResolvedInstances[0].GeometryKey
	pose := p.Product.Instances[0]
	apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "ASSEMBLY_CONSTRAINT", TargetID: constraint})
	fresh := snapshot()
	if staleEvidence, err := f.service.GetAssemblyEngineeringEvidence(t.Context(), p.Document.ID, p.Document.VersionID); err != nil || staleEvidence.RevisionID != p.Document.VersionID {
		t.Fatal("evidence version must match, never latest unrelated result", staleEvidence, err)
	}
	for _, kind := range []string{"PLANE", "CYLINDER", "CIRCLE"} {
		ref := f.support(t, part, kind)
		ref.InstanceID = selected
		ref.InstancePath = &fresh
		inspection, err := f.service.InspectAssemblySupports(t.Context(), p.Document.ID, []workspace.AssemblyGeometryRef{ref})
		if err != nil || len(inspection.Supports) != 1 || inspection.Supports[0].Status != "RESOLVED" || inspection.VersionID != p.Document.VersionID {
			t.Fatal("real exact snap inspection", kind, inspection, err)
		}
		if kind == "CYLINDER" && (inspection.Supports[0].SnapHints == nil || inspection.Supports[0].SnapHints.EndFirst == nil || inspection.Supports[0].SnapHints.EndLast == nil) {
			t.Fatal("real Worker cylinder end centers missing", inspection)
		}
	}
	if p.ResolvedInstances[0].GeometryKey != geometryKey || p.Product.Instances[0].Translation != pose.Translation || p.Product.Instances[0].Rotation != pose.Rotation {
		t.Fatal("not a metadata-only reproduction")
	}
	if old.Segments[0].OwnerVersionID == fresh.Segments[0].OwnerVersionID {
		t.Fatal("OwnerVersion did not advance")
	}
	begin := workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: selected, OccurrencePath: &old, FrameRotation: [4]float64{0, 0, 0, 1}}
	if _, err = f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, begin); err == nil || !strings.Contains(err.Error(), "snapshot fields") {
		t.Fatal("stale semantic path accepted", err)
	}
	begin.OccurrencePath = &fresh
	session, err := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, begin)
	if err != nil {
		t.Fatal("new full snapshot must begin", err)
	}
	target := geometry.AssemblyDragTarget{BodyID: selected, FrameRotation: [4]float64{0, 0, 0, 1}, TargetPose: geometry.AssemblyPose{Translation: [3]float64{1, 1, 1}, Rotation: [4]float64{0, 0, 0, 1}}, TranslationComponents: [3]bool{true, true, true}, TargetSequence: 1}
	frame, err := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: 1, Target: target, Final: true})
	if err != nil || frame.Interaction == nil || !frame.Interaction.EligibleForCommit {
		t.Fatal("actual Worker after deletion", frame, err)
	}
	f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, session.SessionID)
	for _, kind := range []string{"UNDO", "REDO"} {
		apply(workspace.CommandRequest{Type: kind})
		current := snapshot()
		begin.BaseRevisionID = p.Document.VersionID
		begin.OccurrencePath = &current
		next, err := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, begin)
		if err != nil {
			t.Fatal(kind, err)
		}
		f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, next.SessionID)
	}
}
