//go:build integration

package control

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
)

// A serial, small-scene measurement through the real Router/Worker. Inclusive
// RPC time is part of wall time; neither is a kernel-only timing or a P95.
func TestSpatialAnalysisPerformanceThroughRouter(t *testing.T) {
	type rpcSample struct {
		Method  string  `json:"method"`
		Ms      float64 `json:"ms"`
		Bodies  int     `json:"bodies,omitempty"`
		Pairs   int     `json:"pairs,omitempty"`
		Details bool    `json:"details,omitempty"`
	}
	var mu sync.Mutex
	var calls []rpcSample
	measure := false
	interceptor := grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		began := time.Now()
		err := invoke(ctx, method, req, reply, cc, options...)
		sample := rpcSample{Method: method[strings.LastIndex(method, "/")+1:], Ms: float64(time.Since(began).Microseconds()) / 1000}
		switch r := req.(type) {
		case *workerv1.AnalyzeInterferenceRequest:
			sample.Bodies = len(r.Geometry)
			sample.Pairs = len(r.Pairs)
			sample.Details = r.Details
		case *workerv1.AnalyzeSpatialGeometryRequest:
			sample.Bodies = len(r.Geometry)
		default:
			return err
		}
		mu.Lock()
		if measure {
			calls = append(calls, sample)
		}
		mu.Unlock()
		return err
	})
	f := newCompositionControlFixture(t, interceptor)
	ctx := t.Context()
	s := f.service
	create := func(kind string) workspace.DocumentView {
		v, e := s.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: f.actor, Type: kind, Name: "Spatial performance"})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	seq := 0
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.VersionID = v.Document.VersionID
		r.RequestID = fmt.Sprintf("%s-perf-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(ctx, v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = n
	}
	part := create("PART")
	apply(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sid := part.Part.Features[0].ID
	apply(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sid, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -5, Y: -5}, Second: &workspace.SketchPoint2{X: 5, Y: 5}}}})
	apply(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sid, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	product := create("PRODUCT")
	for range 12 {
		apply(&product, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	ids := []string{}
	for i, v := range product.Product.Instances {
		ids = append(ids, v.ID)
		pos := [3]float64{float64(i * 25), 0, 0}
		if i == 1 {
			pos = [3]float64{0, 0, 20}
		}
		apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: v.ID, Translation: pos, Rotation: [4]float64{0, 0, 0, 1}})
	}
	record := func(label string, n int, run func(i int)) {
		t.Helper()
		wall := []float64{}
		mu.Lock()
		calls = nil
		measure = true
		mu.Unlock()
		for i := 0; i < n; i++ {
			began := time.Now()
			run(i)
			wall = append(wall, float64(time.Since(began).Microseconds())/1000)
		}
		mu.Lock()
		measure = false
		data, _ := json.Marshal(struct {
			Label string      `json:"label"`
			Wall  []float64   `json:"wall_ms"`
			RPC   []rpcSample `json:"rpc"`
		}{label, wall, calls})
		mu.Unlock()
		t.Log("SPATIAL_PERFORMANCE", string(data))
	}
	before := product.Document.VersionID
	request := workspace.StaticInterferenceRequest{RevisionID: before, Scope: "ALL", ThresholdOnly: true}
	record("static-threshold-12", 6, func(int) {
		r, e := s.AnalyzeStaticInterference(ctx, product.Document.ID, request)
		if e != nil || !r.Complete || len(r.Pairs) != 66 {
			t.Fatal(e, r)
		}
	})
	d := workspace.SpatialAnalysis{Kind: "SECTION", Scope: "ALL", SectionType: "PLANE", Frame: workspace.InstancePose{Translation: [3]float64{0, 0, 5}, Rotation: [4]float64{0, 0, 0, 1}}, Size: [3]float64{400, 100, 2}, AccuracyMM: 2, CutMode: "NONE"}
	for _, kind := range []string{"SECTION", "SLICE", "BOX", "DISTANCE"} {
		d.Kind = "SECTION"
		d.SectionType = kind
		if kind == "SECTION" {
			d.SectionType = "PLANE"
		}
		if kind == "DISTANCE" {
			d.Kind = kind
			d.SectionType = "PLANE"
		}
		definition := d
		record("spatial-"+kind+"-12", 6, func(int) {
			r, e := s.AnalyzeSpatial(ctx, product.Document.ID, workspace.SpatialAnalysisRequest{RevisionID: before, Definition: definition})
			if e != nil || !r.Complete {
				t.Fatal(e, r)
			}
		})
	}
	resolved, e := s.AnalyzeStaticInterference(ctx, product.Document.ID, request)
	if e != nil {
		t.Fatal(e)
	}
	paths := map[string]workspace.InstancePath{}
	for _, b := range resolved.Bodies {
		paths[b.InstancePath.Canonical] = b.InstancePath
	}
	band := d
	band.Kind = "BAND"
	band.Scope = "BETWEEN"
	band.Selected = []workspace.InstancePath{paths[ids[0]]}
	band.SecondSelection = []workspace.InstancePath{paths[ids[2]]}
	band.MinimumMM = 0
	band.MaximumMM = 50
	band.AccuracyMM = 2
	record("spatial-BAND-2", 6, func(int) {
		r, e := s.AnalyzeSpatial(ctx, product.Document.ID, workspace.SpatialAnalysisRequest{RevisionID: before, Definition: band})
		if e != nil || !r.Complete {
			t.Fatal(e, r)
		}
	})
	end := func(id string) workspace.JointEndpoint {
		return workspace.JointEndpoint{InstanceID: id, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, CapturedX: &[3]float64{1, 0, 0}, Axis: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}, Plane: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-yz"}}
	}
	second := end(ids[1])
	m := workspace.Mechanism{ID: "perf-mechanism", Name: "Performance slider", UnitIDs: ids[:2], Joints: []workspace.MechanismJoint{{ID: "perf-ground", Name: "Ground", Kind: "GROUND", First: end(ids[0])}, {ID: "perf-slider", Name: "Slider", Kind: "PRISMATIC", First: end(ids[0]), Second: &second, OrientationAngle: workspace.MotionQuantity{Unit: "deg"}, Coordinates: []workspace.JointCoordinate{{ID: "perf-coordinate", Role: "AXIAL_TRANSLATION", Dimension: "LENGTH", Zero: workspace.MotionQuantity{Unit: "mm"}, Direction: 1, Lower: &workspace.MotionQuantity{Value: -50, Unit: "mm"}, Upper: &workspace.MotionQuantity{Value: 50, Unit: "mm"}}}}}}
	k := workspace.KinematicsDefinitions{Mechanisms: []workspace.Mechanism{m}, Drivers: []workspace.MotionDriver{{ID: "perf-driver", Name: "Travel", MechanismID: m.ID, JointID: "perf-slider", CoordinateID: "perf-coordinate"}}}
	apply(&product, workspace.CommandRequest{Type: "SAVE_KINEMATICS", Kinematics: &k})
	model := product.Product
	history, e := s.ListHistory(ctx, product.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	opened, e := s.BeginMechanismSession(ctx, product.Document.ID, f.actor, workspace.MechanismSessionBegin{OwnerID: "spatial-performance", Generation: 1, BaseRevisionID: product.Document.VersionID, MechanismID: m.ID, ActiveDriverIDs: []string{"perf-driver"}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.CancelMechanismSession(product.Document.ID, f.actor, opened.SessionID)
	var sequence uint64
	update := func(r workspace.MechanismSessionUpdate) workspace.MechanismSessionFrame {
		t.Helper()
		sequence++
		r.Sequence = sequence
		v, e := s.UpdateMechanismSession(ctx, product.Document.ID, f.actor, opened.SessionID, r)
		if e != nil || !v.Accepted {
			t.Fatal(e, v)
		}
		return v
	}
	update(workspace.MechanismSessionUpdate{Clash: &workspace.MechanismClashPolicy{Mode: "PROMPT"}})
	record("motion-prompt-changing-12", 6, func(i int) {
		v := update(workspace.MechanismSessionUpdate{Values: map[string]workspace.MotionQuantity{"perf-driver": {Value: float64(-12 - i), Unit: "mm"}}})
		if v.Clash == nil || !v.Clash.Report.Complete {
			t.Fatal(v)
		}
	})
	record("motion-prompt-same-12", 6, func(int) {
		v := update(workspace.MechanismSessionUpdate{Clash: &workspace.MechanismClashPolicy{Mode: "PROMPT"}})
		if v.Clash == nil || !v.Clash.Report.Complete {
			t.Fatal(v)
		}
	})
	update(workspace.MechanismSessionUpdate{Clash: &workspace.MechanismClashPolicy{Mode: "OFF"}})
	update(workspace.MechanismSessionUpdate{Values: map[string]workspace.MotionQuantity{"perf-driver": {Value: 0, Unit: "mm"}}})
	update(workspace.MechanismSessionUpdate{Clash: &workspace.MechanismClashPolicy{Mode: "STOP"}})
	record("motion-stop-safe-12", 6, func(i int) {
		v := update(workspace.MechanismSessionUpdate{Values: map[string]workspace.MotionQuantity{"perf-driver": {Value: float64(i + 1), Unit: "mm"}}})
		if v.Clash == nil || !v.Clash.Report.Complete {
			t.Fatal(v)
		}
	})
	current, e := s.GetDocument(ctx, product.Document.ID)
	if e != nil || current.Document.VersionID != product.Document.VersionID || !reflect.DeepEqual(current.Product, model) {
		t.Fatal("analysis changed model", e)
	}
	after, e := s.ListHistory(ctx, product.Document.ID)
	if e != nil || !reflect.DeepEqual(history, after) {
		t.Fatal("analysis changed history", e)
	}
	// Several simultaneous contacts/overlaps must each use the Worker single-pair
	// detail contract; a failed multi-pair detail RPC cannot masquerade as display.
	s.CancelMechanismSession(product.Document.ID, f.actor, opened.SessionID)
	apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: ids[2], Translation: [3]float64{0, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	opened, e = s.BeginMechanismSession(ctx, product.Document.ID, f.actor, workspace.MechanismSessionBegin{OwnerID: "spatial-performance-multiple", Generation: 1, BaseRevisionID: product.Document.VersionID, MechanismID: m.ID, ActiveDriverIDs: []string{"perf-driver"}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.CancelMechanismSession(product.Document.ID, f.actor, opened.SessionID)
	sequence = 0
	update(workspace.MechanismSessionUpdate{Clash: &workspace.MechanismClashPolicy{Mode: "PROMPT"}})
	record("motion-multiple-overlap-12", 1, func(int) {
		v := update(workspace.MechanismSessionUpdate{Values: map[string]workspace.MotionQuantity{"perf-driver": {Value: -20, Unit: "mm"}}})
		overlaps := 0
		for _, p := range v.Clash.Report.Pairs {
			if p.Classification == "PENETRATION" || p.Classification == "CONTAINMENT" {
				overlaps++
				if p.DetailStatus != "COMPLETE" || len(p.OverlapTriangles) == 0 {
					t.Fatal("missing overlap graphics", p)
				}
			}
		}
		if overlaps != 3 {
			t.Fatal("triple overlap", overlaps)
		}
	})
	mu.Lock()
	observed := append([]rpcSample{}, calls...)
	mu.Unlock()
	for _, r := range observed {
		if r.Details && (r.Bodies != 2 || r.Pairs != 1) {
			t.Fatal("detail input contract", r)
		}
	}

}
