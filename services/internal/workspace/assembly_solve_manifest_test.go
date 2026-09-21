package workspace

import (
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssemblySolveManifestFreezesCallerOwnedInputs(t *testing.T) {
	for _, field := range []string{"initial guess", "fixed pose", "angle axis", "angle branch", "publication reference", "publication quantity", "persistent resolution", "intent", "affected bodies"} {
		t.Run(field, func(t *testing.T) {
			pose := geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
			guess, fixed := pose, pose
			axis := [3]float64{0, 0, 1}
			branch := geometry.AssemblyAngleBranchState{WrappedAngle: 1, UnwrappedAngle: 1}
			value, err := modelcore.NewQuantity(80, "mm")
			if err != nil {
				t.Fatal(err)
			}
			publication := PublicationRef{PublicationID: "wheel-diameter", ExpectedType: "PARAMETER", CompatibilityVersion: "1"}
			resolution := PublicationResolution{Status: "CONNECTED", Value: &value}
			persistent := ResolutionSnapshot{Result: modelcore.SelectionResolution{Status: modelcore.SelectionResolved, EvidenceDigest: "original"}}
			bodies := []geometry.AssemblyBody{{ID: "wheel", Pose: pose, InitialGuess: &guess}}
			constraints := []geometry.AssemblyConstraint{{ID: "fix", Kind: "FIX", FirstBodyID: "wheel", FixedPose: &fixed},
				{ID: "angle", Kind: "ANGLE", FirstBodyID: "wheel", AngleReferenceDirection: &axis, AngleBranchState: &branch}}
			evidence := []AssemblyResolutionEvidence{{ConstraintID: "fix", Endpoint: "FIRST", DescriptorDigest: "descriptor", PublicationRef: &publication, Publication: &resolution, Persistent: &persistent}}
			intent := &geometry.AssemblySolveIntent{MovingBodyIDs: []string{"wheel"}}
			affected := []string{"wheel"}
			manifest, err := newAssemblySolveManifest("toy-car", "revision-v1", "model-hash", bodies, nil, constraints, intent, affected, evidence)
			if err != nil {
				t.Fatal(err)
			}
			before, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "initial guess":
				guess.Translation[0] = 100
			case "fixed pose":
				fixed.Translation[0] = 200
			case "angle axis":
				axis[0] = 1
			case "angle branch":
				branch.Winding = 2
			case "publication reference":
				publication.PublicationID = "replacement"
			case "publication quantity":
				value.SIValue = 0.082
			case "persistent resolution":
				persistent.Result.EvidenceDigest = "replacement"
			case "intent":
				intent.MovingBodyIDs[0] = "another-wheel"
			case "affected bodies":
				affected[0] = "another-wheel"
			}
			after, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("caller mutation changed the frozen replay input without changing its digest")
			}
		})
	}
}
