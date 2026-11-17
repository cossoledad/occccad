//go:build integration

package testsupport

import (
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/database"
	"sync"
	"testing"
)

var propertyDatabases struct {
	sync.Mutex
	values map[*testing.T]*database.Pool
}

// PropertyBREP resolves the authoritative artifact for an explicit test oracle.
// It uses the configured development store; no mass value is read from SQL.
func PropertyBREP(t *testing.T, objectID string) *workerv1.ArtifactReference {
	t.Helper()
	propertyDatabases.Lock()
	if propertyDatabases.values == nil {
		propertyDatabases.values = map[*testing.T]*database.Pool{}
	}
	db := propertyDatabases.values[t]
	if db == nil {
		db = OpenDevelopmentDatabase(t)
		propertyDatabases.values[t] = db
		t.Cleanup(func() { propertyDatabases.Lock(); delete(propertyDatabases.values, t); propertyDatabases.Unlock() })
	}
	propertyDatabases.Unlock()
	ref := &workerv1.ArtifactReference{}
	if err := db.QueryRow(t.Context(), `SELECT storage_backend,object_key,sha256,size_bytes,content_type FROM occccad.artifact_objects WHERE id=$1 AND kind='BREP' AND state='READY'`, objectID).Scan(&ref.Backend, &ref.ObjectKey, &ref.Sha256, &ref.SizeBytes, &ref.ContentType); err != nil {
		t.Fatal(err)
	}
	return ref
}
