package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
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
	if !errors.Is(err, database.ErrNoRows) {
		return err
	}

	rows, err := service.database.Query(ctx, `
		SELECT original.id::text,w.id::text,w.document_id::text,d.document_type,
		       w.head_revision_id::text,w.head_sequence,current.model_json,
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

	type historyRow struct {
		originalTransaction, workspaceID, documentID, documentType, headRevision string
		headSequence                                                             uint64
		currentJSON, baseJSON, resultJSON, changeJSON                            []byte
		persistedWrites                                                          []string
	}
	inputs, err := database.CollectRows(rows, func(row database.CollectableRow) (historyRow, error) {
		var v historyRow
		err := row.Scan(&v.originalTransaction, &v.workspaceID, &v.documentID, &v.documentType, &v.headRevision, &v.headSequence, &v.currentJSON, &v.baseJSON, &v.resultJSON, &v.changeJSON, &v.persistedWrites)
		return v, err
	})
	if err != nil {
		return err
	}
	// Allocate all immutable outcomes first, so a compensated Product can point
	// directly at the Part/Product outcomes from this same atomic action.
	revisions := map[string]string{}
	for _, v := range inputs {
		id, _ := uuid.NewV7()
		revisions[v.documentID] = id.String()
	}
	candidates := []atomicDomainCandidate{}
	for _, v := range inputs {
		originalTransaction, workspaceID, documentID, documentType, headRevision := v.originalTransaction, v.workspaceID, v.documentID, v.documentType, v.headRevision
		headSequence, currentJSON, baseJSON, resultJSON, changeJSON, persistedWrites := v.headSequence, v.currentJSON, v.baseJSON, v.resultJSON, v.changeJSON, v.persistedWrites

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
				ORDER BY revert_tx.sequence DESC LIMIT 1`, originalTransaction, workspaceID, actorID(request.ActorID)).Scan(&consumedRevert); errors.Is(err, database.ErrNoRows) {
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
		var outcome []byte
		if request.Type == "REDO" {
			err = service.database.QueryRow(ctx, `SELECT v.model_json FROM occccad.domain_transactions t JOIN occccad.document_versions v ON v.id=t.result_revision_id WHERE t.id=$1 AND t.workspace_id=$2 AND t.kind='REVERT' AND t.status='COMMITTED'`, consumedRevert, workspaceID).Scan(&outcome)
		} else {
			err = service.database.QueryRow(ctx, `SELECT v.model_json FROM occccad.domain_transactions t JOIN occccad.document_versions v ON v.id=t.result_revision_id WHERE t.root_transaction_id=$1 AND t.workspace_id=$2 AND t.kind='REAPPLY' AND t.status='COMMITTED' ORDER BY t.sequence DESC LIMIT 1`, originalTransaction, workspaceID).Scan(&outcome)
			if errors.Is(err, database.ErrNoRows) {
				err = nil
			}
		}
		if err != nil {
			return err
		}
		if len(outcome) > 0 {
			original, err = historyChangeSetAgainstOutcome(documentType, original, outcome, request.Type == "UNDO")
			if err != nil {
				return err
			}
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
		revisionID := revisions[documentID]
		modelHash := canonicalModelHash(nextJSON)
		var graph *modelcore.DependencyGraph
		var manifest modelcore.EvaluationManifest
		if documentType == "PART" {
			evaluated, e := evaluatePartHistory(ctx, service, historyCommit{documentID: documentID, workspaceID: workspaceID, headRevision: headRevision, requestID: request.RequestID + "/" + documentID, modelJSON: nextJSON, changes: reverse}, revisionID)
			if e != nil {
				return e
			}
			if evaluated.revisionState != "READY" {
				return fmt.Errorf("%w: PRODUCT_DESIGN_HISTORY_PART_FAILED", ErrValidation)
			}
			nextJSON, modelHash, graph, manifest = evaluated.nextJSON, evaluated.modelHash, evaluated.graph, evaluated.manifest
			reverse, err = reconcilePersistedChanges(documentType, currentJSON, nextJSON, evaluated.changes)
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
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("%w: ProductDesignTransaction has an incomplete member set", ErrValidation)
	}
	products := map[string]ProductModel{}
	for _, candidate := range candidates {
		if candidate.documentType != "PRODUCT" {
			continue
		}
		var model ProductModel
		if err := json.Unmarshal(candidate.nextJSON, &model); err != nil {
			return err
		}
		for i := range model.Instances {
			instance := &model.Instances[i]
			if revision := revisions[instance.ReferencedDocumentID]; revision != "" && instance.ReferenceMode != "PINNED" {
				instance.ReferencedVersionID, instance.ResolvedVersionID = revision, revision
			}
		}
		products[candidate.documentID] = model
	}
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.documentType != "PRODUCT" {
			continue
		}
		model := products[candidate.documentID]
		for j := range model.ContextBindings {
			binding := &model.ContextBindings[j]
			for k := range binding.OwningInstancePath.Segments {
				segment := &binding.OwningInstancePath.Segments[k]
				owner, ok := products[segment.OwnerDocumentID]
				if !ok {
					continue
				}
				for _, instance := range owner.Instances {
					if instance.ID == segment.InstanceID {
						segment.OwnerVersionID = revisions[segment.OwnerDocumentID]
						segment.ResolvedVersionID = instance.ReferencedVersionID
						segment.InstanceName = instance.Name
						break
					}
				}
			}
			if n := len(binding.OwningInstancePath.Segments); n > 0 {
				binding.Accepted.OwningRevisionID = binding.OwningInstancePath.Segments[n-1].ResolvedVersionID
			}
		}
		final, _ := json.Marshal(model)
		var current []byte
		for _, v := range inputs {
			if v.documentID == candidate.documentID {
				current = v.currentJSON
				break
			}
		}
		candidate.changes, err = reconcilePersistedChanges("PRODUCT", current, final, candidate.changes)
		if err != nil {
			return err
		}
		candidate.nextJSON, candidate.modelHash = final, canonicalModelHash(final)
		candidate.graph, candidate.manifest, err = buildProductEvaluation(model, candidate.revisionID, candidate.modelHash, candidate.changes.ImpactSeeds, nil)
		if err != nil {
			return err
		}
		// The owning Part is not in the database until this atomic commit. Check
		// its complete accepted inputs against the in-memory member instead.
		for _, part := range candidates {
			if part.documentType != "PART" {
				continue
			}
			var base PartModel
			if err := json.Unmarshal(part.nextJSON, &base); err != nil {
				return err
			}
			grouped := map[string][]ContextBinding{}
			for _, binding := range model.ContextBindings {
				path := binding.OwningInstancePath
				if len(path.Segments) == 1 && path.Segments[0].ReferencedDocumentID == part.documentID && path.Segments[0].ResolvedVersionID == part.revisionID {
					grouped[path.Canonical] = append(grouped[path.Canonical], binding)
				}
			}
			for path, bindings := range grouped {
				projection := clonePartModel(base)
				if err := service.resolveVariantModel(ctx, part.documentID, path, &projection, bindings); err != nil {
					return err
				}
				if err := service.evaluatePartBodies(ctx, request.RequestID+"/history-context/"+path, &projection); err != nil {
					return err
				}
			}
		}
	}
	groupUUID, _ := uuid.NewV7()
	return service.commitProductDesignCandidates(ctx, groupUUID.String(), request.RequestID, requestDigest,
		actorID(request.ActorID), rootProductDocumentID, candidates)
}
