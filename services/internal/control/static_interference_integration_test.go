//go:build integration

package control

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAcceptedStaticInterferenceGeometryAndScopeThroughRouter(t *testing.T) {
	s, _, db, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	seq := 0
	create := func(typ, name string) workspace.DocumentView {
		v, e := s.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: typ, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-static-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(ctx, v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = n
	}
	cube := func(size float64) workspace.DocumentView {
		v := create("PART", "Static cube")
		run(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := v.Part.Features[0].ID
		run(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -size / 2, Y: -size / 2}, Second: &workspace.SketchPoint2{X: size / 2, Y: size / 2}}}})
		run(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: size})
		return v
	}
	part := cube(10)
	root := create("PRODUCT", "Accepted static assembly")
	for i := 0; i < 2; i++ {
		run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	second := root.Product.Instances[1].ID
	analyze := func(clearance float64) workspace.StaticInterferenceReport {
		t.Helper()
		r, e := s.AnalyzeStaticInterference(ctx, root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: root.Document.VersionID, Scope: "ALL", ClearanceMM: clearance})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	for _, tc := range []struct {
		x, gap, distance, volume float64
		kind                     string
		pass                     bool
	}{{8, 0, 0, 200, "PENETRATION", false}, {10, 0, 0, 0, "CONTACT", true}, {12, 3, 2, 0, "SEPARATED", false}, {12, 1, 2, 0, "SEPARATED", true}} {
		run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{tc.x, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
		r := analyze(tc.gap)
		if !r.Complete || len(r.Pairs) != 1 || r.Pairs[0].Classification != tc.kind || r.Pairs[0].ClearanceSatisfied != tc.pass || math.Abs(r.Pairs[0].DistanceMM-tc.distance) > 1e-6 || math.Abs(r.Pairs[0].CommonVolumeMM3-tc.volume) > 1e-5 {
			t.Fatal("exact classification", tc, r)
		}
		if r.Bodies[0].ID == r.Bodies[1].ID || r.Bodies[0].GeometryKey != r.Bodies[1].GeometryKey {
			t.Fatal("shared Part instances merged or fixture failed")
		}
	}
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{13, 0, 0}, Rotation: [4]float64{0, 0, math.Sin(math.Pi / 8), math.Cos(math.Pi / 8)}})
	r := analyze(1)
	if math.Abs(r.Pairs[0].DistanceMM-(13-5-5*math.Sqrt2)) > 1e-6 {
		t.Fatal("rotated exact distance", r.Pairs)
	}
	// Persisted source edges stay at the old accepted revision despite a new Head.
	run(&root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{0, 0, 15}, Rotation: [4]float64{0, 0, 0, 1}})
	for _, p := range part.Part.Parameters {
		if p.PropertySlot == "length" {
			run(&part, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: p.ParameterID, Value: 20, Unit: "mm"})
			break
		}
	}
	before := root
	var rawBefore []byte
	if e := db.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, root.Document.VersionID).Scan(&rawBefore); e != nil {
		t.Fatal(e)
	}
	history, e := s.ListHistory(ctx, root.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	r = analyze(0)
	if math.Abs(r.Pairs[0].DistanceMM-5) > 1e-6 {
		t.Fatal("analysis read new source Head", r.Pairs)
	}
	var head string
	if e = db.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, root.Document.ID).Scan(&head); e != nil || head != before.Document.VersionID {
		t.Fatal("analysis advanced Product", e)
	}
	afterHistory, e := s.ListHistory(ctx, root.Document.ID)
	if e != nil || !reflect.DeepEqual(history, afterHistory) {
		t.Fatal("analysis wrote history", e)
	}
	var raw []byte
	if e = db.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, head).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var saved workspace.ProductModel
	if e = json.Unmarshal(raw, &saved); e != nil || string(raw) != string(rawBefore) {
		t.Fatal("analysis changed poses/references", e)
	}
	selected := []workspace.InstancePath{r.Bodies[0].InstancePath, r.Bodies[1].InstancePath}
	if out, e := s.AnalyzeStaticInterference(ctx, root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: head, Scope: "SELECTED", Selected: selected}); e != nil || !out.Complete {
		t.Fatal("selected exact leaves", out, e)
	}
	bad := append([]workspace.InstancePath(nil), selected...)
	bad[0].Segments = append([]workspace.InstancePathSegment(nil), bad[0].Segments...)
	bad[0].Segments[0].ResolvedVersionID = part.Document.VersionID
	if _, e = s.AnalyzeStaticInterference(ctx, root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: head, Scope: "SELECTED", Selected: bad}); e == nil {
		t.Fatal("forged accepted path passed")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.AnalyzeStaticInterference(cancelled, root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: head, Scope: "ALL"}); e == nil {
		t.Fatal("cancelled analysis passed")
	}
	// A new Router/Worker has no seed geometry handles. BREP is sufficient.
	cold, _, _, _, _ := geometryRuntimeTestService(t, testsupport.OpenDevelopmentDatabase(t))
	rebuilt, e := cold.AnalyzeStaticInterference(ctx, root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: head, Scope: "ALL"})
	if e != nil || rebuilt.InputDigest != r.InputDigest || !reflect.DeepEqual(rebuilt.Pairs, r.Pairs) {
		t.Fatal("cold Worker BREP restoration", e, rebuilt.Pairs, r.Pairs)
	}
	// Containment uses another closed solid, not a surface/box approximation.
	small := cube(4)
	contained := create("PRODUCT", "Containment")
	run(&contained, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	run(&contained, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: small.Document.ID})
	run(&contained, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: contained.Product.Instances[1].ID, Translation: [3]float64{0, 0, 3}, Rotation: [4]float64{0, 0, 0, 1}})
	containedResult, e := s.AnalyzeStaticInterference(ctx, contained.Document.ID, workspace.StaticInterferenceRequest{RevisionID: contained.Document.VersionID, Scope: "ALL"})
	if e != nil || !containedResult.Complete || containedResult.Pairs[0].Classification != "CONTAINMENT" || math.Abs(containedResult.Pairs[0].CommonVolumeMM3-64) > 1e-5 {
		t.Fatal("containment", containedResult, e)
	}
	// The same subassembly contains two leaf Parts; they must still be checked.
	module := create("PRODUCT", "Module")
	for i := 0; i < 2; i++ {
		run(&module, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: small.Document.ID})
	}
	run(&module, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: module.Product.Instances[1].ID, Translation: [3]float64{6, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	nested := create("PRODUCT", "Nested")
	run(&nested, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: module.Document.ID})
	run(&nested, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: nested.Product.Instances[0].ID, Translation: [3]float64{100, 20, 5}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}})
	n, e := s.AnalyzeStaticInterference(ctx, nested.Document.ID, workspace.StaticInterferenceRequest{RevisionID: nested.Document.VersionID, Scope: "ALL"})
	if e != nil || !n.Complete || len(n.Pairs) != 1 || len(n.Bodies[0].InstancePath.Segments) != 2 || math.Abs(n.Pairs[0].DistanceMM-2) > 1e-6 {
		t.Fatal("nested same-module leaf pair", n, e)
	}
	// Different Bodies in one Part are not paired internally, but each is
	// checked against the other leaf. Hidden material remains in scope.
	multi := cube(4)
	firstBody := multi.Part.ActiveBodyID
	run(&multi, workspace.CommandRequest{Type: "CREATE_BODY"})
	toolBody := multi.Part.ActiveBodyID
	run(&multi, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := multi.Part.Features[len(multi.Part.Features)-1].ID
	run(&multi, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 28, Y: -2}, Second: &workspace.SketchPoint2{X: 32, Y: 2}}}})
	run(&multi, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 4})
	tool := multi.Part.Features[len(multi.Part.Features)-1]
	hidden := false
	run(&multi, workspace.CommandRequest{Type: "SET_BODY_VISIBILITY", BodyID: toolBody, Visible: hidden})
	assembly := create("PRODUCT", "Multiple Bodies")
	run(&assembly, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: multi.Document.ID})
	run(&assembly, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: small.Document.ID})
	m, e := s.AnalyzeStaticInterference(ctx, assembly.Document.ID, workspace.StaticInterferenceRequest{RevisionID: assembly.Document.VersionID, Scope: "ALL"})
	if e != nil || !m.Complete || len(m.Bodies) != 3 || len(m.Pairs) != 2 {
		t.Fatal("multi Body/hidden scope", m, e)
	}
	run(&multi, workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: firstBody, Operation: "ADD", KeepTools: false, Tools: []workspace.FeatureStageRef{{BodyID: toolBody, FeatureID: tool.ID}}})
	consumed := create("PRODUCT", "Consumed Body")
	run(&consumed, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: multi.Document.ID})
	run(&consumed, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: small.Document.ID})
	m, e = s.AnalyzeStaticInterference(ctx, consumed.Document.ID, workspace.StaticInterferenceRequest{RevisionID: consumed.Document.VersionID, Scope: "ALL"})
	if e != nil || !m.Complete || len(m.Bodies) != 2 || len(m.Pairs) != 1 || m.ConsumedBodies != 1 {
		t.Fatal("consumed Body not excluded", m, e)
	}
	// Budget is checked before RPC. Existing INSERT_INSTANCES retains stable slots.
	crowded := create("PRODUCT", "Crowded")
	run(&crowded, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: small.Document.ID})
	run(&crowded, workspace.CommandRequest{Type: "INSERT_INSTANCES", InstanceID: crowded.Product.Instances[0].ID, PatternCount: 128, PatternAxis: "X", PatternSpacing: 10})
	run(&crowded, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: small.Document.ID})
	if _, e = s.AnalyzeStaticInterference(ctx, crowded.Document.ID, workspace.StaticInterferenceRequest{RevisionID: crowded.Document.VersionID, Scope: "ALL"}); e == nil || !strings.Contains(e.Error(), "STATIC_INTERFERENCE_LIMIT") {
		t.Fatal("body budget not prechecked", e)
	}
}

func TestAcceptedStaticInterferenceContextAndMissingInputsThroughRouter(t *testing.T) {
	c := newContextInsertionScene(t, 24)
	r, e := c.s.AnalyzeStaticInterference(t.Context(), c.root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: c.root.Document.VersionID, Scope: "ALL"})
	if e != nil || !r.Complete || len(r.Bodies) != 2 || r.LayoutParts != 1 {
		t.Fatal("accepted context/layout", r, e)
	}
	for _, b := range r.Bodies {
		if b.VariantKey == "" {
			t.Fatal("variant identity missing")
		}
		matched := false
		for _, v := range c.root.ContextVariants {
			if v.OwningInstancePath.Canonical == b.InstancePath.Canonical {
				matched = v.Part.Bodies[0].GeometryKey == b.GeometryKey
			}
		}
		if !matched {
			t.Fatal("default geometry substituted for accepted projection")
		}
	}
	var rootRaw []byte
	if e = c.db.QueryRow(t.Context(), `SELECT model_json FROM occccad.document_versions WHERE id=$1`, c.root.Document.VersionID).Scan(&rootRaw); e != nil {
		t.Fatal(e)
	}
	var broken workspace.ProductModel
	if e = json.Unmarshal(rootRaw, &broken); e != nil {
		t.Fatal(e)
	}
	broken.ContextBindings[0].Accepted.Status = "BROKEN"
	brokenRaw, _ := json.Marshal(broken)
	if _, e = c.db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$1 WHERE id=$2`, brokenRaw, c.root.Document.VersionID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		c.db.Exec(context.Background(), `UPDATE occccad.document_versions SET model_json=$1 WHERE id=$2`, rootRaw, c.root.Document.VersionID)
	})
	unavailable, e := c.s.AnalyzeStaticInterference(t.Context(), c.root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: c.root.Document.VersionID, Scope: "ALL"})
	if e != nil || unavailable.Complete || len(unavailable.Bodies) != 2 || len(unavailable.Pairs) != 1 || unavailable.Pairs[0].Complete || len(unavailable.Issues) == 0 {
		t.Fatal("failed Variant borrowed default material", unavailable, e)
	}
	if _, e = c.db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$1 WHERE id=$2`, rootRaw, c.root.Document.VersionID); e != nil {
		t.Fatal(e)
	}
	// Inject missing exact input into this test-owned immutable revision. It is
	// restored on cleanup; fallback is deliberately valid and must not be used.
	x := newInsertionScene(t)
	root, e := x.s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "Missing static input"})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		root, e = x.s.ApplyCommand(t.Context(), root.Document.ID, x.request(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: x.v.Document.ID}))
		if e != nil {
			t.Fatal(e)
		}
	}
	var original []byte
	if e = x.db.QueryRow(t.Context(), `SELECT model_json FROM occccad.document_versions WHERE id=$1`, x.v.Document.VersionID).Scan(&original); e != nil {
		t.Fatal(e)
	}
	var failed workspace.PartModel
	if e = json.Unmarshal(original, &failed); e != nil {
		t.Fatal(e)
	}
	failed.Bodies[0].DisplayFallback = &workspace.BodyDisplayFallback{GeometryKey: failed.Bodies[0].GeometryKey, SourceVersionID: x.v.Document.VersionID}
	failed.Bodies[0].GeometryKey = "missing-static-brep"
	next, _ := json.Marshal(failed)
	if _, e = x.db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$1 WHERE id=$2`, next, x.v.Document.VersionID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		x.db.Exec(context.Background(), `UPDATE occccad.document_versions SET model_json=$1 WHERE id=$2`, original, x.v.Document.VersionID)
	})
	missing, e := x.s.AnalyzeStaticInterference(t.Context(), root.Document.ID, workspace.StaticInterferenceRequest{RevisionID: root.Document.VersionID, Scope: "ALL"})
	if e != nil || missing.Complete || len(missing.Bodies) != 2 || len(missing.Pairs) != 1 || missing.Pairs[0].Complete || len(missing.Issues) != 2 {
		t.Fatal("missing input was omitted or certified", missing, e)
	}
}

func TestClashScopesDetailsAndMechanismStopThroughRouter(t *testing.T) {
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
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-clash-%d", v.Document.ID, seq)
		n, e := s.ApplyCommand(ctx, v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = n
	}
	part := create("PART", "Clash moving cube")
	apply(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := part.Part.Features[0].ID
	apply(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -5, Y: -5}, Second: &workspace.SketchPoint2{X: 5, Y: 5}}}})
	apply(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	product := create("PRODUCT", "Clash scopes and runtime")
	for range 3 {
		apply(&product, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	ground, moving, environment := product.Product.Instances[0].ID, product.Product.Instances[1].ID, product.Product.Instances[2].ID
	apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: moving, Translation: [3]float64{0, 0, 20}, Rotation: [4]float64{0, 0, 0, 1}})
	apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: environment, Translation: [3]float64{0, 0, 35}, Rotation: [4]float64{0, 0, 0, 1}})
	all, e := s.AnalyzeStaticInterference(ctx, product.Document.ID, workspace.StaticInterferenceRequest{RevisionID: product.Document.VersionID, Scope: "ALL", ThresholdOnly: true})
	if e != nil || !all.Complete || len(all.Pairs) != 3 {
		t.Fatal(all, e)
	}
	for _, pair := range all.Pairs {
		if pair.DistanceExact || pair.WitnessKind != "NONE" {
			t.Fatal("broad-phase proof invented a measurement", pair)
		}
	}
	paths := map[string]workspace.InstancePath{}
	for _, b := range all.Bodies {
		paths[b.InstancePath.Segments[0].InstanceID] = b.InstancePath
	}
	var betweenDigest string
	for _, tc := range []struct {
		scope string
		a, b  []workspace.InstancePath
		count int
	}{{"SELECTED", []workspace.InstancePath{paths[ground], paths[moving]}, nil, 1}, {"AGAINST_ALL", []workspace.InstancePath{paths[moving]}, nil, 2}, {"BETWEEN", []workspace.InstancePath{paths[ground], paths[moving]}, []workspace.InstancePath{paths[moving], paths[environment]}, 3}} {
		r, err := s.AnalyzeStaticInterference(ctx, product.Document.ID, workspace.StaticInterferenceRequest{RevisionID: product.Document.VersionID, Scope: tc.scope, Selected: tc.a, SecondSelection: tc.b, ThresholdOnly: true})
		if err != nil || !r.Complete || len(r.Pairs) != tc.count {
			t.Fatal(tc, r, err)
		}
		if tc.scope == "BETWEEN" {
			betweenDigest = r.InputDigest
		}
	}
	oneSide, e := s.AnalyzeStaticInterference(ctx, product.Document.ID, workspace.StaticInterferenceRequest{RevisionID: product.Document.VersionID, Scope: "BETWEEN", Selected: []workspace.InstancePath{paths[ground]}, SecondSelection: []workspace.InstancePath{paths[moving], paths[environment]}, ThresholdOnly: true})
	if e != nil || len(oneSide.Pairs) != 2 || oneSide.InputDigest == betweenDigest {
		t.Fatal("same leaves with different pair scope reused analysis identity", oneSide, e)
	}

	detail, e := s.AnalyzeStaticInterference(ctx, product.Document.ID, workspace.StaticInterferenceRequest{RevisionID: product.Document.VersionID, Scope: "ALL", DetailPair: []string{all.Pairs[0].FirstID, all.Pairs[0].SecondID}})
	if e != nil || len(detail.Pairs) != 1 || !detail.Pairs[0].DistanceExact || detail.Pairs[0].WitnessKind != "DISTANCE" || detail.Pairs[0].DetailStatus != "COMPLETE" {
		t.Fatal("authoritative detail", detail, e)
	}
	end := func(id string) workspace.JointEndpoint {
		return workspace.JointEndpoint{InstanceID: id, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, CapturedX: &[3]float64{1, 0, 0}, Axis: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}, Plane: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-yz"}}
	}
	second := end(moving)
	mechanism := workspace.Mechanism{ID: "clash-mechanism", Name: "Clash slider", UnitIDs: []string{ground, moving}, Joints: []workspace.MechanismJoint{{ID: "clash-ground", Name: "Ground", Kind: "GROUND", First: end(ground)}, {ID: "clash-slider", Name: "Slider", Kind: "PRISMATIC", OrientationAngle: workspace.MotionQuantity{Unit: "deg"}, First: end(ground), Second: &second, Coordinates: []workspace.JointCoordinate{{ID: "clash-coordinate", Role: "AXIAL_TRANSLATION", Dimension: "LENGTH", Zero: workspace.MotionQuantity{Unit: "mm"}, Direction: 1, Lower: &workspace.MotionQuantity{Value: -50, Unit: "mm"}, Upper: &workspace.MotionQuantity{Value: 50, Unit: "mm"}}}}}}
	definitions := workspace.KinematicsDefinitions{Mechanisms: []workspace.Mechanism{mechanism}, Drivers: []workspace.MotionDriver{{ID: "clash-driver", Name: "Travel", MechanismID: mechanism.ID, JointID: "clash-slider", CoordinateID: "clash-coordinate"}}}
	apply(&product, workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: product.Document.VersionID, Kinematics: &definitions})
	var original []byte
	if e = db.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, product.Document.VersionID).Scan(&original); e != nil {
		t.Fatal(e)
	}
	history, e := s.ListHistory(ctx, product.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	opened, e := s.BeginMechanismSession(ctx, product.Document.ID, p6Actor, workspace.MechanismSessionBegin{OwnerID: "clash-owner", Generation: 1, BaseRevisionID: product.Document.VersionID, MechanismID: mechanism.ID, ActiveDriverIDs: []string{"clash-driver"}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.CancelMechanismSession(product.Document.ID, p6Actor, opened.SessionID)
	current, e := s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 1, Clash: &workspace.MechanismClashPolicy{Mode: "STOP"}})
	if e != nil || !current.Accepted || current.Clash == nil || !current.Clash.Report.Complete {
		t.Fatal("enable detection", current, e)
	}
	stopped, e := s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 2, Values: map[string]workspace.MotionQuantity{"clash-driver": {Value: -20, Unit: "mm"}}})
	if e != nil || stopped.Accepted || stopped.Clash == nil || !stopped.Clash.Stopped || !stopped.Frame.KinematicValid {
		t.Fatal("stop event", stopped, e)
	}
	z := stopped.Frame.UnitPoses[moving].Translation[2]
	if z < 10-1e-6 || z >= 20 {
		t.Fatal("did not retain the last safe physical checkpoint", z)
	}
	if stopped.Clash.Report.Source != "MECHANISM" || stopped.Clash.Report.FrameSequence != 2 {
		t.Fatal("unscoped collision evidence", stopped.Clash)
	}
	// The source changes, but the session must retain accepted exact BREP.
	for _, parameter := range part.Part.Parameters {
		if parameter.PropertySlot == "length" {
			apply(&part, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameter.ParameterID, Value: 20, Unit: "mm"})
			break
		}
	}
	// Switch to prompt without reaching the previously rejected target, then allow
	// that target and inspect the actual overlap at its authoritative frame.
	_, e = s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 3, Clash: &workspace.MechanismClashPolicy{Mode: "PROMPT"}})
	if e != nil {
		t.Fatal(e)
	}
	prompt, e := s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 4, Values: map[string]workspace.MotionQuantity{"clash-driver": {Value: -20, Unit: "mm"}}})
	if e != nil || !prompt.Accepted || prompt.Clash == nil || !prompt.Clash.Report.Complete || prompt.Clash.Stopped {
		t.Fatal(prompt, e)
	}
	collision := false
	for _, p := range prompt.Clash.Report.Pairs {
		collision = collision || p.Classification == "PENETRATION" || p.Classification == "CONTAINMENT"
	}
	if !collision {
		t.Fatal("prompt omitted the collision", prompt.Clash)
	}
	for _, p := range prompt.Clash.Report.Pairs {
		if (p.Classification == "PENETRATION" || p.Classification == "CONTAINMENT") && (p.DetailStatus != "COMPLETE" || len(p.OverlapTriangles) == 0) {
			t.Fatal("displayed checkpoint omitted exact overlap graphics", p)
		}
	}

	for _, body := range prompt.Clash.Report.Bodies {
		if body.GeometryKey != all.Bodies[0].GeometryKey {
			t.Fatal("motion analysis adopted pending source geometry", body)
		}
	}
	// The endpoint is safe beyond the obstacle. STOP must also check internal
	// continuation samples, instead of inspecting only the requested endpoint.
	_, e = s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 5, Clash: &workspace.MechanismClashPolicy{Mode: "OFF"}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 6, Values: map[string]workspace.MotionQuantity{"clash-driver": {Value: 0, Unit: "mm"}}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 7, Clash: &workspace.MechanismClashPolicy{Mode: "STOP"}})
	if e != nil {
		t.Fatal(e)
	}
	crossed, e := s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 8, Values: map[string]workspace.MotionQuantity{"clash-driver": {Value: 35, Unit: "mm"}}})
	if e != nil || crossed.Accepted || crossed.Clash == nil || !crossed.Clash.Stopped || crossed.Frame.UnitPoses[moving].Translation[2] > 25+1e-6 {
		t.Fatal("safe endpoint skipped intermediate collision", crossed, e)
	}
	atomic, e := s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 9, AtomicTarget: true, Values: map[string]workspace.MotionQuantity{"clash-driver": {Value: 35, Unit: "mm"}}})
	if e != nil || atomic.Clash == nil || !atomic.Clash.Stopped || !reflect.DeepEqual(atomic.Frame, crossed.Frame) {
		t.Fatal("timeline collision changed a frame without its sample time", atomic, e)
	}
	if _, e = s.UpdateMechanismSession(ctx, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 7, Clash: &workspace.MechanismClashPolicy{Mode: "OFF"}}); e == nil {
		t.Fatal("stale policy accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.UpdateMechanismSession(cancelled, product.Document.ID, p6Actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 10, Clash: &workspace.MechanismClashPolicy{Mode: "PROMPT"}}); e == nil {
		t.Fatal("cancelled analysis accepted")
	}
	saved, e := s.AnalyzeStaticInterference(ctx, product.Document.ID, workspace.StaticInterferenceRequest{RevisionID: product.Document.VersionID, Scope: "ALL", MechanismID: mechanism.ID, ThresholdOnly: true})
	if e != nil || saved.Source != "MECHANISM" || !saved.Complete {
		t.Fatal("read-only saved mechanism", saved, e)
	}
	after, e := s.ListHistory(ctx, product.Document.ID)
	if e != nil || !reflect.DeepEqual(history, after) {
		t.Fatal("collision analysis changed history", e)
	}
	var final []byte
	var head string
	if e = db.QueryRow(ctx, `SELECT d.head_version_id::text,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1`, product.Document.ID).Scan(&head, &final); e != nil || head != product.Document.VersionID || string(final) != string(original) {
		t.Fatal("clash modified product, references, poses or mechanism zero", e)
	}
}
