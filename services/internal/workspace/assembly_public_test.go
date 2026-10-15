package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestFixCreationUsesExplicitPose(t *testing.T) {
	current := InstancePose{Translation: [3]float64{1, 2, 3}, Rotation: [4]float64{0, 0, 0, 1}}
	explicit := InstancePose{Translation: [3]float64{12.5, 20, 30}, Rotation: [4]float64{0, 0, 1, 0}}
	raw, _ := json.Marshal(ProductModel{Instances: []ProductInstance{{ID: "a", Translation: current.Translation, Rotation: current.Rotation}}})
	for _, mode := range []string{"SPACE", "RELATIVE"} {
		for _, pose := range []*InstancePose{nil, &explicit} {
			req := CommandRequest{RequestID: "fixed-pose", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &AssemblyGeometryRef{InstanceID: "a", Kind: "BODY"}, FixMode: mode, FixedPose: pose}
			_, payload, err := (&Service{}).adaptLegacyCommand(t.Context(), "product", "PRODUCT", raw, req)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(payload)
			next, _, err := applyAddAssemblyConstraint(raw, encoded)
			if err != nil {
				t.Fatal(err)
			}
			var model ProductModel
			if err = json.Unmarshal(next, &model); err != nil {
				t.Fatal(err)
			}
			want := current
			if pose != nil {
				want = *pose
			}
			if !reflect.DeepEqual(model.Constraints[0].FixedPose, &want) || model.Constraints[0].FixMode != mode {
				t.Fatal("create discarded fixed pose or mode", model.Constraints[0])
			}
		}
	}
}

func TestAssemblyContactExplicitBranchValidation(t *testing.T) {
	for _, branch := range []int32{0, -2, 2} {
		_, payload, err := (&Service{}).adaptLegacyCommand(context.Background(), "product", "PRODUCT", json.RawMessage(`{"instances":[],"constraints":[]}`), CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "CONTACT", ContactBranch: &branch})
		if !errors.Is(err, ErrValidation) || payload != nil {
			t.Fatalf("explicit invalid create branch %d accepted: %v %v", branch, payload, err)
		}
		edit, err := json.Marshal(editAssemblyConstraintPayload{ConstraintID: "contact", ContactBranch: &branch})
		if err != nil {
			t.Fatal(err)
		}
		next, changes, err := applyEditAssemblyConstraint(json.RawMessage(`{"instances":[],"constraints":[]}`), edit)
		if !errors.Is(err, ErrValidation) || next != nil || len(changes.Changes) != 0 {
			t.Fatalf("explicit invalid edit branch %d produced candidate: %s %+v %v", branch, next, changes, err)
		}
	}
	for _, branch := range []int32{0, -1, 1} {
		c := AssemblyConstraint{Kind: "CONTACT", ContactKind: "POINT", ContactBranch: branch}
		if err := canonicalAssemblyDefinition(&c, "", ""); err != nil {
			t.Fatal(err)
		}
		want := branch
		if branch == 0 {
			want = 1
		} // Omitted scalar defaults only after the command pointer was checked.
		if c.ContactBranch != want {
			t.Fatalf("default/valid branch changed: %+v", c)
		}
	}
}

func TestAssemblyPublicFamiliesAndContactBranches(t *testing.T) {
	for _, test := range []struct{ kind, family, relation string }{
		{"DISTANCE", "Offset", ""}, {"COINCIDENT", "Coincidence", ""}, {"CONCENTRIC", "Coincidence", ""},
		{"PARALLEL", "Angle", "PARALLEL"}, {"PERPENDICULAR", "Angle", "PERPENDICULAR"}, {"FIX", "Fix", ""}, {"CONTACT", "Contact", ""},
	} {
		c := AssemblyConstraint{Kind: test.kind, ContactKind: "POINT"}
		if err := canonicalAssemblyDefinition(&c, "", ""); err != nil {
			t.Fatal(err)
		}
		if c.Family != test.family || c.DefinitionVersion != 2 || (test.relation != "" && c.AngleRelation != test.relation) {
			t.Fatalf("lost public intent: %+v", c)
		}
	}
	if !publicContactPair("RING", "SPHERE", "CONE") || !publicContactPair("RING", "CONE", "SPHERE") || publicContactPair("POINT", "SPHERE", "SPHERE") {
		t.Fatal("Contact target branches drifted")
	}
	c := AssemblyConstraint{Kind: "CONTACT", ContactKind: "POINT", ContactSide: "EXTERNAL", ContactBranch: 1}
	if err := validateContactDefinition(c); err != nil {
		t.Fatal(err)
	}
	c.ContactBranch = 0
	if validateContactDefinition(c) == nil {
		t.Fatal("accepted unspecified branch")
	}
}
