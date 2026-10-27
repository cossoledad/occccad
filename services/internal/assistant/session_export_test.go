//go:build integration

package assistant

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
)

type exportedTestArchive struct {
	Metadata ExportMetadata `json:"metadata"`
	Records  []ExportRecord `json:"records"`
	Complete bool           `json:"complete"`
}

func TestAssistantSessionExportCompleteSnapshotAndTurnIsolation(t *testing.T) {
	f := newRequestFixture(t, "初始会话输入")
	const private = "PRIVATE_REASONING_NEVER_EXPORT"
	const tail = "AFTER_VISIBLE_SUMMARY_LIMIT"
	selected := ""
	var selectedCall string
	for i := 0; i < 37; i++ {
		f.send(t, "查看参数轮次 "+strings.Repeat("a", i+1))
		if i == 2 {
			scope := f.r.Scope
			scope.RevisionID = f.snapshot(t).RevisionID
			scope.Selections = []SelectedReference{{Kind: "BODY", ID: scope.WorkingBodyID, BodyID: scope.WorkingBodyID, Name: "Body selection snapshot", SourceDocumentID: f.r.DocumentID, SourceVersionID: scope.RevisionID}}
			scope.SelectionCount = 1
			var err error
			f.r, _, err = f.s.AddMessage(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, MessageInput{RequestID: uuid.NewString(), Text: "选中实体并创建草图", Scope: &scope})
			if err != nil {
				t.Fatal(err)
			}
			selected = f.r.CurrentTurnID
		}
		snapshot := f.snapshot(t)
		c, err := f.s.prepareCall(t.Context(), f.r, "", snapshot.CommandBaseline, []Message{{Role: "user", Content: f.r.Goal}, {Role: "assistant", Content: "public history", ReasoningContent: private}})
		if err != nil {
			t.Fatal(err)
		}
		if err = f.s.captureCallRequest(t.Context(), &c, []Message{{Role: "user", Content: f.r.Goal}, {Role: "assistant", Content: "public history", ReasoningContent: private}}, planningTools()); err != nil {
			t.Fatal(err)
		}
		if err = f.s.markSent(t.Context(), &c); err != nil {
			t.Fatal(err)
		}
		message := toolMessage("cad_query_model", automation.Query{Kind: "BODY", Limit: 1})
		message.Content = strings.Repeat("公开内容", 1100) + tail
		message.ReasoningContent = private
		if err = f.s.receiveCall(t.Context(), &c, Completion{Message: message, Usage: &Usage{PromptTokens: 80, CompletionTokens: 20, TotalTokens: 100}}, nil); err != nil {
			t.Fatal(err)
		}
		result := f.s.executeTool(t.Context(), f.r, c, message.ToolCalls[0])
		if _, err = f.s.recordTool(t.Context(), f.r, &c, 0, message.ToolCalls[0], result); err != nil {
			t.Fatal(err)
		}
		f.refresh(t)
		if i == 2 {
			selectedCall = c.ID
			f.apply(t, "sketch.create", map[string]any{"bodyId": f.r.Scope.WorkingBodyID, "plane": "XY"}, nil)
		}
	}
	// An older call with no explicit owner must remain unassigned, not guessed
	// into the latest turn or reconstructed from its epoch.
	legacy := controlledToolCall(t, f.s, f.r, Message{Role: "assistant", Content: "legacy public", ReasoningContent: private})
	legacy.TurnID = ""
	if err := saveCall(t.Context(), f.s.db, legacy); err != nil {
		t.Fatal(err)
	}
	archive, err := f.s.OpenExport(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if info, err := archive.spool.Stat(); err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("export spool is not private", err)
	}
	// Updates after opening the one-statement snapshot cannot leak into it.
	f.send(t, "NEW_INPUT_AFTER_EXPORT_SNAPSHOT")
	var data bytes.Buffer
	if err = archive.WriteTo(&data, "json"); err != nil {
		t.Fatal(err)
	}
	var full exportedTestArchive
	if err = json.Unmarshal(data.Bytes(), &full); err != nil {
		t.Fatal(err, data.String()[:min(300, data.Len())])
	}
	counts := map[string]int{}
	for _, record := range full.Records {
		counts[record.Kind]++
		if record.Kind == "stage" {
			var stage struct {
				Epoch       uint64 `json:"epoch"`
				GoalVersion uint64 `json:"goalVersion"`
				TaskID      string `json:"taskId"`
			}
			if err := json.Unmarshal(record.Data, &stage); err != nil || stage.Epoch == 0 || stage.GoalVersion != record.Version || stage.TaskID != record.ID {
				t.Fatal("stage execution version was omitted", stage, err)
			}
		}
	}
	if !full.Complete || full.Metadata.AsOf == "" || !full.Metadata.WorkUnfinished || full.Metadata.UnattributedCalls != 1 || counts["input"] != 39 || counts["call"] != 40 || counts["tool"] != 39 || counts["stage"] != 1 || counts["task"] != 1 {
		t.Fatal("export was restricted to UI pages or omitted durable records", full.Metadata, counts)
	}
	if bytes.Contains(data.Bytes(), []byte(private)) || bytes.Contains(data.Bytes(), []byte("NEW_INPUT_AFTER_EXPORT_SNAPSHOT")) || !bytes.Contains(data.Bytes(), []byte(tail)) || !bytes.Contains(data.Bytes(), []byte("Body selection snapshot")) {
		t.Fatal("export leaked private data or truncated public content")
	}
	for _, format := range []string{"json", "markdown"} {
		scoped, err := f.s.OpenExport(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, selected)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		spoolPath := scoped.spool.Name()
		if err = scoped.WriteTo(&out, format); err != nil {
			scoped.Close()
			t.Fatal(err)
		}
		scoped.Close()
		if _, err := os.Stat(spoolPath); !os.IsNotExist(err) {
			t.Fatal("export spool was not removed", err)
		}
		if !strings.Contains(out.String(), selectedCall) || strings.Contains(out.String(), legacy.ID) || strings.Contains(out.String(), private) || !strings.Contains(out.String(), tail) || !strings.Contains(out.String(), "receipt") {
			t.Fatal("round export lost actual request/tool/CAD data", format)
		}
		if format == "json" {
			var round exportedTestArchive
			if err = json.Unmarshal(out.Bytes(), &round); err != nil {
				t.Fatal(err)
			}
			for _, v := range round.Records {
				if v.Kind != "run" && v.TurnID != selected {
					t.Fatal("another round was exported", v.Kind, v.TurnID)
				}
			}
		}
	}
	if other, err := f.s.OpenExport(t.Context(), uuid.NewString(), f.r.DocumentID, f.r.ID, ""); err == nil {
		other.Close()
		t.Fatal("export bypassed owner access")
	}
	if other, err := f.s.OpenExport(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, uuid.NewString()); err == nil {
		other.Close()
		t.Fatal("export accepted a foreign turn")
	}
	if f.provider.calls.Load() != 0 {
		t.Fatal("export invoked a model")
	}
}
