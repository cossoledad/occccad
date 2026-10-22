package assistant

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

// Only the provider's public decisions are scripted. Messages, durable calls,
// scheduler recovery, queries, previews, evaluation and commits use real paths.
type requestFixture struct {
	s        *Service
	domain   *workspace.Service
	r        Run
	provider *questionProvider
}

func newRequestFixture(t *testing.T, text string) *requestFixture {
	t.Helper()
	s, domain, view, actor := assistantFixture(t, "http://127.0.0.1:1")
	provider := &questionProvider{} // Complete fails if a test accidentally calls a model.
	s.provider = provider
	if _, err := s.SaveKey(t.Context(), actor, "synthetic-local-test-key"); err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateRun(t.Context(), actor, view.Document.ID, RunInput{RequestID: uuid.NewString(), Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return &requestFixture{s: s, domain: domain, r: r, provider: provider}
}

func (f *requestFixture) refresh(t *testing.T) {
	t.Helper()
	var err error
	f.r, err = f.s.GetRun(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *requestFixture) send(t *testing.T, text string) MessageInput {
	t.Helper()
	input := MessageInput{RequestID: uuid.NewString(), Text: text}
	var err error
	f.r, _, err = f.s.AddMessage(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if f.r.Goal != text || f.r.RequestAppliedCount != 0 {
		t.Fatal("message did not start an independent current request", f.r)
	}
	return input
}

func (f *requestFixture) call(t *testing.T, message Message) (CallRecord, []Result) {
	t.Helper()
	c := controlledToolCall(t, f.s, f.r, message)
	var err error
	f.r, err = f.s.mutateRun(t.Context(), f.r.ID, func(_ database.Tx, r *Run) error {
		r.PendingCallID = c.ID
		r.Status = "PLANNING"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.r, err = f.s.Step(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID)
	if err != nil {
		t.Fatal(err)
	}
	results := []Result{}
	for _, tool := range message.ToolCalls {
		var raw []byte
		if err = f.s.db.QueryRow(t.Context(), `SELECT payload FROM occccad.assistant_tool_results WHERE call_id=$1 AND tool_id=$2`, c.ID, tool.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var result Result
		if err = json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		results = append(results, result)
	}
	return c, results
}

func (f *requestFixture) stage(t *testing.T, op string, args any, source *QueryResultReference) string {
	t.Helper()
	_, results := f.call(t, toolMessage("cad_describe_operation", map[string]any{"operationId": op}))
	if results[0].IsError {
		t.Fatal(string(results[0].Content))
	}
	step := plannedStep{StepSpec: automation.StepSpec{ID: "step", OperationID: op, Version: 1, Arguments: marshalJSON(args)}}
	if source != nil {
		step.ResultBindings = []QueryResultBinding{{Target: "selectionSet", Source: *source}}
	}
	_, results = f.call(t, toolMessage("cad_create_task", map[string]any{"steps": []plannedStep{step}}))
	if results[0].IsError || f.r.ActiveTaskID == "" {
		t.Fatal(op, string(results[0].Content))
	}
	return f.r.ActiveTaskID
}

func (f *requestFixture) drain(t *testing.T) {
	t.Helper()
	for i := 0; f.r.ActiveTaskID != "" || f.r.RetiringTaskID != ""; i++ {
		if i > 20 { // Test fixture guard only; no runtime loop limit.
			t.Fatal("stage did not reach a known boundary", f.r)
		}
		var err error
		f.r, err = f.s.Step(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.r.PendingCallID != "" {
		// Resume the suspended public tool response after its stage boundary.
		var err error
		f.r, err = f.s.Step(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID)
		if err != nil || f.r.PendingCallID != "" {
			t.Fatal("stage response did not settle", f.r, err)
		}
	}
}

func (f *requestFixture) apply(t *testing.T, op string, args any, source *QueryResultReference) string {
	t.Helper()
	id := f.stage(t, op, args, source)
	f.drain(t)
	task, err := f.s.automation.GetTask(t.Context(), f.r.ActorID, f.r.DocumentID, id)
	if err != nil || task.Status != "COMPLETED" || task.Steps[0].Result == nil {
		t.Fatal(op, task, err)
	}
	if task.Steps[0].Result.Output != nil {
		return task.Steps[0].Result.Output.ID
	}
	return ""
}

func (f *requestFixture) finish(t *testing.T, entry, text string) CallRecord {
	t.Helper()
	message := Message{Role: "assistant", Content: text}
	if entry == "cad_finish" {
		message = toolMessage("cad_finish", map[string]any{"summary": text})
	}
	c, results := f.call(t, message)
	if len(results) > 0 && results[0].IsError || f.r.Status != "COMPLETED" || f.r.Reply != text || f.r.PendingCallID != "" {
		t.Fatal("request did not finish", f.r, results)
	}
	return c
}

func (f *requestFixture) snapshot(t *testing.T) workspace.ModelSnapshot {
	t.Helper()
	snapshot, err := f.domain.ReadModelSnapshot(t.Context(), f.r.DocumentID, "")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func (f *requestFixture) query(t *testing.T, input automation.TopologyQuery) (QueryResultReference, Result) {
	t.Helper()
	c, results := f.call(t, toolMessage("cad_query_topology", input))
	if results[0].IsError {
		t.Fatal(string(results[0].Content))
	}
	ref := queryResultReference(c.ID, c.Response.ToolCalls[0].ID, "cad_query_topology", results[0])
	if ref == nil {
		t.Fatal("missing recorded collection reference")
	}
	return *ref, results[0]
}

func TestAssistantCurrentRequestNativeFilletEditAndNoChange(t *testing.T) {
	for _, entry := range []string{"natural", "cad_finish"} {
		t.Run(entry, func(t *testing.T) {
			f := newRequestFixture(t, "创建100mm立方体，全部边R10圆角")
			body := f.r.Scope.WorkingBodyID
			sketch := f.apply(t, "sketch.create", map[string]any{"bodyId": body, "plane": "XY"}, nil)
			f.apply(t, "sketch.rectangle", map[string]any{"sketchId": sketch, "width": 100, "height": 100, "unit": "mm"}, nil)
			f.apply(t, "part.extrude", map[string]any{"bodyId": body, "sketchId": sketch, "length": 100, "unit": "mm"}, nil)
			all, _ := f.query(t, automation.TopologyQuery{RevisionID: f.r.LatestRevisionID, BodyID: body, Kind: "EDGE", Limit: 1})
			base := f.apply(t, "part.fillet", map[string]any{"bodyId": body, "size": 10, "unit": "mm"}, &all)
			f.finish(t, entry, "100mm立方体和全边R10圆角已应用。")
			f.send(t, "在底面中心打直径40mm的通孔")
			holeSketch := f.apply(t, "sketch.create", map[string]any{"bodyId": body, "plane": "XY"}, nil)
			f.apply(t, "sketch.circle", map[string]any{"sketchId": holeSketch, "centerX": 50, "centerY": 50, "diameter": 40, "unit": "mm"}, nil)
			hole := f.apply(t, "part.cut", map[string]any{"bodyId": body, "sketchId": holeSketch, "extent": "THROUGH_ALL", "unit": "mm"}, nil)
			f.finish(t, entry, "中心直径40mm通孔已应用。")

			f.send(t, "给这个通孔的两个圆边做适当的倒圆角")
			radius := 20.0
			_, faces := f.query(t, automation.TopologyQuery{RevisionID: f.r.LatestRevisionID, BodyID: body, Kind: "FACE", Limit: 20, Filter: &automation.TopologyFilter{GeometryType: "CYLINDER", Radius: &automation.TopologyRadiusFilter{EqualMM: &radius, ToleranceMM: 1e-6}}})
			var page struct {
				Items []automation.TopologyObservation `json:"items"`
			}
			_ = json.Unmarshal(faces.Content, &page)
			wall := ""
			for _, face := range page.Items {
				if face.Geometry.MaterialSide != nil && *face.Geometry.MaterialSide == -1 {
					wall = face.ID
				}
			}
			if wall == "" {
				t.Fatal("no current cavity wall")
			}
			rimRef, rims := f.query(t, automation.TopologyQuery{RevisionID: f.r.LatestRevisionID, BodyID: body, Kind: "EDGE", Limit: 1, Filter: &automation.TopologyFilter{GeometryType: "CIRCLE", Radius: &automation.TopologyRadiusFilter{EqualMM: &radius, ToleranceMM: 1e-6}, AdjacentFaceID: wall}})
			var rimPage map[string]any
			_ = json.Unmarshal(rims.Content, &rimPage)
			if rimPage["matchedCount"] != float64(2) || rimPage["returnedCount"] != float64(1) {
				t.Fatal(rimPage)
			}
			local := f.apply(t, "part.fillet", map[string]any{"bodyId": body, "size": 2, "unit": "mm"}, &rimRef)
			_, checkResults := f.call(t, toolMessage("cad_verify_part", map[string]any{"revisionId": f.r.LatestRevisionID, "checks": []automation.PartCheck{{ID: "local", Kind: "HOLE_RIM_FILLET", BodyID: body, FeatureID: local, ReferenceFeatureID: hole, RadiusPolicy: "DELEGATED", Unit: "mm"}}}))
			if checkResults[0].IsError || !bytes.Contains(checkResults[0].Content, []byte(`"passed":true`)) {
				t.Fatal(string(checkResults[0].Content))
			}
			f.finish(t, entry, "两个孔口圆边已采用2mm圆角。")
			before := f.snapshot(t)
			applied, stages := f.r.AppliedCount, f.r.StageCount
			var transactionsBefore int
			if err := f.s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE workspace_id=$1`, before.WorkspaceID).Scan(&transactionsBefore); err != nil {
				t.Fatal(err)
			}

			keep := f.send(t, "可以，保留")
			f.finish(t, entry, "好的，保留当前结果。")
			var transactionsAfter int
			if err := f.s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE workspace_id=$1`, before.WorkspaceID).Scan(&transactionsAfter); err != nil || transactionsAfter != transactionsBefore {
				t.Fatal("acknowledgment created a CAD transaction", transactionsBefore, transactionsAfter, err)
			}
			if after := f.snapshot(t); after.CommandBaseline != before.CommandBaseline || !bytes.Equal(after.Model, before.Model) || f.r.AppliedCount != applied || f.r.StageCount != stages || f.r.RequestAppliedCount != 0 {
				t.Fatal("acknowledgment wrote CAD or inherited historical applied state", f.r)
			}
			_, turn, err := f.s.AddMessage(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, keep)
			if err != nil || turn.Status != "ANSWERED" || turn.Reply != "好的，保留当前结果。" {
				t.Fatal("message replay changed meaning", turn, err)
			}

			f.send(t, "不喜欢，改成3mm")
			messages, _, err := f.s.buildContext(t.Context(), f.r, nil)
			if err != nil || !strings.Contains(messages[1].Content, local) || !strings.Contains(messages[1].Content, "可以，保留") || strings.Contains(messages[1].Content, "requirements") || !strings.Contains(messages[1].Content, "selectionSetRef") {
				t.Fatal("current request lost recent fillet/context or retained compiled history", err)
			}
			var part workspace.PartModel
			_ = json.Unmarshal(before.Model, &part)
			_, queried := f.call(t, toolMessage("cad_query_model", automation.Query{RevisionID: before.RevisionID, Kind: "PARAMETER", OwnerID: local, Fields: []string{"ownerId", "displayValue", "unit"}, Limit: 10}))
			var parameters automation.QueryResult
			if queried[0].IsError || json.Unmarshal(queried[0].Content, &parameters) != nil || !parameters.Complete || parameters.MatchedCount != 1 || len(parameters.Items) != 1 {
				t.Fatal("recent fillet parameter could not be discovered through the actual tool", string(queried[0].Content))
			}
			parameter := parameters.Items[0]
			if parameter.Ref.Kind != "PARAMETER" || parameter.Properties["ownerId"] != local || parameter.Properties["displayValue"] != float64(2) || parameter.Properties["unit"] != "mm" {
				t.Fatal("parameter query pointed outside the recent fillet", parameter)
			}
			taskID := f.stage(t, "parameter.set_literal", map[string]any{"parameterId": parameter.Ref.ID, "value": 3, "unit": "mm"}, nil)
			// Lose the application stack after the real commit, before its Run checkpoint.
			task, err := f.s.automation.GetTask(t.Context(), f.r.ActorID, f.r.DocumentID, taskID)
			if err != nil {
				t.Fatal(err)
			}
			for task.Status == "ACTIVE" {
				task, err = f.s.automation.Advance(t.Context(), f.r.ActorID, f.r.DocumentID, task.ID, task.Version)
				if err != nil {
					t.Fatal(err)
				}
			}
			f.s = New(f.s.db, f.s.automation, f.s.options)
			f.s.provider = f.provider
			t.Cleanup(f.s.Close)
			f.drain(t)
			f.finish(t, entry, "已将最近的孔口圆角改为3mm，其余特征保留。")
			after := f.snapshot(t)
			var changed workspace.PartModel
			_ = json.Unmarshal(after.Model, &changed)
			if after.Sequence != before.Sequence+1 || len(changed.Features) != len(part.Features) || f.r.RequestAppliedCount != 1 || f.r.AppliedCount != applied+1 || f.r.Targets["FILLET"] != local {
				t.Fatal("edit did not affect exactly one target feature", f.r)
			}
			for i, feature := range changed.Features {
				if feature.ID == local {
					if feature.Length != 3 || len(feature.Selections) != 2 || !bytes.Equal(marshalJSON(feature.Selections), marshalJSON(part.Features[i].Selections)) {
						t.Fatal("local radius/selection changed incorrectly", feature)
					}
				} else if !bytes.Equal(marshalJSON(feature), marshalJSON(part.Features[i])) {
					t.Fatal("unrelated feature changed", feature.ID)
				}
			}
			var commits int
			if err = f.s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, task.Steps[0].Result.Receipt.RequestID).Scan(&commits); err != nil || commits != 1 {
				t.Fatal("recovery duplicated commit", commits, err)
			}
			f.send(t, "保持当前3mm圆角，已有结果无需修改")
			_, optional := f.call(t, toolMessage("cad_verify_part", map[string]any{"revisionId": f.r.LatestRevisionID, "checks": []automation.PartCheck{{ID: "different-expectation", Kind: "SELECTED_EDGES_FILLET", BodyID: body, FeatureID: local, Radius: 4, Unit: "mm", SelectionSet: rimPage["selectionSet"].(string), ExpectedCount: 2}}}))
			if optional[0].IsError || !bytes.Contains(optional[0].Content, []byte(`"passed":false`)) {
				t.Fatal("mismatched optional check was not honest", string(optional[0].Content))
			}
			f.finish(t, entry, "当前已为3mm，无需修改。")
			if current := f.snapshot(t); current.CommandBaseline != after.CommandBaseline || f.r.RequestAppliedCount != 0 {
				t.Fatal("already-satisfied request wrote CAD")
			}
			if f.provider.calls.Load() != 0 || f.r.ModelCalls != 0 {
				t.Fatal("test called a model")
			}
			report := map[string]any{"entry": entry, "documentId": f.r.DocumentID, "baseFilletId": base, "holeId": hole, "localFilletId": local, "adoptedRadiusMM": 2, "editedRadiusMM": 3, "keepRevision": before.RevisionID, "keepCADTransactionsDelta": transactionsAfter - transactionsBefore, "editRevision": after.RevisionID, "editRevisionDelta": after.Sequence - before.Sequence, "preservedOtherFeatures": len(part.Features) - 1, "selectedRims": 2, "queryReturnedCount": 1, "recoveredCommitCount": commits, "modelCalls": f.provider.calls.Load()}
			path, _ := filepath.Abs("../../../build/agent-check/assistant-request-boundaries/native-" + entry + ".json")
			if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, marshalJSON(report), 0644); err != nil {
				t.Fatal(err)
			}
			t.Logf("real message entries + native CAD: R2 chosen, keep unchanged, only local R3 edited; %s", path)
		})
	}
}
