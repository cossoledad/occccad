package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

// applyProductDesignCompensatingHistory keeps a ProductDesignTransaction as one
// user-visible history step. Each member still has an immutable per-workspace
// transaction, but all compensation candidates are prepared before the shared
// CAS/commit boundary.
func (service *Service) applyProductDesignCompensatingHistory(ctx context.Context, originalGroupID string, request CommandRequest, requestDigest string) error {
	var rootProductDocumentID string
	if err := service.database.QueryRow(ctx, `SELECT root_product_document_id::text FROM occccad.product_design_transactions WHERE id=$1 AND status='COMMITTED'`, originalGroupID).Scan(&rootProductDocumentID); err != nil {
		return err
	}
	var storedDigest, storedStatus string
	err := service.database.QueryRow(ctx, `SELECT request_digest,status FROM occccad.product_design_transactions WHERE root_product_document_id=$1 AND request_id=$2`, rootProductDocumentID, request.RequestID).Scan(&storedDigest, &storedStatus)
	if err == nil {
		if storedDigest != requestDigest {
			return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		if storedStatus == "COMMITTED" {
			return nil
		}
		return fmt.Errorf("%w: ProductDesignTransaction is %s", ErrValidation, storedStatus)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	rows, err := service.database.Query(ctx, `
		SELECT original.id::text,w.id::text,w.document_id::text,d.document_type,
		       w.head_revision_id::text,w.head_sequence,current.model_json,current.geometry_key,
		       base.model_json,result.model_json,cs.canonical_blob,cs.write_set
		FROM occccad.domain_transactions original
		JOIN occccad.workspaces w ON w.id=original.workspace_id
		JOIN occccad.documents d ON d.id=w.document_id
		JOIN occccad.document_versions current ON current.id=w.head_revision_id
		JOIN occccad.document_versions base ON base.id=original.base_revision_id
		JOIN occccad.document_versions result ON result.id=original.result_revision_id
		JOIN occccad.change_sets cs ON cs.transaction_id=original.id
		WHERE original.product_design_transaction_id=$1 AND original.kind='DOMAIN' AND original.status='COMMITTED'`, originalGroupID)
	if err != nil {
		return err
	}
	defer rows.Close()

	candidates := []atomicDomainCandidate{}
	for rows.Next() {
		var originalTransaction, workspaceID, documentID, documentType, headRevision string
		var headSequence uint64
		var currentJSON, baseJSON, resultJSON, changeJSON []byte
		var geometryKey *string
		var persistedWrites []string
		if err := rows.Scan(&originalTransaction, &workspaceID, &documentID, &documentType, &headRevision, &headSequence,
			&currentJSON, &geometryKey, &baseJSON, &resultJSON, &changeJSON, &persistedWrites); err != nil {
			return err
		}
		consumedRevert := ""
		if request.Type == "UNDO" {
			var latest string
			if err := service.database.QueryRow(ctx, `SELECT coalesce((SELECT action.kind FROM occccad.domain_transactions action WHERE action.root_transaction_id=$1 AND action.status='COMMITTED' ORDER BY action.sequence DESC LIMIT 1),'')`, originalTransaction).Scan(&latest); err != nil {
				return err
			}
			if latest != "" && latest != "REAPPLY" {
				return fmt.Errorf("%w: ProductDesignTransaction is not uniformly undoable", ErrValidation)
			}
		} else {
			if err := service.database.QueryRow(ctx, `
				WITH boundary AS (
				  SELECT coalesce(max(sequence),0) AS sequence FROM occccad.domain_transactions
				  WHERE workspace_id=$2 AND actor_id=$3 AND status='COMMITTED' AND kind IN ('DOMAIN','RESTORE','CREATE')
				)
				SELECT revert_tx.id::text FROM occccad.domain_transactions revert_tx CROSS JOIN boundary
				WHERE revert_tx.root_transaction_id=$1 AND revert_tx.kind='REVERT' AND revert_tx.status='COMMITTED'
				  AND revert_tx.sequence>boundary.sequence
				  AND NOT EXISTS (SELECT 1 FROM occccad.domain_transactions reapply WHERE reapply.reapplies_transaction_id=revert_tx.id AND reapply.status='COMMITTED')
				ORDER BY revert_tx.sequence DESC LIMIT 1`, originalTransaction, workspaceID, actorID(request.ActorID)).Scan(&consumedRevert); errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: ProductDesignTransaction is not uniformly redoable", ErrValidation)
			} else if err != nil {
				return err
			}
		}
		var original modelcore.ChangeSet
		if err := json.Unmarshal(changeJSON, &original); err != nil {
			return err
		}
		if err := validatePersistedChangeSetStructure(original, persistedWrites); err != nil {
			return fmt.Errorf("%w: invalid persisted ProductDesignTransaction ChangeSet: %v", ErrValidation, err)
		}
		original, err = reconcilePersistedChanges(documentType, baseJSON, resultJSON, original)
		if err != nil {
			return err
		}
		current, err := modelValues(documentType, currentJSON, original)
		if err != nil {
			return err
		}
		var desired map[modelcore.PropertyAddress]json.RawMessage
		if request.Type == "UNDO" {
			desired, err = original.Compensate(current)
		} else {
			desired, err = original.Reapply(current)
		}
		if err != nil {
			return fmt.Errorf("%w: ProductDesignTransaction compensation conflict: %v", ErrValidation, err)
		}
		nextJSON, err := applyModelValues(documentType, currentJSON, desired)
		if err != nil {
			return err
		}
		reverse, err := changesBetweenValues(current, desired, original.ImpactSeeds)
		if err != nil {
			return err
		}
		revisionUUID, _ := uuid.NewV7()
		revisionID := revisionUUID.String()
		modelHash := canonicalModelHash(nextJSON)
		var graph *modelcore.DependencyGraph
		var manifest modelcore.EvaluationManifest
		if documentType == "PART" {
			var model PartModel
			if err := json.Unmarshal(nextJSON, &model); err != nil {
				return err
			}
			normalizePartModel(&model)
			if err := validateAndResolvePartParameters(&model); err != nil {
				return err
			}
			nextJSON, _ = json.Marshal(model)
			modelHash = canonicalModelHash(nextJSON)
			graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, reverse.ImpactSeeds, nil)
		} else {
			var model ProductModel
			if err := json.Unmarshal(nextJSON, &model); err != nil {
				return err
			}
			graph, manifest, err = buildProductEvaluation(model, revisionID, modelHash, reverse.ImpactSeeds, nil)
		}
		if err != nil {
			return err
		}
		kind, typeURI := "REVERT", "occccad://history/product-design-revert"
		if request.Type == "REDO" {
			kind, typeURI = "REAPPLY", "occccad://history/product-design-reapply"
		}
		payload, _ := json.Marshal(map[string]string{"rootTransactionId": originalTransaction, "originalProductDesignTransactionId": originalGroupID})
		candidateRequest := CommandRequest{RequestID: request.RequestID + "/" + documentID, Type: request.Type, ActorID: request.ActorID}
		candidate := atomicDomainCandidate{documentID: documentID, workspaceID: workspaceID, headRevision: headRevision,
			headSequence: headSequence, documentType: documentType, revisionID: revisionID, request: candidateRequest,
			command:  modelcore.DomainCommand{CommandID: newID("command"), TypeURI: typeURI, SchemaVersion: 1, Payload: payload},
			nextJSON: nextJSON, modelHash: modelHash, graph: graph, manifest: manifest, changes: reverse,
			kind: kind, rootTransaction: originalTransaction, consumedRevert: consumedRevert}
		if geometryKey != nil {
			candidate.geometryKey = *geometryKey
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(candidates) < 2 {
		return fmt.Errorf("%w: ProductDesignTransaction has an incomplete member set", ErrValidation)
	}
	groupUUID, _ := uuid.NewV7()
	return service.commitProductDesignCandidates(ctx, groupUUID.String(), request.RequestID, requestDigest,
		actorID(request.ActorID), rootProductDocumentID, candidates)
}
