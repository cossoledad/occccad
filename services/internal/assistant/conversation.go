package assistant

import (
	"context"
	"encoding/json"
	"time"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type SelectedReference struct {
	Role             string                         `json:"role,omitempty"`
	SketchReference  *workspace.SketchGeometryRef   `json:"sketchReference,omitempty"`
	EntityType       string                         `json:"entityType,omitempty"`
	OwnerID          string                         `json:"ownerId,omitempty"`
	OwnerDocumentID  string                         `json:"ownerDocumentId,omitempty"`
	Name             string                         `json:"name,omitempty"`
	GeometryType     string                         `json:"geometryType,omitempty"`
	Kind             string                         `json:"kind"`
	ID               string                         `json:"id,omitempty"`
	BodyID           string                         `json:"bodyId,omitempty"`
	FeatureID        string                         `json:"featureId,omitempty"`
	PublicationID    string                         `json:"publicationId,omitempty"`
	DatumID          string                         `json:"datumId,omitempty"`
	AxisID           string                         `json:"axisId,omitempty"`
	Axis             string                         `json:"axis,omitempty"`
	InstancePath     []string                       `json:"instancePath,omitempty"`
	Selection        *modelcore.PersistentSelection `json:"selection,omitempty"`
	SourceDocumentID string                         `json:"sourceDocumentId,omitempty"`
	SourceVersionID  string                         `json:"sourceVersionId,omitempty"`
}
type FrozenScope struct {
	RootDocumentID string              `json:"rootDocumentId"`
	DocumentID     string              `json:"documentId"`
	RevisionID     string              `json:"revisionId"`
	RootRevisionID string              `json:"rootRevisionId,omitempty"`
	WorkingBodyID  string              `json:"workingBodyId,omitempty"`
	EditContextID  string              `json:"editContextId,omitempty"`
	InstancePath   []string            `json:"instancePath,omitempty"`
	Selections     []SelectedReference `json:"selections"`
	SelectionCount int                 `json:"selectionCount"`
	Complete       bool                `json:"complete"`
}
type RunInput struct {
	RequestID     string       `json:"requestId"`
	Text          string       `json:"text"`
	WorkingBodyID string       `json:"workingBodyId,omitempty"`
	Scope         *FrozenScope `json:"scope,omitempty"`
	Mode          string       `json:"mode,omitempty"`
	TokenBudget   *int64       `json:"tokenBudget"`
}
type MessageInput struct {
	RequestID string       `json:"requestId"`
	Text      string       `json:"text"`
	Scope     *FrozenScope `json:"scope,omitempty"`
}
type Turn struct {
	ID          string      `json:"id"`
	RunID       string      `json:"runId"`
	Sequence    uint64      `json:"sequence"`
	RequestID   string      `json:"requestId"`
	Text        string      `json:"text"`
	Kind        string      `json:"kind"`
	Status      string      `json:"status"`
	Reply       string      `json:"reply,omitempty"`
	Scope       FrozenScope `json:"scope"`
	GoalVersion uint64      `json:"goalVersion"`
	Usage       *TurnUsage  `json:"usage,omitempty"`
	CreatedAt   string      `json:"createdAt"`
}
type RunUsage struct {
	PendingCalls     int64 `json:"pendingCalls"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	ReportedCalls    int64 `json:"reportedCalls"`
	UnknownAttempts  int64 `json:"unknownAttempts"`
	CacheHitTokens   int64 `json:"cacheHitTokens"`
	ReasoningTokens  int64 `json:"reasoningTokens"`
}
type TurnUsage struct {
	RunUsage
	Calls int64 `json:"calls"`
}

type Run struct {
	CurrentTurnID           string            `json:"currentTurnId,omitempty"`
	ID                      string            `json:"id"`
	DocumentID              string            `json:"documentId"`
	Version                 uint64            `json:"version"`
	Epoch                   uint64            `json:"epoch"`
	GoalVersion             uint64            `json:"goalVersion"`
	Goal                    string            `json:"goal"`
	Status                  string            `json:"status"`
	Mode                    string            `json:"mode"`
	Scope                   FrozenScope       `json:"scope"`
	ActiveTaskID            string            `json:"activeTaskId,omitempty"`
	StageCount              uint64            `json:"stageCount"`
	StepCount               uint64            `json:"stepCount"`
	MessageSequence         uint64            `json:"messageSequence"`
	EventSequence           uint64            `json:"eventSequence"`
	ModelCalls              int64             `json:"modelCalls"`
	ToolCalls               int64             `json:"toolCalls"`
	Usage                   RunUsage          `json:"usage"`
	TokenBudget             *int64            `json:"tokenBudget"`
	BudgetVersion           uint64            `json:"budgetVersion"`
	LatestRevisionID        string            `json:"latestRevisionId,omitempty"`
	LatestAppliedRevisionID string            `json:"latestAppliedRevisionId,omitempty"`
	AppliedCount            uint64            `json:"appliedCount"`
	RequestAppliedCount     uint64            `json:"requestAppliedCount"`
	Targets                 map[string]string `json:"targets,omitempty"`
	TargetSelections        map[string]string `json:"targetSelections,omitempty"`
	PresentedRevisionID     string            `json:"presentedRevisionId,omitempty"`
	Reply                   string            `json:"reply,omitempty"`
	Failure                 *automation.Fault `json:"failure,omitempty"`
	WaitingReason           string            `json:"waitingReason,omitempty"`
	CreatedAt               string            `json:"createdAt"`
	UpdatedAt               string            `json:"updatedAt"`
	CredentialScope         string            `json:"credentialScope"`
	ActorID                 string            `json:"-"`
	AccessMode              string            `json:"-"`
	PendingCallID           string            `json:"pendingCallId,omitempty"`
	RetiringTaskID          string            `json:"retiringTaskId,omitempty"`
	LastFailureDigest       string            `json:"lastFailureDigest,omitempty"`
	LastStageFailureDigest  string            `json:"lastStageFailureDigest,omitempty"`
	ConsecutiveTransient    uint64            `json:"consecutiveTransient,omitempty"`
}
type RunEvent struct {
	TurnID   string          `json:"turnId,omitempty"`
	Sequence uint64          `json:"sequence"`
	Epoch    uint64          `json:"epoch"`
	Type     string          `json:"type"`
	At       string          `json:"at"`
	Data     json.RawMessage `json:"data,omitempty"`
}
type Stage struct {
	Sequence    uint64 `json:"sequence"`
	Epoch       uint64 `json:"epoch"`
	TaskID      string `json:"taskId"`
	Status      string `json:"status"`
	GoalVersion uint64 `json:"goalVersion"`
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextAfter  uint64 `json:"nextAfter"`
	HasMore    bool   `json:"hasMore"`
	HasEarlier bool   `json:"hasEarlier"`
}
type CallRecord struct {
	ID                   string                    `json:"id"`
	RunID                string                    `json:"runId"`
	TurnID               string                    `json:"turnId,omitempty"`
	InputID              string                    `json:"inputId,omitempty"`
	LogicalID            string                    `json:"logicalId"`
	Attempt              uint64                    `json:"attempt"`
	Epoch                uint64                    `json:"epoch"`
	Status               string                    `json:"status"`
	Provider             string                    `json:"provider"`
	Generation           Generation                `json:"generation"`
	Baseline             workspace.CommandBaseline `json:"baseline"`
	RequestDigest        string                    `json:"requestDigest"`
	ProviderRequestID    string                    `json:"providerRequestId,omitempty"`
	Usage                *Usage                    `json:"usage,omitempty"`
	UsageState           string                    `json:"usageState"`
	Response             *Message                  `json:"response,omitempty"`
	Failure              *automation.Fault         `json:"failure,omitempty"`
	StartedAt            string                    `json:"startedAt"`
	FinishedAt           string                    `json:"finishedAt,omitempty"`
	ToolCursor           int                       `json:"toolCursor"`
	Sequence             uint64                    `json:"sequence,omitempty"`
	RequestEventSequence uint64                    `json:"requestEventSequence,omitempty"`
	Runtime              *RuntimeStamp             `json:"runtime,omitempty"`
}

func marshalJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}
func (s *Service) now() time.Time {
	if s.options.Now != nil {
		return s.options.Now()
	}
	return time.Now()
}

// Fixed fractional width makes durable time+ID keyset cursors sort correctly.
func timestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }
func terminal(status string) bool  { return status == "COMPLETED" || status == "STOPPED" }
