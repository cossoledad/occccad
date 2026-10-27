//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func TestFilletUntouchedTopologyThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	v, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Fillet retains upstream edge identities"})
	if e != nil {
		t.Fatal(e)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-naming-%d", v.Document.ID, seq)
		next, e := service.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		v = next
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := v.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	base := v.Part.Features[len(v.Part.Features)-1]
	selected := findP7TopologyPick(t, service, v, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return strings.HasPrefix(s.Anchor.OutputSlot, "VERTICAL_FROM_PROFILE_ENDPOINTS/") && s.CreationEvidence.Centroid == [3]float64{10, 10, 10}
	})
	type witness struct {
		anchor   modelcore.SemanticTopologyRef
		centroid [3]float64
	}
	var untouched []witness
	for local := uint64(1); local <= p6TopologyCount(t, activeBodyArtifact(t, v).Topology, "edges"); local++ {
		p, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, activeBodyArtifact(t, v).GeometryKey, "EDGE", local)
		if e != nil {
			t.Fatal(e)
		}
		s := p.PersistentSelection
		if s != nil && strings.HasPrefix(s.Anchor.OutputSlot, "VERTICAL_FROM_PROFILE_ENDPOINTS/") && s.Anchor.OutputSlot != selected.Selection.Anchor.OutputSlot {
			untouched = append(untouched, witness{s.Anchor, s.CreationEvidence.Centroid})
		}
	}
	if len(untouched) != 3 {
		t.Fatal("expected three unmodified vertical edges")
	}
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: selected.Selection, SourceVersionID: selected.SourceVersionID}}}})
	fillet := v.Part.Features[len(v.Part.Features)-1]
	check := func() {
		t.Helper()
		a := activeBodyArtifact(t, v)
		_, reader, e := artifacts.Open(t.Context(), a.Representations["VISUAL"].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		bytes, e := io.ReadAll(reader)
		reader.Close()
		if e != nil {
			t.Fatal(e)
		}
		_, raw, e := visual.Decode(bytes)
		if e != nil {
			t.Fatal(e)
		}
		var metadata workspace.VisualizationManifest
		if e = json.Unmarshal(raw, &metadata); e != nil || metadata.FeatureAssociations == nil {
			t.Fatal("association metadata", e)
		}
		for _, want := range untouched {
			found := false
			for local := uint64(1); local <= p6TopologyCount(t, a.Topology, "edges"); local++ {
				p, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, a.GeometryKey, "EDGE", local)
				if e != nil {
					t.Fatal(e)
				}
				s := p.PersistentSelection
				if s == nil || s.CreationEvidence.Centroid != want.centroid {
					continue
				}
				found = true
				if s.Anchor.FeatureID != want.anchor.FeatureID || s.Anchor.OutputSlot != want.anchor.OutputSlot || !slices.Equal(s.Anchor.SourceIDs, want.anchor.SourceIDs) {
					t.Fatal("untouched edge renamed", want, s.Anchor)
				}
				associated := false
				for _, item := range metadata.FeatureAssociations.Elements {
					if item.Kind == "EDGE" && item.LocalID == local {
						associated = true
						if !slices.Equal(item.Primary, []string{base.ID}) || slices.Contains(item.Modifiers, fillet.ID) || slices.Contains(item.Origins, fillet.ID) {
							t.Fatal("copy falsely attributed to fillet", item)
						}
					}
				}
				if !associated {
					t.Fatal("edge association missing")
				}
			}
			if !found {
				t.Fatal("untouched edge missing", want)
			}
		}
	}
	check()
	changed := fillet
	changed.Length = 2
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: fillet.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, fillet.ID)})
	check()
	apply(workspace.CommandRequest{Type: "UNDO"})
	check()
	apply(workspace.CommandRequest{Type: "REDO"})
	check()
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	opened, e := fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	v = opened
	check()
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-cold", Type: "EDIT_FEATURE", TargetID: fillet.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, fillet.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	final := activeBodyArtifact(t, v)
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold mismatch", role)
		}
	}
	opposite := findP7TopologyPick(t, service, v, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.CreationEvidence.Centroid == [3]float64{10, -10, 10}
	})
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: base.BodyID, Length: .2, Selections: []workspace.FeatureSelection{{Selection: opposite.Selection, SourceVersionID: opposite.SourceVersionID}}}})
	last := v.Part.Features[len(v.Part.Features)-1]
	if last.Selections[0].Selection.Anchor.FeatureID != base.ID || last.EvaluationStatus != "" || math.IsNaN(activeBodyArtifact(t, v).Volume) {
		t.Fatal("downstream edge not editable")
	}
}
