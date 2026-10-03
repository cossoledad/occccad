package workspace

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/google/uuid"
)

func TestSketchWorkflowConstrainedDragPreviewHistory(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	p := func(x, y float64) *SketchPoint2 { return &SketchPoint2{X: x, Y: y} }
	a, b, junction := uuid.NewString(), uuid.NewString(), uuid.NewString()
	before := f.edit(workflowEntity(SketchEntity{ID: a, Kind: "LINE", Role: "PROFILE", Start: p(0, 0), End: p(10, 0)}),
		workflowEntity(SketchEntity{ID: b, Kind: "LINE", Role: "PROFILE", Start: p(10, 0), End: p(10, 10)}),
		workflowEntity(SketchEntity{ID: junction, Kind: "POINT", Role: "CONSTRUCTION", Point: p(10, 0)}),
		workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(a, "END"), workflowRef(junction, "POINT")}}),
		workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(b, "START"), workflowRef(junction, "POINT")}}),
		workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "LENGTH", References: []SketchGeometryRef{workflowRef(a, "WHOLE")}, Value: workflowValue(10), Unit: "mm"}))
	original := f.sketch(before)
	relations, _ := json.Marshal(original.Constraints)
	parameters, _ := json.Marshal(before.Part.Parameters)
	identity := map[string][2]string{}
	for _, e := range original.Entities {
		identity[e.ID] = [2]string{e.StartPointID, e.EndPointID}
	}
	op := SketchOperation{Type: "DRAG_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{a}, Origin: p(0, 0), Translation: p(3, 4), Angle: .5}
	preview, err := f.service.PreviewCommand(f.ctx, f.documentID, CommandRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: "EDIT_SKETCH", SketchID: f.sketchID, Operations: []SketchOperation{op}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.SketchCandidates) != 1 {
		t.Fatal("missing drag candidate")
	}
	cold, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if cold.Document.VersionID != before.Document.VersionID {
		t.Fatal("preview wrote a version")
	}
	check := func(s *SketchFeature) {
		t.Helper()
		byID := map[string]SketchEntity{}
		for _, e := range s.Entities {
			byID[e.ID] = e
			if identity[e.ID] != [2]string{e.StartPointID, e.EndPointID} {
				t.Fatal("drag changed stable endpoints")
			}
		}
		source, neighbor, point := byID[a], byID[b], byID[junction]
		if math.Hypot(source.Start.X-3, source.Start.Y-4) > 1e-6 || math.Hypot(source.End.X-(3+10*math.Cos(.5)), source.End.Y-(4+10*math.Sin(.5))) > 1e-6 {
			t.Fatalf("target not reached: %+v", source)
		}
		if math.Hypot(neighbor.Start.X-source.End.X, neighbor.Start.Y-source.End.Y) > 1e-7 || math.Hypot(point.Point.X-source.End.X, point.Point.Y-source.End.Y) > 1e-7 {
			t.Fatal("drag lost hard coincidence")
		}
		got, _ := json.Marshal(s.Constraints)
		if string(got) != string(relations) {
			t.Fatal("drag changed constraints or dimensions")
		}
	}
	candidate := *original
	candidate.Entities = preview.SketchCandidates[0].Entities
	check(&candidate)
	after := f.edit(op)
	check(f.sketch(after))
	gotParameters, _ := json.Marshal(after.Part.Parameters)
	if string(gotParameters) != string(parameters) {
		t.Fatal("drag changed parameters")
	}
	restored := f.command(CommandRequest{Type: "UNDO"})
	if sketchDragDigest(f.sketch(restored)) != sketchDragDigest(original) {
		t.Fatal("Undo changed original geometry or relations")
	}
	check(f.sketch(f.command(CommandRequest{Type: "REDO"})))
	cold, err = f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	check(f.sketch(cold))
}

func TestSketchWorkflowConstrainedDragFixedAndDirection(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	p := func(x, y float64) *SketchPoint2 { return &SketchPoint2{X: x, Y: y} }
	id := uuid.NewString()
	f.edit(workflowEntity(SketchEntity{ID: id, Kind: "LINE", Role: "PROFILE", Start: p(0, 0), End: p(10, 0)}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "HORIZONTAL", References: []SketchGeometryRef{workflowRef(id, "WHOLE")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(id, "START")}, FixedPoint: p(0, 0)}))
	after := f.edit(SketchOperation{Type: "DRAG_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{id}, Translation: p(3, 4), Angle: .4})
	e := f.sketch(after).Entities[0]
	if math.Hypot(e.Start.X, e.Start.Y) > 1e-8 || math.Abs(e.End.Y) > 1e-8 {
		t.Fatalf("fixed/direction changed: %+v", e)
	}
	if e.End.X < 10 || math.Abs(e.End.X-(3+10*math.Cos(.4))) > 1e-6 {
		t.Fatalf("allowed motion not followed: %+v", e)
	}
}

func TestSketchWorkflowConstrainedDragProfileExtrude(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	seed := cornerRectangleFixture()
	ids := map[string]string{}
	for _, e := range seed.Entities {
		ids[e.ID] = uuid.NewString()
	}
	ops := []SketchOperation{}
	for _, e := range seed.Entities {
		old := e.ID
		e.ID = ids[old]
		ops = append(ops, workflowEntity(e))
		kind := "HORIZONTAL"
		if old == "left" || old == "right" {
			kind = "VERTICAL"
		}
		ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: kind, References: []SketchGeometryRef{workflowRef(e.ID, "WHOLE")}}))
	}
	for _, c := range seed.Constraints {
		c.ID = uuid.NewString()
		c.ParameterID = ""
		for i := range c.References {
			c.References[i].EntityID = ids[c.References[i].EntityID]
		}
		ops = append(ops, workflowConstraint(c))
	}
	ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "LENGTH", References: []SketchGeometryRef{workflowRef(ids["right"], "WHOLE")}, Value: workflowValue(8), Unit: "mm"}))
	f.edit(ops...)
	before := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
	f.volume(before, 160)
	after := f.edit(SketchOperation{Type: "DRAG_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{ids["bottom"]}, Translation: &SketchPoint2{X: 3, Y: 4}})
	f.volume(after, 160)
	s := f.sketch(after)
	for _, e := range s.Entities {
		var source SketchEntity
		for _, se := range seed.Entities {
			if ids[se.ID] == e.ID {
				source = se
			}
		}
		if math.Hypot(e.Start.X-source.Start.X-3, e.Start.Y-source.Start.Y-4) > 1e-6 || math.Hypot(e.End.X-source.End.X-3, e.End.Y-source.End.Y-4) > 1e-6 {
			t.Fatalf("connected rectangle did not follow selected edge: %+v", e)
		}
	}
	loops, err := buildProfileLoops(Feature{ID: f.sketchID, Type: "SKETCH", Sketch: s}, true)
	if err != nil || len(loops) != 1 {
		t.Fatalf("closed profile changed: loops=%d err=%v", len(loops), err)
	}
	f.volume(f.command(CommandRequest{Type: "UNDO"}), 160)
	f.volume(f.command(CommandRequest{Type: "REDO"}), 160)
	cold, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	f.volume(cold, 160)
	if len(cold.Part.Bodies) != len(before.Part.Bodies) {
		t.Fatal("drag created an extra Body")
	}
}
