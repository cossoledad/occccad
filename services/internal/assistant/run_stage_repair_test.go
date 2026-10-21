package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssistantRunInvalidBindingRepairsOnlyUnappliedNativeSuffix(t *testing.T) {
	for _, restart := range []bool{false, true} {
		name := "direct-claim-error"
		if restart {
			name = "restart-after-durable-rejection"
		}
		t.Run(name, func(t *testing.T) {
			s, domain, view, r := stageFixture(t)
			var err error
			r, err = s.mutateRun(t.Context(), r.ID, func(_ database.Tx, current *Run) error {
				current.Requirements = current.Requirements[:2]
				current.Goal = "在 XY 平面建立 80×50 mm 矩形，拉伸 12 mm"
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			r = stageForTest(t, s, r, []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}})
			r = finishStageForTest(t, s, r)
			sketch := r.Targets["SKETCH"]
			rectangleStep := uuid.NewString()
			r = stageForTest(t, s, r, []automation.StepSpec{
				{ID: rectangleStep, OperationID: "sketch.rectangle", Version: 1, Arguments: marshalJSON(map[string]any{"sketchId": sketch, "width": 80, "height": 50, "unit": "mm"})},
				{ID: uuid.NewString(), OperationID: "part.extrude", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "length": 12, "unit": "mm"}), TypedBindings: []automation.InputBinding{{Target: "sketchId", SourceStepID: rectangleStep, OutputID: "PRIMARY", Representation: "ENTITY_ID"}}},
			})
			oldRun := r
			task, err := s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, r.ActiveTaskID)
			if err != nil {
				t.Fatal(err)
			}
			// Commit the real rectangle but interrupt before its Run checkpoint.
			for task.Steps[0].Result == nil {
				task, err = s.automation.Advance(t.Context(), r.ActorID, r.DocumentID, task.ID, task.Version)
				if err != nil {
					t.Fatal(err)
				}
			}
			rectangle := *task.Steps[0].Result
			if r.AppliedCount != 1 || task.Steps[1].Action != nil {
				t.Fatal("fixture did not interrupt before the Run receipt boundary", r, task)
			}
			if restart {
				_, rejectErr := s.automation.Advance(t.Context(), r.ActorID, r.DocumentID, task.ID, task.Version)
				var failure *automation.Fault
				if !errors.As(rejectErr, &failure) || failure.Code != "INVALID_BINDING" {
					t.Fatal("invalid role used as an ID was accepted", rejectErr)
				}
				task, err = s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, task.ID)
				if err != nil || task.Status != "PAUSED" || task.Steps[1].Phase != "FAILED" || task.Steps[1].Failure == nil || task.Steps[1].Failure.Code != "INVALID_BINDING" {
					t.Fatal("claim rejection was not durable before restart", task, err)
				}
				s = New(s.db, s.automation, s.options)
				t.Cleanup(s.Close)
			}
			s.session(r.ID).messages = []Message{{Role: "system", Content: "old working set without the rejected binding"}}
			if err = s.advanceStage(t.Context(), r); err != nil {
				t.Fatal("repairable claim escaped into generic WAITING_INPUT", err)
			}
			r, err = loadRun(t.Context(), s.db, r.ID, false)
			if err != nil || r.Status != "PLANNING" || r.ActiveTaskID != "" || r.AppliedCount != 2 || r.StepCount != 2 || r.LatestAppliedRevisionID != rectangle.Receipt.RevisionID || r.Failure == nil || r.Failure.Code != "INVALID_BINDING" {
				t.Fatal("applied prefix or repair diagnosis was lost", r, err)
			}
			task, err = s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, task.ID)
			if err != nil || task.Status != "STOPPED" || !task.Steps[0].ReceiptRecorded || task.Steps[1].Action != nil || task.Steps[1].Result != nil || task.Steps[1].Failure == nil {
				t.Fatal("old rejected stage remained executable or lost its history", task, err)
			}
			if _, present := s.sessions[r.ID]; present {
				t.Fatal("same-Head repair retained a stale language working set")
			}
			messages, _, err := s.buildContext(t.Context(), r, nil)
			if err != nil || !strings.Contains(string(marshalJSON(messages)), "INVALID_BINDING") || !strings.Contains(string(marshalJSON(messages)), rectangle.Receipt.RevisionID) {
				t.Fatal("fresh planning context lacks rejected input/authoritative prefix", err)
			}
			if err = s.failStage(t.Context(), oldRun, task, r.Failure); err != nil {
				t.Fatal(err)
			}
			r, err = loadRun(t.Context(), s.db, r.ID, false)
			if err != nil || r.Status != "PLANNING" || r.AppliedCount != 2 || r.StepCount != 2 {
				t.Fatal("replaying one failure boundary counted as a second repair", r, err)
			}
			// A new plan uses the already-existing formal Sketch ID. It has no
			// rectangle step and receives a new request only for the new extrusion.
			r = stageForTest(t, s, r, []automation.StepSpec{{ID: uuid.NewString(), OperationID: "part.extrude", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "sketchId": sketch, "length": 12, "unit": "mm"})}})
			r = finishStageForTest(t, s, r)
			verified, err := s.verifyGoal(t.Context(), r)
			if err != nil || verified.Status != "VERIFIED" || r.AppliedCount != 3 || r.StepCount != 3 || r.StageCount != 3 || r.PresentedRevisionID != "" {
				t.Fatal("same Run did not finish the remaining native modeling operation", r, verified, err)
			}
			snapshot, err := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
			if err != nil {
				t.Fatal(err)
			}
			var part workspace.PartModel
			if err = json.Unmarshal(snapshot.Model, &part); err != nil || len(part.Features) != 2 {
				t.Fatal("repair duplicated model features", part, err)
			}
			for _, feature := range part.Features {
				if feature.Sketch != nil && len(feature.Sketch.Entities) != 4 {
					t.Fatal("rectangle was replayed", feature)
				}
			}
			var transactions int
			if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, rectangle.Receipt.RequestID).Scan(&transactions); err != nil || transactions != 1 {
				t.Fatal("the original rectangle request was committed twice", transactions, err)
			}
			var volume float64
			geometryKey := ""
			for _, body := range part.Bodies {
				if body.ID == view.Part.ActiveBodyID {
					geometryKey = body.GeometryKey
				}
			}
			if err = s.db.QueryRow(t.Context(), `SELECT volume FROM occccad.geometry_artifacts WHERE geometry_key=$1`, geometryKey).Scan(&volume); err != nil || math.Abs(volume-48000) > 1e-6 {
				t.Fatal("native solid differs from 80x50x12 mm", volume, err)
			}
			t.Logf("native same-Run repair: original rectangle receipt %s archived once; new stage only extruded existing Sketch %s; volume %.6f mm^3", rectangle.Receipt.RequestID, sketch, volume)
		})
	}
}

type rejectedStageWorkspace struct {
	*syntheticStageWorkspace
	code     string
	previews int
}

func (w *rejectedStageWorkspace) PreviewCommand(_ context.Context, _ string, _ workspace.CommandRequest) (workspace.CommandPreview, error) {
	w.previews++
	return workspace.CommandPreview{}, &automation.Fault{Code: w.code, Message: "controlled deterministic input rejection"}
}

func TestAssistantRunRepeatedStageFailureRecordsSemanticIntentWithoutBlocking(t *testing.T) {
	s, domain, view, r := stageFixture(t)
	initial, err := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
	if err != nil {
		t.Fatal(err)
	}
	controlled := &rejectedStageWorkspace{syntheticStageWorkspace: &syntheticStageWorkspace{head: initial, versions: map[string]workspace.ModelSnapshot{initial.RevisionID: initial}, receipts: map[string]workspace.CommandReceipt{}, candidates: map[string]workspace.CommandBaseline{}}, code: "VALIDATION_FAILED"}
	s.automation = automation.New(s.db, controlled, access.New(s.db))
	if _, err = s.db.Exec(t.Context(), `INSERT INTO occccad.assistant_contracts(run_id,operation_id,digest) VALUES($1,'sketch.create',$2)`, r.ID, s.automation.Catalog.Digest); err != nil {
		t.Fatal(err)
	}
	var firstDigest string
	for attempt, label := range []string{"original_local_label", "different_local_label"} {
		call := ToolCall{ID: uuid.NewString(), Type: "function"}
		call.Function.Name = "cad_create_task"
		call.Function.Arguments = string(marshalJSON(map[string]any{"steps": []automation.StepSpec{{ID: label, OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}}}))
		_, err = s.createStage(t.Context(), r, CallRecord{ID: uuid.NewString(), Baseline: initial.CommandBaseline}, call)
		if err != nil {
			t.Fatal(err)
		}
		r, err = loadRun(t.Context(), s.db, r.ID, false)
		if err != nil || attempt > 0 && r.LastStageFailureDigest != firstDigest {
			t.Fatal("accepting a new task erased the prior semantic failure", r, err)
		}
		if err = s.advanceStage(t.Context(), r); err != nil {
			t.Fatal(err)
		}
		r, err = loadRun(t.Context(), s.db, r.ID, false)
		if err != nil || r.ActiveTaskID != "" || r.AppliedCount != 0 || r.StepCount != 0 || r.LatestRevisionID != initial.RevisionID {
			t.Fatal("controlled rejected input invented progress", r, err)
		}
		if attempt == 0 {
			if r.Status != "PLANNING" || r.LastStageFailureDigest == "" {
				t.Fatal("first deterministic failure did not permit a new plan", r)
			}
			firstDigest = r.LastStageFailureDigest
		} else if r.Status != "PLANNING" || r.LastStageFailureDigest != firstDigest {
			t.Fatal("repeated diagnostic blocked the model from revising its plan", r)
		}
	}
	if controlled.previews != 2 || controlled.commits != 0 {
		t.Fatal("controlled rejection invented a CAD commit", controlled.previews, controlled.commits)
	}
	var repeated int
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_run_events WHERE run_id=$1 AND payload->>'type'='REPEATED_DIAGNOSTIC'`, r.ID).Scan(&repeated); err != nil || repeated != 1 {
		t.Fatal("semantic repetition was not retained as diagnostic evidence", repeated, err)
	}
}

func TestAssistantStageFailureIntentPreservesBindingStrategy(t *testing.T) {
	output := automation.TypedOutput{Role: "PRIMARY", Entity: automation.EntityRef{Kind: "FEATURE", ID: uuid.NewString()}}
	task := automation.Task{Steps: []automation.Step{
		{StepSpec: automation.StepSpec{ID: "old-source", OperationID: "sketch.create", Version: 1}, Result: &automation.ActionResult{TypedOutputs: []automation.TypedOutput{output}}},
		{StepSpec: automation.StepSpec{ID: "old-target", OperationID: "part.extrude", Version: 1, Arguments: json.RawMessage(`{"length":12,"unit":"mm"}`), TypedBindings: []automation.InputBinding{{SourceStepID: "old-source", Target: "sketchId", OutputID: "PRIMARY", Representation: "ENTITY_ID"}}}},
	}}
	before := modelcore.ValueDigest(marshalJSON(stageFailureIntent(task)))
	task.Steps[0].ID, task.Steps[1].ID = "new-source", "new-target"
	task.Steps[1].TypedBindings[0].SourceStepID = "new-source"
	if got := modelcore.ValueDigest(marshalJSON(stageFailureIntent(task))); got != before {
		t.Fatal("local reference labels changed the semantic intent")
	}
	task.Steps[1].TypedBindings[0].OutputID = ""
	task.Steps[1].TypedBindings[0].OutputRole = "PRIMARY"
	if got := modelcore.ValueDigest(marshalJSON(stageFailureIntent(task))); got == before {
		t.Fatal("a corrected role selector was mistaken for unchanged input")
	}
}

func TestAssistantRunStagePermissionRejectionRequiresUserInput(t *testing.T) {
	s, domain, view, r := stageFixture(t)
	initial, err := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
	if err != nil {
		t.Fatal(err)
	}
	controlled := &rejectedStageWorkspace{syntheticStageWorkspace: &syntheticStageWorkspace{head: initial, versions: map[string]workspace.ModelSnapshot{initial.RevisionID: initial}, receipts: map[string]workspace.CommandReceipt{}, candidates: map[string]workspace.CommandBaseline{}}, code: "FORBIDDEN"}
	s.automation = automation.New(s.db, controlled, access.New(s.db))
	r = stageForTest(t, s, r, []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}})
	if err = s.advanceStage(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	r, err = loadRun(t.Context(), s.db, r.ID, false)
	if err != nil || r.Status != "WAITING_INPUT" || r.Failure == nil || r.Failure.Code != "FORBIDDEN" || r.ActiveTaskID != "" || controlled.previews != 1 || controlled.commits != 0 {
		t.Fatal("permission rejection entered automatic plan repair", r, err)
	}
}
