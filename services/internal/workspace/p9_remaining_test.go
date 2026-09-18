package workspace

import (
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func frozenExternal(revision string, value float64) modelcore.ExternalParameterRef {
	quantity := modelcore.Quantity{SIValue: value, Dimension: modelcore.LengthDimension}
	return modelcore.ExternalParameterRef{SourceDocumentID: "skeleton", Revision: modelcore.ReferenceSelector{Mode: "FOLLOW_HEAD", RevisionID: revision},
		PublicationID: "publication-master-length", ExpectedType: modelcore.ValueQuantity,
		ExpectedDimension: modelcore.LengthDimension, ContractVersion: "1.0.0", ResolvedRevisionID: revision,
		ResolvedValue: quantity, ResolvedValueDigest: resolvedDigest(quantity),
		ResolutionSnapshot: modelcore.ReferenceResolutionSnapshot{SourceRevisionID: revision,
			PublicationID: "publication-master-length", ContractDigest: "contract", ValueDigest: resolvedDigest(quantity), Status: "CONNECTED"}}
}

func TestPartReferenceUpdateIsOneFrozenUndoableChangeSet(t *testing.T) {
	before := newPartModel()
	old := frozenExternal("skeleton-r1", 0.040)
	before.Parameters = []modelcore.ParameterDefinition{{ParameterID: "parameter:driven", Key: "driven", Label: "Driven",
		ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Role: "INPUT",
		Source: modelcore.ValueSource{External: &old}}}
	after := before
	after.Parameters = append([]modelcore.ParameterDefinition(nil), before.Parameters...)
	next := frozenExternal("skeleton-r2", 0.055)
	after.Parameters[0].Source = modelcore.ValueSource{External: &next}
	beforeJSON, _ := json.Marshal(before)
	payloadJSON, _ := json.Marshal(updatePartReferencesPayload{Model: after})
	nextJSON, set, err := applyUpdatePartReferences(beforeJSON, payloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Changes) != 1 || set.Changes[0].Target.SlotID != "parameter.source" {
		t.Fatalf("unexpected update ChangeSet: %#v", set)
	}
	if err := set.Finalize(); err != nil {
		t.Fatal(err)
	}
	beforeValues, err := modelValues("PART", beforeJSON, set)
	if err != nil {
		t.Fatal(err)
	}
	revertedJSON, err := applyModelValues("PART", nextJSON, beforeValues)
	if err != nil {
		t.Fatal(err)
	}
	var reverted PartModel
	if err := json.Unmarshal(revertedJSON, &reverted); err != nil {
		t.Fatal(err)
	}
	if got := reverted.Parameters[0].Source.External.ResolvedRevisionID; got != "skeleton-r1" {
		t.Fatalf("undo restored revision %q", got)
	}
}

func TestAssemblyPublicationDescriptorAndCompatibleReplacement(t *testing.T) {
	publication := Publication{ID: "publication-mount-plane", Type: "PLANE", CompatibilityVersion: "1.0.0",
		Target:     PublicationTarget{Kind: "DATUM", DatumID: "replacement-plane"},
		Contract:   PublicationContract{GeometryKind: "PLANE", Symmetry: "NORMAL_UNORIENTED"},
		Resolution: PublicationResolution{Status: "CONNECTED", GeometryKind: "PLANE", Symmetry: "NORMAL_UNORIENTED"}}
	reference := AssemblyGeometryRef{InstanceID: "component", PublicationRef: &PublicationRef{PublicationID: publication.ID,
		ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}}
	if !publicationReferenceCompatible(*reference.PublicationRef, publication) {
		t.Fatal("compatible replacement was rejected")
	}
	if err := applyPublicationDescriptor(&reference, publication); err != nil {
		t.Fatal(err)
	}
	if reference.Kind != "PLANE" || reference.GeometryID != "replacement-plane" {
		t.Fatalf("Publication descriptor was not adapted: %#v", reference)
	}
	publication.Contract.GeometryKind = "AXIS"
	publication.Type = "AXIS"
	if publicationReferenceCompatible(*reference.PublicationRef, publication) {
		t.Fatal("incompatible replacement was accepted")
	}
}

func TestReplaceInstanceHistoryRestoresDocumentAndRevision(t *testing.T) {
	before := ProductModel{Instances: []ProductInstance{{ID: "component", Name: "Component.1",
		ReferencedDocumentID: "old-part", ReferencedVersionID: "old-r1", ReferenceMode: "FOLLOW_HEAD"}}}
	after := before
	after.Instances = append([]ProductInstance(nil), before.Instances...)
	after.Instances[0].ReferencedDocumentID, after.Instances[0].ReferencedVersionID = "replacement-part", "replacement-r1"
	beforeJSON, _ := json.Marshal(before)
	payloadJSON, _ := json.Marshal(replaceInstancePayload{InstanceID: "component", ReferencedDocumentID: "replacement-part",
		ReferencedVersionID: "replacement-r1", Model: after})
	nextJSON, set, err := applyReplaceInstance(beforeJSON, payloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	set, err = reconcilePersistedChanges("PRODUCT", beforeJSON, nextJSON, set)
	if err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PRODUCT", beforeJSON, set)
	if err != nil {
		t.Fatal(err)
	}
	revertedJSON, err := applyModelValues("PRODUCT", nextJSON, values)
	if err != nil {
		t.Fatal(err)
	}
	var reverted ProductModel
	if err := json.Unmarshal(revertedJSON, &reverted); err != nil {
		t.Fatal(err)
	}
	if reverted.Instances[0].ReferencedDocumentID != "old-part" || reverted.Instances[0].ReferencedVersionID != "old-r1" {
		t.Fatalf("replace undo did not restore reference: %#v", reverted.Instances[0])
	}
}

func TestProductPublicationAndContextReferenceParticipateInGraphsAndHistory(t *testing.T) {
	child := Publication{ID: "publication-axis", Name: "Main Axis", Type: "AXIS", CompatibilityVersion: "1.0.0",
		Contract:   PublicationContract{GeometryKind: "AXIS", Symmetry: "DIRECTION_UNORIENTED"},
		Resolution: PublicationResolution{Status: "CONNECTED", ZDirection: [3]float64{0, 0, 1}}}
	instance := ProductInstance{ID: "skeleton-occurrence", Name: "Skeleton.1", ReferencedDocumentID: "skeleton",
		ReferencedVersionID: "skeleton-r1", Rotation: [4]float64{0, 0, 0, 1}}
	forwarded := productPublicationFromChild(instance, child, "product-publication-axis", "Assembly Axis", "assembly interface")
	product := ProductModel{Instances: []ProductInstance{instance}, Publications: []ProductPublication{forwarded}}
	if _, _, err := buildProductEvaluation(product, "product-r1", "hash", nil, nil); err != nil {
		t.Fatal(err)
	}

	part := newPartModel()
	context := ContextReference{ID: "context-axis", Name: "Skeleton Axis", OwningWorkspace: "main",
		SourceDocumentID: "skeleton", ReferenceMode: "FOLLOW_HEAD", ResolvedRevisionID: "skeleton-r1",
		Publication: PublicationRef{PublicationID: child.ID, ExpectedType: "AXIS", CompatibilityVersion: "1.0.0"},
		Transform:   InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, Resolution: child.Resolution}
	if err := materializeContextReference(&part, &context, ""); err != nil {
		t.Fatal(err)
	}
	if len(part.ContextReferences) != 1 || len(part.DatumAxes) != 1 || context.LocalTargetID == "" {
		t.Fatalf("context axis was not materialized: %#v", part)
	}
	if _, _, err := buildPartEvaluation(part, "consumer-r1", "hash", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestP9SkeletonTwoConsumersExplicitUpdateAndIsolateContract(t *testing.T) {
	makeConsumer := func(id string) PartModel {
		model := newPartModel()
		external := frozenExternal("skeleton-r1", 0.040)
		model.Parameters = []modelcore.ParameterDefinition{{ParameterID: "parameter:" + id, Key: "master_length",
			ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Role: "INPUT",
			Source: modelcore.ValueSource{External: &external}}}
		return model
	}
	left, right := makeConsumer("left"), makeConsumer("right")
	for _, model := range []*PartModel{&left, &right} {
		updated := frozenExternal("skeleton-r2", 0.065)
		model.Parameters[0].Source.External = &updated
		if err := validateAndResolvePartParameters(model); err != nil {
			t.Fatal(err)
		}
		if model.Parameters[0].EvaluatedValue == nil || model.Parameters[0].EvaluatedValue.SIValue != 0.065 {
			t.Fatalf("consumer did not accept the same frozen source snapshot: %#v", model.Parameters[0])
		}
	}
	context := ContextReference{ID: "context-master", ReferenceMode: "ISOLATED", LocalTargetID: "parameter:left"}
	left.ContextReferences = []ContextReference{context}
	value := left.Parameters[0].Source.External.ResolvedValue
	left.Parameters[0].Source = modelcore.ValueSource{Literal: &value}
	newSource := frozenExternal("skeleton-r3", 0.080)
	if left.Parameters[0].Source.External != nil || left.Parameters[0].Source.Literal.SIValue == newSource.ResolvedValue.SIValue {
		t.Fatal("isolated consumer still follows the source")
	}
}

func TestContextCurveMaterializesAsSketchExternalAndIsolatesLocally(t *testing.T) {
	model := newPartModel()
	sketch := testRectangleSketch("consumer-sketch", "XY")
	model.Features = append(model.Features, sketch)
	selection := testSelection()
	selection.ExpectedType = modelcore.PersistentTopologyEdge
	reference := ContextReference{ID: "context-curve", Name: "Master Curve", OwningWorkspace: "main",
		SourceDocumentID: "skeleton", ReferenceMode: "FOLLOW_HEAD", ResolvedRevisionID: "skeleton-r1",
		LocalTargetID: sketch.ID, Publication: PublicationRef{PublicationID: "publication-curve", ExpectedType: "CURVE",
			CompatibilityVersion: "1.0.0", PersistentSelection: &selection, SelectionSourceVersionID: "skeleton-r1"},
		Resolution: PublicationResolution{Status: "CONNECTED", GeometryKey: "skeleton-geometry"}}
	if err := materializeContextReference(&model, &reference, ""); err != nil {
		t.Fatal(err)
	}
	external := &model.Features[len(model.Features)-1].Sketch.ExternalGeometry[0]
	if external.SourceDocumentID != "skeleton" || external.ContextReferenceID != reference.ID || external.Status != "PENDING" {
		t.Fatalf("context curve did not use cross-document external geometry: %#v", external)
	}
	external.Status, external.GeometryKind = "CONNECTED", "LINE"
	external.Snapshot = &SketchExternalGeometrySnapshot{Kind: "LINE", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 10, Y: 0}}
	if err := isolateContextCurve(&model, reference); err != nil {
		t.Fatal(err)
	}
	resolvedSketch := model.Features[len(model.Features)-1].Sketch
	if len(resolvedSketch.ExternalGeometry) != 0 || len(resolvedSketch.Entities) == 0 || resolvedSketch.Entities[len(resolvedSketch.Entities)-1].ID != "context-external-context-curve" {
		t.Fatalf("isolated curve was not converted to ordinary sketch geometry: %#v", resolvedSketch)
	}
}
