package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

type Workspace interface {
	ReadModelSnapshot(context.Context, string, string) (workspace.ModelSnapshot, error)
	PreviewCommand(context.Context, string, workspace.CommandRequest) (workspace.CommandPreview, error)
	ExecuteCommand(context.Context, string, workspace.CommandRequest) error
	FindCommandReceipt(context.Context, string, string, string) (workspace.CommandReceipt, error)
	DiscardPreview(string, string, string)
}
type Authorizer interface {
	RequireDocument(context.Context, string, string, access.Role) (access.Role, error)
}
type Service struct {
	db        database.DB
	workspace Workspace
	access    Authorizer
	Catalog   *Catalog
	mu        sync.Mutex
	active    map[string]int
}

func New(db database.DB, ws Workspace, acl Authorizer) *Service {
	return &Service{db: db, workspace: ws, access: acl, Catalog: NewCatalog(), active: map[string]int{}}
}
func (s *Service) authorize(ctx context.Context, actor, document string, role access.Role) error {
	if _, err := uuid.Parse(actor); err != nil {
		return access.ErrUnauthorized
	}
	if _, err := uuid.Parse(document); err != nil {
		return access.ErrNotFound
	}
	_, err := s.access.RequireDocument(ctx, document, actor, role)
	return err
}

type Context struct {
	Snapshot      workspace.ModelSnapshot `json:"snapshot"`
	Permission    access.Role             `json:"permission"`
	Units         string                  `json:"units"`
	WorkingBodyID string                  `json:"workingBodyId,omitempty"`
	Counts        map[string]int          `json:"counts"`
	CatalogDigest string                  `json:"catalogDigest"`
	Scope         string                  `json:"scope"`
}

func (s *Service) Context(ctx context.Context, actor, document string) (Context, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return Context{}, err
	}
	snapshot, err := s.workspace.ReadModelSnapshot(ctx, document, "")
	if err != nil {
		return Context{}, err
	}
	role, err := s.access.RequireDocument(ctx, document, actor, access.RoleViewer)
	if err != nil {
		return Context{}, err
	}
	result := Context{Snapshot: snapshot, Permission: role, Counts: map[string]int{}, CatalogDigest: s.Catalog.Digest, Scope: "DOCUMENT_DEFINITION"}
	if snapshot.DocumentType == "PART" {
		var part workspace.PartModel
		if err = json.Unmarshal(snapshot.Model, &part); err != nil {
			return result, err
		}
		result.Units = part.Units
		result.WorkingBodyID = part.ActiveBodyID
		result.Counts["bodies"] = len(part.Bodies)
		result.Counts["features"] = len(part.Features)
		result.Counts["parameters"] = len(part.Parameters)
	} else {
		var product workspace.ProductModel
		if err = json.Unmarshal(snapshot.Model, &product); err != nil {
			return result, err
		}
		result.Counts["instances"] = len(product.Instances)
		result.Counts["constraints"] = len(product.Constraints)
	}
	return result, nil
}

type ActionRequest struct {
	OperationID  string                    `json:"operationId"`
	Version      int                       `json:"version"`
	Arguments    json.RawMessage           `json:"arguments"`
	ExpectedHead workspace.CommandBaseline `json:"expectedHead"`
	RequestID    string                    `json:"requestId"`
	PreviewID    string                    `json:"previewId,omitempty"`
}
type ActionResult struct {
	Receipt workspace.CommandReceipt `json:"receipt"`
	Outputs []EntityRef              `json:"outputs,omitempty"`
	Output  *EntityRef               `json:"output,omitempty"`
}

func (s *Service) command(ctx context.Context, actor, document string, a ActionRequest) (workspace.CommandRequest, error) {
	if len(a.RequestID) < 1 || len(a.RequestID) > 160 || a.ExpectedHead.WorkspaceID == "" || a.ExpectedHead.RevisionID == "" || a.ExpectedHead.Sequence == 0 {
		return workspace.CommandRequest{}, fault("INVALID_ACTION", "requestId and a complete expectedHead are required")
	}
	command, err := s.Catalog.Compile(a.OperationID, a.Version, a.Arguments)
	if err != nil {
		return command, err
	}
	if err = s.resolveSelections(ctx, document, a, &command); err != nil {
		return command, err
	}
	switch command.Type {
	case "SET_PROPERTIES", "ACTIVATE_MECHANISM", "EXIT_MECHANISM", "SAVE_KINEMATICS", "APPLY_MOTION_FRAME", "CREATE_SKETCH", "CREATE_PUBLICATION", "REDIRECT_PUBLICATION":
		if command.VersionID != "" && command.VersionID != a.ExpectedHead.RevisionID {
			return command, fault("WORKSPACE_HEAD_CONFLICT", "command source revision differs from task baseline")
		}
		command.VersionID = a.ExpectedHead.RevisionID
	}
	if err = workspace.RequireCommandReferences(ctx, s.access, s.workspace, actor, document, command); err != nil {
		return command, err
	}
	command.ActorID = actor
	command.RequestID = a.RequestID
	command.ExpectedHead = &a.ExpectedHead
	command.PreviewID = a.PreviewID
	return command, nil
}
func (s *Service) acquire(actor string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, n := range s.active {
		total += n
	}
	if s.active[actor] >= 2 || total >= 8 {
		return nil, &Fault{Code: "AUTOMATION_BUSY", Message: "automation evaluation capacity reached", Retryable: true}
	}
	s.active[actor]++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.active[actor]--
		if s.active[actor] == 0 {
			delete(s.active, actor)
		}
	}, nil
}
func (s *Service) Prepare(ctx context.Context, actor, document string, a ActionRequest) (workspace.CommandPreview, error) {
	if err := s.authorize(ctx, actor, document, access.RoleEditor); err != nil {
		return workspace.CommandPreview{}, err
	}
	a.PreviewID = ""
	command, err := s.command(ctx, actor, document, a)
	if err != nil {
		return workspace.CommandPreview{}, err
	}
	release, err := s.acquire(actor)
	if err != nil {
		return workspace.CommandPreview{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	preview, err := s.workspace.PreviewCommand(ctx, document, command)
	if err != nil {
		return preview, err
	}
	if preview.EvaluationOutcome == "DEFINITION_ONLY" {
		s.workspace.DiscardPreview(document, actor, preview.PreviewID)
		failure := &Fault{Code: "PREVIEW_DEFINITION_ONLY", Message: "约束定义尚未通过权威求解，请打开装配工具检查诊断后继续"}
		if preview.EvaluationFailure != nil {
			failure.Code = preview.EvaluationFailure.Code
			failure.Phase = preview.EvaluationFailure.Phase
			failure.DiagnosticID = preview.EvaluationFailure.DiagnosticID
			failure.Retryable = preview.EvaluationFailure.Retryable
		}
		return workspace.CommandPreview{}, failure
	}
	if preview.PreviewID == "" || preview.BaseVersionID != a.ExpectedHead.RevisionID || preview.BaseSequence != a.ExpectedHead.Sequence {
		s.workspace.DiscardPreview(document, actor, preview.PreviewID)
		return workspace.CommandPreview{}, fault("PREVIEW_UNQUALIFIED", "preview has no committable candidate for the expected head")
	}
	return preview, nil
}
func (s *Service) Commit(ctx context.Context, actor, document string, a ActionRequest) (ActionResult, error) {
	if err := s.authorize(ctx, actor, document, access.RoleEditor); err != nil {
		return ActionResult{}, err
	}
	if a.PreviewID == "" {
		return ActionResult{}, fault("PREVIEW_REQUIRED", "prepare the exact action before committing")
	}
	command, err := s.command(ctx, actor, document, a)
	if err != nil {
		return ActionResult{}, err
	}
	release, err := s.acquire(actor)
	if err != nil {
		return ActionResult{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	if err = s.workspace.ExecuteCommand(ctx, document, command); err != nil {
		return ActionResult{}, err
	}
	return s.recoverAction(ctx, actor, document, a)
}
func (s *Service) recoverAction(ctx context.Context, actor, document string, a ActionRequest) (ActionResult, error) {
	receipt, err := s.workspace.FindCommandReceipt(ctx, document, actor, a.RequestID)
	if err != nil {
		return ActionResult{}, err
	}
	result := ActionResult{Receipt: receipt}
	args, err := DecodeObject(a.Arguments)
	if err != nil {
		return result, err
	}
	op, _ := s.Catalog.Describe(a.OperationID)
	kind, slot := "", ""
	command := op.CommandType
	if a.OperationID == "sketch.create" {
		command = "CREATE_SKETCH"
	}
	if a.OperationID == "part.extrude" {
		command = "PAD_SKETCH"
	}
	switch command {
	case "CREATE_SKETCH":
		kind, slot = "SKETCH", "entity"
	case "PAD_SKETCH", "CREATE_MODIFY_FEATURE", "CREATE_SOLID_FEATURE", "CREATE_PATTERN", "CREATE_BOOLEAN_FEATURE", "IMPORT_EXCHANGE":
		kind, slot = "FEATURE", "entity"
	case "CREATE_BODY":
		kind, slot = "BODY", "body.entity"
	case "CREATE_PARAMETER":
		kind, slot = "PARAMETER", "parameter.entity"
	case "CREATE_DATUM_PLANE":
		kind, slot = "DATUM_PLANE", "datum.plane"
	case "CREATE_DATUM_AXIS":
		kind, slot = "DATUM_AXIS", "datum.axis"
	case "CREATE_PUBLICATION":
		kind, slot = "PUBLICATION", "publication.entity"
	case "CREATE_PRODUCT_PUBLICATION":
		kind, slot = "PUBLICATION", "product-publication.entity"
	case "CREATE_CONTEXT_INPUT":
		kind, slot = "CONTEXT_INPUT", "context-input.entity"
	case "CREATE_CONTEXT_REFERENCE":
		kind, slot = "CONTEXT_REFERENCE", "context-reference.entity"
	case "CREATE_CONTEXT_BINDING":
		kind, slot = "CONTEXT_BINDING", "context-binding.entity"
	case "INSERT_INSTANCE", "INSERT_INSTANCES":
		kind, slot = "INSTANCE", "entity"
	case "ADD_ASSEMBLY_CONSTRAINT":
		kind, slot = "ASSEMBLY_CONSTRAINT", "assembly-constraint.entity"
	}
	if slot != "" {
		for _, change := range receipt.Changes {
			if change.Kind == "CREATE" && change.SlotID == slot {
				result.Outputs = append(result.Outputs, EntityRef{DocumentID: document, RevisionID: receipt.RevisionID, Kind: kind, ID: change.EntityID})
			}
		}
		if len(result.Outputs) == 1 {
			result.Output = &result.Outputs[0]
		}
	} else {
		key := ""
		switch a.OperationID {
		case "sketch.rectangle":
			key = "sketchId"
			kind = "SKETCH"
		case "parameter.set_literal", "parameter.set_expression":
			key = "parameterId"
			kind = "PARAMETER"
		}
		if key == "" {
			for _, field := range []string{"sketchId", "parameterId", "bodyId", "instanceId", "publicationId"} {
				if textArg(args, field) != "" {
					key = field
					kind = bindingKind(field)
					break
				}
			}
		}
		if id := textArg(args, key); id != "" {
			result.Output = &EntityRef{DocumentID: document, RevisionID: receipt.RevisionID, Kind: kind, ID: id}
		}
	}
	return result, nil
}

func classify(err error) *Fault {
	var f *Fault
	if errors.As(err, &f) {
		return f
	}
	code := "EXECUTION_FAILURE"
	retryable := false
	var domainFailure interface {
		Code() string
		Retryable() bool
	}
	switch {
	case errors.Is(err, context.Canceled):
		code = "CANCELED"
	case errors.Is(err, context.DeadlineExceeded):
		code = "DEADLINE_EXCEEDED"
	case errors.Is(err, database.ErrBusy):
		code, retryable = "DATABASE_BUSY", true
	case errors.As(err, &domainFailure):
		code, retryable = domainFailure.Code(), domainFailure.Retryable()
	case strings.Contains(err.Error(), "HEAD_CONFLICT"):
		code = "WORKSPACE_HEAD_CONFLICT"
	case strings.Contains(err.Error(), "PREVIEW_CANDIDATE"):
		code = "PREVIEW_EXPIRED_OR_MISMATCHED"
	case errors.Is(err, workspace.ErrValidation):
		code = "VALIDATION_FAILED"
	case errors.Is(err, workspace.ErrNotFound):
		code = "NOT_FOUND"
	case errors.Is(err, access.ErrForbidden):
		code = "FORBIDDEN"
	}
	message := err.Error()
	if len(message) > 2048 {
		message = message[:2048]
	}
	result := &Fault{Code: code, Message: message, Retryable: retryable}
	var phased interface{ Phase() string }
	if errors.As(err, &phased) {
		result.Phase = phased.Phase()
	}
	var diagnostic interface{ DiagnosticID() string }
	if errors.As(err, &diagnostic) {
		result.DiagnosticID = diagnostic.DiagnosticID()
	}
	return result
}
func marshal(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("automation serialization: %v", err))
	}
	return raw
}
