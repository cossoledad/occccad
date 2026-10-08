package control

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

type interactionPersistenceGate struct {
	database.DB
	enabled atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (g *interactionPersistenceGate) Exec(ctx context.Context, sql string, args ...any) (database.Result, error) {
	if strings.Contains(sql, "INSERT INTO occccad.product_solve_manifests") && g.enabled.CompareAndSwap(true, false) {
		close(g.entered)
		// Deliberately finish the blocked write after cancellation to reproduce
		// a late infrastructure completion, not merely a cooperative DB error.
		<-g.release
		return g.DB.Exec(context.WithoutCancel(ctx), sql, args...)
	}
	return g.DB.Exec(ctx, sql, args...)
}

func TestAssemblyInteractionCancellationDuringFinalPersistence(t *testing.T) {
	f := newCompositionControlFixture(t)
	gate := &interactionPersistenceGate{DB: f.db, entered: make(chan struct{}), release: make(chan struct{})}
	f.service = workspace.NewWithArtifacts(gate, f.client, f.store)
	part, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PART", Name: "concurrent Part"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "concurrent Product"})
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ActorID: f.actor, RequestID: "insert-" + p.Document.ID, ReferencedDocumentID: part.Document.ID})
	if err != nil {
		t.Fatal(err)
	}
	q := [4]float64{0, 0, 0, 1}
	open := func() workspace.AssemblyInteractionOpened {
		s, e := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: p.Product.Instances[0].ID, FrameRotation: q})
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	one, two := open(), open()
	target := geometry.AssemblyDragTarget{BodyID: one.BodyID, FrameRotation: q, TargetPose: geometry.AssemblyPose{Translation: [3]float64{1, 0, 0}, Rotation: q}, TranslationComponents: [3]bool{true, true, true}, TargetSequence: 1}
	gate.enabled.Store(true)
	type outcome struct {
		frame workspace.AssemblyInteractionFrame
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		frame, e := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: one.SessionID, Sequence: 1, Target: target, Final: true})
		done <- outcome{frame, e}
	}()
	select {
	case <-gate.entered:
	case <-time.After(10 * time.Second):
		close(gate.release)
		t.Fatal("final persistence did not begin")
	}
	cancelled := make(chan struct{})
	go func() {
		f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, two.SessionID)
		f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, one.SessionID)
		close(cancelled)
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		close(gate.release)
		<-done
		t.Fatal("global Session lock held during final persistence")
	}
	close(gate.release)
	result := <-done
	if result.err == nil || result.frame.PreviewID != "" || result.frame.CommitCommand != nil {
		t.Fatal("late cancelled result offered candidate", result)
	}
	current, e := f.service.GetDocument(t.Context(), p.Document.ID)
	if e != nil || current.Document.VersionID != p.Document.VersionID || current.Product.Instances[0].Translation != [3]float64{} {
		t.Fatal("cancelled preview changed persistent state", e)
	}
}
