package workspace

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestSolvedSketchPreviewCandidatesProjection(t *testing.T) {
	model := PartModel{Features: []Feature{{ID: "target", Sketch: &SketchFeature{Entities: []SketchEntity{{ID: "line", Kind: "LINE", Start: &SketchPoint2{X: 3}}}, Solve: SketchSolveState{Status: "SOLVED"}}}, {ID: "other", Sketch: &SketchFeature{}}}}
	payload, _ := json.Marshal(editSketchPayload{SketchID: "target"})
	command := modelcore.DomainCommand{TypeURI: typeEditSketch, Payload: payload}
	candidates := solvedSketchPreviewCandidates(command, model)
	if len(candidates) != 1 || candidates[0].FeatureID != "target" || candidates[0].Solve.Status != "SOLVED" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
	candidates[0].Entities[0].Start.X = 99
	if model.Features[0].Sketch.Entities[0].Start.X != 3 {
		t.Fatal("candidate aliased source model")
	}
	command.TypeURI = "unrelated"
	if len(solvedSketchPreviewCandidates(command, model)) != 0 {
		t.Fatal("unrelated command exposed sketch candidates")
	}
}

func TestSketchProductionPreviewQuickTrimReadOnly(t *testing.T) {
	for _, kind := range []string{"ELLIPSE", "SPLINE"} {
		t.Run(kind, func(t *testing.T) {
			f := newSketchWorkflowFixture(t)
			id, boundary := uuid.NewString(), uuid.NewString()
			entity := SketchEntity{ID: id, Kind: "ELLIPSE", Center: &SketchPoint2{}, MajorRadius: 5, MinorRadius: 2, Role: "PROFILE"}
			hit := .5
			if kind == "SPLINE" {
				entity = SketchEntity{ID: id, Kind: "SPLINE", Mode: "CONTROL", Degree: 2, Poles: []SketchPoint2{{-5, 0}, {0, 4}, {5, 0}}, Knots: []float64{0, 1}, Multiplicities: []uint32{3, 3}, Weights: []float64{1, .6, 1}, ParameterStart: 0, ParameterEnd: 1, Role: "PROFILE"}
				hit = .8
			}
			before := f.edit(workflowEntity(entity), workflowEntity(SketchEntity{ID: boundary, Kind: "LINE", Start: &SketchPoint2{Y: -10}, End: &SketchPoint2{Y: 10}, Role: "CONSTRUCTION"}))
			history, err := f.service.ListHistory(f.ctx, f.documentID)
			if err != nil {
				t.Fatal(err)
			}
			op := SketchOperation{Type: "QUICK_TRIM", OperationID: uuid.NewString(), EntityIDs: []string{id}, BoundaryIDs: []string{boundary}, HitParameter: &hit, TrimMode: "KEEP_HIT"}
			preview, err := f.service.PreviewCommand(f.ctx, f.documentID, CommandRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: "EDIT_SKETCH", SketchID: f.sketchID, Operations: []SketchOperation{op}})
			if err != nil {
				t.Fatal(err)
			}
			if preview.BaseVersionID != before.Document.VersionID || len(preview.SketchCandidates) != 1 {
				t.Fatalf("missing scoped candidate: %+v", preview)
			}
			candidate := preview.SketchCandidates[0]
			if candidate.FeatureID != f.sketchID || candidate.Solve.Status == "FAILED" || candidate.Solve.Status == "CONFLICTING" {
				t.Fatalf("unsolved candidate: %+v", candidate)
			}
			after, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
			if err != nil {
				t.Fatal(err)
			}
			afterHistory, err := f.service.ListHistory(f.ctx, f.documentID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Document.VersionID != before.Document.VersionID || !reflect.DeepEqual(after.Part, before.Part) || len(history) != len(afterHistory) {
				t.Fatal("preview mutated head/model/history")
			}
			var trimmed *SketchEntity
			for i := range candidate.Entities {
				e := &candidate.Entities[i]
				if e.Role == "PROFILE" && e.Kind == map[string]string{"ELLIPSE": "ELLIPTICAL_ARC", "SPLINE": "SPLINE"}[kind] {
					trimmed = e
					break
				}
			}
			if trimmed == nil {
				t.Fatal("no exact trimmed candidate")
			}
			if kind == "ELLIPSE" && (math.Abs(trimmed.StartAngle-math.Pi/2) > 1e-8 || math.Abs(trimmed.EndAngle-3*math.Pi/2) > 1e-8) {
				t.Fatalf("wrong analytic half ellipse: %+v", trimmed)
			}
			if kind == "SPLINE" {
				if trimmed.Mode != "CONTROL" || len(trimmed.Poles) != 3 || math.Abs(trimmed.Poles[0].X) > 1e-8 || math.Abs(trimmed.Poles[0].Y-1.5) > 1e-8 || math.Abs(trimmed.Poles[2].X-5) > 1e-8 {
					t.Fatalf("rational exact subcurve differs: %+v", trimmed)
				}
			}
			committed := f.edit(op)
			if !reflect.DeepEqual(candidate.Entities, f.sketch(committed).Entities) || !reflect.DeepEqual(candidate.Solve, f.sketch(committed).Solve) {
				t.Fatal("readonly candidate differs from identical production command")
			}
		})
	}
}

func TestSketchProductionPreviewRejectsInvalidExtrusionWithoutWrite(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	circle, boundary := uuid.NewString(), uuid.NewString()
	f.edit(workflowEntity(SketchEntity{ID: circle, Kind: "CIRCLE", Center: &SketchPoint2{}, Radius: 5, Role: "PROFILE"}), workflowEntity(SketchEntity{ID: boundary, Kind: "LINE", Start: &SketchPoint2{Y: -10}, End: &SketchPoint2{Y: 10}, Role: "CONSTRUCTION"}))
	before := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
	history, err := f.service.ListHistory(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	hit := .5
	preview, err := f.service.PreviewCommand(f.ctx, f.documentID, CommandRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: "EDIT_SKETCH", SketchID: f.sketchID, Operations: []SketchOperation{{Type: "QUICK_TRIM", OperationID: uuid.NewString(), EntityIDs: []string{circle}, BoundaryIDs: []string{boundary}, HitParameter: &hit, TrimMode: "KEEP_HIT"}}})
	if err == nil || len(preview.SketchCandidates) != 0 {
		t.Fatal("open profile with dependent extrusion reported successful preview")
	}
	after, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	afterHistory, err := f.service.ListHistory(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Document.VersionID != before.Document.VersionID || !reflect.DeepEqual(after.Part, before.Part) || len(history) != len(afterHistory) {
		t.Fatal("failed preview mutated head/model/history")
	}
}
