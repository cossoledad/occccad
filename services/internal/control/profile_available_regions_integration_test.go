//go:build integration

package control

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestExplicitProfileWithUnfinishedSketchThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Independent profile components"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-available-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketchID := view.Part.Features[0].ID
	edit := func(ops ...workspace.SketchOperation) {
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: ops})
	}
	edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "outer", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 10}},
		workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "hole", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 3}},
		workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "unfinished", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: 30}, End: &workspace.SketchPoint2{X: 40}}})
	analysis := view.SketchAnalyses[sketchID]
	if analysis.Status != "OPEN" || !analysis.GeometryVerified || analysis.RegionCount != 1 || analysis.LoopCount != 2 || len(analysis.Issues) == 0 {
		t.Fatalf("partial analysis lost exact regions or diagnostics: %+v", analysis)
	}
	regionID := analysis.RegionIDs[0]
	create := workspace.CommandRequest{ActorID: p6Actor, Type: "CREATE_SOLID_FEATURE", SketchID: sketchID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5}
	head := view.Document.VersionID
	if _, err = service.PreviewCommand(t.Context(), view.Document.ID, create); err == nil {
		t.Fatal("whole sketch ignored unfinished curves")
	}
	read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("failed whole preview changed Head", e)
	}
	create.ProfileSelection = &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{regionID}}
	apply(create)
	padID := view.Part.Features[len(view.Part.Features)-1].ID
	dependencies := func(want string) {
		t.Helper()
		head := view.Document.VersionID
		graph, e := service.GetPartDependencies(t.Context(), view.Document.ID, workspace.PartDependencyRequest{VersionID: head})
		if e != nil {
			t.Fatal("formal dependency read", e)
		}
		found := false
		for _, n := range graph.Nodes {
			if string(n.Key) == "feature:"+padID {
				found = true
				if len(n.Inputs) != 1 || n.Inputs[0].RegionID != regionID || n.Inputs[0].State != want {
					t.Fatal("region input state", n)
				}
			}
		}
		if !found {
			t.Fatal("missing consumer in dependency graph")
		}
		read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
		if e != nil || read.Document.VersionID != head {
			t.Fatal("dependency read changed Head", e)
		}
		if _, e = service.GetPartDependencies(t.Context(), view.Document.ID, workspace.PartDependencyRequest{VersionID: "old"}); e == nil {
			t.Fatal("stale dependency scope accepted")
		}
	}
	dependencies("AVAILABLE")
	volume := func(want float64) {
		t.Helper()
		if math.Abs(activeBodyArtifact(t, view).Volume-want) > 1e-5 {
			t.Fatal("unexpected region material", activeBodyArtifact(t, view).Volume, want)
		}
	}
	volume(5 * math.Pi * 91)
	initial := activeBodyArtifact(t, view)
	edit(workspace.SketchOperation{Type: "UPDATE_ENTITY_POINT", EntityID: "unfinished", SubElement: "END", Point: &workspace.SketchPoint2{X: 45, Y: 5}})
	edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "later-region", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 50}, Radius: 2}})
	volume(5 * math.Pi * 91)
	for _, role := range []string{"BREP", "NAMING"} {
		if activeBodyArtifact(t, view).Representations[role].Digest != initial.Representations[role].Digest {
			t.Fatal("unconsumed geometry changed authoritative result", role)
		}
	}
	saved := func() workspace.Feature {
		for _, f := range view.Part.Features {
			if f.ID == padID {
				return f
			}
		}
		t.Fatal("missing pad")
		return workspace.Feature{}
	}
	if saved().ProfileSelection.RegionIDs[0] != regionID || len(view.SketchAnalyses[sketchID].RegionIDs) != 2 {
		t.Fatal("explicit region set changed")
	}
	edit(workspace.SketchOperation{Type: "DELETE_ENTITIES", EntityIDs: []string{"outer"}})
	if saved().EvaluationStatus != "FAILED" || !strings.Contains(saved().Diagnostic, "PROFILE_REGION_MISSING") {
		t.Fatal("deleted selected region silently rebound", saved())
	}
	dependencies("MISSING")
	comparison, e := service.GetFeatureRepairContext(t.Context(), view.Document.ID, workspace.FeatureRepairRequest{PartDependencyRequest: workspace.PartDependencyRequest{VersionID: view.Document.VersionID}, FeatureID: padID, ActorID: p6Actor})
	if e != nil || comparison.State != "AVAILABLE" || comparison.Snapshot == nil || comparison.Snapshot.VersionID == view.Document.VersionID {
		t.Fatal("healthy region comparison", e, comparison)
	}
	if comparison.Snapshot.Feature.ProfileSelection.RegionIDs[0] != regionID || len(comparison.Snapshot.Artifacts) != 1 || math.Abs(comparison.Snapshot.Artifacts[0].Volume-5*math.Pi*91) > 1e-5 {
		t.Fatal("history substituted current profile", comparison.Snapshot)
	}
	comparisonArtifact := comparison.Snapshot.Artifacts[0]
	if comparisonArtifact.Naming.CanBind || len(comparisonArtifact.Representations) != 1 || !service.PreviewVisualAllowed(view.Document.ID, p6Actor, comparison.DisplayID, comparisonArtifact.Representations["VISUAL"].ObjectID) {
		t.Fatal("comparison became a topology source")
	}
	service.DiscardPreview(view.Document.ID, p6Actor, comparison.DisplayID)
	available := view.SketchAnalyses[sketchID]
	if available.Status != "OPEN" || !available.GeometryVerified || len(available.RegionIDs) != 2 {
		t.Fatal("unrelated open geometry hid repair candidates", available)
	}
	replacement := saved()
	replacementID := ""
	for _, region := range available.Regions {
		for _, curve := range region.Outer.Curves {
			if curve.EntityID == "hole" {
				replacementID = region.ID
			}
		}
	}
	if replacementID == "" {
		t.Fatal("missing explicit former hole as repair target")
	}
	replacement.ProfileSelection = &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{replacementID}}
	repair := workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: padID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, padID), Feature: &replacement}
	apply(repair)
	if saved().EvaluationStatus == "FAILED" {
		t.Fatal("explicit repair did not recover")
	}
	volume(5 * math.Pi * 9)
	final := activeBodyArtifact(t, view)
	apply(workspace.CommandRequest{Type: "UNDO"})
	if saved().EvaluationStatus != "FAILED" {
		t.Fatal("undo lost original missing selection")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, err = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	repair.ActorID = p6Actor
	repair.RequestID = view.Document.ID + "-cold-available"
	repair.ExpectedFeatureDigest = findP6FeatureDigest(t, view.StructureTree, padID)
	cold, err := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, repair)
	if err != nil || cold.Artifact == nil {
		t.Fatal("cold available profile rebuild", err)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold differs", role)
		}
	}
}
