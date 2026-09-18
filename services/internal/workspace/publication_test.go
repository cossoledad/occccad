package workspace

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestPublicationCRUDRedirectKeepsStableIdentityAndContract(t *testing.T) {
	model := newPartModel()
	publication := Publication{ID: "publication-plane", Name: "Mounting Plane", Type: "PLANE",
		SemanticPurpose: "assembly support", CompatibilityVersion: "1.0.0",
		Target:     PublicationTarget{Kind: "DATUM", DatumID: "datum-xy"},
		Contract:   PublicationContract{GeometryKind: "PLANE", Symmetry: "NORMAL_UNORIENTED"},
		Resolution: PublicationResolution{Status: "PENDING"}}
	modelJSON, _ := json.Marshal(model)
	payload, _ := json.Marshal(publicationPayload{Publication: publication})
	afterCreate, changes, err := applyCreatePublication(modelJSON, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 1 || changes.Changes[0].Target.SlotID != "publication.entity" {
		t.Fatalf("publication create changes = %#v", changes)
	}

	redirected := publication
	redirected.Target.DatumID = "datum-xz"
	redirected.Name = "must not replace public name"
	redirectPayload, _ := json.Marshal(publicationPayload{Publication: redirected})
	afterRedirect, _, err := applyRedirectPublication(afterCreate, redirectPayload)
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	_ = json.Unmarshal(afterRedirect, &result)
	if len(result.Publications) != 1 || result.Publications[0].ID != publication.ID ||
		result.Publications[0].Name != publication.Name || result.Publications[0].Target.DatumID != "datum-xz" {
		t.Fatalf("redirect did not preserve the public identity and metadata: %#v", result.Publications)
	}

	incompatible := redirected
	incompatible.Type = "AXIS"
	incompatible.Contract.GeometryKind = "AXIS"
	incompatiblePayload, _ := json.Marshal(publicationPayload{Publication: incompatible})
	if _, _, err = applyRedirectPublication(afterRedirect, incompatiblePayload); err == nil ||
		!strings.Contains(err.Error(), "PUBLICATION_CONTRACT_INCOMPATIBLE") {
		t.Fatalf("incompatible redirect error = %v", err)
	}
}

func TestTopologyPublicationContractsCoverExtrudeBooleanFaceAndEdge(t *testing.T) {
	face := testSelection()
	edge := typedTestSelection(modelcore.PersistentTopologyEdge,
		"END_BOUNDARY_FROM_PROFILE_EDGE/line-1", "LINE")
	model := newPartModel()
	model.Publications = []Publication{
		{ID: "publication-face", Name: "Mounting Surface", Type: "SURFACE", CompatibilityVersion: "1.0.0",
			Target:   PublicationTarget{Kind: "TOPOLOGY", SourceVersionID: "revision-extrude", PersistentSelection: &face},
			Contract: PublicationContract{GeometryKind: "SURFACE", Symmetry: "NONE"}, Resolution: PublicationResolution{Status: "PENDING"}},
		{ID: "publication-edge", Name: "Guide Curve", Type: "CURVE", CompatibilityVersion: "1.0.0",
			Target:   PublicationTarget{Kind: "TOPOLOGY", SourceVersionID: "revision-boolean", PersistentSelection: &edge},
			Contract: PublicationContract{GeometryKind: "CURVE", Symmetry: "PARAMETER_DIRECTION"}, Resolution: PublicationResolution{Status: "PENDING"}},
	}
	if err := validatePublicationDefinitions(model); err != nil {
		t.Fatal(err)
	}
	model.Publications[1].Contract.GeometryKind = "SURFACE"
	if err := validatePublicationDefinitions(model); err == nil || !strings.Contains(err.Error(), "contract is incompatible") {
		t.Fatalf("edge/SURFACE mismatch error = %v", err)
	}
}

func TestDatumPublicationResolverCoversPointAxisPlaneAndFrame(t *testing.T) {
	model := newPartModel()
	tests := []Publication{
		{ID: "point", Name: "Origin", Type: "POINT", Target: PublicationTarget{Kind: "DATUM", DatumID: "axis-system-default"}},
		{ID: "axis", Name: "Main Axis", Type: "AXIS", Target: PublicationTarget{Kind: "DATUM", DatumID: "axis-system-default", Axis: "Z"}},
		{ID: "plane", Name: "Base Plane", Type: "PLANE", Target: PublicationTarget{Kind: "DATUM", DatumID: "datum-xy"}},
		{ID: "frame", Name: "Main Frame", Type: "FRAME", Target: PublicationTarget{Kind: "DATUM", DatumID: "axis-system-default"}},
	}
	for index := range tests {
		if !resolveDatumPublication(&model, &tests[index], "revision-datum") || tests[index].Resolution.Status != "CONNECTED" {
			t.Fatalf("datum Publication %s did not resolve: %#v", tests[index].Type, tests[index].Resolution)
		}
	}
	if tests[1].Resolution.ZDirection != [3]float64{0, 0, 1} || tests[2].Resolution.ZDirection != [3]float64{0, 0, 1} {
		t.Fatalf("datum Publication frames are wrong: axis=%#v plane=%#v", tests[1].Resolution, tests[2].Resolution)
	}
}

func TestExternalParameterSnapshotDrivesLocalParameterAndDependencyGraph(t *testing.T) {
	model := newPartModel()
	value := modelcore.Quantity{SIValue: 0.042, Dimension: modelcore.LengthDimension}
	external := modelcore.ExternalParameterRef{SourceDocumentID: "source-part",
		Revision:      modelcore.ReferenceSelector{Mode: "PINNED", RevisionID: "source-revision"},
		PublicationID: "publication-length", ExpectedType: modelcore.ValueQuantity,
		ExpectedDimension: modelcore.LengthDimension, ContractVersion: "1.0.0",
		ResolvedRevisionID: "source-revision", ResolvedValue: value, ResolvedValueDigest: resolvedDigest(value)}
	model.Parameters = append(model.Parameters, modelcore.ParameterDefinition{ParameterID: "parameter:external-length",
		Key: "external_length", Label: "External Length", ValueType: modelcore.ValueQuantity,
		Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Role: "INPUT",
		Source: modelcore.ValueSource{External: &external}})
	if err := validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	if model.Parameters[0].EvaluatedValue == nil || math.Abs(model.Parameters[0].EvaluatedValue.SIValue-value.SIValue) > 1e-12 {
		t.Fatalf("external snapshot was not evaluated: %#v", model.Parameters[0])
	}
	modelJSON, _ := json.Marshal(model)
	graph, _, err := buildPartEvaluation(model, "consumer-revision", canonicalModelHash(modelJSON), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := modelcore.DependencyEdge{Source: "external-parameter:parameter:external-length",
		Target: "parameter:parameter:external-length", Kind: modelcore.ReadValue}
	found := false
	for _, edge := range graph.Edges {
		if edge == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("external snapshot dependency missing: %#v", graph.Edges)
	}

	model.Parameters[0].Source.External.ResolvedValueDigest = "tampered"
	if err = validateAndResolvePartParameters(&model); err == nil || !strings.Contains(err.Error(), "VALUE_DIGEST_MISMATCH") {
		t.Fatalf("tampered snapshot error = %v", err)
	}
}

func TestExternalParameterCycleCheckFollowsPublishedParameterDependencyNotOnlyDocument(t *testing.T) {
	quantity := modelcore.Quantity{SIValue: 0.01, Dimension: modelcore.LengthDimension}
	consumer := newPartModel()
	consumer.Parameters = []modelcore.ParameterDefinition{
		{ParameterID: "parameter:a", Key: "a", ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension,
			Source: modelcore.ValueSource{Literal: &quantity}},
		{ParameterID: "parameter:b", Key: "b", ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension,
			Source: modelcore.ValueSource{Literal: &quantity}},
	}
	dimension := modelcore.LengthDimension
	consumer.Publications = []Publication{{ID: "publication-a", Name: "A", Type: "PARAMETER", CompatibilityVersion: "1.0.0",
		Target:   PublicationTarget{Kind: "PARAMETER", ParameterID: "parameter:a"},
		Contract: PublicationContract{ValueType: modelcore.ValueQuantity, Dimension: &dimension, UnitPolicy: "SI_CANONICAL"}}}
	source := newPartModel()
	source.Parameters = []modelcore.ParameterDefinition{{ParameterID: "parameter:source", Key: "source",
		ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension,
		Source: modelcore.ValueSource{External: &modelcore.ExternalParameterRef{SourceDocumentID: "consumer",
			ResolvedRevisionID: "consumer-revision", PublicationID: "publication-a"}}}}
	service := &Service{}
	cycle, err := service.externalParameterTargetReaches(t.Context(), "source", source, "parameter:source",
		"consumer", "parameter:b", consumer, map[string]bool{}, 0)
	if err != nil || cycle {
		t.Fatalf("unrelated return edge was treated as a cycle: cycle=%v error=%v", cycle, err)
	}
	expression, err := modelcore.CompileExpression("b", map[string]modelcore.ParameterBinding{
		"b": {ParameterID: "parameter:b", Dimension: modelcore.LengthDimension}}, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	consumer.Parameters[0].Source = modelcore.ValueSource{Expression: &expression}
	cycle, err = service.externalParameterTargetReaches(t.Context(), "source", source, "parameter:source",
		"consumer", "parameter:b", consumer, map[string]bool{}, 0)
	if err != nil || !cycle {
		t.Fatalf("real parameter dependency cycle was not detected: cycle=%v error=%v", cycle, err)
	}
}

func TestBodyAndParameterPublicationsParticipateInHistoryTreeAndDirtyClosure(t *testing.T) {
	model := newPartModel()
	sketch := testRectangleSketch("sketch-publication", "XY")
	cutSketch := testRectangleSketch("sketch-publication-cut", "XY")
	model.Features = append(model.Features, sketch, Feature{ID: "extrude-publication", Type: "LINEAR_EXTRUDE",
		Profile: sketch.ID, Length: 25, Operation: "NEW_BODY"}, cutSketch,
		Feature{ID: "cut-publication", Type: "LINEAR_EXTRUDE", Profile: cutSketch.ID, Length: 25, Operation: "REMOVE"})
	normalizePartModel(&model)
	lengthID := "parameter:extrude-publication:length"
	dimension := modelcore.LengthDimension
	face := testSelection()
	face.Anchor.FeatureID = "extrude-publication"
	model.Publications = []Publication{
		{ID: "publication-body", Name: "Main Body", Type: "BODY", CompatibilityVersion: "1.0.0",
			Target:   PublicationTarget{Kind: "FEATURE_OUTPUT", FeatureID: "extrude-publication", OutputSlot: "BODY"},
			Contract: PublicationContract{GeometryKind: "BODY"}, Resolution: PublicationResolution{Status: "CONNECTED"}},
		{ID: "publication-length", Name: "Length", Type: "PARAMETER", CompatibilityVersion: "1.0.0",
			Target:     PublicationTarget{Kind: "PARAMETER", ParameterID: lengthID},
			Contract:   PublicationContract{ValueType: modelcore.ValueQuantity, Dimension: &dimension, UnitPolicy: "SI_CANONICAL"},
			Resolution: PublicationResolution{Status: "CONNECTED"}},
		{ID: "publication-face", Name: "Final Surface", Type: "SURFACE", CompatibilityVersion: "1.0.0",
			Target:     PublicationTarget{Kind: "TOPOLOGY", SourceVersionID: "source-revision", PersistentSelection: &face},
			Contract:   PublicationContract{GeometryKind: "SURFACE", Symmetry: "NONE"},
			Resolution: PublicationResolution{Status: "CONNECTED"}},
	}
	if err := validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	modelJSON, _ := json.Marshal(model)
	graph, _, err := buildPartEvaluation(model, "revision-publication", canonicalModelHash(modelJSON),
		[]modelcore.DependencyKey{"feature:extrude-publication"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty := graph.DirtyClosure([]modelcore.DependencyKey{"feature:extrude-publication"})
	if !containsDependency(dirty, "publication:publication-body") {
		t.Fatalf("body Publication was not dirtied by its feature: %v", dirty)
	}
	dirty = graph.DirtyClosure([]modelcore.DependencyKey{"feature:cut-publication"})
	if !containsDependency(dirty, "publication:publication-face") {
		t.Fatalf("final-topology Publication was not dirtied by a downstream Boolean: %v", dirty)
	}

	set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{{Target: modelcore.PropertyAddress{
		EntityID: "publication-body", SlotID: "publication.entity"}}}}
	values, err := modelValues("PART", modelJSON, set)
	if err != nil || len(values[set.Changes[0].Target]) == 0 {
		t.Fatalf("Publication history value = %s, error=%v", values[set.Changes[0].Target], err)
	}
	restored, err := applyModelValues("PART", modelJSON, map[modelcore.PropertyAddress]json.RawMessage{
		set.Changes[0].Target: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	var withoutBodyPublication PartModel
	_ = json.Unmarshal(restored, &withoutBodyPublication)
	if len(withoutBodyPublication.Publications) != 2 || withoutBodyPublication.Publications[0].ID != "publication-length" {
		t.Fatalf("Publication history delete result = %#v", withoutBodyPublication.Publications)
	}

	children := partStructureChildren(model, "document:part", "part", "revision-publication", true)
	if len(children) != 3 || children[2].Kind != "PUBLICATION_SET" || len(children[2].Children) != 3 {
		t.Fatalf("Publication structure nodes = %#v", children)
	}
}
