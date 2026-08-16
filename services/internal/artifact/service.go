package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Object struct {
	ID          string `json:"id"`
	Kind        Kind   `json:"kind"`
	SHA256      string `json:"sha256"`
	Backend     string `json:"backend"`
	Key         string `json:"key"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type Service struct {
	database *pgxpool.Pool
	store    Store
}

func NewService(database *pgxpool.Pool, store Store) *Service {
	return &Service{database: database, store: store}
}

func (service *Service) Put(ctx context.Context, kind Kind, contentType string, reader io.Reader) (Object, error) {
	stored, err := service.store.Put(ctx, kind, contentType, reader)
	if err != nil {
		return Object{}, err
	}
	var result Object
	err = service.database.QueryRow(ctx, `INSERT INTO occccad.artifact_objects(
		kind,sha256,storage_backend,object_key,content_type,size_bytes,state,verified_at)
		VALUES($1,$2,'LOCAL',$3,$4,$5,'READY',now())
		ON CONFLICT(kind,sha256) DO UPDATE SET verified_at=now()
		RETURNING id::text,kind,sha256,storage_backend,object_key,content_type,size_bytes`,
		kind, stored.SHA256, stored.Key, contentType, stored.Size).Scan(&result.ID, &result.Kind,
		&result.SHA256, &result.Backend, &result.Key, &result.ContentType, &result.Size)
	return result, err
}

func (service *Service) Open(ctx context.Context, objectID string) (Object, io.ReadCloser, error) {
	var result Object
	err := service.database.QueryRow(ctx, `SELECT id::text,kind,sha256,storage_backend,object_key,
		content_type,size_bytes FROM occccad.artifact_objects WHERE id=$1 AND state='READY'`, objectID).
		Scan(&result.ID, &result.Kind, &result.SHA256, &result.Backend, &result.Key,
			&result.ContentType, &result.Size)
	if err != nil {
		return Object{}, nil, err
	}
	reader, err := service.store.Open(ctx, result.Key)
	return result, reader, err
}

func (service *Service) Get(ctx context.Context, objectID string) (Object, error) {
	var result Object
	err := service.database.QueryRow(ctx, `SELECT id::text,kind,sha256,storage_backend,object_key,
		content_type,size_bytes FROM occccad.artifact_objects WHERE id=$1 AND state='READY'`, objectID).
		Scan(&result.ID, &result.Kind, &result.SHA256, &result.Backend, &result.Key,
			&result.ContentType, &result.Size)
	return result, err
}

// Adopt promotes a Worker-produced staging object through the same hashing and
// metadata path as an HTTP upload. The Worker only knows an opaque object key;
// it never writes PostgreSQL metadata or receives an operating-system path.
func (service *Service) Adopt(ctx context.Context, kind Kind, contentType, stagingKey string) (Object, error) {
	reader, err := service.store.Open(ctx, stagingKey)
	if err != nil {
		return Object{}, err
	}
	result, putErr := service.Put(ctx, kind, contentType, reader)
	closeErr := reader.Close()
	if putErr != nil {
		return Object{}, putErr
	}
	if closeErr != nil {
		return Object{}, closeErr
	}
	if err := service.store.Delete(ctx, stagingKey); err != nil {
		return Object{}, err
	}
	return result, nil
}

func StagingKey(requestID, fileName string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(requestID) + "\x00" + fileName))
	base := filepath.Base(fileName)
	if base == "." || base == "" {
		base = "object.bin"
	}
	return filepath.ToSlash(filepath.Join("exchange", "staging", hex.EncodeToString(digest[:]), base))
}
