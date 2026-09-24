package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
)

// MigrateLocal copies and verifies before switching each metadata row. Identity,
// references and local originals are preserved. Rerunning skips migrated rows.
// Never hold database rows/transactions while uploading to remote storage.
func (s *Service) MigrateLocal(ctx context.Context) (int, error) {
	if s.store.Backend() == "LOCAL" {
		return 0, fmt.Errorf("migration requires a non-local target")
	}
	total := 0
	for {
		rows, err := s.database.Query(ctx, `SELECT id::text,kind,sha256,object_key,content_type,size_bytes FROM occccad.artifact_objects WHERE storage_backend='LOCAL' AND state='READY' ORDER BY id LIMIT 64`)
		if err != nil {
			return total, err
		}
		var batch []Object
		for rows.Next() {
			var o Object
			if err = rows.Scan(&o.ID, &o.Kind, &o.SHA256, &o.Key, &o.ContentType, &o.Size); err != nil {
				rows.Close()
				return total, err
			}
			batch = append(batch, o)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return total, err
		}
		if len(batch) == 0 {
			return total, nil
		}
		for _, o := range batch {
			reader, err := s.staging.Open(ctx, o.Key)
			if err != nil {
				return total, fmt.Errorf("migrate %s: %w", o.ID, err)
			}
			stored, err := s.store.Put(ctx, o.Kind, o.ContentType, reader)
			closeErr := reader.Close()
			if err != nil {
				return total, err
			}
			if closeErr != nil {
				return total, closeErr
			}
			if stored.SHA256 != o.SHA256 || stored.Size != o.Size {
				return total, fmt.Errorf("migrate %s: source integrity mismatch", o.ID)
			}
			result, err := s.database.Exec(ctx, `UPDATE occccad.artifact_objects SET storage_backend=$1,object_key=$2,verified_at=now() WHERE id=$3 AND storage_backend='LOCAL' AND object_key=$4 AND sha256=$5 AND state='READY'`, s.store.Backend(), stored.Key, o.ID, o.Key, o.SHA256)
			if err != nil {
				return total, err
			}
			total += int(result.RowsAffected())
		}
	}
}

// MigrateEmbedded materializes legacy bytea payloads through bounded SQL chunks.
// Existing columns remain as non-authoritative backups; no Revision is changed.
func (s *Service) MigrateEmbedded(ctx context.Context) (int, error) {
	total := 0
	for _, field := range []struct {
		data, id    string
		kind        Kind
		contentType string
	}{
		{"brep_data", "brep_object_id", KindBREP, "application/vnd.opencascade.brep"},
		{"glb_data", "glb_object_id", KindGLB, "model/gltf-binary"},
		{"topology_manifest_data", "topology_manifest_object_id", KindTopologyManifest, "application/vnd.occccad.topology-manifest.v1+protobuf"},
	} {
		// Column names are closed constants above, never caller input.
		for {
			var key string
			err := s.database.QueryRow(ctx, `SELECT geometry_key FROM occccad.geometry_artifacts WHERE `+field.id+` IS NULL AND `+field.data+` IS NOT NULL ORDER BY geometry_key LIMIT 1`).Scan(&key)
			if errors.Is(err, pgx.ErrNoRows) {
				break
			}
			if err != nil {
				return total, err
			}
			reader := &legacyPayloadReader{ctx: ctx, service: s, key: key, column: field.data}
			object, err := s.Put(ctx, field.kind, field.contentType, reader)
			if err != nil {
				return total, err
			}
			result, err := s.database.Exec(ctx, `UPDATE occccad.geometry_artifacts SET `+field.id+`=$1 WHERE geometry_key=$2 AND `+field.id+` IS NULL`, object.ID, key)
			if err != nil {
				return total, err
			}
			total += int(result.RowsAffected())
		}
	}
	return total, nil
}

type legacyPayloadReader struct {
	ctx         context.Context
	service     *Service
	key, column string
	offset      int
	buffer      []byte
	done        bool
}

func (r *legacyPayloadReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.buffer) == 0 {
		if r.done {
			return 0, io.EOF
		}
		if err := r.service.database.QueryRow(r.ctx, `SELECT substring(`+r.column+` from $2 for 1048576) FROM occccad.geometry_artifacts WHERE geometry_key=$1`, r.key, r.offset+1).Scan(&r.buffer); err != nil {
			return 0, err
		}
		r.offset += len(r.buffer)
		r.done = len(r.buffer) < 1048576
		if len(r.buffer) == 0 {
			return 0, io.EOF
		}
	}
	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

// VerifyTarget checks the persisted references against actual object contents.
func (s *Service) VerifyTarget(ctx context.Context) (int, error) {
	total := 0
	after := ""
	for {
		rows, err := s.database.Query(ctx, `SELECT id::text,storage_backend,object_key,sha256,size_bytes FROM occccad.artifact_objects WHERE state='READY' AND id::text>$1 ORDER BY id::text LIMIT 64`, after)
		if err != nil {
			return total, err
		}
		var batch []Object
		for rows.Next() {
			var o Object
			if err = rows.Scan(&o.ID, &o.Backend, &o.Key, &o.SHA256, &o.Size); err != nil {
				rows.Close()
				return total, err
			}
			batch = append(batch, o)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return total, err
		}
		if len(batch) == 0 {
			return total, nil
		}
		for _, o := range batch {
			if o.Backend != s.store.Backend() {
				return total, fmt.Errorf("artifact %s still uses %s", o.ID, o.Backend)
			}
			reader, err := s.store.Open(ctx, o.Key)
			if err != nil {
				return total, err
			}
			hash := sha256.New()
			size, err := io.Copy(hash, reader)
			closeErr := reader.Close()
			if err != nil {
				return total, err
			}
			if closeErr != nil {
				return total, closeErr
			}
			if size != o.Size || hex.EncodeToString(hash.Sum(nil)) != o.SHA256 {
				return total, fmt.Errorf("artifact %s integrity mismatch", o.ID)
			}
			total++
			after = o.ID
		}
	}
}
