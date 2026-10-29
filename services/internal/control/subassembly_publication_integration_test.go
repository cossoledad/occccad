//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
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

func TestReplacePublicComponentScopedHistoryAndHole(t *testing.T) {
	s, _, _, _, _ := featureAssociationRuntimeService(t)
	for _, kind := range []string{"PART", "PRODUCT"} {
		t.Run(kind, func(t *testing.T) {
			seq := 0
			create := func(k, name string) workspace.DocumentView {
				t.Helper()
				v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: k, Name: name})
				if e != nil {
					t.Fatal(e)
				}
				return v
			}
			run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				r.RequestID = fmt.Sprintf("%s-replace-%d", v.Document.ID, seq)
				n, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
				if e != nil {
					t.Fatalf("%s: %v", r.Type, e)
				}
				*v = n
			}
			read := func(v *workspace.DocumentView) {
				t.Helper()
				n, e := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
				if e != nil {
					t.Fatal(e)
				}
				*v = n
			}
			path := func(v workspace.DocumentView, i int) workspace.InstancePath {
				t.Helper()
				session, e := s.GetProductDesignSession(t.Context(), v.Document.ID, v.Product.Instances[i].ID)
				if e != nil || session.ActiveInstancePath == nil {
					t.Fatal(e)
				}
				return *session.ActiveInstancePath
			}
			source := func(name string, x float64) workspace.DocumentView {
				v := create("PART", name)
				run(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				sketch := v.Part.Features[0].ID
				run(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "hole-position", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: x, Y: 10}}}}})
				run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mount position", PublicationType: "POINT", TargetKind: "SKETCH_ENTITY", SketchID: sketch, TargetID: "hole-position"})
				run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mount plane", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
				// An unused broken/extra interface is deliberately not part of the mapping.
				run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Unused plane", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-yz"})
				if kind == "PART" {
					return v
				}
				m := create("PRODUCT", name+" module")
				run(&m, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
				for _, p := range v.Part.Publications {
					q := path(m, 0)
					run(&m, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", PublicationID: p.ID, InstanceID: m.Product.Instances[0].ID, InstancePath: &q, Name: p.Name})
				}
				return m
			}
			a, b := source("Replace A", 20), source("Replace B", 30)
			pubs := func(v workspace.DocumentView) []workspace.Publication {
				if v.Part != nil {
					return v.Part.Publications
				}
				result := []workspace.Publication{}
				for _, p := range v.Product.Publications {
					result = append(result, workspace.Publication{ID: p.ID, Name: p.Name, Type: p.Type, Contract: p.Contract, CompatibilityVersion: p.CompatibilityVersion, Resolution: p.Resolution})
				}
				return result
			}
			pa, pb := pubs(a), pubs(b)
			plate := create("PART", "Replacement mounting plate")
			run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			profile := plate.Part.Features[0].ID
			run(&plate, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -60, Y: -40}, Second: &workspace.SketchPoint2{X: 60, Y: 40}}}})
			run(&plate, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: profile, Length: 5})
			run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			receiver := plate.Part.Features[len(plate.Part.Features)-1].ID
			run(&plate, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Plate plane", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
			unrelated := create("PART", "Unrelated pending component")
			root := create("PRODUCT", "Scoped component replacement")
			for _, v := range []workspace.DocumentView{a, a, plate, unrelated} {
				run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
			}
			run(&root, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: root.Product.Instances[3].ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
			owner, src := path(root, 2), path(root, 0)
			n, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-binding", ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: owner, SourceInstancePath: src, PublicationID: pa[0].ID, PublicationType: "POINT", TargetKind: "SKETCH_EXTERNAL_GEOMETRY", TargetID: receiver, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
			if e != nil {
				t.Fatal(e)
			}
			root = n
			binding := root.Product.ContextBindings[0]
			variant := func() workspace.ContextVariantSnapshot {
				t.Helper()
				for _, v := range root.ContextVariants {
					if v.BaseDocumentID == plate.Document.ID {
						return v
					}
				}
				t.Fatal("missing variant")
				return workspace.ContextVariantSnapshot{}
			}
			external := ""
			for _, f := range variant().Part.Features {
				if f.ID == receiver {
					external = f.Sketch.ExternalGeometry[0].ID
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
			run(&plate, scope(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: receiver, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "local-hole", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 20, Y: 10}}}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "point-link", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "local-hole", SubElement: "POINT"}, {Target: "EXTERNAL", EntityID: external, SubElement: "POINT"}}}}}}))
			read(&root)
			run(&plate, scope(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: plate.Part.ActiveBodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 3, HolePoint: &workspace.PatternPointReference{SketchID: receiver, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "local-hole", SubElement: "POINT"}}}}))
			read(&root)
			hole := plate.Part.Features[len(plate.Part.Features)-1].ID
			owner2, src2 := path(root, 2), path(root, 1)
			seq++
			linked, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-second-binding-%d", root.Document.ID, seq), ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: owner2, SourceInstancePath: src2, PublicationID: pa[0].ID, PublicationType: "POINT", TargetKind: "SKETCH_EXTERNAL_GEOMETRY", TargetID: receiver, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
			if e != nil {
				t.Fatal(e)
			}
			root = linked

			// Keep the accepted A input at x=20 while the occurrence has moved.
			// Replacement accepts B through the current frozen relative frame.
			run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[0].ID, Translation: [3]float64{10, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})

			run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[0].ID, Kind: "BODY"}})
			run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[0].ID, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: pa[1].ID, ExpectedType: "PLANE", CompatibilityVersion: pa[1].CompatibilityVersion}}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[2].ID, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: plate.Part.Publications[0].ID, ExpectedType: "PLANE", CompatibilityVersion: plate.Part.Publications[0].CompatibilityVersion}}})
			q := path(root, 0)
			run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", PublicationID: pa[0].ID, InstancePath: &q, Name: "Public mounting point"})
			// A pending unrelated Head must not be accepted by replacement.
			run(&unrelated, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "pending", Value: 9, Unit: "mm"})
			before := root
			beforeGeometry := variant().Bodies[0].GeometryKey
			untouchedBinding, _ := json.Marshal(root.Product.ContextBindings[1])
			frozenSecond := root.Product.Instances[1]
			frozenOther := root.Product.Instances[3]
			request := workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-candidate", Type: "REPLACE_INSTANCE", VersionID: root.Document.VersionID, InstanceID: root.Product.Instances[0].ID, ReferencedDocumentID: b.Document.ID, ReplacementVersionID: b.Document.VersionID, PublicationMapping: map[string]string{pa[0].ID: pb[0].ID, pa[1].ID: pb[1].ID}}
			reject := func(r workspace.CommandRequest) {
				t.Helper()
				r.RequestID += "-reject"
				if _, e := s.PreviewCommand(t.Context(), root.Document.ID, r); e == nil {
					t.Fatal("invalid replacement preview accepted")
				}
				read(&root)
				if root.Document.VersionID != before.Document.VersionID {
					t.Fatal("failure wrote Head")
				}
			}
			missing := request
			missing.PublicationMapping = map[string]string{pa[0].ID: pb[0].ID}
			reject(missing)
			mismatch := request
			mismatch.PublicationMapping = map[string]string{pa[0].ID: pb[1].ID, pa[1].ID: pb[1].ID}
			reject(mismatch)
			// A contract-compatible source outside the plate invalidates the real Hole.
			outside := source("Outside mounting component", 500)
			outsidePubs := pubs(outside)
			invalidGeometry := request
			invalidGeometry.ReferencedDocumentID = outside.Document.ID
			invalidGeometry.ReplacementVersionID = outside.Document.VersionID
			invalidGeometry.PublicationMapping = map[string]string{pa[0].ID: outsidePubs[0].ID, pa[1].ID: outsidePubs[1].ID}
			reject(invalidGeometry)
			receiverReplacement := request
			receiverReplacement.InstanceID = root.Product.Instances[2].ID
			receiverReplacement.ReferencedDocumentID = unrelated.Document.ID
			receiverReplacement.ReplacementVersionID = unrelated.Document.VersionID
			receiverReplacement.PublicationMapping = nil
			reject(receiverReplacement)

			preview, e := s.PreviewCommand(t.Context(), root.Document.ID, request)
			if e != nil {
				t.Fatal(e)
			}
			if preview.PreviewID == "" || preview.ReplacementView == nil {
				t.Fatal("no qualified replacement view")
			}
			if preview.ReplacementView.Product.Instances[0].ReferencedDocumentID != b.Document.ID {
				t.Fatal("preview still shows old source")
			}
			for _, a := range preview.ReplacementView.Artifacts {
				for role, ref := range a.Representations {
					if role == "VISUAL" && !s.PreviewVisualAllowed(root.Document.ID, p6Actor, preview.PreviewID, ref.ObjectID) {
						t.Fatal("candidate visual not authorized")
					}
				}
			}

			read(&root)
			if root.Document.VersionID != before.Document.VersionID {
				t.Fatal("preview wrote Head")
			}
			s.DiscardPreview(root.Document.ID, p6Actor, preview.PreviewID)
			canceled := request
			canceled.PreviewID = preview.PreviewID
			if _, e = s.ApplyCommand(t.Context(), root.Document.ID, canceled); e == nil {
				t.Fatal("canceled candidate committed")
			}
			preview, e = s.PreviewCommand(t.Context(), root.Document.ID, request)
			if e != nil {
				t.Fatal(e)
			}
			request.PreviewID = preview.PreviewID
			run(&root, request)
			check := func(x float64, sourceID string) {
				t.Helper()
				if root.Product.Instances[0].ReferencedDocumentID != sourceID || root.Product.Instances[0].ID != before.Product.Instances[0].ID || root.Product.Instances[0].Name != before.Product.Instances[0].Name || root.Product.Instances[0].ReferenceMode != before.Product.Instances[0].ReferenceMode {
					t.Fatal("instance identity/policy changed")
				}
				if root.Product.Instances[1].ReferencedVersionID != frozenSecond.ReferencedVersionID || root.Product.Instances[1].ReferencedDocumentID != frozenSecond.ReferencedDocumentID || root.Product.Instances[3].ReferencedVersionID != frozenOther.ReferencedVersionID {
					t.Fatal("unrelated references advanced")
				}
				actualUntouched, _ := json.Marshal(root.Product.ContextBindings[1])
				if string(actualUntouched) != string(untouchedBinding) {
					t.Fatal("unrelated accepted binding changed")
				}

				b := root.Product.ContextBindings[0]
				if b.ID != binding.ID || b.ContextInputID != binding.ContextInputID {
					t.Fatal("binding/input identity changed")
				}
				v := variant()
				artifact, exists := root.Artifacts[v.Bodies[0].GeometryKey]
				if !exists || math.Abs(artifact.Volume-(120*80*5-3*math.Pi*4)) > 1e-4 {
					t.Fatalf("incorrect exact Hole volume %+v", artifact)
				}
				found := false
				for _, f := range v.Part.Features {
					if f.ID == hole {
						found = true
						if f.EvaluationStatus != "" {
							t.Fatalf("Hole failed: %+v", f)
						}
					}
					if f.ID == receiver {
						for _, p := range f.Sketch.Entities {
							if p.ID == "local-hole" && math.Abs(p.Point.X-x) > 1e-6 {
								t.Fatalf("Hole position %v want %v", p.Point.X, x)
							}
						}
					}
				}
				if !found {
					t.Fatal("Hole identity lost")
				}
				for _, c := range root.Product.Constraints {
					if c.EvaluationStatus != "VERIFIED" {
						t.Fatalf("constraint failed %+v", c)
					}
				}
				if root.Product.Publications[0].ID != before.Product.Publications[0].ID || root.Product.Publications[0].Resolution.Status != "CONNECTED" {
					t.Fatal("public interface identity lost")
				}
			}
			stale := request
			stale.RequestID += "-stale"
			if _, e := s.ApplyCommand(t.Context(), root.Document.ID, stale); e == nil {
				t.Fatal("stale replacement committed")
			}

			check(40, b.Document.ID)
			if variant().Bodies[0].GeometryKey == beforeGeometry {
				t.Fatal("Hole geometry did not change with the new point")
			}
			run(&root, workspace.CommandRequest{Type: "UNDO"})
			check(20, a.Document.ID)
			run(&root, workspace.CommandRequest{Type: "REDO"})
			check(40, b.Document.ID)
			read(&root)
			check(40, b.Document.ID)
			// The same accepted input can be fixed, but replacement must not silently reconnect it.
			run(&root, workspace.CommandRequest{Type: "EDIT_CONTEXT_BINDING", VersionID: root.Document.VersionID, ContextBindingID: binding.ID, ReferenceMode: "PINNED"})
			frozen := root.Document.VersionID
			request.VersionID = frozen
			request.PreviewID = ""
			request.ReferencedDocumentID = a.Document.ID
			request.ReplacementVersionID = a.Document.VersionID
			request.PublicationMapping = map[string]string{pb[0].ID: pa[0].ID, pb[1].ID: pa[1].ID}
			request.RequestID += "-fixed"
			if _, e := s.ApplyCommand(t.Context(), root.Document.ID, request); e == nil {
				t.Fatal("fixed binding silently rebound")
			}
			read(&root)
			if root.Document.VersionID != frozen {
				t.Fatal("fixed-input failure wrote history")
			}
		})
	}
}

// Exact solids, six bindings/four instances, a non-first source and a solver
// displacement. Both command paths must produce the same physical hole tools.
func TestReplacementFinalFramesAndManyInputs(t *testing.T) {
	s, store, _, client, _ := featureAssociationRuntimeService(t)
	for _, mode := range []string{"PART-preview", "PRODUCT-direct", "PRODUCT-preview", "PART-inconsistent"} {
		t.Run(mode, func(t *testing.T) {
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
				r.RequestID = fmt.Sprintf("%s-final-frame-%d", v.Document.ID, seq)
				next, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
				if e != nil {
					t.Fatalf("%s: %v", r.Type, e)
				}
				*v = next
			}
			read := func(v *workspace.DocumentView) {
				t.Helper()
				next, e := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
				if e != nil {
					t.Fatal(e)
				}
				*v = next
			}
			path := func(v workspace.DocumentView, index int) workspace.InstancePath {
				t.Helper()
				session, e := s.GetProductDesignSession(t.Context(), v.Document.ID, v.Product.Instances[index].ID)
				if e != nil || session.ActiveInstancePath == nil {
					t.Fatal(e)
				}
				return *session.ActiveInstancePath
			}
			box := func(v *workspace.DocumentView, x float64, operation string) {
				t.Helper()
				run(v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				sk := v.Part.Features[len(v.Part.Features)-1].ID
				run(v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x - 4, Y: -5}, Second: &workspace.SketchPoint2{X: x + 4, Y: 5}}}})
				run(v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: operation, SketchID: sk, Length: 2})
			}
			publications := func(v workspace.DocumentView) map[string]workspace.Publication {
				out := map[string]workspace.Publication{}
				if v.Part != nil {
					for _, p := range v.Part.Publications {
						out[p.Name] = p
					}
				} else {
					for _, p := range v.Product.Publications {
						out[p.Name] = workspace.Publication{ID: p.ID, Type: p.Type, CompatibilityVersion: p.CompatibilityVersion, Resolution: p.Resolution}
					}
				}
				return out
			}
			source := func(name string, x, z float64) workspace.DocumentView {
				v := create("PART", name)
				box(&v, -45, "ADD")
				box(&v, 45, "NEW_BODY")
				run(&v, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
				plane := v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID
				run(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane})
				sk := v.Part.Features[len(v.Part.Features)-1].ID
				for j := 0; j < 4; j++ {
					px, py := x, -12.0
					if j%2 == 0 {
						px = -x
					}
					if j > 1 {
						py = 12
					}
					id := fmt.Sprintf("mount-%d", j)
					run(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: px, Y: py}}}}})
					run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: fmt.Sprintf("Point.%d", j), PublicationType: "POINT", TargetKind: "SKETCH_ENTITY", SketchID: sk, TargetID: id})
				}
				run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mount", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: plane})
				run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Reference", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
				if !strings.HasPrefix(mode, "PRODUCT") {
					return v
				}
				m := create("PRODUCT", name+" module")
				run(&m, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
				for _, p := range v.Part.Publications {
					q := path(m, 0)
					run(&m, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", Name: p.Name, InstancePath: &q, PublicationID: p.ID})
				}
				return m
			}
			a, b := source("Frame source A", 20, 0), source("Frame source B", 25, 1)
			pa, pb := publications(a), publications(b)
			plate := create("PART", "Four-hole receiver")
			run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			profile := plate.Part.Features[0].ID
			run(&plate, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -60, Y: -40}, Second: &workspace.SketchPoint2{X: 60, Y: 40}}}})
			thickness := 5.0
			if strings.HasSuffix(mode, "inconsistent") {
				thickness = 20
			}
			run(&plate, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: profile, Length: thickness})
			run(&plate, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			mount := plate.Part.DatumPlanes[len(plate.Part.DatumPlanes)-1].ID
			run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: mount})
			receiver := plate.Part.Features[len(plate.Part.Features)-1].ID
			targetPlane := "datum-xy"
			if strings.HasSuffix(mode, "inconsistent") {
				targetPlane = mount
			}
			run(&plate, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Receiver plane", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: targetPlane})
			other := create("PART", "Pending outside replacement")
			root := create("PRODUCT", "Physical replacement frames")
			for _, v := range []workspace.DocumentView{a, a, plate, other} {
				run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
			}
			run(&root, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: root.Product.Instances[3].ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
			// An unrelated accepted input precedes the five migrated inputs. Reversing
			// creation order exercises association order independently of instance order.
			bindingByName := map[string]string{}
			bind := func(name, kind, target string, sourceIndex int) {
				t.Helper()
				seq++
				v, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-bind-%d", root.Document.ID, seq), ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: path(root, 2), SourceInstancePath: path(root, sourceIndex), PublicationID: pa[name].ID, PublicationType: pa[name].Type, TargetKind: kind, TargetID: target, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
				if e != nil {
					t.Fatal(e)
				}
				root = v
				bindingByName[name] = root.Product.ContextBindings[len(root.Product.ContextBindings)-1].ID
			}
			bind("Point.0", "SKETCH_EXTERNAL_GEOMETRY", receiver, 0)
			untouchedID := bindingByName["Point.0"]
			order := []string{"Point.3", "Mount", "Point.1", "Point.0", "Point.2"}
			if strings.HasSuffix(mode, "direct") {
				slices.Reverse(order)
			}
			for _, name := range order {
				if name == "Mount" {
					bind(name, "DATUM", mount, 1)
				} else {
					bind(name, "SKETCH_EXTERNAL_GEOMETRY", receiver, 1)
				}
			}
			variantOf := func(v workspace.DocumentView) workspace.ContextVariantSnapshot {
				t.Helper()
				for _, c := range v.ContextVariants {
					if c.BaseDocumentID == plate.Document.ID {
						return c
					}
				}
				t.Fatal("missing variant")
				return workspace.ContextVariantSnapshot{}
			}
			scope := func(r workspace.CommandRequest) workspace.CommandRequest {
				v := variantOf(root)
				r.RootProductDocumentID = root.Document.ID
				r.RootProductRevisionID = root.Document.VersionID
				r.ContextVariantID = v.VariantKey
				r.InstancePath = &v.OwningInstancePath
				return r
			}
			ops := []workspace.SketchOperation{}
			for j := 0; j < 4; j++ {
				name := fmt.Sprintf("Point.%d", j)
				inputID := ""
				for _, link := range root.Product.ContextBindings {
					if link.ID == bindingByName[name] {
						inputID = link.ContextInputID
					}
				}
				ext := ""
				for _, f := range variantOf(root).Part.Features {
					if f.ID == receiver {
						for _, g := range f.Sketch.ExternalGeometry {
							if g.ContextInputID == inputID {
								ext = g.ID
							}
						}
					}
				}
				px, py := 20.0, -12.0
				if j%2 == 0 {
					px = -20
				}
				if j > 1 {
					py = 12
				}
				id := fmt.Sprintf("local-%d", j)
				ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: px, Y: py}}}, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "link-" + id, Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: id, SubElement: "POINT"}, {Target: "EXTERNAL", EntityID: ext, SubElement: "POINT"}}}})
			}
			run(&plate, scope(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: receiver, Operations: ops}))
			read(&root)
			holeIDs := []string{}
			for j := 0; j < 4; j++ {
				run(&plate, scope(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: plate.Part.ActiveBodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 3, HolePoint: &workspace.PatternPointReference{SketchID: receiver, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: fmt.Sprintf("local-%d", j), SubElement: "POINT"}}}}))
				holeIDs = append(holeIDs, plate.Part.Features[len(plate.Part.Features)-1].ID)
				read(&root)
			}
			fixed := 2
			constraintPlane := "Mount"
			if strings.HasSuffix(mode, "inconsistent") {
				fixed = 1
				constraintPlane = "Reference"
			}
			run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[fixed].ID, Kind: "BODY"}})
			run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[1].ID, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: pa[constraintPlane].ID, ExpectedType: "PLANE", CompatibilityVersion: pa[constraintPlane].CompatibilityVersion}}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[2].ID, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: plate.Part.Publications[0].ID, ExpectedType: "PLANE", CompatibilityVersion: plate.Part.Publications[0].CompatibilityVersion}}})
			run(&other, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "pending", Value: 8, Unit: "mm"})
			read(&root)
			before := root
			untouched := ""
			for _, v := range before.Product.ContextBindings {
				if v.ID == untouchedID {
					raw, _ := json.Marshal(v)
					untouched = string(raw)
				}
			}
			mapping := map[string]string{}
			for _, name := range []string{"Point.0", "Point.1", "Point.2", "Point.3", "Mount", constraintPlane} {
				mapping[pa[name].ID] = pb[name].ID
			}
			request := workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-physical-replacement", Type: "REPLACE_INSTANCE", VersionID: root.Document.VersionID, InstanceID: root.Product.Instances[1].ID, ReferencedDocumentID: b.Document.ID, ReplacementVersionID: b.Document.VersionID, PublicationMapping: mapping}
			preview, e := s.PreviewCommand(t.Context(), root.Document.ID, request)
			if strings.HasSuffix(mode, "inconsistent") {
				if e == nil || !strings.Contains(e.Error(), "REPLACEMENT_FRAME_INCONSISTENT") {
					t.Fatalf("inconsistent candidate: %v", e)
				}
				read(&root)
				if root.Document.VersionID != before.Document.VersionID {
					t.Fatal("failed preview advanced Head")
				}
				request.RequestID += "-direct"
				if _, e = s.ApplyCommand(t.Context(), root.Document.ID, request); e == nil || !strings.Contains(e.Error(), "REPLACEMENT_FRAME_INCONSISTENT") {
					t.Fatal("inconsistent direct commit", e)
				}
				read(&root)
				if root.Document.VersionID != before.Document.VersionID {
					t.Fatal("failed commit advanced Head")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if preview.ReplacementView == nil {
				t.Fatal("missing physical preview")
			}
			check := func(v workspace.DocumentView, wantB bool) {
				t.Helper()
				expectedX, expectedZ := 20.0, 0.0
				if wantB {
					expectedX, expectedZ = 25, -1
				}
				if math.Abs(v.Product.Instances[1].Translation[2]-expectedZ) > 1e-6 {
					t.Fatalf("source pose %v", v.Product.Instances[1].Translation)
				}
				for _, i := range []int{0, 3} {
					a, _ := json.Marshal(before.Product.Instances[i])
					b, _ := json.Marshal(v.Product.Instances[i])
					if string(a) != string(b) {
						t.Fatalf("unrelated instance %d changed", i)
					}
				}
				for _, link := range v.Product.ContextBindings {
					if link.ID == untouchedID {
						raw, _ := json.Marshal(link)
						if string(raw) != untouched {
							t.Fatal("unrelated accepted input changed")
						}
					} else if math.Abs(link.Transform.Translation[2]-expectedZ) > 1e-6 {
						t.Fatalf("input %s frame %v disagrees with final pose", link.Name, link.Transform)
					}
				}
				cv := variantOf(v)
				found := map[string]bool{}
				for _, f := range cv.Part.Features {
					if slices.Contains(holeIDs, f.ID) {
						found[f.ID] = true
						if f.EvaluationStatus != "" {
							t.Fatal("hole failed", f.EvaluationStatus)
						}
					}
					if f.ID == receiver {
						if math.Abs(f.Sketch.Support.Origin[2]) > 1e-6 {
							t.Fatalf("support not in final frame: %+v", f.Sketch.Support)
						}
						for _, p := range f.Sketch.Entities {
							if strings.HasPrefix(p.ID, "local-") && math.Abs(math.Abs(p.Point.X)-expectedX) > 1e-6 {
								t.Fatal("incorrect solved point", p.Point)
							}
						}
					}
				}
				if len(found) != 4 {
					t.Fatal("Hole identity lost")
				}
				art := v.Artifacts[cv.Bodies[0].GeometryKey]
				if math.Abs(art.Volume-(120*80*thickness-4*3*math.Pi*4)) > 1e-4 {
					t.Fatal("incorrect exact volume", art.Volume)
				}
				_, reader, e := store.Open(t.Context(), art.Representations["BREP"].ObjectID)
				if e != nil {
					t.Fatal(e)
				}
				brep, e := io.ReadAll(reader)
				reader.Close()
				if e != nil {
					t.Fatal(e)
				}
				topology, _, e := client.GetTopology(t.Context(), art.GeometryID, brep, "FACE", 0)
				if e != nil {
					t.Fatal(e)
				}
				walls := 0
				for _, face := range topology.Faces {
					if face.SurfaceType != 1 {
						continue
					}
					walls++
					bb := face.Bbox
					if math.Abs(bb.MinZ) > 1e-5 || math.Abs(bb.MaxZ-3) > 1e-5 || math.Abs(math.Abs((bb.MinX+bb.MaxX)/2)-expectedX) > 1e-5 {
						t.Fatalf("physical hole wall in wrong position: %+v", bb)
					}
				}
				if walls != 4 {
					t.Fatal("missing precise hole walls", walls)
				}
				if strings.HasPrefix(mode, "PRODUCT") && wantB {
					count := 0
					for _, r := range v.ResolvedInstances {
						if len(r.InstancePath.Segments) > 0 && r.InstancePath.Segments[0].InstanceID == v.Product.Instances[1].ID {
							count++
							if math.Abs(r.Translation[2]+1) > 1e-6 {
								t.Fatal("preview module body not at solved pose")
							}
						}
					}
					if count != 2 {
						t.Fatalf("multi-Body module lost bodies: %d", count)
					}
				}
			}
			check(*preview.ReplacementView, true)
			read(&root)
			if root.Document.VersionID != before.Document.VersionID {
				t.Fatal("preview advanced Head")
			}
			if strings.HasSuffix(mode, "direct") {
				s.DiscardPreview(root.Document.ID, p6Actor, preview.PreviewID)
			} else {
				request.PreviewID = preview.PreviewID
			}
			run(&root, request)
			check(root, true)
			run(&root, workspace.CommandRequest{Type: "UNDO"})
			check(root, false)
			run(&root, workspace.CommandRequest{Type: "REDO"})
			check(root, true)
			read(&root)
			check(root, true)
			read(&plate)
			feature := *func() *workspace.Feature {
				for i := range plate.Part.Features {
					if plate.Part.Features[i].ID == holeIDs[0] {
						return &plate.Part.Features[i]
					}
				}
				t.Fatal("hole absent")
				return nil
			}()
			feature.Diameter = 5
			edit := scope(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, plate.StructureTree, feature.ID)})
			seq++
			edit.ActorID = p6Actor
			edit.RequestID = fmt.Sprintf("%s-edit-original-%d", plate.Document.ID, seq)
			candidate, e := s.PreviewCommand(t.Context(), plate.Document.ID, edit)
			if e != nil {
				t.Fatal("original Hole edit", e)
			}
			s.DiscardPreview(plate.Document.ID, p6Actor, candidate.PreviewID)
		})
	}
}

// Receiver migration consumes accepted inputs, not pending source Heads. Exact
// cylinder walls verify the result, including a receiver with a different frame.
func TestReplaceReceiverPreparedInputsAndPhysicalGeometry(t *testing.T) {
	s, store, _, client, _ := featureAssociationRuntimeService(t)
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
		r.RequestID = fmt.Sprintf("%s-receiver-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatalf("%s %s: %v", r.Type, r.Name, e)
		}
		*v = n
	}
	read := func(v *workspace.DocumentView) {
		t.Helper()
		n, e := s.GetDocument(t.Context(), v.Document.ID, p6Actor)
		if e != nil {
			t.Fatal(e)
		}
		*v = n
	}
	path := func(v workspace.DocumentView, index int) workspace.InstancePath {
		t.Helper()
		x, e := s.GetProductDesignSession(t.Context(), v.Document.ID, v.Product.Instances[index].ID)
		if e != nil {
			t.Fatal(e)
		}
		return *x.ActiveInstancePath
	}
	source := create("PART", "Receiver fixture layout")
	run(&source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "cutDepth", Value: 3, Unit: "mm"})
	sourceParam := source.Part.Parameters[len(source.Part.Parameters)-1].ParameterID
	run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Depth", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: sourceParam})
	run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mount", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
	run(&source, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sourceSketch := source.Part.Features[0].ID
	for j := 0; j < 4; j++ {
		x, y := -20., -12.
		if j%2 == 1 {
			x = 20
		}
		if j > 1 {
			y = 12
		}
		id := fmt.Sprintf("source-point-%d", j)
		run(&source, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sourceSketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: x, Y: y}}}}})
		run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: fmt.Sprintf("Point.%d", j), PublicationType: "POINT", TargetKind: "SKETCH_ENTITY", SketchID: sourceSketch, TargetID: id})
	}
	ports := map[string]workspace.Publication{}
	for _, p := range source.Part.Publications {
		ports[p.Name] = p
	}
	type prepared struct {
		v      workspace.DocumentView
		inputs map[string]string
		holes  []string
		sketch string
		mount  string
	}
	prepare := func(name string, rotated bool) prepared {
		v := create("PART", name)
		n := [3]float64{0, 0, 1}
		if rotated {
			n = [3]float64{0, -1, 0}
		}
		run(&v, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: n, UDirection: [3]float64{1, 0, 0}})
		install := v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID
		run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Installation", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: install})
		run(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: install})
		sk := v.Part.Features[len(v.Part.Features)-1].ID
		run(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -60, Y: -40}, Second: &workspace.SketchPoint2{X: 60, Y: 40}}}})
		run(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", SketchID: sk, Operation: "ADD", Length: 8})
		run(&v, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: n, UDirection: [3]float64{1, 0, 0}})
		mount := v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID
		run(&v, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: "Mount", TargetKind: "DATUM", TargetID: mount, PublicationType: "PLANE", Required: true})
		run(&v, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "cutDepth", Value: 2, Unit: "mm"})
		param := ""
		for _, p := range v.Part.Parameters {
			if p.DisplayAlias == "cutDepth" {
				param = p.ParameterID
			}
		}
		run(&v, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: "Depth", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: param, Required: true})
		run(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: mount})
		sk = v.Part.Features[len(v.Part.Features)-1].ID
		holes := []string{}
		for j := 0; j < 4; j++ {
			x, y := -20., -12.
			if j%2 == 1 {
				x = 20
			}
			if j > 1 {
				y = 12
			}
			if rotated {
				x = x * 1.25
			}
			id := fmt.Sprintf("%s-point-%d", v.Document.ID, j)
			run(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: x, Y: y}}}}})
			run(&v, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: fmt.Sprintf("Point.%d", j), PublicationType: "POINT", TargetKind: "SKETCH_EXTERNAL_GEOMETRY", TargetID: sk, SketchID: sk, OwnerEntityID: id, ConnectInputPoint: true, Required: true})
			run(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: v.Part.ActiveBodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 2, HolePoint: &workspace.PatternPointReference{SketchID: sk, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}}}, ParameterExpressions: map[string]string{"depth": "cutDepth"}})
			holes = append(holes, v.Part.Features[len(v.Part.Features)-1].ID)
		}
		run(&v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Accepted hole position", PublicationType: "POINT", TargetKind: "SKETCH_ENTITY", SketchID: sk, TargetID: fmt.Sprintf("%s-point-0", v.Document.ID)})
		inputs := map[string]string{}
		for _, p := range v.Part.ContextInputs {
			inputs[p.Name] = p.ID
		}
		return prepared{v, inputs, holes, sk, mount}
	}
	old, next := prepare("Original prepared plate", false), prepare("Rotated replacement plate", true)
	// An additional optional plane retains the replacement's standalone default.
	run(&next.v, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, -2, 0}, Normal: [3]float64{0, -1, 0}, UDirection: [3]float64{1, 0, 0}})
	extra := next.v.Part.DatumPlanes[len(next.v.Part.DatumPlanes)-1].ID
	run(&next.v, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: "Optional plane", TargetKind: "DATUM", TargetID: extra, PublicationType: "PLANE"})
	optionalID := next.v.Part.ContextInputs[len(next.v.Part.ContextInputs)-1].ID

	root := create("PRODUCT", "Receiver replacement actual holes")
	for _, v := range []workspace.DocumentView{source, old.v, old.v} {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	run(&root, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: root.Product.Instances[0].ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	for _, name := range []string{"Point.3", "Mount", "Point.1", "Depth", "Point.0", "Point.2"} {
		seq++
		v, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-receiver-bind-%d", root.Document.ID, seq), ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: path(root, 1), SourceInstancePath: path(root, 0), PublicationID: ports[name].ID, PublicationType: ports[name].Type, ContextInputID: old.inputs[name], ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
		if e != nil {
			t.Fatalf("bind %s: %v", name, e)
		}
		root = v
	}
	run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[0].ID, Kind: "BODY"}})
	run(&root, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[0].ID, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: ports["Mount"].ID, ExpectedType: "PLANE", CompatibilityVersion: ports["Mount"].CompatibilityVersion}}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: root.Product.Instances[1].ID, Kind: "PLANE", PublicationRef: &workspace.PublicationRef{PublicationID: old.v.Part.Publications[0].ID, ExpectedType: "PLANE", CompatibilityVersion: old.v.Part.Publications[0].CompatibilityVersion}}})
	forwardPath := path(root, 1)
	run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", Name: "Plate hole", InstancePath: &forwardPath, PublicationID: old.v.Part.Publications[1].ID})
	run(&source, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: sourceParam, Value: 4, Unit: "mm"})
	read(&root)
	before := root
	mapping := map[string]string{}
	for name, id := range old.inputs {
		mapping[id] = next.inputs[name]
	}
	req := workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-replace-receiver", Type: "REPLACE_INSTANCE", VersionID: root.Document.VersionID, InstanceID: root.Product.Instances[1].ID, ReferencedDocumentID: next.v.Document.ID, ReplacementVersionID: next.v.Document.VersionID, ContextInputMapping: mapping, PublicationMapping: map[string]string{old.v.Part.Publications[0].ID: next.v.Part.Publications[0].ID, old.v.Part.Publications[1].ID: next.v.Part.Publications[1].ID}}
	// Both missing and duplicate input maps must fail atomically, before evaluation.
	for _, duplicate := range []bool{false, true} {
		bad := req
		bad.RequestID += fmt.Sprint(duplicate)
		bad.ContextInputMapping = map[string]string{}
		for a, b := range mapping {
			bad.ContextInputMapping[a] = b
		}
		if duplicate {
			bad.ContextInputMapping[old.inputs["Point.1"]] = next.inputs["Point.0"]
		} else {
			delete(bad.ContextInputMapping, old.inputs["Depth"])
		}
		if _, e := s.PreviewCommand(t.Context(), root.Document.ID, bad); e == nil {
			t.Fatal("invalid input map accepted")
		}
		read(&root)
		if root.Document.VersionID != before.Document.VersionID {
			t.Fatal("failed candidate changed Head")
		}
	}
	run(&next.v, workspace.CommandRequest{Type: "EDIT_CONTEXT_INPUT", ContextInputID: optionalID, Name: "Optional plane", Required: true})
	badRequired := req
	badRequired.RequestID += "-required"
	badRequired.ReplacementVersionID = next.v.Document.VersionID
	if _, e := s.PreviewCommand(t.Context(), root.Document.ID, badRequired); e == nil || !strings.Contains(e.Error(), "REPLACEMENT_REQUIRED_INPUT_MISSING") {
		t.Fatal("unbound required input accepted", e)
	}
	run(&next.v, workspace.CommandRequest{Type: "EDIT_CONTEXT_INPUT", ContextInputID: optionalID, Name: "Optional plane", Required: false})
	// A support that itself consumes a migrated port cannot be used to bypass
	// the initial failure, even though its standalone definition is healthy.
	run(&next.v, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Dependent installation", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: next.mount})
	req.ReplacementVersionID = next.v.Document.VersionID
	dependent := req
	dependent.RequestID += "-dependent"
	dependent.PublicationMapping = map[string]string{old.v.Part.Publications[0].ID: next.v.Part.Publications[len(next.v.Part.Publications)-1].ID, old.v.Part.Publications[1].ID: next.v.Part.Publications[1].ID}
	if _, e := s.PreviewCommand(t.Context(), root.Document.ID, dependent); e == nil || !strings.Contains(e.Error(), "REPLACEMENT_PLACEMENT_UNAVAILABLE") {
		t.Fatal("dependent installation bypass accepted", e)
	}
	read(&root)
	if root.Document.VersionID != before.Document.VersionID {
		t.Fatal("failed placement advanced Head")
	}
	preview, e := s.PreviewCommand(t.Context(), root.Document.ID, req)
	if e != nil {
		t.Fatal(e)
	}
	if preview.ReplacementView == nil {
		t.Fatal("no receiver preview")
	}
	check := func(v workspace.DocumentView) {
		t.Helper()
		if v.Product.Instances[1].ReferencedDocumentID != next.v.Document.ID {
			t.Fatal("not replaced")
		}
		for _, i := range []int{0, 2} {
			a, _ := json.Marshal(v.Product.Instances[i])
			b, _ := json.Marshal(before.Product.Instances[i])
			if string(a) != string(b) {
				t.Fatal("unrelated instance changed", i)
			}
		}
		for j, b := range v.Product.ContextBindings {
			a := before.Product.ContextBindings[j]
			expected := a
			expected.ContextInputID = b.ContextInputID
			expected.OwningInstancePath = b.OwningInstancePath
			expected.Transform = b.Transform
			expected.Accepted.RootProductRevisionID = b.Accepted.RootProductRevisionID
			expected.Accepted.OwningRevisionID = b.Accepted.OwningRevisionID
			raw, _ := json.Marshal(b)
			want, _ := json.Marshal(expected)
			if string(raw) != string(want) {
				t.Fatal("accepted input changed beyond receiver migration")
			}
			x, _ := json.Marshal(b.SourceInstancePath)
			y, _ := json.Marshal(a.SourceInstancePath)
			if b.ID != a.ID || b.ContextInputID != mapping[a.ContextInputID] || b.Accepted.SourceRevisionID != a.Accepted.SourceRevisionID || string(x) != string(y) {
				t.Fatal("accepted source or binding identity changed")
			}
		}
		var cv *workspace.ContextVariantSnapshot
		for i := range v.ContextVariants {
			if v.ContextVariants[i].OwningInstancePath.Segments[0].InstanceID == v.Product.Instances[1].ID {
				cv = &v.ContextVariants[i]
			}
		}
		if len(v.Product.Publications) != 1 || v.Product.Publications[0].Resolution.Status != "CONNECTED" || math.Abs(v.Product.Publications[0].Resolution.Origin[0]+20) > 1e-5 || math.Abs(v.Product.Publications[0].Resolution.Origin[1]+12) > 1e-5 || math.Abs(v.Product.Publications[0].Resolution.Origin[2]) > 1e-5 {
			t.Fatal("forward did not use accepted receiver geometry", v.Product.Publications)
		}
		if cv == nil || cv.Part == nil || cv.Status != "READY" {
			t.Fatal("receiver not evaluated")
		}
		optional := slices.IndexFunc(cv.Part.DatumPlanes, func(d workspace.DatumPlane) bool { return d.ID == extra })
		if optional < 0 || cv.Part.DatumPlanes[optional].Origin != [3]float64{0, -2, 0} {
			t.Fatal("unbound optional input lost its own default")
		}
		for _, f := range cv.Part.Features {
			if slices.Contains(next.holes, f.ID) && f.EvaluationStatus != "" {
				t.Fatal("new hole failed")
			}
			if f.ID == next.sketch {
				for _, p := range f.Sketch.Entities {
					if p.Point != nil && strings.Contains(p.ID, "-point-") && (math.Abs(math.Abs(p.Point.X)-20) > 1e-5 || math.Abs(math.Abs(p.Point.Y)-12) > 1e-5) {
						t.Fatal("wrong projected point", p.Point)
					}
				}
			}
		}
		art := v.Artifacts[cv.Bodies[0].GeometryKey]
		if math.Abs(art.Volume-(120*80*8-4*3*math.Pi*4)) > 1e-4 {
			t.Fatal("wrong exact result volume", art.Volume)
		}
		_, reader, e := store.Open(t.Context(), art.Representations["BREP"].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(reader)
		reader.Close()
		if e != nil {
			t.Fatal(e)
		}
		top, _, e := client.GetTopology(t.Context(), art.GeometryID, raw, "FACE", 0)
		if e != nil {
			t.Fatal(e)
		}
		walls := 0
		for _, f := range top.Faces {
			if f.SurfaceType == 1 {
				walls++
				bb := f.Bbox
				if math.Abs(math.Abs((bb.MinX+bb.MaxX)/2)-20) > 1e-5 || math.Abs(math.Abs((bb.MinZ+bb.MaxZ)/2)-12) > 1e-5 || math.Abs(bb.MinY+3) > 1e-5 || math.Abs(bb.MaxY) > 1e-5 {
					t.Fatal("wrong exact rotated hole wall", bb)
				}
			}
		}
		if walls != 4 {
			t.Fatal("missing physical hole walls", walls)
		}
	}
	check(*preview.ReplacementView)
	read(&root)
	if root.Document.VersionID != before.Document.VersionID {
		t.Fatal("preview changed Head")
	}
	s.DiscardPreview(root.Document.ID, p6Actor, preview.PreviewID)
	// Direct commit independently runs the same installation and final checks.
	req.RequestID += "-direct"
	run(&root, req)
	check(root)
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if root.Product.Instances[1].ReferencedDocumentID != old.v.Document.ID {
		t.Fatal("undo receiver")
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	check(root)
	read(&root)
	check(root)
	read(&next.v)
	cv := root.ContextVariants[0]
	for _, v := range root.ContextVariants {
		if v.BaseDocumentID == next.v.Document.ID {
			cv = v
		}
	}
	f := workspace.Feature{}
	for _, feature := range next.v.Part.Features {
		if feature.ID == next.holes[0] {
			f = feature
		}
	}
	f.Diameter = 5
	run(&next.v, workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, next.v.StructureTree, f.ID), RootProductDocumentID: root.Document.ID, RootProductRevisionID: root.Document.VersionID, ContextVariantID: cv.VariantKey, InstancePath: &cv.OwningInstancePath})
	read(&root)
}
