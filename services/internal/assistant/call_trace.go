package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"runtime/debug"
	"sync"

	"github.com/google/uuid"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

// PublicMessage deliberately has no private reasoning or transport credentials.
// Its other fields are the actual messages used for this particular call.
type PublicMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type RuntimeStamp struct {
	Engine        string `json:"engine"`
	Revision      string `json:"revision,omitempty"`
	Modified      bool   `json:"modified,omitempty"`
	CatalogDigest string `json:"catalogDigest"`
	PromptDigest  string `json:"promptDigest"`
}

var buildStamp = sync.OnceValue(func() RuntimeStamp {
	stamp := RuntimeStamp{Engine: "assistant-evidence-trace-v1"}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				stamp.Revision = setting.Value
			case "vcs.modified":
				stamp.Modified = setting.Value == "true"
			}
		}
	}
	return stamp
})

func (s *Service) runtimeStamp() RuntimeStamp {
	stamp := buildStamp()
	stamp.CatalogDigest = s.automation.Catalog.Digest
	stamp.PromptDigest = modelcore.ValueDigest([]byte(systemPrompt))
	return stamp
}

func publicTranscript(messages []Message) []PublicMessage {
	out := make([]PublicMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, PublicMessage{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID})
	}
	return out
}

type RequestSnapshot struct {
	CallID     string          `json:"callId"`
	Messages   []PublicMessage `json:"messages"`
	Tools      []Tool          `json:"tools"`
	Generation Generation      `json:"generation"`
	Runtime    RuntimeStamp    `json:"runtime"`
}

// Store one immutable public request in the paged journal, separate from the
// mutable call cursor. Rewriting a tool cursor never copies the entire prompt.
// Normal event pages exclude this large payload; only a requested call loads it.
func (s *Service) captureCallRequest(ctx context.Context, c *CallRecord, messages []Message, tools []Tool) error {
	snapshot := RequestSnapshot{CallID: c.ID, Messages: publicTranscript(messages), Tools: tools, Generation: c.Generation, Runtime: s.runtimeStamp()}
	_, err := s.mutateRun(ctx, c.RunID, func(tx database.Tx, r *Run) error {
		stored, e := loadCall(ctx, tx, c.ID)
		if e != nil {
			return e
		}
		if stored.RequestEventSequence != 0 {
			*c = stored
			return nil
		}
		if stored.Status != "PREPARED" {
			return aiFault("AI_CALL_STATE", "请求记录只能在发送之前创建", false)
		}
		if e = s.event(ctx, tx, r, "MODEL_REQUEST_CAPTURED", snapshot); e != nil {
			return e
		}
		stored.RequestEventSequence = r.EventSequence
		stored.Runtime = &snapshot.Runtime
		if e = saveCall(ctx, tx, stored); e != nil {
			return e
		}
		*c = stored
		return nil
	})
	return err
}

type CallSummary struct {
	ID               string            `json:"id"`
	Sequence         uint64            `json:"sequence,omitempty"`
	Epoch            uint64            `json:"epoch"`
	InputID          string            `json:"inputId,omitempty"`
	Status           string            `json:"status"`
	StartedAt        string            `json:"startedAt"`
	FinishedAt       string            `json:"finishedAt,omitempty"`
	Model            string            `json:"model"`
	Reply            string            `json:"reply,omitempty"`
	ToolNames        []string          `json:"toolNames"`
	RequestAvailable bool              `json:"requestAvailable"`
	Failure          *automation.Fault `json:"failure,omitempty"`
}

type TracePage struct {
	Items      []CallSummary `json:"items"`
	HasEarlier bool          `json:"hasEarlier"`
	NextCursor string        `json:"nextCursor,omitempty"`
	Runtime    RuntimeStamp  `json:"runtime"`
}

// A recent page is chronological within the page, with a keyset cursor for
// earlier calls. Full prompts and tool results are fetched only on expansion.
func (s *Service) Trace(ctx context.Context, actor, document, run, cursor string, limit int) (TracePage, error) {
	page := TracePage{Items: []CallSummary{}, Runtime: s.runtimeStamp()}
	if _, err := s.GetRun(ctx, actor, document, run); err != nil {
		return page, err
	}
	at, key := "", ""
	if cursor != "" {
		raw, err := decodeCursor(cursor)
		if err != nil {
			return page, err
		}
		var c struct{ At, ID string }
		if json.Unmarshal(raw, &c) != nil || c.At == "" || c.ID == "" {
			return page, aiFault("INVALID_CURSOR", "调用记录游标无效", false)
		}
		at, key = c.At, c.ID
	}
	limit = boundedLimit(limit)
	rows, err := s.db.Query(ctx, `SELECT payload,status FROM occccad.assistant_model_calls WHERE run_id=$1 AND ($2='' OR started_at<$2 OR (started_at=$2 AND id::text<$3)) ORDER BY started_at DESC,id DESC LIMIT $4`, run, at, key, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var status string
		var c CallRecord
		if err = rows.Scan(&raw, &status); err != nil {
			return page, err
		}
		if err = json.Unmarshal(raw, &c); err != nil {
			return page, err
		}
		if len(page.Items) == limit {
			page.HasEarlier = true
			break
		}
		item := CallSummary{ID: c.ID, Sequence: c.Sequence, Epoch: c.Epoch, InputID: c.InputID, Status: status, StartedAt: c.StartedAt, FinishedAt: c.FinishedAt, Model: c.Generation.Model, ToolNames: []string{}, RequestAvailable: c.RequestEventSequence != 0, Failure: c.Failure}
		if c.Response != nil {
			item.Reply = publicProgress(c.Response.Content)
			for _, tool := range c.Response.ToolCalls {
				item.ToolNames = append(item.ToolNames, tool.Function.Name)
			}
		}
		page.Items = append(page.Items, item)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > 0 {
		oldest := page.Items[len(page.Items)-1]
		page.NextCursor = encodeCursor(map[string]string{"At": oldest.StartedAt, "ID": oldest.ID})
	}
	for i, j := 0, len(page.Items)-1; i < j; i, j = i+1, j-1 {
		page.Items[i], page.Items[j] = page.Items[j], page.Items[i]
	}
	return page, nil
}

type TraceTool struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Feedback  json.RawMessage `json:"feedback,omitempty"`
	IsError   bool            `json:"isError"`
	Executed  bool            `json:"executed"`
	Execution *ExecutionTrace `json:"execution,omitempty"`
}

type ExecutionTrace struct {
	TaskID string          `json:"taskId"`
	Status string          `json:"status"`
	Steps  []ExecutionStep `json:"steps"`
}
type ExecutionStep struct {
	ID          string            `json:"id"`
	OperationID string            `json:"operationId"`
	Phase       string            `json:"phase"`
	Arguments   json.RawMessage   `json:"arguments"`
	Failure     *automation.Fault `json:"failure,omitempty"`
	RevisionID  string            `json:"revisionId,omitempty"`
}

type CallTrace struct {
	Call    CallRecord       `json:"call"`
	Request *RequestSnapshot `json:"request,omitempty"`
	Tools   []TraceTool      `json:"tools"`
}

func (s *Service) CallTrace(ctx context.Context, actor, document, run, id string) (CallTrace, error) {
	trace := CallTrace{Tools: []TraceTool{}}
	if _, err := s.GetRun(ctx, actor, document, run); err != nil {
		return trace, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return trace, aiFault("AI_CALL_NOT_FOUND", "调用记录不存在", false)
	}
	c, err := loadCall(ctx, s.db, id)
	if errors.Is(err, database.ErrNoRows) || err == nil && c.RunID != run {
		return trace, aiFault("AI_CALL_NOT_FOUND", "调用记录不存在", false)
	}
	if err != nil {
		return trace, err
	}
	if c.Response != nil {
		c.Response.ReasoningContent = ""
	}
	trace.Call = c
	if c.RequestEventSequence != 0 {
		var raw []byte
		if err = s.db.QueryRow(ctx, `SELECT payload FROM occccad.assistant_run_events WHERE run_id=$1 AND sequence=$2`, run, c.RequestEventSequence).Scan(&raw); err != nil {
			return trace, err
		}
		var event RunEvent
		var request RequestSnapshot
		if err = json.Unmarshal(raw, &event); err != nil {
			return trace, err
		}
		if err = json.Unmarshal(event.Data, &request); err != nil {
			return trace, err
		}
		if event.Type != "MODEL_REQUEST_CAPTURED" || request.CallID != c.ID {
			return trace, aiFault("AI_CALL_TRACE_MISMATCH", "请求快照与调用记录不一致", false)
		}
		trace.Request = &request
	}
	results := map[string]Result{}
	rows, err := s.db.Query(ctx, `SELECT tool_id,payload FROM occccad.assistant_tool_results WHERE call_id=$1 ORDER BY ordinal`, c.ID)
	if err != nil {
		return trace, err
	}
	for rows.Next() {
		var id string
		var raw []byte
		var result Result
		if err = rows.Scan(&id, &raw); err != nil {
			break
		}
		if err = json.Unmarshal(raw, &result); err != nil {
			break
		}
		results[id] = result
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return trace, err
	}
	if c.Response != nil {
		for _, tool := range c.Response.ToolCalls {
			arguments := json.RawMessage(tool.Function.Arguments)
			if !json.Valid(arguments) {
				arguments = marshalJSON(tool.Function.Arguments)
			}
			item := TraceTool{ID: tool.ID, Name: tool.Function.Name, Arguments: arguments}
			if result, ok := results[tool.ID]; ok {
				item.Feedback = result.Content
				item.IsError = result.IsError
				item.Executed = true
			}
			if item.Executed && !item.IsError && item.Name == "cad_create_task" {
				var created struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(item.Feedback, &created) == nil && created.ID != "" {
					if task, e := s.automation.GetTask(ctx, actor, document, created.ID); e == nil && task.RunID == run {
						execution := &ExecutionTrace{TaskID: task.ID, Status: task.Status, Steps: []ExecutionStep{}}
						for _, step := range task.Steps {
							arguments := step.Arguments
							if step.Action != nil {
								arguments = step.Action.Arguments
							}
							entry := ExecutionStep{ID: step.ID, OperationID: step.OperationID, Phase: step.Phase, Arguments: arguments, Failure: step.Failure}
							if step.Result != nil {
								entry.RevisionID = step.Result.Receipt.RevisionID
							}
							execution.Steps = append(execution.Steps, entry)
						}
						item.Execution = execution
					}
				}
			}
			trace.Tools = append(trace.Tools, item)
		}
	}
	return trace, nil
}
