//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"reflect"
	"testing"
)

func TestMeasurementOutputDrivenBodyWaitAndUpdateThroughRouter(t *testing.T) {
	s, _, _, _, _ := featureAssociationRuntimeService(t)
	view, err := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Measurement driven bodies"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	run := func(request workspace.CommandRequest) {
		t.Helper()
		seq++
		request.ActorID = p6Actor
		request.VersionID = view.Document.VersionID
		request.RequestID = fmt.Sprintf("%s-derived-%d", view.Document.ID, seq)
		next, err := s.ApplyCommand(t.Context(), view.Document.ID, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
		view = next
	}
	run(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[0].ID
	run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 0, Y: 0}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	run(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 10})
	sourceFeature := view.Part.Features[len(view.Part.Features)-1]
	run(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 1})
	targetFeature := view.Part.Features[len(view.Part.Features)-1]
	m := workspace.Measurement{Kind: "ITEM", Visible: true, Supports: []workspace.MeasurementSupport{{Kind: "BODY", BodyID: sourceFeature.BodyID, Mode: "GEOMETRY"}}}
	run(workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &m})
	saved := view.Part.Measurements[0]
	outputKey := ""
	for _, p := range view.Part.Parameters {
		if p.Source.Produced != nil && p.Source.Produced.EntityID == saved.ID && p.Source.Produced.OutputSlot == "sizeZMm" {
			outputKey = p.Key
		}
	}
	if outputKey == "" {
		t.Fatal("measurement output parameter missing")
	}
	run(workspace.CommandRequest{Type: "SET_PARAMETER_EXPRESSION", ParameterID: "parameter:" + targetFeature.ID + ":length", Expression: outputKey})
	for _, f := range view.Part.Features {
		if f.ID == targetFeature.ID && math.Abs(f.Length-10) > 1e-8 {
			t.Fatal("measurement did not drive target", f)
		}
	}
	run(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + sourceFeature.ID + ":length", Value: 20, Unit: "mm"})
	if view.Part.Measurements[0].Status != "STALE" {
		t.Fatal("source change did not invalidate measurement", view.Part.Measurements)
	}
	for _, body := range view.Part.Bodies {
		if body.ID == targetFeature.BodyID && (body.GeometryKey != "" || body.DisplayFallback == nil) {
			t.Fatal("dependent Body used stale value or lost its last display", body)
		}
	}
	for _, f := range view.Part.Features {
		if f.ID == targetFeature.ID && f.EvaluationStatus != "BLOCKED" {
			t.Fatal("dependent feature not blocked", f)
		}
	}
	saved = view.Part.Measurements[0]
	run(workspace.CommandRequest{Type: "UPDATE_MEASUREMENT", Measurement: &saved})
	if view.Part.Measurements[0].Status != "CURRENT" {
		t.Fatal("measurement update remains stale", view.Part.Measurements)
	}
	for _, f := range view.Part.Features {
		if f.ID == targetFeature.ID && (math.Abs(f.Length-20) > 1e-8 || f.EvaluationStatus == "BLOCKED") {
			t.Fatal("target failed to recover", f)
		}
	}
	for _, body := range view.Part.Bodies {
		if body.ID == targetFeature.BodyID && (body.GeometryKey == "" || body.DisplayFallback != nil) {
			t.Fatal("dependent Body not recovered", body)
		}
	}
	run(workspace.CommandRequest{Type: "UNDO"})
	if view.Part.Measurements[0].Status != "STALE" {
		t.Fatal("measurement update undo", view.Part.Measurements)
	}
	run(workspace.CommandRequest{Type: "REDO"})
	if view.Part.Measurements[0].Status != "CURRENT" {
		t.Fatal("measurement update redo", view.Part.Measurements)
	}
}

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
	var areaPath workspace.InstancePath
	for _, resolved := range root.ResolvedInstances {
		if len(resolved.InstancePath.Segments) > 0 && resolved.InstancePath.Segments[len(resolved.InstancePath.Segments)-1].InstanceID == second {
			areaPath = resolved.InstancePath
		}
	}
	areaSupport := stored.Supports[0]
	areaSupport.InstancePath = &areaPath
	areaDefinition := workspace.Measurement{Kind: "ITEM", Visible: true, Supports: []workspace.MeasurementSupport{areaSupport}}
	run(&root, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &areaDefinition})
	areaBefore := root.Product.Measurements[1]
	if areaBefore.Status != "CURRENT" || math.Abs(areaBefore.Result.Values["areaMm2"]-100) > 1e-6 {
		t.Fatal("Product face area", areaBefore)
	}
	analysis := workspace.SpatialAnalysis{Kind: "CLASH", Scope: "ALL"}
	run(&root, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", SpatialAnalysis: &analysis})
	if root.Product.SpatialAnalyses[0].Status != "CURRENT" {
		t.Fatal("initial interference currency", root.Product.SpatialAnalyses)
	}
	run(&root, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "DisplayIndependent", Value: 1, Unit: "mm"})
	if root.Product.Measurements[1].Status != "CURRENT" || root.Product.SpatialAnalyses[0].Status != "CURRENT" {
		t.Fatal("unrelated Product parameter invalidated analyses")
	}
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{45, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	areaAfter := root.Product.Measurements[1]
	if root.Product.Measurements[0].Status != "STALE" || root.Product.SpatialAnalyses[0].Status != "STALE" || areaAfter.Status != "CURRENT" || !reflect.DeepEqual(areaBefore.Result, areaAfter.Result) {
		t.Fatal("placement facets invalidated intrinsic area or kept pair analyses current", root.Product.Measurements, root.Product.SpatialAnalyses)
	}
	if areaAfter.OutputCurrency["sizeXMm"] != "STALE" || math.Abs(areaAfter.DisplayAnchors["centroid"][0]-areaBefore.Result.Anchors["centroid"][0]-10) > 1e-6 {
		t.Fatal("area label did not follow rigid pose or stale coordinate accepted", areaAfter)
	}
	outputID := ""
	for _, p := range root.Product.Parameters {
		if p.Source.Produced != nil && p.Source.Produced.EntityID == saved.ID && p.PropertySlot == "distanceMm" {
			outputID = p.ParameterID
		}
	}
	run(&root, workspace.CommandRequest{Type: "CREATE_PUBLICATION", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: outputID})
	publication := root.Product.Publications[len(root.Product.Publications)-1]
	if publication.Resolution.Status != "CONNECTED" || publication.Resolution.CurrencyStatus != "STALE" {
		t.Fatal("stale output publication", publication)
	}
	consumer := create("PRODUCT", "Measurement output consumer")
	run(&consumer, workspace.CommandRequest{Type: "CREATE_PARAMETER", Value: 1, Unit: "mm"})
	_, e = s.ApplyCommand(ctx, consumer.Document.ID, workspace.CommandRequest{Type: "SET_PARAMETER_EXTERNAL", ActorID: p6Actor, VersionID: consumer.Document.VersionID, RequestID: consumer.Document.ID + "-reject-stale", ParameterID: consumer.Product.Parameters[0].ParameterID, SourceDocumentID: root.Document.ID, PublicationID: publication.ID})
	if e == nil {
		t.Fatal("stale Product output accepted externally")
	}
	run(&root, workspace.CommandRequest{Type: "UPDATE_DERIVED", TargetKind: "ALL"})
	if root.Product.Measurements[0].Status != "CURRENT" || root.Product.Measurements[1].OutputCurrency["sizeXMm"] != "CURRENT" || root.Product.SpatialAnalyses[0].Status != "CURRENT" {
		t.Fatal("batch update did not update all producers")
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if root.Product.Measurements[0].Status != "STALE" || root.Product.SpatialAnalyses[0].Status != "STALE" {
		t.Fatal("batch update undo")
	}
	run(&root, workspace.CommandRequest{Type: "DELETE_PRODUCT_PUBLICATION", PublicationID: publication.ID})
	run(&root, workspace.CommandRequest{Type: "DELETE_MEASUREMENT", MeasurementID: saved.ID})
	if len(root.Product.Measurements) != 1 {
		t.Fatal("delete")
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if len(root.Product.Measurements) != 2 {
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
	if e != nil || !report.Result.Complete || math.Abs(report.Result.Values["distanceMm"]-35) > 1e-6 || math.Abs(report.Result.Values["deltaYMm"]-35) > 1e-6 {
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

func TestMeasurementPartRepeatedSavesThroughRouter(t *testing.T) {
	s, db, _, _, _ := featureAssociationRuntimeService(t)
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
	// Multiple retained measurements, then a second Body and measurements
	// bound by an earlier read-only query (the panel's save path).
	for _, kind := range []string{"EDGE", "VERTEX"} {
		definition := workspace.Measurement{Kind: "ITEM", Visible: true, Supports: []workspace.MeasurementSupport{{Kind: kind, BodyID: body.ID, GeometryKey: body.GeometryKey, LocalID: 1, Mode: "GEOMETRY"}}}
		query, err := s.Measure(ctx, part.Document.ID, workspace.MeasurementRequest{RevisionID: part.Document.VersionID, Definition: definition})
		if err != nil || !query.Result.Complete {
			t.Fatal(kind, query, err)
		}
		run(&part, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &query.Definition})
	}
	run(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sid, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 20})
	secondBody := part.Part.Bodies[len(part.Part.Bodies)-1]
	for _, kind := range []string{"ITEM", "BETWEEN", "ANGLE_POINTS"} {
		supports := []workspace.MeasurementSupport{{Kind: "BODY", BodyID: secondBody.ID, Mode: "GEOMETRY"}}
		if kind == "BETWEEN" {
			supports = append(supports, workspace.MeasurementSupport{Kind: "BODY", BodyID: body.ID, Mode: "GEOMETRY"})
		}
		if kind == "ANGLE_POINTS" {
			supports = nil
			for _, id := range []uint64{1, 2, 3} {
				supports = append(supports, workspace.MeasurementSupport{Kind: "VERTEX", BodyID: secondBody.ID, GeometryKey: secondBody.GeometryKey, LocalID: id, Mode: "GEOMETRY"})
			}
		}
		query, err := s.Measure(ctx, part.Document.ID, workspace.MeasurementRequest{RevisionID: part.Document.VersionID, Definition: workspace.Measurement{Kind: kind, Visible: true, Supports: supports}})
		if err != nil || !query.Result.Complete {
			t.Fatal(kind, query, err)
		}
		run(&part, workspace.CommandRequest{Type: "SAVE_MEASUREMENT", Measurement: &query.Definition})
	}

}
