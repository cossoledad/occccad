package workspace

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestSketchDirectRoundedRectangleSplitArcMirrorHole(t *testing.T) {
	for _, variant := range []struct {
		name           string
		cuts           []float64
		reverseCorners bool
		deleteSource   bool
		rotated        bool
	}{{"diameter", []float64{0, .5}, false, false, false}, {"non_seam_reversed", []float64{.62, .14}, true, false, false}, {"delete_source_major_arc", []float64{.62, .14}, false, true, false}, {"rotated_oriented", []float64{.62, .14}, true, false, true}} {
		t.Run(variant.name, func(t *testing.T) {
			f := newSketchWorkflowFixture(t)
			origin := SketchGeometryRef{Target: "SKETCH_ORIGIN", SubElement: "POINT"}
			ids := map[string]string{}
			var v DocumentView
			rotate := func(p SketchPoint2) SketchPoint2 { return p }
			if variant.rotated {
				// Oriented creation uses ordinary geometry and formal relationships,
				// exactly as the production compound-creation contract requires.
				seed := uuid.NewString()
				operations, err := rectangleMacro(seed, SketchPoint2{}, SketchPoint2{X: 100, Y: 60})
				if err != nil {
					t.Fatal(err)
				}
				angle := 37 * math.Pi / 180
				rotate = func(p SketchPoint2) SketchPoint2 {
					return SketchPoint2{X: p.X*math.Cos(angle) - p.Y*math.Sin(angle), Y: p.X*math.Sin(angle) + p.Y*math.Cos(angle)}
				}
				for _, name := range []string{"bottom", "right", "top", "left"} {
					ids[name] = macroID(seed, "line-"+name)
				}
				creation := []SketchOperation{}
				for _, op := range operations {
					if op.Entity != nil {
						a, b := rotate(*op.Entity.Start), rotate(*op.Entity.End)
						op.Entity.Start, op.Entity.End = &a, &b
					}
					if op.Constraint != nil && op.Constraint.Kind == "PARALLEL" {
						continue
					}
					creation = append(creation, op)
				}
				creation = append(creation,
					workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "PARALLEL", References: []SketchGeometryRef{workflowRef(ids["bottom"], "DIRECTION"), workflowRef(ids["top"], "DIRECTION")}}),
					workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "PARALLEL", References: []SketchGeometryRef{workflowRef(ids["right"], "DIRECTION"), workflowRef(ids["left"], "DIRECTION")}}),
					workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "PERPENDICULAR", References: []SketchGeometryRef{workflowRef(ids["bottom"], "DIRECTION"), workflowRef(ids["right"], "DIRECTION")}}),
					workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "ANGLE", References: []SketchGeometryRef{{Target: "SKETCH_X_AXIS", SubElement: "DIRECTION"}, workflowRef(ids["bottom"], "DIRECTION")}, Value: workflowValue(37), Unit: "deg"}),
					workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(ids["bottom"], "START"), origin}}))
				v = f.edit(creation...)
				for _, id := range ids {
					found := false
					for _, e := range f.sketch(v).Entities {
						if e.ID == id {
							found = true
						}
					}
					if !found {
						t.Fatal("created stable oriented edge identity missing")
					}
				}
			} else {
				v = f.edit(SketchOperation{Type: "ADD_RECTANGLE", First: &SketchPoint2{}, Second: &SketchPoint2{X: 100, Y: 60}, FirstReference: &origin})
			}
			for _, e := range f.sketch(v).Entities {
				if variant.rotated {
					break
				}
				if e.Start.Y == 0 && e.End.Y == 0 {
					ids["bottom"] = e.ID
				}
				if e.Start.X == 100 && e.End.X == 100 {
					ids["right"] = e.ID
				}
				if e.Start.Y == 60 && e.End.Y == 60 {
					ids["top"] = e.ID
				}
				if e.Start.X == 0 && e.End.X == 0 {
					ids["left"] = e.ID
				}
			}
			corners := [][4]string{{"bottom", "right", "END", "START"}, {"right", "top", "END", "START"}, {"top", "left", "END", "START"}, {"left", "bottom", "END", "START"}}
			if variant.reverseCorners {
				for i, j := 0, len(corners)-1; i < j; i, j = i+1, j-1 {
					corners[i], corners[j] = corners[j], corners[i]
				}
			}
			v = f.command(CommandRequest{Type: "CREATE_PARAMETER", Name: "cornerRadius", Value: 5, Unit: "mm"})
			source := "cornerRadius"
			for _, corner := range corners {
				op := cornerOperation("FILLET_ENTITIES", uuid.NewString(), ids[corner[0]], ids[corner[1]], corner[2], corner[3])
				op.Value = workflowValue(99)
				op.ParameterSource = &source
				v = f.edit(op)
			}
			visible := func(s *SketchFeature, sourceID string) string {
				for _, e := range s.Entities {
					if e.Kind == "LINE" && e.Role == "PROFILE" && e.SourceEntityID == sourceID {
						return e.ID
					}
				}
				t.Fatal("missing visible line child")
				return ""
			}
			bottom, top := visible(f.sketch(v), ids["bottom"]), visible(f.sketch(v), ids["top"])
			expressionCount := 0
			for _, parameter := range v.Part.Parameters {
				if parameter.Source.Expression != nil && parameter.Source.Expression.SourceText == "cornerRadius" {
					expressionCount++
				}
			}
			if expressionCount != 4 {
				t.Fatalf("corner source AST not persisted on four dimensions: %d", expressionCount)
			}
			for _, e := range f.sketch(v).Entities {
				if e.Kind == "ARC" && e.Radius != 5 {
					t.Fatalf("formula source did not produce R5 geometry: %+v", e)
				}
			}
			length, gap := uuid.NewString(), uuid.NewString()
			v = f.edit(workflowConstraint(SketchConstraint{ID: length, Kind: "LENGTH", References: []SketchGeometryRef{workflowRef(bottom, "WHOLE")}, Value: workflowValue(80), Unit: "mm"}))
			v = f.edit(workflowConstraint(SketchConstraint{ID: gap, Kind: "DISTANCE", References: []SketchGeometryRef{workflowRef(bottom, "WHOLE"), workflowRef(top, "WHOLE")}, Value: workflowValue(50), Unit: "mm"}))
			circle := uuid.NewString()
			circleCenter := rotate(SketchPoint2{X: 45, Y: 25})
			v = f.edit(workflowEntity(SketchEntity{ID: circle, Kind: "CIRCLE", Center: &circleCenter, Radius: 10, Role: "PROFILE"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(circle, "CENTER")}, FixedPoint: &circleCenter}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "RADIUS", References: []SketchGeometryRef{workflowRef(circle, "WHOLE")}, Value: workflowValue(10), Unit: "mm"}))
			v = f.edit(SketchOperation{Type: "SPLIT_ENTITY", OperationID: uuid.NewString(), EntityIDs: []string{circle}, Parameters: variant.cuts})
			var parts []SketchEntity
			for _, e := range f.sketch(v).Entities {
				if e.ID == circle || e.SourceEntityID == circle {
					parts = append(parts, e)
				}
			}
			if len(parts) != 2 {
				t.Fatalf("two circle picks produced %d arcs", len(parts))
			}
			arc := parts[0]
			other := parts[1]
			if arc.ID != circle {
				arc, other = other, arc
			}
			if variant.deleteSource {
				arc, other = other, arc
			}
			v = f.edit(SketchOperation{Type: "QUICK_TRIM", OperationID: uuid.NewString(), EntityIDs: []string{other.ID}, BoundaryIDs: []string{arc.ID}, HitParameter: workflowValue(.5), TrimMode: "DELETE_HIT"})
			chord := uuid.NewString()
			a := SketchPoint2{X: arc.Center.X + arc.Radius*math.Cos(arc.StartAngle), Y: arc.Center.Y + arc.Radius*math.Sin(arc.StartAngle)}
			b := SketchPoint2{X: arc.Center.X + arc.Radius*math.Cos(arc.EndAngle), Y: arc.Center.Y + arc.Radius*math.Sin(arc.EndAngle)}
			v = f.edit(workflowEntity(SketchEntity{ID: chord, Kind: "LINE", Role: "CONSTRUCTION", Start: &a, End: &b}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(chord, "START"), workflowRef(arc.ID, "START")}}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(chord, "END"), workflowRef(arc.ID, "END")}}))
			axis := workflowRef(chord, "DIRECTION")
			v = f.edit(SketchOperation{Type: "MIRROR_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{arc.ID}, Axis: &axis, MirrorMode: "LINKED"})
			sweep := arc.EndAngle - arc.StartAngle
			hole := 100 * (sweep - math.Sin(sweep))
			want := 2 * (90*50 - (4-math.Pi)*25 - hole)
			for _, feature := range v.Part.Features {
				if feature.ID == f.sketchID {
					regions, e := f.service.buildExactProfileRegions(f.ctx, feature, uuid.NewString())
					if e != nil {
						t.Fatal(e)
					}
					dump, _ := json.MarshalIndent(regions, "", "  ")
					_ = os.MkdirAll("../../../build/direct-sketch", 0755)
					_ = os.WriteFile("../../../build/direct-sketch/regions-"+variant.name+"-"+f.documentID+".json", dump, 0600)
				}
			}
			dump, _ := json.MarshalIndent(v.Part, "", "  ")
			_ = os.MkdirAll("../../../build/direct-sketch", 0755)
			_ = os.WriteFile("../../../build/direct-sketch/model-"+variant.name+"-"+f.documentID+".json", dump, 0600)
			v = f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
			f.volume(v, want)
			if len(v.Part.Bodies) != 1 {
				t.Fatal("hole generated an extra Body")
			}
			for _, artifact := range v.Artifacts {
				if artifact.Topology["solids"] != float64(1) {
					t.Fatalf("expected one solid: %+v", artifact.Topology)
				}
			}
			saved, _ := json.Marshal(f.sketch(v))
			params := append([]byte(nil), saved...)
			f.command(CommandRequest{Type: "UNDO"})
			v = f.command(CommandRequest{Type: "REDO"})
			f.volume(v, want)
			cold := NewWithArtifacts(f.service.database, f.service.worker, f.service.artifacts)
			opened, e := cold.GetDocument(f.ctx, f.documentID, f.actor)
			if e != nil {
				t.Fatal(e)
			}
			f.volume(opened, want)
			var radiusParameter string
			for _, p := range opened.Part.Parameters {
				if p.Key == "cornerRadius" {
					radiusParameter = p.ParameterID
				}
			}
			var host DocumentView
			if variant.deleteSource {
				var err error
				host, err = f.service.CreateDocument(f.ctx, CreateDocumentRequest{RequestID: uuid.NewString(), Type: "PRODUCT", Name: "rounded-sketch-host-" + uuid.NewString(), ActorID: f.actor})
				if err != nil {
					t.Fatal(err)
				}
				host, err = f.service.ApplyCommand(f.ctx, host.Document.ID, CommandRequest{RequestID: uuid.NewString(), Type: "INSERT_INSTANCE", ReferencedDocumentID: f.documentID, ActorID: f.actor})
				if err != nil {
					t.Fatal(err)
				}
				if len(host.ResolvedInstances) != 1 || host.ResolvedInstances[0].DocumentID != f.documentID || len(host.ResolvedInstances[0].OwnedSketchIDs) != 1 || host.ResolvedInstances[0].OwnedSketchIDs[0] != f.sketchID {
					t.Fatal("Product lost explicit Part sketch ownership")
				}
			}
			updated := f.command(CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: radiusParameter, Value: 5.5, Unit: "mm"})
			if variant.deleteSource {
				reread, err := f.service.GetDocument(f.ctx, host.Document.ID, f.actor)
				if err != nil {
					t.Fatal(err)
				}
				if host.Document.VersionID != reread.Document.VersionID || len(reread.ResolvedInstances) != 1 || reread.ResolvedInstances[0].OccurrencePath != host.ResolvedInstances[0].OccurrencePath || reread.ResolvedInstances[0].OwnedSketchIDs[0] != f.sketchID {
					t.Fatal("editing Part sketch changed host or stable occurrence identity")
				}
			}
			f.volume(updated, 2*((80+11)*50-(4-math.Pi)*5.5*5.5-hole))
			restored, _ := json.Marshal(f.sketch(opened))
			if !reflect.DeepEqual(params, restored) {
				t.Fatal("undo/redo/cold read changed stable sketch data")
			}
		})
	}
}
