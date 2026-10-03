package workspace

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestSketchUXLinkedMirrorChamferExtrudeParameterHistory(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	bottom, right, top := uuid.NewString(), uuid.NewString(), uuid.NewString()
	width, height := uuid.NewString(), uuid.NewString()
	axis := SketchGeometryRef{Target: "SKETCH_Y_AXIS", SubElement: "DIRECTION"}
	ops := []SketchOperation{
		workflowEntity(SketchEntity{ID: bottom, Kind: "LINE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}, Role: "PROFILE"}),
		workflowEntity(SketchEntity{ID: right, Kind: "LINE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 10, Y: 10}, Role: "PROFILE"}),
		workflowEntity(SketchEntity{ID: top, Kind: "LINE", Start: &SketchPoint2{X: 10, Y: 10}, End: &SketchPoint2{Y: 10}, Role: "PROFILE"}),
		workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(bottom, "END"), workflowRef(right, "START")}}),
		workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(right, "END"), workflowRef(top, "START")}}),
		workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef(bottom, "START")}, FixedPoint: &SketchPoint2{}}),
		workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "POINT_ON_OBJECT", References: []SketchGeometryRef{workflowRef(top, "END"), axis}}),
		workflowConstraint(SketchConstraint{ID: width, Kind: "LENGTH", References: []SketchGeometryRef{workflowRef(bottom, "WHOLE")}, Value: workflowValue(10), Unit: "mm"}),
		workflowConstraint(SketchConstraint{ID: height, Kind: "LENGTH", References: []SketchGeometryRef{workflowRef(right, "WHOLE")}, Value: workflowValue(10), Unit: "mm"}),
	}
	for _, id := range []string{bottom, top} {
		ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "HORIZONTAL", References: []SketchGeometryRef{workflowRef(id, "WHOLE")}}))
	}
	ops = append(ops, workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "VERTICAL", References: []SketchGeometryRef{workflowRef(right, "WHOLE")}}))
	f.edit(ops...)
	mirrorView := f.edit(SketchOperation{Type: "MIRROR_ENTITIES", OperationID: uuid.NewString(), EntityIDs: []string{bottom, right, top}, Axis: &axis, MirrorMode: "LINKED"})
	mirrorIDs := map[string]bool{}
	for _, c := range f.sketch(mirrorView).Constraints {
		if c.Kind == "MIRROR" {
			mirrorIDs[c.ID] = true
		}
	}
	if len(mirrorIDs) != 3 {
		t.Fatalf("expected formal linked mirror relationships, got %d", len(mirrorIDs))
	}
	chamfer := cornerOperation("CHAMFER_ENTITIES", uuid.NewString(), bottom, right, "END", "START")
	chamfer.ChamferMode = "EQUAL"
	cut := f.edit(chamfer)
	parameterID := ""
	for _, c := range f.sketch(cut).Constraints {
		delete(mirrorIDs, c.ID)
		if c.ID == width {
			parameterID = c.ParameterID
		}
	}
	if len(mirrorIDs) != 0 || parameterID == "" {
		t.Fatal("corner silently lost mirror or dimension identity")
	}
	v := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
	f.volume(v, 399)
	profileIDs := func(view DocumentView) []string {
		var sketchFeature Feature
		for _, feature := range view.Part.Features {
			if feature.ID == f.sketchID {
				sketchFeature = feature
			}
		}
		regions, e := buildProfileRegions(sketchFeature)
		if e != nil {
			t.Fatal(e)
		}
		var result []string
		for _, region := range regions {
			result = append(result, region.ID, region.Outer.ID)
			for _, hole := range region.Holes {
				result = append(result, hole.ID)
			}
		}
		return result
	}
	beforeProfiles := profileIDs(v)
	v = f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: width, Value: workflowValue(12)})
	f.volume(v, 479)
	if !reflect.DeepEqual(profileIDs(v), beforeProfiles) {
		t.Fatal("parameter update changed profile identities")
	}
	changed, _ := json.Marshal(f.sketch(v))
	f.volume(f.command(CommandRequest{Type: "UNDO"}), 399)
	v = f.command(CommandRequest{Type: "REDO"})
	f.volume(v, 479)
	replay, _ := json.Marshal(f.sketch(v))
	if string(changed) != string(replay) {
		t.Fatal("redo changed stable sketch data")
	}
	cold := NewWithArtifacts(f.service.database, f.service.worker, f.service.artifacts)
	reopened, e := cold.GetDocument(f.ctx, f.documentID, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	f.volume(reopened, 479)
	reloaded, _ := json.Marshal(f.sketch(reopened))
	if string(changed) != string(reloaded) {
		t.Fatal("cold read changed stable sketch data")
	}
	if !reflect.DeepEqual(profileIDs(reopened), beforeProfiles) {
		t.Fatal("cold read changed profile identities")
	}
	for _, c := range f.sketch(reopened).Constraints {
		if c.ID == width && c.ParameterID != parameterID {
			t.Fatal("cold read changed ParameterId")
		}
	}
}

func TestSketchUXQuickTrimExternalBoundaryPreviewReadOnly(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	// The boundary comes from an actual upstream solid edge, bound through the
	// normal transient-pick -> persistent-selection -> projection pipeline.
	points := []SketchPoint2{{0, -10}, {10, -10}, {10, 10}, {0, 10}}
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	var ops []SketchOperation
	for i, id := range ids {
		a, b := points[i], points[(i+1)%4]
		ops = append(ops, workflowEntity(SketchEntity{ID: id, Kind: "LINE", Start: &a, End: &b, Role: "PROFILE"}), workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{workflowRef(id, "END"), workflowRef(ids[(i+1)%4], "START")}}))
	}
	f.edit(ops...)
	source := f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
	if e := f.service.HydrateDisplay(f.ctx, &source); e != nil {
		t.Fatal(e)
	}
	var edge uint64
	var geometryKey string
	for _, artifact := range source.Artifacts {
		for _, candidate := range artifact.Mesh.Edges {
			if len(candidate.Points) < 2 {
				continue
			}
			a, b := candidate.Points[0], candidate.Points[len(candidate.Points)-1]
			if a[0] == 0 && b[0] == 0 && a[1] != b[1] {
				edge = candidate.LocalID
				geometryKey = artifact.GeometryKey
				break
			}
		}
	}
	if edge == 0 && source.Artifact != nil {
		for _, candidate := range source.Artifact.Mesh.Edges {
			if len(candidate.Points) < 2 {
				continue
			}
			a, b := candidate.Points[0], candidate.Points[len(candidate.Points)-1]
			if a[0] == 0 && b[0] == 0 && a[1] != b[1] {
				edge = candidate.LocalID
				geometryKey = source.Artifact.GeometryKey
				break
			}
		}
	}
	if edge == 0 {
		t.Fatal("no real upstream boundary edge")
	}
	created := f.command(CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	for _, feature := range created.Part.Features {
		if feature.Type == "SKETCH" && feature.ID != f.sketchID {
			f.sketchID = feature.ID
		}
	}
	id, external := uuid.NewString(), uuid.NewString()
	before := f.edit(workflowEntity(SketchEntity{ID: id, Kind: "ELLIPSE", Center: &SketchPoint2{}, MajorRadius: 5, MinorRadius: 2, Role: "PROFILE"}), SketchOperation{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: external, GeometryKey: geometryKey, TopologyID: edge, TopologyKind: "EDGE", SourceVersionID: source.Document.VersionID})
	externalBefore := f.sketch(before).ExternalGeometry
	if len(externalBefore) != 1 || externalBefore[0].Status != "CONNECTED" {
		t.Fatalf("boundary not genuinely projected: %+v", externalBefore)
	}
	history, e := f.service.ListHistory(f.ctx, f.documentID)
	if e != nil {
		t.Fatal(e)
	}
	hit := .5
	op := SketchOperation{Type: "QUICK_TRIM", OperationID: uuid.NewString(), EntityIDs: []string{id}, BoundaryIDs: []string{external}, HitParameter: &hit, TrimMode: "KEEP_HIT"}
	preview, e := f.service.PreviewCommand(f.ctx, f.documentID, CommandRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: "EDIT_SKETCH", SketchID: f.sketchID, Operations: []SketchOperation{op}})
	if e != nil {
		t.Fatal(e)
	}
	if len(preview.SketchCandidates) != 1 {
		t.Fatal("external-boundary exact candidate missing")
	}
	if len(preview.SketchCandidates[0].Entities) != 1 {
		t.Fatal("wrong external trim entity count")
	}
	trimmed := preview.SketchCandidates[0].Entities[0]
	if trimmed.Kind != "ELLIPTICAL_ARC" || math.Abs(trimmed.StartAngle-math.Pi/2) > 1e-8 || math.Abs(trimmed.EndAngle-3*math.Pi/2) > 1e-8 {
		t.Fatalf("external exact intersection selected wrong half ellipse: %+v", trimmed)
	}
	after, e := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if e != nil {
		t.Fatal(e)
	}
	afterHistory, e := f.service.ListHistory(f.ctx, f.documentID)
	if e != nil {
		t.Fatal(e)
	}
	if before.Document.VersionID != after.Document.VersionID || !reflect.DeepEqual(before.Part, after.Part) || len(history) != len(afterHistory) {
		t.Fatal("external preview mutated persistent data")
	}
	committed := f.edit(op)
	if !reflect.DeepEqual(f.sketch(committed).Entities, preview.SketchCandidates[0].Entities) || !reflect.DeepEqual(f.sketch(committed).ExternalGeometry, externalBefore) {
		t.Fatal("external boundary changed or candidate differs from commit")
	}
}
