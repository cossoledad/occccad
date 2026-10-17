package workspace

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/testsupport"
)

func propertyPatch(t *testing.T, model json.RawMessage, documentType string, input PropertyUpdateRequest) (json.RawMessage, modelcore.ChangeSet) {
	t.Helper()
	payload, _ := json.Marshal(input)
	uri := typeSetProperties
	if documentType == "PRODUCT" {
		uri = typeSetProductProperties
	}
	next, set, err := workspaceCommandRegistry.Apply(documentType, model, modelcore.DomainCommand{CommandID: "properties", TypeURI: uri, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	set, err = reconcilePersistedChanges(documentType, model, next, set)
	if err != nil {
		t.Fatal(err)
	}
	return next, set
}

func TestObjectPropertiesPatchInheritanceAndHistory(t *testing.T) {
	model := newPartModel()
	model.Metadata = &DocumentMetadata{Name: "Original"}
	model.Bodies[0].GeometryKey = "frozen"
	model.Bodies[0].Appearance = &SurfaceAppearance{Color: "#3978bd", Faces: []FaceAppearance{{Anchor: modelcore.SemanticTopologyRef{FeatureID: "import", OutputSlot: "face", SourceIDs: []string{"stable-face"}}, Color: "#000000"}}}
	initial, _ := json.Marshal(model)
	name := "新的文档名称"
	one, first := propertyPatch(t, initial, "PART", PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "DOCUMENT", ID: "doc"}}, Name: &name, Appearance: map[string]json.RawMessage{"opacity": json.RawMessage("0")}})
	two, second := propertyPatch(t, one, "PART", PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "BODY", ID: model.Bodies[0].ID}}, Appearance: map[string]json.RawMessage{"roughness": json.RawMessage("0.8"), "color": json.RawMessage("null"), "pickable": json.RawMessage("false")}})
	var after PartModel
	_ = json.Unmarshal(two, &after)
	if after.Metadata.Name != name || after.DefaultAppearance.Opacity == nil || *after.DefaultAppearance.Opacity != 0 || after.Bodies[0].Appearance.Color != "" || len(after.Bodies[0].Appearance.Faces) != 1 || *after.Bodies[0].Appearance.Pickable {
		t.Fatalf("field patch lost inheritance/zero/face state: %s", two)
	}
	compensate := func(raw json.RawMessage, set modelcore.ChangeSet, undo bool) json.RawMessage {
		t.Helper()
		values, err := modelValues("PART", raw, set)
		if err != nil {
			t.Fatal(err)
		}
		var desired map[modelcore.PropertyAddress]json.RawMessage
		if undo {
			desired, err = set.Compensate(values)
		} else {
			desired, err = set.Reapply(values)
		}
		if err != nil {
			t.Fatal(err)
		}
		next, err := applyModelValues("PART", raw, desired)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	undone := compensate(compensate(two, second, true), first, true)
	if canonicalModelHash(undone) != canonicalModelHash(initial) {
		t.Fatalf("two Undos failed\n%s\n%s", initial, undone)
	}
	redone := compensate(compensate(undone, first, false), second, false)
	if canonicalModelHash(redone) != canonicalModelHash(two) {
		t.Fatal("two Redos failed")
	}
	_, prior, err := buildPartEvaluation(model, "before", canonicalModelHash(initial), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := buildPartEvaluation(after, "after", canonicalModelHash(two), second.ImpactSeeds, &prior)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range manifest.DirtyNodes {
		if !strings.HasPrefix(string(node), "appearance:") && !strings.HasPrefix(string(node), "metadata:") {
			t.Fatalf("properties dirtied geometry %s", node)
		}
	}
	if after.Bodies[0].GeometryKey != "frozen" {
		t.Fatal("properties replaced frozen geometry")
	}
}

func TestObjectPropertiesBatchFacesAndValidation(t *testing.T) {
	m := newPartModel()
	raw, _ := json.Marshal(m)
	body := m.Bodies[0].ID
	targets := []PropertyTarget{{Kind: "FACE", BodyID: body, Anchor: &modelcore.SemanticTopologyRef{FeatureID: "import", OutputSlot: "face-a"}}, {Kind: "FACE", BodyID: body, Anchor: &modelcore.SemanticTopologyRef{FeatureID: "import", OutputSlot: "face-b"}}}
	next, set := propertyPatch(t, raw, "PART", PropertyUpdateRequest{Targets: targets, Appearance: map[string]json.RawMessage{"color": json.RawMessage(`"#000000"`), "opacity": json.RawMessage("0.25")}})
	if len(set.Changes) != 1 {
		t.Fatalf("one stable Body slot must capture both face edits: %+v", set)
	}
	var result PartModel
	_ = json.Unmarshal(next, &result)
	if len(result.Bodies[0].Appearance.Faces) != 2 {
		t.Fatal("batch lost a stable face")
	}
	for _, patch := range []map[string]json.RawMessage{{"opacity": json.RawMessage("1.01")}, {"roughness": json.RawMessage("-1")}, {"metalness": json.RawMessage(`"0.5"`)}, {"unknown": json.RawMessage("1")}, {"color": json.RawMessage(`"#fff"`)}} {
		payload, _ := json.Marshal(PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "BODY", ID: body}}, Appearance: patch})
		if _, _, err := applyPartProperties(raw, payload); err == nil {
			t.Fatalf("accepted invalid style %+v", patch)
		}
	}
	reset, _ := json.Marshal(PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "BODY", ID: body}}, ClearFaceOverrides: true})
	cleared, _, err := applyPartProperties(next, reset)
	if err != nil {
		t.Fatal(err)
	}
	var restored PartModel
	_ = json.Unmarshal(cleared, &restored)
	if restored.Bodies[0].Appearance != nil {
		t.Fatal("explicit clear did not remove empty appearance")
	}
}

func TestObjectPropertiesOccurrenceOwnershipAndFieldPrecedence(t *testing.T) {
	alpha := .3
	base := ResolvedAppearance{Color: "#3978bd", Faces: map[uint64]string{1: "#000000"}}
	over := overlayResolvedAppearance(base, SurfaceStyle{GraphicProperties: GraphicProperties{Opacity: &alpha}})
	if over.Color != base.Color || !reflect.DeepEqual(over.Faces, base.Faces) || *over.Opacity != alpha {
		t.Fatal("opacity override erased definition colors")
	}
	color := overlayResolvedAppearance(over, SurfaceStyle{Color: "#d64a3a"})
	if len(color.Faces) != 0 || len(base.Faces) != 1 {
		t.Fatal("whole occurrence color must cover faces without mutating the definition")
	}
	m := ProductModel{Instances: []ProductInstance{{ID: "sub", Name: "Sub", Rotation: [4]float64{0, 0, 0, 1}}}}
	path := appendInstancePath(appendInstancePath(InstancePath{RootDocumentID: "root"}, InstancePathSegment{InstanceID: "sub"}), InstancePathSegment{InstanceID: "part"})
	raw, _ := json.Marshal(m)
	next, set := propertyPatch(t, raw, "PRODUCT", PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "INSTANCE", ID: "part", InstancePath: &path}}, Appearance: map[string]json.RawMessage{"opacity": json.RawMessage("0.3")}})
	var after ProductModel
	_ = json.Unmarshal(next, &after)
	if len(after.AppearanceOverrides) != 1 || !reflect.DeepEqual(after.Instances, m.Instances) || !displayPropertiesOnly(set) {
		t.Fatalf("nested edit mutated a shared definition: %s", next)
	}
	rules := []occurrenceAppearanceRule{{Path: "sub", Appearance: &SurfaceAppearance{Color: "#d64a3a"}}, {Path: "sub/part", Appearance: &SurfaceAppearance{Color: "#3978bd"}}}
	resolved, err := (&Service{}).applyOccurrenceAppearance(t.Context(), "part-document", PartBody{ID: "body"}, base, "sub/part", rules)
	if err != nil || resolved.Color != "#3978bd" {
		t.Fatalf("closest stable occurrence did not win: %+v %v", resolved, err)
	}
}

func TestObjectPropertiesMetadataAtomicHistoryDatabase(t *testing.T) {
	pool := testsupport.OpenDevelopmentDatabase(t)
	service := New(pool, nil)
	ctx := t.Context()
	actor := "00000000-0000-7000-8000-000000000001"
	view, err := service.CreateDocument(ctx, CreateDocumentRequest{RequestID: uuid.NewString(), Name: "properties-" + uuid.NewString(), Type: "PRODUCT", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	id := view.Document.ID
	t.Cleanup(func() {
		cleanup := context.Background()
		for _, query := range []string{
			`DELETE FROM occccad.outbox_events WHERE aggregate_id IN (SELECT id FROM occccad.workspaces WHERE document_id=$1)`,
			`DELETE FROM occccad.workspaces WHERE document_id=$1`,
			`DELETE FROM occccad.document_history WHERE document_id=$1`,
			`UPDATE occccad.documents SET head_version_id=NULL WHERE id=$1`,
			`UPDATE occccad.document_versions SET parent_version_id=NULL,created_by_command_id=NULL WHERE document_id=$1`,
			`DELETE FROM occccad.revision_parents WHERE revision_id IN (SELECT id FROM occccad.document_versions WHERE document_id=$1)`,
			`DELETE FROM occccad.document_versions WHERE document_id=$1`,
			`DELETE FROM occccad.documents WHERE id=$1`,
		} {
			if _, err := pool.Exec(cleanup, query, id); err != nil {
				t.Errorf("cleanup %s: %v", query, err)
				return
			}
		}
	})

	initial := view.Document.VersionID
	original := view.Document.Name
	renamed := "属性测试-" + uuid.NewString()
	request := CommandRequest{RequestID: uuid.NewString(), ActorID: actor, Type: "SET_PROPERTIES", VersionID: initial, Properties: &PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "DOCUMENT", ID: id}}, Name: &renamed, Appearance: map[string]json.RawMessage{"color": json.RawMessage(`"#000000"`), "opacity": json.RawMessage("0.4")}}}
	preview, err := service.PreviewCommand(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.BaseVersionID != initial {
		t.Fatal("preview lost its base")
	}
	unmodified, err := service.GetDocument(ctx, id, actor)
	if err != nil || unmodified.Document.Name != original || unmodified.Document.VersionID != initial {
		t.Fatal("preview persisted a document change", err)
	}
	view, err = service.ApplyCommand(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	if view.Document.Name != renamed || view.Product.DefaultAppearance.Color != "#000000" {
		t.Fatal("metadata and graphics were not committed together")
	}
	for _, kind := range []string{"UNDO", "REDO", "UNDO", "REDO"} {
		view, err = service.ApplyCommand(ctx, id, CommandRequest{RequestID: uuid.NewString(), ActorID: actor, Type: kind})
		if err != nil {
			t.Fatal(kind, err)
		}
		if kind == "UNDO" {
			if view.Document.Name != original || view.Product.DefaultAppearance != nil {
				t.Fatal("Undo did not compensate both participants")
			}
		} else if view.Document.Name != renamed || view.Product.DefaultAppearance.Color != "#000000" {
			t.Fatal("Redo lost metadata or graphics")
		}
	}
	if _, err := service.ApplyCommand(ctx, id, request); err != nil {
		t.Fatal("idempotent property retry failed", err)
	}
	stale := request
	stale.RequestID = uuid.NewString()
	if _, err := service.ApplyCommand(ctx, id, stale); err == nil {
		t.Fatal("stale Revision overwritten")
	}
	legacy := UpdateDocumentRequest{VersionID: view.Document.VersionID, RequestID: uuid.NewString(), ActorID: actor, Name: "rename-api-" + uuid.NewString(), Description: "统一历史"}
	if _, err := service.UpdateDocument(ctx, id, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateDocument(ctx, id, legacy); err != nil {
		t.Fatal("rename retry changed its frozen base", err)
	}
}

func TestObjectPropertiesReplacementAndCopyKeepOwnership(t *testing.T) {
	path := appendInstancePath(InstancePath{RootDocumentID: "source"}, InstancePathSegment{OwnerDocumentID: "source", OwnerVersionID: "old-root", InstanceID: "component", ReferencedDocumentID: "old-part", ResolvedVersionID: "old-part-r1"})
	model := ProductModel{AppearanceOverrides: []OccurrenceAppearance{{InstancePath: path, BodyID: "same-body-id", Appearance: &SurfaceAppearance{Color: "#ff0000"}}}}
	rules := occurrenceAppearanceRules(model, "", nil)
	base := ResolvedAppearance{Color: "#3978bd"}
	resolved, err := (&Service{}).applyOccurrenceAppearance(t.Context(), "replacement-part", PartBody{ID: "same-body-id"}, base, "component", rules)
	if err != nil || resolved.Color != base.Color || len(resolved.Issues) != 1 {
		t.Fatalf("replacement rebound stale style: %+v %v", resolved, err)
	}
	raw, _ := json.Marshal(model)
	copied, err := rebaseCopiedPropertyPaths(raw, "source", "copy", "copy-r1")
	if err != nil {
		t.Fatal(err)
	}
	var clone ProductModel
	_ = json.Unmarshal(copied, &clone)
	got := clone.AppearanceOverrides[0].InstancePath
	if got.RootDocumentID != "copy" || got.Segments[0].OwnerDocumentID != "copy" || got.Segments[0].OwnerVersionID != "copy-r1" || got.Segments[0].ReferencedDocumentID != "old-part" {
		t.Fatalf("copied wrong identity: %+v", got)
	}
	newPath := path
	newPath.Segments = append([]InstancePathSegment(nil), path.Segments...)
	newPath.Segments[0].ReferencedDocumentID = "replacement-part"
	next, _ := propertyPatch(t, raw, "PRODUCT", PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "BODY", ID: "same-body-id", InstancePath: &newPath}}, Appearance: map[string]json.RawMessage{"opacity": json.RawMessage("0.4")}})
	clone = ProductModel{}
	_ = json.Unmarshal(next, &clone)
	if clone.AppearanceOverrides[0].Appearance.Color != "" || clone.AppearanceOverrides[0].InstancePath.Segments[0].ReferencedDocumentID != "replacement-part" {
		t.Fatalf("explicit replacement edit retained foreign faces/style: %s", next)
	}
}

func TestObjectPropertiesDocumentAdapterDropsPickLocators(t *testing.T) {
	input := PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "DOCUMENT", ID: "doc", BodyID: "irrelevant", GeometryKey: "mesh", LocalID: 19}}}
	_, payload, err := (&Service{}).adaptProperties(t.Context(), "doc", "PART", nil, CommandRequest{VersionID: "revision", Properties: &input})
	if err != nil {
		t.Fatal(err)
	}
	target := payload.(PropertyUpdateRequest).Targets[0]
	if target.LocalID != 0 || target.GeometryKey != "" || target.BodyID != "" {
		t.Fatalf("pick evidence entered formal payload: %+v", target)
	}
}
