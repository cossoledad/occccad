//go:build integration

package control

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"google.golang.org/grpc"
)

// Native processes may be private to a test, while their data and artifact
// backend are the same resources as the running development application.
func openDevelopmentGeometry(t *testing.T, pool *GeometryPool, options ...grpc.DialOption) (*geometry.Client, error) {
	t.Helper()
	store, local, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		return nil, err
	}
	options = append([]grpc.DialOption{geometry.ArtifactStaging(store, local)}, options...)
	return geometry.Open(serveGeometry(t, pool), options...)
}

func openDevelopmentDiagnostics(t *testing.T) *debugartifact.Store {
	t.Helper()
	root := filepath.Join(testsupport.Environment(t).LogDirectory, "debug", "cad-diagnostics")
	store, err := debugartifact.NewStore(root, 50, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
