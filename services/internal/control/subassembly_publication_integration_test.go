//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

// Uses production Router/Worker, persisted revisions and accepted context tools.
func TestSubassemblyPublicInterfacesDriveHoleThroughTwoRigidLevels(t *testing.T) {
	s, _, db, _, _ := featureAssociationRuntimeService(t)
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: kind, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.RequestID = fmt.Sprintf("%s-interface-%d", v.Document.ID, seq)
		r.ActorID = p6Actor
		u, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatalf("%s: %v", r.Type, e)
		}
		*v = u
	}
	path := func(v workspace.DocumentView, index int) workspace.InstancePath {
		t.Helper()
		session, e := s.GetProductDesignSession(t.Context(), v.Document.ID, v.Product.Instances[index].ID)
		if e != nil || session.ActiveInstancePath == nil {
			t.Fatal(e)
		}
		return *session.ActiveInstancePath
	}
	read := func(v *workspace.DocumentView) {
		t.Helper()
		u, e := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
		if e != nil {
			t.Fatal(e)
		}
		*v = u
	}
	plan := func(v workspace.DocumentView) workspace.ProductUpdatePlan {
		t.Helper()
		p, e := s.GetProductUpdatePlan(t.Context(), v.Document.ID)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	accept := func(v *workspace.DocumentView) {
		t.Helper()
		p := plan(*v)
		if !p.CanAccept {
			t.Fatalf("cannot accept %+v", p.Entries)
		}
		apply(v, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: p.Digest})
	}
	layout := create("PART", "Module layout")
	apply(&layout, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "pitch", Value: 120, Unit: "mm"})
	pitch := layout.Part.Parameters[0].ParameterID
	apply(&layout, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := layout.Part.Features[0].ID
	value := func(v float64) *float64 { return &v }
	apply(&layout, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "position", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 60, Y: 30}}},
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "alternate", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: -20, Y: 15}}},
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "line", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{X: 5, Y: 6}, End: &workspace.SketchPoint2{X: 25, Y: 6}}},
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "CONSTRUCTION", Center: &workspace.SketchPoint2{X: 2, Y: 3}, Radius: 10}},
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "arc", Kind: "ARC", Role: "CONSTRUCTION", Center: &workspace.SketchPoint2{X: -2, Y: 4}, Radius: 12, StartAngle: .2, EndAngle: 2.5}},
		{Type: "ADD_CONSTRAINT", ParameterSource: func() *string { s := "pitch/2"; return &s }(), Constraint: &workspace.SketchConstraint{ID: "x", Kind: "HORIZONTAL_DISTANCE", Value: value(60), Unit: "mm", References: []workspace.SketchGeometryRef{{Target: "SKETCH_ORIGIN", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "position", SubElement: "POINT"}}}},
		{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "y", Kind: "VERTICAL_DISTANCE", Value: value(30), Unit: "mm", References: []workspace.SketchGeometryRef{{Target: "SKETCH_ORIGIN", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "position", SubElement: "POINT"}}}},
	}})
	for _, id := range []string{"position", "alternate", "line", "circle", "arc"} {
		typ := "CURVE"
		if id == "position" || id == "alternate" {
			typ = "POINT"
		}
		apply(&layout, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: id, PublicationType: typ, TargetKind: "SKETCH_ENTITY", SketchID: sketch, TargetID: id})
	}
	apply(&layout, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "mount", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
	apply(&layout, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "pitch", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: pitch})
	forward := func(v *workspace.DocumentView, source workspace.DocumentView, translation [3]float64, rotation [4]float64) {
		t.Helper()
		apply(v, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: source.Document.ID})
		apply(v, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: v.Product.Instances[0].ID, Translation: translation, Rotation: rotation})
		pubs := source.Part
		if pubs != nil {
			for _, p := range pubs.Publications {
				p0 := path(*v, 0)
				apply(v, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstancePath: &p0, PublicationID: p.ID, Name: p.Name})
			}
		} else {
			for _, p := range source.Product.Publications {
				p0 := path(*v, 0)
				apply(v, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", InstancePath: &p0, PublicationID: p.ID, Name: p.Name})
			}
		}
	}
	adapter := create("PRODUCT", "Layout adapter")
	forward(&adapter, layout, [3]float64{12, 7, 3}, [4]float64{0, 0, math.Sin(.2), math.Cos(.2)})
	// An internal binding is accepted by the adapter, never by its parents.
	internal := create("PART", "Internal module input")
	apply(&internal, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "inside_pitch", Value: 100, Unit: "mm"})
	apply(&adapter, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: internal.Document.ID})
	internalView, internalErr := s.CreateProductContextBinding(t.Context(), adapter.Document.ID, workspace.ProductContextBindingRequest{RequestID: adapter.Document.ID + "-inside", ActorID: p6Actor, ExpectedRootRevisionID: adapter.Document.VersionID, OwningInstancePath: path(adapter, 1), SourceInstancePath: path(adapter, 0), PublicationID: layout.Part.Publications[6].ID, PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: internal.Part.Parameters[0].ParameterID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	if internalErr != nil {
		t.Fatal(internalErr)
	}
	adapter = internalView

	module := create("PRODUCT", "Public module")
	forward(&module, adapter, [3]float64{-8, 13, 4}, [4]float64{math.Sin(.15), 0, 0, math.Cos(.15)})
	plate := create("PART", "Installation plate")
	apply(&plate, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	mount := plate.Part.DatumPlanes[len(plate.Part.DatumPlanes)-1].ID
	apply(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: mount})
	profile := plate.Part.Features[0].ID
	apply(&plate, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -120, Y: -80}, Second: &workspace.SketchPoint2{X: 120, Y: 80}}}})
	apply(&plate, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: profile, Length: 5})
	apply(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: mount})
	receiver := plate.Part.Features[len(plate.Part.Features)-1].ID
	apply(&plate, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "local_pitch", Value: 100, Unit: "mm"})
	localPitch := ""
	for _, parameter := range plate.Part.Parameters {
		if parameter.DisplayAlias == "local_pitch" {
			localPitch = parameter.ParameterID
		}
	}
	if localPitch == "" {
		t.Fatal("missing named local input parameter")
	}
	root := create("PRODUCT", "Assembly with public module")
	for _, v := range []workspace.DocumentView{module, module, plate} {
		apply(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	for i := 0; i < 3; i++ {
		apply(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[i].ID, Translation: [3]float64{float64(i*30 + 10), float64(i*20 - 9), float64(i * 5)}, Rotation: [4]float64{0, math.Sin(float64(i+1) * .12), 0, math.Cos(float64(i+1) * .12)}})
	}
	bind := func(index int, kind, target string) string {
		t.Helper()
		seq++
		v, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{RequestID: fmt.Sprintf("%s-bind-%d", root.Document.ID, seq), ActorID: p6Actor, ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: path(root, 2), SourceInstancePath: path(root, 0), PublicationID: module.Product.Publications[index].ID, PublicationType: module.Product.Publications[index].Type, TargetKind: kind, TargetID: target, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
		if e != nil {
			t.Fatal(e)
		}
		root = v
		return root.Product.ContextBindings[len(root.Product.ContextBindings)-1].ID
	}
	catalog, catalogErr := s.GetContextCatalog(t.Context(), root.Document.ID, path(root, 2).Canonical, "")
	if catalogErr != nil {
		t.Fatal(catalogErr)
	}
	for _, entry := range catalog.Publications {
		if entry.Selectable && len(entry.InstancePath.Segments) != 1 {
			t.Fatal("private nested source exposed as bindable")
		}
	}
	for _, entry := range catalog.Publications {
		if len(entry.InstancePath.Segments) > 1 && entry.Publication.Type == "POINT" {
			_, err := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{RequestID: root.Document.ID + "-private-source", ActorID: p6Actor, ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: path(root, 2), SourceInstancePath: entry.InstancePath, PublicationID: entry.Publication.ID, PublicationType: "POINT", TargetKind: "SKETCH_EXTERNAL_GEOMETRY", TargetID: receiver})
			if err == nil || !strings.Contains(err.Error(), "CONTEXT_PUBLIC_INTERFACE_REQUIRED") {
				t.Fatal("API bypassed module public interface", err)
			}
			break
		}
	}
	var visit func(*workspace.DocumentStructureNode)
	moduleOccurrences := map[string]bool{}
	visit = func(n *workspace.DocumentStructureNode) {
		if n.Kind == "PRODUCT_PUBLICATION" && n.DocumentID == module.Document.ID {
			if n.InstancePath == nil || len(n.InstancePath.Segments) != 1 || n.InstancePath.RootDocumentID != root.Document.ID {
				t.Fatalf("module interface lacks exact occurrence: %+v", n)
			}
			moduleOccurrences[n.InstancePath.Canonical] = true
		}
		for i := range n.Children {
			visit(&n.Children[i])
		}
	}
	visit(root.StructureTree)
	if len(moduleOccurrences) != 2 {
		t.Fatal("repeated module interfaces collapse")
	}
	bind(5, "DATUM", mount)
	bind(6, "PARAMETER", localPitch)
	pointBinding := bind(0, "SKETCH_EXTERNAL_GEOMETRY", receiver)
	for _, i := range []int{2, 3, 4} {
		bind(i, "SKETCH_EXTERNAL_GEOMETRY", receiver)
	}
	variant := func() workspace.ContextVariantSnapshot {
		t.Helper()
		for _, v := range root.ContextVariants {
			if v.BaseDocumentID == plate.Document.ID {
				return v
			}
		}
		t.Fatal("no receiver variant")
		return workspace.ContextVariantSnapshot{}
	}
	external := func(id string) workspace.SketchExternalGeometry {
		t.Helper()
		input := ""
		for _, b := range root.Product.ContextBindings {
			if b.ID == id {
				input = b.ContextInputID
			}
		}
		for _, f := range variant().Part.Features {
			if f.ID == receiver {
				for _, e := range f.Sketch.ExternalGeometry {
					if e.ContextInputID == input {
						return e
					}
				}
			}
		}
		t.Fatal("missing stable port")
		return workspace.SketchExternalGeometry{}
	}
	p := external(pointBinding)
	if p.Snapshot == nil || math.Abs(p.Snapshot.Point.X-60) > 1e-6 || math.Abs(p.Snapshot.Point.Y-30) > 1e-6 {
		t.Fatalf("two-level point projection %+v", p)
	}
	for _, f := range variant().Part.Features {
		if f.ID == receiver {
			for _, e := range f.Sketch.ExternalGeometry {
				if e.Snapshot.Kind == "LINE" && (math.Abs(e.Snapshot.Start.X-5) > 1e-6 || math.Abs(e.Snapshot.End.X-25) > 1e-6) {
					t.Fatalf("forwarded finite line %+v", e.Snapshot)
				}
			}
		}
	}
	scope := func(r workspace.CommandRequest) workspace.CommandRequest {
		v := variant()
		r.RootProductDocumentID = root.Document.ID
		r.RootProductRevisionID = root.Document.VersionID
		r.ContextVariantID = v.VariantKey
		r.InstancePath = &v.OwningInstancePath
		return r
	}
	apply(&plate, scope(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: receiver, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "local", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 60, Y: 30}}}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "coincident", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "local", SubElement: "POINT"}, {Target: "EXTERNAL", EntityID: p.ID, SubElement: "POINT"}}}}}}))
	read(&root)
	apply(&plate, scope(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: plate.Part.ActiveBodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 3, HolePoint: &workspace.PatternPointReference{SketchID: receiver, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "local", SubElement: "POINT"}}}}))
	read(&root)
	hole := plate.Part.Features[len(plate.Part.Features)-1].ID
	portID := p.ID
	assertPoint := func(x, y float64) {
		t.Helper()
		v := variant()
		if v.Status != "READY" {
			t.Fatalf("variant %s: %s", v.Status, v.Diagnostic)
		}
		found := false
		for _, f := range v.Part.Features {
			if f.ID == hole {
				found = true
				if f.EvaluationStatus != "" {
					t.Fatal(f.Diagnostic)
				}
			}
			if f.ID == receiver {
				for _, e := range f.Sketch.Entities {
					if e.ID == "local" && (math.Abs(e.Point.X-x) > 1e-5 || math.Abs(e.Point.Y-y) > 1e-5) {
						t.Fatalf("point %+v, want %v,%v", e.Point, x, y)
					}
				}
			}
		}
		if !found || external(pointBinding).ID != portID {
			t.Fatal("Hole/input identity changed")
		}
	}
	assertPoint(60, 30)
	if artifact, ok := root.Artifacts[variant().Bodies[0].GeometryKey]; !ok || math.Abs(artifact.Volume-(240*160*5-3*math.Pi*4)) > 1e-4 {
		t.Fatalf("exact Hole result volume: %+v", artifact)
	}
	for _, b := range root.Product.ContextBindings {
		if len(b.SourceInstancePath.Segments) != 1 || b.SourceInstancePath.Segments[0].ReferencedDocumentID != module.Document.ID {
			t.Fatal("binding bypassed public module interface")
		}
	}
	before := root.Document.VersionID
	apply(&layout, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: pitch, Value: 140, Unit: "mm"})
	blocked := plan(root)
	if blocked.CanAccept || !blocked.HasUpdates {
		t.Fatalf("parent accepted pending module: %+v", blocked)
	}
	blockedRows := 0
	for _, e := range blocked.Entries {
		if e.DiagnosticCode == "NESTED_REFERENCE_UPDATE_REQUIRED" && e.SourceProductDocumentID == module.Document.ID {
			blockedRows++
		}
	}
	if blockedRows != 2 {
		t.Fatalf("duplicate modules not independently identified: %+v", blocked.Entries)
	}
	_, err := s.ApplyCommand(t.Context(), root.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-blocked", Type: "UPDATE_REFERENCES", UpdatePlanDigest: blocked.Digest})
	if err == nil || !strings.Contains(err.Error(), "PRODUCT_UPDATE_BLOCKED") {
		t.Fatal("parent should refuse", err)
	}
	read(&root)
	if root.Document.VersionID != before {
		t.Fatal("blocked update advanced Head")
	}
	assertPoint(60, 30)
	accept(&adapter)
	accept(&module)
	accept(&root)
	assertPoint(70, 30)
	// A fixed repeated module remains at its accepted Revision.
	apply(&root, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: root.Product.Instances[1].ID, ReferenceMode: "PINNED"})
	fixedModule := root.Product.Instances[1].ReferencedVersionID
	// Compatible redirect keeps the module public ID and receiving constraints/Hole.
	modulePublicID := module.Product.Publications[0].ID
	modulePath := path(module, 0)
	apply(&module, workspace.CommandRequest{Type: "REDIRECT_PRODUCT_PUBLICATION", PublicationID: modulePublicID, TargetID: adapter.Product.Publications[1].ID, InstancePath: &modulePath})
	apply(&module, workspace.CommandRequest{Type: "EDIT_PRODUCT_PUBLICATION", PublicationID: modulePublicID, Name: "Alternate mounting position"})
	accept(&root)
	assertPoint(-20, 15)
	if root.Product.Instances[1].ReferencedVersionID != fixedModule {
		t.Fatal("fixed repeated module followed Head")
	}
	apply(&root, workspace.CommandRequest{Type: "UNDO"})
	assertPoint(70, 30)
	apply(&root, workspace.CommandRequest{Type: "REDO"})
	assertPoint(-20, 15)
	// Explicit pin never needs a selectable candidate. Source loss blocks follow,
	// not the valid accepted result; reconnect the same public interface to repair.
	apply(&root, workspace.CommandRequest{Type: "EDIT_CONTEXT_BINDING", VersionID: root.Document.VersionID, ContextBindingID: pointBinding, ReferenceMode: "PINNED"})
	pinned := root.Product.ContextBindings[2].Accepted.SourceRevisionID
	apply(&layout, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"alternate"}}}})
	accept(&adapter)
	accept(&module)
	accept(&root)
	assertPoint(-20, 15)
	if root.Product.ContextBindings[2].Accepted.SourceRevisionID != pinned {
		t.Fatal("pinned input followed source")
	}
	modulePath = path(module, 0)
	apply(&module, workspace.CommandRequest{Type: "REDIRECT_PRODUCT_PUBLICATION", PublicationID: modulePublicID, TargetID: adapter.Product.Publications[0].ID, InstancePath: &modulePath})
	accept(&root)
	assertPoint(-20, 15)
	apply(&root, workspace.CommandRequest{Type: "EDIT_CONTEXT_BINDING", VersionID: root.Document.VersionID, ContextBindingID: pointBinding, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	accept(&root)
	assertPoint(70, 30)
	// Continue editing the original Hole after interface repair.
	f := workspace.Feature{}
	for _, candidate := range plate.Part.Features {
		if candidate.ID == hole {
			f = candidate
		}
	}
	f.Diameter = 4.5
	apply(&plate, scope(workspace.CommandRequest{Type: "EDIT_FEATURE", VersionID: variant().BaseRevisionID, TargetID: hole, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, plate.StructureTree, hole)}))
	read(&root)
	assertPoint(70, 30)
	warm := variant()
	warmJSON, _ := json.Marshal(warm.Bodies)
	if _, err := db.Exec(t.Context(), `DELETE FROM occccad.product_context_variants WHERE base_document_id=$1`, plate.Document.ID); err != nil {
		t.Fatal(err)
	}
	cold, _, _, _, _ := geometryRuntimeTestService(t, db.(*database.Pool))
	root, err = cold.GetDocument(t.Context(), root.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	assertPoint(70, 30)
	coldJSON, _ := json.Marshal(variant().Bodies)
	if string(warmJSON) != string(coldJSON) {
		t.Fatal("warm/cold result changed")
	}
}

func TestEmptyProductUpdatePlanReturnsEmptyCollectionsWithoutHistory(t *testing.T) {
	s, _, _, _, _ := featureAssociationRuntimeService(t)
	v, err := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "Empty Product update plan"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.GetProductUpdatePlan(t.Context(), v.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.HasUpdates || !p.CanAccept || p.Digest == "" {
		t.Fatalf("unexpected empty Product status: %+v", p)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"entries", "contextVariants"} {
		if string(body[field]) != "[]" {
			t.Fatalf("empty %s must be an array, got %s", field, body[field])
		}
	}
	again, err := s.GetProductUpdatePlan(t.Context(), v.Document.ID)
	if err != nil || again.Digest != p.Digest {
		t.Fatal("read-only checks changed plan", err)
	}
	unchanged, err := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if err != nil || unchanged.Document.VersionID != v.Document.VersionID {
		t.Fatal("empty update check wrote history", err)
	}
}
