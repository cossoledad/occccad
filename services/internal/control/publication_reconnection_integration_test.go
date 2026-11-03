//go:build integration

package control

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestPublishedConstraintUpdateKeepsEditMotionIntentThroughRouter(t *testing.T) {
	s, _, _, _ := featureAssociationTestService(t)
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, err := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{Type: kind, Name: name, ActorID: p6Actor})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID, r.RequestID, r.VersionID = p6Actor, fmt.Sprintf("%s-motion-intent-%d", v.Document.ID, seq), v.Document.VersionID
		u, err := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
		*v = u
	}
	part := create("PART", "Published offset planes")
	run(&part, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{67.12503050403349, -59.79204397838266, 0}, Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}})
	plane := part.Part.DatumPlanes[len(part.Part.DatumPlanes)-1].ID
	for _, name := range []string{"Side", "Side alias"} {
		run(&part, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: name, PublicationType: "PLANE", TargetKind: "PLANE", TargetID: plane})
	}
	run(&part, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 40}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	run(&part, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Cap", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: part.Part.DatumPlanes[len(part.Part.DatumPlanes)-1].ID})
	root := create("PRODUCT", "Update shares edit motion intent")
	for i := 0; i < 2; i++ {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	first, second := root.Product.Instances[0].ID, root.Product.Instances[1].ID
	poses := []workspace.InstancePose{
		{Translation: [3]float64{94.10862611200993, 125.78766620028934, 158.81853737030804}, Rotation: [4]float64{0.6578419006504448, 0.7531560487366549, 2.693256826748463e-9, 2.351175018768318e-9}},
		{Translation: [3]float64{95.47413321906981, 115.72654148051812, 82.50369260134153}, Rotation: [4]float64{-0.9977262182519311, -0.06739728045477787, -3.5669520827410364e-9, 2.4188662608078585e-10}},
	}
	for i, p := range poses {
		run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[i].ID, Translation: p.Translation, Rotation: p.Rotation})
	}
	run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: first, PublicationID: part.Part.Publications[0].ID, Name: "Mount"})
	port := root.Product.Publications[0].ID
	session, err := s.GetProductDesignSession(t.Context(), root.Document.ID, second)
	if err != nil {
		t.Fatal(err)
	}
	a := workspace.AssemblyGeometryRef{InstanceID: first, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{Scope: "LOCAL", PublicationID: port, ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}}
	b := workspace.AssemblyGeometryRef{InstanceID: second, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{Scope: "OCCURRENCE", OwnerPath: session.ActiveInstancePath, PublicationID: part.Part.Publications[2].ID, ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}}
	run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: &a, SecondAssemblyRef: &b})
	target := root.Product.Constraints[0].ID
	if root.Product.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal("edit policy baseline did not converge", root.Product.Constraints[0])
	}
	session, err = s.GetProductDesignSession(t.Context(), root.Document.ID, first)
	if err != nil {
		t.Fatal(err)
	}
	run(&root, workspace.CommandRequest{Type: "EDIT_PRODUCT_PUBLICATION", PublicationID: port, Name: "Mount", InstancePath: session.ActiveInstancePath, TargetID: part.Part.Publications[1].ID})
	for i, p := range poses {
		run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[i].ID, Translation: p.Translation, Rotation: p.Rotation})
	}
	for _, selected := range []bool{true, false} {
		request := workspace.CommandRequest{Type: "UPDATE_ASSEMBLY_CONSTRAINTS"}
		if selected {
			request.TargetID = target
		}
		run(&root, request)
		if c := root.Product.Constraints[0]; c.EvaluationStatus != modelcore.AssemblyConstraintVerified || c.EvaluationFailure != nil {
			t.Fatal("update dropped edit intent", selected, c)
		}
		for i, value := range root.Product.Instances[1].Translation {
			if math.Abs(value-poses[1].Translation[i]) > 1e-8 {
				t.Fatal("update moved the reference", selected)
			}
		}
		for i, value := range root.Product.Instances[1].Rotation {
			if math.Abs(value-poses[1].Rotation[i]) > 1e-8 {
				t.Fatal("update rotated the reference", selected)
			}
		}
		if math.Abs(root.Product.Instances[0].Translation[2]-poses[0].Translation[2]) < 1 {
			t.Fatal("update did not adopt the solved pose")
		}
		run(&root, workspace.CommandRequest{Type: "UNDO"})
		if root.Product.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated {
			t.Fatal("undo did not restore pending state")
		}
	}
}

func TestConstraintRefreshAndUpdateUseAcceptedRevisionsThroughRouter(t *testing.T) {
	s, _, _, _ := featureAssociationTestService(t)
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, err := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{Type: kind, Name: name, ActorID: p6Actor})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID, r.RequestID, r.VersionID = p6Actor, fmt.Sprintf("%s-refresh-%d", v.Document.ID, seq), v.Document.VersionID
		u, err := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
		*v = u
	}
	part := create("PART", "Reference document name")
	run(&part, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Base", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
	run(&part, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	run(&part, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Raised", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: part.Part.DatumPlanes[len(part.Part.DatumPlanes)-1].ID})
	root := create("PRODUCT", "Refresh and update")
	for i := 0; i < 4; i++ {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	ids := []string{}
	for _, i := range root.Product.Instances {
		ids = append(ids, i.ID)
	}
	run(&root, workspace.CommandRequest{Type: "RENAME_INSTANCE", InstanceID: ids[0], Name: "Mounting occurrence"})
	if root.StructureTree.Children[0].Children[0].Name != part.Document.Name || root.StructureTree.Children[0].InstanceName != "Mounting occurrence" {
		t.Fatal("Part reference must use document name independently of occurrence", root.StructureTree.Children[0])
	}
	for _, index := range []int{0, 2} {
		run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ids[index], Kind: "BODY"}})
		run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: ids[index], PublicationID: part.Part.Publications[0].ID, Name: fmt.Sprintf("Mounting %d", index)})
		port := root.Product.Publications[len(root.Product.Publications)-1].ID
		ref := workspace.AssemblyGeometryRef{InstanceID: ids[index], Kind: "PLANE", PublicationRef: &workspace.PublicationRef{Scope: "LOCAL", PublicationID: port, ExpectedType: "PLANE", CompatibilityVersion: "1.0.0"}}
		run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 5, DistanceRelation: "SELECTED_PLANE_NORMAL_V1", FirstAssemblyRef: &ref, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ids[index+1], Kind: "PLANE", GeometryID: "datum-xy"}})
	}
	for index, p := range root.Product.Publications {
		session, err := s.GetProductDesignSession(t.Context(), root.Document.ID, ids[index*2])
		if err != nil {
			t.Fatal(err)
		}
		run(&root, workspace.CommandRequest{Type: "EDIT_PRODUCT_PUBLICATION", PublicationID: p.ID, Name: p.Name, InstancePath: session.ActiveInstancePath, TargetID: part.Part.Publications[1].ID})
	}
	first, other := root.Product.Constraints[1].ID, root.Product.Constraints[3].ID
	before, _ := s.GetDocument(t.Context(), root.Document.ID)
	placements := func(v workspace.DocumentView) []workspace.InstancePose {
		result := []workspace.InstancePose{}
		for _, i := range v.Product.Instances {
			result = append(result, workspace.InstancePose{Translation: i.Translation, Rotation: i.Rotation})
		}
		return result
	}
	// A newer source Head must not be accepted by either constraint operation.
	accepted := part.Document.VersionID
	run(&part, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "NewSourceHead", Value: 1, Unit: "mm"})
	check := func(firstStatus string) {
		t.Helper()
		if string(root.Product.Constraints[1].EvaluationStatus) != firstStatus || root.Product.Constraints[3].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated {
			t.Fatal(root.Product.Constraints)
		}
		for _, instance := range root.Product.Instances {
			if instance.ReferencedVersionID != accepted {
				t.Fatal("constraint operation accepted source Head", instance)
			}
		}
	}
	run(&root, workspace.CommandRequest{Type: "REFRESH_ASSEMBLY_CONSTRAINTS", TargetID: first})
	check("NOT_UPDATED")
	if !reflect.DeepEqual(placements(root), placements(before)) {
		t.Fatal("refresh moved components")
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_ASSEMBLY_CONSTRAINTS", TargetID: first})
	check("VERIFIED")
	if root.Product.Instances[1].Translation == before.Product.Instances[1].Translation {
		t.Fatal("update did not solve raised support")
	}
	if !reflect.DeepEqual(root.Product.Constraints[3], before.Product.Constraints[3]) {
		t.Fatal("unselected pending constraint changed", other)
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check("NOT_UPDATED")
	if !reflect.DeepEqual(placements(root), placements(before)) {
		t.Fatal("undo did not restore poses")
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	check("VERIFIED")
	run(&root, workspace.CommandRequest{Type: "UPDATE_ASSEMBLY_CONSTRAINTS"})
	if root.Product.Constraints[3].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal("set update skipped pending constraint")
	}
}

func TestPublishedGeometryForwardingAndReplacementThroughRouter(t *testing.T) {
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
		r.RequestID = fmt.Sprintf("%s-reconnect-%d", v.Document.ID, seq)
		r.VersionID = v.Document.VersionID
		u, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = u
	}
	box := func(name string, height float64) workspace.DocumentView {
		t.Helper()
		v := create("PART", name)
		run(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := v.Part.Features[0].ID
		run(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 0, Y: 0}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
		run(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: height})
		face := findP7TopologyPick(t, s, v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
			return strings.HasPrefix(p.Anchor.OutputSlot, "END_CAP/")
		})
		run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mounting", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, v).GeometryKey, TopologyID: face.LocalID})
		return v
	}
	source, replacement := box("Published original", 10), box("Published replacement", 20)
	oldPort, newPort := source.Part.Publications[0].ID, replacement.Part.Publications[0].ID
	module := create("PRODUCT", "Forwarded interface module")
	for i := 0; i < 2; i++ {
		run(&module, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: source.Document.ID})
	}
	selected, other := module.Product.Instances[0].ID, module.Product.Instances[1].ID
	// Advance the Part Head without accepting it in the assembly. Publishing
	// the accepted face still succeeds and forwards the original public port.
	accepted := source.Document.VersionID
	run(&source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "new_metadata", Value: 1, Unit: "mm"})
	session, e := s.GetProductDesignSession(t.Context(), module.Document.ID, selected)
	if e != nil {
		t.Fatal(e)
	}
	oldFace := source.Part.Publications[0].Resolution
	run(&module, workspace.CommandRequest{Type: "CREATE_PUBLICATION", InstancePath: session.ActiveInstancePath, SourceVersionID: accepted, PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: oldFace.GeometryKey, TopologyID: oldFace.LocalID})
	outer := module.Product.Publications[0]
	if outer.Target.Element != nil || outer.Target.PublicationID != oldPort || outer.Name != "Mounting" {
		t.Fatal("raw pick bypassed child publication", outer)
	}
	checkPublishedPreview := func(v workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-support-preview-%d", v.Document.ID, seq)
		r.VersionID = v.Document.VersionID
		p, e := s.PreviewCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal("published support preview", e)
		}
		if p.ConstraintEvaluation == nil || p.ConstraintEvaluation.First.Status != modelcore.SupportingElementConnected || p.ConstraintEvaluation.First.DiagnosticCode != "" {
			t.Fatal("checked publication reported unchecked support", p.ConstraintEvaluation)
		}
	}
	publishedConstraint := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 5, DistanceRelation: "SELECTED_PLANE_NORMAL_V1", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: selected, Kind: "FACE", PublicationRef: &workspace.PublicationRef{PublicationID: oldPort, ExpectedType: "SURFACE", CompatibilityVersion: "1.0.0"}}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: other, Kind: "PLANE", GeometryID: "datum-xy"}}
	checkPublishedPreview(module, publishedConstraint)
	run(&module, publishedConstraint)
	constraintID := module.Product.Constraints[0].ID
	// A higher-level consumer references the stable module interface.
	parent := create("PRODUCT", "Module interface consumer")
	for _, v := range []workspace.DocumentView{module, replacement} {
		run(&parent, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	parentConstraint := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 8, DistanceRelation: "SELECTED_PLANE_NORMAL_V1", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: parent.Product.Instances[0].ID, Kind: "FACE", PublicationRef: &workspace.PublicationRef{PublicationID: outer.ID, ExpectedType: "SURFACE", CompatibilityVersion: "1.0.0"}}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: parent.Product.Instances[1].ID, Kind: "PLANE", GeometryID: "datum-xy"}}
	checkPublishedPreview(parent, parentConstraint)
	run(&parent, parentConstraint)
	editPublished := publishedConstraint
	editPublished.Type = "EDIT_ASSEMBLY_CONSTRAINT"
	editPublished.TargetID = constraintID
	editPublished.Value = 7
	checkPublishedPreview(module, editPublished)
	pose := module.Product.Instances[0].Translation
	run(&module, workspace.CommandRequest{Type: "REPLACE_INSTANCE", InstanceID: selected, ReferencedDocumentID: replacement.Document.ID, ReplacementVersionID: replacement.Document.VersionID})
	if p := module.Product.Publications[0]; p.ID != outer.ID || p.Target.PublicationID != newPort || p.Resolution.Status != "CONNECTED" {
		t.Fatal("forwarding not reconnected", p)
	}
	if c := module.Product.Constraints[0]; c.ID != constraintID || c.First.PublicationRef.PublicationID != newPort || c.EvaluationStatus != "NOT_UPDATED" {
		t.Fatal("constraint interface not reconnected", c)
	}
	if module.Product.Instances[0].Translation != pose || module.Product.Instances[1].ReferencedDocumentID != source.Document.ID || module.Product.Instances[1].ReferencedVersionID != accepted {
		t.Fatal("replacement moved or advanced another occurrence")
	}
	run(&module, workspace.CommandRequest{Type: "UNDO"})
	if module.Product.Instances[0].ReferencedDocumentID != source.Document.ID || module.Product.Publications[0].Target.PublicationID != oldPort || module.Product.Constraints[0].First.PublicationRef.PublicationID != oldPort {
		t.Fatal("undo lost original interface")
	}
	run(&module, workspace.CommandRequest{Type: "REDO"})
	if module.Product.Publications[0].Target.PublicationID != newPort {
		t.Fatal("redo lost replacement interface")
	}
	run(&module, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	if module.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
		t.Fatal("reconnected support did not solve", module.Product.Constraints[0])
	}
	run(&parent, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	if c := parent.Product.Constraints[0]; c.EvaluationStatus != "VERIFIED" || c.First.PublicationRef.PublicationID != outer.ID {
		t.Fatal("parent stable interface did not survive replacement", c)
	}
	// Redirecting the lower-level publication changes the upper source after
	// explicit acceptance, without changing either port's persistent identity.
	before := module.Product.Publications[0].Resolution.LocalID
	bottom := findP7TopologyPick(t, s, replacement, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return strings.HasPrefix(p.Anchor.OutputSlot, "START_CAP/")
	})
	run(&replacement, workspace.CommandRequest{Type: "EDIT_PUBLICATION", PublicationID: newPort, Name: "Renamed after connection", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, replacement).GeometryKey, TopologyID: bottom.LocalID})
	run(&module, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	if p := module.Product.Publications[0]; p.Target.PublicationID != newPort || p.Resolution.Status != "CONNECTED" || p.Resolution.LocalID == before || p.Resolution.LocalID != replacement.Part.Publications[0].Resolution.LocalID {
		t.Fatal("forwarded port ignored source edit", p)
	}
	// Module substitution uses the same public-boundary algorithm; internal
	// topology/occurrence paths are not matched by their display names.
	alternate := create("PRODUCT", "Alternative module")
	run(&alternate, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: replacement.Document.ID})
	run(&alternate, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: alternate.Product.Instances[0].ID, PublicationID: newPort, Name: "Mounting"})
	run(&parent, workspace.CommandRequest{Type: "REPLACE_INSTANCE", InstanceID: parent.Product.Instances[0].ID, ReferencedDocumentID: alternate.Document.ID, ReplacementVersionID: alternate.Document.VersionID})
	if c := parent.Product.Constraints[0]; c.First.PublicationRef.PublicationID != alternate.Product.Publications[0].ID || c.EvaluationStatus != "NOT_UPDATED" {
		t.Fatal("module public boundary not reconnected", c)
	}
	run(&parent, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	if parent.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
		t.Fatal("replacement module did not solve", parent.Product.Constraints[0])
	}
	// Multiple aliases on one element require explicit interface selection.
	run(&replacement, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Alternative alias", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, replacement).GeometryKey, TopologyID: bottom.LocalID})
	run(&module, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	session, e = s.GetProductDesignSession(t.Context(), module.Document.ID, selected)
	if e != nil {
		t.Fatal(e)
	}
	head := module.Document.VersionID
	_, e = s.ApplyCommand(t.Context(), module.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: module.Document.ID + "-ambiguous", Type: "CREATE_PUBLICATION", VersionID: head, InstancePath: session.ActiveInstancePath, PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, replacement).GeometryKey, TopologyID: bottom.LocalID})
	if e == nil || !strings.Contains(e.Error(), "PUBLICATION_SOURCE_AMBIGUOUS") {
		t.Fatal("ambiguous geometry silently bypassed public interfaces", e)
	}
	reopened, e := s.GetDocument(t.Context(), module.Document.ID, p6Actor)
	if e != nil || reopened.Document.VersionID != head {
		t.Fatal("rejected publication changed Head", e)
	}
	run(&module, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: selected, PublicationID: newPort, Name: "Explicit selected interface"})

}

func TestReplacementReconnectsPublishedContextSourceThroughRouter(t *testing.T) {
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
		r.RequestID = fmt.Sprintf("%s-input-reconnect-%d", v.Document.ID, seq)
		r.VersionID = v.Document.VersionID
		u, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = u
	}
	source, replacement, receiver := create("PART", "Input source"), create("PART", "New input source"), create("PART", "Input receiver")
	for index, v := range []*workspace.DocumentView{&source, &replacement, &receiver} {
		run(v, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "Height", Value: float64(10 + index*10), Unit: "mm"})
		if index < 2 {
			run(v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Height input", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: v.Part.Parameters[0].ParameterID})
		}
	}
	product := create("PRODUCT", "Reconnected input")
	for _, v := range []workspace.DocumentView{source, receiver} {
		run(&product, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	paths := make([]*workspace.InstancePath, 2)
	for i := 0; i < 2; i++ {
		session, e := s.GetProductDesignSession(t.Context(), product.Document.ID, product.Product.Instances[i].ID)
		if e != nil {
			t.Fatal(e)
		}
		paths[i] = session.ActiveInstancePath
	}
	var err error
	product, err = s.CreateProductContextBinding(t.Context(), product.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: product.Document.ID + "-binding", ExpectedRootRevisionID: product.Document.VersionID, OwningInstancePath: *paths[1], SourceInstancePath: *paths[0], PublicationID: source.Part.Publications[0].ID, PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: receiver.Part.Parameters[0].ParameterID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	if err != nil {
		t.Fatal(err)
	}
	binding := product.Product.ContextBindings[0]
	run(&product, workspace.CommandRequest{Type: "REPLACE_INSTANCE", InstanceID: product.Product.Instances[0].ID, ReferencedDocumentID: replacement.Document.ID, ReplacementVersionID: replacement.Document.VersionID})
	if b := product.Product.ContextBindings[0]; b.ID != binding.ID || b.ContextInputID != binding.ContextInputID || b.Publication.PublicationID != replacement.Part.Publications[0].ID || b.Accepted.Status != "CONNECTED" {
		t.Fatal("source binding did not reconnect", b)
	}
	assertInputValue := func(v workspace.DocumentView, want float64) {
		t.Helper()
		for _, variant := range v.ContextVariants {
			if variant.BaseDocumentID != receiver.Document.ID || variant.Part == nil {
				continue
			}
			for _, p := range variant.Part.Parameters {
				if p.ParameterID != receiver.Part.Parameters[0].ParameterID {
					continue
				}
				if p.EvaluatedValue == nil || p.EvaluatedValue.SIValue != want {
					t.Fatal("reconnected input was not consumed by the receiving Part", p)
				}
				return
			}
		}
		t.Fatal("receiving context variant missing")
	}
	assertInputValue(product, 0.02)
	plan, e := s.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if e != nil || !plan.CanAccept {
		t.Fatal("reconnected input blocked", e, plan)
	}
	run(&product, workspace.CommandRequest{Type: "UNDO"})
	if b := product.Product.ContextBindings[0]; b.Publication.PublicationID != source.Part.Publications[0].ID || b.Accepted.SourceRevisionID != binding.Accepted.SourceRevisionID {
		t.Fatal("binding undo", b)
	}
	assertInputValue(product, 0.01)
	run(&product, workspace.CommandRequest{Type: "REDO"})
	if product.Product.ContextBindings[0].Publication.PublicationID != replacement.Part.Publications[0].ID {
		t.Fatal("binding redo")
	}
	assertInputValue(product, 0.02)
}

func TestLocalProductPublicationConstraintOwnershipThroughRouter(t *testing.T) {
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
		r.RequestID = fmt.Sprintf("%s-local-port-%d", v.Document.ID, seq)
		r.VersionID = v.Document.VersionID
		u, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = u
	}
	box := func(name string, publish bool) workspace.DocumentView {
		t.Helper()
		v := create("PART", name)
		run(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sid := v.Part.Features[0].ID
		run(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sid, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 0, Y: 0}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
		run(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sid, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
		if publish {
			face := findP7TopologyPick(t, s, v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
				return strings.HasPrefix(p.Anchor.OutputSlot, "END_CAP/")
			})
			run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mounting", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, v).GeometryKey, TopologyID: face.LocalID})
		}
		return v
	}
	part, replacement := box("Local interface source", true), box("Local interface replacement", true)
	root := create("PRODUCT", "Local interface owner")
	for i := 0; i < 3; i++ {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	ids := []string{root.Product.Instances[0].ID, root.Product.Instances[1].ID, root.Product.Instances[2].ID}
	for i, id := range ids {
		run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: id, Translation: [3]float64{float64(i * 40), 0, float64(i*20 + 30)}, Rotation: [4]float64{0, 0, 0, 1}})
	}
	run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: ids[0], PublicationID: part.Part.Publications[0].ID, Name: "Assembly mounting"})
	port := root.Product.Publications[0].ID
	local := func() workspace.AssemblyGeometryRef {
		return workspace.AssemblyGeometryRef{InstanceID: ids[2], Kind: "FACE", PublicationRef: &workspace.PublicationRef{Scope: "LOCAL", PublicationID: port, ExpectedType: "SURFACE", CompatibilityVersion: "1.0.0"}}
	}
	inspected, e := s.InspectAssemblySupports(t.Context(), root.Document.ID, []workspace.AssemblyGeometryRef{local()})
	if e != nil {
		t.Fatal(e)
	}
	if r := inspected.Supports[0]; r.Status != "RESOLVED" || r.Reference.InstanceID != ids[0] || r.Descriptor == nil || r.Descriptor.Origin[2] != 10 {
		t.Fatal("local port did not resolve to its body-local geometry", r)
	}
	// Two interface IDs do not imply two different rigid bodies.
	run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: ids[0], PublicationID: part.Part.Publications[0].ID, Name: "Alternative root alias"})
	f, g := local(), local()
	g.InstanceID = ids[1]
	g.PublicationRef.PublicationID = root.Product.Publications[1].ID
	_, sameErr := s.ApplyCommand(t.Context(), root.Document.ID, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ActorID: p6Actor, RequestID: root.Document.ID + "-same-body", VersionID: root.Document.VersionID, ConstraintKind: "DISTANCE", Value: 5, FirstAssemblyRef: &f, SecondAssemblyRef: &g})
	if sameErr == nil || !strings.Contains(sameErr.Error(), "ASSEMBLY_SAME_MOTION_INSTANCE") {
		t.Fatal("aliases bypassed moving-body validation", sameErr)
	}
	run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ids[1], Kind: "BODY"}})
	first := local()
	run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 5, DistanceRelation: "SELECTED_PLANE_NORMAL_V1", FirstAssemblyRef: &first, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ids[1], Kind: "PLANE", GeometryID: "datum-xy"}})
	check := func(want string, instance string) {
		t.Helper()
		c := root.Product.Constraints[1]
		if c.First.PublicationRef.Scope != "LOCAL" || c.First.PublicationRef.PublicationID != port || c.First.InstanceID != instance || string(c.EvaluationStatus) != want {
			t.Fatal("public address/motion owner lost", c)
		}
	}
	check("VERIFIED", ids[0])
	seq++
	_, e = s.ApplyCommand(t.Context(), root.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-in-use", VersionID: root.Document.VersionID, Type: "DELETE_PRODUCT_PUBLICATION", PublicationID: port})
	if e == nil || !strings.Contains(e.Error(), "PUBLICATION_IN_USE") {
		t.Fatal("used local port was deleted", e)
	}
	poses := []workspace.InstancePose{}
	for _, i := range root.Product.Instances {
		poses = append(poses, workspace.InstancePose{Translation: i.Translation, Rotation: i.Rotation})
	}
	run(&root, workspace.CommandRequest{Type: "EDIT_PRODUCT_PUBLICATION", PublicationID: port, Name: "Renamed mounting"})
	check("VERIFIED", ids[0])
	session, e := s.GetProductDesignSession(t.Context(), root.Document.ID, ids[2])
	if e != nil {
		t.Fatal(e)
	}
	run(&root, workspace.CommandRequest{Type: "EDIT_PRODUCT_PUBLICATION", PublicationID: port, Name: "Renamed mounting", TargetID: part.Part.Publications[0].ID, InstancePath: session.ActiveInstancePath})
	check("NOT_UPDATED", ids[2])
	// Creating another interface must not implicitly solve a pending relation.
	run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: ids[0], PublicationID: part.Part.Publications[0].ID, Name: "Unrelated new interface"})
	check("NOT_UPDATED", ids[2])
	for i, v := range root.Product.Instances {
		if v.Translation != poses[i].Translation {
			t.Fatal("retarget moved a component")
		}
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	check("VERIFIED", ids[2])
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check("NOT_UPDATED", ids[2])
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check("NOT_UPDATED", ids[2])
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check("VERIFIED", ids[0])
	run(&root, workspace.CommandRequest{Type: "REDO"})
	check("NOT_UPDATED", ids[2])
	run(&root, workspace.CommandRequest{Type: "REPLACE_INSTANCE", InstanceID: ids[2], ReferencedDocumentID: replacement.Document.ID, ReplacementVersionID: replacement.Document.VersionID})
	check("NOT_UPDATED", ids[2])
	if root.Product.Publications[0].Target.PublicationID != replacement.Part.Publications[0].ID {
		t.Fatal("replaced source was not reconnected")
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	check("VERIFIED", ids[2])
	broken := box("No matching public interface", false)
	run(&root, workspace.CommandRequest{Type: "REPLACE_INSTANCE", InstanceID: ids[2], ReferencedDocumentID: broken.Document.ID, ReplacementVersionID: broken.Document.VersionID})
	check("BROKEN", ids[2])
	if root.Product.Publications[0].Resolution.Status != "BROKEN_PUBLICATION" {
		t.Fatal("broken root interface hidden")
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check("VERIFIED", ids[2])
	// A child Product port keeps its own ID, independently from a root-owned port.
	module := create("PRODUCT", "Published module")
	run(&module, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	run(&module, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: module.Product.Instances[0].ID, Translation: [3]float64{0, 0, 11}, Rotation: [4]float64{0, 0, 0, 1}})
	run(&module, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: module.Product.Instances[0].ID, PublicationID: part.Part.Publications[0].ID, Name: "Module mounting"})
	outer := create("PRODUCT", "Module port owner")
	run(&outer, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: module.Document.ID})
	run(&outer, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: outer.Product.Instances[0].ID, Translation: [3]float64{0, 0, 30}, Rotation: [4]float64{0, 0, 0, 1}})
	own, e := s.GetProductDesignSession(t.Context(), outer.Document.ID, outer.Product.Instances[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	occurrence := workspace.AssemblyGeometryRef{Kind: "FACE", InstanceID: outer.Product.Instances[0].ID, PublicationRef: &workspace.PublicationRef{Scope: "OCCURRENCE", OwnerPath: own.ActiveInstancePath, PublicationID: module.Product.Publications[0].ID, ExpectedType: "SURFACE", CompatibilityVersion: "1.0.0"}}
	r, e := s.InspectAssemblySupports(t.Context(), outer.Document.ID, []workspace.AssemblyGeometryRef{occurrence})
	if e != nil || r.Supports[0].Status != "RESOLVED" || r.Supports[0].Descriptor.Origin[2] != 21 {
		t.Fatal("module port pose applied incorrectly", e, r)
	}
	run(&outer, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: outer.Product.Instances[0].ID, PublicationID: module.Product.Publications[0].ID, Name: "Outer mounting"})
	occurrence.PublicationRef = &workspace.PublicationRef{Scope: "LOCAL", PublicationID: outer.Product.Publications[0].ID, ExpectedType: "SURFACE", CompatibilityVersion: "1.0.0"}
	r, e = s.InspectAssemblySupports(t.Context(), outer.Document.ID, []workspace.AssemblyGeometryRef{occurrence})
	if e != nil || r.Supports[0].Status != "RESOLVED" || r.Supports[0].Descriptor.Origin[2] != 21 {
		t.Fatal("local forwarding port applied pose twice", e, r)
	}
}
