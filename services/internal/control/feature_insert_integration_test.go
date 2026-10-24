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
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "FACE", LocalID: 1},
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
	x.reject(bad, "only FILLET/CHAMFER")
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
