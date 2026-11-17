//go:build integration

package control

import (
	"testing"

	"github.com/occccad/occccad/internal/testsupport/propertyoracle"
	"github.com/occccad/occccad/internal/workspace"
)

func measuredArtifactVolume(t *testing.T, value any) float64 {
	t.Helper()
	var a workspace.Artifact
	switch v := value.(type) {
	case workspace.Artifact:
		a = v
	case *workspace.Artifact:
		if v == nil {
			t.Fatal("missing artifact")
		}
		a = *v
	default:
		t.Fatalf("expected geometry artifact, got %T", value)
	}
	if a.GeometryDomain != "" && a.GeometryDomain != "SOLID_SET" {
		return 0
	}
	ref, ok := a.Representations["BREP"]
	if !ok {
		return 0
	}
	return propertyoracle.Volume(t, a.GeometryID, ref.ObjectID)
}

// A numerical test witness from an explicit property query. It is deliberately
// separate from PersistentSelection.CreationEvidence and never modifies tokens.
func queriedTopologyCenter(t *testing.T, p workspace.TopologyElementProperties) [3]float64 {
	t.Helper()
	if p.Point != nil {
		return *p.Point
	}
	for _, name := range []string{"snapCenter", "centroid"} {
		if value, ok := p.Properties[name].([3]float64); ok {
			return value
		}
	}
	t.Fatalf("missing explicitly queried center: %s/%d %+v", p.Kind, p.LocalID, p.Properties)
	return [3]float64{}
}

func topologyCentersNear(a, b [3]float64) bool {
	for i := range a {
		d := a[i] - b[i]
		if d > 1e-7 || d < -1e-7 {
			return false
		}
	}
	return true
}
