//go:build integration

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssemblySTEPDocumentImport(t *testing.T) {
	root, err := config.ProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                             string
		definitions, instances, maxEmpty int
	}{
		{"Bench Vise", 22, 24, 0}, {"Jackhammer", 185, 288, 5}, {"Walking Robot", 18, 191, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExchangeFixture(t)
			name := tc.name + ".step"
			data, err := os.ReadFile(filepath.Join(root, "tests", "test.data", name))
			if err != nil {
				t.Fatal(err)
			}
			job, err := f.importFile(t, name, data)
			if err != nil || job.State != "SUCCEEDED" || job.DocumentID == nil {
				t.Fatalf("import %s: state=%s err=%v", name, job.State, err)
			}
			var payload struct{ Exchange exchange.Report }
			if err := json.Unmarshal(job.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			report := payload.Exchange
			seen := map[string]bool{}
			instances, empty, parts := 0, 0, 0
			var indicator workspace.DocumentView
			var visit func(string)
			visit = func(id string) {
				if seen[id] {
					return
				}
				seen[id] = true
				v, err := f.h.workspace.GetDocument(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				if v.Product != nil {
					instances += len(v.Product.Instances)
					for _, inst := range v.Product.Instances {
						if inst.ReferenceMode != "PINNED" || inst.ReferencedVersionID == "" {
							t.Fatal("unversioned occurrence")
						}
						visit(inst.ReferencedDocumentID)
					}
					return
				}
				if v.Part == nil || id == *job.DocumentID {
					t.Fatal("expected assembly root and Part leaves")
				}
				parts++
				if len(v.Part.Features) == 0 {
					empty++
					found := false
					for _, w := range report.Warnings {
						if w.DefinitionID != "" && strings.HasPrefix(w.Detail, v.Document.Name+": ") {
							found = true
						}
					}
					if !found {
						t.Fatalf("missing source warning for %s", v.Document.Name)
					}
				} else {
					feature := v.Part.Features[0]
					key := v.Part.Bodies[0].GeometryKey
					if feature.Result != nil {
						key = feature.Result.GeometryKey
					}
					a := v.Artifacts[key]
					if a.BRepBytes == 0 || a.GLBBytes == 0 || !a.Naming.CanBind {
						t.Fatalf("missing accepted geometry for %s", v.Document.Name)
					}
				}
				if len(v.Part.DatumPlanes) != 3 || len(v.Part.AxisSystems) != 1 {
					t.Fatal("missing built-in references")
				}
				for _, plane := range v.Part.DatumPlanes {
					if plane.Visible == nil || *plane.Visible {
						t.Fatal("visible imported plane", v.Document.Name)
					}
				}
				if a := v.Part.AxisSystems[0]; a.Visible == nil || *a.Visible {
					t.Fatal("visible imported axis system", v.Document.Name)
				}
				if tc.name == "Bench Vise" && v.Document.Name == "INDICATOR" {
					indicator = v
				}
			}
			visit(*job.DocumentID)
			t.Logf("definitions=%d instances=%d parts=%d empty=%d warnings=%+v", len(seen), instances, parts, empty, report.Warnings)
			if len(seen) != tc.definitions || report.Definitions != tc.definitions || instances != tc.instances || empty != report.EmptyParts || empty > tc.maxEmpty {
				t.Fatal("source structure or report mismatch")
			}
			if tc.name == "Bench Vise" {
				verifyImportedIndicatorFillet(t, f, indicator)
			}
		})
	}
}

func verifyImportedIndicatorFillet(t *testing.T, f exchangeFixture, v workspace.DocumentView) {
	t.Helper()
	if v.Part == nil {
		t.Fatal("missing indicator")
	}
	body := v.Part.Bodies[0]
	// Locator 23 belongs to the frozen BREP of this fixture. Use its bound
	// persistent selection through preview and commit.
	s := f.h.workspace
	edge, err := s.GetTopologyElementPropertiesAtVersion(t.Context(), v.Document.ID, v.Document.VersionID, body.GeometryKey, "EDGE", 23)
	if err != nil || edge.PersistentSelection == nil || edge.GeometryType != "CIRCLE" {
		t.Fatal("rim selection", err)
	}
	req := workspace.CommandRequest{ActorID: f.actor, RequestID: uuid.NewString(), Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: body.ID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: *edge.PersistentSelection, SourceVersionID: v.Document.VersionID}}}}
	if _, err := s.PreviewCommand(t.Context(), v.Document.ID, req); err == nil || !strings.Contains(err.Error(), "MODIFIER_ALGORITHM_FAILED:FILLET") {
		t.Fatal("oversized rim", err)
	}
	req.RequestID = uuid.NewString()
	req.Feature.Length = .25
	preview, err := s.PreviewCommand(t.Context(), v.Document.ID, req)
	if err != nil || preview.PreviewID == "" {
		t.Fatal("preview", err)
	}
	req.PreviewID = preview.PreviewID
	accepted, err := s.ApplyCommand(t.Context(), v.Document.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	last := accepted.Part.Features[len(accepted.Part.Features)-1]
	key := accepted.Part.Bodies[0].GeometryKey
	if last.Type != "FILLET" || last.Length != req.Feature.Length || key == body.GeometryKey || !accepted.Artifacts[key].Naming.CanBind {
		t.Fatal("invalid fillet commit")
	}
}
