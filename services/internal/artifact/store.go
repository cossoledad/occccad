package artifact

import (
	"context"
	"io"
)

type Kind string

const (
	KindBREP             Kind = "BREP"
	KindGLB              Kind = "GLB"
	KindTopologyManifest Kind = "TOPOLOGY_MANIFEST"
	KindExchangeSource   Kind = "EXCHANGE_SOURCE"
	KindExchangeExport   Kind = "EXCHANGE_EXPORT"
	KindThumbnail        Kind = "THUMBNAIL"
)

type StoredObject struct {
	Key         string
	SHA256      string
	Size        int64
	ContentType string
}

// Store is the storage boundary used by CAD services. Object keys are relative,
// backend-independent identifiers; callers never receive an operating-system path.
type Store interface {
	Backend() string
	Put(context.Context, Kind, string, io.Reader) (StoredObject, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

// contextReader prevents continued spooling after cancellation.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
