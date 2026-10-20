package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/occccad/occccad/internal/database"
)

// CommandBaseline binds a caller's observations to the existing prepare/CAS
// pipeline. It is optional for existing callers; automation requires it.
type CommandBaseline struct {
	WorkspaceID string `json:"workspaceId"`
	RevisionID  string `json:"revisionId"`
	Sequence    uint64 `json:"sequence"`
}

type ModelSnapshot struct {
	CommandBaseline
	DocumentID   string          `json:"documentId"`
	DocumentType string          `json:"documentType"`
	ModelHash    string          `json:"modelHash"`
	Model        json.RawMessage `json:"-"`
}

// ReadModelSnapshot reads an immutable definition without expanding references,
// projecting a tree, evaluating geometry, or opening a document tab. Historical
// reads remain bound to the main workspace transaction that produced them.
// Authorization belongs to the shared application boundary.
func (service *Service) ReadModelSnapshot(ctx context.Context, documentID, revisionID string) (ModelSnapshot, error) {
	const maxModelCharacters = 16 << 20
	var result ModelSnapshot
	err := service.database.QueryRow(ctx, `
		SELECT d.id::text,d.document_type,w.id::text,v.id::text,
		COALESCE(t.sequence,0),v.model_hash,
		CASE WHEN length(CAST(v.model_json AS text))<=$3 THEN v.model_json ELSE NULL END
		FROM occccad.documents d JOIN occccad.workspaces w ON w.document_id=d.id AND w.name='main'
		JOIN occccad.document_versions v ON v.document_id=d.id AND v.id=COALESCE(NULLIF($2,'')::uuid,w.head_revision_id)
		LEFT JOIN occccad.domain_transactions t ON t.workspace_id=w.id AND t.result_revision_id=v.id AND t.status='COMMITTED'
		WHERE d.id=$1 AND d.deleted_at IS NULL`, documentID, revisionID, maxModelCharacters).Scan(
		&result.DocumentID, &result.DocumentType, &result.WorkspaceID, &result.RevisionID, &result.Sequence, &result.ModelHash, &result.Model)
	if errors.Is(err, database.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if len(result.Model) == 0 || len(result.Model) > 64<<20 {
		return ModelSnapshot{}, fmt.Errorf("%w: MODEL_READ_BUDGET_EXCEEDED", ErrValidation)
	}
	return result, nil
}

type CommandReceipt struct {
	CommandBaseline
	TransactionID string             `json:"transactionId"`
	AuditID       string             `json:"auditId,omitempty"`
	NoChange      bool               `json:"noChange,omitempty"`
	RequestID     string             `json:"requestId"`
	Changes       []CommandChangeRef `json:"changes"`
}

type CommandChangeRef struct {
	Kind     string `json:"kind"`
	EntityID string `json:"entityId"`
	SlotID   string `json:"slotId"`
}

// FindCommandReceipt recovers an exact committed request after timeout or
// process restart. Private payloads and geometry do not enter this projection.
func (service *Service) FindCommandReceipt(ctx context.Context, documentID, actor, requestID string) (CommandReceipt, error) {
	result := CommandReceipt{RequestID: requestID, Changes: []CommandChangeRef{}}
	var raw []byte
	err := service.database.QueryRow(ctx, `
		SELECT t.id::text,t.workspace_id::text,t.result_revision_id::text,t.sequence,cs.canonical_blob
		FROM occccad.domain_transactions t JOIN occccad.workspaces w ON w.id=t.workspace_id
		JOIN occccad.change_sets cs ON cs.transaction_id=t.id
		WHERE w.document_id=$1 AND w.name='main' AND t.actor_id=$2 AND t.request_id=$3 AND t.status='COMMITTED'`, documentID, actor, requestID).Scan(
		&result.TransactionID, &result.WorkspaceID, &result.RevisionID, &result.Sequence, &raw)
	if errors.Is(err, database.ErrNoRows) {
		// Mechanism application can be a successful audited no-op. Recover that
		// exact actor-scoped receipt without inventing a transaction or Revision.
		err = service.database.QueryRow(ctx, `SELECT c.id::text,w.id::text,c.payload FROM occccad.commands c JOIN occccad.workspaces w ON w.document_id=c.document_id AND w.name='main' WHERE c.document_id=$1 AND c.request_id=$2 AND c.status='SUCCEEDED' AND c.command_type IN ('ACTIVATE_MECHANISM','EXIT_MECHANISM')`, documentID, requestID).Scan(&result.AuditID, &result.WorkspaceID, &raw)
		if errors.Is(err, database.ErrNoRows) {
			return CommandReceipt{}, ErrNotFound
		}
		if err != nil {
			return CommandReceipt{}, err
		}
		var audit struct {
			Receipt *ApplicationCommandReceipt `json:"applicationReceipt"`
		}
		if json.Unmarshal(raw, &audit) != nil || audit.Receipt == nil || audit.Receipt.ActorID != actor || audit.Receipt.VersionID == "" || audit.Receipt.Sequence == 0 {
			return CommandReceipt{}, ErrNotFound
		}
		result.RevisionID = audit.Receipt.VersionID
		result.Sequence = audit.Receipt.Sequence
		result.NoChange = true
		return result, nil
	}
	if err != nil {
		return result, err
	}
	var changes struct {
		Changes []struct {
			Kind   string `json:"kind"`
			Target struct {
				EntityID string `json:"entityId"`
				SlotID   string `json:"slotId"`
			} `json:"target"`
		} `json:"changes"`
	}
	if err = json.Unmarshal(raw, &changes); err != nil {
		return result, err
	}
	for _, change := range changes.Changes {
		result.Changes = append(result.Changes, CommandChangeRef{change.Kind, change.Target.EntityID, change.Target.SlotID})
	}
	return result, nil
}
