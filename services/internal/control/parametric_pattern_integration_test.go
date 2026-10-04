package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"math"
	"net"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

func TestParametricPatternsSharedUpdateThroughRouter(t *testing.T) {
	binary, url := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || url == "" {
		t.Skip("requires isolated database and built Worker")
	}
	db, err := database.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	directory := t.TempDir()
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: directory, LogDirectory: t.TempDir()})
	t.Cleanup(pool.Close)
	if err = pool.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(serveGeometry(t, pool))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store, err := artifact.NewLocalStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	service := workspace.NewWithArtifacts(db, client, artifact.NewService(db, store))
	sequence := 0
	create := func(kind, name string) workspace.DocumentView {
		t.Helper()
		v, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: kind, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	apply := func(view *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		sequence++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-pattern-%d", view.Document.ID, sequence)
		if r.Type == "CREATE_PATTERN" {
			before := view.Document.VersionID
			p, e := service.PreviewCommand(t.Context(), view.Document.ID, r)
			if e != nil {
				t.Fatal(e)
			}
			r.PreviewID = p.PreviewID
			unchanged, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || unchanged.Document.VersionID != before {
				t.Fatal("preview changed Head", e)
			}
		}
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatalf("%s: %v", r.Type, e)
		}
		*view = v
		if v.Part != nil {
			for _, f := range v.Part.Features {
				if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
					t.Fatalf("%s: %s", f.ID, f.Diagnostic)
				}
			}
		}
	}
	design := create("PART", "Pattern design parameters")
	parameters, publications := map[string]string{}, map[string]string{}
	for _, p := range []struct {
		name, unit string
		value      float64
	}{{"count", "1", 6}, {"pitch", "mm", 20}, {"phase", "deg", 0}} {
		apply(&design, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: p.name, Value: p.value, Unit: p.unit})
		for _, definition := range design.Part.Parameters {
			if definition.Key == p.name {
				parameters[p.name] = definition.ParameterID
			}
		}
		apply(&design, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: p.name, PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: parameters[p.name]})
		for _, publication := range design.Part.Publications {
			if publication.Name == p.name {
				publications[p.name] = publication.ID
			}
		}
	}
	makePart := func(name string, cut bool) (workspace.DocumentView, string) {
		t.Helper()
		v := create("PART", name)
		if cut {
			apply(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			sketch := v.Part.Features[len(v.Part.Features)-1].ID
			apply(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -30, Y: -30}, Second: &workspace.SketchPoint2{X: 30, Y: 30}}}})
			apply(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5})
		}
		apply(&v, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := v.Part.Features[len(v.Part.Features)-1].ID
		radius, pitch, zero := 2.0, 20.0, 0.0
		origin := workspace.SketchGeometryRef{Target: "SKETCH_ORIGIN", SubElement: "POINT"}
		center := workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "circle", SubElement: "CENTER"}
		apply(&v, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{
			{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 20}, Radius: 2}},
			{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "radius", Kind: "RADIUS", Unit: "mm", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "circle", SubElement: "WHOLE"}}, Value: &radius}},
			{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "pitch", Kind: "HORIZONTAL_DISTANCE", Unit: "mm", References: []workspace.SketchGeometryRef{origin, center}, Value: &pitch}},
			{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "height", Kind: "VERTICAL_DISTANCE", Unit: "mm", References: []workspace.SketchGeometryRef{origin, center}, Value: &zero}},
			{Type: "CREATE_PATTERN", Pattern: &workspace.SketchPattern{PatternDefinition: workspace.PatternDefinition{ID: "group", Kind: "CIRCULAR", Distribution: "FULL_CIRCLE", Count: 6, Direction: [3]float64{0, 0, 1}}, EntityIDs: []string{"circle"}}},
		}})
		for key, slot := range map[string]string{"count": "pattern:group:count", "pitch": "constraint:pitch:value", "phase": "pattern:group:phase"} {
			apply(&v, workspace.CommandRequest{Type: "SET_PARAMETER_EXTERNAL", ParameterID: "parameter:" + sketch + ":" + slot, SourceDocumentID: design.Document.ID, PublicationID: publications[key], ReferenceMode: "FOLLOW_HEAD"})
		}
		operation := "ADD"
		if cut {
			operation = "REMOVE"
		}
		apply(&v, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: operation, Length: 5})
		return v, sketch
	}
	fan, fanSketch := makePart("Fan hole group", true)
	cover, _ := makePart("Cover pins", false)
	product := create("PRODUCT", "Shared parameter acceptance")
	for _, id := range []string{fan.Document.ID, fan.Document.ID, cover.Document.ID} {
		apply(&product, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: id})
	}
	if !slices.Contains(product.FollowedDocumentIDs, design.Document.ID) {
		t.Fatal("shared parameter source missing from notification graph")
	}
	second := product.Product.Instances[1].ID
	apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: second, Translation: [3]float64{100, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	for _, count := range []float64{8, 4} {
		apply(&design, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameters["count"], Value: count, Unit: "1"})
		apply(&design, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameters["pitch"], Value: 22, Unit: "mm"})
		apply(&design, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameters["phase"], Value: 30, Unit: "deg"})
		plan, e := service.GetProductUpdatePlan(t.Context(), product.Document.ID)
		if e != nil {
			t.Fatal(e)
		}
		if !plan.HasUpdates {
			t.Fatal("closed Part dependencies did not report pending update")
		}
		product, e = service.AcceptParametricProductUpdate(t.Context(), product.Document.ID, fmt.Sprintf("%s-update-%g", product.Document.ID, count), plan.Digest, p6Actor, func(string, bool) error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		accepted := product.Document.VersionID
		retry, retryErr := service.AcceptParametricProductUpdate(t.Context(), product.Document.ID, fmt.Sprintf("%s-update-%g", product.Document.ID, count), plan.Digest, p6Actor, func(string, bool) error { return nil })
		if retryErr != nil || retry.Document.VersionID != accepted {
			t.Fatal("update retry was not idempotent", retryErr)
		}
		fan, e = service.GetDocument(t.Context(), fan.Document.ID, p6Actor)
		if e != nil {
			t.Fatal(e)
		}
		cover, e = service.GetDocument(t.Context(), cover.Document.ID, p6Actor)
		if e != nil {
			t.Fatal(e)
		}
		if math.Abs(activeBodyArtifact(t, fan).Volume-(18000-count*math.Pi*4*5)) > 1e-5 || math.Abs(activeBodyArtifact(t, cover).Volume-count*math.Pi*4*5) > 1e-5 {
			t.Fatal("shared parameter geometry did not update")
		}
		for _, f := range fan.Part.Features {
			if f.ID == fanSketch {
				if f.Sketch.Patterns[0].Count != count || math.Abs(f.Sketch.Patterns[0].Phase-30) > 1e-10 || math.Abs(f.Sketch.Entities[0].Center.X-22) > 1e-7 {
					t.Fatalf("parameter/solver/frame propagation lost: count=%g phase=%.17g center=%+v", f.Sketch.Patterns[0].Count, f.Sketch.Patterns[0].Phase, f.Sketch.Entities[0].Center)
				}
			}
		}
		instances := product.Product.Instances
		if instances[0].ReferencedVersionID != instances[1].ReferencedVersionID || instances[0].ReferencedVersionID != fan.Document.VersionID || instances[1].ID != second || instances[1].Translation != [3]float64{100, 0, 0} {
			t.Fatal("shared definition update changed occurrence identity or pose")
		}
	}
	// A bad shared count must not adopt a failed candidate or advance Product.
	acceptedProduct, acceptedFan, acceptedCover := product.Document.VersionID, fan.Document.VersionID, cover.Document.VersionID
	apply(&design, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameters["count"], Value: 0, Unit: "1"})
	invalidPlan, e := service.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.AcceptParametricProductUpdate(t.Context(), product.Document.ID, product.Document.ID+"-invalid", invalidPlan.Digest, p6Actor, func(string, bool) error { return nil }); e == nil {
		t.Fatal("invalid shared count accepted")
	}
	// Reopen using a fresh service, without any loaded page or in-memory model.
	cold := workspace.NewWithArtifacts(db, client, artifact.NewService(db, store))
	for id, revision := range map[string]string{product.Document.ID: acceptedProduct, fan.Document.ID: acceptedFan, cover.Document.ID: acceptedCover} {
		read, e := cold.GetDocument(t.Context(), id, p6Actor)
		if e != nil || read.Document.VersionID != revision {
			t.Fatal("failed update advanced a revision", e)
		}
	}
	apply(&design, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameters["count"], Value: 6, Unit: "1"})
	recovery, e := cold.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	product, e = cold.AcceptParametricProductUpdate(t.Context(), product.Document.ID, product.Document.ID+"-recover", recovery.Digest, p6Actor, func(string, bool) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	fan, e = cold.GetDocument(t.Context(), fan.Document.ID, p6Actor)
	if e != nil || math.Abs(activeBodyArtifact(t, fan).Volume-(18000-6*math.Pi*4*5)) > 1e-5 {
		t.Fatal("failed propagation did not recover", e)
	}
	apply(&fan, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + fanSketch + ":constraint:radius:value", Value: 3, Unit: "mm"})
	if math.Abs(activeBodyArtifact(t, fan).Volume-(18000-6*math.Pi*9*5)) > 1e-5 {
		t.Fatal("seed hole diameter did not update every member")
	}
	// The two other evaluators consume the same distribution model and real
	// upstream stages: a seed solid and a sketch reused in spatial frames.
	blade := create("PART", "Solid and spatial patterns")
	apply(&blade, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := blade.Part.Features[0]
	apply(&blade, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 2, Y: 3}}}})
	apply(&blade, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch.ID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5})
	seed := blade.Part.Features[len(blade.Part.Features)-1]
	apply(&blade, workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: seed.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 3, Spacing: 10, Direction: [3]float64{1, 0, 0}}, Source: workspace.FeatureStageRef{BodyID: seed.BodyID, FeatureID: seed.ID}, SourceKind: "GENERATOR_TOOL", ResultMode: "INDEPENDENT"}}})
	if math.Abs(activeBodyArtifact(t, blade).Volume-90) > 1e-6 {
		t.Fatal("independent solid pattern geometry incorrect")
	}
	patternFeature := blade.Part.Features[len(blade.Part.Features)-1]
	memberPick := findP7TopologyPick(t, service, blade, "FACE", func(_ workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == patternFeature.ID && strings.Contains(selection.Anchor.OutputSlot, "MEMBER/2/")
	})
	apply(&blade, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + seed.ID + ":length", Value: 7, Unit: "mm"})
	if math.Abs(activeBodyArtifact(t, blade).Volume-126) > 1e-6 {
		t.Fatal("seed edit not propagated to solids")
	}
	resolution, e := cold.ResolvePersistentSelection(t.Context(), blade.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: memberPick.Selection, SourceVersionID: memberPick.SourceVersionID, TargetVersionID: blade.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if e != nil || resolution.Status != modelcore.SelectionResolved {
		t.Fatalf("member face did not follow seed edit: %#v %v", resolution, e)
	}
	apply(&blade, workspace.CommandRequest{Type: "UNDO"})
	if math.Abs(activeBodyArtifact(t, blade).Volume-90) > 1e-6 {
		t.Fatal("pattern undo incorrect")
	}
	apply(&blade, workspace.CommandRequest{Type: "REDO"})
	if math.Abs(activeBodyArtifact(t, blade).Volume-126) > 1e-6 {
		t.Fatal("pattern redo incorrect")
	}
	apply(&blade, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + patternFeature.ID + ":pattern:count", Value: 2, Unit: "1"})
	resolution, e = cold.ResolvePersistentSelection(t.Context(), blade.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: memberPick.Selection, SourceVersionID: memberPick.SourceVersionID, TargetVersionID: blade.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if e != nil || resolution.Status == modelcore.SelectionResolved {
		t.Fatalf("vanished member rebound: %#v %v", resolution, e)
	}
	apply(&blade, workspace.CommandRequest{Type: "UNDO"})
	suppressed := true
	apply(&blade, workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: patternFeature.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, blade.StructureTree, patternFeature.ID), Suppressed: &suppressed})
	if math.Abs(activeBodyArtifact(t, blade).Volume-42) > 1e-6 {
		t.Fatal("pattern suppression retained generated solids")
	}
	apply(&blade, workspace.CommandRequest{Type: "UNDO"})
	apply(&blade, workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SKETCH_PATTERN", BodyID: sketch.BodyID, Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 3, Spacing: 10, Direction: [3]float64{0, 0, 1}}, Source: workspace.FeatureStageRef{BodyID: sketch.BodyID, FeatureID: sketch.ID}, SourceKind: "SKETCH_FRAME"}}})
	frames := blade.Part.Features[len(blade.Part.Features)-1]
	slot := 1
	apply(&blade, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: frames.ID, ProfileMemberSlot: &slot, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 2})
	if math.Abs(activeBodyArtifact(t, blade).Volume-12) > 1e-6 {
		t.Fatal("spatial sketch was not consumed by extrude")
	}
	first, last := 0, 2
	apply(&blade, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", BodyID: sketch.BodyID, Operation: "NEW_BODY", Sections: []workspace.LoftSection{{SketchID: frames.ID, MemberSlot: &first}, {SketchID: frames.ID, MemberSlot: &last}}}})
	if math.Abs(activeBodyArtifact(t, blade).Volume-120) > 1e-6 {
		t.Fatal("spatial sketch was not consumed by loft")
	}

	// An explicit section disappears: keep the prior display as failed, never
	// silently consume a neighboring frame. Restoring the slot recovers it.
	failed, e := service.ApplyCommand(t.Context(), blade.Document.ID, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", RequestID: blade.Document.ID + "-missing-frame", ActorID: p6Actor, ParameterID: "parameter:" + frames.ID + ":pattern:count", Value: 2, Unit: "1"})
	if e != nil {
		t.Fatal(e)
	}
	lastFeature := failed.Part.Features[len(failed.Part.Features)-1]
	if lastFeature.EvaluationStatus != "FAILED" || !strings.Contains(lastFeature.Diagnostic, "PATTERN_MEMBER_MISSING") {
		t.Fatalf("missing section was not diagnosed: %+v", lastFeature)
	}
	blade = failed
	apply(&blade, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + frames.ID + ":pattern:count", Value: 3, Unit: "1"})
	if math.Abs(activeBodyArtifact(t, blade).Volume-120) > 1e-6 {
		t.Fatal("restored member did not recover downstream loft")
	}

}
