//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"reflect"
	"testing"
)

func TestSpatialAnalysisManualUpdateHistoryAndColdGeometryThroughRouter(t *testing.T) {
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
		r.RequestID = fmt.Sprintf("%s-spatial-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(ctx, v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = n
	}
	part := create("PART", "Spatial exact cube")
	run(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sid := part.Part.Features[0].ID
	run(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sid, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -5, Y: -5}, Second: &workspace.SketchPoint2{X: 5, Y: 5}}}})
	run(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sid, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	root := create("PRODUCT", "Manual spatial analysis")
	for i := 0; i < 2; i++ {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	second := root.Product.Instances[1].ID
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{15, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	definition := workspace.SpatialAnalysis{ID: uuid.NewString(), Name: "Section.1", Kind: "SECTION", Scope: "ALL", SectionType: "PLANE", Frame: workspace.InstancePose{Translation: [3]float64{0, 0, 5}, Rotation: [4]float64{0, 0, 0, 1}}, Size: [3]float64{100, 100, 2}, CutMode: "NONE", AccuracyMM: 1}
	var modelBefore []byte
	_ = db.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, root.Document.VersionID).Scan(&modelBefore)
	result, e := s.AnalyzeSpatial(ctx, root.Document.ID, workspace.SpatialAnalysisRequest{RevisionID: root.Document.VersionID, Definition: definition})
	if e != nil {
		t.Fatal(e)
	}
	if !result.Complete || result.Geometry == nil || len(result.Geometry.Bodies) != 2 {
		t.Fatal("two repeated occurrences", result)
	}
	for _, b := range result.Geometry.Bodies {
		if math.Abs(b.AreaMM2-100) > 1e-6 || len(b.Segments) == 0 {
			t.Fatal("exact section", b)
		}
	}
	var head string
	_ = db.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, root.Document.ID).Scan(&head)
	if head != root.Document.VersionID {
		t.Fatal("read-only analysis wrote history")
	}
	// A real command preview is read-only and its accepted candidate is one transaction.
	previewReq := workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", VersionID: root.Document.VersionID, ActorID: p6Actor, RequestID: uuid.NewString(), SpatialAnalysis: &definition}
	preview, e := s.PreviewCommand(ctx, root.Document.ID, previewReq)
	if e != nil {
		t.Fatal(e)
	}
	if preview.PreviewID == "" {
		t.Fatal("missing formal preview")
	}
	previewReq.PreviewID = preview.PreviewID
	saved, e := s.ApplyCommand(ctx, root.Document.ID, previewReq)
	if e != nil {
		t.Fatal("adopt preview", e)
	}
	root = saved
	if len(root.Product.SpatialAnalyses) != 1 || root.Product.SpatialAnalyses[0].Status != "CURRENT" {
		t.Fatal("save made itself stale", root.Product.SpatialAnalyses)
	}
	ref := *root.Product.SpatialAnalyses[0].Result
	stored, e := s.GetSpatialResult(ctx, root.Document.ID, root.Document.VersionID, definition.ID)
	if e != nil || stored.SourceStamp != result.SourceStamp || stored.Check.RevisionID != root.Document.VersionID {
		t.Fatal("immutable result read/rebase", e, stored)
	}
	// A rename does not recalculate or invalidate evidence.
	renamed := root.Product.SpatialAnalyses[0]
	renamed.Name = "Installation cut"
	run(&root, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", SpatialAnalysis: &renamed})
	if root.Product.SpatialAnalyses[0].Status != "CURRENT" || *root.Product.SpatialAnalyses[0].Result != ref {
		t.Fatal("rename recomputed result", root.Product.SpatialAnalyses)
	}
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{25, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	if root.Product.SpatialAnalyses[0].Status != "STALE" || *root.Product.SpatialAnalyses[0].Result != ref {
		t.Fatal("move overwrote accepted section", root.Product.SpatialAnalyses)
	}
	poses := append([]workspace.ProductInstance{}, root.Product.Instances...)
	run(&root, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", SpatialAnalysis: &renamed})
	if root.Product.SpatialAnalyses[0].Status != "CURRENT" || root.Product.SpatialAnalyses[0].Result.SourceStamp == ref.SourceStamp || !reflect.DeepEqual(root.Product.Instances, poses) {
		t.Fatal("manual update failed or moved components", root.Product.SpatialAnalyses)
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if root.Product.SpatialAnalyses[0].Status != "STALE" || *root.Product.SpatialAnalyses[0].Result != ref {
		t.Fatal("undo update", root.Product.SpatialAnalyses)
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	if root.Product.SpatialAnalyses[0].Status != "CURRENT" {
		t.Fatal("redo", root.Product.SpatialAnalyses)
	}
	// All three saved categories share IDs/history; band uses exact sample distances.
	base := definition
	base.ID = uuid.NewString()
	base.Name = "Distance.1"
	base.Kind = "DISTANCE"
	run(&root, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", SpatialAnalysis: &base})
	minimum, e := s.GetSpatialResult(ctx, root.Document.ID, root.Document.VersionID, base.ID)
	if e != nil || math.Abs(minimum.Check.Pairs[0].DistanceMM-15) > 1e-6 {
		t.Fatal("exact distance", minimum, e)
	}
	clash := base
	clash.ID = uuid.NewString()
	clash.Name = "Clash.1"
	clash.Kind = "CLASH"
	run(&root, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", SpatialAnalysis: &clash})
	// Metadata from another analysis must not invalidate the first.
	for _, a := range root.Product.SpatialAnalyses {
		if a.Status != "CURRENT" {
			t.Fatal("unrelated analysis invalidates evidence", a)
		}
	}
	paths := []workspace.InstancePath{}
	for _, b := range minimum.Check.Bodies {
		paths = append(paths, b.InstancePath)
	}
	band := base
	band.ID = uuid.NewString()
	band.Name = "Distance.2"
	band.Kind = "BAND"
	band.Scope = "BETWEEN"
	band.Selected = paths[:1]
	band.SecondSelection = paths[1:]
	band.MinimumMM = 10
	band.MaximumMM = 17
	band.AccuracyMM = 2
	// Saved definitions rebase stable occurrences to the current Revision; immediate requests do not.
	run(&root, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", SpatialAnalysis: &band})
	bands, e := s.GetSpatialResult(ctx, root.Document.ID, root.Document.VersionID, band.ID)
	if e != nil || !bands.Complete || len(bands.Geometry.Bodies) != 1 || len(bands.Geometry.Bodies[0].Triangles) == 0 {
		t.Fatal("band", bands, e)
	}
	before := root.Document.VersionID
	bad := definition
	bad.ID = uuid.NewString()
	bad.Size[0] = -1
	_, e = s.ApplyCommand(ctx, root.Document.ID, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", VersionID: before, ActorID: p6Actor, RequestID: uuid.NewString(), SpatialAnalysis: &bad})
	if e == nil {
		t.Fatal("invalid section accepted")
	}
	_ = db.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, root.Document.ID).Scan(&head)
	if head != before {
		t.Fatal("failed save changed history")
	}
	// Restart a separate production Worker and reconstruct from exact BREP artifacts.
	cold, _, _, _, _ := geometryRuntimeTestService(t, db.(*database.Pool))
	hot, e := s.AnalyzeSpatial(ctx, root.Document.ID, workspace.SpatialAnalysisRequest{RevisionID: root.Document.VersionID, Definition: definition})
	if e != nil {
		t.Fatal(e)
	}
	rebuilt, e := cold.AnalyzeSpatial(ctx, root.Document.ID, workspace.SpatialAnalysisRequest{RevisionID: root.Document.VersionID, Definition: definition})
	if e != nil || !rebuilt.Complete || !reflect.DeepEqual(hot.Geometry.Bodies, rebuilt.Geometry.Bodies) {
		t.Fatal("cold exact section mismatch", e, rebuilt.Geometry)
	}
	reopen, e := s.GetDocument(ctx, root.Document.ID)
	if e != nil || len(reopen.Product.SpatialAnalyses) != 4 {
		t.Fatal("reopen", e, reopen.Product)
	}

	// Removing the last node canonicalizes the omitted collection; its compensation
	// must still restore exactly one accepted result and support a subsequent redo.
	savedIDs := []string{}
	for _, a := range root.Product.SpatialAnalyses {
		savedIDs = append(savedIDs, a.ID)
	}
	for _, id := range savedIDs {
		run(&root, workspace.CommandRequest{Type: "DELETE_SPATIAL_ANALYSIS", TargetID: id})
	}
	if len(root.Product.SpatialAnalyses) != 0 {
		t.Fatal("delete all analyses", root.Product.SpatialAnalyses)
	}
	run(&root, workspace.CommandRequest{Type: "UNDO"})
	if len(root.Product.SpatialAnalyses) != 1 || root.Product.SpatialAnalyses[0].ID != savedIDs[len(savedIDs)-1] {
		t.Fatal("undo last deletion", root.Product.SpatialAnalyses)
	}
	run(&root, workspace.CommandRequest{Type: "REDO"})
	if len(root.Product.SpatialAnalyses) != 0 {
		t.Fatal("redo last deletion", root.Product.SpatialAnalyses)
	}
	_ = json.Valid(modelBefore)
}

func TestSpatialAnalysisTracksAcceptedContextWithoutAdoptingSourceHead(t *testing.T) {
	c := newContextInsertionScene(t, 24)
	ctx := t.Context()
	a := workspace.SpatialAnalysis{ID: uuid.NewString(), Name: "Section.1", Kind: "SECTION", Scope: "ALL", SectionType: "PLANE", Frame: workspace.InstancePose{Translation: [3]float64{0, 0, 5}, Rotation: [4]float64{0, 0, 0, 1}}, Size: [3]float64{100, 100, 2}, CutMode: "NONE", AccuracyMM: 1}
	c.run(&c.root, workspace.CommandRequest{Type: "SAVE_SPATIAL_ANALYSIS", VersionID: c.root.Document.VersionID, SpatialAnalysis: &a})
	old := *c.root.Product.SpatialAnalyses[0].Result
	if c.root.Product.SpatialAnalyses[0].Status != "CURRENT" {
		t.Fatal(c.root.Product.SpatialAnalyses)
	}
	// The fixture has a pending source Head (25), but accepted dimensions remain 20/24.
	report, e := c.s.GetSpatialResult(ctx, c.root.Document.ID, c.root.Document.VersionID, a.ID)
	if e != nil || !report.Complete {
		t.Fatal(e)
	}
	for _, b := range report.Check.Bodies {
		if b.VariantKey == "" {
			t.Fatal("shared default used", b)
		}
	}
	// Restore just one accepted input to the Part's independent default. Part revisions
	// and occurrence poses stay unchanged, so only the ContextBinding stamp can detect it.
	accepted := c.root.Product.Instances
	binding := c.root.Product.ContextBindings[0].ID
	c.run(&c.root, workspace.CommandRequest{Type: "RESTORE_CONTEXT_INPUT_DEFAULT", VersionID: c.root.Document.VersionID, ContextBindingID: binding})
	if !reflect.DeepEqual(c.root.Product.Instances, accepted) || c.root.Product.SpatialAnalyses[0].Status != "STALE" || *c.root.Product.SpatialAnalyses[0].Result != old {
		t.Fatal("context input failed to invalidate saved evidence", c.root.Product.SpatialAnalyses)
	}
	c.run(&c.root, workspace.CommandRequest{Type: "UPDATE_SPATIAL_ANALYSES", VersionID: c.root.Document.VersionID, SpatialAnalysisIDs: []string{a.ID}})
	if c.root.Product.SpatialAnalyses[0].Status != "CURRENT" || c.root.Product.SpatialAnalyses[0].Result.SourceStamp == old.SourceStamp {
		t.Fatal("context update", c.root.Product.SpatialAnalyses)
	}
}
