package workspace

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestTopologyObservationCurrentMeasurementsAndIdentity(t *testing.T) {
	radius, length, start, end := 3.0, 0.0188495559215, 0.0, 2*math.Pi
	face := testRef("feature", "opaque-face")
	edge := testRef("feature", "opaque-edge")
	vertex := testRef("feature", "opaque-vertex")
	outputs := []*workerv1.SemanticTopologyOutput{
		{TopologyType: 1, SemanticRef: face, Evidence: &workerv1.SelectionEvidence{GeometryType: "CYLINDER", RadiusMm: &radius, Origin: &workerv1.Vec3{}, Direction: &workerv1.Vec3{Z: 1}, Centroid: &workerv1.Vec3{Z: 10}, Adjacent: []*workerv1.SemanticTopologyRef{edge}}},
		{TopologyType: 2, SemanticRef: edge, Evidence: &workerv1.SelectionEvidence{GeometryType: "CIRCLE", RadiusMm: &radius, Origin: &workerv1.Vec3{}, Direction: &workerv1.Vec3{Z: -1}, Centroid: &workerv1.Vec3{}, MeasureDimension: "LENGTH", MeasureSi: &length, ParameterStart: &start, ParameterEnd: &end, Adjacent: []*workerv1.SemanticTopologyRef{face, vertex}}},
		{TopologyType: 3, SemanticRef: vertex, Evidence: &workerv1.SelectionEvidence{GeometryType: "POINT", Origin: &workerv1.Vec3{}, Centroid: &workerv1.Vec3{}}},
	}
	edges, err := projectBodyTopology(t.Context(), "document", "revision", "body", modelcore.PersistentTopologyEdge, outputs)
	if err != nil || len(edges) != 1 {
		t.Fatal(edges, err)
	}
	g := edges[0].Geometry
	if g.RadiusMM == nil || *g.RadiusMM != 3 || g.OriginMM == nil || *g.OriginMM != [3]float64{} || g.Direction == nil || (*g.Direction)[2] != -1 || g.LengthMM == nil || math.Abs(*g.LengthMM-length*1000) > 1e-12 || g.Range.Convention != "ANGLE_RADIANS" {
		t.Fatal("current typed measurements/zero origin lost", g)
	}
	formal := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: "document", SourceBodyID: "body", Anchor: semanticRef(edge), ExpectedType: modelcore.PersistentTopologyEdge, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(outputs[1].Evidence)}
	raw, _ := json.Marshal(formal)
	if edges[0].ID != modelcore.ValueDigest(raw) {
		t.Fatal("stable ID calculation changed")
	}
	faces, err := projectBodyTopology(t.Context(), "document", "revision", "body", modelcore.PersistentTopologyFace, outputs)
	if err != nil || !edges[0].RelationsComplete || len(edges[0].FaceIDs) != 1 || edges[0].FaceIDs[0] != faces[0].ID {
		t.Fatal("typed current adjacency", edges, err)
	}
	outputs[1].Evidence.Adjacent = append(outputs[1].Evidence.Adjacent, testRef("old", "not-current"))
	partial, err := projectBodyTopology(t.Context(), "document", "revision", "body", modelcore.PersistentTopologyEdge, outputs)
	if err != nil || partial[0].RelationsComplete || partial[0].UnresolvedRelations != 1 {
		t.Fatal("old evidence counted as current relation", partial, err)
	}
	outputs[1].Evidence.Adjacent = nil
	missingRelations, err := projectBodyTopology(t.Context(), "document", "revision", "body", modelcore.PersistentTopologyEdge, outputs)
	if err != nil || missingRelations[0].RelationsComplete {
		t.Fatal("absent adjacency evidence became a known empty relation", err)
	}
	missing := observeTopologyGeometry(&workerv1.SelectionEvidence{GeometryType: "CIRCLE", Direction: &workerv1.Vec3{}})
	if missing.OriginMM != nil || missing.RadiusMM != nil || missing.Direction != nil || !slices.Contains(missing.Unavailable, "radiusMM") || !slices.Contains(missing.Unavailable, "direction") {
		t.Fatal("missing properties became invented zero measurements", missing)
	}
	spline := observeTopologyGeometry(&workerv1.SelectionEvidence{GeometryType: "BSPLINE", Origin: &workerv1.Vec3{}, Direction: &workerv1.Vec3{Z: 1}})
	if spline.OriginMM != nil || spline.Direction != nil {
		t.Fatal("placeholder spline frame treated as analytic axis")
	}
}
