//go:build integration

package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSketchDirectionInferenceSolvesThroughRouter(t *testing.T) {
	script, e := filepath.Abs("../../../web/apps/cad/src/cad/testing/sketch-direction-inference.scenario.mjs")
	if e != nil {
		t.Fatal(e)
	}
	command := exec.CommandContext(t.Context(), "node", script)
	command.Dir = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(script))))
	command.Env = append(os.Environ(), "OCCCCAD_SKETCH_INFERENCE_EMIT=1")
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	raw, e := command.Output()
	if e != nil {
		t.Fatal(e, diagnostics.String())
	}
	var cases map[string][]workspace.SketchOperation
	if e = json.Unmarshal(raw, &cases); e != nil {
		t.Fatal(e)
	}
	s, _, _, _, _ := featureAssociationRuntimeService(t)
	view, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Direction inference"})
	if e != nil {
		t.Fatal(e)
	}
	sequence := 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		sequence++
		request.ActorID = p6Actor
		request.RequestID = fmt.Sprintf("%s-inference-%d", view.Document.ID, sequence)
		view, e = s.ApplyCommand(t.Context(), view.Document.ID, request)
		if e != nil {
			t.Fatal(request.Type, e)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketchID := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "stable-line", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: 10, Y: 10}}},
		{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "source-fixed", Kind: "FIXED", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "stable-line", SubElement: "WHOLE"}}}},
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "profile", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 50, Y: 50}, Radius: 5}},
	}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketchID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5})
	initial := activeBodyArtifact(t, view)
	for _, name := range []string{"horizontal", "vertical", "parallel", "perpendicular"} {
		operations := cases[name]
		for _, op := range operations {
			if op.Entity != nil && op.Entity.Role != "CONSTRUCTION" {
				t.Fatal("tool did not preserve construction role")
			}
		}
		request := workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-inference-preview-" + name, Type: "EDIT_SKETCH", SketchID: sketchID, Operations: operations}
		head := view.Document.VersionID
		preview, err := s.PreviewCommand(t.Context(), view.Document.ID, request)
		if err != nil || preview.PreviewID == "" {
			t.Fatal(name, err)
		}
		read, err := s.GetDocument(t.Context(), view.Document.ID, p6Actor)
		if err != nil || read.Document.VersionID != head {
			t.Fatal("direction preview mutated Head", err)
		}
		request.PreviewID = preview.PreviewID
		apply(request)
		sketch := view.Part.Features[0].Sketch
		var added workspace.SketchEntity
		for _, op := range operations {
			if op.Entity != nil {
				for _, entity := range sketch.Entities {
					if entity.ID == op.Entity.ID {
						added = entity
					}
				}
			}
		}
		dx, dy := added.End.X-added.Start.X, added.End.Y-added.Start.Y
		error := math.Abs(dy)
		switch name {
		case "vertical":
			error = math.Abs(dx)
		case "parallel":
			error = math.Abs(dx - dy)
		case "perpendicular":
			error = math.Abs(dx + dy)
		}
		if error > 1e-7 {
			t.Fatal("formal direction was not solved", name, error)
		}
	}
	sketch := view.Part.Features[0].Sketch
	if sketch.Solve.DefinitionStatus != "UNDER_CONSTRAINED" || sketch.Solve.DegreesOfFreedom != 15 || len(sketch.Solve.Components) != 4 {
		t.Fatal("accepted diagnostic components", sketch.Solve)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if activeBodyArtifact(t, view).Representations[role].Digest != initial.Representations[role].Digest {
			t.Fatal("construction inference changed solid", role)
		}
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(view.Part.Features[0].Sketch.Constraints) != 4 {
		t.Fatal("inference undo did not remove one relation")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if len(view.Part.Features[0].Sketch.Constraints) != 5 {
		t.Fatal("inference redo failed")
	}
}
