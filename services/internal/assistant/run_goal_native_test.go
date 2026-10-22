package assistant

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
)

func TestAssistantGoalCoverageNativeCubeHoleAndLocalFillet(t *testing.T) {
	for _, entry := range []string{"natural", "cad_finish"} {
		t.Run(entry, func(t *testing.T) {
			s, domain, view, r := stageFixture(t)
			body := view.Part.ActiveBodyID
			var firstRaw []byte
			if err := s.db.QueryRow(t.Context(), `SELECT payload FROM occccad.assistant_inputs WHERE run_id=$1 AND sequence=1`, r.ID).Scan(&firstRaw); err != nil {
				t.Fatal(err)
			}
			var first Turn
			_ = json.Unmarshal(firstRaw, &first)
			first.Text = "在XY平面建立100mm立方体，再对全边做R10圆角"
			if _, err := s.db.Exec(t.Context(), `UPDATE occccad.assistant_inputs SET payload=$1 WHERE id=$2`, marshalJSON(first), first.ID); err != nil {
				t.Fatal(err)
			}
			var err error
			r, err = s.mutateRun(t.Context(), r.ID, func(_ database.Tx, current *Run) error {
				current.Goal = first.Text
				current.Requirements = reviseRequirements(nil, first)
				current.MessageSequence = 1
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			refresh := func() {
				t.Helper()
				var e error
				r, e = loadRun(t.Context(), s.db, r.ID, false)
				if e != nil {
					t.Fatal(e)
				}
			}
			// Each stage travels through the actual model-facing planning adapter, then
			// the existing Automation preview, authority evaluator and CAS commit.
			apply := func(op string, args any, source *QueryResultReference) string {
				t.Helper()
				desc := toolMessage("cad_describe_operation", map[string]any{"operationId": op})
				dc := controlledToolCall(t, s, r, desc)
				dr := s.executeTool(t.Context(), r, dc, desc.ToolCalls[0])
				if dr.IsError {
					t.Fatal(string(dr.Content))
				}
				if _, e := s.recordTool(t.Context(), r, &dc, 0, desc.ToolCalls[0], dr); e != nil {
					t.Fatal(e)
				}
				refresh()
				step := plannedStep{StepSpec: automation.StepSpec{ID: "step", OperationID: op, Version: 1, Arguments: marshalJSON(args)}}
				if source != nil {
					step.ResultBindings = []QueryResultBinding{{Target: "selectionSet", Source: *source}}
				}
				message := toolMessage("cad_create_task", map[string]any{"steps": []plannedStep{step}})
				c := controlledToolCall(t, s, r, message)
				task, e := s.createStage(t.Context(), r, c, message.ToolCalls[0])
				if e != nil {
					t.Fatal(op, e)
				}
				_, e = s.recordTool(t.Context(), r, &c, 0, message.ToolCalls[0], Result{CallID: message.ToolCalls[0].ID, Content: marshalJSON(task)})
				if e != nil {
					t.Fatal(e)
				}
				if source != nil {
					trace, e := s.CallTrace(t.Context(), r.ActorID, r.DocumentID, r.ID, c.ID)
					if e != nil || len(trace.Tools) != 1 || string(trace.Tools[0].Arguments) != message.ToolCalls[0].Function.Arguments || !bytes.Contains(trace.Tools[0].Arguments, []byte("resultBindings")) {
						t.Fatal("binding mutated original model tool arguments", e)
					}
				}
				refresh()
				r = finishStageForTest(t, s, r)
				done, e := s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, task.ID)
				if e != nil || done.Steps[0].Result == nil || done.Steps[0].Result.Output == nil {
					t.Fatal(done, e)
				}
				return done.Steps[0].Result.Output.ID
			}
			sketch := apply("sketch.create", map[string]any{"bodyId": body, "plane": "XY"}, nil)
			apply("sketch.rectangle", map[string]any{"sketchId": sketch, "width": 100, "height": 100, "unit": "mm"}, nil)
			apply("part.extrude", map[string]any{"bodyId": body, "sketchId": sketch, "length": 100, "unit": "mm"}, nil)
			baseRef, baseQuery := recordedQuery(t, s, r, automation.TopologyQuery{RevisionID: r.LatestRevisionID, BodyID: body, Kind: "EDGE", Limit: 1})
			refresh()
			baseFillet := apply("part.fillet", map[string]any{"bodyId": body, "size": 10, "unit": "mm"}, &baseRef)
			before, e := s.verifyGoal(t.Context(), r)
			if e != nil || !completeGoalCoverage(before) {
				t.Fatal("base goal incomplete", before, e)
			}
			// A new GoalVersion with two distinct clauses must retain the base requirement.
			r, e = s.mutateRun(t.Context(), r.ID, func(tx database.Tx, current *Run) error {
				current.Epoch++
				current.GoalVersion++
				current.MessageSequence++
				turn := Turn{ID: uuid.NewString(), RunID: r.ID, Sequence: current.MessageSequence, GoalVersion: current.GoalVersion, Kind: "STEER", Status: "APPLIED", Text: "底面中心直径40通孔，再给孔口两圆边适当倒圆角", Scope: current.Scope, CreatedAt: timestamp(s.now())}
				turn.Scope.RevisionID = current.LatestRevisionID
				current.Scope = turn.Scope
				current.Goal = turn.Text
				current.Requirements = reviseRequirements(current.Requirements, turn)
				current.Verification = nil
				_, e := tx.Exec(t.Context(), `INSERT INTO occccad.assistant_inputs(id,run_id,sequence,request_id,request_digest,kind,status,payload) VALUES($1,$2,$3,$4,$5,'STEER','APPLIED',$6)`, turn.ID, r.ID, turn.Sequence, uuid.NewString(), "native-goal-steer", marshalJSON(turn))
				return e
			})
			if e != nil {
				t.Fatal(e)
			}
			checksAt := map[string]*GoalVerification{}
			checkIncomplete := func(phase string) {
				t.Helper()
				v, e := s.verifyGoal(t.Context(), r)
				if e != nil || v.Status == "VERIFIED" || completeGoalCoverage(v) {
					t.Fatal("old/partial checks completed a new intent", phase, v, e)
				}
				checksAt[phase] = v
				// Both public completion paths use exactly the same coverage gate.
				natural := controlledToolCall(t, s, r, Message{Role: "assistant", Content: "已经完成。"})
				if e = s.processCall(t.Context(), r, natural, nil); e != nil {
					t.Fatal(e)
				}
				refresh()
				if r.Status == "COMPLETED" {
					t.Fatal("natural reply bypassed coverage", phase)
				}
				message := toolMessage("cad_finish", map[string]any{"summary": "已完成"})
				c := controlledToolCall(t, s, r, message)
				result := s.executeTool(t.Context(), r, c, message.ToolCalls[0])
				if !result.IsError || !bytes.Contains(result.Content, []byte("AI_GOAL_UNVERIFIED")) {
					t.Fatal("cad_finish bypassed coverage", phase, string(result.Content))
				}
				refresh()
			}
			checkIncomplete("missing-hole-and-local")
			holeSketch := apply("sketch.create", map[string]any{"bodyId": body, "plane": "XY"}, nil)
			apply("sketch.circle", map[string]any{"sketchId": holeSketch, "centerX": 50, "centerY": 50, "diameter": 40, "unit": "mm"}, nil)
			hole := apply("part.cut", map[string]any{"bodyId": body, "sketchId": holeSketch, "extent": "THROUGH_ALL", "unit": "mm"}, nil)
			checkIncomplete("missing-local")
			// A forged binding to the old, valid R10 feature cannot cover the new local
			// target even though both are FILLET and its radius was delegated.
			localID := ""
			for _, req := range r.Requirements {
				if req.Kind == "HOLE_RIM_FILLET" {
					localID = req.ID
				}
			}
			if localID == "" {
				t.Fatal("local intent disappeared")
			}
			r, e = s.mutateRun(t.Context(), r.ID, func(_ database.Tx, current *Run) error {
				if current.Targets == nil {
					current.Targets = map[string]string{}
				}
				current.Targets[goalTargetKey(localID)] = baseFillet
				return nil
			})
			if e != nil {
				t.Fatal(e)
			}
			checkIncomplete("wrong-global-fillet-target")
			r, e = s.mutateRun(t.Context(), r.ID, func(_ database.Tx, current *Run) error { delete(current.Targets, goalTargetKey(localID)); return nil })
			if e != nil {
				t.Fatal(e)
			}
			holeRadius := 20.0
			_, facesResult := recordedQuery(t, s, r, automation.TopologyQuery{RevisionID: r.LatestRevisionID, BodyID: body, Kind: "FACE", Filter: &automation.TopologyFilter{GeometryType: "CYLINDER", Radius: &automation.TopologyRadiusFilter{EqualMM: &holeRadius, ToleranceMM: 1e-6}}, Limit: 20})
			var faces struct {
				Items []automation.TopologyObservation `json:"items"`
			}
			_ = json.Unmarshal(facesResult.Content, &faces)
			wall := ""
			for _, face := range faces.Items {
				if face.Geometry.MaterialSide != nil && *face.Geometry.MaterialSide == -1 {
					wall = face.ID
				}
			}
			if wall == "" {
				t.Fatal("fixture cylinder not observed")
			}
			rimRef, rimResult := recordedQuery(t, s, r, automation.TopologyQuery{RevisionID: r.LatestRevisionID, BodyID: body, Kind: "EDGE", Limit: 1, Filter: &automation.TopologyFilter{GeometryType: "CIRCLE", Radius: &automation.TopologyRadiusFilter{EqualMM: &holeRadius, ToleranceMM: 1e-6}, AdjacentFaceID: wall}})
			refresh()
			var rimPage map[string]any
			_ = json.Unmarshal(rimResult.Content, &rimPage)
			if rimPage["matchedCount"] != float64(2) || rimPage["returnedCount"] != float64(1) || rimPage["selectionSetCount"] != float64(2) {
				t.Fatal(rimPage)
			}
			// Recover a new application instance, with no provider/credential setup.
			recovered := New(s.db, s.automation, Options{KeyFile: filepath.Join(t.TempDir(), "recovered-key")})
			t.Cleanup(recovered.Close)
			s = recovered
			// Reference corruption/staleness are distinct from a real geometry failure.
			snapshot, _ := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
			tooLarge := plannedStep{StepSpec: automation.StepSpec{OperationID: "part.fillet", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": body, "size": 1000, "unit": "mm"})}, ResultBindings: []QueryResultBinding{{Target: "selectionSet", Source: rimRef}}}
			if e = s.bindQueryResult(t.Context(), r, snapshot.CommandBaseline, &tooLarge); e != nil {
				t.Fatal(e)
			}
			_, geometryError := s.automation.Prepare(t.Context(), r.ActorID, r.DocumentID, automation.ActionRequest{OperationID: "part.fillet", Version: 1, Arguments: tooLarge.Arguments, RequestID: uuid.NewString(), ExpectedHead: snapshot.CommandBaseline})
			if geometryError == nil || strings.Contains(geometryError.Error(), "QUERY_RESULT_REF") {
				t.Fatal("geometry failure was confused with reference failure", geometryError)
			}
			afterFailure, e := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
			if e != nil || afterFailure.RevisionID != snapshot.RevisionID {
				t.Fatal("failed geometry changed Head", e)
			}
			localFillet := apply("part.fillet", map[string]any{"bodyId": body, "size": 2, "unit": "mm"}, &rimRef)
			verified, e := s.verifyGoal(t.Context(), r)
			if e != nil || !completeGoalCoverage(verified) {
				t.Fatal("complete continuous goal rejected", verified, e)
			}
			// A caller-provided verification collection must agree with the bound
			// hole, rather than replace its required rim collection.
			accept := automation.PartCheck{ID: "rim-collection", Kind: "HOLE_RIM_FILLET", BodyID: body, FeatureID: localFillet, ReferenceFeatureID: hole, RadiusPolicy: "DELEGATED", Unit: "mm", SelectionSet: rimPage["selectionSet"].(string)}
			good, e := s.automation.VerifyPart(t.Context(), r.ActorID, r.DocumentID, automation.PartVerificationRequest{RevisionID: r.LatestRevisionID, GoalVersion: r.GoalVersion, Checks: []automation.PartCheck{accept}})
			if e != nil || !good.Passed {
				t.Fatal("matching rim collection rejected", good, e)
			}
			wrongCollection, e := s.automation.QueryTopology(t.Context(), r.ActorID, r.DocumentID, automation.TopologyQuery{RevisionID: snapshot.RevisionID, BodyID: body, Kind: "EDGE", Limit: 1, Filter: &automation.TopologyFilter{GeometryType: "LINE"}})
			if e != nil || wrongCollection["matchedCount"].(int) == 0 {
				t.Fatal("fixture has no independent non-rim collection", e)
			}
			accept.SelectionSet = wrongCollection["selectionSet"].(string)
			bad, e := s.automation.VerifyPart(t.Context(), r.ActorID, r.DocumentID, automation.PartVerificationRequest{RevisionID: r.LatestRevisionID, GoalVersion: r.GoalVersion, Checks: []automation.PartCheck{accept}})
			if e != nil || bad.Passed || !strings.Contains(bad.Checks[0].Message, "differs from the bound hole") {
				t.Fatal("verification collection replaced semantic target", bad, e)
			}
			var baseCount, localCount int
			for _, req := range verified.requirements {
				if req.Kind == "ALL_EDGES_FILLET" {
					baseCount++
					if req.Radius != 10 || req.SourceGoalVersion != 1 {
						t.Fatal("base intent overwritten", req)
					}
				}
				if req.Kind == "HOLE_RIM_FILLET" {
					localCount++
					if req.Radius != 0 || req.RadiusPolicy != "DELEGATED" {
						t.Fatal("observed radius became hard expectation", req)
					}
				}
			}
			if baseCount != 1 || localCount != 1 {
				t.Fatal("fillet requirements collapsed")
			}
			for _, c := range verified.Checks {
				if c.ID == localID {
					if _, exists := c.Expected["radius"]; exists {
						t.Fatal("radius was backfilled")
					}
					if c.Observed["designChoice"] == nil || c.TargetID != localFillet {
						t.Fatal("missing delegated choice evidence", c)
					}
				}
			}
			checksAt["complete"] = verified
			// Clearing cached bindings must reconstruct the same targets from receipts.
			rebuild := r
			rebuild.Targets = map[string]string{}
			rebuild.TargetSelections = map[string]string{}
			restored, e := s.verifyGoal(t.Context(), rebuild)
			if e != nil || !completeGoalCoverage(restored) {
				t.Fatal("receipt recovery changed goal meaning", restored, e)
			}
			messages, _, e := s.buildContext(t.Context(), r, nil)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(messages[1].Content, rimPage["selectionSet"].(string)) || !strings.Contains(messages[1].Content, "selectionSetRef") {
				t.Fatal("context repeated the collection or lost its reference")
			}
			if !strings.Contains(messages[1].Content, "resultBindings") {
				t.Fatal("stage summary lost original query bindings after recovery")
			}
			var contextPacket map[string]json.RawMessage
			_ = json.Unmarshal([]byte(messages[1].Content), &contextPacket)
			legacyFacts := []map[string]any{}
			factRows, e := s.db.Query(t.Context(), `SELECT t.call_id::text,t.name,c.epoch,t.arguments,t.payload FROM occccad.assistant_tool_results t JOIN occccad.assistant_model_calls c ON c.id=t.call_id WHERE c.run_id=$1 ORDER BY c.started_at DESC,c.id DESC,t.ordinal DESC LIMIT 8`, r.ID)
			if e != nil {
				t.Fatal(e)
			}
			for factRows.Next() {
				var callID, name string
				var epoch uint64
				var args, raw []byte
				if e = factRows.Scan(&callID, &name, &epoch, &args, &raw); e != nil {
					t.Fatal(e)
				}
				var result Result
				if e = json.Unmarshal(raw, &result); e != nil {
					t.Fatal(e)
				}
				legacyFacts = append(legacyFacts, map[string]any{"callId": callID, "name": name, "sourceEpoch": epoch, "arguments": json.RawMessage(args), "result": result.Content})
			}
			if e = factRows.Err(); e != nil {
				t.Fatal(e)
			}
			factRows.Close()
			fullFactsBytes, compactFactsBytes := len(marshalJSON(legacyFacts)), len(contextPacket["recentFacts"])
			if compactFactsBytes >= fullFactsBytes {
				t.Fatal("same-ledger context projection did not shrink", fullFactsBytes, compactFactsBytes)
			}
			if entry == "natural" {
				c := controlledToolCall(t, s, r, Message{Role: "assistant", Content: "100 mm基体、全边R10、中心直径40通孔和孔口局部圆角已完成。"})
				if e = s.processCall(t.Context(), r, c, nil); e != nil {
					t.Fatal(e)
				}
			} else {
				message := toolMessage("cad_finish", map[string]any{"summary": "已完成全部要求"})
				c := controlledToolCall(t, s, r, message)
				result := s.executeTool(t.Context(), r, c, message.ToolCalls[0])
				if result.IsError {
					t.Fatal(string(result.Content))
				}
			}
			refresh()
			if r.Status != "COMPLETED" {
				t.Fatal("completion entry failed", entry, r.Status)
			}
			current, e := domain.GetDocument(t.Context(), r.DocumentID)
			if e != nil {
				t.Fatal(e)
			}
			picks := 0
			for _, f := range current.Part.Features {
				if f.ID == localFillet {
					picks = len(f.Selections)
				}
			}
			if picks != 2 {
				t.Fatal("local fillet did not use complete query collection", picks)
			}
			var sent int
			if e = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_model_calls WHERE run_id=$1 AND status='SENT'`, r.ID).Scan(&sent); e != nil || sent != 0 || r.ModelCalls != 0 {
				t.Fatal("native baseline called a model", sent, r.ModelCalls, e)
			}
			report := map[string]any{"documentId": r.DocumentID, "entry": entry, "baseFilletId": baseFillet, "holeId": hole, "localFilletId": localFillet, "baseQuery": baseQuery, "rimQuery": rimResult, "rimReference": rimRef, "checks": checksAt, "geometryFailure": geometryError.Error(), "contextBytes": len(messages[1].Content), "fullRecentFactsBytes": fullFactsBytes, "compactRecentFactsBytes": compactFactsBytes, "queryTokenBytes": len(rimPage["selectionSet"].(string)), "bindingBytes": len(marshalJSON(QueryResultBinding{Target: "selectionSet", Source: rimRef})), "modelCalls": r.ModelCalls}
			path, _ := filepath.Abs("../../../build/agent-check/goal-result-bindings/native-" + entry + ".json")
			if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(path, marshalJSON(report), 0644); e != nil {
				t.Fatal(e)
			}
			t.Logf("100 mm cube → all edges R10 → central Ø40 through → two rims R2 (delegated); entry %s, reference %d B, token %d B; evidence %s", entry, len(marshalJSON(rimRef)), len(rimPage["selectionSet"].(string)), path)
		})
	}
}
