//go:build integration

package main

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

func (f exchangeFixture) command(t *testing.T, id string, req workspace.CommandRequest) workspace.DocumentView {
	t.Helper()
	req.ActorID = f.actor
	req.RequestID = uuid.NewString()
	view, err := f.h.workspace.ApplyCommand(t.Context(), id, req)
	if err != nil {
		t.Fatalf("%s: %v", req.Type, err)
	}
	return view
}
func (f exchangeFixture) document(t *testing.T, kind, name string) workspace.DocumentView {
	t.Helper()
	folder := f.folder
	v, err := f.h.workspace.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, RequestID: uuid.NewString(), Type: kind, Name: name, FolderID: &folder})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func (f exchangeFixture) exported(t *testing.T, graph *geometry.ExchangeGraph, format string) geometry.ArtifactReference {
	t.Helper()
	id := uuid.NewString()
	ref, err := f.h.geometry.ExportExchange(t.Context(), id, format, artifact.StagingKey(id, "output"), graph)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := f.h.artifacts.Adopt(t.Context(), artifact.KindExchangeSource, ref.ContentType, ref.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	return geometry.ArtifactReference{Backend: obj.Backend, ObjectKey: obj.Key, SHA256: obj.SHA256, Size: obj.Size, ContentType: obj.ContentType}
}
func TestExchangeExportFiltersHiddenGeometryAndOccurrences(t *testing.T) {
	f := newExchangeFixture(t)
	part := f.document(t, "PART", "Visibility fixture")
	id := part.Document.ID
	addBox := func(x float64) string {
		t.Helper()
		body := part.Part.ActiveBodyID
		part = f.command(t, id, workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
		sketch := part.Part.Features[len(part.Part.Features)-1].ID
		part = f.command(t, id, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: 0}, Second: &workspace.SketchPoint2{X: x + 10, Y: 10}}}})
		part = f.command(t, id, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: body, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
		return body
	}
	first := addBox(0)
	part = f.command(t, id, workspace.CommandRequest{Type: "CREATE_BODY"})
	second := addBox(30)
	part = f.command(t, id, workspace.CommandRequest{Type: "SET_BODY_VISIBILITY", BodyID: second, Visible: false})
	line := workspace.Feature{Type: "GEOMETRY", Geometry: &workspace.GeometricFeatureDefinition{Kind: "LINE", Curve: &workspace.GeometryCurveDefinition{Points: []workspace.GeometryControlPoint{{ID: "start", Position: [3]float64{0, 20, 0}}, {ID: "end", Position: [3]float64{50, 20, 0}}}}}}
	part = f.command(t, id, workspace.CommandRequest{Type: "CREATE_GEOMETRY_FEATURE", Feature: &line})
	feature := part.Part.Features[len(part.Part.Features)-1]
	part = f.command(t, id, workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "GEOMETRICAL_SET", TargetID: feature.GeometricalSetID, Visible: false})
	graph, err := f.h.workspace.ExchangeExportGraph(t.Context(), id, workspace.ExchangeExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Definitions) != 1 || len(graph.Definitions[0].Bodies) != 1 || len(graph.Definitions[0].Geometries) != 0 {
		t.Fatal("visible-only Part included hidden Body/result")
	}
	all, err := f.h.workspace.ExchangeExportGraph(t.Context(), id, workspace.ExchangeExportOptions{Visibility: "ALL"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Definitions[0].Bodies) != 2 || len(all.Definitions[0].Geometries) != 1 || all.Definitions[0].Bodies[1].GetVisible() || all.Definitions[0].Geometries[0].GetVisible() {
		t.Fatal("ALL lost hidden state/content")
	}
	for _, schema := range []string{"AP242", "AP214IS"} {
		graph.StepSchema = schema
		ref := f.exported(t, graph, "STEP")
		inspected, err := f.h.geometry.InspectExchange(t.Context(), uuid.NewString(), "STEP", ref)
		if err != nil {
			t.Fatal(err)
		}
		expected := "AP242_MANAGED_MODEL_BASED_3D_ENGINEERING"
		if schema == "AP214IS" {
			expected = "AUTOMOTIVE_DESIGN"
		}
		if !strings.Contains(inspected.SourceSchema, expected) {
			t.Fatalf("wrong schema %s: %s", schema, inspected.SourceSchema)
		}
	}
	// BREP has no presentation/hierarchy channel, but uses the same selection.
	f.exported(t, graph, "BREP")
	product := f.document(t, "PRODUCT", "Shared instances")
	for range 2 {
		product = f.command(t, product.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: id})
	}
	if len(product.ReferenceDisplayScopes) != 2 || len(product.ReferenceDisplayScopes[0].Results) != 1 {
		t.Fatal("Product omitted independent geometry result")
	}
	a, b := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	path := func(instance string) *workspace.InstancePath {
		for _, scope := range product.ReferenceDisplayScopes {
			if scope.InstancePath.Canonical == instance {
				p := scope.InstancePath
				return &p
			}
		}
		t.Fatal("scope missing")
		return nil
	}
	product = f.command(t, product.Document.ID, workspace.CommandRequest{Type: "SET_OCCURRENCE_VISIBILITY", InstancePath: path(a), TargetKind: "BODY", TargetID: second, VisibilityMode: "SHOW"})
	product = f.command(t, product.Document.ID, workspace.CommandRequest{Type: "SET_OCCURRENCE_VISIBILITY", InstancePath: path(b), TargetKind: "INSTANCE", TargetID: b, VisibilityMode: "HIDE"})
	visible, err := f.h.workspace.ExchangeExportGraph(t.Context(), product.Document.ID, workspace.ExchangeExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defs, children := 0, 0
	for _, d := range visible.Definitions {
		if d.Kind == "PART" {
			defs++
			if len(d.Bodies) != 2 {
				t.Fatal("occurrence SHOW did not override source Body")
			}
		} else {
			children += len(d.Children)
		}
	}
	if defs != 1 || children != 1 {
		t.Fatalf("hidden instance not pruned: Parts=%d occurrences=%d", defs, children)
	}
	included, err := f.h.workspace.ExchangeExportGraph(t.Context(), product.Document.ID, workspace.ExchangeExportOptions{Visibility: "ALL"})
	if err != nil {
		t.Fatal(err)
	}
	if len(included.Definitions[len(included.Definitions)-1].Children) != 2 {
		t.Fatal("ALL lost occurrence")
	}
	ref := f.exported(t, included, "STEP")
	inspected, err := f.h.geometry.InspectExchange(t.Context(), uuid.NewString(), "STEP", ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspected.Definitions) != 3 {
		t.Fatalf("distinct visibility variants were merged: %v", inspected)
	}
	if len(included.Warnings) != 1 || included.Warnings[0].Code != "STEP_VISIBILITY_LIMITATION" {
		t.Fatal("ALL silently promised unsupported visibility roundtrip")
	}
	part = f.command(t, id, workspace.CommandRequest{Type: "SET_BODY_VISIBILITY", BodyID: first, Visible: false})
	if _, err := f.h.workspace.ExchangeExportGraph(t.Context(), id, workspace.ExchangeExportOptions{}); err == nil || !strings.Contains(err.Error(), "EXCHANGE_NO_EXPORTABLE_CONTENT") {
		t.Fatal("empty visible selection was not explicit", err)
	}
}

func TestDirectSTEPImportRetainsSelectedDefinitionBeyondCacheCapacity(t *testing.T) {
	f := newExchangeFixture(t)
	graph := &geometry.ExchangeGraph{}
	for n := 1; n <= 32; n++ {
		id := uuid.NewString()
		evaluation, err := f.h.geometry.EvaluatePartFromArtifact(t.Context(), id, id, []geometry.RectangularPad{{Width: float64(n), Height: 2, Length: 3, Plane: "XY"}}, geometry.ArtifactReference{}, artifact.StagingKey(id, "source.brep"), artifact.StagingKey(id, "source.glb"))
		if err != nil {
			t.Fatal(err)
		}
		obj, err := f.h.artifacts.Adopt(t.Context(), artifact.KindBREP, "application/vnd.opencascade.brep", evaluation.BrepArtifact.ObjectKey)
		if err != nil {
			t.Fatal(err)
		}
		graph.Definitions = append(graph.Definitions, &workerv1.ExchangeDefinition{Id: id, Name: fmt.Sprintf("Part %d", n), Kind: "PART", Brep: &workerv1.ArtifactReference{Backend: obj.Backend, ObjectKey: obj.Key, Sha256: obj.SHA256, SizeBytes: uint64(obj.Size), ContentType: obj.ContentType}})
		graph.Roots = append(graph.Roots, &workerv1.ExchangeOccurrence{Id: id, DefinitionId: id, Name: fmt.Sprintf("Part %d", n), Rotation: &workerv1.Quaternion{W: 1}})
	}
	source := f.exported(t, graph, "STEP")
	inspected, err := f.h.geometry.InspectExchange(t.Context(), uuid.NewString(), "STEP", source)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.KernelVersion == "" || inspected.KernelVersion == "routed" {
		t.Fatal("missing actual preparation kernel version")
	}
	target := ""
	for _, def := range inspected.Definitions {
		if def.Name == "Part 1" {
			target = def.Id
			break
		}
	}
	if target == "" {
		t.Fatal("source definition missing")
	}
	id := uuid.NewString()
	evaluated, err := f.h.geometry.ImportExchange(t.Context(), id, id, "STEP", source, target, artifact.StagingKey(id, "import.brep"), artifact.StagingKey(id, "import.glb"))
	if err != nil {
		t.Fatal(err)
	}
	if evaluated.GetTopology().GetSolidCount() != 1 || math.Abs(evaluated.Volume-6) > 1e-7 {
		t.Fatal("wrong selected definition", evaluated.GetTopology(), evaluated.Volume)
	}
}
