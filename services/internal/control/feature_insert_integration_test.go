//go:build integration

package control

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type insertionScene struct {
	t             *testing.T
	s             *workspace.Service
	artifacts     *artifact.Service
	db            database.DB
	client        *geometry.Client
	v             workspace.DocumentView
	base, chamfer workspace.Feature
	seq           int
}

func newInsertionScene(t *testing.T) *insertionScene {
	t.Helper()
	s, a, db, c, _ := featureAssociationRuntimeService(t)
	v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Insert local modifier"})
	if e != nil {
		t.Fatal(e)
	}
	x := &insertionScene{t: t, s: s, artifacts: a, db: db, client: c, v: v}
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := x.v.Part.Features[0].ID
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	x.base = x.v.Part.Features[len(x.v.Part.Features)-1]
	p := findP7TopologyPick(t, s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.CreationEvidence.Centroid == [3]float64{10, 10, 10}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: x.base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: p.Selection, SourceVersionID: p.SourceVersionID}}}})
	x.chamfer = x.v.Part.Features[len(x.v.Part.Features)-1]
	return x
}
func (x *insertionScene) request(r workspace.CommandRequest) workspace.CommandRequest {
	x.seq++
	r.ActorID = p6Actor
	if r.RequestID == "" {
		r.RequestID = fmt.Sprintf("%s-insert-%d", x.v.Document.ID, x.seq)
	}
	return r
}
func (x *insertionScene) apply(r workspace.CommandRequest) {
	x.t.Helper()
	v, e := x.s.ApplyCommand(x.t.Context(), x.v.Document.ID, x.request(r))
	if e != nil {
		x.t.Fatal(r.Type, e)
	}
	x.v = v
}
func (x *insertionScene) input(after, body string) workspace.FeatureInput {
	x.t.Helper()
	r, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: after, BodyID: body})
	if e != nil {
		x.t.Fatal(e)
	}
	return r
}
func (x *insertionScene) pick(after string, centroid [3]float64) (workspace.FeatureSelection, string) {
	x.t.Helper()
	input := x.input(after, x.base.BodyID)
	for local := uint64(1); local <= p6TopologyCount(x.t, input.Artifact.Topology, "edges"); local++ {
		r, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: after, BodyID: x.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "EDGE", LocalID: local})
		if e != nil {
			x.t.Fatal(e)
		}
		if r.Selection.Selection.CreationEvidence.Centroid == centroid {
			return *r.Selection, input.Artifact.GeometryKey
		}
	}
	x.t.Fatal("stage edge missing", centroid)
	return workspace.FeatureSelection{}, ""
}
func (x *insertionScene) insert(after string, point [3]float64, radius float64) workspace.CommandRequest {
	p, key := x.pick(after, point)
	return x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: after, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: "FILLET", BodyID: x.base.BodyID, Length: radius, Selections: []workspace.FeatureSelection{p}}})
}
func (x *insertionScene) reject(r workspace.CommandRequest, reason string) {
	x.t.Helper()
	r = x.request(r)
	head := x.v.Document.VersionID
	for _, preview := range []bool{true, false} {
		var e error
		if preview {
			_, e = x.s.PreviewCommand(x.t.Context(), x.v.Document.ID, r)
		} else {
			_, e = x.s.ApplyCommand(x.t.Context(), x.v.Document.ID, r)
		}
		if e == nil || !strings.Contains(e.Error(), reason) {
			x.t.Fatal("expected atomic rejection", reason, e)
		}
	}
	v, e := x.s.GetDocument(x.t.Context(), x.v.Document.ID, p6Actor)
	if e != nil || v.Document.VersionID != head {
		x.t.Fatal("rejection advanced Head", e)
	}
}

func TestInsertModifierLifecycleThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	tctx := t.Context()
	head := x.v.Document.VersionID
	input := x.input(x.base.ID, x.base.BodyID)
	if input.SourceFeatureID != x.base.ID || math.Abs(input.Artifact.Volume-8000) > 1e-6 || input.Artifact.GeometryKey == activeBodyArtifact(t, x.v).GeometryKey {
		t.Fatal("not the exact base stage")
	}
	for _, r := range []workspace.FeatureInputRequest{
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, GeometryKey: activeBodyArtifact(t, x.v).GeometryKey, Kind: "EDGE", LocalID: 1},
		{VersionID: head, FeatureID: x.base.ID, ResultStage: true, GeometryKey: input.Artifact.GeometryKey, Kind: "EDGE", LocalID: 1},
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, ResultStage: true},
		{VersionID: head, AfterFeatureID: x.v.Part.Features[0].ID, BodyID: x.base.BodyID},
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: "wrong-body"},
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "VERTEX", LocalID: 1},
	} {
		if _, e := x.s.GetFeatureInput(tctx, x.v.Document.ID, r); e == nil {
			t.Fatal("illegal pick authorized", r)
		}
	}
	// Consuming a saved downstream chamfer edge is a geometry/reference blocker.
	x.reject(x.insert(x.base.ID, [3]float64{10, 10, 10}, 1), x.chamfer.ID)
	r := x.insert(x.base.ID, [3]float64{-10, -10, 10}, 1)
	bad := r
	f := *r.Feature
	f.Selections = append([]workspace.FeatureSelection(nil), f.Selections...)
	f.Selections[0].SourceFeatureID = x.chamfer.ID
	bad.Feature = &f
	x.reject(bad, "insertion stage")
	bad = r
	bad.GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
	x.reject(bad, "GeometryKey")
	bad = r
	f = *r.Feature
	f.Type = "SHELL"
	bad.Feature = &f
	x.reject(bad, "modifier selection")
	bad = r
	bad.VersionID = "missing-revision"
	x.reject(bad, "insertion stage")
	before := append([]workspace.Feature(nil), x.v.Part.Features...)
	params := append([]modelcore.ParameterDefinition(nil), x.v.Part.Parameters...)
	history, e := x.s.ListHistory(tctx, x.v.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	r.ParameterExpressions = map[string]string{"length": "0.5 mm + 0.5 mm"}
	preview, e := x.s.PreviewCommand(tctx, x.v.Document.ID, r)
	if e != nil || preview.Artifact == nil {
		t.Fatal("preview", e)
	}
	now, e := x.s.GetDocument(tctx, x.v.Document.ID, p6Actor)
	if e != nil || now.Document.VersionID != head {
		t.Fatal("preview/cancel wrote Head", e)
	}
	stale := r
	f = *r.Feature
	f.Length = 2
	stale.Feature = &f
	stale.ParameterExpressions = nil
	stale.PreviewID = preview.PreviewID
	if _, e = x.s.ApplyCommand(tctx, x.v.Document.ID, stale); e == nil {
		t.Fatal("old preview accepted changed inputs")
	}
	r.PreviewID = preview.PreviewID
	x.apply(r)
	after, e := x.s.ListHistory(tctx, x.v.Document.ID)
	if e != nil || len(after) != len(history)+1 {
		t.Fatal("insertion was not one history step", e)
	}
	created := x.v.Part.Features[2]
	if created.ID == x.chamfer.ID || created.Type != "FILLET" || x.v.Part.Features[3].ID != x.chamfer.ID || created.Selections[0].SourceFeatureID != x.base.ID || created.Selections[0].SourceVersionID != head {
		t.Fatal("middle creation or exact source lost", created)
	}
	for i, f := range x.v.Part.Features {
		if f.Order != i+1 {
			t.Fatal("Order not synchronized")
		}
	}
	for _, p := range params {
		found := false
		for _, q := range x.v.Part.Parameters {
			if p.ParameterID == q.ParameterID {
				found = true
				if !reflect.DeepEqual(p, q) {
					t.Fatal("old parameter modified")
				}
			}
		}
		if !found {
			t.Fatal("old parameter missing")
		}
	}
	for _, old := range before {
		found := false
		for _, f := range x.v.Part.Features {
			if old.ID == f.ID {
				old.Order = f.Order
				found = true
				if !reflect.DeepEqual(old, f) {
					t.Fatal("old feature rebound")
				}
			}
		}
		if !found {
			t.Fatal("old identity lost")
		}
	}
	final := activeBodyArtifact(t, x.v)
	if math.Abs(final.Volume-(7990-20*(1-math.Pi/4))) > 1e-5 {
		t.Fatal("incorrect final geometry", final.Volume)
	}
	stage, e := x.s.GetFeatureInput(tctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, FeatureID: created.ID, ResultStage: true})
	if e != nil || math.Abs(stage.Artifact.Volume-(8000-20*(1-math.Pi/4))) > 1e-5 {
		t.Fatal("incorrect intermediate stage", e)
	}
	retry, e := x.s.ApplyCommand(tctx, x.v.Document.ID, r)
	if e != nil || retry.Document.VersionID != x.v.Document.VersionID {
		t.Fatal("not idempotent", e)
	}
	x.apply(workspace.CommandRequest{Type: "UNDO"})
	if !reflect.DeepEqual(x.v.Part.Features, before) || !reflect.DeepEqual(x.v.Part.Parameters, params) {
		t.Fatal("one Undo did not restore complete order and parameters")
	}
	x.apply(workspace.CommandRequest{Type: "REDO"})
	if activeBodyArtifact(t, x.v).GeometryID != final.GeometryID || x.v.Part.Features[2].ID != created.ID {
		t.Fatal("Redo lost identity/geometry")
	}
	fresh := workspace.NewWithArtifacts(x.db, x.client, x.artifacts)
	opened, e := fresh.GetDocument(tctx, x.v.Document.ID, p6Actor)
	if e != nil || !reflect.DeepEqual(opened.Part.Features, x.v.Part.Features) {
		t.Fatal("reopen", e)
	}
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(tctx), x.v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: x.v.Document.ID + "-cold", Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold mismatch", role)
		}
	}
	staleHead := r
	staleHead.RequestID += "-stale-head"
	x.reject(staleHead, "")
	// The pre-insertion context expires as soon as Revision changes.
	if _, e = x.s.GetFeatureInput(tctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID}); e == nil {
		t.Fatal("stale insertion pick authorized")
	}
}

func TestInsertModifierRangeAndFixedStageThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	x.apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: x.base.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Direction: [3]float64{1, 0, 0}, Count: 2, Spacing: 30}, SourceKind: "FEATURE_DELTA", StartFeatureID: x.base.ID, Source: workspace.FeatureStageRef{BodyID: x.base.BodyID, FeatureID: x.chamfer.ID}, ResultMode: "COMBINE"}}})
	pattern := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.reject(x.insert(x.base.ID, [3]float64{-10, -10, 10}, 1), "FEATURE_DELTA")
	for _, kind := range []string{"DRAFT", "SHELL"} {
		x.reject(x.faceInsertion(kind, x.base.ID), "FEATURE_DELTA")
	}
	if p, e := x.s.PreviewCommand(t.Context(), x.v.Document.ID, x.faceInsertion("DRAFT", pattern.ID)); e != nil || p.Artifact == nil {
		t.Fatal("draft after fixed pattern output", e)
	}
	// An unchanged fixed pattern is a legitimate insertion anchor; this is not a move.
	r := x.insert(pattern.ID, [3]float64{-10, -10, 10}, .2)
	r.Feature.Type = "CHAMFER"
	x.apply(r)
	created := x.v.Part.Features[len(x.v.Part.Features)-1]
	if created.Type != "CHAMFER" || created.Selections[0].SourceFeatureID != pattern.ID {
		t.Fatal("fixed output was not a legitimate stage")
	}
}

func TestInsertModifierRejectsCrossBodyFailureThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	bodyA := x.base.BodyID
	x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	bodyB := x.v.Part.ActiveBodyID
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10.2, Y: -10.2}, Second: &workspace.SketchPoint2{X: -9.8, Y: -9.8}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	x.apply(workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: bodyB, Operation: "INTERSECT", KeepTools: true, Tools: []workspace.FeatureStageRef{{BodyID: bodyA, FeatureID: x.chamfer.ID}}})
	consumer := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: bodyA})
	// The inserted fillet is valid in A, but removes all material used by B's
	// intersection with the declared A stage. All structural inputs stay earlier.
	x.reject(x.insert(x.base.ID, [3]float64{-10, -10, 10}, 2), consumer.ID)
	cross := x.insert(x.base.ID, [3]float64{10, -10, 10}, 1)
	cross.Feature.BodyID = bodyB
	x.reject(cross, "specified Body")
	if !slices.ContainsFunc(x.v.Part.Features, func(f workspace.Feature) bool { return f.ID == consumer.ID && f.EvaluationStatus == "" }) {
		t.Fatal("original consumer lost")
	}
}

// The target Body and a plane's source Body are separate authorization scopes.
func (x *insertionScene) face(after, target, source string, centroid [3]float64, reference bool) (workspace.FeatureSelection, string) {
	return x.faceWhere(after, target, source, reference, func(p workspace.FeatureSelection) bool {
		c := p.Selection.CreationEvidence.Centroid
		return math.Abs(c[0]-centroid[0]) < 1e-7 && math.Abs(c[1]-centroid[1]) < 1e-7 && math.Abs(c[2]-centroid[2]) < 1e-7
	})
}
func (x *insertionScene) faceWhere(after, target, source string, reference bool, match func(workspace.FeatureSelection) bool) (workspace.FeatureSelection, string) {
	x.t.Helper()
	r := workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: after, BodyID: target}
	if reference {
		r.ReferenceBodyID = source
	}
	input, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, r)
	if e != nil {
		x.t.Fatal(e)
	}
	r.GeometryKey = input.Artifact.GeometryKey
	r.Kind = "FACE"
	for local := uint64(1); local <= p6TopologyCount(x.t, input.Artifact.Topology, "faces"); local++ {
		r.LocalID = local
		p, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, r)
		if e != nil {
			x.t.Fatal(e)
		}
		if match(*p.Selection) {
			return *p.Selection, input.Artifact.GeometryKey
		}
	}
	x.t.Fatal("stage face missing", after, source)
	return workspace.FeatureSelection{}, ""
}
func (x *insertionScene) faceInsertion(kind, after string) workspace.CommandRequest {
	x.t.Helper()
	point := [3]float64{-10, 0, 10}
	if kind == "SHELL" {
		point = [3]float64{0, 0, 20}
	}
	p, key := x.face(after, x.base.BodyID, x.base.BodyID, point, false)
	return x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: after, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: kind, BodyID: x.base.BodyID, Length: 2, Angle: 3, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{p}}})
}
func TestInsertFaceModifiersLifecycleThroughRouter(t *testing.T) {
	for _, kind := range []string{"DRAFT", "SHELL"} {
		t.Run(kind, func(t *testing.T) {
			x := newInsertionScene(t)
			ctx := t.Context()
			head := x.v.Document.VersionID
			before := append([]workspace.Feature(nil), x.v.Part.Features...)
			parameters := append([]modelcore.ParameterDefinition(nil), x.v.Part.Parameters...)
			r := x.faceInsertion(kind, x.base.ID)
			slot, expression := "angle", "1 deg + 2 deg"
			if kind == "SHELL" {
				slot, expression = "length", "1 mm + 1 mm"
			}
			r.ParameterExpressions = map[string]string{slot: expression}
			// The final Body and a newer source stage cannot impersonate this input.
			wrong := r
			f := *r.Feature
			f.Selections = append([]workspace.FeatureSelection(nil), f.Selections...)
			f.Selections[0].SourceFeatureID = x.chamfer.ID
			wrong.Feature = &f
			x.reject(wrong, "insertion stage")
			wrong = r
			wrong.GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
			x.reject(wrong, "GeometryKey")
			history, e := x.s.ListHistory(ctx, x.v.Document.ID)
			if e != nil {
				t.Fatal(e)
			}
			preview, e := x.s.PreviewCommand(ctx, x.v.Document.ID, r)
			if e != nil || preview.Artifact == nil {
				t.Fatal("preview", e)
			}
			current, e := x.s.GetDocument(ctx, x.v.Document.ID, p6Actor)
			if e != nil || current.Document.VersionID != head {
				t.Fatal("preview wrote Head", e)
			}
			coldPreview, e := x.s.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, r)
			if e != nil || coldPreview.Artifact == nil {
				t.Fatal("cold preview", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if preview.Artifact.Representations[role].Digest != coldPreview.Artifact.Representations[role].Digest {
					t.Fatal("hot/cold preview mismatch", role)
				}
			}
			stale := r
			bad := *r.Feature
			bad.Length = 3
			bad.Angle = 4
			stale.Feature = &bad
			stale.ParameterExpressions = nil
			stale.PreviewID = preview.PreviewID
			if _, e = x.s.ApplyCommand(ctx, x.v.Document.ID, stale); e == nil {
				t.Fatal("changed inputs adopted old preview")
			}
			r.PreviewID = preview.PreviewID
			x.apply(r)
			created := x.v.Part.Features[2]
			if created.Type != kind || created.ID == x.chamfer.ID || created.Selections[0].SourceFeatureID != x.base.ID || x.v.Part.Features[3].ID != x.chamfer.ID {
				t.Fatal("identity or order lost", created)
			}
			for i, old := range before {
				now := x.v.Part.Features[i+func() int {
					if i > 1 {
						return 1
					}
					return 0
				}()]
				old.Order = now.Order
				if !reflect.DeepEqual(old, now) {
					t.Fatal("existing definition changed", old.Name)
				}
			}
			for _, old := range parameters {
				if !slices.ContainsFunc(x.v.Part.Parameters, func(p modelcore.ParameterDefinition) bool { return reflect.DeepEqual(old, p) }) {
					t.Fatal("old parameter changed", old.ParameterID)
				}
			}
			after, e := x.s.ListHistory(ctx, x.v.Document.ID)
			if e != nil || len(after) != len(history)+1 {
				t.Fatal("insertion not one history entry", e)
			}
			final := activeBodyArtifact(t, x.v)
			if !(final.Volume > 0 && final.Volume < 8000) {
				t.Fatal("no real material change", final.Volume)
			}
			x.apply(workspace.CommandRequest{Type: "UNDO"})
			if !reflect.DeepEqual(x.v.Part.Features, before) {
				t.Fatal("Undo lost definitions")
			}
			x.apply(workspace.CommandRequest{Type: "REDO"})
			if activeBodyArtifact(t, x.v).GeometryID != final.GeometryID || x.v.Part.Features[2].ID != created.ID {
				t.Fatal("Redo lost geometry/identity")
			}
			fresh := workspace.NewWithArtifacts(x.db, x.client, x.artifacts)
			opened, e := fresh.GetDocument(ctx, x.v.Document.ID, p6Actor)
			if e != nil || !reflect.DeepEqual(opened.Part.Features, x.v.Part.Features) {
				t.Fatal("reopen", e)
			}
			edit := workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)}
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, edit)
			if e != nil || cold.Artifact == nil {
				t.Fatal("cold edit", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
					t.Fatal("reopened cold mismatch", role)
				}
			}
			if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID}); e == nil {
				t.Fatal("stale input accepted")
			}
			// A direct commit must perform the same final evaluation as preview adoption.
			x.apply(workspace.CommandRequest{Type: "UNDO"})
			r.RequestID += "-direct"
			r.PreviewID = ""
			r.VersionID = x.v.Document.VersionID
			for i := range r.Feature.Selections {
				r.Feature.Selections[i].SourceVersionID = r.VersionID
			}
			x.apply(r)
			if math.Abs(activeBodyArtifact(t, x.v).Volume-final.Volume) > 1e-6 {
				t.Fatal("direct/preview geometry mismatch")
			}
		})
	}
}

func TestInsertDraftHistoricalNeutralPlaneThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	ctx := t.Context()
	target := x.base.BodyID
	// Create an independent reference Body, then a new target stage after it.
	x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	other := x.v.Part.ActiveBodyID
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 30, Y: -10}, Second: &workspace.SketchPoint2{X: 50, Y: 10}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	otherBase := x.v.Part.Features[len(x.v.Part.Features)-1]
	edge := findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.SourceBodyID == other && p.CreationEvidence.Centroid == [3]float64{50, 10, 5}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: other, Length: .5, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
	reference := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: target})
	p := findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.SourceBodyID == target && p.CreationEvidence.Centroid == [3]float64{-10, -10, 10}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: target, Length: .5, Selections: []workspace.FeatureSelection{{Selection: p.Selection, SourceVersionID: p.SourceVersionID}}}})
	anchor := x.v.Part.Features[len(x.v.Part.Features)-1]
	// Later reference-Body geometry must never be exposed as this earlier plane input.
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: other})
	edge = findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.SourceBodyID == other && p.CreationEvidence.Centroid == [3]float64{30, -10, 5}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: other, Length: .5, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
	later := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: target})
	face, key := x.faceWhere(anchor.ID, target, target, false, func(p workspace.FeatureSelection) bool {
		return p.Selection.CreationEvidence.GeometryType == "PLANE" && math.Abs(p.Selection.CreationEvidence.Centroid[0]+10) < 1e-7
	})
	neutral, referenceKey := x.faceWhere(anchor.ID, target, other, true, func(p workspace.FeatureSelection) bool {
		return math.Abs(p.Selection.CreationEvidence.Centroid[2]) < 1e-7
	})
	if neutral.SourceFeatureID != reference.ID || neutral.Selection.Anchor.FeatureID != otherBase.ID || neutral.SourceFeatureID == neutral.Selection.Anchor.FeatureID {
		t.Fatal("semantic anchor used as source stage", neutral)
	}
	r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: anchor.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: "DRAFT", BodyID: target, Angle: 2, NeutralPlane: &neutral, Selections: []workspace.FeatureSelection{face}}})
	for _, badStage := range []string{later.ID, otherBase.ID} {
		wrong := r
		f := *r.Feature
		n := neutral
		n.SourceFeatureID = badStage
		f.NeutralPlane = &n
		wrong.Feature = &f
		x.reject(wrong, "plane")
	}
	wrong := r
	f := *r.Feature
	n := neutral
	n.SourceVersionID = "00000000-0000-7000-8000-000000000099"
	f.NeutralPlane = &n
	wrong.Feature = &f
	x.reject(wrong, "neutral plane")
	inputRequest := workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: anchor.ID, BodyID: target, ReferenceBodyID: other}
	display, e := x.s.GetFeatureInputGeometry(ctx, x.v.Document.ID, inputRequest)
	if e != nil || display.SourceFeatureID != reference.ID || display.Artifact.GeometryKey != referenceKey || display.Artifact.BodyID != other {
		t.Fatal("reference display authorization", e)
	}
	finalOther := ""
	for _, b := range x.v.Part.Bodies {
		if b.ID == other {
			finalOther = b.GeometryKey
		}
	}
	if finalOther == referenceKey {
		t.Fatal("test has no later reference geometry")
	}
	for _, kind := range []string{"FACE", "EDGE"} {
		bad := inputRequest
		bad.Kind = kind
		bad.LocalID = 1
		bad.GeometryKey = finalOther
		if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, bad); e == nil {
			t.Fatal("downstream/edge plane pick accepted")
		}
	}
	// A Body created after the insertion point cannot be borrowed for a plane.
	if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: x.base.ID, BodyID: target, ReferenceBodyID: other}); e == nil {
		t.Fatal("future Body authorized")
	}
	head := x.v.Document.VersionID
	pre, e := x.s.PreviewCommand(ctx, x.v.Document.ID, r)
	if e != nil || pre.Artifact == nil {
		t.Fatal("cross-Body plane preview", e)
	}
	originalOther := finalOther
	r.PreviewID = pre.PreviewID
	x.apply(r)
	created := x.v.Part.Features[slices.IndexFunc(x.v.Part.Features, func(f workspace.Feature) bool { return f.ID == anchor.ID })+1]
	if created.NeutralPlane.SourceFeatureID != reference.ID || created.NeutralPlane.SourceVersionID != head || created.NeutralPlane.Selection.SourceBodyID != other {
		t.Fatal("plane source changed")
	}
	for _, b := range x.v.Part.Bodies {
		if b.ID == other && b.GeometryKey != originalOther {
			t.Fatal("unrelated Body changed")
		}
	}
	cold, e := x.s.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold cross-Body plane", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != pre.Artifact.Representations[role].Digest {
			t.Fatal("plane hot/cold mismatch", role)
		}
	}
}

func TestInsertDraftDatumOrderAndFaceModifierFailureThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	top := findP7TopologyPick(t, x.s, x.v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.CreationEvidence.GeometryType == "PLANE" && math.Abs(p.CreationEvidence.Centroid[2]-20) < 1e-7
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "TOPOLOGY", FeatureID: x.chamfer.ID, SourceVersionID: top.SourceVersionID, Selection: &top.Selection}}})
	future := x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1]
	x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "PLANE", EntityID: future.ID}}})
	chain := x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1]
	r := x.faceInsertion("DRAFT", x.base.ID)
	r.Feature.NeutralPlaneID = chain.ID
	x.reject(r, "DEPENDENCY_CYCLE")
	r = x.faceInsertion("SHELL", x.base.ID)
	r.Feature.Length = 40
	x.reject(r, "Shell.1")
	// A datum created later in wall-clock time is legal when its full dependency
	// chain comes from standard datums, rather than a later solid stage.
	x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "PLANE", EntityID: "datum-xy"}}})
	r = x.faceInsertion("DRAFT", x.base.ID)
	r.Feature.NeutralPlaneID = x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID
	if p, e := x.s.PreviewCommand(t.Context(), x.v.Document.ID, r); e != nil || p.Artifact == nil {
		t.Fatal("legal custom datum rejected", e)
	}
}

func TestInsertFaceModifiersBooleanAndRemovalProtectionThroughRouter(t *testing.T) {
	t.Run("Boolean output", func(t *testing.T) {
		x := newInsertionScene(t)
		body := x.base.BodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
		toolBody := x.v.Part.ActiveBodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 8, Y: -20}, Second: &workspace.SketchPoint2{X: 12, Y: 20}}}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 25})
		tool := x.v.Part.Features[len(x.v.Part.Features)-1]
		x.apply(workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: body, Operation: "REMOVE", KeepTools: true, Tools: []workspace.FeatureStageRef{{BodyID: toolBody, FeatureID: tool.ID}}})
		anchor := x.v.Part.Features[len(x.v.Part.Features)-1]
		top, key := x.faceWhere(anchor.ID, body, body, false, func(p workspace.FeatureSelection) bool {
			return math.Abs(p.Selection.CreationEvidence.Centroid[2]-20) < 1e-7
		})
		r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: anchor.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: "SHELL", BodyID: body, Length: 2, Selections: []workspace.FeatureSelection{top}}})
		x.apply(r)
		last := x.v.Part.Features[len(x.v.Part.Features)-1]
		if last.Type != "SHELL" || last.Selections[0].SourceFeatureID != anchor.ID {
			t.Fatal("Boolean output stage lost")
		}
	})
	t.Run("net removal range", func(t *testing.T) {
		x := newInsertionScene(t)
		body := x.base.BodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -3, Y: -3}, Second: &workspace.SketchPoint2{X: 3, Y: 3}}}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 10})
		cut := x.v.Part.Features[len(x.v.Part.Features)-1]
		side, _ := x.face(cut.ID, body, body, [3]float64{-3, 0, 5}, false)
		x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "DRAFT", BodyID: body, Angle: 2, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{side}}})
		end := x.v.Part.Features[len(x.v.Part.Features)-1]
		x.apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "REMOVE", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Direction: [3]float64{1, 0, 0}, Count: 2, Spacing: 8}, SourceKind: "FEATURE_DELTA", StartFeatureID: cut.ID, Source: workspace.FeatureStageRef{BodyID: body, FeatureID: end.ID}, ResultMode: "COMBINE"}}})
		for _, kind := range []string{"DRAFT", "SHELL"} {
			pick, key := x.faceWhere(cut.ID, body, body, false, func(p workspace.FeatureSelection) bool {
				if kind == "SHELL" {
					return math.Abs(p.Selection.CreationEvidence.Centroid[2]-20) < 1e-7
				}
				return math.Abs(p.Selection.CreationEvidence.Centroid[0]+10) < 1e-7
			})
			r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: cut.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: kind, BodyID: body, Length: 1, Angle: 2, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{pick}}})
			x.reject(r, "FEATURE_DELTA")
		}
	})
}

func TestInsertContextVariantModifiersThroughRouter(t *testing.T) {
	for _, kind := range []string{"DRAFT", "SHELL"} {
		for _, adopt := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/adopt=%v", kind, adopt), func(t *testing.T) {
				x := newInsertionScene(t)
				ctx := t.Context()
				body := x.base.BodyID
				param := ""
				for _, p := range x.v.Part.Parameters {
					if p.OwnerFeatureID == x.base.ID && p.PropertySlot == "length" {
						param = p.ParameterID
					}
				}
				x.apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: param, Value: 10, Unit: "mm"})
				x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
				referenceBody := x.v.Part.ActiveBodyID
				x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 30, Y: -10}, Second: &workspace.SketchPoint2{X: 50, Y: 10}}}})
				x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
				reference := x.v.Part.Features[len(x.v.Part.Features)-1]
				referenceParam := ""
				for _, p := range x.v.Part.Parameters {
					if p.OwnerFeatureID == reference.ID && p.PropertySlot == "length" {
						referenceParam = p.ParameterID
					}
				}
				x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: body})
				anchor := x.base
				if kind == "DRAFT" {
					edge := findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
						return p.SourceBodyID == body && p.CreationEvidence.Centroid == [3]float64{-10, -10, 5}
					})
					x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: body, Length: .5, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
					anchor = x.v.Part.Features[len(x.v.Part.Features)-1]
				}

				create := func(typ, name string) workspace.DocumentView {
					v, e := x.s.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: typ, Name: name})
					if e != nil {
						t.Fatal(e)
					}
					return v
				}
				run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
					r = x.request(r)
					n, e := x.s.ApplyCommand(ctx, v.Document.ID, r)
					if e != nil {
						t.Fatal(r.Type, e)
					}
					*v = n
				}
				source := create("PART", "Context insertion layout")
				run(&source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "height", Value: 20, Unit: "mm"})
				run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Height", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: source.Part.Parameters[0].ParameterID})
				root := create("PRODUCT", "Projected historical insertion")
				run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: source.Document.ID})
				run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: x.v.Document.ID})
				for _, parameter := range []string{param, referenceParam} {
					a, e := x.s.GetProductDesignSession(ctx, root.Document.ID, root.Product.Instances[0].ID)
					if e != nil {
						t.Fatal(e)
					}
					b, e := x.s.GetProductDesignSession(ctx, root.Document.ID, root.Product.Instances[1].ID)
					if e != nil {
						t.Fatal(e)
					}
					root, e = x.s.CreateProductContextBinding(ctx, root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-input-%s", root.Document.ID, parameter), OwningInstancePath: *b.ActiveInstancePath, SourceInstancePath: *a.ActiveInstancePath, PublicationID: source.Part.Publications[0].ID, PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: parameter})
					if e != nil {
						t.Fatal(e)
					}
				}
				acceptedRoot := root.Document.VersionID
				run(&source, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: source.Part.Parameters[0].ParameterID, Value: 25, Unit: "mm"})
				pendingRoot, pendingError := x.s.GetDocument(ctx, root.Document.ID, p6Actor)
				if pendingError != nil || pendingRoot.Document.VersionID != acceptedRoot {
					t.Fatal("unaccepted source update advanced Product", pendingError)
				}
				root = pendingRoot
				for _, feature := range root.ContextVariants[0].Part.Features {
					if (feature.ID == x.base.ID || feature.ID == reference.ID) && feature.Length != 20 {
						t.Fatal("projection read source Head instead of accepted 20 mm", feature.Length)
					}
				}

				var e error
				x.v, e = x.s.GetDocument(ctx, x.v.Document.ID, p6Actor)
				if e != nil {
					t.Fatal(e)
				}
				variant := root.ContextVariants[0]
				scope := workspace.ContextReadScope{RootProductDocumentID: root.Document.ID, RootProductRevisionID: root.Document.VersionID, ContextVariantID: variant.VariantKey, InstancePath: &variant.OwningInstancePath}
				pick := func(other string, match func(modelcore.TopologySelectionEvidence) bool) (workspace.FeatureSelection, string) {
					r := workspace.FeatureInputRequest{ContextReadScope: scope, VersionID: x.v.Document.VersionID, AfterFeatureID: anchor.ID, BodyID: body, ReferenceBodyID: other}
					input, e := x.s.GetFeatureInput(ctx, x.v.Document.ID, r)
					if e != nil {
						t.Fatal(e)
					}
					r.GeometryKey = input.Artifact.GeometryKey
					r.Kind = "FACE"
					for i := uint64(1); i <= p6TopologyCount(t, input.Artifact.Topology, "faces"); i++ {
						r.LocalID = i
						p, e := x.s.GetFeatureInput(ctx, x.v.Document.ID, r)
						if e != nil {
							t.Fatal(e)
						}
						if match(p.Selection.Selection.CreationEvidence) {
							return *p.Selection, r.GeometryKey
						}
					}
					t.Fatal("context stage face missing")
					return workspace.FeatureSelection{}, ""
				}
				face, key := pick("", func(e modelcore.TopologySelectionEvidence) bool {
					if kind == "SHELL" {
						return math.Abs(e.Centroid[2]-10) < 1e-7
					}
					return math.Abs(e.Centroid[0]+10) < 1e-7 && e.GeometryType == "PLANE"
				})
				independent := x.input(anchor.ID, body)
				if independent.Artifact.GeometryKey == key {
					t.Fatal("fixture did not project 10 to 20 mm")
				}
				f := workspace.Feature{Type: kind, BodyID: body, Length: 2, Angle: 2, Selections: []workspace.FeatureSelection{face}}
				if kind == "DRAFT" {
					neutral, _ := pick(referenceBody, func(e modelcore.TopologySelectionEvidence) bool { return math.Abs(e.Centroid[2]-10) < 1e-7 })
					f.NeutralPlane = &neutral
					if neutral.SourceFeatureID != reference.ID {
						t.Fatal("reference stage lost")
					}
				}
				r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: anchor.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &f, RootProductDocumentID: scope.RootProductDocumentID, RootProductRevisionID: scope.RootProductRevisionID, ContextVariantID: scope.ContextVariantID, InstancePath: scope.InstancePath})
				wrong := r
				wrong.GeometryKey = independent.Artifact.GeometryKey
				x.reject(wrong, "GeometryKey")
				wrong = r
				forged := f
				forged.Selections = append([]workspace.FeatureSelection(nil), f.Selections...)
				forged.Selections[0].Selection.CreationEvidence.Centroid[2] += 10
				wrong.Feature = &forged
				x.reject(wrong, "independent definition evidence")
				wrong = r
				wrong.RootProductRevisionID = source.Document.VersionID
				x.reject(wrong, "CONTEXT_PRODUCT_REVISION_CHANGED")
				wrong = r
				wrong.ContextVariantID = "stale-variant"
				x.reject(wrong, "CONTEXT_VARIANT_CHANGED")
				if f.NeutralPlane != nil {
					wrong = r
					badFeature := f
					badPlane := *f.NeutralPlane
					badPlane.Selection.CreationEvidence.Centroid[2] += 10
					badFeature.NeutralPlane = &badPlane
					wrong.Feature = &badFeature
					x.reject(wrong, "independent definition evidence")
				}

				preview, e := x.s.PreviewCommand(ctx, x.v.Document.ID, r)
				if e != nil || preview.Artifact == nil {
					t.Fatal("projected preview", e)
				}
				cold, e := x.s.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, r)
				if e != nil || cold.Artifact == nil {
					t.Fatal("cold projected preview", e)
				}
				for _, role := range []string{"BREP", "NAMING"} {
					if preview.Artifact.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
						t.Fatal("projected hot/cold mismatch", role)
					}
				}
				unchanged, e := x.s.GetDocument(ctx, x.v.Document.ID, p6Actor)
				if e != nil || unchanged.Document.VersionID != x.v.Document.VersionID {
					t.Fatal("preview wrote Head", e)
				}
				if adopt {
					r.PreviewID = preview.PreviewID
				}
				x.apply(r)
				created := x.v.Part.Features[slices.IndexFunc(x.v.Part.Features, func(f workspace.Feature) bool { return f.ID == anchor.ID })+1]
				if !reflect.DeepEqual(created.Selections, f.Selections) || !reflect.DeepEqual(created.NeutralPlane, f.NeutralPlane) {
					t.Fatal("projected topology/evidence was persisted")
				}
				for _, p := range x.v.Part.Parameters {
					if (p.ParameterID == param || p.ParameterID == referenceParam) && math.Abs(p.EvaluatedValue.SIValue-.01) > 1e-9 {
						t.Fatal("projected value overwrote default")
					}
				}
				if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, workspace.FeatureInputRequest{ContextReadScope: scope, VersionID: x.v.Document.VersionID, AfterFeatureID: anchor.ID, BodyID: body}); e == nil {
					t.Fatal("old occurrence accepted after commit")
				}
			})
		}
	}
}
