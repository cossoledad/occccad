package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OpenConfigured leaves backend selection at the composition root. Domain
// services depend only on Store; additional providers implement that interface.
func OpenConfigured(ctx context.Context, directory string) (Store, *LocalStore, error) {
	local, err := NewLocalStore(directory)
	if err != nil {
		return nil, nil, err
	}
	backend := strings.ToUpper(strings.TrimSpace(os.Getenv("OCCCCAD_ARTIFACT_BACKEND")))
	switch backend {
	case "", "LOCAL":
		return local, local, nil
	case "S3":
		secure := true
		if value := os.Getenv("OCCCCAD_S3_SECURE"); value != "" {
			secure, err = strconv.ParseBool(value)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid OCCCCAD_S3_SECURE")
			}
		}
		store, err := NewS3Store(ctx, S3Config{Endpoint: os.Getenv("OCCCCAD_S3_ENDPOINT"), AccessKey: os.Getenv("OCCCCAD_S3_ACCESS_KEY"), SecretKey: os.Getenv("OCCCCAD_S3_SECRET_KEY"), Bucket: os.Getenv("OCCCCAD_S3_BUCKET"), Region: os.Getenv("OCCCCAD_S3_REGION"), Secure: secure, TemporaryDirectory: filepath.Join(local.Root(), "artifacts", ".s3-spool")})
		return store, local, err
	default:
		return nil, nil, fmt.Errorf("unknown artifact backend %q", backend)
	}
}
