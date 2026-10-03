package workspace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// Node owns real production tool/input gestures; every emitted operation is
// consumed by the production Go service and matching Worker, not a geometry
// implementation in the scenario. Replies are actual authoritative snapshots.
func TestSketchDirectProductionToolReplay(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	initial, e := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	script, e := filepath.Abs("../../../web/apps/cad/src/cad/testing/sketch-direct-workflow.scenario.mjs")
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.CommandContext(f.ctx, "node", script)
	cmd.Dir = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(script))))
	cmd.Env = append(os.Environ(), "OCCCCAD_SKETCH_REPLAY=1")
	input, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	output, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	var errors bytes.Buffer
	cmd.Stderr = &errors
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	encoder := json.NewEncoder(input)
	if e = encoder.Encode(initial); e != nil {
		t.Fatal(e)
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	latest := initial
	operationCount := 0
	previewCount := 0
	finished := false
	holeArea := math.NaN()
	padDepth := 0.0
	for scanner.Scan() {
		var message struct {
			Kind       string            `json:"kind"`
			Type       string            `json:"type"`
			FeatureID  string            `json:"featureId"`
			Operations []SketchOperation `json:"operations"`
			Intent     struct {
				RequestID     string `json:"requestId"`
				BaseVersionID string `json:"baseVersionId"`
			} `json:"intent"`
			Request        CommandRequest `json:"request"`
			ExpectedVolume float64        `json:"expectedVolume"`
		}
		if e = json.Unmarshal(scanner.Bytes(), &message); e != nil {
			t.Fatalf("invalid Node protocol %q: %v; stderr=%s", scanner.Text(), e, errors.String())
		}
		kind := message.Kind
		if kind == "" {
			kind = message.Type
		}
		request := CommandRequest{RequestID: message.Intent.RequestID, ActorID: f.actor, Type: "EDIT_SKETCH", SketchID: message.FeatureID, Operations: message.Operations}
		if request.RequestID == "" {
			request.RequestID = uuid.NewString()
		}
		if request.SketchID == "" {
			request.SketchID = f.sketchID
		}
		var response any
		switch kind {
		case "operation":
			operationCount++
			for _, operation := range request.Operations {
				if operation.Type == "MIRROR_ENTITIES" && len(operation.EntityIDs) == 1 {
					for _, entity := range f.sketch(latest).Entities {
						if entity.ID == operation.EntityIDs[0] && entity.Kind == "ARC" {
							theta := math.Abs(entity.EndAngle - entity.StartAngle)
							holeArea = entity.Radius * entity.Radius * (theta - math.Sin(theta))
						}
					}
				}
			}
			updated, err := f.service.ApplyCommand(f.ctx, f.documentID, request)
			if err != nil {
				response = map[string]string{"error": err.Error()}
				raw, _ := json.Marshal(message)
				t.Logf("production replay rejected %s: %v", raw, err)
			} else {
				latest = updated
				response = updated
				solve := f.sketch(updated).Solve
				if solve.Status == "CONFLICTING" || solve.Status == "FAILED" || solve.Status == "INVALID_MODEL" {
					t.Fatalf("accepted tool operation has unresolved authoritative solve: operations=%+v solve=%+v", request.Operations, solve)
				}
			}
		case "preview":
			previewCount++
			preview, err := f.service.PreviewCommand(f.ctx, f.documentID, request)
			if err != nil {
				response = map[string]string{"error": err.Error()}
			} else {
				response = preview
			}
		case "command":
			request = message.Request
			if request.Type == "PAD_SKETCH" {
				padDepth = request.Length
			}
			request.ActorID = f.actor
			if request.RequestID == "" {
				request.RequestID = uuid.NewString()
			}
			if request.SketchID == "" {
				request.SketchID = f.sketchID
			}
			updated, err := f.service.ApplyCommand(f.ctx, f.documentID, request)
			if err != nil {
				response = map[string]string{"error": err.Error()}
			} else {
				latest = updated
				response = updated
			}
		case "exit", "done":
			finished = true
			response = map[string]bool{"ok": true}
			if message.ExpectedVolume > 0 {
				f.volume(latest, message.ExpectedVolume)
			}
		default:
			t.Fatalf("unknown Node protocol %q; stderr=%s", kind, errors.String())
		}
		_ = os.MkdirAll("../../../build/direct-sketch", 0755)
		trace, traceErr := os.OpenFile("../../../build/direct-sketch/go-tool-replay-"+f.documentID+".jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if traceErr == nil {
			_ = json.NewEncoder(trace).Encode(map[string]any{"message": message, "response": response})
			_ = trace.Close()
		}
		if e = encoder.Encode(response); e != nil {
			t.Fatal(e)
		}
		if finished {
			// The Node readline iterator owns stdin. Closing it after the final
			// acknowledgement lets Node finish without waiting on this scanner.
			_ = input.Close()
		}
	}
	_ = input.Close()
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if e = cmd.Wait(); e != nil {
		t.Fatalf("production tool replay: %v; stderr=%s", e, errors.String())
	}
	if !finished || operationCount < 10 {
		t.Fatalf("incomplete real tool replay operations=%d previews=%d; stderr=%s", operationCount, previewCount, errors.String())
	}
	// The source arc defines the exact reflected lens hole and four R5 corners. This oracle
	// is independent of the tool's operation/geometry counts and Worker success.
	if math.IsNaN(holeArea) || padDepth <= 0 {
		t.Fatal("actual arc mirror and pad intent were not observed")
	}
	want := padDepth * (90*50 - (4-math.Pi)*25 - holeArea)
	f.volume(latest, want)
	if len(latest.Part.Bodies) != 1 {
		t.Fatal("replay changed per-Body ownership")
	}
	for _, artifact := range latest.Artifacts {
		if fmt.Sprint(artifact.Topology["solids"]) != "1" {
			t.Fatalf("replay is not one valid solid: %+v", artifact.Topology)
		}
	}
	t.Logf("real tool replay operations=%d previews=%d volume=%.9f", operationCount, previewCount, want)
}
