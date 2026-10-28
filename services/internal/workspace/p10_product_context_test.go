package workspace

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestP10ScopedNamesUseVersionedNFKCCaseFoldProfile(t *testing.T) {
	t.Parallel()
	if nameNormalizationProfile != "nfkc-casefold-v1" {
		t.Fatalf("unexpected naming profile %q", nameNormalizationProfile)
	}
	if scopedNameKey("  Ｆａｃｅ１ ") != scopedNameKey("face1") {
		t.Fatal("compatibility-equivalent names must share one scope key")
	}
	if err := ensureUniqueScopedName("Ｆａｃｅ１", []string{"Face1"}, ""); err == nil {
		t.Fatal("NFKC/case-fold duplicate must be rejected")
	}
}

func TestP10RenameInstancePreservesIdentityAndRejectsSiblingCollision(t *testing.T) {
	t.Parallel()
	before := ProductModel{Instances: []ProductInstance{{ID: "stable-a", Name: "Body.1"}, {ID: "stable-b", Name: "Wheel.1"}}}
	raw, _ := json.Marshal(before)
	payload, _ := json.Marshal(renameInstancePayload{InstanceID: "stable-a", Name: "Chassis.1"})
	next, changes, err := applyRenameInstance(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var renamed ProductModel
	if err := json.Unmarshal(next, &renamed); err != nil {
		t.Fatal(err)
	}
	if renamed.Instances[0].ID != "stable-a" || renamed.Instances[0].Name != "Chassis.1" {
		t.Fatalf("rename changed stable identity: %+v", renamed.Instances[0])
	}
	if len(changes.Changes) != 1 || changes.Changes[0].Target.SlotID != "instance.name" {
		t.Fatalf("rename ChangeSet = %+v", changes)
	}
	collision, _ := json.Marshal(renameInstancePayload{InstanceID: "stable-a", Name: "ｗｈｅｅｌ．１"})
	if _, _, err := applyRenameInstance(raw, collision); err == nil {
		t.Fatal("normalized sibling collision must be rejected")
	}
}

func TestP10ContextInputIsPartOwnedAndCompensatable(t *testing.T) {
	t.Parallel()
	before, _ := json.Marshal(newPartModel())
	input := ContextInput{ID: "context-input-1", Name: "MountPlaneInput", Type: "PLANE", Required: true,
		Target: ContextInputTarget{Kind: "DATUM", TargetID: "datum-xy"}, Contract: PublicationContract{GeometryKind: "PLANE"}}
	payload, _ := json.Marshal(contextInputPayload{Input: input})
	next, changes, err := applyCreateContextInput(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelValues("PART", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := changes.Compensate(current)
	if err != nil {
		t.Fatal(err)
	}
	reverted, err := applyModelValues("PART", next, desired)
	if err != nil {
		t.Fatal(err)
	}
	var model PartModel
	if err := json.Unmarshal(reverted, &model); err != nil {
		t.Fatal(err)
	}
	if len(model.ContextInputs) != 0 {
		t.Fatalf("undo retained ContextInput: %+v", model.ContextInputs)
	}
}

func TestP10ProductEvaluationAcceptsTypedNestedEndpoints(t *testing.T) {
	t.Parallel()
	ownerPath := InstancePath{RootDocumentID: "root"}
	ownerPath = appendInstancePath(ownerPath, InstancePathSegment{OwnerDocumentID: "root", OwnerVersionID: "root-r1",
		InstanceID: "subproduct", InstanceName: "WheelAssembly.1", ReferencedDocumentID: "wheel-product", ResolvedVersionID: "wheel-r1"})
	ownerPath = appendInstancePath(ownerPath, InstancePathSegment{OwnerDocumentID: "wheel-product", OwnerVersionID: "wheel-r1",
		InstanceID: "rim", InstanceName: "Rim.1", ReferencedDocumentID: "rim-part", ResolvedVersionID: "rim-r1"})
	sourcePath := InstancePath{RootDocumentID: "root"}
	sourcePath = appendInstancePath(sourcePath, InstancePathSegment{OwnerDocumentID: "root", OwnerVersionID: "root-r1",
		InstanceID: "skeleton", InstanceName: "Skeleton.1", ReferencedDocumentID: "skeleton-part", ResolvedVersionID: "skeleton-r1"})
	model := ProductModel{Instances: []ProductInstance{
		{ID: "subproduct", Name: "WheelAssembly.1"}, {ID: "skeleton", Name: "Skeleton.1"},
	}, Publications: []ProductPublication{{ID: "mount-axis", Name: "MountAxis", Type: "AXIS",
		Target: ProductPublicationTarget{InstancePath: ownerPath, PublicationID: "rim-axis"}}},
		ContextBindings: []ContextBinding{{ID: "binding-1", Name: "RimDiameterBinding", OwningInstancePath: ownerPath,
			ContextInputID: "rim-diameter-input", SourceInstancePath: sourcePath,
			Publication: PublicationRef{PublicationID: "wheel-diameter", ExpectedType: "PARAMETER"}}}}
	graph, _, err := buildProductEvaluation(model, "root-r2", "model-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 6 || !strings.Contains(ownerPath.Canonical, "/") {
		t.Fatalf("nested Product graph/path not retained: nodes=%d path=%q", len(graph.Nodes), ownerPath.Canonical)
	}
}

func TestP10ContextCatalogRejectsDependencyCycle(t *testing.T) {
	t.Parallel()
	edges := map[string][]string{"skeleton": {"body"}, "body": {"wheel"}}
	if !contextDependencyReachable(edges, "skeleton", "wheel") {
		t.Fatal("transitive dependency must be reachable")
	}
	if contextDependencyReachable(edges, "wheel", "skeleton") {
		t.Fatal("reverse dependency must remain available")
	}
}

func TestP10TypedPathRejectsDisplayOrSegmentSpoofing(t *testing.T) {
	t.Parallel()
	path := InstancePath{RootDocumentID: "root", Canonical: "instance-a", Segments: []InstancePathSegment{{InstanceID: "instance-b"}}}
	if err := validateNonRootInstancePath(path, "root"); err == nil {
		t.Fatal("canonical identity must be derived from typed stable segments")
	}
	path.Canonical = "instance-b"
	path.Display = "arbitrary presentation"
	if err := validateNonRootInstancePath(path, "root"); err != nil {
		t.Fatalf("display name must not participate in path identity: %v", err)
	}
	resolved := path
	resolved.Segments = append([]InstancePathSegment(nil), path.Segments...)
	resolved.Segments[0].InstanceName = "renamed presentation"
	if err := validateResolvedInstancePath(path, resolved); err != nil {
		t.Fatalf("resolved path compared a display name: %v", err)
	}
	resolved.Segments[0].ReferencedDocumentID = "spoofed-document"
	if err := validateResolvedInstancePath(path, resolved); err == nil {
		t.Fatal("resolved path accepted a spoofed document identity")
	}
}

func TestP10NestedProductBindingsRebaseIntoRootContext(t *testing.T) {
	t.Parallel()
	prefix := InstancePath{RootDocumentID: "root"}
	prefix = appendInstancePath(prefix, InstancePathSegment{InstanceID: "wheel-product", InstanceName: "Wheel.1"})
	local := InstancePath{RootDocumentID: "wheel-document"}
	local = appendInstancePath(local, InstancePathSegment{InstanceID: "rim", InstanceName: "Rim.1"})
	rebased := rebaseInstancePath("root", prefix, local)
	if rebased.RootDocumentID != "root" || rebased.Canonical != "wheel-product/rim" || rebased.Display != "Wheel.1/Rim.1" {
		t.Fatalf("unexpected rebased path: %+v", rebased)
	}
}

func TestP10ForwardedPublicationUsesNestedRigidPose(t *testing.T) {
	t.Parallel()
	publication := Publication{Type: "AXIS", Resolution: PublicationResolution{Status: "CONNECTED",
		Origin: [3]float64{1, 0, 0}, ZDirection: [3]float64{1, 0, 0}, SourceDigest: "source"}}
	// +90 degrees around Z, followed by a translation.
	pose := InstancePose{Translation: [3]float64{10, 20, 0}, Rotation: [4]float64{0, 0, 0.7071067811865476, 0.7071067811865476}}
	forwarded := publicationThroughRigidPose(publication, pose)
	if math.Abs(forwarded.Resolution.Origin[0]-10) > 1e-9 || math.Abs(forwarded.Resolution.Origin[1]-21) > 1e-9 ||
		math.Abs(forwarded.Resolution.ZDirection[0]) > 1e-9 || math.Abs(forwarded.Resolution.ZDirection[1]-1) > 1e-9 {
		t.Fatalf("forwarded descriptor did not enter Product frame: %+v", forwarded.Resolution)
	}
	if forwarded.Resolution.SourceDigest == "source" {
		t.Fatal("forwarded source digest must include occurrence pose")
	}
}

func TestForwardedNativeSketchFrameProjectsEachRigidLevelOnce(t *testing.T) {
	first := InstancePose{Translation: [3]float64{10, 20, 3}, Rotation: [4]float64{0, 0, math.Sin(.3), math.Cos(.3)}}
	second := InstancePose{Translation: [3]float64{-7, 4, 11}, Rotation: [4]float64{math.Sin(.2), 0, 0, math.Cos(.2)}}
	receiver := InstancePose{Translation: [3]float64{2, -9, 1}, Rotation: [4]float64{0, math.Sin(.1), 0, math.Cos(.1)}}
	for _, kind := range []string{"POINT", "LINE", "CIRCLE", "ARC"} {
		t.Run(kind, func(t *testing.T) {
			native := &NativeSketchPublication{SketchID: "sketch", EntityID: "element", Origin: [3]float64{1, 2, 5}, XDirection: [3]float64{1, 0, 0}, Normal: [3]float64{0, 0, 1}, Geometry: SketchExternalGeometrySnapshot{Kind: kind, Point: &SketchPoint2{X: 4, Y: 6}, Start: &SketchPoint2{X: -3, Y: 2}, End: &SketchPoint2{X: 9, Y: 7}, Center: &SketchPoint2{X: 2, Y: 3}, Radius: 8, StartAngle: .4, EndAngle: 4.2}}
			p := Publication{Type: "CURVE", Resolution: PublicationResolution{Status: "CONNECTED", NativeSketch: native, SourceDigest: resolvedDigest(native)}}
			descriptor, err := nativePublicationDescriptor(p.Resolution)
			if err != nil {
				t.Fatal(err)
			}
			p.Resolution = descriptor
			original := resolvedDigest(p)
			forwarded := publicationThroughRigidPose(publicationThroughRigidPose(p, first), second)
			actual, err := publicationProjectionSource(forwarded.Resolution, receiver)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := publicationProjectionSource(p.Resolution, composeInstancePose(receiver, composeInstancePose(second, first)))
			if err != nil {
				t.Fatal(err)
			}
			ownerProjection, err := publicationProjectionSource(forwarded.Resolution, InstancePose{Rotation: [4]float64{0, 0, 0, 1}})
			if err != nil {
				t.Fatal(err)
			}
			for _, pair := range [][2][3]float64{{forwarded.Resolution.Origin, ownerProjection.Origin}, {actual.Origin, expected.Origin}, {actual.Direction, expected.Direction}, {actual.XDirection, expected.XDirection}} {
				for i := 0; i < 3; i++ {
					if math.Abs(pair[0][i]-pair[1][i]) > 1e-9 {
						t.Fatalf("%s projected in wrong frame: actual=%+v expected=%+v", kind, actual, expected)
					}
				}
			}
			if original != resolvedDigest(p) || forwarded.Resolution.NativeSketch == native {
				t.Fatal("forwarding mutated/shared source frame")
			}
			if forwarded.Resolution.SourceDigest == p.Resolution.SourceDigest {
				t.Fatal("pose not represented in digest")
			}
			if kind == "ARC" && (*actual.ParameterStart != .4 || *actual.ParameterEnd != 4.2) {
				t.Fatal("arc domain changed")
			}
		})
	}
}

func TestContextBindingBoundaryUsesDirectReceiverAndPublicModuleInterface(t *testing.T) {
	direct := InstancePath{Segments: []InstancePathSegment{{InstanceID: "module"}}}
	nested := InstancePath{Segments: []InstancePathSegment{{InstanceID: "module"}, {InstanceID: "private-layout"}}}
	if err := validateContextBindingBoundary(direct, direct); err != nil {
		t.Fatal(err)
	}
	if err := validateContextBindingBoundary(direct, nested); err == nil || !strings.Contains(err.Error(), "CONTEXT_PUBLIC_INTERFACE_REQUIRED") {
		t.Fatal("private layout bypass", err)
	}
	if err := validateContextBindingBoundary(nested, direct); err == nil || !strings.Contains(err.Error(), "CONTEXT_DIRECT_RECEIVER_REQUIRED") {
		t.Fatal("nested receiver", err)
	}
}

func TestUpdateReferencesCompensatesAcceptedBindingSnapshots(t *testing.T) {
	before := ProductModel{ContextBindings: []ContextBinding{{ID: "binding", ContextInputID: "local-port", Accepted: ContextBindingResolutionSnapshot{SourceRevisionID: "module-v1"}, Resolution: PublicationResolution{SourceDigest: "geometry-v1"}}}}
	next := before
	next.ContextBindings = append([]ContextBinding(nil), before.ContextBindings...)
	next.ContextBindings[0].Accepted.SourceRevisionID = "module-v2"
	next.ContextBindings[0].Resolution.SourceDigest = "geometry-v2"
	raw, _ := json.Marshal(before)
	payload, _ := json.Marshal(updateReferencesPayload{Model: next})
	result, changes, err := applyUpdateReferences(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 1 || changes.Changes[0].Target.SlotID != "context-binding.entity" {
		t.Fatalf("binding absent from update history: %+v", changes)
	}
	current, err := modelValues("PRODUCT", result, changes)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := changes.Compensate(current)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", result, desired)
	if err != nil {
		t.Fatal(err)
	}
	var reverted ProductModel
	if err := json.Unmarshal(restored, &reverted); err != nil {
		t.Fatal(err)
	}
	if resolvedDigest(reverted.ContextBindings) != resolvedDigest(before.ContextBindings) {
		t.Fatal("undo did not restore exact accepted inputs")
	}
}
