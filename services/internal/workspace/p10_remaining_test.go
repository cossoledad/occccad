package workspace

import (
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssemblySolveManifestIsDeterministicAndFreezesSolverPolicy(t *testing.T) {
	bodies := []geometry.AssemblyBody{
		{ID: "body", Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}},
		{ID: "wheel", Pose: geometry.AssemblyPose{Translation: [3]float64{10, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}},
	}
	constraints := []geometry.AssemblyConstraint{{ID: "mount", Kind: "RIGID", FirstBodyID: "body", SecondBodyID: "wheel",
		FixedPose: &geometry.AssemblyPose{Translation: [3]float64{10, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}}}
	first, err := newAssemblySolveManifest("toy-car", "revision-v1", "model-hash", bodies, nil, constraints, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newAssemblySolveManifest("toy-car", "revision-v1", "model-hash", bodies, nil, constraints, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("identical frozen inputs produced different digests: %s != %s", first.Digest, second.Digest)
	}
	reordered, err := newAssemblySolveManifest("toy-car", "revision-v1", "model-hash",
		[]geometry.AssemblyBody{bodies[1], bodies[0]}, nil, constraints, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reordered.Digest != first.Digest {
		t.Fatal("source array order polluted the canonical SolveManifest digest")
	}
	if first.SolverProfile.SchemaVersion != 2 || first.SolverProfile.MaxIterations == 0 || first.SolverBuildPolicy == "" {
		t.Fatalf("solver policy was not frozen: %#v", first)
	}
	bodies[1].Pose.Translation[0] = 11
	changed, err := newAssemblySolveManifest("toy-car", "revision-v1", "model-hash", bodies, nil, constraints, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Digest == first.Digest {
		t.Fatal("changing a frozen occurrence pose did not change the manifest digest")
	}
}

func TestContextVariantIdentitySharesToyCarWheelInputsAcrossOccurrences(t *testing.T) {
	value, err := modelcore.NewQuantity(80, "mm")
	if err != nil {
		t.Fatal(err)
	}
	binding := ContextBinding{ID: "wheel-fl-diameter", Name: "Wheel FL diameter", ContextInputID: "wheel-outer-diameter",
		Publication: PublicationRef{PublicationID: "skeleton-wheel-diameter", ExpectedType: "PARAMETER", CompatibilityVersion: "1"},
		Accepted: ContextBindingResolutionSnapshot{RootProductRevisionID: "toy-car-v1", SourceRevisionID: "skeleton-v2",
			OwningRevisionID: "wheel-v1", ContractDigest: "contract", SourceDigest: "80mm", Status: "CONNECTED"},
		Transform:  InstancePose{Rotation: [4]float64{0, 0, 0, 1}},
		Resolution: PublicationResolution{Status: "CONNECTED", Value: &value, ValueDigest: "80mm"}}
	frontLeft := binding
	frontLeft.SourceInstancePath = testInstancePath("toy-car", "skeleton", "Skeleton.1", "skeleton-part", "skeleton-v2")
	frontLeft.OwningInstancePath = testInstancePath("toy-car", "wheel-fl", "Wheel.FL", "wheel-product", "wheel-v1")
	frontRight := binding
	frontRight.ID, frontRight.Name = "wheel-fr-diameter", "Wheel FR diameter"
	frontRight.Transform.Translation = [3]float64{120, 0, 0}
	frontRight.SourceInstancePath = testInstancePath("toy-car", "skeleton-copy", "Skeleton source", "skeleton-part", "skeleton-v2")
	frontRight.OwningInstancePath = testInstancePath("toy-car", "wheel-fr", "Wheel.FR", "wheel-product", "wheel-v1")

	leftDigest, rightDigest := contextVariantBindingDigest([]ContextBinding{frontLeft}), contextVariantBindingDigest([]ContextBinding{frontRight})
	if leftDigest != rightDigest || contextVariantKey("wheel-v1", leftDigest) != contextVariantKey("wheel-v1", rightDigest) {
		t.Fatal("occurrence names, binding ids, and paths polluted the shared Wheel context variant identity")
	}
	otherValue, _ := modelcore.NewQuantity(82, "mm")
	frontRight.Resolution.Value, frontRight.Resolution.ValueDigest = &otherValue, "82mm"
	if contextVariantBindingDigest([]ContextBinding{frontRight}) == leftDigest {
		t.Fatal("different accepted Product inputs incorrectly shared a context variant")
	}
	cylinder := binding
	cylinder.Publication.ExpectedType = "SURFACE"
	cylinder.Resolution = PublicationResolution{Status: "CONNECTED", GeometryKind: "CYLINDER", Radius: 40,
		GeometryKey: "wheel-cylinder", SourceDigest: "wheel-cylinder-40"}
	cylinderDigest := contextVariantBindingDigest([]ContextBinding{cylinder})
	cylinder.Resolution.Radius = 41
	if contextVariantBindingDigest([]ContextBinding{cylinder}) == cylinderDigest {
		t.Fatal("different exact cylindrical Publication radii incorrectly shared a context variant")
	}
}

func TestToyCarReleaseManifestFreezesFourWheelOccurrencePaths(t *testing.T) {
	paths := []InstancePath{
		testInstancePath("toy-car", "wheel-fl", "Wheel.FL", "wheel-product", "wheel-v1"),
		testInstancePath("toy-car", "wheel-fr", "Wheel.FR", "wheel-product", "wheel-v1"),
		testInstancePath("toy-car", "wheel-rl", "Wheel.RL", "wheel-product", "wheel-v1"),
		testInstancePath("toy-car", "wheel-rr", "Wheel.RR", "wheel-product", "wheel-v1"),
	}
	manifest := ProductReleaseManifest{SchemaVersion: 1, RootProductDocumentID: "toy-car", RootProductRevisionID: "toy-car-v1",
		RootSnapshotDigest: "snapshot", EvaluatorVersion: evaluatorVersion, NamingPolicyDigest: modelcore.TopologyNamingPolicyDigest,
		Gates: []ProductReleaseGate{{Code: "PRODUCT_REFERENCES_CURRENT", Status: "PASSED"}}}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path.Canonical] {
			t.Fatalf("duplicate stable Wheel occurrence path %q", path.Canonical)
		}
		seen[path.Canonical] = true
		manifest.Occurrences = append(manifest.Occurrences, ProductReleaseOccurrence{InstancePath: path,
			DocumentID: "wheel-product", DocumentType: "PRODUCT", RevisionID: "wheel-v1",
			Pose: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, EvaluationManifestDigest: "wheel-evaluation"})
	}
	manifest.Digest = resolvedDigest(func() ProductReleaseManifest { value := manifest; value.Digest = ""; return value }())
	before := manifest.Digest
	workspaceHead := "wheel-v2"
	_ = workspaceHead
	if got := resolvedDigest(func() ProductReleaseManifest { value := manifest; value.Digest = ""; return value }()); got != before {
		t.Fatalf("an external Workspace Head variable changed the frozen release: %s != %s", got, before)
	}
}

func testInstancePath(root, instanceID, name, documentID, revisionID string) InstancePath {
	return appendInstancePath(InstancePath{RootDocumentID: root}, InstancePathSegment{OwnerDocumentID: root,
		OwnerVersionID: "toy-car-v1", InstanceID: instanceID, InstanceName: name,
		ReferencedDocumentID: documentID, ResolvedVersionID: revisionID})
}
