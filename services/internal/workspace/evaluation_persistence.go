package workspace

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

func queueDependencyEdges(batch *database.Batch, revisionID string, graph *modelcore.DependencyGraph) error {
	if len(graph.Edges) == 0 {
		return nil
	}
	edges := slices.Clone(graph.Edges)
	slices.SortFunc(edges, func(a, b modelcore.DependencyEdge) int {
		return cmp.Or(cmp.Compare(a.Source, b.Source), cmp.Compare(a.Target, b.Target), cmp.Compare(a.Kind, b.Kind))
	})
	data, err := json.Marshal(edges)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	// The full evaluation digest continues to bind node inputs. Only the
	// structural edges may share a prior immutable owner in the same document.
	batch.Queue(`UPDATE occccad.document_versions AS version
 SET dependency_edge_digest=$2, dependency_edge_owner=(
   SELECT prior.id FROM occccad.document_versions prior
   WHERE prior.document_id=version.document_id AND prior.sequence<version.sequence
     AND prior.dependency_edge_digest=$2 AND prior.dependency_edge_digest<>'' AND prior.dependency_edge_owner IS NULL
   ORDER BY prior.sequence LIMIT 1
 ) WHERE version.id=$1`, revisionID, hex.EncodeToString(digest[:]))
	batch.Queue(`INSERT INTO occccad.dependency_edge_rows(revision_id,source_key,target_key,edge_kind)
 SELECT $1,edge->>'source',edge->>'target',edge->>'kind' FROM jsonb_array_elements($2::jsonb) AS edge
 WHERE EXISTS(SELECT 1 FROM occccad.document_versions WHERE id=$1 AND dependency_edge_owner IS NULL)`, revisionID, string(data))
	return nil
}

func persistEvaluationProjection(ctx context.Context, tx database.Tx, revisionID, documentType, modelHash, dependencyDigest string, graph *modelcore.DependencyGraph, manifest modelcore.EvaluationManifest) error {
	if len(manifest.PartCheckpoints) > 0 {
		batch := &database.Batch{}
		queuePartCheckpoints(batch, manifest)
		if err := database.ExecBatch(ctx, tx, batch); err != nil {
			return err
		}
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := modelcore.ValueDigest(manifestJSON)
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,'SUCCEEDED',true)`, revisionID, strings.ToLower(documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest); err != nil {
		return err
	}
	batch := &database.Batch{}
	if err = queueDependencyEdges(batch, revisionID, graph); err != nil {
		return err
	}
	if err = database.ExecBatch(ctx, tx, batch); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE occccad.document_versions SET dependency_snapshot_digest=$1,evaluation_manifest=$2 WHERE id=$3`, dependencyDigest, manifestJSON, revisionID)
	return err
}

func persistInitialTransaction(ctx context.Context, tx database.Tx, workspaceID, documentID, revisionID, actor, modelHash, typeURI string, modelJSON []byte) error {
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
