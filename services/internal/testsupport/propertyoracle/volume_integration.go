//go:build integration

// Package propertyoracle performs explicit exact measurements for assertions.
// Feature execution and performance timing never depend on stored mass values.
package propertyoracle

import (
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"sync"
	"testing"
)

var propertyWorkers struct {
	sync.Mutex
	values map[*testing.T]*geometry.Client
}

func Volume(t *testing.T, geometryID, objectID string) float64 {
	t.Helper()
	frozen := testsupport.PropertyBREP(t, objectID)
	propertyWorkers.Lock()
	if propertyWorkers.values == nil {
		propertyWorkers.values = map[*testing.T]*geometry.Client{}
	}
	client := propertyWorkers.values[t]
	if client == nil {
		var close func()
		var err error
		client, close, err = geometry.OpenDevelopmentWorker(t.Context())
		if err != nil {
			propertyWorkers.Unlock()
			t.Fatal(err)
		}
		propertyWorkers.values[t] = client
		t.Cleanup(func() { propertyWorkers.Lock(); delete(propertyWorkers.values, t); propertyWorkers.Unlock(); close() })
	}
	propertyWorkers.Unlock()
	input := geometry.AnalysisGeometry{ID: "property", GeometryID: geometryID, BRep: geometry.ArtifactReference{Backend: frozen.Backend, ObjectKey: frozen.ObjectKey, SHA256: frozen.Sha256, Size: int64(frozen.SizeBytes), ContentType: frozen.ContentType}, Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}
	result, err := client.MeasureGeometry(t.Context(), "explicit-volume-query", "ITEM", []geometry.AnalysisGeometry{input}, [][]geometry.MeasurementElement{{{InputID: "property", Kind: "BODY", Mode: "GEOMETRY"}}})
	if err != nil {
		t.Fatal(err)
	}
	volume, ok := result.Values["volumeMm3"]
	if !result.Complete || !ok {
		t.Fatalf("missing exact volume: %+v", result)
	}
	return volume
}
