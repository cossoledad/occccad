//go:build integration

package geometry

import (
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

func measuredEvaluationVolume(t *testing.T, client *Client, response *workerv1.EvaluatePartResponse) float64 {
	t.Helper()
	ref := response.GetBrepArtifact()
	input := AnalysisGeometry{ID: "property", GeometryID: response.GetGeometryId(), BRep: ArtifactReference{Backend: ref.GetBackend(), ObjectKey: ref.GetObjectKey(), SHA256: ref.GetSha256(), Size: int64(ref.GetSizeBytes()), ContentType: ref.GetContentType()}, Pose: AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}
	result, err := client.MeasureGeometry(t.Context(), "explicit-volume-oracle", "ITEM", []AnalysisGeometry{input}, [][]MeasurementElement{{{InputID: "property", Kind: "BODY", Mode: "GEOMETRY"}}})
	if err != nil {
		t.Fatal(err)
	}
	value, ok := result.Values["volumeMm3"]
	if !result.Complete || !ok {
		t.Fatalf("missing exact mass query: %+v", result)
	}
	return value
}
