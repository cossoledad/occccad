package workspace

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
)

type sketchWorkflowFixture struct {
	t                           *testing.T
	ctx                         context.Context
	service                     *Service
	actor, documentID, sketchID string
}

func newSketchWorkflowFixture(t *testing.T) *sketchWorkflowFixture {
	t.Helper()
	url, address, root := os.Getenv("OCCCCAD_TEST_DATABASE_URL"), os.Getenv("OCCCCAD_TEST_WORKER_ADDRESS"), os.Getenv("OCCCCAD_TEST_ARTIFACT_ROOT")
	if url == "" || address == "" || root == "" {
		t.Skip("isolated database, matching-source Worker and artifact root are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	worker, err := geometry.Open(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = worker.Close() })
	if _, err = worker.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	store, err := artifact.NewLocalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithArtifacts(pool, worker, artifact.NewService(pool, store))
	actor := "00000000-0000-7000-8000-000000000001"
	view, err := service.CreateDocument(ctx, CreateDocumentRequest{RequestID: uuid.NewString(), Name: "sketch-workflow-" + uuid.NewString(), Type: "PART", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	f := &sketchWorkflowFixture{t: t, ctx: ctx, service: service, actor: actor, documentID: view.Document.ID}
	view = f.command(CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	for _, feature := range view.Part.Features {
		if feature.Type == "SKETCH" {
			f.sketchID = feature.ID
		}
	}
	if f.sketchID == "" {
		t.Fatal("CREATE_SKETCH did not persist sketch")
	}
	return f
}
func (f *sketchWorkflowFixture) command(r CommandRequest) DocumentView {
	f.t.Helper()
	r.RequestID = uuid.NewString()
	r.ActorID = f.actor
	if r.SketchID == "" {
		r.SketchID = f.sketchID
	}
	v, e := f.service.ApplyCommand(f.ctx, f.documentID, r)
	if e != nil {
		f.t.Fatalf("%s: %v", r.Type, e)
	}
	return v
}
func (f *sketchWorkflowFixture) edit(ops ...SketchOperation) DocumentView {
	return f.command(CommandRequest{Type: "EDIT_SKETCH", Operations: ops})
}
func (f *sketchWorkflowFixture) sketch(v DocumentView) *SketchFeature {
	f.t.Helper()
	for _, feature := range v.Part.Features {
		if feature.ID == f.sketchID {
			return feature.Sketch
		}
	}
	f.t.Fatal("missing sketch")
	return nil
}
func (f *sketchWorkflowFixture) volume(v DocumentView, want float64) {
	f.t.Helper()
	total := 0.0
	for _, artifact := range v.Artifacts {
		total += artifact.Volume
	}
	if len(v.Artifacts) == 0 && v.Artifact != nil {
		total = v.Artifact.Volume
	}
	if math.Abs(total-want) > 1e-6*math.Max(1, want) {
		f.t.Fatalf("exact per-Body solid volume got %g want %.12g bodies=%d", total, want, len(v.Artifacts))
	}
}

func workflowEntity(e SketchEntity) SketchOperation {
	return SketchOperation{Type: "ADD_ENTITY", Entity: &e}
}
func workflowConstraint(c SketchConstraint) SketchOperation {
	return SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &c}
}
func workflowRef(id, sub string) SketchGeometryRef {
	return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
}
func workflowValue(v float64) *float64 { return &v }

func TestSketchWorkflowLinkedArcMirrorExtrudeHistoryColdRead(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	arc := SketchEntity{ID: uuid.NewString(), Kind: "ARC", Center: &SketchPoint2{}, Radius: 5, StartAngle: -math.Pi / 2, EndAngle: math.Pi / 2, Role: "PROFILE"}
	radius := uuid.NewString()
	center := uuid.NewString()
	axis := SketchGeometryRef{Target: "SKETCH_Y_AXIS", SubElement: "DIRECTION"}
	f.edit(workflowEntity(arc), workflowConstraint(SketchConstraint{ID: radius, Kind: "RADIUS", References: []SketchGeometryRef{workflowRef(arc.ID, "WHOLE")}, Value: workflowValue(5), Unit: "mm"}), workflowConstraint(SketchConstraint{ID: center, Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(arc.ID, "CENTER")}, FixedPoint: &SketchPoint2{}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "POINT_ON_OBJECT", References: []SketchGeometryRef{workflowRef(arc.ID, "START"), axis}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "POINT_ON_OBJECT", References: []SketchGeometryRef{workflowRef(arc.ID, "END"), axis}}))
	mirrored := f.edit(SketchOperation{Type: "MIRROR_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{arc.ID}, Axis: &axis, MirrorMode: "LINKED"})
	if f.sketch(mirrored).Solve.Status == "CONFLICTING" {
		t.Fatalf("mirror solve %+v", f.sketch(mirrored).Solve)
	}
	view := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 3})
	f.volume(view, 75*math.Pi)
	originalJSON, _ := json.Marshal(f.sketch(view))
	parameterID := ""
	for _, c := range f.sketch(view).Constraints {
		if c.ID == radius {
			parameterID = c.ParameterID
		}
	}
	if parameterID == "" {
		t.Fatal("radius missing stable ParameterId")
	}
	view = f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: radius, Value: workflowValue(7)})
	f.volume(view, 147*math.Pi)
	changedJSON, _ := json.Marshal(f.sketch(view))
	f.volume(f.command(CommandRequest{Type: "UNDO"}), 75*math.Pi)
	view = f.command(CommandRequest{Type: "REDO"})
	f.volume(view, 147*math.Pi)
	gotJSON, _ := json.Marshal(f.sketch(view))
	if string(gotJSON) != string(changedJSON) {
		t.Fatal("Redo changed stable geometry/constraint identities or solved data")
	}
	cold := NewWithArtifacts(f.service.database, f.service.worker, f.service.artifacts)
	reopened, e := cold.GetDocument(f.ctx, f.documentID, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	f.volume(reopened, 147*math.Pi)
	for _, c := range f.sketch(reopened).Constraints {
		if c.ID == radius && c.ParameterID != parameterID {
			t.Fatal("Parameter identity changed on reopen")
		}
	}
	if string(originalJSON) == string(changedJSON) {
		t.Fatal("radius edit did not change solved geometry")
	}
}

func TestSketchWorkflowEllipseArcExactSplitExtrude(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	e := SketchEntity{ID: uuid.NewString(), Kind: "ELLIPTICAL_ARC", Center: &SketchPoint2{}, MajorRadius: 8, MinorRadius: 3, Rotation: .3, StartAngle: 0, EndAngle: math.Pi, Role: "PROFILE"}
	p0, p1 := ellipsePoint(e, 0), ellipsePoint(e, math.Pi)
	line := SketchEntity{ID: uuid.NewString(), Kind: "LINE", Start: &p1, End: &p0, Role: "PROFILE"}
	f.edit(workflowEntity(e), workflowEntity(line), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(e.ID, "END"), workflowRef(line.ID, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(e.ID, "START"), workflowRef(line.ID, "END")}}))
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 24*math.Pi)
	view := f.edit(SketchOperation{Type: "SPLIT_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{e.ID}, Parameters: []float64{.37}})
	f.volume(view, 24*math.Pi)
	view = f.command(CommandRequest{Type: "UNDO"})
	f.volume(view, 24*math.Pi)
	view = f.command(CommandRequest{Type: "REDO"})
	f.volume(view, 24*math.Pi)
}

func TestSketchWorkflowParallelSpacingReferenceAndReversal(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	a := SketchEntity{ID: uuid.NewString(), Kind: "LINE", Start: &SketchPoint2{X: 11, Y: 19}, End: &SketchPoint2{X: 23, Y: 24}, Role: "CONSTRUCTION"}
	b := SketchEntity{ID: uuid.NewString(), Kind: "LINE", Start: &SketchPoint2{X: 19, Y: 37}, End: &SketchPoint2{X: 7, Y: 32}, Role: "CONSTRUCTION"}
	dim := SketchConstraint{ID: uuid.NewString(), Kind: "DISTANCE", References: []SketchGeometryRef{workflowRef(a.ID, "WHOLE"), workflowRef(b.ID, "WHOLE")}, Value: workflowValue(8), Unit: "mm"}
	view := f.edit(workflowEntity(a), workflowEntity(b), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED", References: []SketchGeometryRef{workflowRef(a.ID, "WHOLE")}}), workflowConstraint(dim))
	assertGap := func(v DocumentView, want float64) {
		t.Helper()
		s := f.sketch(v)
		var aa, bb SketchEntity
		for _, e := range s.Entities {
			if e.ID == a.ID {
				aa = e
			}
			if e.ID == b.ID {
				bb = e
			}
		}
		dx, dy := aa.End.X-aa.Start.X, aa.End.Y-aa.Start.Y
		gap := math.Abs(dx*(bb.Start.Y-aa.Start.Y)-dy*(bb.Start.X-aa.Start.X)) / math.Hypot(dx, dy)
		if math.Abs(gap-want) > 1e-7 {
			t.Fatalf("support spacing %g want %g", gap, want)
		}
	}
	assertGap(view, 8)
	view = f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: dim.ID, Value: workflowValue(13)})
	assertGap(view, 13)
	s := f.sketch(view)
	for _, c := range s.Constraints {
		if c.ID == dim.ID {
			dim = c
		}
	}
	parameterID := dim.ParameterID
	beforeDOF := s.Solve.DegreesOfFreedom
	dim.Reference = true
	view = f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: dim.ID, Constraint: &dim})
	if f.sketch(view).Solve.DegreesOfFreedom <= beforeDOF {
		t.Fatal("reference dimension still removes freedom")
	}
	// Fixed first line and the visible parallel relation remain, while the
	// measured distance follows a genuine geometry move rather than driving it.
	var point SketchPoint2
	for _, e := range f.sketch(view).Entities {
		if e.ID == b.ID {
			point = *e.Start
		}
	}
	point.X += -5 * 5.0 / 13
	point.Y += 5 * 12.0 / 13
	view = f.edit(SketchOperation{Type: "UPDATE_ENTITY_POINT", EntityID: b.ID, SubElement: "START", Point: &point})
	measured := 0.0
	for _, c := range f.sketch(view).Constraints {
		if c.ID == dim.ID {
			if c.ParameterID != parameterID || c.Value == nil {
				t.Fatal("reference lost parameter identity or measurement")
			}
			measured = *c.Value
		}
	}
	if math.Abs(measured-13) < 1e-6 {
		t.Fatal("reference measurement stayed stale after geometry move")
	}
	assertGap(view, measured)
}

func TestSketchWorkflowRectangleBatchFilletsDimensionUpdate(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	s := cornerRectangleFixture()
	mapping := map[string]string{}
	for _, e := range s.Entities {
		mapping[e.ID] = uuid.NewString()
	}
	widthID := ""
	ops := []SketchOperation{}
	for _, e := range s.Entities {
		e.ID = mapping[e.ID]
		ops = append(ops, workflowEntity(e))
	}
	for _, c := range s.Constraints {
		c.ID = uuid.NewString()
		for i := range c.References {
			c.References[i].EntityID = mapping[c.References[i].EntityID]
		}
		if c.Kind == "LENGTH" {
			widthID = c.ID
			c.ParameterID = ""
		}
		ops = append(ops, workflowConstraint(c))
	}
	for _, id := range []string{"bottom", "top"} {
		ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "HORIZONTAL", References: []SketchGeometryRef{workflowRef(mapping[id], "WHOLE")}}))
	}
	for _, id := range []string{"left", "right"} {
		ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "VERTICAL", References: []SketchGeometryRef{workflowRef(mapping[id], "WHOLE")}}))
	}
	ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "LENGTH", References: []SketchGeometryRef{workflowRef(mapping["left"], "WHOLE")}, Value: workflowValue(8), Unit: "mm"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(mapping["bottom"], "START")}, FixedPoint: &SketchPoint2{}}))
	f.edit(ops...)
	corners := [][4]string{{"bottom", "left", "START", "END"}, {"bottom", "right", "END", "START"}, {"right", "top", "END", "START"}, {"top", "left", "END", "START"}}
	ops = nil
	for _, c := range corners {
		ops = append(ops, cornerOperation("FILLET_ENTITIES", uuid.NewString(), mapping[c[0]], mapping[c[1]], c[2], c[3]))
	}
	f.edit(ops...)
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 2*(80-4+math.Pi))
	view := f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: widthID, Value: workflowValue(12)})
	f.volume(view, 2*(96-4+math.Pi))
	f.volume(f.command(CommandRequest{Type: "UNDO"}), 2*(80-4+math.Pi))
}

func TestSketchWorkflowQuickTrimPeriodicCirclesClosedUnion(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	a, b := uuid.NewString(), uuid.NewString()
	f.edit(workflowEntity(SketchEntity{ID: a, Kind: "CIRCLE", Center: &SketchPoint2{}, Radius: 5, Role: "PROFILE"}), workflowEntity(SketchEntity{ID: b, Kind: "CIRCLE", Center: &SketchPoint2{X: 6}, Radius: 5, Role: "PROFILE"}))
	hitA, hitB := .01, .5
	v := f.edit(SketchOperation{Type: "QUICK_TRIM", OperationID: uuid.NewString(), EntityIDs: []string{a}, BoundaryIDs: []string{b}, HitParameter: &hitA, TrimMode: "DELETE_HIT"}, SketchOperation{Type: "QUICK_TRIM", OperationID: uuid.NewString(), EntityIDs: []string{b}, BoundaryIDs: []string{a}, HitParameter: &hitB, TrimMode: "DELETE_HIT"})
	if v.SketchAnalyses[f.sketchID].Status != "CLOSED" {
		raw, _ := json.Marshal(f.sketch(v))
		t.Logf("trim model %s", raw)
	}
	area := 50*math.Pi - 50*math.Acos(.6) + 24
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 2*area)
}

func TestSketchWorkflowExtendEndpointClosesRectangle(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	s := cornerRectangleFixture()
	ops := []SketchOperation{}
	mapping := map[string]string{}
	for _, e := range s.Entities {
		mapping[e.ID] = uuid.NewString()
	}
	for _, e := range s.Entities {
		e.ID = mapping[e.ID]
		if e.ID == mapping["bottom"] {
			e.End = &SketchPoint2{X: 7}
		}
		ops = append(ops, workflowEntity(e))
	}
	for _, c := range s.Constraints {
		if c.ID == "join-bottom" || c.Kind == "LENGTH" {
			continue
		}
		c.ID = uuid.NewString()
		for i := range c.References {
			c.References[i].EntityID = mapping[c.References[i].EntityID]
		}
		ops = append(ops, workflowConstraint(c))
	}
	f.edit(ops...)
	end := workflowRef(mapping["bottom"], "END")
	f.edit(SketchOperation{Type: "EXTEND_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{mapping["bottom"]}, BoundaryIDs: []string{mapping["right"]}, FirstReference: &end, Point: &SketchPoint2{X: 10}})
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 160)
}

func TestSketchWorkflowRationalSplineKnotSplitExtrude(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	id, left, bottom := uuid.NewString(), uuid.NewString(), uuid.NewString()
	spline := SketchEntity{ID: id, Kind: "SPLINE", Mode: "CONTROL", Degree: 2, Poles: []SketchPoint2{{5, 0}, {5, 5}, {0, 5}}, Knots: []float64{0, 1}, Multiplicities: []uint32{3, 3}, Weights: []float64{1, math.Sqrt(.5), 1}, ParameterStart: 0, ParameterEnd: 1, Role: "PROFILE"}
	f.edit(workflowEntity(spline), workflowEntity(SketchEntity{ID: left, Kind: "LINE", Start: &SketchPoint2{Y: 5}, End: &SketchPoint2{}, Role: "PROFILE"}), workflowEntity(SketchEntity{ID: bottom, Kind: "LINE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 5}, Role: "PROFILE"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(id, "END"), workflowRef(left, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(left, "END"), workflowRef(bottom, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(bottom, "END"), workflowRef(id, "START")}}))
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 12.5*math.Pi)
	// Inserting a real knot and exact segmentation retain the rational conic,
	// rather than fitting a few sampled points to a different curve.
	f.volume(f.edit(SketchOperation{Type: "EDIT_SPLINE_POINT", OperationID: uuid.NewString(), EntityID: id, PointAction: "INSERT", KnotParameter: workflowValue(.4)}), 12.5*math.Pi)
	f.volume(f.edit(SketchOperation{Type: "SPLIT_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{id}, Parameters: []float64{.3}}), 12.5*math.Pi)
	f.volume(f.command(CommandRequest{Type: "UNDO"}), 12.5*math.Pi)
	f.volume(f.command(CommandRequest{Type: "REDO"}), 12.5*math.Pi)
}

func TestSketchWorkflowHolesIslandsDisconnectedSolidsOneBody(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	ops := []SketchOperation{}
	for i, r := range []float64{10, 6, 2, 3} {
		x := 0.0
		if i == 3 {
			x = 30
		}
		ops = append(ops, workflowEntity(SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Center: &SketchPoint2{X: x}, Radius: r, Role: "PROFILE"}))
	}
	f.edit(ops...)
	view := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
	f.volume(view, 154*math.Pi)
	if len(view.Part.Bodies) != 1 || len(view.Artifacts) != 1 {
		t.Fatalf("disconnected solids split Body: bodies=%d artifacts=%d", len(view.Part.Bodies), len(view.Artifacts))
	}
	analysis := view.SketchAnalyses[f.sketchID]
	if !analysis.GeometryVerified || analysis.RegionCount != 3 || analysis.LoopCount != 4 {
		t.Fatalf("exact region analysis %+v", analysis)
	}
}

func TestSketchWorkflowMixedChainIndependentOffsetExtrude(t *testing.T) {
	for _, mode := range []string{"MITER", "ROUND"} {
		t.Run(mode, func(t *testing.T) {
			f := newSketchWorkflowFixture(t)
			arc, line := uuid.NewString(), uuid.NewString()
			f.edit(workflowEntity(SketchEntity{ID: arc, Kind: "ARC", Center: &SketchPoint2{}, Radius: 5, StartAngle: 0, EndAngle: math.Pi, Role: "PROFILE"}), workflowEntity(SketchEntity{ID: line, Kind: "LINE", Start: &SketchPoint2{X: -5}, End: &SketchPoint2{X: 5}, Role: "PROFILE"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(arc, "END"), workflowRef(line, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(line, "END"), workflowRef(arc, "START")}}))
			f.edit(SketchOperation{Type: "OFFSET_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{line, arc}, Value: workflowValue(-1), Mode: mode}, SketchOperation{Type: "UPDATE_ENTITY_ROLE", EntityID: arc, Role: "CONSTRUCTION"}, SketchOperation{Type: "UPDATE_ENTITY_ROLE", EntityID: line, Role: "CONSTRUCTION"})
			area := 18*math.Pi + 36*math.Asin(1.0/6) + math.Sqrt(35)
			if mode == "ROUND" {
				area = 18.5*math.Pi + 10
			}
			f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 2*area)
			before, e := f.service.GetDocument(f.ctx, f.documentID, f.actor)
			if e != nil {
				t.Fatal(e)
			}
			_, e = f.service.ApplyCommand(f.ctx, f.documentID, CommandRequest{Type: "EDIT_SKETCH", RequestID: uuid.NewString(), ActorID: f.actor, SketchID: f.sketchID, Operations: []SketchOperation{{Type: "OFFSET_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{arc}, Value: workflowValue(5), Mode: mode}}})
			if e == nil {
				t.Fatal("zero radius offset was accepted")
			}
			after, e := f.service.GetDocument(f.ctx, f.documentID, f.actor)
			if e != nil {
				t.Fatal(e)
			}
			a, _ := json.Marshal(before.Part)
			b, _ := json.Marshal(after.Part)
			if string(a) != string(b) {
				t.Fatal("illegal offset modified committed model")
			}
		})
	}
}

func TestSketchWorkflowFitPointEditRebuildsCanonicalAndSolid(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	id, line := uuid.NewString(), uuid.NewString()
	f.edit(workflowEntity(SketchEntity{ID: id, Kind: "SPLINE", Mode: "FIT", Degree: 3, ControlPoints: []SketchPoint2{{0, 0}, {5, 4}, {10, 0}}, Role: "PROFILE"}), workflowEntity(SketchEntity{ID: line, Kind: "LINE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{}, Role: "PROFILE"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(id, "END"), workflowRef(line, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(line, "END"), workflowRef(id, "START")}}))
	before := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
	var spline SketchEntity
	for _, e := range f.sketch(before).Entities {
		if e.ID == id {
			spline = e
		}
	}
	pointID := spline.ControlPointIDs[1]
	oldPoles, _ := json.Marshal(spline.Poles)
	after := f.edit(SketchOperation{Type: "UPDATE_ENTITY_POINT", EntityID: id, SubElement: "CONTROL", ControlPointID: pointID, Point: &SketchPoint2{X: 5, Y: 6}})
	var updated SketchEntity
	for _, e := range f.sketch(after).Entities {
		if e.ID == id {
			updated = e
		}
	}
	poles, _ := json.Marshal(updated.Poles)
	if string(poles) == string(oldPoles) || updated.ControlPointIDs[1] != pointID {
		t.Fatal("FIT move retained stale canonical curve or changed fit point identity")
	}
	// Equal chord intervals put the middle interpolation point at the parameter
	// midpoint; it must be on the accepted true curve after the edit.
	p, err := evaluateCanonicalSpline(updated, (updated.ParameterStart+updated.ParameterEnd)/2)
	if err != nil || math.Hypot(p.X-5, p.Y-6) > 1e-6 {
		t.Fatalf("canonical curve misses edited fit point: %+v %v", p, err)
	}
	volume := func(v DocumentView) float64 {
		total := 0.0
		for _, a := range v.Artifacts {
			total += a.Volume
		}
		return total
	}
	if volume(after) <= volume(before)+1 {
		t.Fatalf("solid did not follow FIT edit: %g -> %g", volume(before), volume(after))
	}
	// Independent dense Green-integral oracle checks the true accepted curve,
	// not entity counts or a successful response.
	area := 0.0
	previous, err := evaluateCanonicalSpline(updated, updated.ParameterStart)
	if err != nil {
		t.Fatal(err)
	}
	const steps = 10000
	for i := 1; i <= steps; i++ {
		p, err := evaluateCanonicalSpline(updated, updated.ParameterStart+(updated.ParameterEnd-updated.ParameterStart)*float64(i)/steps)
		if err != nil {
			t.Fatal(err)
		}
		area += (previous.X*p.Y - p.X*previous.Y) / 2
		previous = p
	}
	f.volume(after, 2*math.Abs(area))
	f.volume(f.command(CommandRequest{Type: "UNDO"}), volume(before))
}

func TestSketchWorkflowSelfMirrorCircleTracksAxisEdits(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	axisID, circleID, startID, endID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	start := SketchConstraint{ID: startID, Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(axisID, "START")}, FixedPoint: &SketchPoint2{Y: -10}}
	end := SketchConstraint{ID: endID, Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(axisID, "END")}, FixedPoint: &SketchPoint2{Y: 10}}
	f.edit(workflowEntity(SketchEntity{ID: axisID, Kind: "LINE", Start: &SketchPoint2{Y: -10}, End: &SketchPoint2{Y: 10}, Role: "CONSTRUCTION"}), workflowEntity(SketchEntity{ID: circleID, Kind: "CIRCLE", Center: &SketchPoint2{}, Radius: 2, Role: "PROFILE"}), workflowConstraint(start), workflowConstraint(end), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "RADIUS", References: []SketchGeometryRef{workflowRef(circleID, "WHOLE")}, Value: workflowValue(2), Unit: "mm"}))
	axis := workflowRef(axisID, "WHOLE")
	view := f.edit(SketchOperation{Type: "MIRROR_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{circleID}, Axis: &axis, MirrorMode: "LINKED"})
	if len(f.sketch(view).Entities) != 2 {
		t.Fatal("self mirrored circle duplicated")
	}
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 8*math.Pi)
	start.FixedPoint = &SketchPoint2{X: 3, Y: -10}
	end.FixedPoint = &SketchPoint2{X: 3, Y: 10}
	view = f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: startID, Constraint: &start}, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: endID, Constraint: &end})
	f.volume(view, 8*math.Pi)
	for _, e := range f.sketch(view).Entities {
		if e.ID == circleID && math.Abs(e.Center.X-3) > 1e-7 {
			t.Fatalf("self mirror detached after axis translation %+v", e.Center)
		}
	}
	start.FixedPoint = &SketchPoint2{X: -10, Y: 4}
	end.FixedPoint = &SketchPoint2{X: 10, Y: 4}
	view = f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: startID, Constraint: &start}, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: endID, Constraint: &end})
	f.volume(view, 8*math.Pi)
	for _, e := range f.sketch(view).Entities {
		if e.ID == circleID && math.Abs(e.Center.Y-4) > 1e-7 {
			t.Fatalf("self mirror detached after axis rotation %+v", e.Center)
		}
	}
}

func TestSketchWorkflowCopyTransformDeleteExtrudeHistory(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	s := cornerRectangleFixture()
	ids := map[string]string{}
	selected := []string{}
	ops := []SketchOperation{}
	for _, e := range s.Entities {
		ids[e.ID] = uuid.NewString()
		selected = append(selected, ids[e.ID])
	}
	for _, e := range s.Entities {
		e.ID = ids[e.ID]
		ops = append(ops, workflowEntity(e))
	}
	for _, c := range s.Constraints {
		if c.Kind != "COINCIDENT" {
			continue
		}
		c.ID = uuid.NewString()
		for i := range c.References {
			c.References[i].EntityID = ids[c.References[i].EntityID]
		}
		ops = append(ops, workflowConstraint(c))
	}
	f.edit(ops...)
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 160)
	operationID := uuid.NewString()
	view := f.edit(SketchOperation{Type: "TRANSFORM_ENTITIES", OperationID: operationID, EntityIDs: selected, Copy: true, ConstraintPolicy: "INTERNAL", Angle: .3, Origin: &SketchPoint2{}, Translation: &SketchPoint2{X: 30}})
	f.volume(view, 320)
	copyIDs := []string{}
	for _, e := range f.sketch(view).Entities {
		if e.CreatedByOperationID == operationID {
			copyIDs = append(copyIDs, e.ID)
		}
	}
	if len(copyIDs) != 4 {
		t.Fatal("copy provenance not preserved")
	}
	f.volume(f.edit(SketchOperation{Type: "TRANSFORM_ENTITIES", OperationID: uuid.NewString(), EntityIDs: selected, ConstraintPolicy: "INTERNAL", Angle: .7, Translation: &SketchPoint2{X: -20}}), 320)
	f.volume(f.edit(SketchOperation{Type: "DELETE_ENTITIES", EntityIDs: copyIDs}), 160)
	f.volume(f.command(CommandRequest{Type: "UNDO"}), 320)
	f.volume(f.command(CommandRequest{Type: "REDO"}), 160)
}

func workflowParametricRectangle(f *sketchWorkflowFixture) (map[string]string, string) {
	s := cornerRectangleFixture()
	ids := map[string]string{}
	ops := []SketchOperation{}
	widthID := ""
	for _, e := range s.Entities {
		ids[e.ID] = uuid.NewString()
	}
	for _, e := range s.Entities {
		e.ID = ids[e.ID]
		ops = append(ops, workflowEntity(e))
	}
	for _, c := range s.Constraints {
		c.ID = uuid.NewString()
		for i := range c.References {
			c.References[i].EntityID = ids[c.References[i].EntityID]
		}
		if c.Kind == "LENGTH" {
			c.ParameterID = ""
			widthID = c.ID
		}
		ops = append(ops, workflowConstraint(c))
	}
	for _, id := range []string{"bottom", "top"} {
		ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "HORIZONTAL", References: []SketchGeometryRef{workflowRef(ids[id], "WHOLE")}}))
	}
	for _, id := range []string{"left", "right"} {
		ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "VERTICAL", References: []SketchGeometryRef{workflowRef(ids[id], "WHOLE")}}))
	}
	ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "LENGTH", References: []SketchGeometryRef{workflowRef(ids["left"], "WHOLE")}, Value: workflowValue(8), Unit: "mm"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(ids["bottom"], "START")}, FixedPoint: &SketchPoint2{}}))
	f.edit(ops...)
	return ids, widthID
}

func TestSketchWorkflowChamferModesDimensionsAndExtrude(t *testing.T) {
	for _, mode := range []string{"EQUAL", "TWO_LENGTHS", "LENGTH_ANGLE"} {
		t.Run(mode, func(t *testing.T) {
			f := newSketchWorkflowFixture(t)
			ids, widthID := workflowParametricRectangle(f)
			op := cornerOperation("CHAMFER_ENTITIES", uuid.NewString(), ids["bottom"], ids["left"], "START", "END")
			op.ChamferMode = mode
			op.ChamferSecond = 2
			op.ChamferAngle = 30
			f.edit(op)
			second := 1.0
			if mode == "TWO_LENGTHS" {
				second = 2
			}
			if mode == "LENGTH_ANGLE" {
				second = math.Tan(math.Pi / 6)
			}
			f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 160-second)
			secondAfter := second
			if mode != "TWO_LENGTHS" {
				secondAfter *= 2
			}
			f.volume(f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: macroID(op.OperationID, "corner/first-length"), Value: workflowValue(2)}), 160-2*secondAfter)
			f.volume(f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: widthID, Value: workflowValue(12)}), 192-2*secondAfter)
			f.volume(f.command(CommandRequest{Type: "UNDO"}), 160-2*secondAfter)
		})
	}
}

func TestSketchWorkflowLineArcAndArcArcFilletsExtrudeUpdate(t *testing.T) {
	for _, pair := range []string{"LINE_ARC", "ARC_ARC"} {
		t.Run(pair, func(t *testing.T) {
			f := newSketchWorkflowFixture(t)
			a, b := uuid.NewString(), uuid.NewString()
			alpha := math.Acos(.6)
			first := SketchEntity{ID: a, Kind: "LINE", Start: &SketchPoint2{X: -5}, End: &SketchPoint2{X: 5}, Role: "PROFILE"}
			second := SketchEntity{ID: b, Kind: "ARC", Center: &SketchPoint2{}, Radius: 5, StartAngle: 0, EndAngle: math.Pi, Role: "PROFILE"}
			radius := 1.0
			if pair == "ARC_ARC" {
				first = SketchEntity{ID: a, Kind: "ARC", Center: &SketchPoint2{X: -3}, Radius: 5, StartAngle: -alpha, EndAngle: alpha, Role: "PROFILE"}
				second = SketchEntity{ID: b, Kind: "ARC", Center: &SketchPoint2{X: 3}, Radius: 5, StartAngle: math.Pi - alpha, EndAngle: math.Pi + alpha, Role: "PROFILE"}
				radius = .5
			}
			f.edit(workflowEntity(first), workflowEntity(second), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED", References: []SketchGeometryRef{workflowRef(a, "WHOLE")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED", References: []SketchGeometryRef{workflowRef(b, "WHOLE")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(a, "END"), workflowRef(b, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(b, "END"), workflowRef(a, "START")}}))
			op := cornerOperation("FILLET_ENTITIES", uuid.NewString(), a, b, "END", "START")
			op.Value = &radius
			f.edit(op)
			area := func(r float64) float64 {
				if pair == "LINE_ARC" {
					x := math.Sqrt(25 - 10*r)
					beta := math.Asin(r / (5 - r))
					return 12.5*(math.Pi-beta) + (r*x*(math.Sin(beta)+1)-r*r*math.Cos(beta)+r*r*(beta+math.Pi/2))/2
				}
				h := math.Sqrt((5-r)*(5-r) - 9)
				beta := math.Atan2(h, 3)
				return 25*(alpha+beta) - 15*(math.Sin(alpha)+math.Sin(beta)) + r*h*math.Cos(beta) + r*r*(math.Pi-2*beta)/2
			}
			before := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
			f.volume(before, 2*area(radius))
			newRadius := radius * 1.5
			change := SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: macroID(op.OperationID, "corner/radius"), Value: &newRadius}
			beforeJSON, _ := json.Marshal(before.Part)
			payloadJSON, _ := json.Marshal(editSketchPayload{SketchID: f.sketchID, Operations: []SketchOperation{change}})
			candidateJSON, _, err := applyEditSketch(beforeJSON, payloadJSON)
			if err != nil {
				t.Fatal(err)
			}
			var candidate PartModel
			_ = json.Unmarshal(candidateJSON, &candidate)
			if err = f.service.solveSketches(f.ctx, uuid.NewString(), &candidate); err != nil {
				t.Fatal(err)
			}
			for _, feature := range candidate.Features {
				if feature.Sketch != nil && feature.Sketch.Solve.Status == "CONFLICTING" {
					raw, _ := json.MarshalIndent(candidate, "", "  ")
					path := filepath.Join(filepath.Dir(os.Getenv("OCCCCAD_TEST_ARTIFACT_ROOT")), pair+"-corner-candidate.json")
					_ = os.WriteFile(path, raw, 0600)
					t.Fatalf("radius candidate solve %+v; reproduction %s", feature.Sketch.Solve, path)
				}
			}
			f.volume(f.edit(change), 2*area(newRadius))
			f.volume(f.command(CommandRequest{Type: "UNDO"}), 2*area(radius))
		})
	}
}

func TestSketchWorkflowArcComplementAndCloseExtrudeUpdate(t *testing.T) {
	for _, kind := range []string{"ARC", "ELLIPTICAL_ARC"} {
		t.Run(kind, func(t *testing.T) {
			f := newSketchWorkflowFixture(t)
			id, chord, dim := uuid.NewString(), uuid.NewString(), uuid.NewString()
			e := SketchEntity{ID: id, Kind: kind, Center: &SketchPoint2{}, Radius: 5, MajorRadius: 5, MinorRadius: 3, StartAngle: 0, EndAngle: math.Pi / 2, Role: "PROFILE"}
			minor, dimensionKind := 5.0, "RADIUS"
			if kind == "ELLIPTICAL_ARC" {
				minor, dimensionKind = 3, "MAJOR_RADIUS"
			}
			f.edit(workflowEntity(e), workflowConstraint(SketchConstraint{ID: dim, Kind: dimensionKind, Unit: "mm", References: []SketchGeometryRef{workflowRef(id, "WHOLE")}, Value: workflowValue(5)}))
			if kind == "ELLIPTICAL_ARC" {
				f.edit(workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "MINOR_RADIUS", Unit: "mm", Value: workflowValue(3), References: []SketchGeometryRef{workflowRef(id, "WHOLE")}}))
			}
			complement := f.edit(SketchOperation{Type: "ARC_COMPLEMENT", OperationID: uuid.NewString(), EntityIDs: []string{id}})
			arc := f.sketch(complement).Entities[0]
			if math.Abs(arc.StartAngle-math.Pi/2) > 1e-9 || math.Abs(arc.EndAngle-2*math.Pi) > 1e-9 {
				t.Fatalf("complement lost native interval: %+v", arc)
			}
			f.edit(workflowEntity(SketchEntity{ID: chord, Kind: "LINE", Start: &SketchPoint2{Y: minor}, End: &SketchPoint2{X: 5}, Role: "PROFILE"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(id, "START"), workflowRef(chord, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(id, "END"), workflowRef(chord, "END")}}))
			f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 2*5*minor*(3*math.Pi/4+.5))
			f.volume(f.edit(SketchOperation{Type: "DELETE_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{chord}}, SketchOperation{Type: "CLOSE_CURVE", OperationID: uuid.NewString(), EntityIDs: []string{id}}), 2*math.Pi*5*minor)
			if kind == "ARC" {
				minor = 7
			}
			f.volume(f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: dim, Value: workflowValue(7)}), 2*math.Pi*7*minor)
		})
	}
}

func TestSketchWorkflowFreeExtensionPlacementHistory(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	source := SketchEntity{ID: uuid.NewString(), Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 3, Y: 7}, End: &SketchPoint2{X: 8, Y: 7}}
	f.edit(workflowEntity(source))
	ref := workflowRef(source.ID, "END")
	view := f.edit(SketchOperation{Type: "EXTEND_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{source.ID}, FirstReference: &ref, Point: &SketchPoint2{X: 13, Y: 9}})
	check := func(view DocumentView, x float64) {
		f.t.Helper()
		sketch := f.sketch(view)
		for _, e := range sketch.Entities {
			if e.Kind == "LINE" {
				if math.Abs(e.End.X-x) > 1e-8 || math.Abs(e.End.Y-7) > 1e-8 {
					t.Fatalf("wrong analytic endpoint %+v", e.End)
				}
				return
			}
		}
		t.Fatal("extended line missing")
	}
	check(view, 13)
	check(f.command(CommandRequest{Type: "UNDO"}), 8)
	check(f.command(CommandRequest{Type: "REDO"}), 13)
	cold, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	check(cold, 13)
}

func TestSketchWorkflowSplitExtendedArcContact(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	const rotation = 0.37
	p := func(x, y float64) *SketchPoint2 {
		return &SketchPoint2{X: 12 + x*math.Cos(rotation) - y*math.Sin(rotation), Y: 19 + x*math.Sin(rotation) + y*math.Cos(rotation)}
	}
	line := SketchEntity{ID: uuid.NewString(), Kind: "LINE", Role: "PROFILE", Start: p(-10.123, 0), End: p(20.1726, 0)}
	arc := SketchEntity{ID: uuid.NewString(), Kind: "ARC", Role: "PROFILE", Center: p(0, -5), Radius: 5, StartAngle: rotation, EndAngle: rotation + math.Pi/4}
	f.edit(workflowEntity(line), workflowEntity(arc), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(line.ID, "START")}, FixedPoint: line.Start}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(line.ID, "END")}, FixedPoint: line.End}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(arc.ID, "CENTER")}, FixedPoint: arc.Center}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "RADIUS", References: []SketchGeometryRef{workflowRef(arc.ID, "WHOLE")}, Value: workflowValue(5), Unit: "mm"}))
	end := workflowRef(arc.ID, "END")
	view := f.edit(SketchOperation{Type: "EXTEND_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{arc.ID}, FirstReference: &end, BoundaryIDs: []string{line.ID}, Point: p(0, 0)})
	for _, e := range f.sketch(view).Entities {
		if e.ID == arc.ID {
			end.PointID = e.EndPointID
		}
	}
	view = f.edit(SketchOperation{Type: "SPLIT_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{line.ID}, Parameters: []float64{10.123 / 30.2956}, FirstReference: &end})
	identities := map[string][2]string{}
	for _, e := range f.sketch(view).Entities {
		identities[e.ID] = [2]string{e.StartPointID, e.EndPointID}
	}
	check := func(v DocumentView) {
		t.Helper()
		s := f.sketch(v)
		length := 0.0
		segments := 0
		contact := p(0, 0)
		joined := false
		for _, e := range s.Entities {
			if expected, ok := identities[e.ID]; !ok || expected != [2]string{e.StartPointID, e.EndPointID} {
				t.Fatalf("unstable split identity: %+v", e)
			}
			if e.ID == arc.ID && (math.Abs(e.Radius-5) > 1e-8 || math.Abs(e.EndAngle-(rotation+math.Pi/2)) > 1e-8) {
				t.Fatalf("extension/split changed circle support: %+v", e)
			}
			if e.Kind == "LINE" {
				segments++
				length += math.Hypot(e.End.X-e.Start.X, e.End.Y-e.Start.Y)
				if math.Min(math.Hypot(e.End.X-contact.X, e.End.Y-contact.Y), math.Hypot(e.Start.X-contact.X, e.Start.Y-contact.Y)) > 1e-7 {
					t.Fatalf("split misses exact contact: %+v", e)
				}
			}
		}
		if segments != 2 || math.Abs(length-30.2956) > 1e-7 {
			t.Fatalf("split altered source shape: %d %g", segments, length)
		}
		for _, c := range s.Constraints {
			if c.Kind == "COINCIDENT" {
				for _, r := range c.References {
					if r.EntityID == arc.ID && r.SubElement == "END" {
						joined = true
					}
				}
			}
		}
		if !joined {
			t.Fatal("contact lacks formal endpoint connection")
		}
	}
	check(view)
	f.command(CommandRequest{Type: "UNDO"})
	check(f.command(CommandRequest{Type: "REDO"}))
	cold, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	check(cold)
}

func TestSketchWorkflowSplitOnObjectPoint(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	line := SketchEntity{ID: uuid.NewString(), Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 2, Y: 3}, End: &SketchPoint2{X: 17, Y: 9}}
	point := SketchEntity{ID: uuid.NewString(), Kind: "POINT", Point: &SketchPoint2{X: 7, Y: 5}, Role: "CONSTRUCTION"}
	outside := SketchEntity{ID: uuid.NewString(), Kind: "POINT", Point: &SketchPoint2{X: 7, Y: 5.5}, Role: "CONSTRUCTION"}
	before := f.edit(workflowEntity(line), workflowEntity(point), workflowEntity(outside))
	bad := workflowRef(outside.ID, "POINT")
	_, err := f.service.ApplyCommand(f.ctx, f.documentID, CommandRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: "EDIT_SKETCH", SketchID: f.sketchID, Operations: []SketchOperation{{Type: "SPLIT_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{line.ID}, Parameters: []float64{1.0 / 3}, FirstReference: &bad}}})
	if err == nil {
		t.Fatal("off-curve point must not be accepted")
	}
	after, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if after.Document.VersionID != before.Document.VersionID {
		t.Fatal("failed snap changed the revision")
	}
	ref := workflowRef(point.ID, "POINT")
	view := f.edit(SketchOperation{Type: "SPLIT_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{line.ID}, Parameters: []float64{1.0 / 3}, FirstReference: &ref})
	joined := false
	for _, c := range f.sketch(view).Constraints {
		if c.Kind == "COINCIDENT" {
			for _, r := range c.References {
				if r.EntityID == point.ID && r.SubElement == "POINT" {
					joined = true
				}
			}
		}
	}
	if !joined {
		t.Fatal("split lacks explicit point connection")
	}
	length := 0.0
	for _, e := range f.sketch(view).Entities {
		if e.Kind == "LINE" {
			length += math.Hypot(e.End.X-e.Start.X, e.End.Y-e.Start.Y)
			if math.Min(math.Hypot(e.Start.X-7, e.Start.Y-5), math.Hypot(e.End.X-7, e.End.Y-5)) > 1e-8 {
				t.Fatal("point snap changed the requested cut")
			}
		}
	}
	if math.Abs(length-math.Hypot(15, 6)) > 1e-8 {
		t.Fatal("point split changed source shape")
	}
}

func TestSketchWorkflowSplitCirclePointReferences(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	circle := SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 5}
	a := SketchEntity{ID: uuid.NewString(), Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 5 * math.Cos(.2*math.Pi), Y: 5 * math.Sin(.2*math.Pi)}}
	b := SketchEntity{ID: uuid.NewString(), Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 0, Y: -5}}
	f.edit(workflowEntity(circle), workflowEntity(a), workflowEntity(b))
	ar, br := workflowRef(a.ID, "POINT"), workflowRef(b.ID, "POINT")
	view := f.edit(SketchOperation{Type: "SPLIT_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{circle.ID}, Parameters: []float64{.1, .75}, FirstReference: &ar, SecondReference: &br})
	sweep := 0.0
	joins := map[string]bool{}
	for _, e := range f.sketch(view).Entities {
		if e.Kind == "ARC" {
			sweep += math.Abs(e.EndAngle - e.StartAngle)
			if math.Abs(e.Radius-5) > 1e-8 || math.Hypot(e.Center.X, e.Center.Y) > 1e-8 {
				t.Fatal("split lost original circle support")
			}
		}
	}
	if math.Abs(sweep-2*math.Pi) > 1e-8 {
		t.Fatal("periodic split lost original curve range")
	}
	for _, c := range f.sketch(view).Constraints {
		if c.Kind == "COINCIDENT" {
			for _, r := range c.References {
				if r.EntityID == a.ID || r.EntityID == b.ID {
					joins[r.EntityID] = true
				}
			}
		}
	}
	if !joins[a.ID] || !joins[b.ID] {
		t.Fatal("two-point split dropped a snapped reference")
	}
	f.volume(f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2}), 50*math.Pi)
}
