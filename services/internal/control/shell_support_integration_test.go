//go:build integration

package control

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestSketchOnShellOuterFaceThroughRouter(t *testing.T) {
	service, store, db, client, _ := featureAssociationRuntimeService(t)
	v, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Shell outer support"})
	if e != nil {
		t.Fatal(e)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-shell-support-%d", v.Document.ID, seq)
		}
		next, err := service.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
		v = next
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "YZ"})
	profile := v.Part.Features[len(v.Part.Features)-1].ID
	operations := []workspace.SketchOperation{}
	for i := 0; i < 6; i++ {
		a := float64(i) * math.Pi / 3
		b := float64((i+1)%6) * math.Pi / 3
		operations = append(operations, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: fmt.Sprintf("edge-%d", i), StartPointID: fmt.Sprintf("point-%d", i), EndPointID: fmt.Sprintf("point-%d", (i+1)%6), Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: 30 * math.Cos(a), Y: 30 * math.Sin(a)}, End: &workspace.SketchPoint2{X: 30 * math.Cos(b), Y: 30 * math.Sin(b)}}})
	}
	for i := 0; i < 6; i++ {
		operations = append(operations, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: fmt.Sprintf("join-%d", i), Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: fmt.Sprintf("edge-%d", i), SubElement: "END"}, {Target: "ENTITY", EntityID: fmt.Sprintf("edge-%d", (i+1)%6), SubElement: "START"}}}})
	}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: operations})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 40})
	base := v.Part.Features[len(v.Part.Features)-1]
	outerBefore := findP7TopologyPick(t, service, v, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return math.Abs(s.CreationEvidence.Centroid[0]-40) < 1e-6
	})
	opening := findP7TopologyPick(t, service, v, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return math.Abs(s.CreationEvidence.Direction[0]) < 1e-6 && s.CreationEvidence.Direction[2] < -0.5
	})
	shell := workspace.Feature{Type: "SHELL", BodyID: base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: opening.Selection, SourceVersionID: opening.SourceVersionID}}}
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &shell})
	shell = v.Part.Features[len(v.Part.Features)-1]
	resolution, err := service.ResolvePersistentSelection(t.Context(), v.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: outerBefore.Selection, SourceVersionID: outerBefore.SourceVersionID, TargetVersionID: v.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if err != nil || resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
		t.Fatal("outer reference acquired the generated inner wall", resolution, err)
	}
	outer := resolution.Candidates[0]
	a := activeBodyArtifact(t, v)
	for local := uint64(1); local <= p6TopologyCount(t, a.Topology, "faces"); local++ {
		p, err := service.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, a.GeometryKey, "FACE", local)
		if err != nil || p.NamingStatus != "RESOLVED" {
			t.Fatal("face cannot be bound in its own revision", local, p.NamingStatus, err)
		}
	}
	request := workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-outer-sketch", Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: outer.GeometryKey, TopologyID: outer.LocalID, VersionID: v.Document.VersionID, BodyID: base.BodyID}
	before := v.Document.VersionID
	preview, err := service.PreviewCommand(t.Context(), v.Document.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if err != nil || reopened.Document.VersionID != before {
		t.Fatal("preview advanced Head", err)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	sketch := v.Part.Features[len(v.Part.Features)-1]
	if s := sketch.Sketch.Support; s.Status != "CONNECTED" || s.SourceFeatureID != shell.ID || s.PersistentSelection.Anchor.FeatureID != base.ID || math.Abs(s.Origin[0]-40) > 1e-6 {
		t.Fatal("support stage replaced by anchor or offset", s)
	}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -3, Y: -3}, Second: &workspace.SketchPoint2{X: 3, Y: 3}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch.ID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", BodyID: base.BodyID, Length: 3})
	child := v.Part.Features[len(v.Part.Features)-1]
	changed := shell
	changed.Length = 2
	edit := workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: shell.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, shell.ID)}
	apply(edit)
	check := func() {
		t.Helper()
		for _, f := range v.Part.Features {
			if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
				t.Fatal("downstream failed", f.ID, f.Diagnostic)
			}
			if f.ID == sketch.ID && (f.Sketch.Support.Status != "CONNECTED" || math.Abs(f.Sketch.Support.Origin[0]-40) > 1e-6) {
				t.Fatal("outer support moved to offset wall", f.Sketch.Support)
			}
		}
		if v.Part.Features[len(v.Part.Features)-1].ID != child.ID {
			t.Fatal("child identity changed")
		}
	}
	check()
	apply(workspace.CommandRequest{Type: "UNDO"})
	check()
	apply(workspace.CommandRequest{Type: "REDO"})
	check()
	fresh := workspace.NewWithArtifacts(db, client, store)
	v, e = fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	check()
	edit.ActorID = p6Actor
	edit.RequestID = v.Document.ID + "-cold"
	edit.ExpectedFeatureDigest = findP6FeatureDigest(t, v.StructureTree, shell.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, edit)
	if e != nil || cold.Artifact == nil {
		t.Fatal(e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if !reflect.DeepEqual(cold.Artifact.Representations[role].Digest, activeBodyArtifact(t, v).Representations[role].Digest) {
			t.Fatal("cold mismatch", role)
		}
	}
}

func TestEngineeringHoleOnShellOuterWallThroughRouter(t *testing.T) {
	service, store, db, client, _ := featureAssociationRuntimeService(t)
	v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Engineering Hole on shell"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-shell-hole-%d", v.Document.ID, seq)
		v, err = service.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := v.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -15, Y: -15}, Second: &workspace.SketchPoint2{X: 15, Y: 15}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
	base := v.Part.Features[len(v.Part.Features)-1]
	opening := findP7TopologyPick(t, service, v, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == base.ID && math.Abs(s.CreationEvidence.Direction[2]) < 1e-6
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "SHELL", BodyID: base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: opening.Selection, SourceVersionID: opening.SourceVersionID}}}})
	shell := v.Part.Features[len(v.Part.Features)-1]
	outer := findP7TopologyPick(t, service, v, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == base.ID && math.Abs(s.CreationEvidence.Centroid[2]-30) < 1e-6
	})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", BodyID: base.BodyID, VersionID: v.Document.VersionID, GeometryKey: outer.GeometryKey, TopologyID: outer.LocalID})
	location := v.Part.Features[len(v.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "center", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: base.BodyID, Operation: "REMOVE", Extent: "BLIND", HoleKind: "COUNTERBORE", Diameter: 4, Depth: 3, RecessDiameter: 8, RecessDepth: .4, Reversed: true, HolePoint: &workspace.PatternPointReference{SketchID: location.ID, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "center", SubElement: "POINT"}}}})
	hole := v.Part.Features[len(v.Part.Features)-1]
	before := activeBodyArtifact(t, v)
	shell.Length = 2
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: shell.ID, Feature: &shell, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, shell.ID)})
	for _, f := range v.Part.Features {
		if f.EvaluationStatus != "" {
			t.Fatal("downstream failed", f.Name, f.Diagnostic)
		}
		if f.ID == location.ID && (f.Sketch.Support.Status != "CONNECTED" || math.Abs(f.Sketch.Support.Origin[2]-30) > 1e-6 || f.Sketch.Support.SourceFeatureID != shell.ID) {
			t.Fatal("outer support drift", f.Sketch.Support)
		}
	}
	final := activeBodyArtifact(t, v)
	if final.GeometryID == before.GeometryID || v.Part.Features[len(v.Part.Features)-1].ID != hole.ID {
		t.Fatal("thickness did not rebuild original Hole")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if activeBodyArtifact(t, v).GeometryID != before.GeometryID {
		t.Fatal("undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if activeBodyArtifact(t, v).GeometryID != final.GeometryID {
		t.Fatal("redo")
	}
	fresh := workspace.NewWithArtifacts(db, client, store)
	opened, e := fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil || activeBodyArtifact(t, opened).GeometryID != final.GeometryID {
		t.Fatal("reopen", e)
	}
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-shell-hole-cold", Type: "EDIT_FEATURE", TargetID: hole.ID, Feature: &hole, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, hole.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold differs", role)
		}
	}
}
