package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

const historyCapabilitiesSQL = `
		WITH workspace AS (
			SELECT id FROM occccad.workspaces WHERE document_id=$1 AND name='main'
		), boundary AS (
			SELECT coalesce(max(sequence),0) AS sequence FROM occccad.domain_transactions
			WHERE workspace_id=(SELECT id FROM workspace) AND actor_id=$2 AND status='COMMITTED'
			  AND kind IN ('DOMAIN','RESTORE','CREATE')
		)
		SELECT
			EXISTS (
				SELECT 1 FROM occccad.domain_transactions root
				WHERE root.workspace_id=(SELECT id FROM workspace) AND root.actor_id=$2
				  AND root.status='COMMITTED' AND root.kind IN ('DOMAIN','RESTORE')
				  AND COALESCE((SELECT action.kind FROM occccad.domain_transactions action WHERE action.root_transaction_id=root.id AND action.status='COMMITTED' ORDER BY action.sequence DESC LIMIT 1),'REAPPLY')='REAPPLY'
			),
			EXISTS (
				SELECT 1 FROM occccad.domain_transactions revert_tx CROSS JOIN boundary
				WHERE revert_tx.workspace_id=(SELECT id FROM workspace) AND revert_tx.actor_id=$2
				  AND revert_tx.kind='REVERT' AND revert_tx.status='COMMITTED'
				  AND revert_tx.sequence>boundary.sequence
				  AND NOT EXISTS (SELECT 1 FROM occccad.domain_transactions reapply
				      WHERE reapply.reapplies_transaction_id=revert_tx.id AND reapply.status='COMMITTED')
			)`

func (service *Service) historyCapabilities(ctx context.Context, documentID, actor string) (bool, bool, error) {
	if strings.TrimSpace(actor) == "" {
		return false, false, nil
	}
	actor = actorID(actor)
	var canUndo, canRedo bool
	err := service.database.QueryRow(ctx, historyCapabilitiesSQL, documentID, actor).Scan(&canUndo, &canRedo)
	return canUndo, canRedo, err
}

// Reuse exactly the same history fold, pipelining page-sized reads to avoid one
// network round trip per document. The page's rows must already be closed.
func (service *Service) populateHistoryCapabilities(ctx context.Context, documents []DocumentSummary, actor string) error {
	if strings.TrimSpace(actor) == "" || len(documents) == 0 {
		return nil
	}
	const chunkSize = 128
	for start := 0; start < len(documents); start += chunkSize {
		end := min(start+chunkSize, len(documents))
		batch := &database.Batch{}
		for _, document := range documents[start:end] {
			batch.Queue(historyCapabilitiesSQL, document.ID, actorID(actor))
		}
		results := service.database.SendBatch(ctx, batch)
		for i := start; i < end; i++ {
			if err := results.QueryRow().Scan(&documents[i].CanUndo, &documents[i].CanRedo); err != nil {
				_ = results.Close()
				return err
			}
		}
		if err := results.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) applyCompensatingHistory(ctx context.Context, documentID string, request CommandRequest) error {
	request.RequestID = requestID(request.RequestID)
	var workspaceID, headRevision, documentType string
	var headSequence uint64
	var modelJSON json.RawMessage
	if err := service.database.QueryRow(ctx, `
		SELECT w.id::text,w.head_revision_id::text,w.head_sequence,d.document_type,v.model_json
		FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id
		JOIN occccad.document_versions v ON v.id=w.head_revision_id
		WHERE w.document_id=$1 AND w.name='main' AND d.deleted_at IS NULL`, documentID).
		Scan(&workspaceID, &headRevision, &headSequence, &documentType, &modelJSON); errors.Is(err, database.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	actor := actorID(request.ActorID)
	historyPayload, _ := json.Marshal(map[string]string{"type": request.Type, "versionId": request.VersionID})
	historyDigest := modelcore.ValueDigest(historyPayload)
	var storedDigest, storedActor string
	if err := service.database.QueryRow(ctx, `SELECT request_digest,actor_id::text FROM occccad.domain_transactions WHERE workspace_id=$1 AND request_id=$2 AND status='COMMITTED'`, workspaceID, request.RequestID).Scan(&storedDigest, &storedActor); err == nil {
		if storedActor != actorID(request.ActorID) || storedDigest != historyDigest {
			return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		return nil
	} else if !errors.Is(err, database.ErrNoRows) {
		return err
	}
	var rootTransaction, consumedRevert, productDesignTransaction string
	var changeJSON []byte
	var persistedWrites []string
	if request.Type == "UNDO" {
		err := service.database.QueryRow(ctx, `
			SELECT root.id::text,coalesce(root.product_design_transaction_id::text,''),cs.canonical_blob,cs.write_set
			FROM occccad.domain_transactions root
			JOIN occccad.change_sets cs ON cs.transaction_id=root.id
			WHERE root.workspace_id=$1 AND root.actor_id=$2 AND root.status='COMMITTED'
			  AND root.kind IN ('DOMAIN','RESTORE')
			  AND COALESCE((SELECT action.kind FROM occccad.domain_transactions action WHERE action.root_transaction_id=root.id AND action.status='COMMITTED' ORDER BY action.sequence DESC LIMIT 1),'REAPPLY')='REAPPLY'
			ORDER BY root.sequence DESC LIMIT 1`, workspaceID, actor).Scan(&rootTransaction, &productDesignTransaction, &changeJSON, &persistedWrites)
		if errors.Is(err, database.ErrNoRows) {
			return fmt.Errorf("%w: nothing to undo for this actor", ErrValidation)
		}
		if err != nil {
			return err
		}
	} else {
		err := service.database.QueryRow(ctx, `
			WITH boundary AS (
				SELECT coalesce(max(sequence),0) AS sequence
				FROM occccad.domain_transactions
				WHERE workspace_id=$1 AND actor_id=$2 AND status='COMMITTED'
				  AND kind IN ('DOMAIN','RESTORE','CREATE')
			)
			SELECT root.id::text,revert_tx.id::text,coalesce(root.product_design_transaction_id::text,''),cs.canonical_blob,cs.write_set
			FROM occccad.domain_transactions revert_tx
			JOIN occccad.domain_transactions root ON root.id=revert_tx.root_transaction_id
			JOIN occccad.change_sets cs ON cs.transaction_id=root.id
			CROSS JOIN boundary
			WHERE revert_tx.workspace_id=$1 AND revert_tx.actor_id=$2
			  AND revert_tx.kind='REVERT' AND revert_tx.status='COMMITTED'
			  AND revert_tx.sequence>boundary.sequence
			  AND NOT EXISTS (SELECT 1 FROM occccad.domain_transactions reapply
			      WHERE reapply.reapplies_transaction_id=revert_tx.id AND reapply.status='COMMITTED')
			ORDER BY revert_tx.sequence DESC LIMIT 1`, workspaceID, actor).Scan(&rootTransaction, &consumedRevert, &productDesignTransaction, &changeJSON, &persistedWrites)
		if errors.Is(err, database.ErrNoRows) {
			return fmt.Errorf("%w: REDO_NOT_AVAILABLE", ErrValidation)
		}
		if err != nil {
			return err
		}
	}
	if productDesignTransaction != "" {
		return service.applyProductDesignCompensatingHistory(ctx, productDesignTransaction, request, historyDigest)
	}
	var original modelcore.ChangeSet
	if err := json.Unmarshal(changeJSON, &original); err != nil {
		return err
	}
	if err := validatePersistedChangeSetStructure(original, persistedWrites); err != nil {
		return fmt.Errorf("%w: invalid persisted ChangeSet: %v", ErrValidation, err)
	}
	// The immutable base/result revisions are the final authority for the
	// transaction's before/after values. Reconstructing through the persisted
	// write set also repairs ChangeSets produced before evaluator-normalized
	// sketch values were recorded, without weakening compensation conflict checks.
	var transactionBase, transactionResult json.RawMessage
	if err := service.database.QueryRow(ctx, `
		SELECT base.model_json,result.model_json
		FROM occccad.domain_transactions domain_tx
		JOIN occccad.document_versions base ON base.id=domain_tx.base_revision_id
		JOIN occccad.document_versions result ON result.id=domain_tx.result_revision_id
		WHERE domain_tx.id=$1`, rootTransaction).Scan(&transactionBase, &transactionResult); err != nil {
		return err
	}
	reconciled, err := reconcilePersistedChanges(documentType, transactionBase, transactionResult, original)
	if err != nil {
		return err
	}
	original = reconciled
	// Both Product solve evidence and Part Publication resolution can be
	// recomputed for a compensation Revision. Compare against that immutable
	// committed outcome, not the root transaction's older evaluator fields.
	// Business fields remain strictly guarded by the same ChangeSet checks.
	// Registered adapters compare derived values against the immutable outcome.
	var actualOutcome json.RawMessage
	if request.Type == "REDO" {
		err = service.database.QueryRow(ctx, `SELECT v.model_json FROM occccad.domain_transactions t
            JOIN occccad.document_versions v ON v.id=t.result_revision_id
            WHERE t.id=$1 AND t.workspace_id=$2 AND t.status='COMMITTED' AND t.kind='REVERT'`, consumedRevert, workspaceID).Scan(&actualOutcome)
	} else {
		err = service.database.QueryRow(ctx, `SELECT v.model_json FROM occccad.domain_transactions t
            JOIN occccad.document_versions v ON v.id=t.result_revision_id
            WHERE t.root_transaction_id=$1 AND t.workspace_id=$2 AND t.status='COMMITTED' AND t.kind='REAPPLY'
            ORDER BY t.sequence DESC LIMIT 1`, rootTransaction, workspaceID).Scan(&actualOutcome)
		if errors.Is(err, database.ErrNoRows) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	if len(actualOutcome) > 0 {
		original, err = historyChangeSetAgainstOutcome(documentType, original, actualOutcome, request.Type == "UNDO")
		if err != nil {
			return err
		}
	}
	current, err := modelValues(documentType, modelJSON, original)
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
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	nextJSON, err := applyModelValues(documentType, modelJSON, desired)
	if err != nil {
		return err
	}
	reverse, err := changesBetweenValues(current, desired, original.ImpactSeeds)
	if err != nil {
		return err
	}
	kind := "REVERT"
	typeURI := "occccad://history/revert"
	if request.Type == "REDO" {
		kind = "REAPPLY"
		typeURI = "occccad://history/reapply"
	}
	return service.commitHistoryRevision(ctx, historyCommit{documentID: documentID, workspaceID: workspaceID,
		headRevision: headRevision, documentType: documentType, actorID: actor, requestID: request.RequestID,
		headSequence: headSequence, modelJSON: nextJSON, changes: reverse, kind: kind, typeURI: typeURI,
		rootTransaction: rootTransaction, consumedRevert: consumedRevert, requestDigest: historyDigest})
}

func validatePersistedChangeSetStructure(set modelcore.ChangeSet, persistedWrites []string) error {
	if err := set.ValidateStructure(); err != nil {
		return err
	}
	if len(set.Changes) != len(persistedWrites) {
		return fmt.Errorf("canonical change count does not match persisted write set")
	}
	writes := make(map[string]struct{}, len(persistedWrites))
	for _, key := range persistedWrites {
		if _, duplicate := writes[key]; duplicate {
			return fmt.Errorf("persisted write set contains duplicate %s", key)
		}
		writes[key] = struct{}{}
	}
	for _, change := range set.Changes {
		if _, ok := writes[change.Target.Key()]; !ok {
			return fmt.Errorf("change target %s is absent from persisted write set", change.Target.Key())
		}
	}
	return nil
}

func (service *Service) applyRestoreRevision(ctx context.Context, documentID string, request CommandRequest) error {
	request.RequestID = requestID(request.RequestID)
	var workspaceID, headRevision, documentType string
	var headSequence uint64
	var current, target json.RawMessage
	if err := service.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,d.document_type,current.model_json,target.model_json FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id JOIN occccad.document_versions current ON current.id=w.head_revision_id JOIN occccad.document_versions target ON target.id=$2 AND target.document_id=d.id WHERE w.document_id=$1 AND w.name='main'`, documentID, request.VersionID).Scan(&workspaceID, &headRevision, &headSequence, &documentType, &current, &target); errors.Is(err, database.ErrNoRows) {
		return fmt.Errorf("%w: restore point does not belong to this document", ErrValidation)
	} else if err != nil {
		return err
	}
	historyPayload, _ := json.Marshal(map[string]string{"type": "RESTORE", "versionId": request.VersionID})
	historyDigest := modelcore.ValueDigest(historyPayload)
	var storedDigest, storedActor string
	if err := service.database.QueryRow(ctx, `SELECT request_digest,actor_id::text FROM occccad.domain_transactions WHERE workspace_id=$1 AND request_id=$2 AND status='COMMITTED'`, workspaceID, request.RequestID).Scan(&storedDigest, &storedActor); err == nil {
		if storedActor != actorID(request.ActorID) || storedDigest != historyDigest {
			return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		return nil
	} else if !errors.Is(err, database.ErrNoRows) {
		return err
	}
	change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: documentID, SlotID: "document.model"}, json.RawMessage(current), json.RawMessage(target))
	set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"document:" + modelcore.DependencyKey(documentID)}}
	if err := set.Finalize(); err != nil {
		return err
	}
	return service.commitHistoryRevision(ctx, historyCommit{documentID: documentID, workspaceID: workspaceID, headRevision: headRevision, documentType: documentType, actorID: actorID(request.ActorID), requestID: request.RequestID, headSequence: headSequence, modelJSON: target, changes: set, kind: "RESTORE", typeURI: "occccad://history/restore", requestDigest: historyDigest})
}

func modelValues(documentType string, modelJSON json.RawMessage, set modelcore.ChangeSet) (map[modelcore.PropertyAddress]json.RawMessage, error) {
	adapter, err := documentAdapters.Lookup(documentType)
	if err != nil {
		return nil, err
	}
	return adapter.HistoryValues(modelJSON, set)
}

func applyModelValues(documentType string, modelJSON json.RawMessage, values map[modelcore.PropertyAddress]json.RawMessage) (json.RawMessage, error) {
	adapter, err := documentAdapters.Lookup(documentType)
	if err != nil {
		return nil, err
	}
	return adapter.ApplyHistoryValues(modelJSON, values)
}

// reconcilePersistedChanges rebuilds the handler ChangeSet against the exact
// model bytes that will be committed. Authoritative evaluators may normalize a
// command candidate (for example, PlaneGCS updates sketch coordinates and solve
// diagnostics), so persisting the handler's pre-evaluation after value would
// make a subsequent compensation compare against a state that never existed.
func reconcilePersistedChanges(documentType string, beforeJSON, afterJSON json.RawMessage, set modelcore.ChangeSet) (modelcore.ChangeSet, error) {
	before, err := modelValues(documentType, beforeJSON, set)
	if err != nil {
		return modelcore.ChangeSet{}, err
	}
	after, err := modelValues(documentType, afterJSON, set)
	if err != nil {
		return modelcore.ChangeSet{}, err
	}
	return changesBetweenValues(before, after, set.ImpactSeeds)
}

func changesBetweenValues(before, after map[modelcore.PropertyAddress]json.RawMessage, seeds []modelcore.DependencyKey) (modelcore.ChangeSet, error) {
	set := modelcore.ChangeSet{ImpactSeeds: append([]modelcore.DependencyKey(nil), seeds...)}
	addresses := map[modelcore.PropertyAddress]struct{}{}
	for address := range before {
		addresses[address] = struct{}{}
	}
	for address := range after {
		addresses[address] = struct{}{}
	}
	for address := range addresses {
		beforeValue := before[address]
		afterValue := after[address]
		kind := modelcore.ChangeUpdate
		if len(beforeValue) == 0 {
			kind = modelcore.ChangeCreate
		} else if len(afterValue) == 0 {
			kind = modelcore.ChangeDelete
		}
		var beforeAny, afterAny any
		if len(beforeValue) > 0 {
			beforeAny = json.RawMessage(beforeValue)
		}
		if len(afterValue) > 0 {
			afterAny = json.RawMessage(afterValue)
		}
		change, err := modelcore.NewChange(kind, address, beforeAny, afterAny)
		if err != nil {
			return set, err
		}
		set.Changes = append(set.Changes, change)
	}
	return set, set.Finalize()
}

type historyCommit struct {
	documentID, workspaceID, headRevision, documentType, actorID, requestID, kind, typeURI string
	rootTransaction, consumedRevert                                                        string
	requestDigest                                                                          string
	headSequence                                                                           uint64
	modelJSON                                                                              json.RawMessage
	changes                                                                                modelcore.ChangeSet
}

func (service *Service) commitHistoryRevision(ctx context.Context, input historyCommit) error {
	revisionUUID, _ := uuid.NewV7()
	transactionUUID, _ := uuid.NewV7()
	revisionID := revisionUUID.String()
	transactionID := transactionUUID.String()
	adapter, err := documentAdapters.Lookup(input.documentType)
	if err != nil {
		return err
	}
	evaluated, err := adapter.EvaluateHistory(ctx, service, input, revisionID)
	input.modelJSON, input.changes = evaluated.nextJSON, evaluated.changes
	modelHash, graph, manifest := evaluated.modelHash, evaluated.graph, evaluated.manifest
	revisionState, evaluationStatus := evaluated.revisionState, evaluated.evaluationStatus
	if err != nil {
		return err
	}
	manifestJSON, _ := json.Marshal(manifest)
	manifestDigest := modelcore.ValueDigest(manifestJSON)
	dependencyDigest, _ := graph.Digest()
	changesJSON, _ := json.Marshal(input.changes)
	payload, _ := json.Marshal(map[string]string{"rootTransactionId": input.rootTransaction, "consumedRevertTransactionId": input.consumedRevert})
	requestDigest := input.requestDigest
	if requestDigest == "" {
		requestDigest = modelcore.ValueDigest(payload)
	}
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentHead string
	var currentSequence uint64
	if err = tx.QueryRow(ctx, `SELECT head_revision_id::text,head_sequence FROM occccad.workspaces WHERE id=$1 FOR UPDATE`, input.workspaceID).Scan(&currentHead, &currentSequence); err != nil {
		return err
	}
	if currentHead != input.headRevision || currentSequence != input.headSequence {
		var completedDigest, completedActor string
		completedErr := tx.QueryRow(ctx, `SELECT request_digest,actor_id::text FROM occccad.domain_transactions WHERE workspace_id=$1 AND request_id=$2 AND status='COMMITTED'`, input.workspaceID, input.requestID).Scan(&completedDigest, &completedActor)
		if completedErr == nil {
			if completedActor != input.actorID || completedDigest != requestDigest {
				return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
			}
			return nil
		}
		if !errors.Is(completedErr, database.ErrNoRows) {
			return completedErr
		}

		return fmt.Errorf("%w: WORKSPACE_HEAD_CONFLICT", ErrValidation)
	}
	var revisionSequence uint64
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(sequence),0)+1 FROM occccad.document_versions WHERE document_id=$1`, input.documentID).Scan(&revisionSequence); err != nil {
		return err
	}
	traceID, spanID := traceIDs(ctx)
	var commandID string
	if err = tx.QueryRow(ctx, `INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id) VALUES($1,$2,$3,$4,'SUCCEEDED',now(),$5,$6) RETURNING id::text`, input.requestID, input.kind, input.documentID, payload, traceID, spanID).Scan(&commandID); err != nil {
		return err
	}
	batch := &database.Batch{}
	batch.Queue(`INSERT INTO occccad.document_versions(id,document_id,parent_version_id,sequence,model_json,state,created_by_command_id,model_hash,dependency_snapshot_digest,evaluation_manifest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, revisionID, input.documentID, input.headRevision, revisionSequence, input.modelJSON, revisionState, commandID, modelHash, dependencyDigest, manifestJSON)
	batch.Queue(`INSERT INTO occccad.revision_parents(revision_id,parent_revision_id) VALUES($1,$2)`, revisionID, input.headRevision)
	var revertID, reapplyID any
	var rootID any
	if input.rootTransaction != "" {
		rootID = input.rootTransaction
	}
	if input.kind == "REVERT" {
		revertID = input.rootTransaction
	} else if input.kind == "REAPPLY" {
		reapplyID = input.consumedRevert
	}
	batch.Queue(`INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,base_revision_id,result_revision_id,root_transaction_id,reverts_transaction_id,reapplies_transaction_id,committed_at) VALUES($1,$2,$3,$4,$5,$6,$7,'COMMITTED',$8,$9,$10,$11,$12,now())`, transactionID, input.workspaceID, currentSequence+1, input.actorID, input.requestID, requestDigest, input.kind, input.headRevision, revisionID, rootID, revertID, reapplyID)
	batch.Queue(`INSERT INTO occccad.transaction_commands(transaction_id,ordinal,command_id,type_uri,schema_version,payload,payload_digest) VALUES($1,0,$2,$3,1,$4,$5)`, transactionID, newID("command"), input.typeURI, payload, requestDigest)
	writes := []string{}
	for _, change := range input.changes.Changes {
		writes = append(writes, change.Target.Key())
	}
	batch.Queue(`INSERT INTO occccad.change_sets(transaction_id,canonical_blob,canonical_digest,write_set,impact_seeds) VALUES($1,$2,$3,$4,$5)`, transactionID, changesJSON, input.changes.CanonicalDigest, writes, input.changes.ImpactSeeds)
	batch.Queue(`INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,$7,true)`, revisionID, strings.ToLower(input.documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest, evaluationStatus)
	for _, edge := range graph.Edges {
		batch.Queue(`INSERT INTO occccad.dependency_edges(revision_id,source_key,target_key,edge_kind) VALUES($1,$2,$3,$4)`, revisionID, edge.Source, edge.Target, edge.Kind)
	}
	event, _ := json.Marshal(map[string]any{"workspaceId": input.workspaceID, "sequence": currentSequence + 1, "revisionId": revisionID, "transactionId": transactionID})
	batch.Queue(`INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload) VALUES('WORKSPACE',$1,'workspace.transaction.committed.v1',1,$2)`, input.workspaceID, event)
	if err := database.ExecBatch(ctx, tx, batch); err != nil {
		return err
	}

	var position int
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(position),-1)+1 FROM occccad.document_history WHERE document_id=$1`, input.documentID).Scan(&position); err != nil {
		return err
	}
	batch = &database.Batch{}
	batch.Queue(`INSERT INTO occccad.document_history(document_id,position,version_id,command_id) VALUES($1,$2,$3,$4)`, input.documentID, position, revisionID, commandID)
	batch.Queue(`INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type) VALUES($1,$2,$3,$4)`, input.documentID, revisionID, commandID, input.kind)
	batch.Queue(`UPDATE occccad.workspaces SET head_revision_id=$1,head_sequence=$2,updated_at=now() WHERE id=$3`, revisionID, currentSequence+1, input.workspaceID)
	batch.Queue(`UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`, revisionID, input.documentID)
	if err := database.ExecBatch(ctx, tx, batch); err != nil {
		return err
	}

	if err := adapter.Persist(ctx, tx, revisionID, input.modelJSON); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
