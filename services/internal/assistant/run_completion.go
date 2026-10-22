package assistant

import (
	"context"
	"strings"

	"github.com/occccad/occccad/internal/database"
)

func completionResult(r Run, c CallRecord) map[string]any {
	return map[string]any{"status": "COMPLETED", "requestVersion": r.GoalVersion, "revisionId": c.Baseline.RevisionID, "appliedCount": r.RequestAppliedCount}
}

// A completion means the agent has ended this request, not that the application
// has interpreted or verified the conversation. CAD effects still require exact
// receipts, and unfinished/uncertain operations must reach their own boundary.
func (s *Service) completeRequestTx(ctx context.Context, tx database.Tx, current *Run, r Run, c *CallRecord, reply string) error {
	if current.Epoch != r.Epoch || current.GoalVersion != r.GoalVersion || c.Epoch != r.Epoch || c.RunID != r.ID || c.InputID != "" || terminal(current.Status) || current.Status == "PAUSED" {
		return context.Canceled
	}
	if current.ActiveTaskID != "" || current.RetiringTaskID != "" {
		return aiFault("AI_EXECUTION_PENDING", "当前或原操作尚未收敛，需先核对真实回执", false)
	}
	if current.PendingCallID != "" && current.PendingCallID != c.ID {
		return context.Canceled
	}
	if current.AccessMode == "WRITE" {
		var head string
		if err := tx.QueryRow(ctx, `SELECT head_revision_id::text FROM occccad.workspaces WHERE document_id=$1 AND name='main' FOR UPDATE`, current.DocumentID).Scan(&head); err != nil {
			return err
		}
		if head != c.Baseline.RevisionID {
			return aiFault("AI_COMPLETION_STALE", "模型已变化，请读取当前版本后结束本次请求", false)
		}
	}
	current.Status = "COMPLETED"
	current.PendingCallID = ""
	current.LatestRevisionID = c.Baseline.RevisionID
	current.Reply = strings.TrimSpace(reply)
	if current.Reply == "" {
		current.Reply = "本次请求已结束。"
	}
	current.WaitingReason = ""
	current.ConsecutiveTransient = 0
	// Retain the actual diagnostic if the agent ends after a failed or partial
	// operation. Text and completion alone never clear a CAD failure.
	c.Status = "APPLIED"
	if err := saveRequestReply(ctx, tx, *current); err != nil {
		return err
	}
	return s.event(ctx, tx, current, "REQUEST_FINISHED", map[string]any{"goalVersion": current.GoalVersion, "revisionId": current.LatestRevisionID, "appliedCount": current.RequestAppliedCount, "reply": current.Reply, "failure": current.Failure})
}

func (s *Service) finishRequest(ctx context.Context, r Run, c CallRecord, reply string) error {
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if err := s.completeRequestTx(ctx, tx, current, r, &c, reply); err != nil {
			return err
		}
		return saveCall(ctx, tx, c)
	})
	if err == nil {
		s.clearSession(r.ID)
	}
	return err
}

func (s *Service) clearSession(id string) {
	s.runMu.Lock()
	delete(s.sessions, id)
	s.runMu.Unlock()
}
