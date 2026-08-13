package workspace

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test uses an explicitly supplied disposable database. It validates the
// SQL state fold behind both API capabilities and Undo/Redo target selection.
func TestHistoryCapabilitiesAcrossTwoUndoAndRedoSteps(t *testing.T) {
	url := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OCCCCAD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	service := &Service{database: pool}
	documentID := uuid.NewString()
	revisionID := uuid.NewString()
	workspaceID := uuid.NewString()
	actor := "00000000-0000-7000-8000-000000000001"
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.documents(id,document_type,name,owner_user_id) VALUES($1,'PRODUCT',$2,$3)`, documentID, "history-test-"+documentID, actor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		_, _ = pool.Exec(cleanup, `DELETE FROM occccad.workspaces WHERE document_id=$1`, documentID)
		_, _ = pool.Exec(cleanup, `UPDATE occccad.documents SET head_version_id=NULL WHERE id=$1`, documentID)
		_, _ = pool.Exec(cleanup, `DELETE FROM occccad.document_versions WHERE document_id=$1`, documentID)
		_, _ = pool.Exec(cleanup, `DELETE FROM occccad.documents WHERE id=$1`, documentID)
	})
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,sequence,model_json,state,model_hash) VALUES($1,$2,1,'{"instances":[]}','READY','initial')`, revisionID, documentID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE occccad.documents SET head_version_id=$1 WHERE id=$2`, revisionID, documentID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.workspaces(id,document_id,name,head_revision_id,head_sequence,base_revision_id) VALUES($1,$2,'main',$3,1,$3)`, workspaceID, documentID, revisionID); err != nil {
		t.Fatal(err)
	}

	rootSketch := uuid.NewString()
	rootPad := uuid.NewString()
	insertRoot := func(id string, sequence int) {
		t.Helper()
		if _, insertErr := pool.Exec(ctx, `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status) VALUES($1,$2,$3,$4,$5,$5,'DOMAIN','COMMITTED')`, id, workspaceID, sequence, actor, "request-"+id); insertErr != nil {
			t.Fatal(insertErr)
		}
	}
	insertRoot(rootSketch, 2)
	insertRoot(rootPad, 3)
	assert := func(wantUndo, wantRedo bool) {
		t.Helper()
		undo, redo, capErr := service.historyCapabilities(ctx, documentID, actor)
		if capErr != nil {
			t.Fatal(capErr)
		}
		if undo != wantUndo || redo != wantRedo {
			t.Fatalf("capabilities got undo=%v redo=%v, want undo=%v redo=%v", undo, redo, wantUndo, wantRedo)
		}
	}
	insertAction := func(kind string, sequence int, root, consumed string) string {
		t.Helper()
		id := uuid.NewString()
		var revert, reapply any
		if kind == "REVERT" {
			revert = root
		} else {
			reapply = consumed
		}
		if _, insertErr := pool.Exec(ctx, `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,root_transaction_id,reverts_transaction_id,reapplies_transaction_id) VALUES($1,$2,$3,$4,$5,$5,$6,'COMMITTED',$7,$8,$9)`, id, workspaceID, sequence, actor, fmt.Sprintf("request-%d", sequence), kind, root, revert, reapply); insertErr != nil {
			t.Fatal(insertErr)
		}
		return id
	}

	assert(true, false)
	revertPad := insertAction("REVERT", 4, rootPad, "")
	assert(true, true)
	revertSketch := insertAction("REVERT", 5, rootSketch, "")
	assert(false, true)
	insertAction("REAPPLY", 6, rootSketch, revertSketch)
	assert(true, true)
	insertAction("REAPPLY", 7, rootPad, revertPad)
	assert(true, false)
}
