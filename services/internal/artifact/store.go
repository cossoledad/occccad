package artifact

import (
	"context"
	"io"
)

type Kind string

const (
	KindBREP           Kind = "BREP"
	KindGLB            Kind = "GLB"
	KindExchangeSource Kind = "EXCHANGE_SOURCE"
	KindExchangeExport Kind = "EXCHANGE_EXPORT"
	KindThumbnail      Kind = "THUMBNAIL"
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
	Put(context.Context, Kind, string, io.Reader) (StoredObject, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
