package control

import (
	"bytes"
	"context"
	"encoding/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssemblyDiagnosticNestedSourcesThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	insert := func(sourceID string) workspace.DocumentView {
		t.Helper()
		product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Diagnostic nesting"})
		if err != nil {
			t.Fatal(err)
		}
		product, err = f.service.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "INSERT_INSTANCE", ReferencedDocumentID: sourceID})
		if err != nil {
			t.Fatal(err)
		}
		return product
	}
	child := insert(part.Document.ID)
	root := insert(child.Document.ID)
	bundle, err := f.service.ExportAssemblyDiagnostic(t.Context(), root.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	var file geometry.AssemblyReplay
	if err := json.Unmarshal(bundle, &file); err != nil {
		t.Fatal(err)
	}
	var snapshot workspace.AssemblyDiagnosticSnapshot
	if err := json.Unmarshal(file.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Documents) != 2 {
		t.Fatal("nested source identity closure incomplete", len(snapshot.Documents))
	}
	for _, source := range snapshot.Documents {
		if source.DocumentID != part.Document.ID {
			continue
		}
		if source.RevisionID != part.Document.VersionID || len(source.BodyIDs) != len(part.Part.Bodies) || len(source.BodyIDs) == 0 {
			t.Fatal("Part revision or CAD Body identity lost")
		}
	}
	if bytes.Contains(bundle, []byte(`"model":`)) || bytes.Contains(bundle, []byte(`"manifest":`)) {
		t.Fatal("default diagnostic contains duplicated numerical input or full Part history")
	}

	current, err := f.service.GetDocument(t.Context(), root.Document.ID)
	if err != nil || current.Document.VersionID != root.Document.VersionID {
		t.Fatal("nested export changed head", err)
	}
}

func TestAssemblyDiagnosticMissingSupportDoesNotBlockExportThroughRouter(t *testing.T) {
	var unavailable atomic.Bool
	f := newCompositionControlFixture(t, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if unavailable.Load() && strings.HasSuffix(method, "/GetTopology") {
			return status.Error(codes.NotFound, "diagnostic fixture exact support missing")
		}
		return invoke(ctx, method, req, reply, cc, opts...)
	}))
	part := f.importPart(t, "cylinder-r6-h12")
	product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Partial diagnostic compilation"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		r.ActorID = f.actor
		product, err = f.service.ApplyCommand(t.Context(), product.Document.ID, r)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"A", "B"} {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name})
	}
	a, b := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "BODY"}})
	first, second := f.support(t, part, "PLANE"), f.support(t, part, "PLANE")
	first.InstanceID, second.InstanceID = a, b
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: &first, SecondAssemblyRef: &second, DirectionRelation: "SAME"})
	head := product.Document.VersionID
	unavailable.Store(true)
	bundle, err := f.service.ExportAssemblyDiagnostic(t.Context(), product.Document.ID)
	if err != nil {
		t.Fatal("one unresolved constraint blocked export", err)
	}
	var file geometry.AssemblyReplay
	var snapshot workspace.AssemblyDiagnosticSnapshot
	if err = json.Unmarshal(bundle, &file); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(file.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Definitions) != 2 || len(snapshot.CompileFailures) < 2 || len(snapshot.Geometry) != 0 || len(snapshot.Primitives) != 1 {
		t.Fatal("unresolved endpoints lost or fabricated geometry", string(bundle))
	}
	out, err := workspace.ReplayAssemblyDiagnostic(t.Context(), f.client, bundle, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(out, &file)
	if file.Outcome != "INPUT_ERROR" {
		t.Fatal("missing support reported as a successful solve", string(out))
	}
	unavailable.Store(false)
	current, err := f.service.GetDocument(t.Context(), product.Document.ID)
	if err != nil || current.Document.VersionID != head {
		t.Fatal("diagnostic compilation changed formal state", err)
	}
	detailed, err := f.service.ExportAssemblyDiagnosticWithOptions(t.Context(), product.Document.ID, workspace.AssemblyDiagnosticExportOptions{DetailedNaming: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = workspace.ReplayAssemblyDiagnostic(t.Context(), f.client, detailed, false)
	if err != nil {
		t.Fatal("optional Naming table references invalid", err)
	}
}
