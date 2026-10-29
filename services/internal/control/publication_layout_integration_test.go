//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"io"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestLayoutPublicationsDriveLocalPointsAndHolesThroughRouter(t *testing.T) {
	s, _, db, _, _ := featureAssociationRuntimeService(t)
	seq := 0
	value := func(v float64) *float64 { return &v }
	create := func(name, kind string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Name: name, Type: kind})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-layout-%d", v.Document.ID, seq)
		}
		r.ActorID = p6Actor
		updated, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = updated
	}
	layout := create("Layout without solid", "PART")
	apply(&layout, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "pitch", Value: 120, Unit: "mm"})
	pitch := layout.Part.Parameters[0].ParameterID
	apply(&layout, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sourceSketch := layout.Part.Features[0].ID
	for i, p := range [][2]float64{{-60, -30}, {60, -30}, {60, 30}, {-60, 30}} {
		id := fmt.Sprintf("location-%d", i)
		apply(&layout, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sourceSketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: p[0], Y: p[1]}}}}})
		point := workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}
		origin := workspace.SketchGeometryRef{Target: "SKETCH_ORIGIN", SubElement: "POINT"}
		refs := []workspace.SketchGeometryRef{origin, point}
		if p[0] < 0 {
			refs = []workspace.SketchGeometryRef{point, origin}
		}
		expression := "pitch/2"
		apply(&layout, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sourceSketch, Operations: []workspace.SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: id + "-x", Kind: "HORIZONTAL_DISTANCE", References: refs, Value: value(60), Unit: "mm"}, ParameterSource: &expression}}})
		refs = []workspace.SketchGeometryRef{origin, point}
		if p[1] < 0 {
			refs = []workspace.SketchGeometryRef{point, origin}
		}
		apply(&layout, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sourceSketch, Operations: []workspace.SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: id + "-y", Kind: "VERTICAL_DISTANCE", References: refs, Value: value(30), Unit: "mm"}}}})
		apply(&layout, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Hole location " + id, PublicationType: "POINT", TargetKind: "SKETCH_ENTITY", SketchID: sourceSketch, TargetID: id})
	}
	apply(&layout, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mount", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
	apply(&layout, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Pitch", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: pitch})
	if len(layout.Part.Features) != 1 || len(layout.Part.Publications) != 6 {
		t.Fatal("layout must have no solid features")
	}
	plates := []workspace.DocumentView{create("Base plate", "PART"), create("Cover plate", "PART")}
	planes := []string{}
	sketches := []string{}
	for i := range plates {
		v := &plates[i]
		apply(v, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
		planes = append(planes, v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID)
		apply(v, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: planes[i]})
		profile := v.Part.Features[0].ID
		apply(v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -150, Y: -150}, Second: &workspace.SketchPoint2{X: 150, Y: 150}}}})
		apply(v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: profile, Length: 5})
		apply(v, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: planes[i]})
		sketches = append(sketches, v.Part.Features[len(v.Part.Features)-1].ID)
		apply(v, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "local_pitch", Value: 100, Unit: "mm"})
		for _, parameter := range v.Part.Parameters {
			if parameter.PropertySlot == "length" {
				apply(v, workspace.CommandRequest{Type: "SET_PARAMETER_EXPRESSION", ParameterID: parameter.ParameterID, Expression: "local_pitch/20"})
				break
			}
		}

	}
	product := create("Layout driven plates", "PRODUCT")
	for _, id := range []string{layout.Document.ID, plates[0].Document.ID, plates[1].Document.ID} {
		apply(&product, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: id})
	}
	apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: product.Product.Instances[0].ID, Translation: [3]float64{10, 20, 0}, Rotation: [4]float64{math.Sin(.15), 0, 0, math.Cos(.15)}})
	for i := 1; i < 3; i++ {
		apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: product.Product.Instances[i].ID, Translation: [3]float64{float64(i) * 20, 30, 0}, Rotation: [4]float64{0, 0, math.Sin(.4), math.Cos(.4)}})
	}
	path := func(instance int) workspace.InstancePath {
		t.Helper()
		session, e := s.GetProductDesignSession(t.Context(), product.Document.ID, product.Product.Instances[instance].ID)
		if e != nil || session.ActiveInstancePath == nil {
			t.Fatal("path", e)
		}
		return *session.ActiveInstancePath
	}
	// Path entries in DesignSession may use a named field; see compile contract.
	for plate := 0; plate < 2; plate++ {
		bind := func(publication int, kind, target string) {
			t.Helper()
			seq++
			request := workspace.ProductContextBindingRequest{ExpectedRootRevisionID: product.Document.VersionID, RequestID: fmt.Sprintf("%s-binding-%d", product.Document.ID, seq), ActorID: p6Actor, OwningInstancePath: path(plate + 1), SourceInstancePath: path(0), PublicationID: layout.Part.Publications[publication].ID, PublicationType: layout.Part.Publications[publication].Type, TargetKind: kind, TargetID: target, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"}
			if plate == 0 && publication == 0 {
				before, e := s.GetDocument(t.Context(), plates[plate].Document.ID, p6Actor)
				if e != nil {
					t.Fatal(e)
				}
				request.Preview = true
				candidate, e := s.CreateProductContextBinding(t.Context(), product.Document.ID, request)
				if e != nil || candidate.CommandPreview == nil {
					t.Fatal("binding preview", e)
				}
				unchanged, e := s.GetDocument(t.Context(), plates[plate].Document.ID, p6Actor)
				if e != nil || unchanged.Document.VersionID != before.Document.VersionID || len(unchanged.Part.ContextInputs) != len(before.Part.ContextInputs) {
					t.Fatal("preview persisted a half port", e)
				}
				request.Preview = false
				request.PreviewID = candidate.CommandPreview.PreviewID
			}
			v, e := s.CreateProductContextBinding(t.Context(), product.Document.ID, request)
			if e != nil {
				t.Fatal("binding", kind, e)
			}
			product = v
		}
		bind(4, "DATUM", planes[plate])
		for _, p := range plates[plate].Part.Parameters {
			if p.DisplayAlias == "local_pitch" {
				bind(5, "PARAMETER", p.ParameterID)
			}
		}

		for j := 0; j < 4; j++ {
			bind(j, "SKETCH_EXTERNAL_GEOMETRY", sketches[plate])
		}
		var variant workspace.ContextVariantSnapshot
		for _, v := range product.ContextVariants {
			if v.BaseDocumentID == plates[plate].Document.ID {
				variant = v
			}
		}
		if variant.Part == nil {
			t.Fatal("no authoritative editing projection")
		}
		if plate == 0 {
			readonly := workspace.CommandRequest{Type: "EDIT_DATUM_PLANE", RequestID: product.Document.ID + "-readonly-plane", ActorID: p6Actor, TargetID: planes[plate], Name: "Should not overwrite defaults", RootProductDocumentID: product.Document.ID, RootProductRevisionID: product.Document.VersionID, ContextVariantID: variant.VariantKey, InstancePath: &variant.OwningInstancePath}
			if _, err := s.PreviewCommand(t.Context(), plates[plate].Document.ID, readonly); err == nil || !strings.Contains(err.Error(), "CONTEXT_INPUT_VALUE_READ_ONLY") {
				t.Fatal("bound datum accepted a definition write", err)
			}
		}
		var ports []workspace.SketchExternalGeometry
		for _, f := range variant.Part.Features {
			if f.ID == sketches[plate] {
				ports = f.Sketch.ExternalGeometry
			}
		}
		if len(ports) != 4 {
			t.Fatal("local input slots missing", len(ports))
		}
		for j, port := range ports {
			v, e := s.GetDocument(t.Context(), plates[plate].Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			var scope workspace.ContextVariantSnapshot
			for _, candidate := range product.ContextVariants {
				if candidate.BaseDocumentID == v.Document.ID {
					scope = candidate
				}
			}
			r := workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketches[plate], RootProductDocumentID: product.Document.ID, RootProductRevisionID: product.Document.VersionID, ContextVariantID: scope.VariantKey, InstancePath: &scope.OwningInstancePath, Operations: []workspace.SketchOperation{
				{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: fmt.Sprintf("local-%d", j), Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}},
				{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: fmt.Sprintf("join-%d", j), Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: fmt.Sprintf("local-%d", j), SubElement: "POINT"}, {Target: "EXTERNAL", EntityID: port.ID, SubElement: "POINT"}}}},
			}}
			seq++
			r.RequestID = fmt.Sprintf("%s-context-%d", v.Document.ID, seq)
			r.ActorID = p6Actor
			preview, e := s.PreviewCommand(t.Context(), v.Document.ID, r)
			if e != nil {
				t.Fatal("context sketch preview", e)
			}
			r.PreviewID = preview.PreviewID
			apply(&v, r)
			product, e = s.GetDocument(t.Context(), product.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			for _, candidate := range product.ContextVariants {
				if candidate.BaseDocumentID == v.Document.ID {
					scope = candidate
				}
			}
			apply(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", RootProductDocumentID: product.Document.ID, RootProductRevisionID: product.Document.VersionID, ContextVariantID: scope.VariantKey, InstancePath: &scope.OwningInstancePath, Feature: &workspace.Feature{Type: "HOLE", BodyID: v.Part.ActiveBodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 2, HolePoint: &workspace.PatternPointReference{SketchID: sketches[plate], Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: fmt.Sprintf("local-%d", j), SubElement: "POINT"}}}})
			product, e = s.GetDocument(t.Context(), product.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	// The final contextual Hole and both workspace references form one history action.
	lastHole := product.ContextVariants[len(product.ContextVariants)-1].Part.Features
	lastID := lastHole[len(lastHole)-1].ID
	apply(&product, workspace.CommandRequest{Type: "UNDO"})
	historyPlan, historyErr := s.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if historyErr != nil || historyPlan.HasUpdates {
		t.Fatal("atomic Undo left references behind member compensation Heads", historyErr)
	}
	apply(&product, workspace.CommandRequest{Type: "REDO"})
	apply(&product, workspace.CommandRequest{Type: "UNDO"})
	historyPlan, historyErr = s.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if historyErr != nil || historyPlan.HasUpdates {
		t.Fatal("repeated Undo introduced an automatic follow action", historyErr)
	}
	apply(&product, workspace.CommandRequest{Type: "REDO"})
	found := false
	for _, variant := range product.ContextVariants {
		for _, f := range variant.Part.Features {
			found = found || f.ID == lastID
		}
	}
	if !found {
		t.Fatal("contextual Undo/Redo lost Hole identity")
	}
	apply(&layout, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: pitch, Value: 140, Unit: "mm"})
	plan, e := s.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if e != nil || !plan.HasUpdates || !plan.CanAccept {
		t.Fatal("update plan", e, plan)
	}
	apply(&product, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	for _, variant := range product.ContextVariants {
		if variant.Part == nil || variant.Status != "READY" {
			t.Fatal("accepted input did not produce a real Part")
		}
		for _, f := range variant.Part.Features {
			if f.Type == "LINEAR_EXTRUDE" && math.Abs(f.Length-7) > 1e-6 {
				t.Fatal("bound parameter did not drive Feature", f.Length)
			}
		}
		holes := 0
		for _, f := range variant.Part.Features {
			if f.Type == "HOLE" {
				holes++
				if f.EvaluationStatus != "" {
					t.Fatal("Hole failed", f.Diagnostic)
				}
			}
			if f.Sketch != nil {
				for _, p := range f.Sketch.Entities {
					if p.Kind == "POINT" && len(p.ID) > 6 && p.ID[:6] == "local-" && math.Abs(math.Abs(p.Point.X)-70) > 1e-5 {
						t.Fatal("accepted Hole location did not update", p)
					}
				}
			}
		}
		if holes != 4 {
			t.Fatal("Hole feature loop missing", holes)
		}
	}
	// Independent reads retain saved defaults, not the accepted 140 mm projection.
	base, e := s.GetDocument(t.Context(), plates[0].Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	if len(base.Part.ContextReferences) != 0 {
		t.Fatal("transient binding projection was written into shared Part")
	}
	for _, f := range base.Part.Features {
		if f.Sketch != nil {
			for _, port := range f.Sketch.ExternalGeometry {
				if port.ContextInputID == "" {
					continue
				}
				var input workspace.ContextInput
				for _, p := range base.Part.ContextInputs {
					if p.ID == port.ContextInputID {
						input = p
					}
				}
				if input.Default == nil || input.Default.External == nil {
					t.Fatal("missing saved input default")
				}
				a, _ := json.Marshal(port.Snapshot)
				b, _ := json.Marshal(input.Default.External)
				if string(a) != string(b) {
					t.Fatal("independent Part consumed a Product input")
				}
			}
		}
	}
	// Binding mode edits preserve the accepted input; only explicit update adopts a new source.
	binding := product.Product.ContextBindings[1]
	apply(&product, workspace.CommandRequest{Type: "EDIT_CONTEXT_BINDING", VersionID: product.Document.VersionID, ContextBindingID: binding.ID, ReferenceMode: "PINNED", Name: "Fixed hole input"})
	pinnedRevision := product.Product.ContextBindings[1].Accepted.SourceRevisionID
	apply(&layout, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: pitch, Value: 160, Unit: "mm"})
	plan, e = s.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if e != nil || !plan.CanAccept {
		t.Fatal("pinned plan", e)
	}
	apply(&product, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	if product.Product.ContextBindings[1].Accepted.SourceRevisionID != pinnedRevision {
		t.Fatal("fixed input followed Head")
	}
	apply(&product, workspace.CommandRequest{Type: "EDIT_CONTEXT_BINDING", VersionID: product.Document.VersionID, ContextBindingID: binding.ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	plan, e = s.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if e != nil || !plan.CanAccept || !plan.HasUpdates {
		t.Fatal("resume plan", e)
	}
	apply(&product, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	if product.Product.ContextBindings[1].Accepted.SourceRevisionID != layout.Document.VersionID {
		t.Fatal("restored input did not update")
	}
	// A repeated definition reuses the same Part-owned input, not a Binding-derived slot.
	apply(&product, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: plates[0].Document.ID})
	seq++
	request := workspace.ProductContextBindingRequest{RequestID: fmt.Sprintf("%s-repeat-%d", product.Document.ID, seq), ActorID: p6Actor, ContextInputID: binding.ContextInputID, ExpectedRootRevisionID: product.Document.VersionID, OwningInstancePath: path(3), SourceInstancePath: path(0), PublicationID: binding.Publication.PublicationID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"}
	product, e = s.CreateProductContextBinding(t.Context(), product.Document.ID, request)
	if e != nil {
		t.Fatal("repeat port", e)
	}
	if product.Product.ContextBindings[len(product.Product.ContextBindings)-1].ContextInputID != binding.ContextInputID {
		t.Fatal("repeated input identity changed")
	}
	// A shared-definition edit must leave a PINNED repeated occurrence at its accepted base.
	apply(&product, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: product.Product.Instances[3].ID, ReferenceMode: "PINNED"})
	frozenBase := product.Product.Instances[3].ReferencedVersionID
	original, e := s.GetDocument(t.Context(), plates[0].Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	var active workspace.ContextVariantSnapshot
	for _, v := range product.ContextVariants {
		if v.OwningInstancePath.Canonical == product.Product.Instances[1].ID {
			active = v
		}
	}
	holeID := ""
	for _, f := range original.Part.Features {
		if f.Type == "HOLE" {
			holeID = f.ID
			break
		}
	}
	parameterID := ""
	for _, p := range original.Part.Parameters {
		if p.OwnerFeatureID == holeID && p.PropertySlot == "diameter" {
			parameterID = p.ParameterID
		}
	}
	if parameterID == "" {
		t.Fatal("missing stable Hole diameter parameter")
	}
	apply(&original, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameterID, Value: 4.5, Unit: "mm", RootProductDocumentID: product.Document.ID, RootProductRevisionID: product.Document.VersionID, ContextVariantID: active.VariantKey, InstancePath: &active.OwningInstancePath})
	product, e = s.GetDocument(t.Context(), product.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	if product.Product.Instances[3].ReferencedVersionID != frozenBase {
		t.Fatal("pinned occurrence followed edited definition")
	}
	for _, b := range product.Product.ContextBindings {
		if b.OwningInstancePath.Canonical == product.Product.Instances[3].ID && (b.Accepted.OwningRevisionID != frozenBase || b.OwningInstancePath.Segments[0].ResolvedVersionID != frozenBase) {
			t.Fatal("pinned binding provenance was rewritten")
		}
	}
	before := map[string][]string{}
	for _, v := range product.ContextVariants {
		for _, b := range v.Bodies {
			before[v.OwningInstancePath.Canonical] = append(before[v.OwningInstancePath.Canonical], b.GeometryKey)
		}
	}
	// Clear only this fixture's derived variants and use a fresh production Worker.
	for _, plate := range plates {
		if _, e = db.Exec(t.Context(), `DELETE FROM occccad.product_context_variants WHERE base_document_id=$1`, plate.Document.ID); e != nil {
			t.Fatal(e)
		}
	}
	cold, _, _, _, _ := geometryRuntimeTestService(t, db.(*database.Pool))
	reopened, e := cold.GetDocument(t.Context(), product.Document.ID, p6Actor)
	if e != nil {
		t.Fatal("cold rebuild", e)
	}
	for _, v := range reopened.ContextVariants {
		if v.Part == nil || v.Status != "READY" {
			t.Fatal("cold context unavailable")
		}
		keys := []string{}
		for _, b := range v.Bodies {
			keys = append(keys, b.GeometryKey)
		}
		a, _ := json.Marshal(keys)
		b, _ := json.Marshal(before[v.OwningInstancePath.Canonical])
		if string(a) != string(b) {
			t.Fatal("warm/cold geometry mismatch", v.OwningInstancePath.Display)
		}
	}

}

func TestNativeLayoutCurvesUseExactProjectionAndStableInputSlots(t *testing.T) {
	s, _, _, _, _ := featureAssociationRuntimeService(t)
	create := func(name string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Name: name, Type: "PART"})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	seq := 0
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-curves-%d", v.Document.ID, seq)
		next, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = next
	}
	source := create("Native layout curves")
	apply(&source, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := source.Part.Features[0].ID
	point := func(x, y float64) *workspace.SketchPoint2 { return &workspace.SketchPoint2{X: x, Y: y} }
	entities := []workspace.SketchEntity{{ID: "line", Kind: "LINE", Role: "CONSTRUCTION", Start: point(0, 0), End: point(20, 0)}, {ID: "circle", Kind: "CIRCLE", Role: "CONSTRUCTION", Center: point(30, 0), Radius: 5}, {ID: "arc", Kind: "ARC", Role: "CONSTRUCTION", Center: point(0, 30), Radius: 10, StartAngle: 0, EndAngle: math.Pi / 2}}
	for _, entity := range entities {
		apply(&source, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &entity}}})
		apply(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "CURVE", TargetKind: "SKETCH_ENTITY", SketchID: sketch, TargetID: entity.ID})
	}
	target := create("Projected curves")
	apply(&target, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	local := target.Part.Features[0].ID
	root, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Name: "Tilted exact references", Type: "PRODUCT"})
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{source.Document.ID, target.Document.ID} {
		apply(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: id})
	}
	apply(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[0].ID, Translation: [3]float64{12, 7, 3}, Rotation: [4]float64{math.Sin(.2), 0, 0, math.Cos(.2)}})
	paths := func() (workspace.InstancePath, workspace.InstancePath) {
		t.Helper()
		a, e := s.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[0].ID)
		if e != nil {
			t.Fatal(e)
		}
		b, e := s.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[1].ID)
		if e != nil {
			t.Fatal(e)
		}
		return *a.ActiveInstancePath, *b.ActiveInstancePath
	}
	for i, p := range source.Part.Publications {
		a, b := paths()
		root, e = s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{RequestID: fmt.Sprintf("%s-curve-%d", root.Document.ID, i), ActorID: p6Actor, OwningInstancePath: b, SourceInstancePath: a, PublicationID: p.ID, PublicationType: "CURVE", TargetKind: "SKETCH_EXTERNAL_GEOMETRY", TargetID: local})
		if e != nil {
			t.Fatal(e)
		}
	}
	variant := root.ContextVariants[0]
	external := variant.Part.Features[0].Sketch.ExternalGeometry
	if len(external) != 3 {
		t.Fatal("missing curve inputs")
	}
	for i, want := range []string{"LINE", "ELLIPSE", "ELLIPTICAL_ARC"} {
		if external[i].Snapshot == nil || external[i].Snapshot.Kind != want || external[i].ID == root.Product.ContextBindings[i].ID {
			t.Fatal("projection did not retain exact kind/stable slot", external[i])
		}
	}
	if math.Abs(external[0].Snapshot.Start.X-12) > 1e-8 || math.Abs(external[0].Snapshot.End.X-32) > 1e-8 || math.Abs(external[1].Snapshot.MinorRadius-5*math.Cos(.4)) > 1e-8 {
		t.Fatal("exact transformed curve projection is incorrect", external)
	}
	oldSlot := external[0].ID
	binding := root.Product.ContextBindings[0]
	// Redirecting a compatible publication changes geometry, never the local slot.
	apply(&source, workspace.CommandRequest{Type: "REDIRECT_PUBLICATION", PublicationID: source.Part.Publications[0].ID, PublicationType: "CURVE", TargetKind: "SKETCH_ENTITY", SketchID: sketch, TargetID: "arc"})
	plan, e := s.GetProductUpdatePlan(t.Context(), root.Document.ID)
	if e != nil || !plan.CanAccept {
		t.Fatal("redirect plan", e)
	}
	apply(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	if root.ContextVariants[0].Part.Features[0].Sketch.ExternalGeometry[0].ID != oldSlot || root.Product.ContextBindings[0].ID != binding.ID {
		t.Fatal("redirect changed input identity")
	}
	// A missing source cannot replace the accepted geometry or fall back to defaults.
	apply(&source, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"arc"}}}})
	blocked, e := s.GetProductUpdatePlan(t.Context(), root.Document.ID)
	if e != nil || blocked.CanAccept {
		t.Fatal("broken source was not blocked", e)
	}
	head := root.Document.VersionID
	if _, e = s.ApplyCommand(t.Context(), root.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES", RequestID: root.Document.ID + "-blocked-update", ActorID: p6Actor, UpdatePlanDigest: blocked.Digest}); e == nil {
		t.Fatal("failed update committed")
	}
	root, e = s.GetDocument(t.Context(), root.Document.ID, p6Actor)
	if e != nil || root.Document.VersionID != head || root.ContextVariants[0].Status != "READY" {
		t.Fatal("failed update lost accepted result", e)
	}
	// Freezing accepted geometry must not require the now-broken source to resolve.
	frozen := root.Product.ContextBindings[0]
	apply(&root, workspace.CommandRequest{Type: "EDIT_CONTEXT_BINDING", VersionID: root.Document.VersionID, ContextBindingID: frozen.ID, ReferenceMode: "PINNED", Name: "Frozen accepted line"})
	if root.ContextVariants[0].Status != "READY" || root.Product.ContextBindings[0].Resolution.SourceDigest != frozen.Resolution.SourceDigest {
		t.Fatal("pin queried or replaced broken source")
	}
	for _, index := range []int{0, 2} {
		a, _ := paths()
		existing := root.Product.ContextBindings[index]
		apply(&root, workspace.CommandRequest{Type: "EDIT_CONTEXT_BINDING", VersionID: root.Document.VersionID, ContextBindingID: existing.ID, SourceInstancePath: &a, PublicationID: source.Part.Publications[1].ID})
		if root.Product.ContextBindings[index].ID != existing.ID || root.Product.ContextBindings[index].ContextInputID != existing.ContextInputID {
			t.Fatal("reconnect changed stable identities")
		}
	}
	healthy, e := s.GetProductUpdatePlan(t.Context(), root.Document.ID)
	if e != nil || !healthy.CanAccept {
		t.Fatal("reconnect did not restore update", e)
	}
	apply(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: healthy.Digest})
	a, b := paths()
	bad := workspace.ProductContextBindingRequest{RequestID: root.Document.ID + "-unsupported", ActorID: p6Actor, OwningInstancePath: b, SourceInstancePath: a, PublicationID: source.Part.Publications[0].ID, PublicationType: "CURVE", TargetKind: "FEATURE_INPUT", TargetID: local}
	if _, e = s.CreateProductContextBinding(t.Context(), root.Document.ID, bad); e == nil {
		t.Fatal("unimplemented Feature consumer was accepted")
	}
}

func TestPublishedSolidLineAndContextManagedWritesThroughRouter(t *testing.T) {
	s, _, db, _, _ := featureAssociationRuntimeService(t)
	seq := 0
	create := func(name, kind string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Name: name, Type: kind})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-finite-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = n
	}
	source := create("Published solid edge", "PART")
	target := create("Finite edge receiver", "PART")
	for _, v := range []*workspace.DocumentView{&source, &target} {
		apply(v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := v.Part.Features[0].ID
		apply(v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -20, Y: -10}, Second: &workspace.SketchPoint2{X: 20, Y: 10}}}})
		apply(v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: sketch, Length: 8})
	}
	// Different defaults expose accidental projection echo during a permitted edit.
	for _, p := range source.Part.Parameters {
		if p.PropertySlot == "length" {
			apply(&source, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: p.ParameterID, Value: 12, Unit: "mm"})
			break
		}
	}
	artifact := activeBodyArtifact(t, source)
	var edge uint64
	for i := uint64(1); i <= p6TopologyCount(t, artifact.Topology, "edges"); i++ {
		p, e := s.GetTopologyElementPropertiesAtVersion(t.Context(), source.Document.ID, source.Document.VersionID, artifact.GeometryKey, "EDGE", i)
		if e != nil {
			t.Fatal(e)
		}
		if p.GeometryType == "LINE" && p.Properties["direction"].([3]float64)[2] == 0 {
			edge = i
			break
		}
	}
	if edge == 0 {
		t.Fatal("missing finite horizontal edge")
	}
	// Same standard datum ID in another Part is not an authorization token.
	bad := workspace.CommandRequest{Type: "CREATE_PUBLICATION", RequestID: source.Document.ID + "-foreign", ActorID: p6Actor, PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy", SourceDocumentID: target.Document.ID, VersionID: source.Document.VersionID}
	if _, e := s.PreviewCommand(t.Context(), source.Document.ID, bad); e == nil {
		t.Fatal("foreign datum document accepted")
	}
	apply(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "CURVE", TargetKind: "EDGE", GeometryKey: artifact.GeometryKey, TopologyID: edge, VersionID: source.Document.VersionID, SourceDocumentID: source.Document.ID, Name: "Finite edge"})
	pub := source.Part.Publications[0]
	if pub.Resolution.ParameterStart == nil || pub.Resolution.ParameterEnd == nil {
		t.Fatal("published line lost finite BREP domain")
	}
	var sourceParameter, targetParameter string
	for _, p := range source.Part.Parameters {
		if p.PropertySlot == "length" {
			sourceParameter = p.ParameterID
		}
	}
	for _, p := range target.Part.Parameters {
		if p.PropertySlot == "length" {
			targetParameter = p.ParameterID
		}
	}
	apply(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: sourceParameter, Name: "Linked depth"})
	apply(&target, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	receiver := target.Part.Features[len(target.Part.Features)-1].ID
	root := create("Finite edge context", "PRODUCT")
	apply(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: source.Document.ID})
	apply(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: target.Document.ID})
	for i, kind := range []string{"SKETCH_EXTERNAL_GEOMETRY", "PARAMETER"} {
		a, e := s.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[0].ID)
		if e != nil {
			t.Fatal(e)
		}
		b, e := s.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[1].ID)
		if e != nil {
			t.Fatal(e)
		}
		targetID := receiver
		if i == 1 {
			targetID = targetParameter
		}
		n, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-edge-bind-%d", root.Document.ID, i), OwningInstancePath: *b.ActiveInstancePath, SourceInstancePath: *a.ActiveInstancePath, PublicationID: source.Part.Publications[i].ID, PublicationType: source.Part.Publications[i].Type, TargetKind: kind, TargetID: targetID})
		if e != nil {
			t.Fatal(e)
		}
		root = n
	}
	variant := root.ContextVariants[0]
	var projected *workspace.SketchExternalGeometrySnapshot
	for _, f := range variant.Part.Features {
		if f.ID == receiver {
			projected = f.Sketch.ExternalGeometry[0].Snapshot
		}
	}
	if projected == nil || projected.Kind != "LINE" || projected.Start == projected.End {
		t.Fatal("finite edge failed exact projection", projected)
	}
	apply(&root, workspace.CommandRequest{Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: root.Product.ContextBindings[0].ID})
	localLine := root.Product.ContextBindings[0].Local
	if localLine == nil || localLine.GeometryKind != "LINE" || localLine.ParameterStart == nil || localLine.ParameterEnd == nil {
		t.Fatal("finite BREP line did not become self-contained")
	}
	checkLine := func(v workspace.DocumentView) {
		t.Helper()
		for _, f := range v.ContextVariants[0].Part.Features {
			if f.ID == receiver {
				p := f.Sketch.ExternalGeometry[0].Snapshot
				if p == nil || p.Kind != "LINE" || math.Hypot(p.Start.X-projected.Start.X, p.Start.Y-projected.Start.Y) > 1e-7 || math.Hypot(p.End.X-projected.End.X, p.End.Y-projected.End.Y) > 1e-7 {
					t.Fatal("localized BREP line changed finite geometry", p)
				}
			}
		}
	}
	checkLine(root)
	variant = root.ContextVariants[0]
	var readErr error
	target, readErr = s.GetDocument(t.Context(), target.Document.ID, p6Actor)
	if readErr != nil {
		t.Fatal(readErr)
	}
	extrude := variant.Part.Features[1]
	extrude.Length = 17
	r := workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: target.Document.ID + "-protected-feature", TargetID: extrude.ID, Feature: &extrude, ExpectedFeatureDigest: findP6FeatureDigest(t, target.StructureTree, extrude.ID), RootProductDocumentID: root.Document.ID, RootProductRevisionID: root.Document.VersionID, ContextVariantID: variant.VariantKey, InstancePath: &variant.OwningInstancePath}
	for _, preview := range []bool{true, false} {
		var e error
		if preview {
			_, e = s.PreviewCommand(t.Context(), target.Document.ID, r)
		} else {
			_, e = s.ApplyCommand(t.Context(), target.Document.ID, r)
		}
		if e == nil || !strings.Contains(e.Error(), "CONTEXT_INPUT_VALUE_READ_ONLY") {
			t.Fatal("Feature edit bypassed read protection", e)
		}
	}
	r.RequestID += "-expression"
	extrude.Length = 12
	r.ParameterExpressions = map[string]string{"length": "12 mm"}
	if _, e := s.PreviewCommand(t.Context(), target.Document.ID, r); e == nil || !strings.Contains(e.Error(), "CONTEXT_INPUT_VALUE_READ_ONLY") {
		t.Fatal("expression bypassed read protection", e)
	}
	now, e := s.GetDocument(t.Context(), target.Document.ID, p6Actor)
	if e != nil || now.Document.VersionID != target.Document.VersionID {
		t.Fatal("rejected write advanced Head", e)
	}
	r.RequestID += "-permitted"
	r.ParameterExpressions = nil
	extrude.Reversed = true // unrelated field remains editable
	next, editErr := s.ApplyCommand(t.Context(), target.Document.ID, r)
	if editErr != nil {
		t.Fatal("unbound field rejected", editErr)
	}
	if next.Part.Features[1].Length != 8 || !next.Part.Features[1].Reversed {
		t.Fatal("projected managed value leaked into default definition", next.Part.Features[1])
	}
	root, editErr = s.GetDocument(t.Context(), root.Document.ID, p6Actor)
	if editErr != nil {
		t.Fatal(editErr)
	}
	apply(&root, workspace.CommandRequest{Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: root.Product.ContextBindings[1].ID})
	apply(&root, workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "INSTANCE", TargetID: root.Product.Instances[0].ID})
	if err := s.DeleteDocument(t.Context(), source.Document.ID, root.Document.ID+"-line-source-unavailable"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `DELETE FROM occccad.product_context_variants WHERE base_document_id=$1`, target.Document.ID); err != nil {
		t.Fatal(err)
	}
	root, editErr = s.GetDocument(workspace.WithColdEvaluation(t.Context()), root.Document.ID, p6Actor)
	if editErr != nil {
		t.Fatal("finite local line still needs source BREP", editErr)
	}
	checkLine(root)
	for len(root.Product.ContextBindings) > 0 {
		apply(&root, workspace.CommandRequest{Type: "RESTORE_CONTEXT_INPUT_DEFAULT", VersionID: root.Document.VersionID, ContextBindingID: root.Product.ContextBindings[0].ID})
	}
	if len(root.ContextVariants) != 1 || root.ContextVariants[0].Part == nil || root.ContextVariants[0].Part.Features[1].Length != 8 {
		t.Fatal("last restoration lost the accepted default view")
	}

}

func TestContextDrivenSketchDimensionWritesThroughRouter(t *testing.T) {
	s, _, _, _, _ := featureAssociationRuntimeService(t)
	seq := 0
	create := func(name, kind string) workspace.DocumentView {
		t.Helper()
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Name: name, Type: kind})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-linked-dim-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = n
	}
	source, target := create("Linked dimension source", "PART"), create("Linked dimension receiver", "PART")
	apply(&source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "pitch", Value: 12, Unit: "mm"})
	parameterID := ""
	for _, p := range source.Part.Parameters {
		if p.DisplayAlias == "pitch" {
			parameterID = p.ParameterID
		}
	}
	apply(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: parameterID})
	apply(&target, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := target.Part.Features[0].ID
	value := 8.
	apply(&target, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 8, Y: 3}}}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "dimension", Kind: "HORIZONTAL_DISTANCE", Value: &value, Unit: "mm", References: []workspace.SketchGeometryRef{{Target: "SKETCH_ORIGIN", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "point", SubElement: "POINT"}}}}}})
	dimension := target.Part.Features[0].Sketch.Constraints[0]
	root := create("Linked dimension product", "PRODUCT")
	apply(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: source.Document.ID})
	apply(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: target.Document.ID})
	a, e := s.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.GetProductDesignSession(t.Context(), root.Document.ID, root.Product.Instances[1].ID)
	if e != nil {
		t.Fatal(e)
	}
	root, e = s.CreateProductContextBinding(t.Context(), root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-bind-dimension", OwningInstancePath: *b.ActiveInstancePath, SourceInstancePath: *a.ActiveInstancePath, PublicationID: source.Part.Publications[0].ID, PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: dimension.ParameterID})
	if e != nil {
		t.Fatal(e)
	}
	variant := root.ContextVariants[0]
	shown := variant.Part.Features[0].Sketch.Constraints[0]
	if shown.Value == nil || *shown.Value != 12 {
		t.Fatal("accepted dimension did not drive local point")
	}
	target, e = s.GetDocument(t.Context(), target.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	change := 17.
	text := "18 mm"
	for i, op := range []workspace.SketchOperation{{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: dimension.ID, Value: &change}, {Type: "UPDATE_CONSTRAINT", ConstraintID: dimension.ID, Constraint: &shown, ParameterSource: &text}} {
		r := workspace.CommandRequest{Type: "EDIT_SKETCH", ActorID: p6Actor, RequestID: fmt.Sprintf("%s-reject-%d", target.Document.ID, i), SketchID: sketch, Operations: []workspace.SketchOperation{op}, RootProductDocumentID: root.Document.ID, RootProductRevisionID: root.Document.VersionID, ContextVariantID: variant.VariantKey, InstancePath: &variant.OwningInstancePath}
		if _, e := s.PreviewCommand(t.Context(), target.Document.ID, r); e == nil || !strings.Contains(e.Error(), "CONTEXT_INPUT_VALUE_READ_ONLY") {
			t.Fatal("dimension preview bypassed protection", e)
		}
		if _, e := s.ApplyCommand(t.Context(), target.Document.ID, r); e == nil || !strings.Contains(e.Error(), "CONTEXT_INPUT_VALUE_READ_ONLY") {
			t.Fatal("dimension commit bypassed protection", e)
		}
	}
	now, e := s.GetDocument(t.Context(), target.Document.ID, p6Actor)
	if e != nil || now.Document.VersionID != target.Document.VersionID {
		t.Fatal("rejected dimension advanced Head", e)
	}
	r := workspace.CommandRequest{Type: "EDIT_SKETCH", ActorID: p6Actor, RequestID: target.Document.ID + "-rename-dimension", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "UPDATE_CONSTRAINT", ConstraintID: shown.ID, Constraint: &shown, ParameterKey: "linked_position"}}, RootProductDocumentID: root.Document.ID, RootProductRevisionID: root.Document.VersionID, ContextVariantID: variant.VariantKey, InstancePath: &variant.OwningInstancePath}
	preview, e := s.PreviewCommand(t.Context(), target.Document.ID, r)
	if e != nil {
		t.Fatal("rename echoed projected dimension", e)
	}
	r.PreviewID = preview.PreviewID
	renamed, e := s.ApplyCommand(t.Context(), target.Document.ID, r)
	if e != nil {
		t.Fatal(e)
	}
	if *renamed.Part.Features[0].Sketch.Constraints[0].Value != 8 {
		t.Fatal("rename overwrote independent default")
	}
	currentRoot, e := s.GetDocument(t.Context(), root.Document.ID, p6Actor)
	if e != nil || *currentRoot.ContextVariants[0].Part.Features[0].Sketch.Constraints[0].Value != 12 {
		t.Fatal("rename lost accepted dimension", e)
	}
}

// Local values are exact receiver-space inputs, not pinned source references.
func TestContextInputLocalizeDefaultsReconnectAndColdRebuild(t *testing.T) {
	s, store, db, client, _ := featureAssociationRuntimeService(t)
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
		r.RequestID = fmt.Sprintf("%s-local-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
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
		p, e := s.GetProductDesignSession(t.Context(), v.Document.ID, v.Product.Instances[i].ID)
		if e != nil {
			t.Fatal(e)
		}
		return *p.ActiveInstancePath
	}
	source := create("PART", "Exact local source")
	run(&source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "depth", Value: 3, Unit: "mm"})
	depthSource := source.Part.Parameters[0].ParameterID
	run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Depth", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: depthSource})
	run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Mount", PublicationType: "PLANE", TargetKind: "PLANE", TargetID: "datum-xy"})
	run(&source, workspace.CommandRequest{Type: "CREATE_DATUM_AXIS", Direction: [3]float64{0, 0, 1}})
	sourceAxis := source.Part.DatumAxes[len(source.Part.DatumAxes)-1].ID
	run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Axis", PublicationType: "AXIS", TargetKind: "DATUM_AXIS", TargetID: sourceAxis})
	run(&source, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sourceSketch := source.Part.Features[0].ID
	entities := []workspace.SketchEntity{{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 20, Y: 10}}, {ID: "line", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{X: -20, Y: 20}, End: &workspace.SketchPoint2{X: 20, Y: 20}}, {ID: "circle", Kind: "CIRCLE", Role: "CONSTRUCTION", Center: &workspace.SketchPoint2{X: -20, Y: 0}, Radius: 8}, {ID: "arc", Kind: "ARC", Role: "CONSTRUCTION", Center: &workspace.SketchPoint2{X: 0, Y: -20}, Radius: 5, StartAngle: .2, EndAngle: 2.4}}
	for _, e := range entities {
		entity := e
		run(&source, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sourceSketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &entity}}})
		typ := "CURVE"
		if e.Kind == "POINT" {
			typ = "POINT"
		}
		run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: e.ID, PublicationType: typ, TargetKind: "SKETCH_ENTITY", SketchID: sourceSketch, TargetID: e.ID})
	}
	plate := create("PART", "Repeated local receiver")
	run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := plate.Part.Features[0].ID
	run(&plate, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -60, Y: -40}, Second: &workspace.SketchPoint2{X: 60, Y: 40}}}})
	run(&plate, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", Operation: "ADD", SketchID: profile, Length: 8})
	run(&plate, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "localDepth", Value: 2, Unit: "mm"})
	depth := ""
	for _, p := range plate.Part.Parameters {
		if p.DisplayAlias == "localDepth" {
			depth = p.ParameterID
		}
	}
	run(&plate, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: "Depth", TargetKind: "PARAMETER", TargetID: depth})
	run(&plate, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	mount := plate.Part.DatumPlanes[len(plate.Part.DatumPlanes)-1].ID
	run(&plate, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: "Mount", TargetKind: "DATUM", PublicationType: "PLANE", TargetID: mount})
	run(&plate, workspace.CommandRequest{Type: "CREATE_DATUM_AXIS", Direction: [3]float64{1, 0, 0}})
	axis := plate.Part.DatumAxes[len(plate.Part.DatumAxes)-1].ID
	run(&plate, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: "Axis", TargetKind: "DATUM", PublicationType: "AXIS", TargetID: axis})
	run(&plate, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: mount})
	sketch := plate.Part.Features[len(plate.Part.Features)-1].ID
	for _, e := range entities {
		entity := e
		entity.ID = "local-" + e.ID
		if entity.Kind == "POINT" {
			entity.Point = &workspace.SketchPoint2{X: 10, Y: 10}
		}
		run(&plate, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &entity}}})
		typ := "CURVE"
		if entity.Kind == "POINT" {
			typ = "POINT"
		}
		run(&plate, workspace.CommandRequest{Type: "CREATE_CONTEXT_INPUT", Name: e.ID, TargetKind: "SKETCH_EXTERNAL_GEOMETRY", TargetID: sketch, SketchID: sketch, OwnerEntityID: entity.ID, PublicationType: typ, ConnectInputPoint: typ == "POINT"})
	}
	run(&plate, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: plate.Part.ActiveBodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 4, Depth: 2, HolePoint: &workspace.PatternPointReference{SketchID: sketch, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "local-point", SubElement: "POINT"}}}, ParameterExpressions: map[string]string{"depth": "localDepth"}})
	hole := plate.Part.Features[len(plate.Part.Features)-1].ID
	mouth := findP7TopologyPick(t, s, plate, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.Anchor.FeatureID == hole && strings.HasPrefix(p.Anchor.OutputSlot, "START_BOUNDARY_FROM_PROFILE_EDGE/")
	})
	run(&plate, workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: plate.Part.ActiveBodyID, Length: .15, Selections: []workspace.FeatureSelection{{Selection: mouth.Selection, SourceVersionID: mouth.SourceVersionID}}}})
	chamfer := plate.Part.Features[len(plate.Part.Features)-1].ID
	run(&plate, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Hole depth", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: depth})
	depthPub := plate.Part.Publications[0].ID
	independentRevision := plate.Document.VersionID
	root := create("PRODUCT", "Local values isolate occurrences")
	for _, v := range []workspace.DocumentView{source, plate, plate} {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: v.Document.ID})
	}
	for i := 0; i < 3; i++ {
		run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: root.Product.Instances[i].ID, Translation: [3]float64{float64(i) * 5, 4, 0}, Rotation: [4]float64{0, 0, math.Sin(.2), math.Cos(.2)}})
	}
	run(&root, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: root.Product.Instances[0].ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	bindingIDs := []map[string]string{{}, {}}
	for i := 1; i <= 2; i++ {
		for _, p := range source.Part.Publications {
			input := ""
			for _, v := range plate.Part.ContextInputs {
				if v.Name == p.Name {
					input = v.ID
				}
			}
			seq++
			r := workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-local-bind-%d", root.Document.ID, seq), ExpectedRootRevisionID: root.Document.VersionID, OwningInstancePath: path(root, i), SourceInstancePath: path(root, 0), PublicationID: p.ID, PublicationType: p.Type, ContextInputID: input, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"}
			n, e := s.CreateProductContextBinding(t.Context(), root.Document.ID, r)
			if e != nil {
				t.Fatal(p.Name, e)
			}
			root = n
			bindingIDs[i-1][p.Name] = root.Product.ContextBindings[len(root.Product.ContextBindings)-1].ID
		}
	}
	variant := func(v workspace.DocumentView, instanceID string) workspace.ContextVariantSnapshot {
		t.Helper()
		for _, c := range v.ContextVariants {
			if c.OwningInstancePath.Segments[0].InstanceID == instanceID {
				return c
			}
		}
		t.Fatal("missing receiver variant")
		return workspace.ContextVariantSnapshot{}
	}
	receiverPath := path(root, 1)
	run(&root, workspace.CommandRequest{Type: "CREATE_PRODUCT_PUBLICATION", Name: "Accepted hole depth", InstancePath: &receiverPath, PublicationID: depthPub})
	first, second := root.Product.Instances[1].ID, root.Product.Instances[2].ID
	wantedPoint := map[string]float64{first: 20, second: 20}
	baseline := root
	firstPart := variant(root, first)
	firstArt := root.Artifacts[firstPart.Bodies[0].GeometryKey]
	originalBindings, _ := json.Marshal(root.Product.ContextBindings[7:])
	check := func(v workspace.DocumentView, instance string, wantVolume float64, wantDepth float64) {
		t.Helper()
		cv := variant(v, instance)
		if cv.Status != "READY" || cv.Part == nil {
			t.Fatal("invalid local candidate")
		}
		art := v.Artifacts[cv.Bodies[0].GeometryKey]
		if math.Abs(art.Volume-wantVolume) > 1e-5 {
			t.Fatal("wrong exact local volume", art.Volume, wantVolume)
		}
		for _, f := range cv.Part.Features {
			if (f.ID == hole || f.ID == chamfer) && f.EvaluationStatus != "" {
				t.Fatal("downstream identity failed", f.ID, f.EvaluationStatus)
			}
			if f.ID == hole && math.Abs(f.Depth-wantDepth) > 1e-7 {
				t.Fatal("wrong depth", f.Depth)
			}
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
		top, _, e := client.GetTopology(t.Context(), art.GeometryID, brep, "FACE", 0)
		if e != nil {
			t.Fatal(e)
		}
		walls := 0
		for _, f := range top.Faces {
			if f.SurfaceType == 1 {
				walls++
				owner := v.Product.Instances[slices.IndexFunc(v.Product.Instances, func(i workspace.ProductInstance) bool { return i.ID == instance })]
				delta := float64(0) - owner.Translation[0]
				wantX, wantY := wantedPoint[instance]+math.Cos(.4)*delta, 10-math.Sin(.4)*delta
				if math.Abs((f.Bbox.MinX+f.Bbox.MaxX)/2-wantX) > 1e-6 || math.Abs((f.Bbox.MinY+f.Bbox.MaxY)/2-wantY) > 1e-6 {
					t.Fatal("exact receiver-space hole position changed", f.Bbox, wantX, wantY)
				}
				if math.Abs(f.Bbox.MaxZ-wantDepth) > 1e-5 {
					t.Fatal("exact hole depth changed", f.Bbox)
				}
			}
		}
		if walls != 1 {
			t.Fatal("missing exact wall", walls)
		}
	}
	run(&source, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: depthSource, Value: 4, Unit: "mm"})
	run(&source, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sourceSketch, Operations: []workspace.SketchOperation{{Type: "UPDATE_ENTITY_POINT", EntityID: "point", SubElement: "POINT", Point: &workspace.SketchPoint2{X: 100, Y: 10}}}})
	read(&root)
	if root.Document.VersionID != baseline.Document.VersionID {
		t.Fatal("pending input silently accepted")
	}
	for j, name := range []string{"point", "Depth", "Mount", "Axis", "line", "circle", "arc"} {
		req := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-local-preview-%d", root.Document.ID, j), Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: bindingIDs[0][name]}
		preview, e := s.PreviewCommand(t.Context(), root.Document.ID, req)
		if e != nil {
			t.Fatal(name, e)
		}
		if preview.ReplacementView == nil {
			t.Fatal("missing full input preview")
		}
		head := root.Document.VersionID
		read(&root)
		if root.Document.VersionID != head {
			t.Fatal("preview advanced Head")
		}
		if j == 0 {
			s.DiscardPreview(root.Document.ID, p6Actor, preview.PreviewID)
			cancelled := root
			read(&root)
			if cancelled.Document.VersionID != root.Document.VersionID {
				t.Fatal("cancel changed model")
			}
			preview, e = s.PreviewCommand(t.Context(), root.Document.ID, req)
			if e != nil {
				t.Fatal(e)
			}
		}
		req.PreviewID = preview.PreviewID
		run(&root, req)
		check(root, first, firstArt.Volume, 3)
		b := root.Product.ContextBindings[slices.IndexFunc(root.Product.ContextBindings, func(b workspace.ContextBinding) bool { return b.ID == req.ContextBindingID })]
		raw, _ := json.Marshal(b)
		if b.Local == nil || strings.Contains(string(raw), source.Document.ID) || b.Publication.PublicationID != "" || len(b.SourceInstancePath.Segments) != 0 {
			t.Fatal("source reference survived localization")
		}
	}
	other, _ := json.Marshal(root.Product.ContextBindings[7:])
	check(root, second, root.Artifacts[variant(baseline, second).Bodies[0].GeometryKey].Volume, 3)
	if string(other) != string(originalBindings) {
		t.Fatal("other occurrence inputs changed")
	}
	read(&plate)
	if plate.Document.VersionID != independentRevision {
		t.Fatal("shared defaults mutated")
	}
	stale := workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-stale-local", Type: "RESTORE_CONTEXT_INPUT_DEFAULT", VersionID: baseline.Document.VersionID, ContextBindingID: bindingIDs[0]["Depth"]}
	if _, e := s.PreviewCommand(t.Context(), root.Document.ID, stale); e == nil || !strings.Contains(e.Error(), "CONTEXT_BINDING_REVISION_CHANGED") {
		t.Fatal("stale default restoration accepted", e)
	}
	run(&plate, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: depth, Value: 5, Unit: "mm"})
	beforeDefault := root.Document.VersionID
	defaultPreview, e := s.PreviewCommand(t.Context(), root.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-accepted-default", Type: "RESTORE_CONTEXT_INPUT_DEFAULT", VersionID: root.Document.VersionID, ContextBindingID: bindingIDs[0]["Depth"]})
	if e != nil || defaultPreview.ReplacementView == nil {
		t.Fatal("accepted default preview", e)
	}
	check(*defaultPreview.ReplacementView, first, firstArt.Volume+4*math.Pi, 2)
	if defaultPreview.ReplacementView.Product.Instances[1].ReferencedVersionID != independentRevision {
		t.Fatal("restoration advanced receiver to new Part Head")
	}
	s.DiscardPreview(root.Document.ID, p6Actor, defaultPreview.PreviewID)
	read(&root)
	if root.Document.VersionID != beforeDefault {
		t.Fatal("cancelled defaults wrote history")
	}
	run(&plate, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: depth, Value: 2, Unit: "mm"})
	// Undo/Redo restores the union and every local snapshot in one action.
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	last := root.Product.ContextBindings[slices.IndexFunc(root.Product.ContextBindings, func(b workspace.ContextBinding) bool { return b.ID == bindingIDs[0]["arc"] })]
	if last.Local != nil {
		t.Fatal("undo did not restore source")
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	check(root, first, firstArt.Volume, 3)
	// The local snapshot is explicitly protected from receiver replacement.
	badReplace := workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-local-replace", Type: "REPLACE_INSTANCE", VersionID: root.Document.VersionID, InstanceID: first, ReferencedDocumentID: plate.Document.ID, ReplacementVersionID: plate.Document.VersionID}
	if _, e := s.PreviewCommand(t.Context(), root.Document.ID, badReplace); e == nil || !strings.Contains(e.Error(), "REPLACEMENT_LOCAL_INPUT_UNSUPPORTED") {
		t.Fatal("local assignment silently dropped by replacement", e)
	}
	for _, id := range bindingIDs[1] {
		run(&root, workspace.CommandRequest{Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: id})
	}
	plan, e := s.GetProductUpdatePlan(t.Context(), root.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	hotPlan, e := s.GetProductUpdatePlan(t.Context(), root.Document.ID)
	if e != nil || hotPlan.Digest != plan.Digest {
		t.Fatal("cold/warm update plan differ", e, plan.Digest, hotPlan.Digest)
	}
	for _, entry := range plan.Entries {
		if entry.Kind == "CONTEXT_BINDING" && (entry.ReferenceMode != "LOCAL" || entry.Currency != "CURRENT" || entry.SourceDisplayPath != "本地输入") {
			t.Fatal("local input still queries upstream", entry)
		}
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	check(root, first, firstArt.Volume, 3)
	// The source occurrence can now advance without changing local input values.
	pointPub := source.Part.Publications[slices.IndexFunc(source.Part.Publications, func(p workspace.Publication) bool { return p.Name == "point" })]
	reconnect := workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-bad-reconnect", Type: "EDIT_CONTEXT_BINDING", VersionID: root.Document.VersionID, ContextBindingID: bindingIDs[0]["point"], SourceInstancePath: func() *workspace.InstancePath { p := path(root, 0); return &p }(), PublicationID: pointPub.ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"}
	head := root.Document.VersionID
	if _, e := s.PreviewCommand(t.Context(), root.Document.ID, reconnect); e == nil {
		t.Fatal("invalid reconnect geometry accepted")
	}
	read(&root)
	if root.Document.VersionID != head {
		t.Fatal("failed reconnect changed Head")
	}
	run(&source, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sourceSketch, Operations: []workspace.SketchOperation{{Type: "UPDATE_ENTITY_POINT", EntityID: "point", SubElement: "POINT", Point: &workspace.SketchPoint2{X: 25, Y: 10}}}})
	plan, e = s.GetProductUpdatePlan(t.Context(), root.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	reconnect.VersionID = root.Document.VersionID
	reconnect.SourceInstancePath = func() *workspace.InstancePath { p := path(root, 0); return &p }()
	run(&root, reconnect)
	wantedPoint[first] = 25
	check(root, first, firstArt.Volume, 3)
	b := root.Product.ContextBindings[slices.IndexFunc(root.Product.ContextBindings, func(b workspace.ContextBinding) bool { return b.ID == bindingIDs[0]["point"] })]
	if b.Local != nil {
		t.Fatal("reconnect retained local state")
	}
	run(&root, workspace.CommandRequest{Type: "RESTORE_CONTEXT_INPUT_DEFAULT", VersionID: root.Document.VersionID, ContextBindingID: bindingIDs[0]["Depth"]})
	check(root, first, firstArt.Volume+4*math.Pi, 2)
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check(root, first, firstArt.Volume, 3)
	run(&root, workspace.CommandRequest{Type: "REDO"})
	check(root, first, firstArt.Volume+4*math.Pi, 2)
	// The old tree/API deletion is precisely the same restoration operation,
	// including forwarded values and candidate preview/cancel semantics.
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check(root, first, firstArt.Volume, 3)
	deleteReq := workspace.CommandRequest{ActorID: p6Actor, RequestID: root.Document.ID + "-delete-default-preview", Type: "DELETE_CONTEXT_BINDING", VersionID: root.Document.VersionID, ContextBindingID: bindingIDs[0]["Depth"]}
	beforeDelete := root.Document.VersionID
	p, e := s.PreviewCommand(t.Context(), root.Document.ID, deleteReq)
	if e != nil || p.ReplacementView == nil {
		t.Fatal("delete default preview", e)
	}
	if p.ReplacementView.Product.Publications[0].Resolution.Value == nil || math.Abs(p.ReplacementView.Product.Publications[0].Resolution.Value.SIValue-.002) > 1e-9 {
		t.Fatal("forwarded default not refreshed", p.ReplacementView.Product.Publications)
	}
	s.DiscardPreview(root.Document.ID, p6Actor, p.PreviewID)
	read(&root)
	if root.Document.VersionID != beforeDelete {
		t.Fatal("delete cancel wrote Head")
	}
	run(&root, deleteReq)
	check(root, first, firstArt.Volume+4*math.Pi, 2)
	if math.Abs(root.Product.Publications[0].Resolution.Value.SIValue-.002) > 1e-9 {
		t.Fatal("delete did not commit publication refresh")
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	check(root, first, firstArt.Volume, 3)
	if math.Abs(root.Product.Publications[0].Resolution.Value.SIValue-.003) > 1e-9 {
		t.Fatal("undo did not restore forwarded snapshot")
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	check(root, first, firstArt.Volume+4*math.Pi, 2)
	// Every source is removed from the current graph before making it unavailable.
	run(&root, workspace.CommandRequest{Type: "LOCALIZE_CONTEXT_INPUT", VersionID: root.Document.VersionID, ContextBindingID: bindingIDs[0]["point"]})
	run(&root, workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "INSTANCE", TargetID: root.Product.Instances[0].ID})
	if e := s.DeleteDocument(t.Context(), source.Document.ID, root.Document.ID+"-remove-source"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(t.Context(), `DELETE FROM occccad.product_context_variants WHERE base_document_id=$1`, plate.Document.ID); e != nil {
		t.Fatal(e)
	}
	cold, e := s.GetDocument(workspace.WithColdEvaluation(t.Context()), root.Document.ID, p6Actor)
	if e != nil {
		t.Fatal("local rebuild read unavailable source", e)
	}
	root = cold
	check(root, first, firstArt.Volume+4*math.Pi, 2)
	read(&root)
	check(root, first, firstArt.Volume+4*math.Pi, 2)
	// Saved modules remain readable when rebased through multiple owning Products.
	module := create("PRODUCT", "Saved local module")
	run(&module, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: root.Document.ID})
	top := create("PRODUCT", "Nested local assembly")
	run(&top, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: module.Document.ID})
	want := variant(root, first)
	verifyNested := func(v workspace.DocumentView) {
		t.Helper()
		found := false
		for _, cv := range v.ContextVariants {
			segments := cv.OwningInstancePath.Segments
			if len(segments) != 3 || segments[2].InstanceID != first {
				continue
			}
			found = true
			if cv.Status != "READY" || cv.Part == nil || cv.Bodies[0].GeometryKey != want.Bodies[0].GeometryKey || !reflect.DeepEqual(cv.Part, want.Part) {
				t.Fatal("nested LOCAL lost precise accepted geometry", cv.Status, cv.Diagnostic)
			}
			for _, r := range v.ResolvedInstances {
				if r.OccurrencePath == cv.OwningInstancePath.Canonical && r.BodyID == cv.Bodies[0].ID && r.GeometryKey != cv.Bodies[0].GeometryKey {
					t.Fatal("nested display used shared defaults")
				}
			}
		}
		if !found {
			t.Fatal("missing nested local projection")
		}
	}
	verifyNested(top)
	read(&top)
	verifyNested(top)
	if _, e := db.Exec(t.Context(), `DELETE FROM occccad.product_context_variants WHERE base_document_id=$1`, plate.Document.ID); e != nil {
		t.Fatal(e)
	}
	nestedCold, e := s.GetDocument(workspace.WithColdEvaluation(t.Context()), top.Document.ID, p6Actor)
	if e != nil {
		t.Fatal("nested local cold rebuild", e)
	}
	verifyNested(nestedCold)

}
