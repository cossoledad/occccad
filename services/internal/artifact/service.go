package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"

	"github.com/occccad/occccad/internal/database"
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
	database database.DB
	store    Store
	staging  Store
}

func NewService(database database.DB, store Store, staging ...Store) *Service {
	local := store
	if len(staging) > 0 {
		local = staging[0]
	}
	return &Service{database: database, store: store, staging: local}
}

func (service *Service) Put(ctx context.Context, kind Kind, contentType string, reader io.Reader) (Object, error) {
	stored, err := service.store.Put(ctx, kind, contentType, reader)
	if err != nil {
		return Object{}, err
	}
	var result Object
	err = service.database.QueryRow(ctx, `INSERT INTO occccad.artifact_objects(
		kind,sha256,storage_backend,object_key,content_type,size_bytes,state,verified_at)
		VALUES($1,$2,$6,$3,$4,$5,'READY',now())
		ON CONFLICT(kind,sha256) DO UPDATE SET verified_at=now(),storage_backend=EXCLUDED.storage_backend,object_key=EXCLUDED.object_key
		RETURNING id::text,kind,sha256,storage_backend,object_key,content_type,size_bytes`,
		kind, stored.SHA256, stored.Key, contentType, stored.Size, service.store.Backend()).Scan(&result.ID, &result.Kind,
		&result.SHA256, &result.Backend, &result.Key, &result.ContentType, &result.Size)
	return result, err
}

func (service *Service) Open(ctx context.Context, objectID string) (Object, io.ReadCloser, error) {
	result, err := service.Get(ctx, objectID)
	if err != nil {
		return Object{}, nil, err
	}
	reader, err := service.OpenReadyObject(ctx, result)
	return result, reader, err
}

// OpenReadyObject uses metadata already obtained from a READY object query.
// The caller retains the same expected digest when reading the payload.
func (service *Service) OpenReadyObject(ctx context.Context, object Object) (io.ReadCloser, error) {
	backend := service.store
	if object.Backend != backend.Backend() {
		backend = service.staging
		if object.Backend != backend.Backend() {
			return nil, fmt.Errorf("unconfigured artifact backend %s", object.Backend)
		}
	}
	return backend.Open(ctx, object.Key)
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
	reader, err := service.staging.Open(ctx, stagingKey)
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
	if err := service.staging.Delete(ctx, stagingKey); err != nil {
		return Object{}, err
	}
	return result, nil
}

// AdoptVerifiedBytes checks the complete staging payload before publication
// and returns those same bytes for immediate decoding, including with S3.
func (service *Service) AdoptVerifiedBytes(ctx context.Context, kind Kind, contentType, stagingKey, digest string, size int64) (Object, []byte, error) {
	if size < 0 || size == math.MaxInt64 || len(digest) != sha256.Size*2 {
		return Object{}, nil, fmt.Errorf("invalid staged object reference")
	}
	source, err := service.staging.Open(ctx, stagingKey)
	if err != nil {
		return Object{}, nil, err
	}
	var captured bytes.Buffer
	n, readErr := captured.ReadFrom(io.LimitReader(contextReader{ctx, source}, size+1))
	closeErr := source.Close()
	if readErr != nil {
		return Object{}, nil, readErr
	}
	if closeErr != nil {
		return Object{}, nil, closeErr
	}
	data := captured.Bytes()
	sum := sha256.Sum256(data)
	if n != size || hex.EncodeToString(sum[:]) != digest {
		return Object{}, nil, fmt.Errorf("staged artifact integrity mismatch")
	}
	object, err := service.Put(ctx, kind, contentType, bytes.NewReader(data))
	if err != nil {
		return Object{}, nil, err
	}
	if object.SHA256 != digest || object.Size != size {
		return Object{}, nil, fmt.Errorf("stored artifact integrity mismatch")
	}
	if err := service.staging.Delete(ctx, stagingKey); err != nil {
		return Object{}, nil, err
	}
	return object, data, nil
}

func StagingKey(requestID, fileName string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(requestID) + "\x00" + fileName))
	base := filepath.Base(fileName)
	if base == "." || base == "" {
		base = "object.bin"
	}
	return filepath.ToSlash(filepath.Join("exchange", "staging", hex.EncodeToString(digest[:]), base))
}

// AdoptTransformed decorates a staged result before publication. In particular,
// adding business visualization metadata must not publish a second, unused GLB.
func (service *Service) AdoptTransformed(ctx context.Context, kind Kind, contentType, key, digest string, size int64, transform func(io.Reader, int64) (io.Reader, error)) (Object, error) {
	if size < 0 || size == int64(^uint64(0)>>1) {
		return Object{}, fmt.Errorf("invalid staged object size")
	}
	reader, err := service.staging.Open(ctx, key)
	if err != nil {
		return Object{}, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = reader.Close()
		}
	}()
	hash := sha256.New()
	source := &countedReader{reader: io.TeeReader(io.LimitReader(contextReader{ctx, reader}, size+1), hash)}
	output, err := transform(source, size)
	if err != nil {
		return Object{}, err
	}
	verified := &verifiedTransform{output: output, finish: func() error {
		if _, err := io.Copy(io.Discard, source); err != nil {
			return err
		}
		closeErr := reader.Close()
		closed = true
		if closeErr != nil {
			return closeErr
		}
		if source.count != size || hex.EncodeToString(hash.Sum(nil)) != digest {
			return fmt.Errorf("staged artifact integrity mismatch")
		}
		return ctx.Err()
	}}
	object, err := service.Put(ctx, kind, contentType, verified)
	if err != nil {
		return Object{}, err
	}
	if err = service.staging.Delete(ctx, key); err != nil {
		return Object{}, err
	}
	return object, nil
}

type countedReader struct {
	reader io.Reader
	count  int64
}

func (r *countedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	return n, err
}

// A Store may finalize only after EOF. Verify the entire original object before
// exposing that EOF, including a discarded previous metadata suffix. A corrupt,
// truncated or canceled source never becomes a READY transformed object.
type verifiedTransform struct {
	output  io.Reader
	finish  func() error
	checked bool
}

func (r *verifiedTransform) Read(p []byte) (int, error) {
	n, err := r.output.Read(p)
	if err == io.EOF && !r.checked {
		r.checked = true
		if verifyErr := r.finish(); verifyErr != nil {
			return n, verifyErr
		}
	}
	return n, err
}
