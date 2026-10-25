package workspace

import "testing"

func TestIntersectionCutCollectsBothStableEndpointsAcrossPeriodicSeam(t *testing.T) {
	source := &SketchFeature{Entities: []SketchEntity{{ID: "circle", Kind: "CIRCLE"}}}
	hit := SketchIntersection{Parameters: [2]float64{.2, .6}, References: [2]SketchGeometryRef{{Target: "ENTITY", EntityID: "circle", SubElement: "WHOLE"}, {Target: "ENTITY", EntityID: "support", SubElement: "WHOLE"}}}
	op := SketchOperation{Type: "REPLACE_CURVE_INTERVALS", EntityIDs: []string{"circle"}, Intervals: []SketchCurveInterval{{Start: .2, End: .6}, {Start: .6, End: 1.2}}, ComputedEntities: []SketchEntity{{ID: "circle", StartPointID: "a", EndPointID: "b"}, {ID: "second", StartPointID: "c", EndPointID: "d"}}}
	refs, err := sketchIntersectionCutReferences(source, []SketchOperation{op}, hit, 0)
	if err != nil || len(refs) != 2 {
		t.Fatal(refs, err)
	}
	if refs[0].EntityID != "circle" || refs[0].PointID != "a" || refs[1].EntityID != "second" || refs[1].SubElement != "END" || refs[1].PointID != "d" {
		t.Fatal("periodic cut lost the wrapped endpoint", refs)
	}
	refs, err = sketchIntersectionCutReferences(source, []SketchOperation{op}, hit, 1)
	if err != nil || len(refs) != 1 || refs[0] != hit.References[1] {
		t.Fatal("uncut support changed", refs, err)
	}
	hit.Parameters[0] = .4
	if _, err = sketchIntersectionCutReferences(source, []SketchOperation{op}, hit, 0); err == nil {
		t.Fatal("missing endpoint silently fell back to whole source")
	}
}
