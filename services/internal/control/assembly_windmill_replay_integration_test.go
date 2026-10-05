package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestWindmillCylinderMinimalReplayThroughRouter(t *testing.T) {
	_, _, _, client := featureAssociationTestService(t)
	input, e := os.ReadFile(filepath.Join("..", "..", "..", "tests", "test.data", "windmill-cylinder-reference.3dreplay"))
	if e != nil {
		t.Fatal(e)
	}
	data, e := client.ReplayAssembly(t.Context(), input)
	if e != nil {
		t.Fatal(e)
	}
	var replay geometry.AssemblyReplay
	if e = json.Unmarshal(data, &replay); e != nil {
		t.Fatal(e)
	}
	var result workerv1.SolveAssemblyResponse
	if e = protojson.Unmarshal(replay.Result, &result); e != nil {
		t.Fatal(e)
	}
	if result.Status != "CONVERGED" {
		t.Fatal(result.Status, result.Diagnostic)
	}
	for _, c := range result.Components {
		if c.Solved {
			if c.Preference.Status != workerv1.AssemblyPreferenceStatus_PREFERENCE_CONVERGED || !c.Preference.GeometricallyFeasible || c.Preference.ReferenceOptimality > 1e-8 || c.Preference.TotalOptimality > 1e-8 {
				t.Fatal(c.Preference)
			}
		}
	}
	if len(data) > 32*1024 {
		t.Fatal("minimal kernel replay grew unexpectedly", len(data))
	}
}
