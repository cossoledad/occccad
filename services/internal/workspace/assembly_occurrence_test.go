package workspace

import (
	"errors"
	"math"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssemblyNestedOccurrenceUsesAcceptedLeafAndBodyFrame(t *testing.T) {
	root := ProductModel{Instances: []ProductInstance{{ID: "sub", ReferencedDocumentID: "subdoc", ReferencedVersionID: "accepted", Translation: [3]float64{100, 0, 0}}}}
	leaf := ProductInstance{ID: "leaf", ReferencedDocumentID: "part", ReferencedVersionID: "part-accepted", Translation: [3]float64{2, 3, 4}, Rotation: [4]float64{0, 0, math.Sqrt(0.5), math.Sqrt(0.5)}}
	reference := AssemblyGeometryRef{InstanceID: "sub", InstancePath: &InstancePath{Segments: []InstancePathSegment{{InstanceID: "sub", ReferencedDocumentID: "subdoc"}, {InstanceID: "leaf", ReferencedDocumentID: "part"}}}}
	load := func(instance ProductInstance) (ProductModel, error) {
		if instance.ReferencedVersionID != "accepted" {
			t.Fatal("looked outside accepted snapshot")
		}
		return ProductModel{Instances: []ProductInstance{leaf}}, nil
	}
	got, pose, err := resolveAssemblyOccurrence(root, reference, load)
	if err != nil || got.ReferencedVersionID != "part-accepted" {
		t.Fatalf("leaf resolution: %v %v", got, err)
	}
	transformed := assemblyGeometryInBody(geometry.AssemblyGeometry{BodyID: "sub", Kind: "PLANE", Origin: [3]float64{1, 0, 0}, Direction: [3]float64{1, 0, 0}}, pose)
	for i, expected := range [3]float64{2, 4, 4} {
		if math.Abs(transformed.Origin[i]-expected) > 1e-9 {
			t.Fatal(transformed)
		}
	}
	if math.Abs(transformed.Direction[1]-1) > 1e-9 || transformed.BodyID != "sub" {
		t.Fatal(transformed)
	}
	reference.InstancePath.Segments[1].InstanceID = "deleted"
	if _, _, err = resolveAssemblyOccurrence(root, reference, load); !errors.Is(err, ErrValidation) {
		t.Fatal("missing occurrence must break support", err)
	}
	reference.InstancePath.Segments[0].InstanceID = "another"
	if _, _, err = resolveAssemblyOccurrence(root, reference, load); !errors.Is(err, ErrValidation) {
		t.Fatal("cross-body path accepted", err)
	}
}

func TestAssemblyIntrinsicIncompatibilityAndConflictAreDistinct(t *testing.T) {
	a := geometry.AssemblyGeometry{Kind: "CYLINDER", Radius: 1}
	b := geometry.AssemblyGeometry{Kind: "CYLINDER", Radius: 2}
	if incompatibleAssemblyGeometry(geometry.AssemblyConstraint{Kind: "COINCIDENT"}, a, b) == "" {
		t.Fatal("unequal surfaces cannot coincide")
	}
	if incompatibleAssemblyGeometry(geometry.AssemblyConstraint{Kind: "CONCENTRIC"}, a, b) != "" {
		t.Fatal("unequal cylinders can be concentric")
	}
	model := ProductModel{Constraints: []AssemblyConstraint{{ID: "conflict"}, {ID: "intrinsic", EvaluationStatus: modelcore.AssemblyConstraintImpossible}}}
	if err := acceptAssemblyEvaluationFailure(&model, &assemblySolveFailure{status: "UNSATISFIED", code: "CONFLICT"}, true); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated || model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintImpossible {
		t.Fatal(model.Constraints)
	}
	for _, command := range []string{typeAddAssemblyConstraint, typeEditAssemblyConstraint, typeSetAssemblyConstraintState} {
		if !retainsAssemblyDefinition(command) {
			t.Fatal(command)
		}
	}
	if retainsAssemblyDefinition(typeMoveInstance) {
		t.Fatal("failed movement must not commit")
	}
}
