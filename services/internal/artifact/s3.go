package artifact

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Store keeps only temporary hashing spools on disk. Multipart upload memory is
// bounded independently of file size. Cancellation aborts only this upload ID.
type S3Store struct {
	client                     *minio.Client
	bucket, temporaryDirectory string
}
type S3Config struct {
	Endpoint, AccessKey, SecretKey, Bucket, Region, TemporaryDirectory string
	Secure                                                             bool
}

func NewS3Store(ctx context.Context, c S3Config) (*S3Store, error) {
	if c.Endpoint == "" || c.Bucket == "" || c.AccessKey == "" || c.SecretKey == "" {
		return nil, fmt.Errorf("S3 endpoint, bucket and credentials are required")
	}
	client, err := minio.New(c.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Secure: c.Secure, Region: c.Region, BucketLookup: minio.BucketLookupPath, MaxRetries: 1})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(ctx, c.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("S3 bucket %q does not exist; provision it before startup", c.Bucket)
	}
	if err := os.MkdirAll(c.TemporaryDirectory, 0700); err != nil {
		return nil, err
	}
	return &S3Store{client, c.Bucket, c.TemporaryDirectory}, nil
}
func (*S3Store) Backend() string { return "S3" }
func (s *S3Store) Put(ctx context.Context, kind Kind, contentType string, r io.Reader) (StoredObject, error) {
	dir, err := os.MkdirTemp(s.temporaryDirectory, "upload-*")
	if err != nil {
		return StoredObject{}, err
	}
	defer os.RemoveAll(dir)
	spool, err := NewLocalStore(dir)
	if err != nil {
		return StoredObject{}, err
	}
	object, err := spool.Put(ctx, kind, contentType, r)
	if err != nil {
		return StoredObject{}, err
	}
	file, err := spool.Open(ctx, object.Key)
	if err != nil {
		return StoredObject{}, err
	}
	defer file.Close()
	if err := s.upload(ctx, object, file, contentType); err != nil {
		return StoredObject{}, err
	}
	return object, nil
}
func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := object.Stat(); err != nil {
		object.Close()
		return nil, err
	}
	return object, nil
}
func (s *S3Store) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// Use SDK multipart primitives so abort has its own deadline even when the
// caller is canceled. No resume, retries or deletion of other upload sessions.
func (s *S3Store) upload(ctx context.Context, object StoredObject, reader io.Reader, contentType string) error {
	options := minio.PutObjectOptions{ContentType: contentType, PartSize: 16 << 20, NumThreads: 1}
	if object.Size <= 16<<20 {
		info, err := s.client.PutObject(ctx, s.bucket, object.Key, reader, object.Size, options)
		if err != nil {
			return err
		}
		if info.Size != object.Size {
			return fmt.Errorf("S3 upload size mismatch")
		}
		return nil
	}
	core := minio.Core{Client: s.client}
	uploadID, err := core.NewMultipartUpload(ctx, s.bucket, object.Key, options)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := core.AbortMultipartUpload(cleanup, s.bucket, object.Key, uploadID); err != nil {
				slog.Warn("abort incomplete artifact upload", "object_key", object.Key, "upload_id", uploadID, "error", err)
			}
		}
	}()
	partSize := max(int64(16<<20), (object.Size+9999)/10000)
	var parts []minio.CompletePart
	for offset := int64(0); offset < object.Size; offset += partSize {
		size := min(partSize, object.Size-offset)
		part, err := core.PutObjectPart(ctx, s.bucket, object.Key, uploadID, len(parts)+1, io.LimitReader(reader, size), size, minio.PutObjectPartOptions{})
		if err != nil {
			return err
		}
		if part.Size != size {
			return fmt.Errorf("S3 part size mismatch")
		}
		parts = append(parts, minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	if _, err := core.CompleteMultipartUpload(ctx, s.bucket, object.Key, uploadID, parts, options); err != nil {
		return err
	}
	complete = true
	return nil
}
