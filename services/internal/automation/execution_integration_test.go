//go:build integration

package automation_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAutomationBackgroundStagesObserveAndVerifyAllEdges(t *testing.T) {
	db, domain, view := automationFixture(t)
	service := automation.New(db, domain, access.New(db))
	actor, document, runID := access.DefaultUserID, view.Document.ID, uuid.NewString()
	context, err := service.Context(t.Context(), actor, document)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	encode := func(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }
	first, err := service.CreateTask(t.Context(), actor, document, automation.CreateTask{RequestID: uuid.NewString(), Goal: "80 × 50 mm rectangle, 12 mm extrusion, then all edges R2", ExecutionMode: "AUTOMATIC", RunID: runID, PlanEpoch: 1, ExpectedHead: context.Snapshot.CommandBaseline, Steps: []automation.StepSpec{
		{ID: ids[0], OperationID: "sketch.create", Version: 1, Arguments: encode(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})},
		{ID: ids[1], OperationID: "sketch.rectangle", Version: 1, Arguments: encode(map[string]any{"width": 80, "height": 50, "unit": "mm"}), TypedBindings: []automation.InputBinding{{Target: "sketchId", SourceStepID: ids[0], Representation: "ENTITY_ID"}}},
		{ID: ids[2], OperationID: "part.extrude", Version: 1, Arguments: encode(map[string]any{"bodyId": view.Part.ActiveBodyID, "length": 12, "unit": "mm"}), TypedBindings: []automation.InputBinding{{Target: "sketchId", SourceStepID: ids[0], OutputRole: "PRIMARY", Representation: "ENTITY_ID"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	advance := func(task automation.Task) automation.Task {
		t.Helper()
		for task.Status == "ACTIVE" {
			var e error
			task, e = service.Advance(t.Context(), actor, document, task.ID, task.Version)
			if e != nil {
				t.Fatal(e)
			}
		}
		if task.Status != "COMPLETED" {
			t.Fatal("stage did not complete", task)
		}
		for _, step := range task.Steps {
			if step.Phase != "APPLIED" || step.Result == nil || step.Presentation.Status != "NOT_PRESENTED" {
				t.Fatal("background operation invented presentation or missed receipt", step)
			}
		}
		return task
	}
	first = advance(first)
	created := []string{}
	for _, step := range first.Steps {
		if step.Result.Output != nil {
			created = append(created, step.Result.Output.ID)
		}
	}
	page, err := service.QueryTopology(t.Context(), actor, document, automation.TopologyQuery{RevisionID: first.Baseline.RevisionID, BodyID: view.Part.ActiveBodyID, Kind: "EDGE", Limit: 2})
	if err != nil || page["matchedCount"] != 12 || page["pageComplete"] != false || page["selectionSetComplete"] != true {
		t.Fatal("new topology was not observed as a complete set", page, err)
	}
	selectionSet := page["selectionSet"].(string)
	// This second stage is in the same Run and can only be built after the
	// first stage produced authoritative topology. No final topology IDs exist
	// in the initial plan and no viewport participates in execution.
	second, err := service.CreateTask(t.Context(), actor, document, automation.CreateTask{RequestID: uuid.NewString(), Goal: first.Goal, ExecutionMode: "AUTOMATIC", RunID: runID, PlanEpoch: 1, ExpectedHead: first.Baseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "part.fillet", Version: 1, Arguments: encode(map[string]any{"bodyId": view.Part.ActiveBodyID, "selectionSet": selectionSet, "size": 2, "unit": "mm"})}}})
	if err != nil {
		t.Fatal(err)
	}
	second = advance(second)
	created = append(created, second.Steps[0].Result.Output.ID)
	checks := []automation.PartCheck{
		{ID: "body", Kind: "ENTITY_EXISTS", BodyID: view.Part.ActiveBodyID},
		{ID: "rectangle", Kind: "RECTANGLE_DIMENSIONS", Width: 80, Height: 50, Unit: "mm"},
		{ID: "length", Kind: "EXTRUSION_LENGTH", Length: 12, Unit: "mm"},
		{ID: "all-edges", Kind: "ALL_EDGES_FILLET", Radius: 2, Unit: "mm", ExpectedCount: 12, SelectionSet: selectionSet},
		{ID: "solid", Kind: "BODY_GEOMETRY_VALID", BodyID: view.Part.ActiveBodyID},
	}
	verified, err := service.VerifyPart(t.Context(), actor, document, automation.PartVerificationRequest{RevisionID: second.Baseline.RevisionID, GoalVersion: 1, CreatedEntityIDs: created, Checks: checks})
	if err != nil || !verified.Passed || verified.Snapshot.RevisionID != second.Baseline.RevisionID || verified.GoalVersion != 1 {
		t.Fatalf("native goal verification: %+v %v", verified, err)
	}
	volume := verified.Checks[len(verified.Checks)-1].Observed["volume"].(float64)
	if math.Abs(volume-47523.138502) > 1e-3 {
		t.Fatal("unexpected native R2 solid volume", volume)
	}
	// New intent cannot be proved by old successful checks or by matching an
	// unrelated historical feature. Required checks remain tied to goal v2.
	checks[2].Length = 20
	checks = append(checks, automation.PartCheck{ID: "no-fillet", Kind: "NO_FEATURE_TYPE", ExpectedType: "FILLET"})
	changed, err := service.VerifyPart(t.Context(), actor, document, automation.PartVerificationRequest{GoalVersion: 2, CreatedEntityIDs: created, Checks: checks})
	if err != nil || changed.Passed || changed.GoalVersion != 2 || changed.Checks[2].Passed || changed.Checks[len(changed.Checks)-1].Passed {
		t.Fatal("old success incorrectly proved changed intent", changed, err)
	}
	missing, err := service.VerifyPart(t.Context(), actor, document, automation.PartVerificationRequest{GoalVersion: 2, Checks: []automation.PartCheck{{ID: "rectangle", Kind: "RECTANGLE_DIMENSIONS", Width: 80, Height: 50, Unit: "mm"}}})
	if err != nil || missing.Passed {
		t.Fatal("verification found an unrelated existing feature without a goal output scope", missing, err)
	}
	// Presentation may fail independently and never repeats a modeling command.
	second, err = service.Control(t.Context(), actor, document, second.ID, automation.Control{Action: "presentation_failed", ExpectedVersion: second.Version, StepID: second.Steps[0].ID, RevisionID: second.Baseline.RevisionID, RunID: runID, PlanEpoch: 1})
	if err != nil || second.Status != "COMPLETED" || second.Steps[0].Phase != "APPLIED" || second.Steps[0].Presentation.Status != "FAILED" {
		t.Fatal(second, err)
	}
	second, err = service.Control(t.Context(), actor, document, second.ID, automation.Control{Action: "displayed", ExpectedVersion: second.Version, StepID: second.Steps[0].ID, RevisionID: second.Baseline.RevisionID, RunID: runID, PlanEpoch: 1})
	if err != nil || second.Steps[0].Presentation.Status != "PRESENTED" {
		t.Fatal(second, err)
	}
	for _, step := range first.Steps {
		if step.Presentation.Status != "NOT_PRESENTED" {
			t.Fatal("final frame fabricated a historical presentation")
		}
	}
	t.Logf("same Run, two bounded stages: native 80x50x12 mm solid, all 12 exact edges R2, volume %.6f mm3; application and presentation remain independent", volume)
}

func TestAutomationConfirmationBindsCandidateAndReplansAfterRestart(t *testing.T) {
	db, domain, view := automationFixture(t)
	service := automation.New(db, domain, access.New(db))
	actor, document, runID := access.DefaultUserID, view.Document.ID, uuid.NewString()
	snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(t.Context(), actor, document, automation.CreateTask{RequestID: uuid.NewString(), Goal: "Confirm exact preview", RunID: runID, PlanEpoch: 2, ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"XY"}`)}}})
	if err != nil || task.ExecutionMode != "CONFIRM" {
		t.Fatal(task, err)
	}
	task, err = service.Advance(t.Context(), actor, document, task.ID, task.Version)
	if err != nil || task.Status != "WAIT_CONFIRM" {
		t.Fatal(task, err)
	}
	input := automation.Control{Action: "confirm", ExpectedVersion: task.Version, StepID: task.Steps[0].ID, PreviewID: task.Steps[0].Preview.PreviewID, ExpectedHead: &task.Baseline, RunID: runID, PlanEpoch: 2}
	bad := input
	bad.PlanEpoch = 1
	if _, err = service.Control(t.Context(), actor, document, task.ID, bad); err == nil {
		t.Fatal("old plan epoch approved candidate")
	}
	bad = input
	bad.PreviewID = uuid.NewString()
	if _, err = service.Control(t.Context(), actor, document, task.ID, bad); err == nil {
		t.Fatal("another candidate was confirmed")
	}
	if _, err = service.Advance(t.Context(), actor, document, task.ID, task.Version); err == nil {
		t.Fatal("CONFIRM task advanced without explicit confirmation")
	}
	task, err = service.Control(t.Context(), actor, document, task.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	oldPreview := input.PreviewID
	domain.DiscardPreview(document, actor, oldPreview) // emulate process-local candidate loss
	service = automation.New(db, domain, access.New(db))
	task, err = service.Advance(t.Context(), actor, document, task.ID, task.Version)
	if err != nil || task.Status != "ACTIVE" || task.Steps[0].Phase != "PLANNED" || task.Steps[0].Confirmation != nil || task.Steps[0].Result != nil {
		t.Fatal("lost candidate should be rebuilt, not silently authorized", task, err)
	}
	task, err = service.Advance(t.Context(), actor, document, task.ID, task.Version)
	if err != nil || task.Status != "WAIT_CONFIRM" || task.Steps[0].Preview.PreviewID == oldPreview {
		t.Fatal("replacement preview missing", task, err)
	}
	input.ExpectedVersion = task.Version
	if _, err = service.Control(t.Context(), actor, document, task.ID, input); err == nil {
		t.Fatal("old preview confirmation approved replacement candidate")
	}
	input.PreviewID = task.Steps[0].Preview.PreviewID
	task, err = service.Control(t.Context(), actor, document, task.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	task, err = service.Advance(t.Context(), actor, document, task.ID, task.Version)
	if err != nil || task.Status != "COMPLETED" || task.Steps[0].Phase != "APPLIED" || task.Steps[0].Presentation.Status != "NOT_PRESENTED" {
		t.Fatal(task, err)
	}
}

type heldCommit struct {
	automation.Workspace
	entered, release chan struct{}
}

func (w *heldCommit) ExecuteCommand(ctx context.Context, document string, request workspace.CommandRequest) error {
	close(w.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.release:
		return w.Workspace.ExecuteCommand(ctx, document, request)
	}
}

func TestAutomationStopArchivesInflightCommitWithoutNewActions(t *testing.T) {
	db, domain, view := automationFixture(t)
	held := &heldCommit{Workspace: domain, entered: make(chan struct{}), release: make(chan struct{})}
	service := automation.New(db, held, access.New(db))
	actor, document := access.DefaultUserID, view.Document.ID
	snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(t.Context(), actor, document, automation.CreateTask{RequestID: uuid.NewString(), Goal: "Stop in-flight commit", ExecutionMode: "AUTOMATIC", ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"XY"}`)}, {ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"YZ"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	task, err = service.Advance(t.Context(), actor, document, task.ID, task.Version)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		task automation.Task
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		next, e := service.Advance(t.Context(), actor, document, task.ID, task.Version)
		done <- outcome{next, e}
	}()
	select {
	case <-held.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("commit did not enter")
	}
	current, err := service.GetTask(t.Context(), actor, document, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Control(t.Context(), actor, document, task.ID, automation.Control{Action: "stop", ExpectedVersion: current.Version}); err != nil {
		t.Fatal(err)
	}
	close(held.release)
	result := <-done
	if result.err != nil || result.task.Status != "STOPPED" || result.task.Steps[0].Result == nil || result.task.Steps[0].Phase != "APPLIED" || result.task.Steps[1].Action != nil {
		t.Fatal("stop lost receipt or activated the next step", result)
	}
	if _, err = service.Advance(t.Context(), actor, document, task.ID, result.task.Version); err == nil {
		t.Fatal("stopped task issued a new action")
	}
	// A process may die after the authoritative commit and explicit stop, but
	// before copying the receipt into the task. Reconciliation is read-only
	// with respect to the model and remains available for the retired epoch.
	interrupted := result.task
	interrupted.Steps[0].Result = nil
	interrupted.Steps[0].Phase = "COMMITTING"
	interrupted.Baseline = snapshot.CommandBaseline
	raw, _ := json.Marshal(interrupted)
	if _, err = db.Exec(t.Context(), `UPDATE occccad.automation_tasks SET payload=$1,lease_until=0,lease_token='' WHERE id=$2`, raw, task.ID); err != nil {
		t.Fatal(err)
	}
	service = automation.New(db, domain, access.New(db))
	reconciled, err := service.ReconcileTaskReceipts(t.Context(), actor, document, task.ID)
	if err != nil || reconciled.Status != "STOPPED" || reconciled.Steps[0].Result == nil || reconciled.Steps[0].Phase != "APPLIED" {
		t.Fatal("retired epoch lost a committed fact", reconciled, err)
	}
	var commits int
	if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, reconciled.Steps[0].Action.RequestID).Scan(&commits); err != nil || commits != 1 {
		t.Fatal("receipt recovery repeated a CAD write", commits, err)
	}
	again, err := service.ReconcileTaskReceipts(t.Context(), actor, document, task.ID)
	if err != nil || again.Version != reconciled.Version {
		t.Fatal("receipt replay changed an already reconciled journal", again, err)
	}
}
