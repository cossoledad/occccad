package automation_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func automationFixture(t *testing.T) (database.DB, *workspace.Service, workspace.DocumentView) {
	t.Helper()
	db := testsupport.OpenDevelopmentDatabase(t)
	worker, closeWorker, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeWorker)
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, worker, artifact.NewService(db, store, staging))
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: access.DefaultUserID, Type: "PART", Name: "Automation concurrency " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	return db, domain, view
}

type heldPreview struct {
	automation.Workspace
	entered, release chan struct{}
}

func (w *heldPreview) PreviewCommand(ctx context.Context, document string, request workspace.CommandRequest) (workspace.CommandPreview, error) {
	close(w.entered)
	select {
	case <-ctx.Done():
		return workspace.CommandPreview{}, ctx.Err()
	case <-w.release:
		return w.Workspace.PreviewCommand(ctx, document, request)
	}
}

func TestAutomationConcurrentLeaseAndControlDuringEvaluation(t *testing.T) {
	for _, control := range []string{"pause", "stop"} {
		t.Run(control, func(t *testing.T) {
			db, domain, view := automationFixture(t)
			blocked := &heldPreview{Workspace: domain, entered: make(chan struct{}), release: make(chan struct{})}
			service := automation.New(db, blocked, access.New(db))
			actor := access.DefaultUserID
			snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			task, err := service.CreateTask(t.Context(), actor, view.Document.ID, automation.CreateTask{RequestID: uuid.NewString(), Goal: "Controlled preview", ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"XY"}`)}}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			type completion struct {
				task automation.Task
				err  error
			}
			done := make(chan completion, 1)
			go func() {
				next, e := service.Advance(ctx, actor, view.Document.ID, task.ID, task.Version)
				done <- completion{next, e}
			}()
			select {
			case <-blocked.entered:
			case <-ctx.Done():
				t.Fatal("preview did not start")
			}
			current, err := service.GetTask(ctx, actor, view.Document.ID, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.Advance(ctx, actor, view.Document.ID, task.ID, current.Version); err == nil {
				t.Fatal("second evaluator bypassed workspace lease")
			}
			if _, err = service.Control(ctx, actor, view.Document.ID, task.ID, automation.Control{Action: control, ExpectedVersion: current.Version}); err != nil {
				t.Fatal(err)
			}
			close(blocked.release)
			result := <-done
			if result.err != nil {
				t.Fatal(result.err)
			}
			wanted := "PAUSED"
			if control == "stop" {
				wanted = "STOPPED"
			}
			if result.task.Status != wanted {
				t.Fatalf("late preview overwrote %s: %s", wanted, result.task.Status)
			}
			if control == "stop" && result.task.Steps[0].Preview != nil {
				t.Fatal("stopped task retained transient preview access")
			}
		})
	}
}

func TestAutomationProductionTaskPreviewCommitRecoveryAndHistory(t *testing.T) {
	db := testsupport.OpenDevelopmentDatabase(t)
	worker, closeWorker, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer closeWorker()
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, worker, artifact.NewService(db, store, staging))
	actor := access.DefaultUserID
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Type: "PART", Name: "Automation " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	service := automation.New(db, domain, access.New(db))
	current, err := service.Context(t.Context(), actor, view.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	args := func(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
	input := automation.CreateTask{RequestID: uuid.NewString(), Goal: "80 by 50 rectangle extruded 12 mm", ExpectedHead: current.Snapshot.CommandBaseline, Steps: []automation.StepSpec{
		{ID: ids[0], OperationID: "sketch.create", Version: 1, Arguments: args(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})},
		{ID: ids[1], OperationID: "sketch.rectangle", Version: 1, Arguments: json.RawMessage(`{"width":80,"height":50,"unit":"mm"}`), Bindings: map[string]string{"sketchId": ids[0]}},
		{ID: ids[2], OperationID: "part.extrude", Version: 1, Arguments: args(map[string]any{"bodyId": view.Part.ActiveBodyID, "length": 12, "unit": "mm"}), Bindings: map[string]string{"sketchId": ids[0]}},
	}}
	task, err := service.CreateTask(t.Context(), actor, view.Document.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := service.CreateTask(t.Context(), actor, view.Document.ID, input)
	if err != nil || duplicate.ID != task.ID {
		t.Fatal("task creation is not idempotent", err)
	}
	for i := range task.Steps {
		before := task.Baseline
		task, err = service.Advance(t.Context(), actor, view.Document.ID, task.ID, task.Version)
		if err != nil {
			t.Fatal(err)
		}
		if task.Steps[i].Phase != "PREVIEW_READY" {
			t.Fatalf("preview phase: %+v", task.Steps[i])
		}
		unchanged, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
		if err != nil || unchanged.CommandBaseline != before {
			t.Fatal("preview changed Head", err)
		}
		task, err = service.Advance(t.Context(), actor, view.Document.ID, task.ID, task.Version)
		if err != nil || task.Steps[i].Phase != "COMMITTED" {
			t.Fatalf("commit: %+v %v", task.Steps[i], err)
		}
		if i == 0 {
			// Emulate a process dying after the domain commit and before recording
			// the step receipt. A fresh service must recover, not create again.
			task.Steps[i].Phase = "COMMITTING"
			task.Steps[i].Result = nil
			task.Baseline = before
			raw, _ := json.Marshal(task)
			if _, err = db.Exec(t.Context(), `UPDATE occccad.automation_tasks SET payload=$1,lease_until=0,lease_token='' WHERE id=$2`, raw, task.ID); err != nil {
				t.Fatal(err)
			}
			service = automation.New(db, domain, access.New(db))
			task, err = service.Advance(t.Context(), actor, view.Document.ID, task.ID, task.Version)
			if err != nil || task.Steps[i].Phase != "COMMITTED" {
				t.Fatalf("receipt recovery: %+v %v", task, err)
			}
			var count int
			if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, task.Steps[i].Action.RequestID).Scan(&count); err != nil || count != 1 {
				t.Fatal("request committed more than once", count, err)
			}
		}
		if _, err = service.Advance(t.Context(), actor, view.Document.ID, task.ID, task.Version); err == nil {
			t.Fatal("advanced without displaying the current result")
		}
		task, err = service.Control(t.Context(), actor, view.Document.ID, task.ID, automation.Control{Action: "displayed", ExpectedVersion: task.Version, StepID: ids[i], RevisionID: task.Steps[i].Result.Receipt.RevisionID})
		if err != nil {
			t.Fatal(err)
		}
	}
	if task.Status != "COMPLETED" {
		t.Fatal("task did not complete", task.Status)
	}
	final, err := domain.GetDocument(t.Context(), view.Document.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Part.Features) != 2 || final.Part.Features[0].Sketch == nil || len(final.Part.Features[0].Sketch.Entities) != 4 {
		t.Fatalf("unexpected final model: %+v", final.Part.Features)
	}
	volume := 0.0
	for _, a := range final.Artifacts {
		volume += a.Volume
	}
	if math.Abs(volume-48000) > 1e-4 {
		t.Fatalf("production volume=%g, want 48000", volume)
	}
	for _, kind := range []string{"UNDO", "REDO"} {
		if _, err = domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: kind, ActorID: actor, RequestID: uuid.NewString()}); err != nil {
			t.Fatal("automation history", kind, err)
		}
	}
	// Reading from a new service instance retains the durable checkpoint.
	parameterID := ""
	for _, parameter := range final.Part.Parameters {
		if parameter.OwnerFeatureID == task.Steps[2].Result.Output.ID {
			if parameterID != "" {
				t.Fatal("extrude has ambiguous length facade")
			}
			parameterID = parameter.ParameterID
		}
	}
	if parameterID == "" {
		t.Fatal("extrusion length parameter facade missing")
	}
	for _, operation := range []string{"parameter.set_literal", "parameter.set_expression"} {
		snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		arguments := map[string]any{"parameterId": parameterID, "value": 20, "unit": "mm"}
		if operation == "parameter.set_expression" {
			arguments = map[string]any{"parameterId": parameterID, "expression": "24 mm"}
		}
		action := automation.ActionRequest{OperationID: operation, Version: 1, Arguments: args(arguments), ExpectedHead: snapshot.CommandBaseline, RequestID: uuid.NewString()}
		preview, err := service.Prepare(t.Context(), actor, view.Document.ID, action)
		if err != nil {
			t.Fatal(operation, err)
		}
		action.PreviewID = preview.PreviewID
		if _, err = service.Commit(t.Context(), actor, view.Document.ID, action); err != nil {
			t.Fatal(operation, err)
		}
	}
	updated, err := domain.GetDocument(t.Context(), view.Document.ID, actor)
	if err != nil || math.Abs(updated.Part.Features[1].Length-24) > 1e-8 {
		t.Fatal("parameter operations did not reach the production evaluator", err)
	}
	loaded, err := automation.New(db, domain, access.New(db)).GetTask(t.Context(), actor, view.Document.ID, task.ID)
	if err != nil || loaded.Version != task.Version || loaded.Status != "COMPLETED" {
		t.Fatal("durable checkpoint did not survive service recreation", err)
	}
}

func TestAutomationPauseExplicitRebaseAndStoppedDisplay(t *testing.T) {
	db := testsupport.OpenDevelopmentDatabase(t)
	worker, closeWorker, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer closeWorker()
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, worker, artifact.NewService(db, store, staging))
	actor := access.DefaultUserID
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Type: "PART", Name: "Automation control " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	service := automation.New(db, domain, access.New(db))
	snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	input := automation.CreateTask{RequestID: uuid.NewString(), Goal: "Create one sketch", ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"XY"}`)}}}
	task, err := service.CreateTask(t.Context(), actor, view.Document.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	advance := func() {
		t.Helper()
		var e error
		task, e = service.Advance(t.Context(), actor, view.Document.ID, task.ID, task.Version)
		if e != nil {
			t.Fatal(e)
		}
	}
	control := func(action string, head *workspace.CommandBaseline) {
		t.Helper()
		var e error
		task, e = service.Control(t.Context(), actor, view.Document.ID, task.ID, automation.Control{Action: action, ExpectedVersion: task.Version, ExpectedHead: head})
		if e != nil {
			t.Fatal(e)
		}
	}
	advance()
	old := *task.Steps[0].Action
	control("pause", nil)
	control("resume", nil)
	if task.Steps[0].Phase != "PLANNED" || task.Steps[0].Preview != nil {
		t.Fatal("resuming reused a short-lived candidate")
	}
	if _, err = service.Commit(t.Context(), actor, view.Document.ID, old); err == nil {
		t.Fatal("revoked preview remained committable")
	}
	advance()
	if _, err = domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: uuid.NewString(), Type: "CREATE_SKETCH", Plane: "XZ"}); err != nil {
		t.Fatal(err)
	}
	advance()
	if task.Status != "PAUSED" || task.Steps[0].Phase != "FAILED" || task.Steps[0].Failure.Code != "WORKSPACE_HEAD_CONFLICT" {
		t.Fatalf("stale task did not fail closed: %+v", task.Steps[0])
	}
	snapshot, err = domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	control("resume", &snapshot.CommandBaseline)
	advance()
	advance()
	if task.Steps[0].Phase != "COMMITTED" {
		t.Fatalf("explicit rebase did not produce a qualified commit: %+v", task.Steps[0])
	}
	if _, err = service.Control(t.Context(), actor, view.Document.ID, task.ID, automation.Control{Action: "displayed", ExpectedVersion: task.Version, StepID: task.Steps[0].ID, RevisionID: uuid.NewString()}); err == nil {
		t.Fatal("wrong revision acknowledged as displayed")
	}
	control("stop", nil)
	task, err = service.Control(t.Context(), actor, view.Document.ID, task.ID, automation.Control{Action: "displayed", ExpectedVersion: task.Version, StepID: task.Steps[0].ID, RevisionID: task.Steps[0].Result.Receipt.RevisionID})
	if err != nil || task.Status != "STOPPED" {
		t.Fatal("display acknowledgment overrode explicit stop", err)
	}
	if _, err = service.GetTask(t.Context(), uuid.NewString(), view.Document.ID, task.ID); err == nil {
		t.Fatal("another actor read a private task")
	}
	// Stopping an idle, ready preview revokes its token as well as the task.
	snapshot, err = domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	input.RequestID = uuid.NewString()
	input.ExpectedHead = snapshot.CommandBaseline
	input.Steps[0].ID = uuid.NewString()
	task, err = service.CreateTask(t.Context(), actor, view.Document.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	advance()
	stoppedAction := *task.Steps[0].Action
	control("stop", nil)
	if task.Steps[0].Preview != nil {
		t.Fatal("stopped task retained an idle preview")
	}
	if _, err = service.Commit(t.Context(), actor, view.Document.ID, stoppedAction); err == nil {
		t.Fatal("stopped preview remained committable")
	}
}
