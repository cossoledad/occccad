package assistant

import (
	"context"
	"errors"
	"testing"
)

func TestAssistantTurnUsageStableOwnershipRetriesAndRecovery(t *testing.T) {
	f := newRequestFixture(t, "查看当前模型")
	first := f.r.CurrentTurnID
	prepare := func() CallRecord {
		t.Helper()
		f.refresh(t)
		snapshot := f.snapshot(t)
		c, err := f.s.prepareCall(t.Context(), f.r, "", snapshot.CommandBaseline, []Message{{Role: "user", Content: f.r.Goal}})
		if err != nil || c.InputID != "" || c.TurnID != f.r.CurrentTurnID {
			t.Fatal("call source was not frozen independently", c, err)
		}
		if err = f.s.captureCallRequest(t.Context(), &c, []Message{{Role: "user", Content: f.r.Goal}}, planningTools()); err != nil {
			t.Fatal(err)
		}
		if err = f.s.markSent(t.Context(), &c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	failed := prepare()
	usage := &Usage{PromptTokens: 70, CompletionTokens: 30, TotalTokens: 100}
	if err := f.s.receiveCall(t.Context(), &failed, Completion{Usage: usage}, errors.New("transport failed after billed response")); err != nil {
		t.Fatal(err)
	}
	if err := f.s.receiveCall(t.Context(), &failed, Completion{Usage: &Usage{TotalTokens: 999}}, errors.New("duplicate result")); err != nil {
		t.Fatal(err)
	}
	f.refresh(t)
	// Epoch changes without a new user turn must keep the exact same owner.
	var err error
	f.r, err = f.s.ControlRun(t.Context(), f.r.ActorID, f.r.DocumentID, f.r.ID, RunControl{Action: "mode", Mode: "AUTOMATIC", ExpectedVersion: f.r.Version, Epoch: f.r.Epoch})
	if err != nil || f.r.CurrentTurnID != first {
		t.Fatal(f.r, err)
	}
	retry := prepare()
	f.send(t, "给边线倒圆角")
	second := f.r.CurrentTurnID
	if err = f.s.receiveCall(t.Context(), &retry, Completion{}, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	if err = f.s.receiveCall(t.Context(), &retry, Completion{}, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	old, _, _ := loadInput(t.Context(), f.s.db, first)
	if old.Usage.TotalTokens != 100 || old.Usage.UnknownAttempts != 1 || old.Usage.PendingCalls != 0 || old.Usage.Calls != 2 {
		t.Fatal("retry/unknown double counted or moved to latest turn", old.Usage)
	}
	current := prepare()
	if err = f.s.receiveCall(t.Context(), &current, Completion{Message: Message{Role: "assistant", Content: "公开回复", ReasoningContent: "PRIVATE_NOT_EXPORTED"}, Usage: &Usage{PromptTokens: 30, CompletionTokens: 10, TotalTokens: 40}}, nil); err != nil {
		t.Fatal(err)
	}
	f.s = New(f.s.db, f.s.automation, f.s.options)
	t.Cleanup(f.s.Close)
	if err = f.s.receiveCall(t.Context(), &retry, Completion{Usage: &Usage{PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60}}, errors.New("late billed result")); err != nil {
		t.Fatal(err)
	}
	if err = f.s.receiveCall(t.Context(), &retry, Completion{Usage: &Usage{TotalTokens: 60}}, errors.New("replayed late result")); err != nil {
		t.Fatal(err)
	}
	f.refresh(t)
	a, _, _ := loadInput(t.Context(), f.s.db, first)
	b, _, _ := loadInput(t.Context(), f.s.db, second)
	if a.Usage.TotalTokens != 160 || a.Usage.PromptTokens != 120 || a.Usage.ReportedCalls != 2 || a.Usage.UnknownAttempts != 0 || a.Usage.Calls != 2 || b.Usage.TotalTokens != 40 || b.Usage.Calls != 1 || f.r.Usage.TotalTokens != 200 || f.r.Usage.ReportedCalls != 3 || f.r.Usage.UnknownAttempts != 0 || f.r.Usage.PendingCalls != 0 {
		t.Fatal("per-call attribution was lost after recovery", a.Usage, b.Usage, f.r.Usage)
	}
	if retry.TurnID != first || retry.InputID != "" || f.r.CurrentTurnID != second {
		t.Fatal("late call was re-attributed", retry)
	}
}
