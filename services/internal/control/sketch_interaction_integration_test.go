package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"reflect"
	"testing"
)

func TestExactSketchIntersectionEditsThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Exact sketch crossing"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		r.ActorID = p6Actor
		v, err = service.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := v.Part.Features[0].ID
	edit := func(ops ...workspace.SketchOperation) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
	}
	entities := func() []workspace.SketchEntity { return v.Part.Features[0].Sketch.Entities }
	edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "a", Kind: "ARC", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 10, StartAngle: 0, EndAngle: math.Pi}}, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "b", Kind: "ARC", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 10}, Radius: 10, StartAngle: math.Pi / 2, EndAngle: 3 * math.Pi / 2}})
	input := workspace.SketchIntersectionsRequest{VersionID: v.Document.VersionID, SketchID: sketch, EntityIDs: []string{"a", "b"}}
	hits, e := service.SketchIntersections(t.Context(), v.Document.ID, input)
	if e != nil || len(hits.Intersections) != 1 {
		t.Fatal(e, hits)
	}
	hit := hits.Intersections[0]
	if math.Abs(hit.Point[0]-5) > 1e-8 || math.Abs(hit.Point[1]-math.Sqrt(75)) > 1e-8 || !hit.ConstraintSupported {
		t.Fatal(hit)
	}
	head := v.Document.VersionID
	op := workspace.SketchOperation{Type: "SPLIT_INTERSECTION", OperationID: "crossing", EntityIDs: input.EntityIDs, Parameters: hit.Parameters[:], Mode: "BOTH"}
	request := workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{op}}
	preview, e := service.PreviewCommand(t.Context(), v.Document.ID, request)
	if e != nil || preview.PreviewID == "" {
		t.Fatal(e)
	}
	read, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("preview/read changed Head", e)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	if len(entities()) != 4 {
		t.Fatal("two open arcs must produce four curves", entities())
	}
	split := append([]workspace.SketchEntity(nil), entities()...)
	remove := ""
	for _, e := range entities() {
		if e.SourceEntityID == "a" && e.ID != "a" {
			remove = e.ID
		}
	}
	if remove == "" {
		t.Fatal("missing stable split segment")
	}
	edit(workspace.SketchOperation{Type: "DELETE_ENTITIES", EntityIDs: []string{remove}})
	if len(entities()) != 3 {
		t.Fatal("wrong deleted interval")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if !reflect.DeepEqual(entities(), split) {
		t.Fatal("undo deletion")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(entities()) != 2 {
		t.Fatal("split was not one transaction")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	apply(workspace.CommandRequest{Type: "REDO"})
	if len(entities()) != 3 {
		t.Fatal("redo")
	}
	// Creation uses exact branch evidence, not a sampled point or result ordinal.
	apply(workspace.CommandRequest{Type: "UNDO"})
	apply(workspace.CommandRequest{Type: "UNDO"})
	op.Type = "CREATE_INTERSECTION"
	op.OperationID = "point"
	edit(op)
	if len(entities()) != 3 {
		t.Fatal("point creation")
	}
	c := v.Part.Features[0].Sketch.Constraints
	if len(c) != 2 || c[0].Kind != "POINT_ON_OBJECT" || c[1].Kind != "POINT_ON_OBJECT" {
		t.Fatal(c)
	}
	saved := v.Part.Features[0].Sketch
	opened, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || !reflect.DeepEqual(saved, opened.Part.Features[0].Sketch) {
		t.Fatal("reopen", e)
	}
	stale := input
	if _, e = service.SketchIntersections(t.Context(), v.Document.ID, stale); e == nil {
		t.Fatal("stale analysis accepted")
	}
	bad := op
	bad.Parameters = []float64{.123, .456}
	before := v.Document.VersionID
	if _, e = service.PreviewCommand(t.Context(), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{bad}}); e == nil {
		t.Fatal("invented intersection accepted")
	}
	opened, e = service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || opened.Document.VersionID != before {
		t.Fatal("invalid branch wrote Head", e)
	}
	// A parameter seam is a valid chosen position, never an extra artificial cut.
	edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 50}, Radius: 10}}, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "diameter", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{X: 40}, End: &workspace.SketchPoint2{X: 60}}})
	input.VersionID = v.Document.VersionID
	input.EntityIDs = []string{"circle", "diameter"}
	closed, e := service.SketchIntersections(t.Context(), v.Document.ID, input)
	if e != nil || len(closed.Intersections) != 2 {
		t.Fatal("closed curve analysis", e, closed)
	}
	parameters := []float64{}
	for _, h := range closed.Intersections {
		parameters = append(parameters, h.Parameters[:]...)
	}
	count := len(entities())
	edit(workspace.SketchOperation{Type: "SPLIT_INTERSECTION", OperationID: "seam", EntityIDs: input.EntityIDs, Parameters: parameters, Mode: "BOTH"})
	if len(entities()) != count+1 {
		t.Fatal("closed seam produced an extra interval", entities())
	}
	// Endpoint contact preserves the existing connection and adds no duplicate relation.
	edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "touch-a", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{X: 100, Y: 30}, End: &workspace.SketchPoint2{X: 120, Y: 30}}}, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "touch-b", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{X: 120, Y: 30}, End: &workspace.SketchPoint2{X: 140, Y: 50}}}, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "joined-touch", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "touch-a", SubElement: "END"}, {Target: "ENTITY", EntityID: "touch-b", SubElement: "START"}}}})
	input.VersionID = v.Document.VersionID
	input.EntityIDs = []string{"touch-a", "touch-b"}
	touch, e := service.SketchIntersections(t.Context(), v.Document.ID, input)
	if e != nil || len(touch.Intersections) != 1 {
		t.Fatal("endpoint contact", e, touch)
	}
	constraints := len(v.Part.Features[0].Sketch.Constraints)
	edit(workspace.SketchOperation{Type: "CREATE_INTERSECTION", OperationID: "touch-point", EntityIDs: input.EntityIDs, Parameters: touch.Intersections[0].Parameters[:]})
	if len(v.Part.Features[0].Sketch.Constraints) != constraints+1 {
		t.Fatal("endpoint contact duplicated an accepted connection")
	}

}

func TestSketchAngleSectorsAndAutomaticRelationsThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	for _, reverse := range []bool{false, true} {
		for _, clockwise := range []bool{false, true} {
			v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Angle sector"})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				r.ActorID = p6Actor
				v, err = service.ApplyCommand(t.Context(), v.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			sketch := v.Part.Features[0].ID
			edit := func(ops ...workspace.SketchOperation) {
				t.Helper()
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
			}
			a, b := workspace.SketchPoint2{X: 15, Y: 5}, workspace.SketchPoint2{X: 15 + 10*math.Cos(math.Pi/6), Y: 10}
			if reverse {
				a, b = b, a
			}
			edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &a, End: &b}})
			sector := &workspace.SketchAngleSector{SecondReversed: reverse, Clockwise: clockwise}
			angle := 30.
			if clockwise {
				angle = 330
			}
			constraint := workspace.SketchConstraint{ID: "angle", Kind: "ANGLE", References: []workspace.SketchGeometryRef{{Target: "SKETCH_X_AXIS", SubElement: "DIRECTION"}, {Target: "ENTITY", EntityID: "line", SubElement: "DIRECTION"}}, AngleSector: sector, Value: &angle, Unit: "deg", LabelPosition: &workspace.SketchPoint2{X: 20, Y: 12}}
			edit(workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &constraint, ParameterSource: sketchInteractionSource(fmt.Sprintf("%g deg * 2", angle/2))})
			saved := v.Part.Features[0].Sketch
			originalParameter := saved.Constraints[0].ParameterID
			measured := func() float64 {
				e := v.Part.Features[0].Sketch.Entities[0]
				p := math.Atan2(e.End.Y-e.Start.Y, e.End.X-e.Start.X) * 180 / math.Pi
				if reverse {
					p -= 180
				}
				p = math.Mod(p+720, 360)
				if clockwise {
					p = 360 - p
				}
				return p
			}
			if math.Abs(measured()-angle) > 1e-5 {
				t.Fatal("initial sector solve", reverse, clockwise, measured(), angle)
			}
			changed := saved.Constraints[0]
			newValue := 60.
			if clockwise {
				newValue = 300
			}
			changed.Value = &newValue
			edit(workspace.SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: "angle", Constraint: &changed, ParameterSource: sketchInteractionSource(fmt.Sprintf("%g deg", newValue))})
			if math.Abs(measured()-newValue) > 1e-5 {
				t.Fatal("sector input conversion", reverse, clockwise, measured())
			}
			before := append([]workspace.SketchEntity(nil), v.Part.Features[0].Sketch.Entities...)
			changed = v.Part.Features[0].Sketch.Constraints[0]
			changed.LabelPosition = &workspace.SketchPoint2{X: -30, Y: -40}
			edit(workspace.SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: "angle", Constraint: &changed})
			if !reflect.DeepEqual(before, v.Part.Features[0].Sketch.Entities) || v.Part.Features[0].Sketch.Constraints[0].ParameterID != originalParameter {
				t.Fatal("label placement changed engineering definition")
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			apply(workspace.CommandRequest{Type: "UNDO"})
			if math.Abs(measured()-angle) > 1e-5 {
				t.Fatal("angle undo")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			apply(workspace.CommandRequest{Type: "REDO"})
			// Ordinary point-on-line and midpoint relationships remain editable by point dimensions.
			p := workspace.SketchPoint2{X: 10, Y: 25}
			end := workspace.SketchPoint2{X: 30, Y: 25}
			edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "support", Kind: "LINE", Role: "CONSTRUCTION", Start: &p, End: &end}}, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 20, Y: 25}}}, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "on", Kind: "POINT_ON_OBJECT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "point", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "support", SubElement: "WHOLE"}}}})
			value := 10.
			edit(workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "length", Kind: "DISTANCE", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "support", SubElement: "START"}, {Target: "ENTITY", EntityID: "point", SubElement: "POINT"}}, Value: &value, Unit: "mm"}})
			if !reverse && !clockwise {
				support := workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "support", SubElement: "WHOLE"}
				edit(workspace.SketchOperation{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 20, Y: 25}, Second: &workspace.SketchPoint2{X: 26, Y: 31}, FirstCapture: &workspace.SketchPointCapture{Kind: "MIDPOINT", References: []workspace.SketchGeometryRef{support}}})
				found := false
				for _, c := range v.Part.Features[0].Sketch.Constraints {
					if c.Kind == "MIDPOINT" && c.References[1].EntityID == "support" {
						found = true
					}
				}
				if !found {
					t.Fatal("rectangle macro lost midpoint intent")
				}
				radius := 8.
				edit(workspace.SketchOperation{Type: "CREATE_POLYGON", OperationID: "capture-polygon", Point: &workspace.SketchPoint2{X: 20, Y: 25}, Value: &radius, Angle: 0, Sides: 3, Mode: "CIRCUMSCRIBED", Role: "CONSTRUCTION", FirstCapture: &workspace.SketchPointCapture{Kind: "POINT_ON_OBJECT", References: []workspace.SketchGeometryRef{support}}})
				head := v.Document.VersionID
				bad := workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 80, Y: 80}, Second: &workspace.SketchPoint2{X: 90, Y: 90}, FirstCapture: &workspace.SketchPointCapture{Kind: "MIDPOINT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "point", SubElement: "POINT"}}}}}}
				if _, err := service.ApplyCommand(t.Context(), v.Document.ID, bad); err == nil {
					t.Fatal("invalid capture support was silently dropped")
				}
				read, err := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
				if err != nil || read.Document.VersionID != head {
					t.Fatal("failed macro wrote partial geometry", err)
				}
			}

		}
	}
}

func sketchInteractionSource(s string) *string { return &s }
