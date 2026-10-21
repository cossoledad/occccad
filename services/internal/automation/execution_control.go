package automation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type HandoffExpectation struct {
	ID              string                    `json:"id"`
	RunID           string                    `json:"runId,omitempty"`
	PlanEpoch       uint64                    `json:"planEpoch"`
	UICommand       string                    `json:"uiCommand"`
	Baseline        workspace.CommandBaseline `json:"baseline"`
	DomainRequestID string                    `json:"domainRequestId"`
}

func actionIntent(action ActionRequest) string {
	action.PreviewID = ""
	return modelcore.ValueDigest(marshal(action))
}

func confirmationMatches(task Task, step Step) bool {
	c := step.Confirmation
	return c != nil && step.Action != nil && step.Preview != nil && c.RunID == task.RunID && c.PlanEpoch == task.PlanEpoch && c.StepID == step.ID && c.TaskVersion+1 == task.Version && c.ExpectedHead == step.Action.ExpectedHead && c.PreviewID == step.Action.PreviewID && c.PreviewID == step.Preview.PreviewID && c.IntentDigest == actionIntent(*step.Action)
}

func currentTaskHead(ctx context.Context, db database.Executor, task Task) (workspace.CommandBaseline, error) {
	var head workspace.CommandBaseline
	err := db.QueryRow(ctx, `SELECT id::text,head_revision_id::text,head_sequence FROM occccad.workspaces WHERE id=$1 AND document_id=$2`, task.Baseline.WorkspaceID, task.DocumentID).Scan(&head.WorkspaceID, &head.RevisionID, &head.Sequence)
	return head, err
}

func invalidateCandidate(s *Service, document, actor string, task *Task) {
	step := task.pending()
	if step == nil {
		return
	}
	step.Confirmation = nil
	// An already issued commit is reconciled by its original identity. Revoking
	// an idle candidate cannot cancel a commit that has crossed that boundary.
	if step.Phase == "COMMITTING" {
		return
	}
	if step.Preview != nil {
		s.workspace.DiscardPreview(document, actor, step.Preview.PreviewID)
	}
	step.Preview = nil
	if step.Action != nil {
		step.Action.PreviewID = ""
	}
	if step.Phase == "PREVIEW_READY" {
		step.Phase = "PLANNED"
	}
}

func (s *Service) controlHandoff(task *Task, input Control, validated *ActionResult) error {
	step := task.pending()
	if step == nil || step.ID != input.StepID || step.Handoff == nil || input.HandoffID != step.Handoff.ID {
		return fault("UI_HANDOFF_INVALID", "result must identify the issued handoff and step")
	}
	terminalReconciliation := input.Action == "ui_complete" && (step.Phase == "UI_CANCELLED" || step.Phase == "UI_FAILED")
	if step.Phase != "HANDED_OFF" && step.Phase != "UI_OPENED" && step.Phase != "UI_UNKNOWN" && !terminalReconciliation {
		return fault("UI_HANDOFF_INVALID", "this handoff already has a terminal result")
	}
	switch input.Action {
	case "ui_opened":
		if step.Phase != "HANDED_OFF" {
			return fault("UI_HANDOFF_INVALID", "an uncertain handoff cannot be replayed")
		}
		step.Phase = "UI_OPENED"
	case "ui_unknown":
		step.Phase = "UI_UNKNOWN"
		step.Failure = &Fault{Code: "UI_RESULT_UNKNOWN", Message: "工具结果未知。请核对原操作回执或人工接管；系统不会重放工具。"}
	case "ui_failed", "ui_cancelled":
		step.Phase = "UI_FAILED"
		step.Failure = &Fault{Code: "UI_COMMAND_UNAVAILABLE", Message: "工具未打开，请检查当前选择或从工具栏继续操作"}
		if input.Action == "ui_cancelled" {
			step.Phase = "UI_CANCELLED"
			step.Failure = &Fault{Code: "UI_CANCELLED", Message: "用户取消了交互工具，依赖此操作的步骤尚未执行。"}
		}
		if task.Status != "STOPPED" {
			task.Status = "PAUSED"
		}
	case "ui_complete":
		if validated == nil || validated.Receipt.RequestID != step.Handoff.DomainRequestID {
			return fault("UI_RECEIPT_MISMATCH", "exact handoff receipt evidence is required")
		}
		step.Result = validated
		step.Phase = "UI_COMPLETED"
		task.Baseline = validated.Receipt.CommandBaseline
		if task.Status == "WAIT_UI" {
			task.Status = "ACTIVE"
			if task.pending() == nil {
				task.Status = "COMPLETED"
			}
		}
	}
	task.event(step.ID, step.Phase)
	return nil
}

func handoffDomainType(command string) string {
	return map[string]string{"part.fillet": "CREATE_MODIFY_FEATURE", "part.chamfer": "CREATE_MODIFY_FEATURE", "part.pad": "PAD_SKETCH", "part.extrude": "PAD_SKETCH", "sketch.create": "CREATE_SKETCH", "sketch.start": "CREATE_SKETCH", "assembly.constraint": "ADD_ASSEMBLY_CONSTRAINT"}[command]
}

// Read immutable receipt facts before acquiring the task's short journal lock.
// SQLite intentionally has a single connection, and Workspace reads must never
// recursively acquire that connection from inside an open task transaction.
func (s *Service) validateHandoffReceipt(ctx context.Context, actor, document string, task Task, input Control) (ActionResult, error) {
	step := task.pending()
	if step == nil || step.ID != input.StepID || step.Handoff == nil || step.Handoff.ID != input.HandoffID {
		return ActionResult{}, fault("UI_HANDOFF_INVALID", "completion must identify the issued handoff")
	}
	if input.DomainRequestID == "" || input.DomainRequestID != step.Handoff.DomainRequestID {
		return ActionResult{}, fault("UI_RECEIPT_MISMATCH", "completion requires the handoff's original domain request identity")
	}
	if handoffDomainType(step.Handoff.UICommand) == "" {
		return ActionResult{}, fault("UI_EFFECT_UNVERIFIED", "this tool has no declared domain completion contract; continue with explicit model observation or manual takeover")
	}
	receipt, err := s.workspace.FindCommandReceipt(ctx, document, actor, input.DomainRequestID)
	if err != nil {
		return ActionResult{}, err
	}
	var baseRevision string
	if err = s.db.QueryRow(ctx, `SELECT base_revision_id::text FROM occccad.domain_transactions WHERE id=$1 AND actor_id=$2 AND request_id=$3 AND workspace_id=$4 AND status='COMMITTED'`, receipt.TransactionID, actor, input.DomainRequestID, step.Handoff.Baseline.WorkspaceID).Scan(&baseRevision); err != nil {
		return ActionResult{}, err
	}
	if baseRevision != step.Handoff.Baseline.RevisionID || receipt.Sequence != step.Handoff.Baseline.Sequence+1 || receipt.NoChange {
		return ActionResult{}, fault("UI_RECEIPT_MISMATCH", "receipt does not apply the expected handoff baseline")
	}
	if expected := handoffDomainType(step.Handoff.UICommand); expected != "" {
		var actual string
		if err = s.db.QueryRow(ctx, `SELECT command_type FROM occccad.commands WHERE document_id=$1 AND request_id=$2 AND status='SUCCEEDED'`, document, input.DomainRequestID).Scan(&actual); err != nil {
			return ActionResult{}, err
		}
		if actual != expected {
			return ActionResult{}, fault("UI_RECEIPT_MISMATCH", "unrelated domain command cannot complete this interactive tool")
		}
		if step.Handoff.UICommand == "part.fillet" || step.Handoff.UICommand == "part.chamfer" {
			snapshot, err := s.workspace.ReadModelSnapshot(ctx, document, receipt.RevisionID)
			if err != nil {
				return ActionResult{}, err
			}
			var part workspace.PartModel
			if err = json.Unmarshal(snapshot.Model, &part); err != nil {
				return ActionResult{}, err
			}
			kind := strings.ToUpper(strings.TrimPrefix(step.Handoff.UICommand, "part."))
			matched := false
			for _, change := range receipt.Changes {
				for _, feature := range part.Features {
					if change.Kind == "CREATE" && change.EntityID == feature.ID && feature.Type == kind {
						matched = true
					}
				}
			}
			if !matched {
				return ActionResult{}, fault("UI_RECEIPT_MISMATCH", "receipt does not create the expected modifier")
			}
		}
	}

	return ActionResult{Receipt: receipt}, nil
}
