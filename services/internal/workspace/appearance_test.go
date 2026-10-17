package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestBodyColorHistoryAndGeometryIndependence(t *testing.T) {
	model := newPartModel()
	id := model.Bodies[0].ID
	model.Bodies[0].GeometryKey = "frozen-geometry"
	raw, _ := json.Marshal(model)
	apply := func(current json.RawMessage, color string) (json.RawMessage, modelcore.ChangeSet) {
		t.Helper()
		payload, _ := json.Marshal(bodyColorPayload{BodyID: id, Color: color})
		next, set, err := applyBodyColor(current, payload)
		if err != nil {
			t.Fatal(err)
		}
		return next, set
	}
	red, first := apply(raw, "#D64A3A")
	blue, second := apply(red, "#3978bd")
	compensate := func(current json.RawMessage, set modelcore.ChangeSet, undo bool) json.RawMessage {
		t.Helper()
		values, err := modelValues("PART", current, set)
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
		next, err := applyModelValues("PART", current, desired)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	current := compensate(blue, second, true)
	current = compensate(current, first, true)
	var restored PartModel
	_ = json.Unmarshal(current, &restored)
	if restored.Bodies[0].Appearance != nil || restored.Bodies[0].GeometryKey != "frozen-geometry" {
		t.Fatal("Undo changed geometry or failed to clear color")
	}
	current = compensate(current, first, false)
	current = compensate(current, second, false)
	_ = json.Unmarshal(current, &restored)
	if restored.Bodies[0].Appearance.Color != "#3978bd" || restored.Bodies[0].GeometryKey != "frozen-geometry" {
		t.Fatal("two-step Redo lost color or geometry")
	}
	if canonicalModelHash(raw) == canonicalModelHash(red) {
		t.Fatal("color absent from Revision hash")
	}
	var after PartModel
	_ = json.Unmarshal(red, &after)
	_, prior, err := buildPartEvaluation(model, "before", canonicalModelHash(raw), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := buildPartEvaluation(after, "after", canonicalModelHash(red), first.ImpactSeeds, &prior)
	if err != nil {
		t.Fatal(err)
	}
	for _, dirty := range manifest.DirtyNodes {
		if string(dirty) != "appearance:"+id {
			t.Fatalf("appearance dirtied geometry %s", dirty)
		}
	}
	for _, color := range []string{"red", "#fff", "#zz0000", "#12345678"} {
		payload, _ := json.Marshal(bodyColorPayload{BodyID: id, Color: color})
		if _, _, err := applyBodyColor(raw, payload); err == nil {
			t.Fatalf("accepted invalid color %q", color)
		}
	}
}

func TestFaceInheritOverridesOnlyTheSelectedStableLineage(t *testing.T) {
	a := modelcore.SemanticTopologyRef{FeatureID: "import", OutputSlot: "original", SourceIDs: []string{"stable"}}
	b := modelcore.SemanticTopologyRef{FeatureID: "split", OutputSlot: "first", SourceIDs: []string{"stable-b"}}
	c := modelcore.SemanticTopologyRef{FeatureID: "split", OutputSlot: "second", SourceIDs: []string{"stable-c"}}
	protoRef := func(ref modelcore.SemanticTopologyRef) *workerv1.SemanticTopologyRef {
		return &workerv1.SemanticTopologyRef{FeatureId: ref.FeatureID, OutputSlot: ref.OutputSlot, SourceIds: ref.SourceIDs}
	}
	body := PartBody{ID: "body", Appearance: &SurfaceAppearance{Color: "#d64a3a", Faces: []FaceAppearance{{Anchor: a, Color: "#3978bd"}}}}
	manifest := &topologyManifest{Tips: map[string]*workerv1.FeatureResult{"body": {BodyId: "body", SemanticOutputs: []*workerv1.SemanticTopologyOutput{{TopologyType: 1, LocalId: 19, SemanticRef: protoRef(b)}, {TopologyType: 1, LocalId: 4, SemanticRef: protoRef(c)}}}}, FeatureResults: []*workerv1.FeatureResult{{BodyId: "body", TopologyHistoryComplete: true, TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Sources: []*workerv1.SemanticTopologyRef{protoRef(a)}, Result: protoRef(b)}, {Sources: []*workerv1.SemanticTopologyRef{protoRef(a)}, Result: protoRef(c)}}}}}}
	style, err := patchTargetAppearance(body.Appearance, PropertyTarget{Kind: "FACE", Anchor: &b}, PropertyUpdateRequest{Appearance: map[string]json.RawMessage{"color": json.RawMessage("null")}})
	if err != nil {
		t.Fatal(err)
	}
	body.Appearance = style
	resolved := resolveAppearance(body, manifest)
	if resolved.UnresolvedFaces != 0 || resolved.Faces[19] != "" || resolved.Faces[4] != "#3978bd" || len(resolved.FaceInherit[19]) != 1 {
		t.Fatalf("inherit changed a sibling or retained ancestor color: %+v", resolved)
	}
	style, err = patchTargetAppearance(body.Appearance, PropertyTarget{Kind: "FACE", Anchor: &b}, PropertyUpdateRequest{Appearance: map[string]json.RawMessage{"color": json.RawMessage(`"#000000"`)}})
	if err != nil {
		t.Fatal(err)
	}
	body.Appearance = style
	resolved = resolveAppearance(body, manifest)
	if resolved.UnresolvedFaces != 0 || resolved.Faces[19] != "#000000" || resolved.Faces[4] != "#3978bd" {
		t.Fatalf("explicit result color failed to override inheritance: %+v", resolved)
	}
}

func TestImportedAppearanceFollowsLineageWithoutLocatorIdentity(t *testing.T) {
	ref := func(slot string) *workerv1.SemanticTopologyRef {
		return &workerv1.SemanticTopologyRef{FeatureId: "import", OutputSlot: slot, SourceIds: []string{"persistent-id"}}
	}
	a, b, c := ref("import.topology"), ref("split-a"), ref("split-b")
	body := PartBody{ID: "body", Appearance: &SurfaceAppearance{Faces: []FaceAppearance{{Anchor: semanticRef(a), Color: "#000000"}}}}
	manifest := &topologyManifest{Tips: map[string]*workerv1.FeatureResult{"body": {BodyId: "body", SemanticOutputs: []*workerv1.SemanticTopologyOutput{{TopologyType: 1, LocalId: 19, SemanticRef: b}, {TopologyType: 1, LocalId: 4, SemanticRef: c}}}}, FeatureResults: []*workerv1.FeatureResult{{BodyId: "body", TopologyHistoryComplete: true, TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Sources: []*workerv1.SemanticTopologyRef{a}, Result: b}, {Sources: []*workerv1.SemanticTopologyRef{a}, Result: c}}}}}}
	resolved := resolveAppearance(body, manifest)
	if !reflect.DeepEqual(resolved.Faces, map[uint64]string{19: "#000000", 4: "#000000"}) {
		t.Fatalf("stable color did not follow split: %+v", resolved)
	}
	if missing := resolveAppearance(body, nil); missing.UnresolvedFaces != 1 || len(missing.Faces) != 0 {
		t.Fatal("missing Naming guessed a face")
	}
}
