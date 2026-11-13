package thumbnail

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/visual"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Optional real-GLB measurements. Functional/cancellation contracts remain in
// ordinary tests; this export is not an industrial throughput assertion.
func TestStudioMeasurements(t *testing.T) {
	directory := os.Getenv("OCCCCAD_VISUAL_STUDIO_DIR")
	if directory == "" {
		t.Skip("NOT_APPLICABLE: optional native-GLB studio measurement export")
	}
	phase := os.Getenv("OCCCCAD_VISUAL_STUDIO_PHASE")
	if phase == "" {
		phase = "current"
	}
	output := filepath.Join(filepath.Dir(directory), "comparison", phase)
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	var reports []map[string]any
	for _, name := range []string{"fillet", "sphere", "shallow", "saddle"} {
		for _, quality := range []string{"legacy", "fine", "balanced"} {
			data, err := os.ReadFile(filepath.Join(directory, name+"-"+quality+".glb"))
			if err != nil {
				t.Fatal(err)
			}
			mesh, _, err := visual.Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			view := partView(mesh)
			if name == "shallow" || name == "saddle" {
				a := view.Artifacts["fixture"]
				a.GeometryDomain = "SHEET_SET"
				view.Artifacts["fixture"] = a
			}
			var samples []map[string]float64
			for i := 0; i < 4; i++ {
				start := time.Now()
				scene, err := collectScene(context.Background(), view)
				if err != nil {
					t.Fatal(err)
				}
				prepared := time.Since(start).Seconds() * 1000
				start = time.Now()
				raster := newRaster(scene)
				if err = raster.draw(context.Background(), scene); err != nil {
					t.Fatal(err)
				}
				drawn := time.Since(start).Seconds() * 1000
				start = time.Now()
				png, err := encodePNG(context.Background(), raster.image())
				if err != nil {
					t.Fatal(err)
				}
				encoded := time.Since(start).Seconds() * 1000
				if i > 0 {
					samples = append(samples, map[string]float64{"prepareMs": prepared, "rasterMs": drawn, "pngMs": encoded})
				}
				if i == 3 {
					if err = os.WriteFile(filepath.Join(output, fmt.Sprintf("thumbnail-%s-%s.png", name, quality)), png, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			reports = append(reports, map[string]any{"model": name, "quality": quality, "triangles": len(mesh.Triangles), "vertices": len(mesh.Vertices), "samples": samples})
		}
	}
	raw, err := json.MarshalIndent(map[string]any{"renderer": RendererVersion, "size": []int{Width, Height}, "supersampling": samples, "reports": reports}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "thumbnail-measurements.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
}
