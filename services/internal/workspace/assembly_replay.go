package workspace

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/occccad/occccad/internal/debugartifact"
)

type debugArtifactWrite struct {
	documentID, requestID string
	data                  []byte
}

func (service *Service) SetDebugArtifactStore(store *debugartifact.Store) {
	service.debugArtifacts = store
	service.debugArtifactWrites = make(chan debugArtifactWrite, 64)
	go func() {
		for value := range service.debugArtifactWrites {
			archiveCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, err := store.Save(archiveCtx, value.documentID, value.requestID, value.data)
			cancel()
			if err != nil {
				slog.Error("archive assembly replay", "document_id", value.documentID, "request_id", value.requestID, "error", err)
			}
		}
	}()
}

func (service *Service) captureAssemblyReplay(ctx context.Context, documentID, requestID string) func([]byte, error) {
	return func(data []byte, err error) {
		if err != nil {
			slog.ErrorContext(ctx, "archive assembly replay", "document_id", documentID, "request_id", requestID, "error", err)
			return
		}
		if service.debugArtifacts == nil {
			return
		}
		// A bounded single-writer queue keeps diagnostics outside latency and load-bearing state.
		select {
		case service.debugArtifactWrites <- debugArtifactWrite{documentID: documentID, requestID: requestID, data: append([]byte(nil), data...)}:
		default:
			slog.WarnContext(ctx, "drop assembly replay because debug queue is full", "document_id", documentID, "request_id", requestID)
		}
	}
}

func (service *Service) ListAssemblyReplays(ctx context.Context, documentID string) ([]debugartifact.Item, error) {
	if service.debugArtifacts == nil {
		return []debugartifact.Item{}, nil
	}
	return service.debugArtifacts.List(ctx, documentID, 50)
}

func (service *Service) ReadAssemblyReplay(ctx context.Context, documentID, id, requestID string) (debugartifact.Item, []byte, error) {
	if service.debugArtifacts == nil {
		return debugartifact.Item{}, nil, ErrNotFound
	}
	item, data, err := service.debugArtifacts.Read(ctx, documentID, id, requestID)
	if errors.Is(err, debugartifact.ErrNotFound) {
		return item, data, ErrNotFound
	}
	return item, data, err
}
