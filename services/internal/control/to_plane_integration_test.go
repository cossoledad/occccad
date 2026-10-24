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

func TestToPlaneDatumLifecycleThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	for _, remove := range []bool{false, true} {
		t.Run(fmt.Sprintf("remove=%v", remove), func(t *testing.T) {
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "To plane lifecycle"})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				if r.RequestID == "" {
					r.RequestID = fmt.Sprintf("%s-plane-%d", view.Document.ID, seq)
				}
				view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			endZ, startDatum, operation := 10.0, "datum-xy", "NEW_BODY"
			if remove {
				apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				sketch := view.Part.Features[0].ID
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -20, Y: -20}, Second: &workspace.SketchPoint2{X: 20, Y: 20}}}})
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
				apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Start", Origin: [3]float64{0, 0, 30}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
				startDatum = view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
				endZ = 20
				operation = "REMOVE"
			}
			apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "End", Origin: [3]float64{0, 0, endZ}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: startDatum})
			sketch := view.Part.Features[len(view.Part.Features)-1].ID
			circle := func(id string, x, r float64) workspace.SketchOperation {
				return workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: x}, Radius: r}}
			}
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{circle("outer", 0, 4), circle("hole", 0, 2), {Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 8, Y: -2}, Second: &workspace.SketchPoint2{X: 12, Y: 2}}}})
			ids := append([]string(nil), view.SketchAnalyses[sketch].RegionIDs...)
			create := workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-preview", Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: operation, Extent: "TO_PLANE", TerminationPlaneID: plane.ID, Reversed: remove, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: ids}}

			// Invalid geometry remains an atomic preview rejection, not a stale candidate.
			for name, target := range map[string]string{"parallel": "datum-xz", "zero": startDatum, "backwards": plane.ID} {
				invalid := create
				invalid.RequestID = view.Document.ID + "-invalid-" + name
				invalid.TerminationPlaneID = target
				if name == "backwards" {
					invalid.Reversed = !remove
				}
				if _, e := service.PreviewCommand(t.Context(), view.Document.ID, invalid); e == nil || !strings.Contains(e.Error(), "TO_PLANE_") {
					t.Fatal("invalid extent accepted", name, e)
				}
			}
			head := view.Document.VersionID
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, create)
			if e != nil || preview.Artifact == nil {
				t.Fatal("parallel preview", e)
			}
			current, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || current.Document.VersionID != head {
				t.Fatal("preview advanced Head", e)
			}
			create.PreviewID = preview.PreviewID
			apply(create)
			feature := view.Part.Features[len(view.Part.Features)-1]
			saved := func(id string) workspace.Feature {
				t.Helper()
				for _, f := range view.Part.Features {
					if f.ID == id {
						return f
					}
				}
				t.Fatal("missing feature")
				return workspace.Feature{}
			}
			stage := func() workspace.Artifact {
				t.Helper()
				r, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: feature.ID, ResultStage: true})
				if e != nil {
					t.Fatal(e)
				}
				return r.Artifact
			}
			want := func(tool float64) float64 {
				if remove {
					return 48000 - tool
				}
				return tool
			}
			if math.Abs(stage().Volume-want(120*math.Pi+160)) > 1e-5 {
				t.Fatal("wrong parallel result", stage().Volume)
			}

			if _, e := service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-delete-plane", Type: "DELETE_NODE", TargetKind: "DATUM_PLANE", TargetID: plane.ID}); e == nil {
				t.Fatal("referenced termination datum deleted")
			}
			cap := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
				return s.Anchor.FeatureID == feature.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
			})
			if _, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-datum-cycle", Type: "EDIT_DATUM_PLANE", TargetID: plane.ID, Normal: plane.Normal, UDirection: plane.UDirection, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "TOPOLOGY", FeatureID: feature.ID, Selection: &cap.Selection, SourceVersionID: view.Document.VersionID}}}); e == nil {
				t.Fatal("termination datum/feature cycle accepted")
			}
			if remove {
				input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: feature.ID, BodyID: feature.BodyID})
				if e != nil {
					t.Fatal(e)
				}
				var bottom *workspace.FeatureSelection
				for local := uint64(1); local <= p6TopologyCount(t, input.Artifact.Topology, "faces"); local++ {
					r, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: feature.ID, BodyID: feature.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "FACE", LocalID: local})
					if e == nil && strings.HasPrefix(r.Selection.Selection.Anchor.OutputSlot, "START_CAP/") {
						bottom = r.Selection
						break
					}
				}
				if bottom == nil {
					t.Fatal("same-Body target not found")
				}
				through := feature
				through.TerminationPlaneID = ""
				through.TerminationPlane = bottom
				preview, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-same-body", Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &through, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
				if e != nil || preview.Artifact == nil || math.Abs(preview.Artifact.Volume-(48000-30*(12*math.Pi+16))) > 1e-5 {
					t.Fatal("same-Body face termination", e)
				}
			}
			edge := findP7TopologyPick(t, service, view, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
				return s.Anchor.FeatureID == feature.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_BOUNDARY_FROM_PROFILE_EDGE/") && s.CreationEvidence.GeometryType == "LINE"
			})
			kind := "CHAMFER"
			if remove {
				kind = "FILLET"
			}
			apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: kind, BodyID: feature.BodyID, Length: .2, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
			modifier := view.Part.Features[len(view.Part.Features)-1]
			change := workspace.CommandRequest{Type: "EDIT_DATUM_PLANE", TargetID: plane.ID, Origin: plane.Origin, Normal: [3]float64{-.25, 0, 1}, UDirection: [3]float64{1, 0, .25}}
			apply(change)
			expected := 120*math.Pi + 200
			if remove {
				expected = 120*math.Pi + 120
			}
			if math.Abs(stage().Volume-want(expected)) > 1e-5 {
				t.Fatal("target rotation did not update exact result", stage().Volume, want(expected))
			}
			if saved(modifier.ID).EvaluationStatus != "" {
				t.Fatal("target update lost modifier references", saved(modifier.ID))
			}
			edited := saved(modifier.ID)
			edited.Length = .3
			apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: edited.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, edited.ID), Feature: &edited})
			// A valid datum becoming parallel is a retained downstream model failure.
			invalid := change
			invalid.Normal = [3]float64{1, 0, 0}
			invalid.UDirection = [3]float64{0, 1, 0}
			apply(invalid)
			if saved(feature.ID).EvaluationStatus != "FAILED" || !strings.Contains(saved(feature.ID).Diagnostic, "TO_PLANE_DIRECTION_PARALLEL") || saved(modifier.ID).EvaluationStatus != "BLOCKED" {
				t.Fatal("invalid termination not diagnosed", saved(feature.ID))
			}
			apply(change)
			if saved(feature.ID).EvaluationStatus != "" || saved(modifier.ID).EvaluationStatus != "" {
				t.Fatal("termination restore failed")
			}
			result := saved(feature.ID)
			if result.ID != feature.ID || result.BodyID != feature.BodyID || result.Name != feature.Name || result.Order != feature.Order || !reflect.DeepEqual(result.ProfileSelection, feature.ProfileSelection) {
				t.Fatal("association update changed identity or regions")
			}
			final := activeBodyArtifact(t, view)
			apply(workspace.CommandRequest{Type: "UNDO"})
			if saved(feature.ID).EvaluationStatus != "FAILED" {
				t.Fatal("undo lost invalid termination")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			if activeBodyArtifact(t, view).GeometryID != final.GeometryID {
				t.Fatal("redo differs")
			}
			fresh := workspace.NewWithArtifacts(db, client, artifacts)
			opened, e := fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || activeBodyArtifact(t, opened).GeometryID != final.GeometryID {
				t.Fatal("reopen differs", e)
			}
			r := workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_FEATURE", RequestID: view.Document.ID + "-cold", TargetID: modifier.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, modifier.ID), Feature: &edited}
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, r)
			if e != nil || cold.Artifact == nil {
				t.Fatal("cold", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
					t.Fatal("cold differs", role)
				}
			}
		})
	}
}

func TestToPlaneFaceRepairThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "To plane face repair"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-face-%d", view.Document.ID, seq)
		}
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	saved := func(id string) workspace.Feature {
		t.Helper()
		for _, f := range view.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("missing feature")
		return workspace.Feature{}
	}
	rectangle := func(x, y, w, h float64) string {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		s := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: s, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + w, Y: y + h}}}})
		return s
	}
	baseSketch := rectangle(-5, -5, 10, 10)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: baseSketch, Generator: "LINEAR_EXTRUDE", Length: 10, Operation: "ADD"})
	base := view.Part.Features[len(view.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	tipSketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: tipSketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "source-circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 2}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: tipSketch, Generator: "LINEAR_EXTRUDE", Length: 20, Operation: "ADD"})
	tip := view.Part.Features[len(view.Part.Features)-1]
	face := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == base.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	curved := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return p.GeometryType == "CYLINDER"
	})
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	body := view.Part.ActiveBodyID
	// The selected region is outside the finite target face. Its support plane is infinite.
	sketch := rectangle(10, -2, 4, 4)
	regions := append([]string(nil), view.SketchAnalyses[sketch].RegionIDs...)
	if _, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-nonplanar", Type: "CREATE_SOLID_FEATURE", SketchID: sketch, BodyID: body, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Extent: "TO_PLANE", TerminationPlane: &workspace.FeatureSelection{Selection: curved.Selection, SourceVersionID: curved.SourceVersionID}}); e == nil {
		t.Fatal("non-planar target accepted")
	}
	pick := &workspace.FeatureSelection{Selection: face.Selection, SourceVersionID: face.SourceVersionID}
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, BodyID: body, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Extent: "TO_PLANE", TerminationPlane: pick, ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: regions}})
	feature := view.Part.Features[len(view.Part.Features)-1]
	if feature.TerminationPlane.SourceFeatureID != tip.ID || feature.TerminationPlane.SourceFeatureID == feature.TerminationPlane.Selection.Anchor.FeatureID {
		t.Fatal("source stage is not the actual Body tip", feature.TerminationPlane)
	}
	if math.Abs(activeBodyArtifact(t, view).Volume-160) > 1e-6 {
		t.Fatal("finite face boundary incorrectly constrained extent")
	}
	base.Length = 12
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: base.ID, Feature: &base, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, base.ID)})
	if math.Abs(activeBodyArtifact(t, view).Volume-192) > 1e-6 {
		t.Fatal("face move did not update termination")
	}
	for _, id := range []string{base.ID, tip.ID} {
		if _, e := service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-delete-" + id, Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: id}); e == nil {
			t.Fatal("target source deletion accepted")
		}
	}
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: base.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, base.ID), Suppressed: &suppressed})
	if saved(feature.ID).EvaluationStatus != "FAILED" || !strings.Contains(saved(feature.ID).Diagnostic, "TO_PLANE_TARGET_UNRESOLVED") {
		t.Fatal("lost target not retained", saved(feature.ID))
	}
	failedHead := view.Document.VersionID
	input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: feature.ID})
	if e != nil || input.TerminationPick == nil || input.TerminationPick.Status == modelcore.SelectionResolved || len(input.Artifacts) < 2 {
		t.Fatal("lost target prevents inspecting valid upstream stage", e, input)
	}
	// Re-saving an unresolved definition is not an explicit repair.
	broken := saved(feature.ID)
	if _, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-unrepaired", Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &broken, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)}); e == nil {
		t.Fatal("unresolved target accepted")
	}
	upstream, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: feature.ID, BodyID: base.BodyID})
	if e != nil || upstream.SourceFeatureID != tip.ID {
		t.Fatal("upstream unavailable", e)
	}
	for local := uint64(1); local <= p6TopologyCount(t, upstream.Artifact.Topology, "faces"); local++ {
		bound, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: failedHead, FeatureID: feature.ID, BodyID: base.BodyID, GeometryKey: upstream.Artifact.GeometryKey, Kind: "FACE", LocalID: local})
		if e == nil && strings.HasPrefix(bound.Selection.Selection.Anchor.OutputSlot, "END_CAP/") {
			broken.TerminationPlane = bound.Selection
			break
		}
	}
	if broken.TerminationPlane.SourceVersionID != failedHead || broken.TerminationPlane.SourceFeatureID != tip.ID {
		t.Fatal("replacement not bound to current legal stage")
	}
	repair := workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-repair", Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &broken, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)}
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, repair)
	if e != nil || preview.Artifact == nil {
		t.Fatal("repair preview", e)
	}
	current, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || current.Document.VersionID != failedHead {
		t.Fatal("cancel moved Head", e)
	}
	stale := repair
	modified := broken
	modified.Reversed = true
	stale.Feature = &modified
	stale.PreviewID = preview.PreviewID
	if _, e = service.ApplyCommand(t.Context(), view.Document.ID, stale); e == nil {
		t.Fatal("stale preview adopted")
	}
	repair.PreviewID = preview.PreviewID
	apply(repair)
	final := activeBodyArtifact(t, view)
	result := saved(feature.ID)
	if math.Abs(final.Volume-320) > 1e-6 || result.ID != feature.ID || result.Order != feature.Order || result.BodyID != feature.BodyID || result.Name != feature.Name || !reflect.DeepEqual(result.ProfileSelection, feature.ProfileSelection) {
		t.Fatal("repair identity/geometry", result, final.Volume)
	}

	// Generator-tool/range replication cannot silently re-execute this extent.
	for _, kind := range []string{"GENERATOR_TOOL", "FEATURE_DELTA"} {
		pattern := &workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 2, Spacing: 20, Direction: [3]float64{1, 0, 0}}, Source: workspace.FeatureStageRef{BodyID: body, FeatureID: feature.ID}, SourceKind: kind, ResultMode: "COMBINE"}}
		wantError := "PATTERN_REEXECUTION_UNSUPPORTED"
		if kind == "FEATURE_DELTA" {
			pattern.Pattern.StartFeatureID = feature.ID
			wantError = "PATTERN_RANGE_INVALID"
		}
		if _, e = service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-pattern-" + kind, Type: "CREATE_PATTERN", Feature: pattern}); e == nil || !strings.Contains(e.Error(), wantError) {
			t.Fatal("to-plane tool replication accepted", kind, e)
		}
	}
	// BODY_STAGE copies the already evaluated solid, including its inclined cap.
	for _, kind := range []string{"LINEAR", "MIRROR"} {
		definition := workspace.PatternDefinition{Kind: kind, Distribution: "FIXED_STEP", Count: 2, Spacing: 20, Direction: [3]float64{1, 0, 0}}
		if kind == "MIRROR" {
			definition = workspace.PatternDefinition{Kind: "MIRROR", Distribution: "FIXED_STEP", Count: 2, MirrorPlaneID: "datum-yz", Direction: [3]float64{1, 0, 0}}
		}
		pattern := &workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: definition, Source: workspace.FeatureStageRef{BodyID: body, FeatureID: feature.ID}, SourceKind: "BODY_STAGE", ResultMode: "COMBINE"}}
		copied, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-copy-" + kind, Type: "CREATE_PATTERN", Feature: pattern})
		if e != nil || copied.Artifact == nil || math.Abs(copied.Artifact.Volume-2*final.Volume) > 1e-5 {
			t.Fatal("evaluated Body copy", kind, e)
		}
	}
	// Downstream geometry must not be promoted to an upstream target.
	own := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == feature.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	cycle := result
	cycle.TerminationPlane = &workspace.FeatureSelection{Selection: own.Selection, SourceVersionID: own.SourceVersionID, SourceFeatureID: feature.ID}
	if _, e = service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-cycle", Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &cycle, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)}); e == nil {
		t.Fatal("self target accepted")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if saved(feature.ID).EvaluationStatus != "FAILED" {
		t.Fatal("undo lost failed definition")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	opened, e := fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || activeBodyArtifact(t, opened).GeometryID != final.GeometryID {
		t.Fatal("reopen", e)
	}
	repair.PreviewID = ""
	repair.RequestID = view.Document.ID + "-cold"
	repair.ExpectedFeatureDigest = findP6FeatureDigest(t, view.StructureTree, feature.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, repair)
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold mismatch", role)
		}
	}
}
