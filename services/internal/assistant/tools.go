// Package assistant connects a bounded conversational planner to scoped CAD
// tools. Geometry writes remain in the application-owned automation pipeline.
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
	revision        string
}

func NewHost(service *automation.Service, actor, document string, writable bool) *Host {
	return &Host{service: service, actor: actor, document: document, writable: writable}
}
func object(properties map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func stringSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 160}
}
func Tools() []Tool {
	return []Tool{
		{"cad_describe_type", "Load one nested command input type referenced as cad://types/<name>. Progressive disclosure avoids sending all command schemas.", object(map[string]any{"name": stringSchema()}, "name")},
		{"cad_query_topology", "Query exact Naming topology in one Body/revision. selectionSet includes ALL matches even when items is paginated; use it directly for all-edge fillets/chamfers. Optional ids select queried stable topology identities; includeSelections supplies formal references for detailed commands. Never invent IDs.", object(map[string]any{"revisionId": stringSchema(), "bodyId": stringSchema(), "kind": map[string]any{"type": "string", "enum": []string{"EDGE", "FACE", "VERTEX"}}, "ids": map[string]any{"type": "array", "items": stringSchema(), "maxItems": 100}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20}, "cursor": map[string]any{"type": "string", "maxLength": 2048}, "includeSelections": map[string]any{"type": "boolean"}}, "revisionId", "bodyId", "kind")},
		{"cad_context", "Read the authorized current document definition context; no full tree or geometry.", object(map[string]any{})},
		{"cad_search_operations", "Discover relevant CAD operations; full schemas are loaded only by describe.", object(map[string]any{"query": map[string]any{"type": "string", "maxLength": 256}, "cursor": map[string]any{"type": "string", "maxLength": 256}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20}})},
		{"cad_describe_operation", "Read one exact versioned operation input/output contract.", object(map[string]any{"operationId": stringSchema()}, "operationId")},
		{"cad_query_model", "Read a bounded immutable definition page. Preserve cursor/snapshot; pages are not all matches. Kinds: BODY, SKETCH, FEATURE, PARAMETER, DATUM_PLANE, DATUM_AXIS, INSTANCE, PUBLICATION, CONTEXT_INPUT, CONTEXT_REFERENCE, CONTEXT_BINDING, ASSEMBLY_CONSTRAINT, MECHANISM, SKETCH_ENTITY, SKETCH_CONSTRAINT. Query sketch children with ownerId. Field definition loads one exact DTO; prefer small fields for discovery. Fields: name,type,bodyId,ownerId,length,plane,status,parameterAlias,value,displayValue,unit,definitionDigest. Before editing a feature, query its exact id with fields [definition,definitionDigest]; use definitionDigest as expectedFeatureDigest. For parameters value is a canonical SI quantity; displayValue is an unrounded number in unit.", object(map[string]any{"revisionId": stringSchema(), "kind": stringSchema(), "bodyId": stringSchema(), "ownerId": stringSchema(), "id": stringSchema(), "name": map[string]any{"type": "string", "maxLength": 256}, "fields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 16}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}, "cursor": map[string]any{"type": "string", "maxLength": 2048}})},
		{"cad_create_task", "Persist a checked document plan. Each step uses a discovered operation, exact inputs, stable UUID and optional explicit preceding-output bindings.", object(map[string]any{"requestId": stringSchema(), "goal": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048}, "expectedHead": object(map[string]any{"workspaceId": stringSchema(), "revisionId": stringSchema(), "sequence": map[string]any{"type": "integer", "minimum": 1}}, "workspaceId", "revisionId", "sequence"), "steps": map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": object(map[string]any{"id": stringSchema(), "operationId": stringSchema(), "version": map[string]any{"type": "integer", "minimum": 1}, "arguments": map[string]any{"type": "object"}, "bindings": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}}, "id", "operationId", "version", "arguments")}}, "requestId", "goal", "expectedHead", "steps")},
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
	case "cad_query_occurrence":
		var input automation.OccurrenceQuery
		if err = decode(call.Arguments, &input); err == nil {
			if h.revision != "" && input.RevisionID != h.revision {
				err = errors.New("occurrence snapshot is outside context")
			} else {
				value, err = h.service.QueryOccurrence(ctx, h.actor, h.document, input)
			}
		}
	case "cad_mate_candidates":
		var input automation.MateCandidateQuery
		if err = decode(call.Arguments, &input); err == nil {
			if h.revision != "" && input.RevisionID != h.revision {
				err = errors.New("candidate snapshot is outside context")
			} else {
				value, err = h.service.MateCandidates(ctx, h.actor, h.document, input)
			}
		}
	case "cad_verify_assembly":
		var input automation.AssemblyVerificationInput
		if err = decode(call.Arguments, &input); err == nil {
			value, err = h.service.VerifyAssembly(ctx, h.actor, h.document, input)
		}

	case "cad_describe_type":
		var input struct {
			Name string `json:"name"`
		}
		if err = decode(call.Arguments, &input); err == nil {
			value, err = automation.DescribeType(input.Name)
		}
	case "cad_query_topology":
		var input automation.TopologyQuery
		if err = decode(call.Arguments, &input); err == nil {
			if h.revision != "" && input.RevisionID != h.revision {
				err = errors.New("topology is outside the frozen historical context")
			} else {
				value, err = h.service.QueryTopology(ctx, h.actor, h.document, input)
			}
		}
	case "cad_context":
		var input struct{}
		if err = decode(call.Arguments, &input); err == nil {
			value, err = h.service.ContextAt(ctx, h.actor, h.document, h.revision)
		}
	case "cad_search_operations":
		var input struct {
			Query  string `json:"query"`
			Cursor string `json:"cursor"`
			Limit  int    `json:"limit"`
		}
		if err = decode(call.Arguments, &input); err == nil {
			var current automation.Context
			current, err = h.service.Context(ctx, h.actor, h.document)
			if err == nil {
				value, err = h.service.Catalog.SearchPage(input.Query, current.Snapshot.DocumentType, input.Limit, input.Cursor)
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
			if h.revision != "" {
				if input.RevisionID != "" && input.RevisionID != h.revision {
					err = errors.New("query is outside the frozen historical context")
				} else {
					input.RevisionID = h.revision
				}
			}
			if err == nil {
				value, err = h.service.Query(ctx, h.actor, h.document, input)
			}
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
