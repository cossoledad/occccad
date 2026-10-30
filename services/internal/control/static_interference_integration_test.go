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
