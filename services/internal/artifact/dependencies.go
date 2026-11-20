package artifact

import (
	"context"
	"fmt"
	"sort"
)

// Fixed statement shapes keep both provider query plans reusable and let the
// SQLite application-query audit compile every statement, including batches.
const artifactBatchSize = 64
const readyObjectsSQL = `SELECT id::text,kind,sha256,storage_backend,object_key,content_type,size_bytes FROM occccad.artifact_objects WHERE state='READY' AND kind=$1 AND sha256 IN ($2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,$46,$47,$48,$49,$50,$51,$52,$53,$54,$55,$56,$57,$58,$59,$60,$61,$62,$63,$64,$65)`
const dependencyParentsSQL = `SELECT parent_id::text,child_id::text FROM occccad.artifact_dependencies WHERE parent_id IN ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,$46,$47,$48,$49,$50,$51,$52,$53,$54,$55,$56,$57,$58,$59,$60,$61,$62,$63,$64)`
const dependencyInsertSQL = `INSERT INTO occccad.artifact_dependencies(parent_id,child_id) VALUES ($1,$2),($3,$4),($5,$6),($7,$8),($9,$10),($11,$12),($13,$14),($15,$16),($17,$18),($19,$20),($21,$22),($23,$24),($25,$26),($27,$28),($29,$30),($31,$32),($33,$34),($35,$36),($37,$38),($39,$40),($41,$42),($43,$44),($45,$46),($47,$48),($49,$50),($51,$52),($53,$54),($55,$56),($57,$58),($59,$60),($61,$62),($63,$64),($65,$66),($67,$68),($69,$70),($71,$72),($73,$74),($75,$76),($77,$78),($79,$80),($81,$82),($83,$84),($85,$86),($87,$88),($89,$90),($91,$92),($93,$94),($95,$96),($97,$98),($99,$100),($101,$102),($103,$104),($105,$106),($107,$108),($109,$110),($111,$112),($113,$114),($115,$116),($117,$118),($119,$120),($121,$122),($123,$124),($125,$126),($127,$128) ON CONFLICT(parent_id,child_id) DO NOTHING`
const reachableObjectsSQL = `WITH RECURSIVE seeds(id) AS (SELECT id FROM occccad.artifact_objects WHERE id IN ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,$46,$47,$48,$49,$50,$51,$52,$53,$54,$55,$56,$57,$58,$59,$60,$61,$62,$63,$64)), reachable(id) AS (SELECT id FROM seeds UNION SELECT d.child_id FROM occccad.artifact_dependencies d JOIN reachable r ON r.id=d.parent_id) SELECT id::text FROM reachable ORDER BY id`

func paddedIDs(ids []any) []any { out := make([]any, artifactBatchSize); copy(out, ids); return out }

// ReadyByDigests resolves immutable storage objects, not business topology IDs.
// Callers still validate the root's ownership, digest and complete closure.
func (s *Service) ReadyByDigests(ctx context.Context, kind Kind, digests []string) (map[string]Object, error) {
	out := map[string]Object{}
	for begin := 0; begin < len(digests); begin += artifactBatchSize {
		end := min(begin+artifactBatchSize, len(digests))
		ids := []any{}
		for _, digest := range digests[begin:end] {
			ids = append(ids, digest)
		}
		args := append([]any{kind}, paddedIDs(ids)...)
		rows, err := s.database.Query(ctx, readyObjectsSQL, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var value Object
			if err = rows.Scan(&value.ID, &value.Kind, &value.SHA256, &value.Backend, &value.Key, &value.ContentType, &value.Size); err != nil {
				rows.Close()
				return nil, err
			}
			out[value.SHA256] = value
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) Dependencies(ctx context.Context, parent string) ([]Object, error) {
	return s.dependencyObjects(ctx, `SELECT o.id::text,o.kind,o.sha256,o.storage_backend,o.object_key,o.content_type,o.size_bytes FROM occccad.artifact_dependencies d JOIN occccad.artifact_objects o ON o.id=d.child_id WHERE d.parent_id=$1 AND o.state='READY' ORDER BY o.id`, parent)
}

// NamingHistoryObjects follows the published predecessor chain. A digest
// filter can fetch just the artifact required by an exact-stage restore.
func (s *Service) NamingHistoryObjects(ctx context.Context, root, digest string) ([]Object, error) {
	return s.dependencyObjects(ctx, `WITH RECURSIVE history(id) AS (
 SELECT child_id FROM occccad.artifact_dependencies WHERE parent_id=$1
 UNION SELECT d.child_id FROM occccad.artifact_dependencies d JOIN history h ON h.id=d.parent_id
 ) SELECT o.id::text,o.kind,o.sha256,o.storage_backend,o.object_key,o.content_type,o.size_bytes FROM history h
 JOIN occccad.artifact_objects o ON o.id=h.id
 WHERE o.state='READY' AND ($2='' OR o.sha256=$2) ORDER BY o.id`, root, digest)
}

// NamingHistorySuffix reads the published chain down to the accepted source
// checkpoint. A different modeling branch naturally runs back to the root.
func (s *Service) NamingHistorySuffix(ctx context.Context, root, checkpoint string) ([]Object, error) {
	if checkpoint == "" {
		return s.NamingHistoryObjects(ctx, root, "")
	}
	return s.dependencyObjects(ctx, `WITH RECURSIVE history(id,sha256) AS (
 SELECT o.id,o.sha256 FROM occccad.artifact_dependencies d JOIN occccad.artifact_objects o ON o.id=d.child_id WHERE d.parent_id=$1
 UNION SELECT o.id,o.sha256 FROM history h JOIN occccad.artifact_dependencies d ON d.parent_id=h.id JOIN occccad.artifact_objects o ON o.id=d.child_id WHERE h.sha256<>$2
 ) SELECT o.id::text,o.kind,o.sha256,o.storage_backend,o.object_key,o.content_type,o.size_bytes FROM history h
 JOIN occccad.artifact_objects o ON o.id=h.id WHERE o.state='READY' AND h.sha256<>$2 ORDER BY o.id`, root, checkpoint)
}

func (s *Service) dependencyObjects(ctx context.Context, query string, args ...any) ([]Object, error) {
	rows, err := s.database.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Object
	for rows.Next() {
		var o Object
		if err = rows.Scan(&o.ID, &o.Kind, &o.SHA256, &o.Backend, &o.Key, &o.ContentType, &o.Size); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// RecordNamingClosure records a validated Merkle chain and its root atomically.
// The Naming decoder has verified every SHA, owner, predecessor and span before
// calling this method. A block's one predecessor is immutable, including when
// concurrent roots share it; a different dependency set is corruption.
func (s *Service) RecordNamingClosure(ctx context.Context, root Object, ordered []Object) error {
	return s.recordNamingClosure(ctx, root, ordered, 0)
}

// The caller has verified the predecessor against an accepted root. Its
// upstream dependency is already published and is shared verbatim.
func (s *Service) RecordNamingContinuation(ctx context.Context, root, predecessor Object, suffix []Object) error {
	if predecessor.Kind != KindNamingHistory || len(suffix) == 0 {
		return fmt.Errorf("invalid naming continuation")
	}
	return s.recordNamingClosure(ctx, root, append([]Object{predecessor}, suffix...), 1)
}

func (s *Service) recordNamingClosure(ctx context.Context, root Object, ordered []Object, beginAt int) error {
	if root.Kind != KindTopologyManifest || len(ordered) == 0 {
		return fmt.Errorf("invalid naming closure root")
	}
	tip := ordered[len(ordered)-1]
	expected := map[string]map[string]bool{root.ID: {tip.ID: true}}
	for i, o := range ordered[beginAt:] {
		if o.Kind != KindNamingHistory || o.ID == root.ID || expected[o.ID] != nil || beginAt == 1 && o.ID == ordered[0].ID {
			return fmt.Errorf("invalid naming closure member")
		}
		expected[o.ID] = map[string]bool{}
		if i+beginAt > 0 {
			expected[o.ID][ordered[i+beginAt-1].ID] = true
		}
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Serializes publication of this root. Shared blocks can only insert the
	// same already-validated predecessor edge, in the same prefix order.
	if _, err = tx.Exec(ctx, `UPDATE occccad.artifact_objects SET verified_at=verified_at WHERE id=$1 AND state='READY'`, root.ID); err != nil {
		return err
	}
	parents := []any{}
	for id := range expected {
		parents = append(parents, id)
	}
	existing := map[string]map[string]bool{}
	for begin := 0; begin < len(parents); begin += artifactBatchSize {
		end := min(begin+artifactBatchSize, len(parents))
		rows, e := tx.Query(ctx, dependencyParentsSQL, paddedIDs(parents[begin:end])...)
		if e != nil {
			return e
		}
		for rows.Next() {
			var parent, child string
			if e = rows.Scan(&parent, &child); e != nil {
				rows.Close()
				return e
			}
			if existing[parent] == nil {
				existing[parent] = map[string]bool{}
			}
			existing[parent][child] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	for parent, children := range existing {
		if len(children) != len(expected[parent]) {
			return fmt.Errorf("immutable artifact dependency mismatch")
		}
		for child := range children {
			if !expected[parent][child] {
				return fmt.Errorf("immutable artifact dependency mismatch")
			}
		}
	}
	var pairs [][2]string
	for i, o := range ordered[beginAt:] {
		if i+beginAt > 0 && len(existing[o.ID]) == 0 {
			pairs = append(pairs, [2]string{o.ID, ordered[i+beginAt-1].ID})
		}
	}
	if len(existing[root.ID]) == 0 {
		pairs = append(pairs, [2]string{root.ID, tip.ID})
	}
	for begin := 0; begin < len(pairs); begin += artifactBatchSize {
		end := min(begin+artifactBatchSize, len(pairs))
		args := make([]any, 0, artifactBatchSize*2)
		for i := 0; i < artifactBatchSize; i++ {
			pair := pairs[begin]
			if begin+i < end {
				pair = pairs[begin+i]
			}
			args = append(args, pair[0], pair[1])
		}
		if _, err = tx.Exec(ctx, dependencyInsertSQL, args...); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ReachableObjects is a read-only closure audit. Physical collection is not
// enabled here: callers must supply/pin Revision, preview and active Job roots.
func (s *Service) ReachableObjects(ctx context.Context, roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	for begin := 0; begin < len(roots); begin += artifactBatchSize {
		end := min(begin+artifactBatchSize, len(roots))
		args := []any{}
		for _, id := range roots[begin:end] {
			args = append(args, id)
		}
		rows, err := s.database.Query(ctx, reachableObjectsSQL, paddedIDs(args)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			seen[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
