//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"strings"
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

func TestProductGeometryPublicationAndBrokenFilletThroughRouter(t *testing.T) {
	s, _, _, _ := featureAssociationTestService(t)
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: kind, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-element-%d", v.Document.ID, seq)
		r.VersionID = v.Document.VersionID
		u, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = u
	}
	part := create("PART", "Published fillet source")
	run(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := part.Part.Features[0].ID
	run(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	run(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	edge := findP7TopologyPick(t, s, part, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return strings.HasPrefix(p.Anchor.OutputSlot, "VERTICAL_FROM_PROFILE_ENDPOINTS/")
	})
	run(&part, workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: part.Part.Features[1].BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
	fillet := part.Part.Features[len(part.Part.Features)-1]
	face := findP7TopologyPick(t, s, part, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.Anchor.FeatureID == fillet.ID
	})
	root := create("PRODUCT", "Direct geometric interfaces")
	for i := 0; i < 2; i++ {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	path := func(index int) workspace.InstancePath {
		t.Helper()
		session, e := s.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[index].ID)
		if e != nil {
			t.Fatal(e)
		}
		return *session.ActiveInstancePath
	}
	for i := 0; i < 2; i++ {
		p := path(i)
		run(&root, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, part).GeometryKey, TopologyID: face.LocalID, InstancePath: &p})
	}
	directIDs := []string{root.Product.Publications[0].ID, root.Product.Publications[1].ID}
	for i, p := range root.Product.Publications {
		if p.Target.Element == nil || p.Target.PublicationID != "" || p.Resolution.Status != "CONNECTED" || p.Resolution.GeometryKey != activeBodyArtifact(t, part).GeometryKey || p.Target.InstancePath.Canonical != root.Product.Instances[i].ID || p.Resolution.Element == nil || p.Resolution.Element.FeatureID != fillet.ID || !strings.Contains(p.Resolution.Element.Path, fillet.Name) {
			t.Fatal("missing direct element scope/name", p)
		}
	}
	unchanged, e := s.GetDocument(t.Context(), part.Document.ID, p6Actor)
	if e != nil || len(unchanged.Part.Publications) != 0 || unchanged.Document.VersionID != part.Document.VersionID {
		t.Fatal("Product publishing modified shared Part", e)
	}
	before := root.Product.Publications[0].Resolution.Origin
	other := root.Product.Publications[1].Resolution.Origin
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[0].ID, Translation: [3]float64{30, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	if math.Abs(root.Product.Publications[0].Resolution.Origin[0]-before[0]-30) > 1e-8 || root.Product.Publications[1].Resolution.Origin != other {
		t.Fatal("occurrence geometry was transformed incorrectly")
	}
	run(&part, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Rounded mounting face", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, part).GeometryKey, TopologyID: face.LocalID})
	partID := part.Part.Publications[0].ID
	suppressed := true
	run(&part, workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: fillet.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, part.StructureTree, fillet.ID), Suppressed: &suppressed})
	if p := part.Part.Publications[0]; p.ID != partID || p.Resolution.Status != "BROKEN_PUBLICATION" || p.Resolution.GeometryKey != "" || p.Resolution.LocalID != 0 || p.Resolution.Element == nil {
		t.Fatal("suppressed generated face did not retain a broken named port", p)
	}
	// Product still displays its accepted source; accepting the newer snapshot breaks its own ports.
	accepted, e := s.GetDocument(t.Context(), root.Document.ID, p6Actor)
	if e != nil || accepted.Product.Publications[0].Resolution.Status != "CONNECTED" {
		t.Fatal("read followed a mutable Part Head", e)
	}
	accept := func() {
		t.Helper()
		p, e := s.GetProductUpdatePlan(t.Context(), root.Document.ID)
		if e != nil || !p.CanAccept {
			t.Fatal("cannot accept source", e, p.Entries)
		}
		run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: p.Digest})
	}
	accept()
	for i, p := range root.Product.Publications {
		if p.ID != directIDs[i] || p.Resolution.Status != "BROKEN_PUBLICATION" || p.Resolution.GeometryKey != "" {
			t.Fatal("Product port silently recovered unavailable geometry", p)
		}
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if root.Product.Publications[0].Resolution.Status != "CONNECTED" {
		t.Fatal("undo did not restore accepted interface")
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	if root.Product.Publications[0].Resolution.Status != "BROKEN_PUBLICATION" {
		t.Fatal("redo did not restore broken interface")
	}
	run(&part, workspace.CommandRequest{Type: "UNDO"})
	if part.Part.Publications[0].Resolution.Status != "CONNECTED" || part.Part.Publications[0].ID != partID {
		t.Fatal("unsuppress did not restore original port identity")
	}
	accept()
	for i, p := range root.Product.Publications {
		if p.ID != directIDs[i] || p.Resolution.Status != "CONNECTED" {
			t.Fatal("restored feature did not recover direct interface", p)
		}
	}
	// Explicit retargeting is still atomic, and never follows an arbitrary same-named face.
	p := path(1)
	run(&root, workspace.CommandRequest{Type: "EDIT_PRODUCT_PUBLICATION", PublicationID: directIDs[0], Name: "Retargeted face", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: activeBodyArtifact(t, part).GeometryKey, TopologyID: face.LocalID, InstancePath: &p})
	if root.Product.Publications[0].ID != directIDs[0] || root.Product.Publications[0].Target.InstancePath.Canonical != p.Canonical {
		t.Fatal("retarget replaced public identity")
	}
}

func TestProductDirectPublicationUsesContextGeometryAndNestedConsumer(t *testing.T) {
	s, _, _, _ := featureAssociationTestService(t)
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: kind, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-direct-context-%d", v.Document.ID, seq)
		r.VersionID = v.Document.VersionID
		u, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = u
	}
	path := func(v workspace.DocumentView, index int) workspace.InstancePath {
		t.Helper()
		p, e := s.GetProductDesignSession(t.Context(), v.Document.ID, v.Product.Instances[index].ID)
		if e != nil {
			t.Fatal(e)
		}
		return *p.ActiveInstancePath
	}
	receiver := create("PART", "Variable receiver")
	run(&receiver, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "height", Value: 10, Unit: "mm"})
	height := receiver.Part.Parameters[0].ParameterID
	run(&receiver, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := receiver.Part.Features[0].ID
	run(&receiver, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 0, Y: 0}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	run(&receiver, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10, LengthExpression: "height"})
	baseKey := activeBodyArtifact(t, receiver).GeometryKey
	control := create("PART", "Module design input")
	run(&control, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "height", Value: 30, Unit: "mm"})
	run(&control, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: control.Part.Parameters[0].ParameterID})
	module := create("PRODUCT", "Context interface module")
	for _, v := range []workspace.DocumentView{control, receiver} {
		run(&module, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	bound, e := s.CreateProductContextBinding(t.Context(), module.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: module.Document.ID + "-binding", ExpectedRootRevisionID: module.Document.VersionID, OwningInstancePath: path(module, 1), SourceInstancePath: path(module, 0), PublicationID: control.Part.Publications[0].ID, PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: height, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	if e != nil {
		t.Fatal(e)
	}
	module = bound
	if len(module.ContextVariants) != 1 || module.ContextVariants[0].Part == nil {
		t.Fatal("context geometry missing")
	}
	variant := module.ContextVariants[0]
	key := variant.Bodies[0].GeometryKey
	if key == baseKey {
		t.Fatal("fixture did not change the receiver geometry")
	}
	faceID := uint64(0)
	for local := uint64(1); local <= p6TopologyCount(t, module.Artifacts[key].Topology, "faces"); local++ {
		p, e := s.GetTopologyElementProperties(t.Context(), module.Document.ID, key, "FACE", local)
		if e != nil {
			t.Fatal(e)
		}
		origin, ok := p.Properties["origin"].([3]float64)
		normal, normalOK := p.Properties["normal"].([3]float64)
		if ok && normalOK && math.Abs(origin[2]-30) < 1e-8 && math.Abs(normal[2]) > .99 {
			faceID = local
			break
		}
	}

	if faceID == 0 {
		t.Fatal("context end cap unavailable")
	}
	receiverPath := path(module, 1)
	run(&module, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Context top face", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: key, TopologyID: faceID, InstancePath: &receiverPath})
	port := module.Product.Publications[0]
	if port.Target.Element == nil || port.Target.Element.SourceGeometryKey != key || port.Resolution.GeometryKey != key || math.Abs(port.Resolution.Origin[2]-30) > 1e-8 {
		t.Fatal("publication used definition geometry instead of context", port)
	}
	checked, err := s.InspectAssemblySupports(t.Context(), module.Document.ID, []workspace.AssemblyGeometryRef{{InstanceID: module.Product.Instances[1].ID, Kind: "FACE", PublicationRef: &workspace.PublicationRef{Scope: "LOCAL", PublicationID: port.ID, ExpectedType: "SURFACE", CompatibilityVersion: port.CompatibilityVersion}}})
	if err != nil || checked.Supports[0].Status != "RESOLVED" || math.Abs(checked.Supports[0].Descriptor.Origin[2]-30) > 1e-8 {
		t.Fatal("local context publication used base geometry", err, checked)
	}
	parent := create("PRODUCT", "Nested context consumer")
	run(&parent, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: module.Document.ID})
	run(&parent, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: receiver.Document.ID})
	run(&parent, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: parent.Product.Instances[0].ID, Translation: [3]float64{50, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	run(&parent, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstanceID: parent.Product.Instances[0].ID, PublicationID: port.ID, Name: "Exposed top face"})
	forwarded := parent.Product.Publications[0]
	if forwarded.Resolution.Status != "CONNECTED" || math.Abs(forwarded.Resolution.Origin[0]-50) > 1e-8 || math.Abs(forwarded.Resolution.Origin[2]-30) > 1e-8 {
		t.Fatal("nested direct publication did not preserve its exact frame", forwarded)
	}
	run(&parent, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 5, DistanceRelation: "SELECTED_PLANE_NORMAL_V1", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: parent.Product.Instances[0].ID, Kind: "FACE", PublicationRef: &workspace.PublicationRef{PublicationID: port.ID, ExpectedType: "SURFACE", CompatibilityVersion: port.CompatibilityVersion}}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: parent.Product.Instances[1].ID, Kind: "PLANE", GeometryID: "datum-xy"}})
	if parent.Product.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal("downstream constraint cannot consume direct Product interface", parent.Product.Constraints[0])
	}
}
