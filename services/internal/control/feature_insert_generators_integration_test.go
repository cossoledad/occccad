//go:build integration

package control

import (
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestInsertExpandedGeneratorsAndExtentsThroughRouter(t *testing.T) {
	for _, kind := range []string{"TWO_SIDED", "SYMMETRIC", "THROUGH_ALL", "TO_PLANE", "REVOLVE", "HOLE", "LOFT"} {
		t.Run(kind, func(t *testing.T) {
			x := newInsertionScene(t)
			initial := activeBodyArtifact(t, x.v).Volume
			sketch := x.historicalSketch(x.base.ID, kind == "HOLE", false)
			expected := initial
			if kind == "REVOLVE" {
				// A frame reassignment is a formal current command before insertion.
				x.apply(workspace.CommandRequest{Type: "REASSIGN_SKETCH_SUPPORT", SketchID: sketch.ID, DatumPlaneID: "datum-xz", VersionID: x.v.Document.VersionID})
			}
			if kind == "HOLE" {
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "position", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
			} else if kind == "LOFT" {
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "section-a", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 2}}}})
			} else {
				a, b := workspace.SketchPoint2{X: -2, Y: -2}, workspace.SketchPoint2{X: 2, Y: 2}
				if kind == "REVOLVE" {
					a, b = workspace.SketchPoint2{X: 2, Y: 0}, workspace.SketchPoint2{X: 4, Y: 10}
				}
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &a, Second: &b}}})
			}
			after := sketch.ID
			if kind == "LOFT" {
				x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Loft end", Origin: [3]float64{0, 0, 10}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
				plane := x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID
				input := x.input(sketch.ID, sketch.BodyID)
				x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", AfterFeatureID: sketch.ID, BodyID: sketch.BodyID, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey, DatumPlaneID: plane})
				for _, f := range x.v.Part.Features {
					if f.Sketch != nil && f.ID != sketch.ID && f.ID != x.base.Profile {
						after = f.ID
					}
				}
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: after, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "section-b", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 3}}}})
			}
			input := x.input(after, sketch.BodyID)
			r := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", AfterFeatureID: after, BodyID: sketch.BodyID, SketchID: sketch.ID, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Extent: kind, Length: 10, Length2: 2}
			switch kind {
			case "TWO_SIDED":
				expected -= 160
			case "SYMMETRIC":
				r.Operation = "ADD"
				r.Length = 50
				expected += 480
			case "THROUGH_ALL":
				expected -= 320
			case "TO_PLANE":
				x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "End", Origin: [3]float64{0, 0, 25}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
				r.VersionID = x.v.Document.VersionID
				r.GeometryKey = x.input(after, sketch.BodyID).Artifact.GeometryKey
				r.Operation = "ADD"
				r.TerminationPlaneID = x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID
				expected += 80
			case "REVOLVE":
				r.Extent = "FINITE"
				r.Generator = "REVOLVE"
				r.Angle = 360
				r.AxisEntityID = "AXIS_SYSTEM:axis-system-default:Z"
				expected -= 120 * math.Pi
			case "HOLE":
				r.Feature = &workspace.Feature{Type: "HOLE", BodyID: sketch.BodyID, Operation: "REMOVE", Extent: "BLIND", HolePoint: &workspace.PatternPointReference{SketchID: sketch.ID, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "position", SubElement: "POINT"}}, Diameter: 4, Depth: 5, Reversed: true}
				expected -= 20 * math.Pi
			case "LOFT":
				r.Feature = &workspace.Feature{Type: "LOFT", BodyID: sketch.BodyID, Operation: "REMOVE", Sections: []workspace.LoftSection{{SketchID: sketch.ID}, {SketchID: after}}}
				expected -= 190 * math.Pi / 3
			}
			r = x.request(r)
			head := x.v.Document.VersionID
			preview, e := x.s.PreviewCommand(t.Context(), x.v.Document.ID, r)
			if e != nil || preview.Artifact == nil {
				t.Fatal("historical generator preview", e)
			}
			read, e := x.s.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
			if e != nil || read.Document.VersionID != head {
				t.Fatal("preview mutated Head", e)
			}
			r.PreviewID = preview.PreviewID
			x.apply(r)
			if math.Abs(activeBodyArtifact(t, x.v).Volume-expected) > 1e-4 {
				t.Fatal("inserted geometry", activeBodyArtifact(t, x.v).Volume, expected)
			}
			inserted := x.v.Part.Features[len(x.v.Part.Features)-2]
			if inserted.ID == x.chamfer.ID || x.v.Part.Features[len(x.v.Part.Features)-1].ID != x.chamfer.ID {
				t.Fatal("generator appended after downstream modifier")
			}
			final := activeBodyArtifact(t, x.v)
			x.apply(workspace.CommandRequest{Type: "UNDO"})
			if math.Abs(activeBodyArtifact(t, x.v).Volume-initial) > 1e-5 {
				t.Fatal("undo failed")
			}
			x.apply(workspace.CommandRequest{Type: "REDO"})
			if math.Abs(activeBodyArtifact(t, x.v).Volume-expected) > 1e-4 {
				t.Fatal("redo failed")
			}
			fresh := workspace.NewWithArtifacts(x.db, x.client, x.artifacts)
			x.v, e = fresh.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), x.v.Document.ID, x.request(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: inserted.ID, Feature: &inserted, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, inserted.ID)}))
			if e != nil || cold.Artifact == nil {
				t.Fatal("cold historical generator", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
					t.Fatal("cold differs", role)
				}
			}
		})
	}
}

func TestInsertInputSketchBeforeFirstGeneratorThroughRouter(t *testing.T) {
	s, a, db, client, _ := featureAssociationRuntimeService(t)
	v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Replace first generator input"})
	if e != nil {
		t.Fatal(e)
	}
	x := &insertionScene{t: t, s: s, artifacts: a, db: db, client: client, v: v}
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	original := x.v.Part.Features[0].ID
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: original, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: original, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	base := x.v.Part.Features[1]
	input := x.input(original, base.BodyID)
	if input.SourceFeatureID != "" || len(input.Artifact.Mesh.Triangles) > 0 {
		t.Fatal("empty prefix leaked the later solid", input.SourceFeatureID)
	}
	before := x.v.Document.VersionID
	r := x.request(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY", BodyID: base.BodyID, AfterFeatureID: original, VersionID: before, GeometryKey: input.Artifact.GeometryKey})
	preview, e := s.PreviewCommand(t.Context(), x.v.Document.ID, r)
	if e != nil {
		t.Fatal("empty input preview", e)
	}
	r.PreviewID = preview.PreviewID
	x.apply(r)
	var sketch workspace.Feature
	for _, f := range x.v.Part.Features {
		if f.Sketch != nil && f.ID != original {
			sketch = f
		}
	}
	if sketch.ID == "" || x.v.Part.Features[len(x.v.Part.Features)-1].ID != base.ID {
		t.Fatal("new input appended beyond consumer")
	}
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "replacement", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 5}}}})
	base.Profile = sketch.ID
	base.ProfileSelection = &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: append([]string(nil), x.v.SketchAnalyses[sketch.ID].RegionIDs...)}
	x.apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: base.ID, Feature: &base, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, base.ID)})
	if math.Abs(activeBodyArtifact(t, x.v).Volume-500*math.Pi) > 1e-5 {
		t.Fatal("generator did not consume newly inserted input")
	}
	x.apply(workspace.CommandRequest{Type: "UNDO"})
	if math.Abs(activeBodyArtifact(t, x.v).Volume-8000) > 1e-5 {
		t.Fatal("replacement undo failed")
	}
	x.apply(workspace.CommandRequest{Type: "REDO"})
	input = x.input(original, base.BodyID)
	bad := x.request(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY", BodyID: base.BodyID, AfterFeatureID: original, VersionID: x.v.Document.VersionID, GeometryKey: activeBodyArtifact(t, x.v).GeometryKey})
	x.reject(bad, "GeometryKey")
	if input.SourceFeatureID != "" {
		t.Fatal("reopening input started authorizing downstream topology")
	}
}
