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
	input := automation.CreateTask{RequestID: uuid.NewString(), Goal: "80 by 50 rectangle extruded 12 mm", ExecutionMode: "AUTOMATIC", ExpectedHead: current.Snapshot.CommandBaseline, Steps: []automation.StepSpec{
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
		if err != nil || task.Steps[i].Phase != "APPLIED" {
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
			if err != nil || task.Steps[i].Phase != "APPLIED" {
				t.Fatalf("receipt recovery: %+v %v", task, err)
			}
			var count int
			if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, task.Steps[i].Action.RequestID).Scan(&count); err != nil || count != 1 {
				t.Fatal("request committed more than once", count, err)
			}
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
	input := automation.CreateTask{RequestID: uuid.NewString(), Goal: "Create one sketch", ExecutionMode: "AUTOMATIC", ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"XY"}`)}}}
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
	if task.Status != "REPLAN_REQUIRED" || task.Baseline == snapshot.CommandBaseline || task.Steps[0].Action.ExpectedHead == snapshot.CommandBaseline {
		t.Fatal("resuming silently rebased old action arguments", task)
	}
	if _, err = service.Advance(t.Context(), actor, view.Document.ID, task.ID, task.Version); err == nil {
		t.Fatal("stale stage executed without replanning")
	}
	input.RequestID = uuid.NewString()
	input.ExpectedHead = snapshot.CommandBaseline
	input.Steps[0].ID = uuid.NewString()
	input.Steps = append(input.Steps, automation.StepSpec{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"YZ"}`)})
	task, err = service.CreateTask(t.Context(), actor, view.Document.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	advance()
	advance()
	if task.Steps[0].Phase != "APPLIED" {
		t.Fatalf("freshly planned stage did not produce a qualified commit: %+v", task.Steps[0])
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
	input.Steps = input.Steps[:1]
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

func TestAutomationAllEdgesFilletAndWorkbenchHandoff(t *testing.T) {
	db, domain, view := automationFixture(t)
	actor := access.DefaultUserID
	document := view.Document.ID
	service := automation.New(db, domain, access.New(db))
	run := func(id string, args any) automation.ActionResult {
		t.Helper()
		snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(args)
		action := automation.ActionRequest{OperationID: id, Version: 1, Arguments: raw, RequestID: uuid.NewString(), ExpectedHead: snapshot.CommandBaseline}
		preview, err := service.Prepare(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(err)
		}
		head, err := domain.ReadModelSnapshot(t.Context(), document, "")
		if err != nil || head.RevisionID != snapshot.RevisionID {
			t.Fatal("preview changed Head", err)
		}
		action.PreviewID = preview.PreviewID
		result, err := service.Commit(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	sketch := run("sketch.create", map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})
	run("sketch.rectangle", map[string]any{"sketchId": sketch.Output.ID, "width": 80, "height": 50, "unit": "mm"})
	extrude := run("part.extrude", map[string]any{"bodyId": view.Part.ActiveBodyID, "sketchId": sketch.Output.ID, "length": 12, "unit": "mm"})
	run("command.rename_feature", map[string]any{"targetId": extrude.Output.ID, "name": "Rounded base"})
	run("command.set_body_color", map[string]any{"bodyId": view.Part.ActiveBodyID, "color": "#6699aa"})
	snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil {
		t.Fatal(err)
	}
	input := automation.TopologyQuery{RevisionID: snapshot.RevisionID, BodyID: view.Part.ActiveBodyID, Kind: "EDGE", Limit: 2}
	page, err := service.QueryTopology(t.Context(), actor, document, input)
	if err != nil || page["matchedCount"] != 12 || page["hasMore"] != true || page["pageComplete"] != false || page["selectionSetComplete"] != true {
		t.Fatal(page, err)
	}
	token := page["selectionSet"].(string)
	if len(token) > 1024 {
		t.Fatal("topology set not compact", len(token))
	}
	input.Cursor = page["nextCursor"].(string)
	next, err := service.QueryTopology(t.Context(), actor, document, input)
	if err != nil || next["selectionSet"] != token {
		t.Fatal(next, err)
	}
	input.BodyID = "wrong-body"
	if _, err = service.QueryTopology(t.Context(), actor, document, input); err == nil {
		t.Fatal("topology Body scope not checked")
	}
	// One bounded set selects every edge; only formal selections enter the feature.
	result := run("part.fillet", map[string]any{"bodyId": view.Part.ActiveBodyID, "selectionSet": token, "size": 2, "unit": "mm"})
	final, err := domain.GetDocument(t.Context(), document)
	if err != nil {
		t.Fatal(err)
	}
	feature := final.Part.Features[len(final.Part.Features)-1]
	if feature.Type != "FILLET" || feature.Length != 2 || len(feature.Selections) != 12 || result.Output == nil || result.Output.ID != feature.ID {
		t.Fatal("fillet not committed", feature, result)
	}
	for _, pick := range feature.Selections {
		if pick.SourceVersionID != snapshot.RevisionID || pick.Selection.SourceBodyID != view.Part.ActiveBodyID || pick.Selection.SourceDocumentID != document {
			t.Fatal("persistent reference scope changed", pick)
		}
	}
	volume := 0.
	for _, artifact := range final.Artifacts {
		volume += artifact.Volume
	}
	if volume <= 0 || volume >= 48000 {
		t.Fatal("fillet did not change the real solid volume", volume)
	}
	fresh, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil {
		t.Fatal(err)
	}
	badRaw, _ := json.Marshal(map[string]any{"bodyId": view.Part.ActiveBodyID, "selectionSet": token, "size": 2, "unit": "mm"})
	if _, err = service.Prepare(t.Context(), actor, document, automation.ActionRequest{OperationID: "part.fillet", Version: 1, Arguments: badRaw, RequestID: uuid.NewString(), ExpectedHead: fresh.CommandBaseline}); err == nil {
		t.Fatal("stale all-edge selection accepted on a new revision")
	}
	// UI dispatch is a journaled handoff, never an authoritative model completion.
	task, err := service.CreateTask(t.Context(), actor, document, automation.CreateTask{RequestID: uuid.NewString(), Goal: "Open the fillet tool", ExpectedHead: fresh.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "ui.part.fillet", Version: 1, Arguments: json.RawMessage(`{}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Advance(t.Context(), actor, document, task.ID, task.Version); err == nil {
		t.Fatal("UI command bypassed the browser")
	}
	task, err = service.Control(t.Context(), actor, document, task.ID, automation.Control{Action: "handoff", ExpectedVersion: task.Version, StepID: task.Steps[0].ID})
	if err != nil || task.Status != "WAIT_UI" || task.Steps[0].Phase != "HANDED_OFF" || task.Steps[0].Result != nil {
		t.Fatal(task, err)
	}
	if _, err = service.Control(t.Context(), actor, document, task.ID, automation.Control{Action: "handoff", ExpectedVersion: task.Version, StepID: task.Steps[0].ID}); err == nil {
		t.Fatal("completed handoff was replayed")
	}
	handoff := task.Steps[0].Handoff
	if handoff == nil || handoff.DomainRequestID == "" {
		t.Fatal("handoff intent was not persisted")
	}
	inputControl := automation.Control{Action: "ui_opened", ExpectedVersion: task.Version, StepID: task.Steps[0].ID, HandoffID: handoff.ID}
	task, err = service.Control(t.Context(), actor, document, task.ID, inputControl)
	if err != nil || task.Status != "WAIT_UI" || task.Steps[0].Phase != "UI_OPENED" || task.Steps[0].Result != nil {
		t.Fatal("opening a UI tool was fabricated as a modeling completion", task, err)
	}
	inputControl.Action = "ui_complete"
	inputControl.ExpectedVersion = task.Version
	inputControl.DomainRequestID = uuid.NewString()
	if _, err = service.Control(t.Context(), actor, document, task.ID, inputControl); err == nil {
		t.Fatal("unrelated commit identity completed an interactive handoff")
	}
	inputControl.Action = "ui_unknown"
	inputControl.DomainRequestID = ""
	task, err = service.Control(t.Context(), actor, document, task.ID, inputControl)
	if err != nil || task.Status != "WAIT_UI" || task.Steps[0].Phase != "UI_UNKNOWN" {
		t.Fatal(task, err)
	}
	inputControl.Action = "ui_opened"
	inputControl.ExpectedVersion = task.Version
	if _, err = service.Control(t.Context(), actor, document, task.ID, inputControl); err == nil {
		t.Fatal("unknown handoff was automatically replayed")
	}
	inputControl.Action = "ui_cancelled"
	task, err = service.Control(t.Context(), actor, document, task.ID, inputControl)
	if err != nil || task.Status != "PAUSED" || task.Steps[0].Phase != "UI_CANCELLED" || task.Steps[0].Result != nil {
		t.Fatal("user cancellation was treated as success", task, err)
	}
	unchanged, _ := domain.ReadModelSnapshot(t.Context(), document, "")
	if unchanged.CommandBaseline != fresh.CommandBaseline {
		t.Fatal("handoff mutated the model")
	}
	t.Logf("Native 80x50x12 box: all 12 edges received R2; resulting solid volume %.6f mm3; selectionSet %d bytes", volume, len(token))
}

func TestAutomationGenericSketchSolidAndProductCommands(t *testing.T) {
	db, domain, view := automationFixture(t)
	service := automation.New(db, domain, access.New(db))
	actor := access.DefaultUserID
	run := func(document, id string, args any) automation.ActionResult {
		t.Helper()
		snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(args)
		action := automation.ActionRequest{OperationID: id, Version: 1, Arguments: raw, RequestID: uuid.NewString(), ExpectedHead: snapshot.CommandBaseline}
		preview, err := service.Prepare(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(id, err)
		}
		action.PreviewID = preview.PreviewID
		result, err := service.Commit(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(id, err)
		}
		return result
	}
	sketch := run(view.Document.ID, "command.create_sketch", map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})
	run(view.Document.ID, "command.edit_sketch", map[string]any{"sketchId": sketch.Output.ID, "operations": []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle-" + uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 5}}}})
	run(view.Document.ID, "command.create_solid_feature", map[string]any{"bodyId": view.Part.ActiveBodyID, "sketchId": sketch.Output.ID, "generator": "LINEAR_EXTRUDE", "operation": "ADD", "length": 10})
	part, err := domain.GetDocument(t.Context(), view.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	volume := 0.
	for _, artifact := range part.Artifacts {
		volume += artifact.Volume
	}
	if math.Abs(volume-250*math.Pi) > 1e-6 {
		t.Fatal("generic circle extrusion did not reach native Worker", volume)
	}
	product, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Type: "PRODUCT", Name: "AI product command fixture"})
	if err != nil {
		t.Fatal(err)
	}
	instance := run(product.Document.ID, "command.insert_instance", map[string]any{"referencedDocumentId": view.Document.ID})
	if instance.Output == nil || instance.Output.Kind != "INSTANCE" {
		t.Fatal(instance)
	}
	run(product.Document.ID, "command.rename_instance", map[string]any{"instanceId": instance.Output.ID, "name": "AI cylinder"})
	result, err := domain.GetDocument(t.Context(), product.Document.ID)
	if err != nil || len(result.Product.Instances) != 1 || result.Product.Instances[0].Name != "AI cylinder" {
		t.Fatal(result, err)
	}
	copies := run(product.Document.ID, "command.insert_instances", map[string]any{"instanceId": instance.Output.ID, "patternAxis": "X", "patternCount": 3, "patternSpacing": 20})
	if len(copies.Outputs) != 2 {
		t.Fatal("batch instance receipts lost created identities", copies)
	}
	beforeNoop, err := domain.ReadModelSnapshot(t.Context(), product.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	noop := run(product.Document.ID, "command.exit_mechanism", map[string]any{"mechanismApplication": map[string]any{"baseRevisionId": beforeNoop.RevisionID}})
	if !noop.Receipt.NoChange || noop.Receipt.AuditID == "" || noop.Receipt.TransactionID != "" || noop.Receipt.CommandBaseline != beforeNoop.CommandBaseline {
		t.Fatal("audited no-op lost its exact unchanged receipt", noop)
	}
	if _, err = domain.FindCommandReceipt(t.Context(), product.Document.ID, uuid.NewString(), noop.Receipt.RequestID); err == nil {
		t.Fatal("no-op receipt leaked across actors")
	}
	page, err := service.Query(t.Context(), actor, product.Document.ID, automation.Query{Kind: "INSTANCE", Fields: []string{"definition"}, Limit: 1})
	if err != nil || page.Items[0].Properties["definition"] == nil {
		t.Fatal(page, err)
	}
	// The reference for an instance that does not yet exist is constructed by
	// the application from its exact output, then bound as one AssemblyRef.
	baseline, err := domain.ReadModelSnapshot(t.Context(), product.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	insertStep := uuid.NewString()
	task, err := service.CreateTask(t.Context(), actor, product.Document.ID, automation.CreateTask{RequestID: uuid.NewString(), Goal: "Insert an existing cylinder and fix its new occurrence", ExecutionMode: "AUTOMATIC", RunID: uuid.NewString(), PlanEpoch: 1, ExpectedHead: baseline.CommandBaseline, Steps: []automation.StepSpec{{ID: insertStep, OperationID: "command.insert_instance", Version: 1, Arguments: marshalOccurrenceArguments(map[string]any{"referencedDocumentId": view.Document.ID})}, {ID: uuid.NewString(), OperationID: "command.add_assembly_constraint", Version: 1, Arguments: json.RawMessage(`{"constraintKind":"FIX","fixMode":"SPACE"}`), TypedBindings: []automation.InputBinding{{Target: "firstAssemblyRef", SourceStepID: insertStep, OutputRole: "PRIMARY", Representation: "ASSEMBLY_REF"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	for task.Status == "ACTIVE" {
		task, err = service.Advance(t.Context(), actor, product.Document.ID, task.ID, task.Version)
		if err != nil {
			t.Fatal(err)
		}
	}
	if task.Status != "COMPLETED" || task.Steps[1].Result == nil || len(task.Steps[0].Result.TypedOutputs) != 1 || task.Steps[0].Result.TypedOutputs[0].InstancePath == nil {
		t.Fatal("new occurrence typed reference did not reach native constraint commit", task)
	}
}

func marshalOccurrenceArguments(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }
