package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

func (service *Service) historyCapabilities(ctx context.Context, documentID, actor string) (bool, bool, error) {
	if strings.TrimSpace(actor) == "" {
		return false, false, nil
	}
	actor = actorID(actor)
	var canUndo, canRedo bool
	err := service.database.QueryRow(ctx, `
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
				LEFT JOIN LATERAL (
					SELECT action.kind FROM occccad.domain_transactions action
					WHERE action.root_transaction_id=root.id AND action.status='COMMITTED'
					ORDER BY action.sequence DESC LIMIT 1
				) latest ON true
				WHERE root.workspace_id=(SELECT id FROM workspace) AND root.actor_id=$2
				  AND root.status='COMMITTED' AND root.kind IN ('DOMAIN','RESTORE')
				  AND (latest.kind IS NULL OR latest.kind='REAPPLY')
			),
			EXISTS (
				SELECT 1 FROM occccad.domain_transactions revert_tx CROSS JOIN boundary
				WHERE revert_tx.workspace_id=(SELECT id FROM workspace) AND revert_tx.actor_id=$2
				  AND revert_tx.kind='REVERT' AND revert_tx.status='COMMITTED'
				  AND revert_tx.sequence>boundary.sequence
				  AND NOT EXISTS (SELECT 1 FROM occccad.domain_transactions reapply
				      WHERE reapply.reapplies_transaction_id=revert_tx.id AND reapply.status='COMMITTED')
			)`, documentID, actor).Scan(&canUndo, &canRedo)
	return canUndo, canRedo, err
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
		Scan(&workspaceID, &headRevision, &headSequence, &documentType, &modelJSON); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	actor := actorID(request.ActorID)
	historyPayload, _ := json.Marshal(map[string]string{"type": request.Type, "versionId": request.VersionID})
	historyDigest := modelcore.ValueDigest(historyPayload)
	var storedDigest string
	if err := service.database.QueryRow(ctx, `SELECT request_digest FROM occccad.domain_transactions WHERE workspace_id=$1 AND request_id=$2 AND status='COMMITTED'`, workspaceID, request.RequestID).Scan(&storedDigest); err == nil {
		if storedDigest != historyDigest {
			return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var rootTransaction, consumedRevert string
	var changeJSON []byte
	if request.Type == "UNDO" {
		err := service.database.QueryRow(ctx, `
			SELECT root.id::text,cs.canonical_blob
			FROM occccad.domain_transactions root
			JOIN occccad.change_sets cs ON cs.transaction_id=root.id
			LEFT JOIN LATERAL (
				SELECT action.kind FROM occccad.domain_transactions action
				WHERE action.root_transaction_id=root.id AND action.status='COMMITTED'
				ORDER BY action.sequence DESC LIMIT 1
			) latest ON true
			WHERE root.workspace_id=$1 AND root.actor_id=$2 AND root.status='COMMITTED'
			  AND root.kind IN ('DOMAIN','RESTORE')
			  AND (latest.kind IS NULL OR latest.kind='REAPPLY')
			ORDER BY root.sequence DESC LIMIT 1`, workspaceID, actor).Scan(&rootTransaction, &changeJSON)
		if errors.Is(err, pgx.ErrNoRows) {
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
			SELECT root.id::text,revert_tx.id::text,cs.canonical_blob
			FROM occccad.domain_transactions revert_tx
			JOIN occccad.domain_transactions root ON root.id=revert_tx.root_transaction_id
			JOIN occccad.change_sets cs ON cs.transaction_id=root.id
			CROSS JOIN boundary
			WHERE revert_tx.workspace_id=$1 AND revert_tx.actor_id=$2
			  AND revert_tx.kind='REVERT' AND revert_tx.status='COMMITTED'
			  AND revert_tx.sequence>boundary.sequence
			  AND NOT EXISTS (SELECT 1 FROM occccad.domain_transactions reapply
			      WHERE reapply.reapplies_transaction_id=revert_tx.id AND reapply.status='COMMITTED')
			ORDER BY revert_tx.sequence DESC LIMIT 1`, workspaceID, actor).Scan(&rootTransaction, &consumedRevert, &changeJSON)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: REDO_NOT_AVAILABLE", ErrValidation)
		}
		if err != nil {
			return err
		}
	}
	var original modelcore.ChangeSet
	if err := json.Unmarshal(changeJSON, &original); err != nil {
		return err
	}
	if err := original.Finalize(); err != nil {
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

func (service *Service) applyRestoreRevision(ctx context.Context, documentID string, request CommandRequest) error {
	request.RequestID = requestID(request.RequestID)
	var workspaceID, headRevision, documentType string
	var headSequence uint64
	var current, target json.RawMessage
	if err := service.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,d.document_type,current.model_json,target.model_json FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id JOIN occccad.document_versions current ON current.id=w.head_revision_id JOIN occccad.document_versions target ON target.id=$2 AND target.document_id=d.id WHERE w.document_id=$1 AND w.name='main'`, documentID, request.VersionID).Scan(&workspaceID, &headRevision, &headSequence, &documentType, &current, &target); errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: restore point does not belong to this document", ErrValidation)
	} else if err != nil {
		return err
	}
	historyPayload, _ := json.Marshal(map[string]string{"type": "RESTORE", "versionId": request.VersionID})
	historyDigest := modelcore.ValueDigest(historyPayload)
	var storedDigest string
	if err := service.database.QueryRow(ctx, `SELECT request_digest FROM occccad.domain_transactions WHERE workspace_id=$1 AND request_id=$2 AND status='COMMITTED'`, workspaceID, request.RequestID).Scan(&storedDigest); err == nil {
		if storedDigest != historyDigest {
			return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
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
	result := map[modelcore.PropertyAddress]json.RawMessage{}
	if documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return nil, err
		}
		for _, change := range set.Changes {
			switch change.Target.SlotID {
			case "entity":
				for _, feature := range model.Features {
					if feature.ID == change.Target.EntityID {
						result[change.Target], _ = json.Marshal(feature)
					}
				}
			case "sketch.model":
				for _, feature := range model.Features {
					if feature.ID == change.Target.EntityID && feature.Sketch != nil {
						result[change.Target], _ = json.Marshal(feature.Sketch)
					}
				}
			case "parameter.source":
				for _, parameter := range model.Parameters {
					if parameter.ParameterID == change.Target.EntityID {
						result[change.Target], _ = json.Marshal(parameter.Source)
					}
				}
			case "document.model":
				result[change.Target] = append(json.RawMessage(nil), modelJSON...)
			}
		}
	} else {
		var model ProductModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return nil, err
		}
		for _, change := range set.Changes {
			for _, instance := range model.Instances {
				if instance.ID != change.Target.EntityID {
					continue
				}
				switch change.Target.SlotID {
				case "entity":
					result[change.Target], _ = json.Marshal(instance)
				case "instance.translation":
					result[change.Target], _ = json.Marshal(instance.Translation)
				case "instance.reference":
					result[change.Target], _ = json.Marshal(struct{ Mode, Version string }{instance.ReferenceMode, instance.ReferencedVersionID})
				}
			}
			if change.Target.SlotID == "document.model" {
				result[change.Target] = append(json.RawMessage(nil), modelJSON...)
			}
		}
	}
	return result, nil
}

func applyModelValues(documentType string, modelJSON json.RawMessage, values map[modelcore.PropertyAddress]json.RawMessage) (json.RawMessage, error) {
	if documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return nil, err
		}
		for address, value := range values {
			switch address.SlotID {
			case "document.model":
				return append(json.RawMessage(nil), value...), nil
			case "entity":
				index := -1
				for i := range model.Features {
					if model.Features[i].ID == address.EntityID {
						index = i
					}
				}
				if len(value) == 0 || string(value) == "null" {
					if index >= 0 {
						for _, dependent := range model.Features {
							if dependent.Profile == address.EntityID {
								return nil, fmt.Errorf("%w: cannot remove %s while feature %s depends on it", ErrValidation, address.EntityID, dependent.ID)
							}
						}
						model.Features = append(model.Features[:index], model.Features[index+1:]...)
					}
					filtered := model.Parameters[:0]
					for _, parameter := range model.Parameters {
						if !strings.HasPrefix(parameter.ParameterID, "parameter:"+address.EntityID+":") {
							filtered = append(filtered, parameter)
						}
					}
					model.Parameters = filtered
				} else {
					var feature Feature
					if err := json.Unmarshal(value, &feature); err != nil {
						return nil, err
					}
					if index >= 0 {
						model.Features[index] = feature
					} else {
						model.Features = append(model.Features, feature)
					}
				}
			case "sketch.model":
				index := -1
				for i := range model.Features {
					if model.Features[i].ID == address.EntityID {
						index = i
						break
					}
				}
				if index < 0 || model.Features[index].Type != "SKETCH" || model.Features[index].Sketch == nil {
					return nil, fmt.Errorf("%w: sketch %s was deleted", ErrValidation, address.EntityID)
				}
				if len(value) == 0 || string(value) == "null" {
					return nil, fmt.Errorf("%w: sketch.model cannot be removed independently", ErrValidation)
				}
				var sketch SketchFeature
				if err := json.Unmarshal(value, &sketch); err != nil {
					return nil, err
				}
				model.Features[index].Sketch = &sketch
			case "parameter.source":
				for i := range model.Parameters {
					if model.Parameters[i].ParameterID == address.EntityID {
						if err := json.Unmarshal(value, &model.Parameters[i].Source); err != nil {
							return nil, err
						}
					}
				}
			}
		}
		normalizePartModel(&model)
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, err
		}
		return json.Marshal(model)
	}
	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, err
	}
	for address, value := range values {
		if address.SlotID == "document.model" {
			return append(json.RawMessage(nil), value...), nil
		}
		index := -1
		for i := range model.Instances {
			if model.Instances[i].ID == address.EntityID {
				index = i
			}
		}
		switch address.SlotID {
		case "entity":
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.Instances = append(model.Instances[:index], model.Instances[index+1:]...)
				}
			} else {
				var instance ProductInstance
				if err := json.Unmarshal(value, &instance); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.Instances[index] = instance
				} else {
					model.Instances = append(model.Instances, instance)
				}
			}
		case "instance.translation":
			if index < 0 {
				return nil, fmt.Errorf("%w: instance was deleted", ErrValidation)
			}
			if err := json.Unmarshal(value, &model.Instances[index].Translation); err != nil {
				return nil, err
			}
		case "instance.reference":
			if index < 0 {
				return nil, fmt.Errorf("%w: instance was deleted", ErrValidation)
			}
			var reference struct{ Mode, Version string }
			if err := json.Unmarshal(value, &reference); err != nil {
				return nil, err
			}
			model.Instances[index].ReferenceMode = reference.Mode
			model.Instances[index].ReferencedVersionID = reference.Version
		}
	}
	return json.Marshal(model)
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
	modelHash := canonicalModelHash(input.modelJSON)
	geometryKey := ""
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var err error
	if input.documentType == "PART" {
		var model PartModel
		if err = json.Unmarshal(input.modelJSON, &model); err != nil {
			return err
		}
		normalizePartModel(&model)
		if err = validateAndResolvePartParameters(&model); err != nil {
			return err
		}
		input.modelJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(input.modelJSON)
		graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, input.changes.ImpactSeeds, nil)
		if err == nil {
			geometryKey, err = service.evaluatePart(ctx, input.requestID, model)
		}
	} else {
		var model ProductModel
		if err = json.Unmarshal(input.modelJSON, &model); err == nil {
			graph, manifest, err = buildProductEvaluation(model, revisionID, modelHash, input.changes.ImpactSeeds, nil)
		}
	}
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
	var nullableGeometry any
	if geometryKey != "" {
		nullableGeometry = geometryKey
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,parent_version_id,sequence,model_json,geometry_key,state,created_by_command_id,model_hash,dependency_snapshot_digest,evaluation_manifest) VALUES($1,$2,$3,$4,$5,$6,'READY',$7,$8,$9,$10)`, revisionID, input.documentID, input.headRevision, revisionSequence, input.modelJSON, nullableGeometry, commandID, modelHash, dependencyDigest, manifestJSON); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.revision_parents(revision_id,parent_revision_id) VALUES($1,$2)`, revisionID, input.headRevision); err != nil {
		return err
	}
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
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,base_revision_id,result_revision_id,root_transaction_id,reverts_transaction_id,reapplies_transaction_id,committed_at) VALUES($1,$2,$3,$4,$5,$6,$7,'COMMITTED',$8,$9,$10,$11,$12,now())`, transactionID, input.workspaceID, currentSequence+1, input.actorID, input.requestID, requestDigest, input.kind, input.headRevision, revisionID, rootID, revertID, reapplyID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.transaction_commands(transaction_id,ordinal,command_id,type_uri,schema_version,payload,payload_digest) VALUES($1,0,$2,$3,1,$4,$5)`, transactionID, newID("command"), input.typeURI, payload, requestDigest); err != nil {
		return err
	}
	writes := []string{}
	for _, change := range input.changes.Changes {
		writes = append(writes, change.Target.Key())
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.change_sets(transaction_id,canonical_blob,canonical_digest,write_set,impact_seeds) VALUES($1,$2,$3,$4,$5)`, transactionID, changesJSON, input.changes.CanonicalDigest, writes, input.changes.ImpactSeeds); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,'SUCCEEDED',true)`, revisionID, strings.ToLower(input.documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest); err != nil {
		return err
	}
	for _, edge := range graph.Edges {
		if _, err = tx.Exec(ctx, `INSERT INTO occccad.dependency_edges(revision_id,source_key,target_key,edge_kind) VALUES($1,$2,$3,$4)`, revisionID, edge.Source, edge.Target, edge.Kind); err != nil {
			return err
		}
	}
	event, _ := json.Marshal(map[string]any{"workspaceId": input.workspaceID, "sequence": currentSequence + 1, "revisionId": revisionID, "transactionId": transactionID})
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload) VALUES('WORKSPACE',$1,'workspace.transaction.committed.v1',1,$2)`, input.workspaceID, event); err != nil {
		return err
	}
	var position int
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(position),-1)+1 FROM occccad.document_history WHERE document_id=$1`, input.documentID).Scan(&position); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.document_history(document_id,position,version_id,command_id) VALUES($1,$2,$3,$4)`, input.documentID, position, revisionID, commandID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type) VALUES($1,$2,$3,$4)`, input.documentID, revisionID, commandID, input.kind); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE occccad.workspaces SET head_revision_id=$1,head_sequence=$2,updated_at=now() WHERE id=$3`, revisionID, currentSequence+1, input.workspaceID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`, revisionID, input.documentID); err != nil {
		return err
	}
	if input.documentType == "PRODUCT" {
		var model ProductModel
		_ = json.Unmarshal(input.modelJSON, &model)
		if err = insertProductInstances(ctx, tx, revisionID, model); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
