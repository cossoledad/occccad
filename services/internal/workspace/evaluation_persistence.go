package workspace

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

func persistEvaluationProjection(ctx context.Context, tx pgx.Tx, revisionID, documentType, modelHash, dependencyDigest string, graph *modelcore.DependencyGraph, manifest modelcore.EvaluationManifest) error {
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := modelcore.ValueDigest(manifestJSON)
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,'SUCCEEDED',true)`, revisionID, strings.ToLower(documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest); err != nil {
		return err
	}
	for _, edge := range graph.Edges {
		if _, err = tx.Exec(ctx, `INSERT INTO occccad.dependency_edges(revision_id,source_key,target_key,edge_kind) VALUES($1,$2,$3,$4)`, revisionID, edge.Source, edge.Target, edge.Kind); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE occccad.document_versions SET dependency_snapshot_digest=$1,evaluation_manifest=$2 WHERE id=$3`, dependencyDigest, manifestJSON, revisionID)
	return err
}

func persistInitialTransaction(ctx context.Context, tx pgx.Tx, workspaceID, documentID, revisionID, actor, modelHash, typeURI string, modelJSON []byte) error {
	transactionUUID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	transactionID := transactionUUID.String()
	requestID := "initial:" + documentID
	requestDigest := modelcore.ValueDigest(modelJSON)
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,result_revision_id,committed_at) VALUES($1,$2,1,$3,$4,$5,'CREATE','COMMITTED',$6,now())`, transactionID, workspaceID, actor, requestID, requestDigest, revisionID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.transaction_commands(transaction_id,ordinal,command_id,type_uri,schema_version,payload,payload_digest) VALUES($1,0,$2,$3,1,$4,$5)`, transactionID, newID("command"), typeURI, modelJSON, requestDigest); err != nil {
		return err
	}
	change, err := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: documentID, SlotID: "document.model"}, nil, json.RawMessage(modelJSON))
	if err != nil {
		return err
	}
	set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"document:" + modelcore.DependencyKey(documentID)}}
	if err = set.Finalize(); err != nil {
		return err
	}
	setJSON, _ := json.Marshal(set)
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.change_sets(transaction_id,canonical_blob,canonical_digest,write_set,impact_seeds) VALUES($1,$2,$3,$4,$5)`, transactionID, setJSON, set.CanonicalDigest, []string{change.Target.Key()}, set.ImpactSeeds); err != nil {
		return err
	}
	event, _ := json.Marshal(map[string]any{"workspaceId": workspaceID, "sequence": 1, "revisionId": revisionID, "transactionId": transactionID, "modelHash": modelHash})
	_, err = tx.Exec(ctx, `INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload) VALUES('WORKSPACE',$1,'workspace.transaction.committed.v1',1,$2)`, workspaceID, event)
	return err
}
