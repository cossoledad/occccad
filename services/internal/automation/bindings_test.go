package automation

import (
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestAutomationTypedBindingsUseDeclaredSlotsAndStableOutputIdentity(t *testing.T) {
	op, err := NewCatalog().Describe("command.add_assembly_constraint")
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{"insert": true}
	args := map[string]any{"firstAssemblyRef": map[string]any{"kind": "BODY"}}
	bindings := []InputBinding{{Target: "firstAssemblyRef.instanceId", SourceStepID: "insert", OutputID: "second-created-instance", Representation: "ENTITY_ID"}, {Target: "firstAssemblyRef.instancePath", SourceStepID: "insert", OutputID: "second-created-instance", Representation: "INSTANCE_PATH"}}
	for _, binding := range bindings {
		if err := validateInputBinding(op, args, binding, known); err != nil {
			t.Fatal(err)
		}
	}
	for _, target := range []string{"instances[0]", "firstAssemblyRef.arbitrary.child", "groupMembers.0.instanceId", "firstAssemblyRef.__proto__", "firstAssemblyRef.geometryId.radius"} {
		if err := validateInputBinding(op, args, InputBinding{Target: target, SourceStepID: "insert", Representation: "ENTITY_ID"}, known); err == nil {
			t.Fatal("arbitrary path accepted", target)
		}
	}
	path := workspace.InstancePath{RootDocumentID: "product", Canonical: "second-created-instance", Segments: []workspace.InstancePathSegment{{OwnerDocumentID: "product", OwnerVersionID: "revision", InstanceID: "second-created-instance", ReferencedDocumentID: "part", ResolvedVersionID: "part-revision"}}}
	prior := Step{StepSpec: StepSpec{ID: "insert"}, Result: &ActionResult{TypedOutputs: []TypedOutput{{Role: "INSTANCE", Entity: EntityRef{ID: "first-created-instance", Kind: "INSTANCE"}}, {Role: "INSTANCE", Entity: EntityRef{ID: "second-created-instance", Kind: "INSTANCE"}, InstancePath: &path, AssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: "second-created-instance", InstancePath: &path, Kind: "BODY"}}}}}
	step := Step{StepSpec: StepSpec{ID: "fix", TypedBindings: bindings}}
	task := Task{Steps: []Step{prior, step}}
	if err := bindInputs(&task, &step, args); err != nil {
		t.Fatal(err)
	}
	first := args["firstAssemblyRef"].(map[string]any)
	if first["instanceId"] != "second-created-instance" || first["instancePath"] != &path || first["kind"] != "BODY" {
		t.Fatal("typed nested binding did not preserve the formal reference", args)
	}
	step.TypedBindings = []InputBinding{{Target: "firstAssemblyRef", SourceStepID: "insert", Representation: "ASSEMBLY_REF"}}
	if err := bindInputs(&task, &step, map[string]any{}); err == nil {
		t.Fatal("multi-output binding silently chose an array position")
	}
	step.TypedBindings[0].OutputRole = "INSTANCE"
	if err := bindInputs(&task, &step, map[string]any{}); err == nil {
		t.Fatal("ambiguous role silently chose an array position")
	}
	step.TypedBindings[0].OutputRole = ""
	step.TypedBindings[0].OutputID = "second-created-instance"
	whole := map[string]any{}
	if err := bindInputs(&task, &step, whole); err != nil {
		t.Fatal(err)
	}
	if whole["firstAssemblyRef"].(*workspace.AssemblyGeometryRef).InstancePath != &path {
		t.Fatal("whole-object binding lost the occurrence identity")
	}
}

func TestAutomationRectangleVerificationRequiresClosedGeometry(t *testing.T) {
	point := func(x, y float64) *workspace.SketchPoint2 { return &workspace.SketchPoint2{X: x, Y: y} }
	sketch := workspace.SketchFeature{Entities: []workspace.SketchEntity{{Kind: "LINE", Start: point(0, 0), End: point(80, 0)}, {Kind: "LINE", Start: point(80, 0), End: point(80, 50)}, {Kind: "LINE", Start: point(80, 50), End: point(0, 50)}, {Kind: "LINE", Start: point(0, 50), End: point(0, 0)}}}
	width, height, ok := rectangleDimensions(sketch, 1e-6)
	if !ok || width != 80 || height != 50 {
		t.Fatal("valid rectangle rejected")
	}
	sketch.Entities[3].End = point(80, 50)
	if _, _, ok = rectangleDimensions(sketch, 1e-6); ok {
		t.Fatal("open/duplicated geometry with the same bounds was accepted as a rectangle")
	}
}
