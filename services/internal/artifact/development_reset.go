package artifact

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
)

// DevelopmentResetter is an optional administrative capability, deliberately
// separate from normal Store operations. Callers must stop writers first.
type DevelopmentResetter interface {
	DevelopmentResetTarget() string
	ResetDevelopment(context.Context) error
}

func (s *S3Store) DevelopmentResetTarget() string {
	return "S3 bucket " + s.bucket + " at " + s.client.EndpointURL().Redacted()
}

// ResetDevelopment empties this configured bucket, including old versions,
// delete markers and abandoned multipart uploads; it preserves the bucket.
func (s *S3Store) ResetDevelopment(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	batch := make(chan minio.ObjectInfo, 1000)
	flush := func() error {
		close(batch)
		var failure error
		for result := range s.client.RemoveObjects(ctx, s.bucket, batch, minio.RemoveObjectsOptions{}) {
			if result.Err != nil && failure == nil {
				failure = fmt.Errorf("delete S3 object %q version %q: %w", result.ObjectName, result.VersionID, result.Err)
			}
		}
		batch = make(chan minio.ObjectInfo, 1000)
		return failure
	}
	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			return fmt.Errorf("list S3 versions: %w", object.Err)
		}
		batch <- object
		if len(batch) == cap(batch) {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if len(batch) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}
	core := minio.Core{Client: s.client}
	for upload := range s.client.ListIncompleteUploads(ctx, s.bucket, "", true) {
		if upload.Err != nil {
			return fmt.Errorf("list S3 multipart uploads: %w", upload.Err)
		}
		if err := core.AbortMultipartUpload(ctx, s.bucket, upload.Key, upload.UploadID); err != nil {
			return fmt.Errorf("abort S3 multipart upload: %w", err)
		}
	}
	// Do not report success if another writer repopulated the target or cleanup
	// was rejected by object retention. Cross-store reset is not transactional.
	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			return object.Err
		}
		return fmt.Errorf("S3 bucket still contains objects; stop writers before reset")
	}
	for upload := range s.client.ListIncompleteUploads(ctx, s.bucket, "", true) {
		if upload.Err != nil {
			return upload.Err
		}
		return fmt.Errorf("S3 bucket still contains multipart uploads; stop writers before reset")
	}
	return ctx.Err()
}
