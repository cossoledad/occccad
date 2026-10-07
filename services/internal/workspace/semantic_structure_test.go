package workspace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPartStructureKeepsSharedSketchAsOneDefinition(t *testing.T) {
	model := PartModel{
		Bodies:       []PartBody{{ID: "body-a", Name: "Body.1", GeometryKey: "geometry-a"}, {ID: "body-b", Name: "Body.2", GeometryKey: "geometry-b"}},
		ActiveBodyID: "body-a",
		Features: []Feature{
			{ID: "sketch", BodyID: "body-a", Type: "SKETCH", Name: "Sketch.1"},
			{ID: "pad-a", BodyID: "body-a", Type: "PAD", Name: "Pad.1", Profile: "sketch", Operation: "ADD"},
			{ID: "pad-b", BodyID: "body-b", Type: "PAD", Name: "Pad.2", Profile: "sketch", Operation: "REMOVE"},
		},
	}
	root := DocumentStructureNode{ID: "document:part", Kind: "PART", DocumentID: "part", VersionID: "revision-1",
		Children: partStructureChildren(model, "document:part", "part", "revision-1", true)}
	annotateStructure(&root, "part", "")
	definitions, references := 0, 0
	var visit func(DocumentStructureNode)
	visit = func(node DocumentStructureNode) {
		if node.EntityID == "sketch" {
			if node.Subject == nil || node.Subject.DocumentID != "part" || node.Subject.EntityKind != "SKETCH" ||
				node.Subject.EntityID != "sketch" || node.Snapshot == nil || node.Snapshot.RevisionID != "revision-1" {
				t.Fatalf("Sketch entry lost semantic subject or snapshot: %+v", node)
			}
			switch node.Kind {
			case "SKETCH":
				definitions++
				if node.BodyID != "body-a" || node.PresentationRole != "DEFINITION" || len(node.Capabilities) != 0 {
					t.Fatalf("shared sketch definition has wrong owner or delete ability: %+v", node)
				}
			case "SKETCH_INPUT_REFERENCE":
				references++
				if node.BodyID != "body-a" || node.PresentationRole != "INPUT_REFERENCE" || len(node.Capabilities) != 0 {
					t.Fatalf("input reference changed ownership or became deletable: %+v", node)
				}
			}
		}
		if node.EntityID == "pad-b" && node.Name != "Pocket.2" {
			t.Fatalf("REMOVE should present as Pocket without changing generator: %+v", node)
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	if definitions != 1 || references != 2 {
		t.Fatalf("expected one Sketch definition and two input entries, got %d and %d", definitions, references)
	}
}

func TestPartStructureNestsSingleBodySketchUse(t *testing.T) {
	model := PartModel{Bodies: []PartBody{{ID: "body-a", Name: "Body.1"}}, ActiveBodyID: "body-a",
		Features: []Feature{{ID: "sketch", BodyID: "body-a", Type: "SKETCH", Name: "Sketch.1"},
			{ID: "pad", BodyID: "body-a", Type: "PAD", Name: "Pad.1", Profile: "sketch", Operation: "ADD"}}}
	children := partStructureChildren(model, "document:part", "part", "revision-1", true)
	for _, body := range children {
		if body.Kind != "BODY" {
			continue
		}
		if len(body.Children) != 1 || body.Children[0].EntityID != "pad" ||
			len(body.Children[0].Children) != 1 || body.Children[0].Children[0].EntityID != "sketch" ||
			body.Children[0].Children[0].PresentationRole != "FEATURE_INPUT" {
			t.Fatalf("single same-Body use was not nested under its Feature: %+v", body.Children)
		}
		return
	}
	t.Fatal("Body missing")
}

func TestStructureIdentityAndReferenceCurrencyAreIndependentOfTreePosition(t *testing.T) {
	path := InstancePath{RootDocumentID: "product", Canonical: "instance-a", Display: "Part(A)",
		Segments: []InstancePathSegment{{OwnerDocumentID: "product", OwnerVersionID: "product-r1",
			InstanceID: "instance-a", ReferencedDocumentID: "part", ResolvedVersionID: "part-r1"}}}
	root := DocumentStructureNode{ID: "document:product/instance:instance-a/reference", Kind: "PART",
		DocumentID: "part", VersionID: "part-r1", InstancePath: &path,
		Children: []DocumentStructureNode{{ID: "a-different-display-path", Kind: "CONTEXT_REFERENCE",
			DocumentID: "part", VersionID: "part-r1", EntityID: "reference-a",
			ResolutionStatus: "CONNECTED", ConnectionStatus: "CONNECTED", InstancePath: &path}}}
	annotateStructure(&root, "part", "")
	bindStructureReferenceCurrency(&root, []ReferenceUpdate{{ConsumerKind: "CONTEXT_REFERENCE",
		ConsumerID: "reference-a", Status: "UPDATE_AVAILABLE"}})
	child := root.Children[0]
	if root.Subject == nil || root.Subject.EntityID != "part" || root.Occurrence == nil ||
		root.Occurrence.RootDocumentID != "product" || child.Subject == nil ||
		child.Subject.EntityID != "reference-a" || child.Snapshot == nil ||
		child.Snapshot.RevisionID != "part-r1" || child.CurrencyStatus != "UPDATE_AVAILABLE" ||
		child.ConnectionStatus != "CONNECTED" {
		t.Fatalf("semantic identity and reference statuses = root %+v, child %+v", root, child)
	}
}

func TestDocumentViewKeepsMeshOutOfStructureSnapshot(t *testing.T) {
	view := DocumentView{Artifacts: map[string]Artifact{"geometry-a": {
		GeometryKey: "geometry-a", Mesh: Mesh{Vertices: [][3]float64{{1, 2, 3}}},
	}}, StructureTree: &DocumentStructureNode{ID: "document:part/body:body-a", Kind: "BODY",
		EntityID: "body-a", GeometryKey: "geometry-a"}}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"vertices"`) || strings.Contains(string(raw), `"triangles"`) ||
		strings.Contains(string(raw), `"mesh"`) {
		t.Fatalf("DocumentView serialized decoded mesh data: %s", raw)
	}
}

func TestAssemblyConstraintStructureNamesAndSetVisibility(t *testing.T) {
	model := ProductModel{Constraints: []AssemblyConstraint{
		{ID: "contact-a", Kind: "CONTACT"}, {ID: "fix", Kind: "FIX"},
		{ID: "contact-b", Kind: "CONTACT"}, {ID: "group", Kind: "FIX_TOGETHER"},
	}}
	check := func(model ProductModel, wantVisible bool) {
		t.Helper()
		raw, _ := json.Marshal(model)
		root, err := projectProductStructure(t.Context(), &Service{}, DocumentStructureNode{ID: "document:product", DocumentID: "product", VersionID: "revision"}, raw, InstancePath{}, map[string]bool{})
		if err != nil {
			t.Fatal(err)
		}
		var group *DocumentStructureNode
		for i := range root.Children {
			if root.Children[i].Kind == "ASSEMBLY_CONSTRAINT_SET" {
				group = &root.Children[i]
			}
		}
		if group == nil || group.EntityID != "assembly-constraints" || group.Subject == nil || group.Subject.DocumentID != "product" || group.LocalVisible == nil || *group.LocalVisible != wantVisible {
			t.Fatalf("constraint set lost identity/aggregate visibility: %+v", group)
		}
		for i, want := range []string{"#接触.1", "#固定.1", "#接触.2", "#固联组.1"} {
			if !strings.HasPrefix(group.Children[i].Name, want+"（") {
				t.Fatalf("constraint name = %q, want prefix %q", group.Children[i].Name, want)
			}
		}
	}
	check(model, true)
	for i := range model.Constraints {
		model.Constraints[i].Visible = boolPointer(false)
	}
	check(model, false)
	model.Constraints[1].Visible = boolPointer(true)
	check(model, true)
}
