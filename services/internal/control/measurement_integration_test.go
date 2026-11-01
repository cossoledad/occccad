//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"reflect"
	"testing"
)

func TestMeasurementPartProductHistoryAndAcceptedOccurrencesThroughRouter(t *testing.T) {
	s, _, db, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	seq := 0
	create := func(kind, name string) workspace.DocumentView {
		v, e := s.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: kind, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.VersionID = v.Document.VersionID
		r.RequestID = fmt.Sprintf("%s-measure-%d", v.Document.ID, seq)
		next, e := s.ApplyCommand(ctx, v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = next
	}
	part := create("PART", "Measurement exact cube")
	run(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sid := part.Part.Features[0].ID
	run(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sid, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -5, Y: -5}, Second: &workspace.SketchPoint2{X: 5, Y: 5}}}})
	run(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sid, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	body := part.Part.Bodies[0]
	m := workspace.Measurement{Kind: "ITEM", Visible: true, Supports: []workspace.MeasurementSupport{{Kind: "BODY", BodyID: body.ID, Mode: "GEOMETRY"}}}
	report, e := s.Measure(ctx, part.Document.ID, workspace.MeasurementRequest{RevisionID: part.Document.VersionID, Definition: m})
	if e != nil || !report.Result.Complete || math.Abs(report.Result.Values["volumeMm3"]-1000) > 1e-6 {
		t.Fatal("exact Part through Router", report, e)
	}
	var head string
	_ = db.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, part.Document.ID).Scan(&head)
	if head != part.Document.VersionID {
		t.Fatal("read-only measurement advanced Head")
	}
	run(&part, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &m})
	if len(part.Part.Measurements) != 1 || part.Part.Measurements[0].Status != "CURRENT" {
		t.Fatal("Part save made itself stale", part.Part.Measurements)
	}
	run(&part, workspace.CommandRequest{Type: "UNDO"})
	if len(part.Part.Measurements) != 0 {
		t.Fatal("Part undo")
	}
	run(&part, workspace.CommandRequest{Type: "REDO"})
	if part.Part.Measurements[0].Status != "CURRENT" {
		t.Fatal("Part redo")
	}
	face := workspace.Measurement{Kind: "ITEM", Visible: true, Supports: []workspace.MeasurementSupport{{Kind: "FACE", BodyID: body.ID, GeometryKey: body.GeometryKey, LocalID: 1, Mode: "GEOMETRY"}}}
	run(&part, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &face})
	stored := part.Part.Measurements[1]
	if stored.Supports[0].Selection == nil || stored.Supports[0].LocalID != 0 || stored.Supports[0].GeometryKey != "" {
		t.Fatal("local locator persisted", stored)
	}
	root := create("PRODUCT", "Measurement repeated occurrences")
	for i := 0; i < 2; i++ {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	second := root.Product.Instances[1].ID
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{25, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	supports := []workspace.MeasurementSupport{}
	for _, r := range root.ResolvedInstances {
		path := r.InstancePath
		supports = append(supports, workspace.MeasurementSupport{Kind: "OCCURRENCE", InstancePath: &path, Mode: "GEOMETRY"})
	}
	m = workspace.Measurement{Kind: "BETWEEN", Visible: true, Supports: supports}
	report, e = s.Measure(ctx, root.Document.ID, workspace.MeasurementRequest{RevisionID: root.Document.VersionID, Definition: m})
	if e != nil || !report.Result.Complete || math.Abs(report.Result.Values["distanceMm"]-15) > 1e-6 {
		t.Fatal("repeated occurrence distance", report, e)
	}
	run(&root, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &m})
	saved := root.Product.Measurements[0]
	if saved.Status != "CURRENT" {
		t.Fatal("Product save made itself stale", saved)
	}
	evidence := saved.Result
	saved.Name = "Installation gap"
	run(&root, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &saved})
	if !reflect.DeepEqual(evidence, root.Product.Measurements[0].Result) {
		t.Fatal("rename recomputed evidence")
	}
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{35, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	if root.Product.Measurements[0].Status != "STALE" {
		t.Fatal("move did not invalidate measurement", root.Product.Measurements)
	}
	saved = root.Product.Measurements[0]
	poses := append([]workspace.ProductInstance{}, root.Product.Instances...)
	run(&root, workspace.CommandRequest{Type: "UPDATE_MEASUREMENT", Measurement: &saved})
	if root.Product.Measurements[0].Status != "CURRENT" || math.Abs(root.Product.Measurements[0].Result.Values["distanceMm"]-25) > 1e-6 || !reflect.DeepEqual(poses, root.Product.Instances) {
		t.Fatal("manual update moved components or wrong result", root.Product.Measurements)
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if root.Product.Measurements[0].Status != "STALE" {
		t.Fatal("undo update")
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	if root.Product.Measurements[0].Status != "CURRENT" {
		t.Fatal("redo update")
	}
	saved = root.Product.Measurements[0]
	saved.Visible = false
	run(&root, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &saved})
	if root.Product.Measurements[0].Visible {
		t.Fatal("visibility")
	}
	run(&root, workspace.CommandRequest{Type: "DELETE_MEASUREMENT", MeasurementID: saved.ID})
	if len(root.Product.Measurements) != 0 {
		t.Fatal("delete")
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if len(root.Product.Measurements) != 1 {
		t.Fatal("undo delete")
	}
	nested := create("PRODUCT", "Measurement nested occurrences")
	run(&nested, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: root.Document.ID})
	run(&nested, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: nested.Product.Instances[0].ID, Translation: [3]float64{100, 20, 0}, Rotation: [4]float64{0, 0, math.Sin(math.Pi / 4), math.Cos(math.Pi / 4)}})
	supports = nil
	for _, r := range nested.ResolvedInstances {
		p := r.InstancePath
		supports = append(supports, workspace.MeasurementSupport{Kind: "BODY", BodyID: r.BodyID, InstancePath: &p, Mode: "GEOMETRY"})
	}
	report, e = s.Measure(ctx, nested.Document.ID, workspace.MeasurementRequest{RevisionID: nested.Document.VersionID, Definition: workspace.Measurement{Kind: "BETWEEN", Supports: supports}})
	if e != nil || !report.Result.Complete || math.Abs(report.Result.Values["distanceMm"]-25) > 1e-6 || math.Abs(report.Result.Values["deltaYMm"]-25) > 1e-6 {
		t.Fatal("nested pose applied incorrectly", report, e)
	}
	// A forged referenced document with the same textual path must not resolve.
	bad := supports[0]
	path := *bad.InstancePath
	path.Segments = append([]workspace.InstancePathSegment{}, path.Segments...)
	path.Segments[0].ReferencedDocumentID = part.Document.ID
	bad.InstancePath = &path
	_, e = s.Measure(ctx, nested.Document.ID, workspace.MeasurementRequest{RevisionID: nested.Document.VersionID, Definition: workspace.Measurement{Kind: "ITEM", Supports: []workspace.MeasurementSupport{bad}}})
	if e == nil {
		t.Fatal("forged typed occurrence was accepted")
	}
}
