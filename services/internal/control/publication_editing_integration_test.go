//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestPublicationEditingAndReplacementInterfacesThroughRouter(t *testing.T) {
	s, _, _, _ := featureAssociationTestService(t)
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{Type: kind, Name: name, ActorID: p6Actor})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-publication-ux-%d", v.Document.ID, seq)
		r.VersionID = v.Document.VersionID
		updated, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = updated
	}
	source := create("PART", "Published mounting plane")
	replacement := create("PART", "Replacement with explicit interface")
	for _, v := range []*workspace.DocumentView{&source, &replacement} {
		run(v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mounting", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
	}
	oldPort, newPort := source.Part.Publications[0].ID, replacement.Part.Publications[0].ID
	module := create("PRODUCT", "Mounting module")
	run(&module, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: source.Document.ID})
	child := module.Product.Instances[0].ID
	run(&module, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: child, PublicationID: oldPort, Name: "Mounting interface"})
	port := module.Product.Publications[0].ID
	root := create("PRODUCT", "Interface consumer")
	for _, v := range []workspace.DocumentView{module, source} {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	a, b := root.Product.Instances[0].ID, root.Product.Instances[1].ID
	run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 5, DistanceRelation: "SELECTED_PLANE_NORMAL_V1", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: port, ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "PLANE", GeometryID: "datum-xy"}})
	quantity := root.Product.Constraints[0].QuantityParameter.ParameterID
	found := false
	for _, p := range root.Product.Parameters {
		if p.ParameterID == quantity {
			found = true
		}
	}
	if !found {
		t.Fatal("assembly quantity absent from parameter manager projection")
	}
	run(&root, workspace.CommandRequest{Type: "EDIT_PARAMETER", ParameterID: quantity, Name: "clearance", Value: 15, Unit: "mm"})
	if root.Product.Constraints[0].Value != 15 {
		t.Fatal(root.Product.Constraints[0])
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if root.Product.Constraints[0].Value != 5 {
		t.Fatal("undo quantity", root.Product.Constraints[0])
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	if root.Product.Constraints[0].Value != 15 {
		t.Fatal("redo quantity")
	}
	run(&root, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: quantity, Name: "Assembly clearance"})
	if math.Abs(root.Product.Publications[0].Resolution.Value.SIValue-.015) > 1e-12 {
		t.Fatal(root.Product.Publications)
	}
	// Renaming and choosing a new geometric element preserve the source port ID.
	run(&source, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 2}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	newDatum := source.Part.DatumPlanes[len(source.Part.DatumPlanes)-1].ID
	run(&source, workspace.CommandRequest{Type: "EDIT_PUBLICATION", PublicationID: oldPort, Name: "Renamed mounting", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: newDatum})
	if source.Part.Publications[0].ID != oldPort || source.Part.Publications[0].Target.DatumID != newDatum {
		t.Fatal(source.Part.Publications)
	}
	run(&module, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	if root.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
		t.Fatal("published element edit", root.Product.Constraints[0])
	}
	// Replacement remains an immediate edit; private relations break. Repair one
	// forwarded public port, then every parent consumer retains its stable address.
	run(&module, workspace.CommandRequest{Type: "REPLACE_INSTANCE", InstanceID: child, ReferencedDocumentID: replacement.Document.ID, ReplacementVersionID: replacement.Document.VersionID})
	if module.Product.Publications[0].Resolution.Status != "BROKEN_PUBLICATION" {
		t.Fatal("replacement did not invalidate forwarded source")
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	if root.Product.Constraints[0].EvaluationStatus != "BROKEN" {
		t.Fatal(root.Product.Constraints[0])
	}
	path := workspace.InstancePath{RootDocumentID: module.Document.ID, Canonical: child, Segments: []workspace.InstancePathSegment{{OwnerDocumentID: module.Document.ID, OwnerVersionID: module.Document.VersionID, InstanceID: child, ReferencedDocumentID: replacement.Document.ID, ResolvedVersionID: replacement.Document.VersionID}}}
	run(&module, workspace.CommandRequest{Type: "EDIT_PRODUCT_PUBLICATION", PublicationID: port, Name: "Mounting interface", TargetID: newPort, InstancePath: &path})
	if module.Product.Publications[0].ID != port || module.Product.Publications[0].Resolution.Status != "CONNECTED" {
		t.Fatal("repaired port", module.Product.Publications)
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	if root.Product.Constraints[0].EvaluationStatus != "VERIFIED" || root.Product.Constraints[0].First.PublicationRef.PublicationID != port {
		t.Fatal("consumer did not reconnect through its stable public interface", root.Product.Constraints[0])
	}
}
