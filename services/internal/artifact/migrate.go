package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
