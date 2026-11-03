package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestReplacementPublicationCorrespondenceRequiresUniqueCompatibleInterface(t *testing.T) {
	old := Publication{ID: "old", Name: "Mounting", Type: "PLANE", CompatibilityVersion: "1.0.0", Contract: PublicationContract{GeometryKind: "PLANE", Symmetry: "NORMAL_UNORIENTED"}}
	next := old
	next.ID = "new"
	next.Resolution.Status = "CONNECTED"
	cases := []struct {
		name          string
		before, after []Publication
		want          bool
	}{
		{"matching", []Publication{old}, []Publication{next}, true},
		{"missing", []Publication{old}, nil, false},
		{"ambiguous replacement", []Publication{old}, []Publication{next, next}, false},
		{"ambiguous source", []Publication{old, old}, []Publication{next}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := replacementPublicationCorrespondence(c.before, c.after)[old.ID]
			if ok != c.want {
				t.Fatal(ok)
			}
		})
	}
	for _, mutate := range []func(*Publication){func(p *Publication) { p.Name = "Other" }, func(p *Publication) { p.Type = "AXIS" }, func(p *Publication) { p.Contract.Symmetry = "ORIENTED" }, func(p *Publication) { p.Contract.UnitPolicy = "UNSUPPORTED" }, func(p *Publication) { p.CompatibilityVersion = "2.0.0" }, func(p *Publication) { p.Resolution.Status = "BROKEN_PUBLICATION" }} {
		bad := next
		mutate(&bad)
		if len(replacementPublicationCorrespondence([]Publication{old}, []Publication{bad})) != 0 {
			t.Fatal("incompatible interface matched", bad)
		}
	}
}

func TestReplacementReconnectsPublicBoundaryAtomicallyAndPreservesPrivateLinks(t *testing.T) {
	old := ProductInstance{ID: "i", Name: "Part.1", ReferencedDocumentID: "old-doc", ReferencedVersionID: "r1"}
	next := old
	next.ReferencedDocumentID = "new-doc"
	next.ReferencedVersionID = "r2"
	path := appendInstancePath(InstancePath{RootDocumentID: "root"}, InstancePathSegment{OwnerDocumentID: "root", OwnerVersionID: "root-r", InstanceID: old.ID, ReferencedDocumentID: old.ReferencedDocumentID, ResolvedVersionID: old.ReferencedVersionID})
	port := Publication{ID: "new-port", Name: "Mounting", Type: "PLANE", CompatibilityVersion: "1.0.0", Target: PublicationTarget{Kind: "DATUM", DatumID: "new-datum"}, Contract: PublicationContract{GeometryKind: "PLANE"}, Resolution: PublicationResolution{Status: "CONNECTED", SourceDigest: "new-digest"}}
	pubref := func() *PublicationRef {
		return &PublicationRef{PublicationID: "old-port", ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}
	}
	model := ProductModel{Instances: []ProductInstance{old}, Constraints: []AssemblyConstraint{
		{ID: "published", First: AssemblyGeometryRef{InstanceID: old.ID, Kind: "PLANE", InstancePath: &path, PublicationRef: pubref(), GeometryID: "old-datum", GeometryKey: "old-geometry", TopologyID: 7}},
		{ID: "private", First: AssemblyGeometryRef{InstanceID: old.ID, Kind: "PLANE", GeometryID: "old-datum"}},
		{ID: "other", First: AssemblyGeometryRef{InstanceID: "other", Kind: "PLANE", PublicationRef: pubref()}},
		{ID: "partially-repaired", First: AssemblyGeometryRef{InstanceID: old.ID, Kind: "PLANE", InstancePath: &path, PublicationRef: pubref()}, Second: &AssemblyGeometryRef{InstanceID: "unrelated-missing", Kind: "FACE", Resolution: &ResolutionSnapshot{Result: modelcore.SelectionResolution{Status: modelcore.SelectionMissing}}}},
	}, Publications: []ProductPublication{{ID: "outer", Type: port.Type, CompatibilityVersion: port.CompatibilityVersion, Contract: port.Contract, Target: ProductPublicationTarget{InstancePath: path, PublicationID: "old-port"}}}, ContextBindings: []ContextBinding{
		{ID: "source", SourceInstancePath: path, Publication: *pubref()},
		{ID: "receiver", OwningInstancePath: path, SourceInstancePath: path, Publication: *pubref()},
	}}
	before, _ := json.Marshal(model)
	invalidateReplacedRelations(&model, "root", "root-r", old, next)
	reconnectReplacedPublications(&model, old, next, map[string]Publication{"old-port": port})
	model.Instances[0] = next
	if c := model.Constraints[0]; c.First.PublicationRef.PublicationID != port.ID || c.First.GeometryID != "new-datum" || c.First.GeometryKey != "" || c.First.TopologyID != 0 || c.EvaluationStatus != modelcore.AssemblyConstraintNotUpdated || c.First.InstancePath.Segments[0].ReferencedDocumentID != next.ReferencedDocumentID {
		t.Fatal(c)
	}
	if model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintBroken || model.Constraints[2].First.PublicationRef.PublicationID != "old-port" || model.Constraints[3].EvaluationStatus != modelcore.AssemblyConstraintBroken {
		t.Fatal(model.Constraints)
	}
	if model.Publications[0].ID != "outer" || model.Publications[0].Target.PublicationID != port.ID {
		t.Fatal(model.Publications)
	}
	if model.ContextBindings[0].Accepted.Status != "CONNECTED" || model.ContextBindings[0].Publication.PublicationID != port.ID || model.ContextBindings[1].Accepted.Status != "BROKEN" {
		t.Fatal(model.ContextBindings)
	}
	// No aliased slice may rewrite the evidence retained by an unrelated endpoint.
	if path.Segments[0].ReferencedDocumentID != old.ReferencedDocumentID {
		t.Fatal("input path mutated")
	}
	payload, _ := json.Marshal(replaceInstancePayload{InstanceID: old.ID, ReferencedDocumentID: next.ReferencedDocumentID, ReferencedVersionID: next.ReferencedVersionID, Model: model})
	after, set, err := applyReplaceInstance(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	set, err = reconcilePersistedChanges("PRODUCT", before, after, set)
	if err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PRODUCT", before, set)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", after, values)
	if err != nil {
		t.Fatal(err)
	}
	var a, b ProductModel
	_ = json.Unmarshal(before, &a)
	_ = json.Unmarshal(restored, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("undo lost a reference, binding or publication", string(restored))
	}
}

func TestPublicationElementMatchingPreservesSemanticTypeAndAxis(t *testing.T) {
	frame := Publication{Type: "FRAME", Target: PublicationTarget{Kind: "DATUM", DatumID: "frame"}, Resolution: PublicationResolution{Status: "CONNECTED"}}
	point := frame
	point.Type = "POINT"
	if samePublicationElement(frame, point) {
		t.Fatal("frame captured by point port")
	}
	x := frame
	x.Type = "AXIS"
	x.Target.Axis = "X"
	y := x
	y.Target.Axis = "Y"
	if samePublicationElement(x, y) {
		t.Fatal("distinct frame axes matched")
	}
	if !samePublicationElement(x, x) {
		t.Fatal("same axis not matched")
	}
	face := Publication{Type: "SURFACE", Target: PublicationTarget{Kind: "TOPOLOGY"}, Resolution: PublicationResolution{Status: "CONNECTED", GeometryKey: "accepted", TopologyKind: "FACE", LocalID: 7}}
	other := face
	other.Resolution.GeometryKey = "another-snapshot"
	if samePublicationElement(face, other) {
		t.Fatal("local topology number crossed artifacts")
	}
	other = face
	other.Resolution.Status = "BROKEN_PUBLICATION"
	if samePublicationElement(face, other) {
		t.Fatal("broken port captured a valid element")
	}
	if !samePublicationElement(face, face) {
		t.Fatal("same accepted topology not matched")
	}
}
