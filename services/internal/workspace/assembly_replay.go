package workspace

import (
	"context"
	"log/slog"
	"time"
)

func (service *Service) captureAssemblyReplay(ctx context.Context, documentID, requestID string) func([]byte, error) {
	return func(data []byte, err error) {
		if err == nil {
			// A canceled preview must retain the input that actually reached the worker.
			archiveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_, err = service.database.Exec(archiveCtx, `INSERT INTO occccad.assembly_replays(id,document_id,request_id,replay) VALUES($1,$2,$3,$4)`, newID("solve"), documentID, requestID, data)
		}
		if err != nil {
			slog.ErrorContext(ctx, "archive assembly replay", "document_id", documentID, "request_id", requestID, "error", err)
		}
	}
}
