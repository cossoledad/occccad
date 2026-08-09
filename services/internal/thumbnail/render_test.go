package thumbnail

import (
	"bytes"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestRenderPartUsesMeshGeometry(t *testing.T) {
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Name: "Bracket", Type: "PART"},
		Artifact: &workspace.Artifact{GeometryKey: "shape-a", Mesh: workspace.Mesh{
			Vertices: [][3]float64{{0, 0, 0}, {20, 0, 0}, {0, 10, 8}}, Triangles: [][3]uint32{{0, 1, 2}},
		}}}
	result, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte("<polygon")) || !bytes.Contains(result, []byte("Bracket")) {
		t.Fatalf("rendered SVG does not contain projected geometry: %s", result)
	}
}

func TestRenderProductChangesWhenInstanceMoves(t *testing.T) {
	artifact := workspace.Artifact{GeometryKey: "shape", Mesh: workspace.Mesh{
		Vertices: [][3]float64{{0, 0, 0}, {10, 0, 0}, {0, 10, 5}}, Triangles: [][3]uint32{{0, 1, 2}},
	}}
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Name: "Assembly", Type: "PRODUCT"},
		Artifacts: map[string]workspace.Artifact{"shape": artifact}, ResolvedInstances: []workspace.ResolvedInstance{
			{ID: "one", GeometryKey: "shape"}, {ID: "two", GeometryKey: "shape", Translation: [3]float64{20, 0, 0}},
		}}
	first, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	view.ResolvedInstances[1].Translation = [3]float64{50, 10, 0}
	second, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("moving a Product instance did not change its thumbnail")
	}
}

func TestRenderSketchWithoutSolid(t *testing.T) {
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Name: "Sketch", Type: "PART"},
		Part: &workspace.PartModel{Features: []workspace.Feature{{Type: "RECTANGLE_SKETCH", Plane: "XY",
			Rectangle: &workspace.Rectangle{Origin: [2]float64{2, 3}, Width: 10, Height: 5}}}}}
	result, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte("<polyline")) {
		t.Fatalf("rendered SVG does not contain sketch geometry: %s", result)
	}
}
