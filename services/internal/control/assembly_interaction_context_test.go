package control

import (
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssemblyInteractionNestedContextInvalidation(t *testing.T) {
	f := newCompositionControlFixture(t)
	create := func(kind string) workspace.DocumentView {
		v, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: kind, Name: "context test"})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	insert := func(p workspace.DocumentView, child string) workspace.DocumentView {
		v, e := f.service.ApplyCommand(t.Context(), p.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ActorID: f.actor, RequestID: "insert-" + p.Document.ID, ReferencedDocumentID: child})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	part := create("PART")
	p := insert(create("PRODUCT"), part.Document.ID)
	root := insert(create("PRODUCT"), p.Document.ID)
	design, e := f.service.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	edit := workspace.AssemblyInteractionEditContext{RootDocumentID: root.Document.ID, RootRevisionID: root.Document.VersionID, ActiveDocumentID: p.Document.ID, ActiveRevisionID: p.Document.VersionID, InstancePath: design.ActiveInstancePath}
	q := [4]float64{0, 0, 0, 1}
	begin := workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: p.Product.Instances[0].ID, FrameRotation: q, EditContext: &edit}
	for _, bad := range []string{"root-revision", "active-revision", "typed-path"} {
		copy := edit
		path := *edit.InstancePath
		path.Segments = append([]workspace.InstancePathSegment(nil), path.Segments...)
		copy.InstancePath = &path
		switch bad {
		case "root-revision":
			copy.RootRevisionID = "wrong"
		case "active-revision":
			copy.ActiveRevisionID = "wrong"
		case "typed-path":
			copy.InstancePath.Segments[0].ResolvedVersionID = "wrong"
		}
		input := begin
		input.EditContext = &copy
		if _, e = f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, input); e == nil {
			t.Fatal("invalid context accepted", bad)
		}
	}
	session, e := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, begin)
	if e != nil {
		t.Fatal(e)
	}
	defer f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, session.SessionID)
	update := func(seq uint64) (workspace.AssemblyInteractionFrame, error) {
		target := geometry.AssemblyDragTarget{BodyID: session.BodyID, FrameRotation: q, TargetPose: geometry.AssemblyPose{Translation: [3]float64{float64(seq), 0, 0}, Rotation: q}, TranslationComponents: [3]bool{true, true, true}, TargetSequence: seq}
		return f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: seq, Target: target})
	}
	// The Session owns its Begin snapshot, even when a Go caller retains pointers.
	edit.RootRevisionID = "caller-mutated"
	frame, e := update(1)
	if e != nil || frame.Interaction == nil || !frame.Interaction.EligibleForCommit {
		t.Fatal(frame, e)
	}
	pinned := insert(create("PRODUCT"), p.Document.ID)
	pinned, e = f.service.ApplyCommand(t.Context(), pinned.Document.ID, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", ActorID: f.actor, RequestID: "pin-" + pinned.Document.ID, InstanceID: pinned.Product.Instances[0].ID, ReferenceMode: "PINNED"})
	if e != nil {
		t.Fatal(e)
	}
	design, e = f.service.GetProductDesignSession(t.Context(), pinned.Document.ID, pinned.Product.Instances[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	readOnly := begin
	readOnly.EditContext = &workspace.AssemblyInteractionEditContext{RootDocumentID: pinned.Document.ID, RootRevisionID: pinned.Document.VersionID, ActiveDocumentID: p.Document.ID, ActiveRevisionID: p.Document.VersionID, InstancePath: design.ActiveInstancePath}
	if _, e = f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, readOnly); e == nil {
		t.Fatal("PINNED path accepted for modification")
	}
	if e = f.service.DeleteDocument(t.Context(), part.Document.ID, "delete-"+part.Document.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = update(2); e == nil {
		t.Fatal("deleted document was hidden by cached context")
	}
	var head string
	e = f.db.QueryRow(t.Context(), `SELECT head_revision_id::text FROM occccad.workspaces WHERE document_id=$1 AND name='main'`, p.Document.ID).Scan(&head)
	if e != nil || head != p.Document.VersionID {
		t.Fatal("failed preview changed Head", e)
	}
}
