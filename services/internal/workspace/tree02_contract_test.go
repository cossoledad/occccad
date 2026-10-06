package workspace

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestDisjointAddKeepsSketchBodyAndExplicitNewBodyCreatesOne(t *testing.T) {
	model := newPartModel()
	model.Bodies = append(model.Bodies, PartBody{ID: "body-other", Name: "Body.2", Order: 2, Visible: true})
	model.ActiveBodyID = "body-other"
	sketch := testRectangleSketch("sketch-in-first", "XY")
	sketch.BodyID = model.Bodies[0].ID
	model.Features = append(model.Features, sketch)
	normalizePartModel(&model)
	modelJSON, _ := json.Marshal(model)
	request := CommandRequest{Type: "CREATE_SOLID_FEATURE", RequestID: "pad-one", SketchID: sketch.ID,
		Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10}
	typeURI, payload, err := (&Service{}).adaptLegacyCommand(context.Background(), "part", "PART", modelJSON, request)
	if err != nil {
		t.Fatal(err)
	}
	payloadJSON, _ := json.Marshal(payload)
	result, _, err := workspaceCommandRegistry.Apply("PART", modelJSON, modelcore.DomainCommand{CommandID: "pad-one", TypeURI: typeURI, SchemaVersion: 1, Payload: payloadJSON})
	if err != nil {
		t.Fatal(err)
	}
	var after PartModel
	_ = json.Unmarshal(result, &after)
	if len(after.Bodies) != 2 || after.Features[len(after.Features)-1].BodyID != sketch.BodyID {
		t.Fatalf("ADD changed Body ownership: bodies=%d feature=%+v", len(after.Bodies), after.Features[len(after.Features)-1])
	}
	request.RequestID, request.Operation = "pad-new", "NEW_BODY"
	typeURI, payload, err = (&Service{}).adaptLegacyCommand(context.Background(), "part", "PART", result, request)
	if err != nil {
		t.Fatal(err)
	}
	payloadJSON, _ = json.Marshal(payload)
	result, _, err = workspaceCommandRegistry.Apply("PART", result, modelcore.DomainCommand{CommandID: "pad-new", TypeURI: typeURI, SchemaVersion: 1, Payload: payloadJSON})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(result, &after)
	if len(after.Bodies) != 3 || after.Bodies[2].CreatedByFeatureID != after.Features[len(after.Features)-1].ID {
		t.Fatalf("NEW_BODY did not create exactly one owned Body: %+v", after.Bodies)
	}
}

func TestProductSketchDisplayOwnershipUsesSketchBodyNotConsumerBody(t *testing.T) {
	model := newPartModel()
	model.Bodies = append(model.Bodies, PartBody{ID: "body-other", Name: "Body.2", Order: 2, Visible: true})
	sketch := testRectangleSketch("sketch-in-first", "XY")
	sketch.BodyID = "body-main"
	model.Features = append(model.Features, sketch, Feature{ID: "pad-in-second", Type: "PAD", BodyID: "body-other", Profile: sketch.ID})
	if ids := ownedSketchIDs(model, "body-main"); len(ids) != 1 || ids[0] != sketch.ID {
		t.Fatalf("owning Body sketch IDs = %v", ids)
	}
	if ids := ownedSketchIDs(model, "body-other"); len(ids) != 0 {
		t.Fatalf("consumer Body claimed input Sketch: %v", ids)
	}
	// Evaluation includes the cross-Body input for Pad geometry; display ownership
	// still belongs to the Sketch's original Body.
	if other := bodyModel(model, "body-other"); len(other.Features) != 2 {
		t.Fatalf("consumer Body lost its Sketch input: %+v", other.Features)
	} else if visuals := visualizationManifest(other); len(visuals.Primitives) == 0 || visuals.Primitives[0].FeatureID != sketch.ID {
		t.Fatalf("consumer artifact no longer carries the cross-Body Sketch input: %+v", visuals.Primitives)
	}
}

func TestDefinitionVisibilityChangesProjectionAndCompensatesWithoutGeometryChange(t *testing.T) {
	model := newPartModel()
	sketch := testRectangleSketch("sketch-visible", "XY")
	sketch.BodyID = model.Bodies[0].ID
	model.Features = append(model.Features, sketch)
	normalizePartModel(&model)
	before, _ := json.Marshal(model)
	entityID := sketch.Sketch.Entities[0].ID
	payload, _ := json.Marshal(definitionVisibilityPayload{EntityKind: "SKETCH_ENTITY", EntityID: entityID,
		OwnerEntityID: sketch.ID, Visible: false})
	after, changes, err := applyDefinitionVisibility(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var updated PartModel
	_ = json.Unmarshal(after, &updated)
	if visibleOrDefault(updated.Features[0].Sketch.Entities[0].Visible) {
		t.Fatal("entity display state was not saved")
	}
	nodes := partStructureChildren(updated, "document:part", "part", "revision", true)
	var projected *DocumentStructureNode
	var visit func([]DocumentStructureNode)
	visit = func(items []DocumentStructureNode) {
		for i := range items {
			if items[i].Kind == "SKETCH_ENTITY" && items[i].EntityID == entityID {
				projected = &items[i]
			}
			visit(items[i].Children)
		}
	}
	visit(nodes)
	if projected == nil || projected.LocalVisible == nil || *projected.LocalVisible {
		t.Fatal("tree projection did not inherit saved element visibility")
	}
	beforeGraph, _, err := buildPartEvaluation(model, "revision-before", canonicalModelHash(before), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	afterGraph, _, err := buildPartEvaluation(updated, "revision-after", canonicalModelHash(after), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest, _ := beforeGraph.Digest()
	afterDigest, _ := afterGraph.Digest()
	if beforeDigest != afterDigest {
		t.Fatal("display metadata changed geometry dependency inputs")
	}
	if err := changes.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PART", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PART", after, restored)
	if err != nil {
		t.Fatal(err)
	}
	updated = PartModel{}
	_ = json.Unmarshal(undone, &updated)
	if !visibleOrDefault(updated.Features[0].Sketch.Entities[0].Visible) {
		t.Fatal("undo did not restore element visibility")
	}
}

func TestVisibilityCommandsFinalizeEmptyImpactSeedsForPersistence(t *testing.T) {
	part := newPartModel()
	before, _ := json.Marshal(part)
	payload, _ := json.Marshal(definitionVisibilityPayload{EntityKind: "BODY", EntityID: part.Bodies[0].ID, Visible: false})
	_, partChanges, err := workspaceCommandRegistry.Apply("PART", before, modelcore.DomainCommand{
		CommandID: "hide-body", TypeURI: typeDefinitionVisibility, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if partChanges.ImpactSeeds == nil {
		t.Fatal("Part display command would persist SQL NULL impact_seeds")
	}
	path := InstancePath{RootDocumentID: "product", Canonical: "instance", Segments: []InstancePathSegment{{InstanceID: "instance"}}}
	product := ProductModel{}
	before, _ = json.Marshal(product)
	payload, _ = json.Marshal(occurrenceVisibilityPayload{InstancePath: path, EntityKind: "BODY", EntityID: part.Bodies[0].ID, Mode: "HIDE"})
	_, productChanges, err := workspaceCommandRegistry.Apply("PRODUCT", before, modelcore.DomainCommand{
		CommandID: "hide-occurrence", TypeURI: typeOccurrenceVisibility, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if productChanges.ImpactSeeds == nil {
		t.Fatal("Product display command would persist SQL NULL impact_seeds")
	}
}

func TestBodyPublicationUsesStableBodyTargetAndScopedDefaultName(t *testing.T) {
	model := newPartModel()
	model.Bodies = append(model.Bodies, PartBody{ID: "body-two", Name: "Body.2", Visible: true})
	model.Publications = []Publication{{ID: "existing", Name: "Body.1", Type: "BODY",
		Target: PublicationTarget{Kind: "BODY_RESULT", BodyID: "body-two"}, Contract: PublicationContract{GeometryKind: "BODY"}}}
	publication, err := (&Service{}).publicationFromRequest(context.Background(), "part", model,
		CommandRequest{TargetKind: "BODY", TargetID: model.Bodies[0].ID, PublicationType: "BODY"}, "publication-new")
	if err != nil {
		t.Fatal(err)
	}
	if publication.Target.Kind != "BODY_RESULT" || publication.Target.BodyID != model.Bodies[0].ID || publication.Name != "Body.2" {
		t.Fatalf("Body publication selected the wrong target or name: %+v", publication)
	}
	if _, err := (&Service{}).publicationFromRequest(context.Background(), "part", model,
		CommandRequest{TargetKind: "BODY", TargetID: "feature-in-other-body", PublicationType: "BODY"}, "invalid"); err == nil {
		t.Fatal("feature ID must not be accepted as a Body target")
	}
	before, _ := json.Marshal(model)
	removeBody, _ := json.Marshal(bodyCommand{Action: "DELETE", BodyID: "body-two"})
	if _, _, err := applyBodyCommand(before, removeBody); err == nil {
		t.Fatal("published Body was deleted with a live Publication")
	}
	removePublication, _ := json.Marshal(publicationDeletePayload{PublicationID: "existing"})
	after, _, err := applyDeletePublication(before, removePublication)
	if err != nil {
		t.Fatal(err)
	}
	var remaining PartModel
	if err := json.Unmarshal(after, &remaining); err != nil {
		t.Fatal(err)
	}
	if bodyIndex(remaining, "body-two") < 0 || len(remaining.Publications) != 0 {
		t.Fatal("deleting Publication removed its Body target")
	}
}

func TestParameterAliasAndValueEditIsAtomicAndUndoable(t *testing.T) {
	model := newPartModel()
	sketch := testRectangleSketch("sketch-parameter", "XY")
	model.Features = append(model.Features, sketch, Feature{ID: "pad-parameter", Type: "LINEAR_EXTRUDE", BodyID: model.Bodies[0].ID,
		Profile: sketch.ID, Length: 10, Operation: "ADD"})
	normalizePartModel(&model)
	before, _ := json.Marshal(model)
	id := "parameter:pad-parameter:length"
	wrong, _ := modelcore.NewQuantity(1, "deg")
	payload, _ := json.Marshal(editParameterPayload{ParameterID: id, Key: "Depth", Source: modelcore.ValueSource{Literal: &wrong}})
	if changed, _, err := applyEditParameter(before, payload); err == nil || changed != nil {
		t.Fatalf("invalid value partially committed alias: %v", err)
	}
	value, _ := modelcore.NewQuantity(24, "mm")
	payload, _ = json.Marshal(editParameterPayload{ParameterID: id, Key: "Depth", Source: modelcore.ValueSource{Literal: &value}})
	after, changes, err := applyEditParameter(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var updated PartModel
	_ = json.Unmarshal(after, &updated)
	if updated.Parameters[0].ParameterID != id || updated.Parameters[0].Key != "Depth" || updated.Parameters[0].EvaluatedValue.SIValue != value.SIValue {
		t.Fatalf("atomic edit lost identity or value: %+v", updated.Parameters[0])
	}
	if err := changes.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PART", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PART", after, restored)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(undone, &updated)
	if updated.Parameters[0].Key == "Depth" || updated.Parameters[0].Source.Literal.SIValue == value.SIValue {
		t.Fatalf("undo did not restore alias and source: %+v", updated.Parameters[0])
	}
}

func TestParameterDisplayPathFollowsNamesAndRequiredParameterRejectsDelete(t *testing.T) {
	model := newPartModel()
	sketch := testRectangleSketch("sketch-display", "XY")
	model.Features = append(model.Features, sketch, Feature{ID: "pad-display", Type: "LINEAR_EXTRUDE", Name: "凸台.2",
		BodyID: model.Bodies[0].ID, Profile: sketch.ID, Length: 12, Operation: "ADD"})
	normalizePartModel(&model)
	var parameter modelcore.ParameterDefinition
	for _, item := range model.Parameters {
		if item.OwnerFeatureID == "pad-display" {
			parameter = item
			break
		}
	}
	if parameter.ParameterID == "" {
		t.Fatal("feature parameter metadata missing")
	}
	first := parameterPresentation(model, parameter)
	if first.QualifiedDisplayPath != "Body.1\\凸台.2\\第一限制\\长度" || first.DisplayAlias != "length_1" {
		t.Fatalf("unexpected readable parameter identity: %+v", first)
	}
	model.Bodies[0].Name = "零件几何体.1"
	model.Features[1].Name = "凸台.3"
	second := parameterPresentation(model, parameter)
	if second.QualifiedDisplayPath != "零件几何体.1\\凸台.3\\第一限制\\长度" || second.ParameterID != first.ParameterID {
		t.Fatalf("renaming changed stable parameter identity or left stale path: %+v", second)
	}
	before, _ := json.Marshal(model)
	payload, _ := json.Marshal(deleteParameterPayload{ParameterID: parameter.ParameterID})
	if _, _, err := applyDeleteParameter(before, payload); err == nil {
		t.Fatal("required feature parameter was independently deleted")
	}
	renamePayload, _ := json.Marshal(renameFeaturePayload{FeatureID: "pad-display", Name: "凸台.4"})
	renamedJSON, changes, err := applyRenameFeature(before, renamePayload)
	if err != nil {
		t.Fatal(err)
	}
	var renamed PartModel
	if err := json.Unmarshal(renamedJSON, &renamed); err != nil {
		t.Fatal(err)
	}
	if got := parameterPresentation(renamed, parameter); got.ParameterID != parameter.ParameterID || got.QualifiedDisplayPath != "零件几何体.1\\凸台.4\\第一限制\\长度" {
		t.Fatalf("Feature rename did not update parameter display path: %+v", got)
	}
	if err := changes.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PART", renamedJSON, changes)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PART", renamedJSON, restored)
	if err != nil {
		t.Fatal(err)
	}
	renamed = PartModel{}
	if err := json.Unmarshal(undone, &renamed); err != nil {
		t.Fatal(err)
	}
	if got := parameterPresentation(renamed, parameter); got.QualifiedDisplayPath != second.QualifiedDisplayPath {
		t.Fatalf("Feature rename undo did not restore parameter path: %+v", got)
	}
}

func TestOccurrenceVisibilityKeepsOverridesSeparateAndRestoresInheritance(t *testing.T) {
	first := InstancePath{RootDocumentID: "product", Canonical: "first", Segments: []InstancePathSegment{{InstanceID: "first"}}}
	second := InstancePath{RootDocumentID: "product", Canonical: "second", Segments: []InstancePathSegment{{InstanceID: "second"}}}
	model := ProductModel{Instances: []ProductInstance{{ID: "first"}, {ID: "second"}}}
	before, _ := json.Marshal(model)
	payload, _ := json.Marshal(occurrenceVisibilityPayload{InstancePath: first, EntityKind: "BODY", EntityID: "body-one", Mode: "SHOW"})
	after, changes, err := applyOccurrenceVisibility(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var updated ProductModel
	_ = json.Unmarshal(after, &updated)
	if len(updated.VisibilityOverrides) != 1 || updated.VisibilityOverrides[0].InstancePath.Canonical != "first" {
		t.Fatal("occurrence override leaked")
	}
	root := DocumentStructureNode{Kind: "PRODUCT", Children: []DocumentStructureNode{
		{Kind: "BODY", EntityID: "body-one", InstancePath: &first},
		{Kind: "BODY", EntityID: "body-one", InstancePath: &second},
	}}
	applyOccurrenceVisibilityProjection(&root, updated.VisibilityOverrides)
	if root.Children[0].VisibilityMode != "SHOW" || root.Children[1].VisibilityMode != "" {
		t.Fatal("projection mixed repeated Part occurrences")
	}
	if err := changes.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PRODUCT", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PRODUCT", after, restored)
	if err != nil {
		t.Fatal(err)
	}
	updated = ProductModel{}
	_ = json.Unmarshal(undone, &updated)
	if len(updated.VisibilityOverrides) != 0 {
		t.Fatal("undo retained occurrence override")
	}
	payload, _ = json.Marshal(occurrenceVisibilityPayload{InstancePath: first, EntityKind: "BODY", EntityID: "body-one", Mode: "INHERIT"})
	inherited, _, err := applyOccurrenceVisibility(after, payload)
	if err != nil {
		t.Fatal(err)
	}
	updated = ProductModel{}
	_ = json.Unmarshal(inherited, &updated)
	if len(updated.VisibilityOverrides) != 0 {
		t.Fatal("restore inheritance retained override")
	}
}

func TestPublicationDependencyScanKeepsOwnerAndTargetSeparate(t *testing.T) {
	part := PartModel{ContextReferences: []ContextReference{{ID: "ref", SourceDocumentID: "source",
		Publication: PublicationRef{PublicationID: "publication"}}}}
	raw, _ := json.Marshal(part)
	if use, err := publicationUseInModel("PART", raw, "source", "publication"); err != nil || use != "ContextReference ref" {
		t.Fatalf("Part dependency not found: %q %v", use, err)
	}
	if use, err := publicationUseInModel("PART", raw, "other", "publication"); err != nil || use != "" {
		t.Fatalf("another source was treated as the same Publication: %q %v", use, err)
	}
	path := InstancePath{Segments: []InstancePathSegment{{ReferencedDocumentID: "source"}}}
	product := ProductModel{ContextBindings: []ContextBinding{{ID: "binding", SourceInstancePath: path,
		Publication: PublicationRef{PublicationID: "publication"}}}}
	raw, _ = json.Marshal(product)
	if use, err := publicationUseInModel("PRODUCT", raw, "source", "publication"); err != nil || use != "ContextBinding binding" {
		t.Fatalf("Product dependency not found: %q %v", use, err)
	}
	product.ContextBindings = nil
	product.Publications = []ProductPublication{{ID: "forward", Target: ProductPublicationTarget{InstancePath: path, PublicationID: "publication"}}}
	raw, _ = json.Marshal(product)
	if use, err := publicationUseInModel("PRODUCT", raw, "source", "publication"); err != nil || use != "Product Publication forward" {
		t.Fatalf("forwarding dependency not found: %q %v", use, err)
	}
}

func TestProductPublicationRedirectPreservesIdentityNameAndContract(t *testing.T) {
	current := ProductPublication{ID: "forward", Name: "Body.1", Type: "BODY", CompatibilityVersion: "1",
		Target:   ProductPublicationTarget{InstancePath: InstancePath{Canonical: "first"}, PublicationID: "source-one"},
		Contract: PublicationContract{GeometryKind: "BODY"}}
	model := ProductModel{Publications: []ProductPublication{current}}
	before, _ := json.Marshal(model)
	replacement := current
	replacement.Name = "wrong generated name"
	replacement.Target = ProductPublicationTarget{InstancePath: InstancePath{Canonical: "second"}, PublicationID: "source-two"}
	payload, _ := json.Marshal(productPublicationPayload{Publication: replacement})
	after, changes, err := applyRedirectProductPublication(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var updated ProductModel
	if err := json.Unmarshal(after, &updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Publications[0]; got.ID != current.ID || got.Name != current.Name || got.Target.PublicationID != "source-two" {
		t.Fatalf("Product redirect changed identity or failed to replace target: %+v", got)
	}
	if err := changes.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PRODUCT", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PRODUCT", after, restored)
	if err != nil {
		t.Fatal(err)
	}
	updated = ProductModel{}
	if err := json.Unmarshal(undone, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Publications[0].Target.PublicationID != "source-one" {
		t.Fatal("redirect Undo did not restore source")
	}
	replacement.Contract.GeometryKind = "SURFACE"
	payload, _ = json.Marshal(productPublicationPayload{Publication: replacement})
	if _, _, err := applyRedirectProductPublication(before, payload); err == nil {
		t.Fatal("incompatible Product redirect accepted")
	}
}

func TestUserParameterCreateDeleteAndUndo(t *testing.T) {
	model := newPartModel()
	before, _ := json.Marshal(model)
	typeURI, payload, err := (&Service{}).adaptLegacyCommand(context.Background(), "part", "PART", before,
		CommandRequest{Type: "CREATE_PARAMETER", RequestID: "create-user-parameter", Value: 25, Unit: "mm"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload)
	after, created, err := workspaceCommandRegistry.Apply("PART", before, modelcore.DomainCommand{CommandID: "create-user-parameter", TypeURI: typeURI, Payload: encoded, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	var updated PartModel
	if err := json.Unmarshal(after, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Parameters) != 1 || updated.Parameters[0].Lifecycle != "USER" || updated.Parameters[0].Label != "Parameter.1" {
		t.Fatalf("user parameter creation lost display identity: %+v", updated.Parameters)
	}
	if err := created.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PART", after, created)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := created.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PART", after, restored)
	if err != nil {
		t.Fatal(err)
	}
	updated = PartModel{}
	_ = json.Unmarshal(undone, &updated)
	if len(updated.Parameters) != 0 {
		t.Fatal("create Undo retained user parameter")
	}
	remove, _ := json.Marshal(deleteParameterPayload{ParameterID: payload.(createUserParameterPayload).Parameter.ParameterID})
	deleted, deletion, err := applyDeleteParameter(after, remove)
	if err != nil {
		t.Fatal(err)
	}
	updated = PartModel{}
	_ = json.Unmarshal(deleted, &updated)
	if len(updated.Parameters) != 0 {
		t.Fatal("delete retained user parameter")
	}
	if err := deletion.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err = modelValues("PART", deleted, deletion)
	if err != nil {
		t.Fatal(err)
	}
	restored, err = deletion.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err = applyModelValues("PART", deleted, restored)
	if err != nil {
		t.Fatal(err)
	}
	updated = PartModel{}
	_ = json.Unmarshal(undone, &updated)
	if len(updated.Parameters) != 1 || updated.Parameters[0].ParameterID != payload.(createUserParameterPayload).Parameter.ParameterID {
		t.Fatal("delete Undo did not restore the same user parameter")
	}
}
