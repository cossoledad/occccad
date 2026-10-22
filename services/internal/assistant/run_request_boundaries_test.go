package assistant

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssistantCurrentRequestGeometryFailureCanEnd(t *testing.T) {
	for _, entry := range []string{"natural", "cad_finish"} {
		t.Run(entry, func(t *testing.T) {
			f := newRequestFixture(t, "创建20mm立方体")
			body := f.r.Scope.WorkingBodyID
			sketch := f.apply(t, "sketch.create", map[string]any{"bodyId": body, "plane": "XY"}, nil)
			f.apply(t, "sketch.rectangle", map[string]any{"sketchId": sketch, "width": 20, "height": 20, "unit": "mm"}, nil)
			f.apply(t, "part.extrude", map[string]any{"bodyId": body, "sketchId": sketch, "length": 20, "unit": "mm"}, nil)
			f.finish(t, entry, "立方体已创建。")
			f.send(t, "创建一个辅助草图，并给全部边倒圆角")
			f.apply(t, "sketch.create", map[string]any{"bodyId": body, "plane": "YZ"}, nil)
			ref, _ := f.query(t, automation.TopologyQuery{RevisionID: f.r.LatestRevisionID, BodyID: body, Kind: "EDGE", Limit: 1})
			before := f.snapshot(t)
			f.stage(t, "part.fillet", map[string]any{"bodyId": body, "size": 1000, "unit": "mm"}, &ref)
			f.drain(t)
			if f.r.Status != "PLANNING" || f.r.Failure == nil || f.r.RequestAppliedCount != 1 {
				t.Fatal("known failure did not return to agent with partial receipts", f.r)
			}
			diagnostic := f.r.Failure.Code
			f.finish(t, entry, "辅助草图已创建；圆角因几何求值失败未应用，当前实体保留。")
			if f.r.Failure == nil || f.r.Failure.Code != diagnostic || f.r.ActiveTaskID != "" || f.snapshot(t).CommandBaseline != before.CommandBaseline {
				t.Fatal("finish fabricated CAD success or cleared diagnostic", f.r)
			}
			if _, err := f.s.Step(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID); err == nil || f.provider.calls.Load() != 0 {
				t.Fatal("finished failure retried itself", err)
			}
		})
	}
}

func TestAssistantCurrentRequestLateStageAndCompletionFences(t *testing.T) {
	f := newRequestFixture(t, "建立草图")
	id := f.stage(t, "sketch.create", map[string]any{"bodyId": f.r.Scope.WorkingBodyID, "plane": "XY"}, nil)
	old := f.r
	activeReply := controlledToolCall(t, f.s, old, Message{Role: "assistant", Content: "完成。"})
	if err := f.s.processCall(t.Context(), old, activeReply, nil); err == nil || !strings.Contains(err.Error(), "AI_EXECUTION_PENDING") {
		t.Fatal("active CAD operation was called completed", err)
	}
	task, err := f.s.automation.GetTask(t.Context(), f.r.ActorID, f.r.DocumentID, id)
	if err != nil {
		t.Fatal(err)
	}
	for task.Status == "ACTIVE" {
		task, err = f.s.automation.Advance(t.Context(), f.r.ActorID, f.r.DocumentID, id, task.Version)
		if err != nil {
			t.Fatal(err)
		}
	}
	late := controlledToolCall(t, f.s, old, Message{Role: "assistant", Content: "旧操作已完成"})
	f.send(t, "可以，保留")
	f.send(t, "保留当前结果即可")
	if f.r.RetiringTaskID != id {
		t.Fatal("a second message lost the retiring boundary", f.r)
	}
	if err = f.s.processCall(t.Context(), old, late, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("late reply completed new work", err)
	}
	if err = f.s.checkpointTask(t.Context(), old, task, "PLANNING", "COMPLETED", true, false, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("old epoch checkpoint advanced new work", err)
	}
	for _, entry := range []string{"natural", "cad_finish"} {
		message := Message{Role: "assistant", Content: "保留。"}
		if entry == "cad_finish" {
			message = toolMessage("cad_finish", map[string]any{"summary": "保留。"})
		}
		call := controlledToolCall(t, f.s, f.r, message)
		if entry == "natural" {
			if err = f.s.processCall(t.Context(), f.r, call, nil); err == nil || !strings.Contains(err.Error(), "AI_EXECUTION_PENDING") {
				t.Fatal("natural completion ignored retirement", err)
			}
		} else {
			result := f.s.executeTool(t.Context(), f.r, call, message.ToolCalls[0])
			result, err = f.s.recordTool(t.Context(), f.r, &call, 0, message.ToolCalls[0], result)
			if err != nil || !result.IsError || !bytes.Contains(result.Content, []byte("AI_EXECUTION_PENDING")) {
				t.Fatal("tool completion ignored retirement", result, err)
			}
		}
	}
	f.refresh(t)
	if f.r.Status == "COMPLETED" || f.r.AppliedCount != 0 {
		t.Fatal("unobserved receipts were called applied", f.r)
	}
	f.drain(t)
	if f.r.AppliedCount != 1 || f.r.RequestAppliedCount != 0 || f.r.Status != "PLANNING" || f.r.Goal != "保留当前结果即可" {
		t.Fatal("retired receipt was attributed to the new request", f.r)
	}
	before := f.snapshot(t)
	f.finish(t, "natural", "好的，保留当前结果。")
	if f.snapshot(t).CommandBaseline != before.CommandBaseline {
		t.Fatal("retirement response modified CAD")
	}
	if err = f.s.processCall(t.Context(), old, late, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("late reply revived a finished request", err)
	}
}

func TestAssistantCurrentRequestStaleCompletionAndFinishReplay(t *testing.T) {
	for _, entry := range []string{"natural", "cad_finish"} {
		t.Run(entry, func(t *testing.T) {
			f := newRequestFixture(t, "可以，保留")
			message := Message{Role: "assistant", Content: "保留。"}
			if entry == "cad_finish" {
				message = toolMessage("cad_finish", map[string]any{"summary": "保留。"})
			}
			stale := controlledToolCall(t, f.s, f.r, message)
			if err := f.domain.ExecuteCommand(t.Context(), f.r.DocumentID, workspace.CommandRequest{Type: "RENAME_BODY", BodyID: f.r.Scope.WorkingBodyID, Name: "Human edit", ActorID: f.r.ActorID, RequestID: uuid.NewString()}); err != nil {
				t.Fatal(err)
			}
			if entry == "natural" {
				if err := f.s.processCall(t.Context(), f.r, stale, nil); err == nil || !strings.Contains(err.Error(), "AI_COMPLETION_STALE") {
					t.Fatal("stale response reported completion", err)
				}
			} else {
				result := f.s.executeTool(t.Context(), f.r, stale, message.ToolCalls[0])
				result, err := f.s.recordTool(t.Context(), f.r, &stale, 0, message.ToolCalls[0], result)
				if err != nil || !result.IsError || !bytes.Contains(result.Content, []byte("AI_COMPLETION_STALE")) {
					t.Fatal("stale finish reported completion", result, err)
				}
			}
			f.refresh(t)
			before := f.snapshot(t)
			c := f.finish(t, entry, "已保留当前结果。")
			if entry == "cad_finish" {
				tool := c.Response.ToolCalls[0]
				// Replaying after a restart reads the original normalized result;
				// it does not run completion a second time or change its meaning.
				f.s = New(f.s.db, f.s.automation, f.s.options)
				t.Cleanup(f.s.Close)
				result, err := f.s.recordTool(t.Context(), f.r, &c, 0, tool, Result{CallID: tool.ID, Content: []byte(`{"different":true}`)})
				if err != nil || result.IsError || !bytes.Contains(result.Content, []byte(`"status":"COMPLETED"`)) {
					t.Fatal("finish ledger replay diverged", result, err)
				}
			}
			f.refresh(t)
			if f.r.Status != "COMPLETED" || f.r.RequestAppliedCount != 0 || f.snapshot(t).CommandBaseline != before.CommandBaseline {
				t.Fatal("completion replay wrote CAD", f.r)
			}
		})
	}
}
