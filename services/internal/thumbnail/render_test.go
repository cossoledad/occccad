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
	}, Visualization: workspace.VisualizationManifest{SchemaVersion: 1, Primitives: []workspace.VisualPrimitive{
		{ID: "point", FeatureID: "sketch", Kind: "POINTS", Semantic: "SKETCH_POINT", Positions: [][3]float64{{2, 3, 0}}, Selectable: true},
		{ID: "line", FeatureID: "sketch", Kind: "POLYLINE", Semantic: "SKETCH_CURVE", Positions: [][3]float64{{0, 0, 0}, {5, 0, 0}}, Selectable: true},
	}}}
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
	if !bytes.Contains(first, []byte("<circle")) || !bytes.Contains(first, []byte("<polyline")) {
		t.Fatalf("Product thumbnail omitted referenced non-solid geometry: %s", first)
	}
}

func TestRenderSketchWithoutSolid(t *testing.T) {
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Name: "Sketch", Type: "PART"},
		Part: &workspace.PartModel{Features: []workspace.Feature{{Type: "SKETCH", Plane: "XY",
			Sketch: &workspace.SketchFeature{SchemaVersion: 1, Support: workspace.SketchSupport{Plane: "XY"}, Entities: []workspace.SketchEntity{
				{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: 2, Y: 3}, End: &workspace.SketchPoint2{X: 12, Y: 3}},
			}}}}}}
	result, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte("<polyline")) {
		t.Fatalf("rendered SVG does not contain sketch geometry: %s", result)
	}
}
