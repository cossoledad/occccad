//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func TestLoftMergedSectionAssociationsThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	v, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Loft generated-side associations"})
	if e != nil {
		t.Fatal(e)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-loft-association-%d", v.Document.ID, seq)
		next, e := service.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		v = next
	}
	sections := []workspace.LoftSection{}
	for _, z := range []float64{0, 30} {
		request := workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"}
		if z != 0 {
			apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			request.Plane = ""
			request.DatumPlaneID = v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID
		}
		apply(request)
		sketch := v.Part.Features[len(v.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -30, Y: -30}, Second: &workspace.SketchPoint2{X: 30, Y: 30}}}})
		sections = append(sections, workspace.LoftSection{SketchID: sketch})
	}
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: sections}})
	loft := v.Part.Features[len(v.Part.Features)-1]
	if len(v.Part.Bodies) != 2 {
		t.Fatal("fixture must reproduce a new Body", v.Part.Bodies)
	}
	check := func(a workspace.Artifact) {
		t.Helper()
		_, reader, e := artifacts.Open(t.Context(), a.Representations["VISUAL"].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(reader)
		reader.Close()
		if e != nil {
			t.Fatal(e)
		}
		_, metadata, e := visual.Decode(raw)
		if e != nil {
			t.Fatal(e)
		}
		var display workspace.VisualizationManifest
		if e = json.Unmarshal(metadata, &display); e != nil {
			t.Fatal(e)
		}
		if display.FeatureAssociations == nil {
			t.Fatal("missing GLB associations")
		}
		faces, merged := 0, 0
		for _, item := range display.FeatureAssociations.Elements {
			if !slices.Equal(item.Primary, []string{loft.ID}) || !slices.Equal(item.Origins, []string{loft.ID}) || item.Status != "MAPPED" {
				t.Fatal("loft output assigned to section inputs", item)
			}
			if item.Kind != "FACE" {
				continue
			}
			faces++
			p, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, a.GeometryKey, "FACE", item.LocalID)
			if e != nil {
				t.Fatal(e)
			}
			if p.PersistentSelection == nil || p.PersistentSelection.Anchor.FeatureID != loft.ID {
				t.Fatal("authoritative identity changed", p)
			}
			if strings.HasPrefix(p.PersistentSelection.Anchor.OutputSlot, "MERGED_FROM/") {
				merged++
				for _, s := range sections {
					if !slices.Contains(item.Supports, s.SketchID) {
						t.Fatal("lost actual section support", item)
					}
				}
			}
		}
		if faces != 6 || merged != 4 {
			t.Fatal("expected four merged side faces and two generated caps", faces, merged)
		}
	}
	check(activeBodyArtifact(t, v))
	changed := loft
	changed.Ruled = true
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: loft.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, loft.ID)})
	check(activeBodyArtifact(t, v))
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(activeBodyArtifact(t, v))
	apply(workspace.CommandRequest{Type: "REDO"})
	check(activeBodyArtifact(t, v))
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	v, e = fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	check(activeBodyArtifact(t, v))
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-cold", Type: "EDIT_FEATURE", TargetID: loft.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, loft.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold rebuild", e)
	}
	check(*cold.Artifact)
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != activeBodyArtifact(t, v).Representations[role].Digest {
			t.Fatal("cold rebuild changed authority", role)
		}
	}
}
