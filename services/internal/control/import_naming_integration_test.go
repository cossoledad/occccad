package control

import (
	"fmt"
	"math"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func verifyImportedNaming(t *testing.T, db *database.Pool, client *geometry.Client, artifacts *artifact.Service, initial workspace.DocumentView) {
	t.Helper()
	service := workspace.NewWithArtifacts(db, client, artifacts)
	part, err := service.GetDocument(t.Context(), initial.Document.ID)
	if err != nil || !part.Artifact.Naming.CanBind || part.Part.Features[0].ImportDefinitionID == "" {
		t.Fatalf("cold imported naming: %v %+v", err, part.Artifact)
	}
	base := part
	sequence := 0
	apply := func(id string, req workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		sequence++
		req.RequestID = fmt.Sprintf("import-naming-%s-%d", initial.Document.ID, sequence)
		req.ActorID = p6Actor
		result, err := service.ApplyCommand(t.Context(), id, req)
		if err != nil {
			t.Fatalf("import %s: %v", req.Type, err)
		}
		return result
	}
	// All topology types must be bindable with independent opaque roots.
	ids := map[string]bool{}
	for _, group := range []struct {
		kind  string
		count int
	}{{"FACE", 6}, {"EDGE", 12}, {"VERTEX", 8}} {
		for i := 1; i <= group.count; i++ {
			selection, err := service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{SourceVersionID: part.Document.VersionID, GeometryKey: part.Artifact.GeometryKey, Kind: group.kind, LocalID: uint64(i)})
			if err != nil || len(selection.Anchor.SourceIDs) != 1 {
				t.Fatalf("bind imported %s %d: %+v %v", group.kind, i, selection, err)
			}
			id := selection.Anchor.SourceIDs[0]
			if ids[id] {
				t.Fatalf("duplicate stable identity %s", id)
			}
			ids[id] = true
		}
	}
	for _, normal := range [][3]float64{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
		pick := findP7TopologyPick(t, service, part, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
			n, ok := p.Properties["normal"].([3]float64)
			return ok && n == normal
		})
		for _, operation := range []string{"ADD", "REMOVE"} {
			part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: pick.GeometryKey, TopologyID: pick.LocalID, VersionID: pick.SourceVersionID})
			sketch := part.Part.Features[len(part.Part.Features)-1]
			support := sketch.Sketch.Support
			if support.Normal != normal {
				t.Fatalf("import oriented normal %v != %v", support.Normal, normal)
			}
			u, n := support.XDirection, support.Normal
			v := [3]float64{n[1]*u[2] - n[2]*u[1], n[2]*u[0] - n[0]*u[2], n[0]*u[1] - n[1]*u[0]}
			var x, y float64
			for i := 0; i < 3; i++ {
				d := pick.Selection.CreationEvidence.Centroid[i] - support.Origin[i]
				x += d * u[i]
				y += d * v[i]
			}
			first, second := workspace.SketchPoint2{X: x - 1, Y: y - 1}, workspace.SketchPoint2{X: x + 1, Y: y + 1}
			part = apply(part.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &first, Second: &second}}})
			part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", SketchID: sketch.ID, Operation: operation, Length: 1, Reversed: operation == "REMOVE"})
			expected := base.Artifact.Volume + 4
			if operation == "REMOVE" {
				expected = base.Artifact.Volume - 4
			}
			if !part.Artifact.Naming.CanBind || math.Abs(part.Artifact.Volume-expected) > 1e-5 {
				t.Fatalf("%s on %v: volume %g naming %+v", operation, normal, part.Artifact.Volume, part.Artifact.Naming)
			}
			resolution, err := service.ResolvePersistentSelection(t.Context(), part.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID, TargetVersionID: part.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
			if err != nil || resolution.Status != modelcore.SelectionResolved {
				t.Fatalf("import lineage resolution: %+v %v", resolution, err)
			}
			for i := 0; i < 3; i++ {
				part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})
			}
		}
	}
	// A symmetric groove splits the original top face. Never select a nearest half.
	top := findP7TopologyPick(t, service, part, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		n, ok := p.Properties["normal"].([3]float64)
		return ok && n == [3]float64{0, 0, 1}
	})
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: top.GeometryKey, TopologyID: top.LocalID, VersionID: top.SourceVersionID})
	splitSketch := part.Part.Features[len(part.Part.Features)-1]
	frame := splitSketch.Sketch.Support
	var sx, sy float64
	for i := 0; i < 3; i++ {
		sx += (top.Selection.CreationEvidence.Centroid[i] - frame.Origin[i]) * frame.XDirection[i]
	}
	sy = (top.Selection.CreationEvidence.Centroid[0]-frame.Origin[0])*(-frame.XDirection[1]) + (top.Selection.CreationEvidence.Centroid[1]-frame.Origin[1])*frame.XDirection[0]
	a, b := workspace.SketchPoint2{X: sx - 1, Y: sy - 11}, workspace.SketchPoint2{X: sx + 1, Y: sy + 11}
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: splitSketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &a, Second: &b}}})
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Generator: "LINEAR_EXTRUDE", SketchID: splitSketch.ID, Operation: "REMOVE", Length: 1, Reversed: true})
	split, err := service.ResolvePersistentSelection(t.Context(), part.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: top.Selection, SourceVersionID: top.SourceVersionID, TargetVersionID: part.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if err != nil || split.Status != modelcore.SelectionAmbiguous || len(split.Candidates) != 2 {
		t.Fatalf("symmetric split must remain ambiguous: %+v %v", split, err)
	}
	for i := 0; i < 3; i++ {
		part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})
	}
	// Projection and constraints consume the same exact persistent references.
	edge := findP7TopologyPick(t, service, part, "EDGE", func(p workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return p.GeometryType == "LINE" && math.Abs(selection.CreationEvidence.Direction[2]) < 0.5
	})
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketchID := part.Part.Features[len(part.Part.Features)-1].ID
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "ADD_EXTERNAL_GEOMETRY", GeometryKey: edge.GeometryKey, TopologyKind: "EDGE", TopologyID: edge.LocalID, SourceVersionID: edge.SourceVersionID}}})
	if len(part.Part.Features[len(part.Part.Features)-1].Sketch.ExternalGeometry) != 1 {
		t.Fatal("import edge projection missing")
	}
	edge = findP7TopologyPick(t, service, part, "EDGE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return p.GeometryType == "LINE"
	})
	product, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "Import naming assembly"})
	if err != nil {
		t.Fatal(err)
	}
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	first := product.Product.Instances[0].ID
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	second := product.Product.Instances[1].ID
	product = apply(product.Document.ID, p7Coincident(p7AssemblyPick(first, edge), p7AssemblyPick(second, edge)))
	constraint := product.Product.Constraints[len(product.Product.Constraints)-1]
	assertP7Constraint(t, product, constraint.ID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected)
	for i := 0; i < 2; i++ {
		part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})
	}
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	assertP7Constraint(t, product, constraint.ID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected)
}
