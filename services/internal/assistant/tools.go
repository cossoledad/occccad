// Package assistant adapts CAD automation into a small tool surface and bounded
// context packets. It has no model SDK, credentials, or network model calls.
package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/occccad/occccad/internal/automation"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}
type Call struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
type Result struct {
	CallID  string          `json:"callId"`
	Content json.RawMessage `json:"content"`
	IsError bool            `json:"isError"`
}

// Host binds principal and target outside model-controlled arguments. A future
// provider adapter translates Tool/Call/Result, retaining this application gate.
type Host struct {
	service         *automation.Service
	actor, document string
	writable        bool
}

func NewHost(service *automation.Service, actor, document string, writable bool) *Host {
	return &Host{service, actor, document, writable}
}
func object(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func stringSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 160}
}
func Tools() []Tool {
	return []Tool{
		{"cad_context", "Read the authorized current document definition context; no full tree or geometry.", object(map[string]any{})},
		{"cad_search_operations", "Discover relevant CAD operations; full schemas are loaded only by describe.", object(map[string]any{"query": map[string]any{"type": "string", "maxLength": 256}})},
		{"cad_describe_operation", "Read one exact versioned operation input/output contract.", object(map[string]any{"operationId": stringSchema()}, "operationId")},
		{"cad_query_model", "Read a bounded immutable definition page. Preserve the cursor query and snapshot; samples never imply all matches.", object(map[string]any{"revisionId": stringSchema(), "kind": stringSchema(), "bodyId": stringSchema(), "id": stringSchema(), "name": map[string]any{"type": "string", "maxLength": 256}, "fields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 16}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}, "cursor": map[string]any{"type": "string", "maxLength": 2048}})},
		{"cad_create_task", "Persist a checked Part plan. Each step uses a discovered operation, exact inputs, stable UUID and optional explicit preceding-output bindings.", object(map[string]any{"requestId": stringSchema(), "goal": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048}, "expectedHead": object(map[string]any{"workspaceId": stringSchema(), "revisionId": stringSchema(), "sequence": map[string]any{"type": "integer", "minimum": 1}}, "workspaceId", "revisionId", "sequence"), "steps": map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": object(map[string]any{"id": stringSchema(), "operationId": stringSchema(), "version": map[string]any{"type": "integer", "minimum": 1}, "arguments": map[string]any{"type": "object"}, "bindings": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}}, "id", "operationId", "version", "arguments")}}, "requestId", "goal", "expectedHead", "steps")},
		{"cad_get_task", "Recover persisted step phases and exact commit receipts before retrying any unknown result.", object(map[string]any{"taskId": stringSchema()}, "taskId")},
		{"cad_advance_task", "Advance one bounded phase: prepare a preview or commit the qualified current preview. Display acknowledgment is owned by the application UI.", object(map[string]any{"taskId": stringSchema(), "expectedVersion": map[string]any{"type": "integer", "minimum": 1}}, "taskId", "expectedVersion")},
	}
}
func decode(raw json.RawMessage, value any) error {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return errors.New("invalid tool argument size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing tool arguments")
	}
	return nil
}
func (h *Host) Call(ctx context.Context, call Call) Result {
	var value any
	var err error
	switch call.Name {
	case "cad_context":
		var input struct{}
		if err = decode(call.Arguments, &input); err == nil {
			value, err = h.service.Context(ctx, h.actor, h.document)
		}
	case "cad_search_operations":
		var input struct {
			Query string `json:"query"`
		}
		if err = decode(call.Arguments, &input); err == nil {
			var current automation.Context
			current, err = h.service.Context(ctx, h.actor, h.document)
			if err == nil {
				value, err = h.service.Catalog.Search(input.Query, current.Snapshot.DocumentType, 10)
			}
		}
	case "cad_describe_operation":
		var input struct {
			OperationID string `json:"operationId"`
		}
		if err = decode(call.Arguments, &input); err == nil {
			value, err = h.service.Catalog.Describe(input.OperationID)
		}
	case "cad_query_model":
		var input automation.Query
		if err = decode(call.Arguments, &input); err == nil {
			value, err = h.service.Query(ctx, h.actor, h.document, input)
		}
	case "cad_create_task":
		if !h.writable {
			err = errors.New("host scope is read-only")
			break
		}
		var input automation.CreateTask
		if err = decode(call.Arguments, &input); err == nil {
			value, err = h.service.CreateTask(ctx, h.actor, h.document, input)
		}
	case "cad_get_task", "cad_advance_task":
		var input struct {
			TaskID          string `json:"taskId"`
			ExpectedVersion uint64 `json:"expectedVersion,omitempty"`
		}
		if err = decode(call.Arguments, &input); err == nil {
			if call.Name == "cad_get_task" {
				var packet ContextPacket
				packet, err = h.Context(ctx, input.TaskID, nil, 64<<10)
				value = packet.Checkpoint
			} else if h.writable {
				value, err = h.service.Advance(ctx, h.actor, h.document, input.TaskID, input.ExpectedVersion)
			} else {
				err = errors.New("host scope is read-only")
			}
		}
	default:
		err = errors.New("unsupported CAD tool")
	}
	result := Result{CallID: call.ID}
	if err != nil {
		result.IsError = true
		var f *automation.Fault
		if errors.As(err, &f) {
			value = f
		} else {
			value = map[string]any{"code": "TOOL_CALL_FAILED", "message": err.Error()}
		}
	}
	result.Content, _ = json.Marshal(value)
	if len(result.Content) > 64<<10 {
		result.IsError = true
		result.Content = json.RawMessage(`{"code":"TOOL_OUTPUT_BUDGET_EXCEEDED","message":"Narrow the query or read a task checkpoint instead of full history."}`)
	}
	return result
}
