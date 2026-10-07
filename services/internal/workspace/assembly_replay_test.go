package workspace

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

func TestDiagnosticReplayActivationKeepsDefinitionStates(t *testing.T) {
	definitions := []AssemblyConstraint{
		{DefinitionVersion: 2, ID: "accepted", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "body", Kind: "BODY"}, EvaluationStatus: "VERIFIED", Mode: "DRIVING"},
		{DefinitionVersion: 2, ID: "pending", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "body", Kind: "BODY"}, EvaluationStatus: "NOT_UPDATED", Mode: "DRIVING"},
		{DefinitionVersion: 2, ID: "disabled", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "body", Kind: "BODY"}, EvaluationStatus: "VERIFIED", Mode: "DRIVING", Suppressed: true},
		{DefinitionVersion: 2, ID: "measurement", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "body", Kind: "BODY"}, EvaluationStatus: "VERIFIED", Mode: "MEASURED"},
		{DefinitionVersion: 2, ID: "broken", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "body", Kind: "BODY"}, EvaluationStatus: "BROKEN", Mode: "DRIVING"},
	}
	primitives := []geometry.AssemblyConstraint{}
	for _, definition := range definitions {
		primitives = append(primitives, geometry.AssemblyConstraint{ID: definition.ID, Kind: "FIX", FirstBodyID: "body", Mode: definition.Mode})
	}
	manifest, err := newAssemblySolveManifest("product", "revision", "model", []geometry.AssemblyBody{{ID: "body", Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}}, nil, primitives, nil, nil, nil, definitions)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(manifest.Definitions)
	if err != nil {
		t.Fatal(err)
	}
	for _, includePending := range []bool{false, true, false} {
		if err := prepareDiagnosticManifest(&manifest, includePending); err != nil {
			t.Fatal(err)
		}
		expected := map[string]string{"accepted": "DRIVING", "pending": "SUPPRESSED", "disabled": "SUPPRESSED", "measurement": "MEASURED", "broken": "SUPPRESSED"}
		if includePending {
			expected["pending"] = "DRIVING"
		}
		for _, constraint := range manifest.Constraints {
			if constraint.Mode != expected[constraint.ID] {
				t.Fatal("incorrect replay activation", constraint.ID, constraint.Mode)
			}
		}
		after, err := json.Marshal(manifest.Definitions)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("replay activation overwrote saved states")
		}
		if manifest.Digest != assemblyManifestDigest(manifest) {
			t.Fatal("diagnostic digest not frozen")
		}
	}
}
