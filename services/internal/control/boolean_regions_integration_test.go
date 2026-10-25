package control

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestBooleanResultRegionsLifecycleThroughRouter(t *testing.T) {
	service, store, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Boolean exact result regions"})
	if err != nil {
		t.Fatal(err)
	}
	doc, sequence := view.Document.ID, 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		sequence++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-cells-%d", doc, sequence)
		v, e := service.ApplyCommand(t.Context(), doc, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		view = v
	}
	rectangle := func(x, y, w, h, length float64) workspace.Feature {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + w, Y: y + h}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: length})
		return view.Part.Features[len(view.Part.Features)-1]
	}
	base, tool := rectangle(0, 0, 30, 10, 10), rectangle(10, -5, 10, 20, 20)
	head := view.Document.VersionID
	analyze := func(featureID string) workspace.BooleanRegionsAnalysis {
		t.Helper()
		v, e := service.AnalyzeBooleanRegions(t.Context(), doc, workspace.BooleanRegionsRequest{VersionID: view.Document.VersionID, FeatureID: featureID, BodyID: base.BodyID, Tool: workspace.FeatureStageRef{BodyID: tool.BodyID, FeatureID: tool.ID}, Operation: "REMOVE"})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	analysis := analyze("")
	var regions []modelcore.BooleanRegionRef
	for _, c := range analysis.Regions {
		if c.Eligible {
			regions = append(regions, c.Reference)
			if math.Abs(c.Artifact.Volume-1000) > 1e-6 {
				t.Fatal(c.Artifact.Volume)
			}
		}
	}
	if len(regions) != 2 || reflect.DeepEqual(regions[0].Boundary, regions[1].Boundary) {
		t.Fatal("independent same-membership cells not distinguished")
	}
	untouched, e := service.GetDocument(t.Context(), doc)
	if e != nil || untouched.Document.VersionID != head {
		t.Fatal("analysis wrote Head", e)
	}
	input := workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", ActorID: p6Actor, RequestID: doc + "-region-preview", BodyID: base.BodyID, Operation: "REMOVE", Tools: []workspace.FeatureStageRef{{BodyID: tool.BodyID, FeatureID: tool.ID}}, KeepTools: true, BooleanResult: &modelcore.BooleanResultSelection{Mode: "EXPLICIT"}}
	if _, e := service.PreviewCommand(t.Context(), doc, input); e == nil || !strings.Contains(e.Error(), "BOOLEAN_REGION_SELECTION_EMPTY") {
		t.Fatal("empty selection accepted", e)
	}
	input.BooleanResult.Regions = regions[:1]
	input.RequestID = doc + "-select-one"
	preview, e := service.PreviewCommand(t.Context(), doc, input)
	if e != nil || preview.Artifact == nil || math.Abs(preview.Artifact.Volume-1000) > 1e-6 {
		t.Fatal("select one preview", e)
	}
	afterPreview, e := service.GetDocument(t.Context(), doc)
	if e != nil || afterPreview.Document.VersionID != head {
		t.Fatal("preview wrote Head", e)
	}
	input.PreviewID = preview.PreviewID
	view, e = service.ApplyCommand(t.Context(), doc, input)
	if e != nil {
		t.Fatal(e)
	}
	feature := view.Part.Features[len(view.Part.Features)-1]
	if feature.BooleanResult == nil || !reflect.DeepEqual(feature.BooleanResult.Regions, regions[:1]) {
		t.Fatal("region proof not persisted")
	}
	check := func(volume float64, consumed bool) {
		t.Helper()
		for _, b := range view.Part.Bodies {
			if b.ID == base.BodyID && math.Abs(view.Artifacts[b.GeometryKey].Volume-volume) > 1e-6 {
				t.Fatal("target volume", view.Artifacts[b.GeometryKey].Volume, volume)
			}
			if b.ID == tool.BodyID && b.Consumed != consumed {
				t.Fatal("keepTools coupled to retained regions")
			}
		}
	}
	check(1000, false)
	committedHead := view.Document.VersionID
	invalid := feature
	invalid.BooleanResult = &modelcore.BooleanResultSelection{Mode: "EXPLICIT"}
	invalidDigest, e := workspace.FeatureDefinitionDigest(*view.Part, feature.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.ApplyCommand(t.Context(), doc, workspace.CommandRequest{ActorID: p6Actor, RequestID: doc + "-invalid-direct-edit", Type: "EDIT_FEATURE", TargetID: feature.ID, ExpectedFeatureDigest: invalidDigest, Feature: &invalid}); e == nil || !strings.Contains(e.Error(), "BOOLEAN_REGION_SELECTION_EMPTY") {
		t.Fatal("direct commit bypassed result validation", e)
	}
	unchanged, e := service.GetDocument(t.Context(), doc)
	if e != nil || unchanged.Document.VersionID != committedHead {
		t.Fatal("invalid edit advanced Head", e)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(3000, false)
	apply(workspace.CommandRequest{Type: "REDO"})
	check(1000, false)
	edit := func(f workspace.Feature) {
		t.Helper()
		digest, e := workspace.FeatureDefinitionDigest(*view.Part, f.ID)
		if e != nil {
			t.Fatal(e)
		}
		apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, ExpectedFeatureDigest: digest, Feature: &f})
	}
	feature.KeepTools = false
	edit(feature)
	check(1000, true)
	feature.BooleanResult = &modelcore.BooleanResultSelection{Mode: "EXPLICIT", Regions: regions}
	edit(feature)
	check(2000, true)
	feature.BooleanResult.Regions = regions[:1]
	edit(feature)
	check(1000, true)
	base.Length = 12
	edit(base)
	check(1200, true)
	// Shorten the tool below the target: the two separated pieces become one.
	// The old references must remain failed, never silently keep the merged region.
	tool.Length = 5
	edit(tool)
	var failed workspace.Feature
	for _, f := range view.Part.Features {
		if f.ID == feature.ID {
			failed = f
		}
	}
	if failed.EvaluationStatus != "FAILED" || failed.BooleanResult == nil {
		t.Fatalf("missing region failure definition: %+v", failed)
	}
	repaired := analyze(feature.ID)
	var replacement []modelcore.BooleanRegionRef
	for _, c := range repaired.Regions {
		if c.Eligible {
			replacement = append(replacement, c.Reference)
		}
	}
	if len(replacement) != 1 {
		t.Fatal("expected connected repair candidate", len(replacement))
	}
	failed.BooleanResult = &modelcore.BooleanResultSelection{Mode: "EXPLICIT", Regions: replacement}
	edit(failed)
	check(3100, true)
	if view.Part.Features[len(view.Part.Features)-1].ID != feature.ID {
		t.Fatal("repair replaced feature identity")
	}
	apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: base.BodyID})
	pick := findP7TopologyPick(t, service, view, "EDGE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return p.GeometryType == "LINE"
	})
	modifier := workspace.Feature{Type: "CHAMFER", BodyID: base.BodyID, Length: 0.25, Selections: []workspace.FeatureSelection{{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID}}}
	r := workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &modifier, ActorID: p6Actor, RequestID: doc + "-downstream"}
	candidate, e := service.PreviewCommand(t.Context(), doc, r)
	if e != nil || candidate.Artifact == nil {
		t.Fatal("downstream naming", e)
	}
	r.PreviewID = candidate.PreviewID
	view, e = service.ApplyCommand(t.Context(), doc, r)
	if e != nil {
		t.Fatal("downstream commit", e)
	}
	pick = findP7TopologyPick(t, service, view, "EDGE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return p.GeometryType == "LINE"
	})
	fillet := workspace.Feature{Type: "FILLET", BodyID: base.BodyID, Length: 0.1, Selections: []workspace.FeatureSelection{{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID}}}
	r = workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &fillet, ActorID: p6Actor, RequestID: doc + "-downstream-fillet"}
	candidate, e = service.PreviewCommand(t.Context(), doc, r)
	if e != nil || candidate.Artifact == nil {
		t.Fatal("downstream fillet", e)
	}
	r.PreviewID = candidate.PreviewID
	view, e = service.ApplyCommand(t.Context(), doc, r)
	if e != nil {
		t.Fatal("downstream fillet commit", e)
	}
	fresh := workspace.NewWithArtifacts(db, client, store)
	reopened, e := fresh.GetDocument(t.Context(), doc, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(view.Part.Features, reopened.Part.Features) || view.Document.VersionID != reopened.Document.VersionID {
		t.Fatal("reopen changed definitions/history")
	}
	last := view.Part.Features[len(view.Part.Features)-1]
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), doc, workspace.CommandRequest{ActorID: p6Actor, RequestID: doc + "-cold-rebuild", Type: "EDIT_FEATURE", TargetID: last.ID, Feature: &last, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, last.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold exact Boolean rebuild", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != activeBodyArtifact(t, view).Representations[role].Digest {
			t.Fatal("cold result changed", role)
		}
	}
	// Bind the region result through the ordinary assembly support route. Two
	// occurrences of this same Part must retain distinct constraint scopes.
	face := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return p.GeometryType == "PLANE"
	})
	product, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "Boolean retained region assembly"})
	if e != nil {
		t.Fatal(e)
	}
	productApply := func(r workspace.CommandRequest) {
		t.Helper()
		sequence++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-region-assembly-%d", doc, sequence)
		product, e = service.ApplyCommand(t.Context(), product.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
	}
	productApply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: doc, Name: "First"})
	first := product.Product.Instances[0].ID
	productApply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: doc, Name: "Second"})
	second := product.Product.Instances[1].ID
	productApply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: first, Kind: "BODY"}})
	a := workspace.AssemblyGeometryRef{InstanceID: first, Kind: "FACE", SourceVersionID: view.Document.VersionID, PersistentSelection: &face.Selection}
	b := a
	b.InstanceID = second
	productApply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", DirectionRelation: "SAME", FirstAssemblyRef: &a, SecondAssemblyRef: &b})
	if len(product.Product.Constraints) != 2 {
		t.Fatal("missing assembly constraints")
	}
	constraint := product.Product.Constraints[1]
	if constraint.First.InstanceID == constraint.Second.InstanceID {
		t.Fatal("repeated Part support scope was collapsed")
	}
	if _, e := service.AnalyzeBooleanRegions(t.Context(), doc, workspace.BooleanRegionsRequest{VersionID: head, BodyID: base.BodyID, Tool: workspace.FeatureStageRef{BodyID: tool.BodyID, FeatureID: tool.ID}, Operation: "REMOVE"}); e == nil {
		t.Fatal("accepted stale analysis")
	}
}
