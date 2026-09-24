package workspace

import (
	"errors"
	"testing"
)

func TestAssemblyPartDefaultsResolveNestedDatumReferences(t *testing.T) {
	part, err := decodeAssemblyPartModel([]byte(`{"units":"mm","features":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	root := ProductModel{Instances: []ProductInstance{{ID: "sub", ReferencedDocumentID: "subdoc", ReferencedVersionID: "sub-v1"}}}
	for _, ref := range []AssemblyGeometryRef{{Kind: "PLANE", GeometryID: "datum-xy"}, {Kind: "PLANE", GeometryID: "datum-xz"}, {Kind: "AXIS", GeometryID: "axis-system-default", Axis: "X"}, {Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Y"}, {Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}, {Kind: "POINT", GeometryID: "axis-system-default"}} {
		ref.InstanceID = "sub"
		ref.InstancePath = &InstancePath{Segments: []InstancePathSegment{{InstanceID: "sub"}, {InstanceID: "leaf"}}}
		instance, _, err := resolveAssemblyOccurrence(root, ref, func(selected ProductInstance) (ProductModel, error) {
			return ProductModel{Instances: []ProductInstance{{ID: "leaf", ReferencedDocumentID: "part", ReferencedVersionID: "part-v1"}}}, nil
		})
		if err != nil || instance.ReferencedDocumentID != "part" || !datumAssemblyReferenceExists(part, ref) {
			t.Fatalf("nested %s/%s: %+v %v", ref.Kind, ref.Axis, instance, err)
		}
	}
	if datumAssemblyReferenceExists(part, AssemblyGeometryRef{Kind: "PLANE", GeometryID: "deleted-custom-plane"}) {
		t.Fatal("invented missing custom support")
	}
	if _, err := decodeAssemblyPartModel([]byte(`invalid`)); err == nil {
		t.Fatal("malformed Part accepted")
	}
}

func TestProductUpdatePlanOwnsOnlyDirectReferences(t *testing.T) {
	item := func(path, doc, revision, mode string, depth int) expandedOccurrence {
		return expandedOccurrence{Path: InstancePath{Canonical: path, Segments: make([]InstancePathSegment, depth)}, DocumentID: doc, RevisionID: revision, ReferenceMode: mode}
	}
	// Child has already accepted leaf-v2, but root still contains the old child
	// snapshot showing leaf-v1. Accepting child-v2 must not be blocked by leaf-v1.
	items := []expandedOccurrence{item("", "root", "root-v1", "", 0), item("sub", "child", "child-v1", "FOLLOW_HEAD", 1), item("sub/leaf", "part", "part-v1", "FOLLOW_HEAD", 2), item("pin", "pinned", "pin-v1", "PINNED", 1), item("pin/leaf", "pinned-part", "old", "FOLLOW_HEAD", 2)}
	plan := ProductUpdatePlan{CanAccept: true}
	calls := 0
	candidate := appendDirectReferenceUpdates(&plan, items, func(doc string) (string, error) {
		calls++
		if doc != "child" {
			t.Fatalf("root tried to update %s", doc)
		}
		return "child-v2", nil
	})
	if calls != 1 || !plan.CanAccept || !plan.HasUpdates || len(plan.Entries) != 1 || candidate["sub"] != "child-v2" {
		t.Fatalf("bad parent plan: %+v %v", plan, candidate)
	}
	plan = ProductUpdatePlan{CanAccept: true}
	appendDirectReferenceUpdates(&plan, items, func(string) (string, error) { return "child-v1", nil })
	if plan.HasUpdates || !plan.CanAccept {
		t.Fatal("descendant manufactured a parent update")
	}
	plan = ProductUpdatePlan{CanAccept: true}
	appendDirectReferenceUpdates(&plan, items, func(string) (string, error) { return "", errors.New("source missing") })
	if plan.CanAccept || plan.Entries[0].DiagnosticCode != "REFERENCE_SOURCE_MISSING" {
		t.Fatal("actual missing child no longer blocks")
	}
	// The child's own plan advances its direct leaf, producing the revision which
	// the parent accepts above; PINNED siblings remain untouched.
	plan = ProductUpdatePlan{CanAccept: true}
	candidate = appendDirectReferenceUpdates(&plan, []expandedOccurrence{item("leaf", "part", "part-v1", "FOLLOW_HEAD", 1)}, func(string) (string, error) { return "part-v2", nil })
	if !plan.CanAccept || candidate["leaf"] != "part-v2" {
		t.Fatal(plan)
	}
}
