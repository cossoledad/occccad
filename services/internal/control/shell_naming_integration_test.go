//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func TestShellInteriorOwnershipThroughRouter(t *testing.T) {
	service, store, db, client, _ := featureAssociationRuntimeService(t)
	for _, round := range []bool{false, true} {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			v, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Shell ownership"})
			if e != nil {
				t.Fatal(e)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				r.RequestID = fmt.Sprintf("%s-shell-%d", v.Document.ID, seq)
				v, e = service.ApplyCommand(t.Context(), v.Document.ID, r)
				if e != nil {
					t.Fatal(r.Type, e)
				}
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			sketch := v.Part.Features[len(v.Part.Features)-1].ID
			op := workspace.SketchOperation{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}
			if round {
				op = workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "aaaaaaaa-0000-4000-8000-000000000001", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 10}}
			}
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{op}})
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
			base := v.Part.Features[len(v.Part.Features)-1]
			pick := findP7TopologyPick(t, service, v, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
				return math.Abs(s.CreationEvidence.Centroid[2]-20) < 1e-6
			})
			shell := workspace.Feature{Type: "SHELL", BodyID: base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID}}}
			apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &shell})
			shell = v.Part.Features[len(v.Part.Features)-1]
			check := func(thickness float64) {
				t.Helper()
				a := activeBodyArtifact(t, v)
				_, reader, e := store.Open(t.Context(), a.Representations["VISUAL"].ObjectID)
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
				var manifest workspace.VisualizationManifest
				if e = json.Unmarshal(metadata, &manifest); e != nil {
					t.Fatal(e)
				}
				if manifest.FeatureAssociations == nil {
					t.Fatal("missing authoritative display associations")
				}
				count := 0
				for _, item := range manifest.FeatureAssociations.Elements {
					if item.Kind != "FACE" && item.Kind != "EDGE" {
						continue
					}
					p, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, a.GeometryKey, item.Kind, item.LocalID)
					if e != nil || p.PersistentSelection == nil {
						t.Fatal("topology", e)
					}
					evidence := p.PersistentSelection.CreationEvidence
					c := evidence.Centroid
					if item.Kind == "FACE" {
						inner := math.Abs(c[2]-thickness) < 1e-6
						if round {
							inner = inner || (evidence.RadiusMM != nil && math.Abs(*evidence.RadiusMM-(10-thickness)) < 1e-6)
						} else {
							inner = inner || math.Abs(math.Abs(c[0])-(10-thickness)) < 1e-6 || math.Abs(math.Abs(c[1])-(10-thickness)) < 1e-6
						}
						owner := base.ID
						if inner {
							owner = shell.ID
							count++
						}
						if p.PersistentSelection.Anchor.FeatureID != owner || !slices.Equal(item.Primary, []string{owner}) {
							t.Fatalf("face authority/display disagree or ownership wrong: inner=%v anchor=%+v association=%+v", inner, p.PersistentSelection.Anchor, item)
						}
					} else {
						outer := evidence.RadiusMM != nil && math.Abs(*evidence.RadiusMM-10) < 1e-6
						if !round {
							outer = math.Abs(math.Abs(c[0])-10) < 1e-6 || math.Abs(math.Abs(c[1])-10) < 1e-6
						}
						if outer && (p.PersistentSelection.Anchor.FeatureID != base.ID || !slices.Equal(item.Primary, []string{base.ID})) {
							t.Fatal("retained outside edge claimed by shell", item, p.PersistentSelection.Anchor)
						}
					}
				}
				want := 5
				if round {
					want = 2
				}
				if count != want {
					t.Fatal("interior missing", count, want)
				}
			}
			check(1)
			changed := shell
			changed.Length = 2
			apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: shell.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, shell.ID)})
			check(2)
			apply(workspace.CommandRequest{Type: "UNDO"})
			check(1)
			apply(workspace.CommandRequest{Type: "REDO"})
			check(2)
			fresh := workspace.NewWithArtifacts(db, client, store)
			v, e = fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			check(2)
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: v.Document.ID + "-shell-cold", Type: "EDIT_FEATURE", TargetID: shell.ID, Feature: &changed, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, shell.ID)})
			if e != nil || cold.Artifact == nil {
				t.Fatal(e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != activeBodyArtifact(t, v).Representations[role].Digest {
					t.Fatal("cold mismatch", role)
				}
			}
		})
	}
}
