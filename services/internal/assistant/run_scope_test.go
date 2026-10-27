//go:build integration

package assistant

import (
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/workspace"
	"testing"
)

func TestAssistantInputRoutingUsesOnlyActualScopePermissions(t *testing.T) {
	f := newRequestFixture(t, "如何创建一个矩形？")
	if f.r.AccessMode != "WRITE" {
		t.Fatal("punctuation or keywords changed permissions", f.r)
	}
	for _, text := range []string{"查看并修改参数", "为什么失败？请修复", "可以把圆角改成3mm吗？", "可以，保留"} {
		f.send(t, text)
		if f.r.AccessMode != "WRITE" || f.r.CurrentTurnID == "" {
			t.Fatal("text classified permissions", f.r)
		}
	}
	old := f.snapshot(t)
	if err := f.domain.ExecuteCommand(t.Context(), f.r.DocumentID, workspace.CommandRequest{Type: "RENAME_BODY", BodyID: f.r.Scope.WorkingBodyID, Name: "Current fixture", ActorID: f.r.ActorID, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	scope := f.r.Scope
	scope.RevisionID = old.RevisionID
	before := f.r
	current, turn, err := f.s.AddMessage(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, MessageInput{RequestID: uuid.NewString(), Text: "修改成3mm", Scope: &scope})
	if err != nil || turn.Kind != "QUERY" || current.Epoch != before.Epoch || current.CurrentTurnID != before.CurrentTurnID {
		t.Fatal("historical context gained write permission from language", turn, current, err)
	}
	snapshot, _ := f.s.automation.ReadSnapshot(t.Context(), f.r.ActorID, f.r.DocumentID, old.RevisionID)
	c, err := f.s.prepareCall(t.Context(), current, turn.ID, snapshot.CommandBaseline, []Message{{Role: "user", Content: turn.Text}})
	if err != nil || c.InputID != turn.ID || c.TurnID != turn.ID {
		t.Fatal("QUERY control identity was repurposed", c, err)
	}
}
