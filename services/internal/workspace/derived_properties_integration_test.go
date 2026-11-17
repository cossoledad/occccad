//go:build integration

package workspace

import (
	"testing"

	"github.com/occccad/occccad/internal/testsupport/propertyoracle"
)

func measuredArtifactVolume(t *testing.T, value any) float64 {
	t.Helper()
	var a Artifact
	switch v := value.(type) {
	case Artifact:
		a = v
	case *Artifact:
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
