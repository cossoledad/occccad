package workspace

import (
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
	"os"
	"testing"
)

func TestNamingV2TablesBodyTipsAndHistory(t *testing.T) {
	refs := []*workerv1.SemanticTopologyRef{testRef("extrude-1", "END_CAP/region-1"), testRef("cut", "left"), testRef("cut", "right"), testRef("other-body", "face")}
	m := &workerv1.PartTopologyManifest{SchemaVersion: 2, GeometryId: "frozen", CoordinateSpace: "PART_LOCAL", LengthUnit: "mm", SemanticRefs: refs,
		Evidence: []*workerv1.NamingEvidence{{EvidenceDigest: "plane", MeasureDimension: "AREA", Adjacent: []uint32{4}, Geometry: &workerv1.NamingEvidence_Plane{Plane: &workerv1.NamingFrame{Direction: &workerv1.Vec3{Z: 1}}}}},
		Transitions: []*workerv1.NamingTransition{
			{FeatureId: "extrude-1", BodyId: "body-main", Complete: true, Lineage: []*workerv1.NamingLineage{{Result: 1, Evidence: 1}}},
			{FeatureId: "cut", BodyId: "body-main", Complete: true, Lineage: []*workerv1.NamingLineage{{Sources: []uint32{1}, Result: 2, Evidence: 1, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT}, {Sources: []uint32{1}, Result: 3, Evidence: 1, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT}}, Ambiguous: []*workerv1.NamingAmbiguous{{Sources: []uint32{1}, Candidates: []uint32{2, 3}, DiagnosticCode: "SPLIT"}}},
			{FeatureId: "other-body", BodyId: "body-b", Complete: true, Lineage: []*workerv1.NamingLineage{{Result: 4, Evidence: 1}}}},
		Bodies: []*workerv1.NamingBody{{BodyId: "body-main", Transitions: []uint32{1, 2}, TipTransition: 2, Tip: []*workerv1.NamingTopologyLocator{{TopologyType: 1, LocalId: 2, SemanticRef: 2, Evidence: 1}, {TopologyType: 1, LocalId: 3, SemanticRef: 3, Evidence: 1}}}, {BodyId: "body-b", Transitions: []uint32{3}, TipTransition: 3, Tip: []*workerv1.NamingTopologyLocator{{TopologyType: 1, LocalId: 4, SemanticRef: 4, Evidence: 1}}}}}
	// Each body is now its own authoritative artifact.
	crossBody := proto.Clone(m).(*workerv1.PartTopologyManifest)
	if _, err := unpackNaming(crossBody); err == nil {
		t.Fatal("cross-body naming accepted")
	}
	m.Transitions = m.Transitions[:2]
	m.Bodies = m.Bodies[:1]
	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &workerv1.PartTopologyManifest{}
	if err = proto.Unmarshal(data, decoded); err != nil || !proto.Equal(m, decoded) {
		t.Fatal("protobuf roundtrip", err)
	}
	domain, err := unpackNaming(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(domain.FeatureResults[0].SemanticOutputs) != 0 {
		t.Fatal("historical full snapshot reconstructed")
	}
	out, body := manifestOutput(domain, modelcore.PersistentTopologyFace, 2)
	if body != "body-main" || !proto.Equal(out.SemanticRef, refs[1]) || !proto.Equal(out.Evidence.Adjacent[0], refs[3]) {
		t.Fatal("tip index/table reconstruction")
	}
	if domain.FeatureResults[1].SemanticOutputs[0].Evidence != domain.FeatureResults[1].SemanticOutputs[1].Evidence {
		t.Fatal("shared evidence expanded redundantly")
	}
	selection := testSelection()
	if got := resolveManifest(selection, "key", domain); got.Status != modelcore.SelectionAmbiguous || len(got.Candidates) != 2 {
		t.Fatalf("split/body isolation: %+v", got)
	}
	// Merge produces one semantic descendant and keeps both source references.
	merged := proto.Clone(m).(*workerv1.PartTopologyManifest)
	merged.Transitions[1].Ambiguous = nil
	merged.Transitions[1].Lineage = []*workerv1.NamingLineage{{Sources: []uint32{1, 3}, Result: 2, Evidence: 1, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MERGED}}
	merged.Bodies[0].Tip = merged.Bodies[0].Tip[:1]
	domain, err = unpackNaming(merged)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolveManifest(selection, "key", domain); got.Status != modelcore.SelectionResolved {
		t.Fatalf("merge: %+v", got)
	}
	deleted := proto.Clone(m).(*workerv1.PartTopologyManifest)
	deleted.Transitions[1].Lineage = nil
	deleted.Transitions[1].Ambiguous = nil
	deleted.Transitions[1].Deleted = []*workerv1.NamingDeleted{{Source: 1, Evidence: 1, Reason: "removed"}}
	deleted.Bodies[0].Tip = nil
	domain, err = unpackNaming(deleted)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolveManifest(selection, "key", domain); got.Status != modelcore.SelectionMissing {
		t.Fatalf("deleted: %+v", got)
	}
	for name, mutate := range map[string]func(*workerv1.PartTopologyManifest){"bad ref": func(m *workerv1.PartTopologyManifest) { m.Bodies[0].Tip[0].SemanticRef = 99 }, "bad evidence": func(m *workerv1.PartTopologyManifest) { m.Bodies[0].Tip[0].Evidence = 0 }, "wrong body": func(m *workerv1.PartTopologyManifest) { m.Bodies[0].TipTransition = 3 }, "duplicate locator": func(m *workerv1.PartTopologyManifest) { m.Bodies[0].Tip[1].LocalId = 2 }, "unknown unit": func(m *workerv1.PartTopologyManifest) { m.LengthUnit = "m" }} {
		t.Run(name, func(t *testing.T) {
			bad := proto.Clone(m).(*workerv1.PartTopologyManifest)
			mutate(bad)
			if _, err := unpackNaming(bad); err == nil {
				t.Fatal("invalid artifact accepted")
			}
		})
	}
}
func TestNamingV2NativeRoundTrip(t *testing.T) {
	path := os.Getenv("OCCCCAD_TEST_NAMING_FIXTURE")
	if path == "" {
		t.Skip("requires native v2 naming fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := &workerv1.PartTopologyManifest{}
	if err = proto.Unmarshal(data, m); err != nil {
		t.Fatal(err)
	}
	domain, err := unpackNaming(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(domain.Tips) != 1 {
		t.Fatal("lost native body tips")
	}
	o, body := manifestOutput(domain, modelcore.PersistentTopologyFace, 2)
	if body != "body-a" || o.GetSemanticRef().GetFeatureId() != "feature-1" || o.Evidence.GetAdjacent()[0].FeatureId != "adjacency-feature" {
		t.Fatal("native table reconstruction")
	}
}

func TestNamingV2TypedEvidenceReconstruction(t *testing.T) {
	frame := &workerv1.NamingFrame{Origin: &workerv1.Vec3{X: 12}, Direction: &workerv1.Vec3{Z: 1}}
	start, end := 0.25, 2.75
	curve := func(family, convention string) *workerv1.NamingCurve {
		return &workerv1.NamingCurve{Family: family, Frame: frame, Range: &workerv1.NamingCurveRange{Start: &start, End: &end, Convention: convention}}
	}
	evidence := []*workerv1.NamingEvidence{
		{Geometry: &workerv1.NamingEvidence_Plane{Plane: frame}},
		{Geometry: &workerv1.NamingEvidence_Cylinder{Cylinder: frame}},
		{Geometry: &workerv1.NamingEvidence_Line{Line: curve("LINE", "LINE_MM")}},
		{Geometry: &workerv1.NamingEvidence_Circle{Circle: curve("CIRCLE", "ANGLE_RADIANS")}},
		{Geometry: &workerv1.NamingEvidence_Point{Point: &workerv1.NamingPoint{Position: frame.Origin}}},
		{Geometry: &workerv1.NamingEvidence_Spline{Spline: curve("BSPLINE", "NATIVE_CURVE_PARAMETER")}},
		{Geometry: &workerv1.NamingEvidence_Other{Other: &workerv1.NamingOtherGeometry{Family: "ELLIPSE", Frame: frame, Range: curve("ELLIPSE", "ANGLE_RADIANS").Range}}},
	}
	m := &workerv1.PartTopologyManifest{SchemaVersion: 2, GeometryId: "geometry", CoordinateSpace: "PART_LOCAL", LengthUnit: "mm", Evidence: evidence, SemanticRefs: []*workerv1.SemanticTopologyRef{testRef("f", "face")}, Transitions: []*workerv1.NamingTransition{{FeatureId: "f", BodyId: "body", Complete: true}}, Bodies: []*workerv1.NamingBody{{BodyId: "body", Transitions: []uint32{1}, TipTransition: 1}}}
	for i := range evidence {
		m.Bodies[0].Tip = append(m.Bodies[0].Tip, &workerv1.NamingTopologyLocator{TopologyType: 1, LocalId: uint64(i + 1), SemanticRef: 1, Evidence: uint32(i + 1)})
	}
	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip := &workerv1.PartTopologyManifest{}
	if err = proto.Unmarshal(data, roundTrip); err != nil {
		t.Fatal(err)
	}
	domain, err := unpackNaming(roundTrip)
	if err != nil {
		t.Fatal(err)
	}
	for i, kind := range []string{"PLANE", "CYLINDER", "LINE", "CIRCLE", "POINT", "BSPLINE", "ELLIPSE"} {
		e := domain.Tips["body"].SemanticOutputs[i].Evidence
		if e.GeometryType != kind || e.GetOrigin().GetX() != 12 {
			t.Fatalf("typed evidence %s changed: %+v", kind, e)
		}
		if i == 2 || i == 3 || i >= 5 {
			if e.GetParameterStart() != start || e.GetParameterEnd() != end {
				t.Fatalf("curve parameter changed: %+v", e)
			}
		}
	}
	roundTrip.Evidence[2].GetLine().Range.Convention = "ANGLE_RADIANS"
	if _, err = unpackNaming(roundTrip); err == nil {
		t.Fatal("line parameter with angular unit accepted")
	}
}
