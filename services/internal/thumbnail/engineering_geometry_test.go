package thumbnail

import (
	"bytes"
	"context"
	"testing"

	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func TestEngineeringThumbnailIncludesWiresPointsAndInheritedVisibility(t *testing.T) {
	hidden := false
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Type: "PART"}, Part: &workspace.PartModel{
		Bodies:          []workspace.PartBody{{ID: "empty", Visible: true}},
		GeometricalSets: []workspace.GeometricalSet{{ID: "root"}, {ID: "child", ParentID: "root"}},
		Features: []workspace.Feature{
			{ID: "wire", Type: "GEOMETRY", GeometricalSetID: "child", Result: &workspace.FeatureGeometryResult{ID: "result", Domain: "WIRE_SET", GeometryKey: "wire"}},
			{ID: "point", Type: "GEOMETRY", GeometricalSetID: "child", Result: &workspace.FeatureGeometryResult{ID: "result", Domain: "POINT_SET", GeometryKey: "point"}},
		},
		DatumPoints: []workspace.DatumPoint{{ID: "reference", GeometricalSetID: "child", Position: vec{0, 5, 3}}},
	}, Artifacts: map[string]workspace.Artifact{
		"wire":  {GeometryKey: "wire", GeometryDomain: "WIRE_SET", Mesh: workspace.Mesh{Edges: []workspace.MeshEdge{{LocalID: 1, Points: []vec{{0, 0, 0}, {20, 0, 10}}}}}},
		"point": {GeometryKey: "point", GeometryDomain: "POINT_SET", Mesh: workspace.Mesh{TopologyVertices: []visual.Point{{LocalID: 1, Point: vec{10, 5, 5}}}}},
	}}
	scene, err := collectScene(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if !scene.hasBounds || len(scene.meshes) != 1 || len(scene.lines) != 2 {
		t.Fatal("geometry-only Part lost render content")
	}
	png, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	image := decode(t, png)
	white, curve := 0, 0
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			r, g, b, _ := image.At(x, y).RGBA()
			if r > 55000 && g > 55000 && b > 55000 {
				white++
			}
			if b > r+6000 && r > g+1500 {
				curve++
			}
		}
	}
	if white < 2 || curve < 20 {
		t.Fatalf("point cross or engineering curve missing: white=%d curve=%d", white, curve)
	}
	view.Part.GeometricalSets[0].Visible = &hidden
	scene, err = collectScene(context.Background(), view)
	if err != nil || scene.hasBounds {
		t.Fatal("hidden parent did not hide content", err)
	}
	blank, err := Render(view)
	if err != nil || bytes.Equal(blank, png) {
		t.Fatal("hidden contents remained in thumbnail", err)
	}
}

func TestEngineeringThumbnailAddsSheetAlongsideMaterial(t *testing.T) {
	view := partView(boxMesh(10, 10, 10))
	view.Part.GeometricalSets = []workspace.GeometricalSet{{ID: "set"}}
	view.Part.Features = []workspace.Feature{{ID: "sheet", Type: "GEOMETRY", GeometricalSetID: "set", Result: &workspace.FeatureGeometryResult{ID: "result", Domain: "SHEET_SET", GeometryKey: "sheet"}}}
	view.Artifacts["sheet"] = workspace.Artifact{GeometryKey: "sheet", GeometryDomain: "SHEET_SET", Mesh: workspace.Mesh{Vertices: []vec{{30, 0, 0}, {30, 20, 0}, {30, 20, 20}}, Triangles: [][3]uint32{{0, 1, 2}}}}
	scene, err := collectScene(context.Background(), view)
	if err != nil || len(scene.meshes) != 2 {
		t.Fatal("surface vanished beside a Body", err)
	}
	data, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, data)
}

func TestEngineeringReferencePointDoesNotReplaceSketchThumbnail(t *testing.T) {
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Type: "PART"}, Part: &workspace.PartModel{GeometricalSets: []workspace.GeometricalSet{{ID: "set"}}, Features: []workspace.Feature{{ID: "sketch", Type: "SKETCH", GeometricalSetID: "set", Sketch: &workspace.SketchFeature{Entities: []workspace.SketchEntity{{ID: "line", Kind: "LINE", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: 10}}}}}}, DatumPoints: []workspace.DatumPoint{{ID: "reference", GeometricalSetID: "set", Position: vec{5, 5, 5}}}}}
	scene, err := collectScene(context.Background(), view)
	if err != nil || len(scene.lines) != 2 {
		t.Fatal("reference point suppressed the independent sketch", err)
	}
	if !scene.lines[0].engineering || scene.lines[1].engineering {
		t.Fatal("sketch and engineering point styles were mixed")
	}
}
