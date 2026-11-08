//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"slices"
	"strings"
	"testing"
)

func TestSketchSolidCombinedWorkflowAcrossRepeatedOccurrences(t *testing.T) {
	c := newContextInsertionScene(t, 30)
	body := c.base.BodyID
	defBase := 0.
	for _, b := range c.v.Part.Bodies {
		if b.ID == body {
			defBase = c.v.Artifacts[b.GeometryKey].Volume
		}
	}
	base := map[string]float64{}
	for _, v := range c.root.ContextVariants {
		for _, b := range v.Part.Bodies {
			if b.ID == body {
				base[v.OwningInstancePath.Canonical] = c.root.Artifacts[b.GeometryKey].Volume
			}
		}
	}
	poses := append([]workspace.ProductInstance(nil), c.root.Product.Instances...)
	run := func(r workspace.CommandRequest) { t.Helper(); c.apply(c.scoped(r)); c.reload() }
	last := func() workspace.Feature { return c.v.Part.Features[len(c.v.Part.Features)-1] }
	run(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
	slot := last().ID
	run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: slot, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "wall-line", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: -6, Y: -2}, End: &workspace.SketchPoint2{X: 6, Y: -2}}}}})
	run(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: body, SketchID: slot, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Extent: "TO_NEXT", ThinWall: &workspace.ThinWallDefinition{Input: "CHAINS", Mode: "ONE_SIDED", Side: "LEFT", Join: "MITER", Thickness: 1, EntityIDs: []string{"wall-line"}}})
	cut := last()
	copies := workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "REMOVE", Pattern: &workspace.FeaturePattern{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: cut.ID}, SourceKind: "FEATURE_REEXECUTION", PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 2, Spacing: 4, Direction: [3]float64{0, 1, 0}}}}
	run(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &copies})
	run(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{-4, 0, 0}, Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}})
	plane := c.v.Part.DatumPlanes[len(c.v.Part.DatumPlanes)-1].ID
	run(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, DatumPlaneID: plane})
	profile := last().ID
	run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "section", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: -6, Y: 5}, Radius: .5}}}})
	run(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 5}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	plane = c.v.Part.DatumPlanes[len(c.v.Part.DatumPlanes)-1].ID
	run(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, DatumPlaneID: plane})
	path := last().ID
	run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: path, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "route", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: -4, Y: -6}, End: &workspace.SketchPoint2{X: 4, Y: -6}}}}})
	sweep := workspace.Feature{Type: "SWEEP", BodyID: body, Profile: profile, Operation: "NEW_BODY", Sweep: &workspace.SweepDefinition{Path: workspace.CurveChainReference{SketchID: path, EntityIDs: []string{"route"}}, FrameMode: "CORRECTED_FRENET", CornerMode: "MITER"}}
	run(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &sweep})
	sweep = last()
	run(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
	positions := last().ID
	run(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: positions, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "left", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: -5, Y: 6}}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "right", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 5, Y: 6}}}}})
	hole := workspace.Feature{Type: "HOLE", BodyID: body, Operation: "REMOVE", Extent: "BLIND", Diameter: 1, Depth: 3, Thread: &workspace.HoleThreadDefinition{Mode: "ANNOTATION", Designation: "test thread", Hand: "RIGHT", MajorDiameter: 1.2, Pitch: .2, Depth: 2}}
	for _, id := range []string{"left", "right"} {
		hole.HolePoints = append(hole.HolePoints, workspace.PatternPointReference{SketchID: positions, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}})
	}
	run(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &hole})
	hole = last()
	check := func(withHoles bool) {
		t.Helper()
		holeVolume := 0.
		if withHoles {
			holeVolume = 1.5 * math.Pi
		}
		definitionVolume := 0.
		for _, b := range c.v.Part.Bodies {
			if b.ID == body {
				definitionVolume = c.v.Artifacts[b.GeometryKey].Volume
			}
		}
		if math.Abs(definitionVolume-(defBase-240-holeVolume)) > 1e-5 {
			t.Fatal("default context was overwritten", definitionVolume, defBase)
		}
		for _, v := range c.root.ContextVariants {
			if v.Status != "READY" {
				t.Fatal("repeated context failed", v.Diagnostic)
			}
			height := 0.
			for _, f := range v.Part.Features {
				if f.ID == c.base.ID {
					height = f.Length
				}
			}
			if height != 20 && height != 30 {
				t.Fatal("accepted input followed newer source Head", height)
			}
			for _, b := range v.Part.Bodies {
				got := c.root.Artifacts[b.GeometryKey].Volume
				want := 2 * math.Pi
				if b.ID == body {
					want = base[v.OwningInstancePath.Canonical] - 24*height - holeVolume
				}
				if math.Abs(got-want) > 1e-5 {
					t.Fatal("combined context volume", v.OwningInstancePath.Canonical, b.ID, got, want)
				}
			}
		}
		for i, p := range poses {
			if c.root.Product.Instances[i].Translation != p.Translation || c.root.Product.Instances[i].Rotation != p.Rotation {
				t.Fatal("model edit moved occurrence")
			}
		}
	}
	check(true)
	run(workspace.CommandRequest{Type: "UNDO"})
	check(false)
	run(workspace.CommandRequest{Type: "REDO"})
	check(true)
	// Cold reconstruction is performed in the actual edited occurrence, including
	// the other occurrence's separately accepted inputs during command preparation.
	saved := last()
	if saved.ID != hole.ID {
		t.Fatal("redo changed hole identity")
	}
	r := c.scoped(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: sweep.ID, Feature: &sweep, ExpectedFeatureDigest: findP6FeatureDigest(t, c.v.StructureTree, sweep.ID)})
	hot, e := c.s.PreviewCommand(t.Context(), c.v.Document.ID, r)
	if e != nil || hot.Artifact == nil {
		t.Fatal("combined hot preview", e)
	}
	cold, e := c.s.PreviewCommand(workspace.WithColdEvaluation(t.Context()), c.v.Document.ID, r)
	if e != nil || cold.Artifact == nil {
		t.Fatal("combined cold preview", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if hot.Artifact.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("combined cold identity", role)
		}
	}
}

func TestMasterSketchRegionsDriveShellAndLinkedOffsetBodies(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Master sketch shell workflow"})
	if e != nil {
		t.Fatal(e)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-master-%d", view.Document.ID, seq)
		var err error
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "wall", Value: 2, Unit: "mm"})
	wall := view.Part.Parameters[0].ParameterID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	master := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: master, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -20, Y: -10}, Second: &workspace.SketchPoint2{X: 20, Y: 10}}}})
	rectangle := view.SketchAnalyses[master].RegionIDs[0]
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: master, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{rectangle}}})
	shellBody := view.Part.ActiveBodyID
	top := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return strings.HasPrefix(p.Anchor.OutputSlot, "END_CAP/")
	})
	shell := workspace.Feature{Type: "SHELL", BodyID: shellBody, Length: 2, Selections: []workspace.FeatureSelection{{Selection: top.Selection, SourceVersionID: top.SourceVersionID}}}
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &shell, ParameterExpressions: map[string]string{"length": "wall"}})
	// Independent unfinished geometry is deliberately added after the first solid.
	// Its presence must not invalidate the saved explicit rectangular region.
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: master, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "outer", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 35}, Radius: 5}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "unfinished", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: 50}, End: &workspace.SketchPoint2{X: 55}}}}})
	expression := "wall"
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: master, Operations: []workspace.SketchOperation{{Type: "CREATE_OFFSET", Offset: &workspace.SketchOffset{ID: "inside", EntityIDs: []string{"outer"}, Distance: 2, Join: "MITER"}, ParameterSource: &expression}}})
	a := view.SketchAnalyses[master]
	if !a.GeometryVerified || len(a.RegionIDs) != 2 || a.Status == "CLOSED" {
		t.Fatal("available components lost", a)
	}
	ring := ""
	for _, id := range a.RegionIDs {
		if id != rectangle {
			ring = id
		}
	}
	if ring == "" {
		t.Fatal("ring region missing")
	}
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: master, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 5, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{ring}}})
	ringFeature := view.Part.Features[len(view.Part.Features)-1]
	ringBody := ringFeature.BodyID
	check := func(thickness float64) {
		t.Helper()
		for _, b := range view.Part.Bodies {
			want := 5 * math.Pi * (25 - math.Pow(5-thickness, 2))
			if b.ID == shellBody {
				want = 16000 - (40-2*thickness)*(20-2*thickness)*(20-thickness)
			} else if b.ID != ringBody {
				t.Fatal("unexpected body", b.ID)
			}
			if got := view.Artifacts[b.GeometryKey].Volume; math.Abs(got-want) > 1e-5 {
				t.Fatal("shared wall parameter did not drive both bodies", b.ID, got, want)
			}
		}
		if !slices.Contains(view.SketchAnalyses[master].RegionIDs, rectangle) || !slices.Contains(view.SketchAnalyses[master].RegionIDs, ring) {
			t.Fatal("formal regions changed across wall parameter")
		}
	}
	check(2)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: wall, Value: 3, Unit: "mm"})
	check(3)
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(2)
	apply(workspace.CommandRequest{Type: "REDO"})
	check(3)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	check(3)
	for _, f := range view.Part.Features {
		if f.ID == ringFeature.ID {
			ringFeature = f
		}
	}
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-master-cold", TargetID: ringFeature.ID, Feature: &ringFeature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, ringFeature.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("master cold reconstruction", e)
	}
	for _, b := range view.Part.Bodies {
		if b.ID == ringBody {
			for _, role := range []string{"BREP", "NAMING"} {
				if view.Artifacts[b.GeometryKey].Representations[role].Digest != cold.Artifact.Representations[role].Digest {
					t.Fatal("master cold identity", role)
				}
			}
		}
	}
}

func TestInsertSweepBeforeModifierKeepsInputsAndColdHistory(t *testing.T) {
	x := newInsertionScene(t)
	prior := activeBodyArtifact(t, x.v).Volume
	body := x.base.BodyID
	sketch := func(after, plane, datum string) workspace.Feature {
		t.Helper()
		old := map[string]bool{}
		for _, f := range x.v.Part.Features {
			old[f.ID] = true
		}
		input := x.input(after, body)
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", AfterFeatureID: after, BodyID: body, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey, Plane: plane, DatumPlaneID: datum})
		for _, f := range x.v.Part.Features {
			if !old[f.ID] && f.Sketch != nil {
				return f
			}
		}
		t.Fatal("historical input missing")
		return workspace.Feature{}
	}
	profile := sketch(x.base.ID, "YZ", "")
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "section", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{Y: 20}, Radius: 1}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	path := sketch(profile.ID, "", x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID)
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: path.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "route", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: 8}}}}})
	feature := workspace.Feature{Type: "SWEEP", BodyID: body, Profile: profile.ID, Operation: "ADD", Sweep: &workspace.SweepDefinition{Path: workspace.CurveChainReference{SketchID: path.ID, EntityIDs: []string{"route"}}, FrameMode: "CORRECTED_FRENET", CornerMode: "MITER"}}
	input := x.input(path.ID, body)
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &feature, AfterFeatureID: path.ID, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey})
	for _, f := range x.v.Part.Features {
		if f.Type == "SWEEP" {
			feature = f
		}
		if f.ID == x.chamfer.ID && f.EvaluationStatus != "" {
			t.Fatal("downstream modifier was not preserved", f)
		}
	}
	if x.v.Part.Features[len(x.v.Part.Features)-1].ID != x.chamfer.ID || feature.Sweep.Path.Direction == nil {
		t.Fatal("insertion position or direction identity")
	}
	if got := activeBodyArtifact(t, x.v).Volume; math.Abs(got-(prior+4*math.Pi)) > 1e-5 {
		t.Fatal("historical sweep volume", got, prior+4*math.Pi)
	}
	final := activeBodyArtifact(t, x.v)
	cold, e := x.s.PreviewCommand(workspace.WithColdEvaluation(t.Context()), x.v.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: x.v.Document.ID + "-historical-sweep-cold", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, feature.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("historical sweep cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("historical sweep identity", role)
		}
	}
}
