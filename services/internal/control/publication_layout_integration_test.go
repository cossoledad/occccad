package control

import (
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
	"math"
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
