package control

import (
	"context"
	"fmt"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// Real Router/Worker/DB for all geometry and healthy solves. Only the numerical
// RPC boundary is fault-injected; no mock can manufacture a satisfying pose.
func TestAssemblyDeferredDefinitionPreviewCommit(t *testing.T) {
	var fault, calls atomic.Int32
	interceptor := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		if strings.HasSuffix(method, "/SolveAssembly") {
			calls.Add(1)
			switch fault.Load() {
			case 1:
				return status.Error(codes.DeadlineExceeded, "injected solve child deadline")
			case 2:
				return status.Error(codes.Unavailable, "injected solve transport unavailable")
			case 4:
				return status.Error(codes.PermissionDenied, "injected solve permission failure")
			case 5:
				return context.Canceled
			case 3:
				if err := invoke(ctx, method, req, reply, cc, options...); err != nil {
					return err
				}
				response := reply.(*workerv1.SolveAssemblyResponse)
				response.Status = "NUMERICAL_FAILURE"
				response.Diagnostic = "injected numerical failure"
				return nil
			}
		}
		return invoke(ctx, method, req, reply, cc, options...)
	}
	f := newCompositionControlFixture(t, grpc.WithUnaryInterceptor(interceptor))
	part := f.importPart(t, "cylinder-r6-h12")
	t.Run("invalid and canceled RPC cannot become definition candidates on retry", func(t *testing.T) {
		fault.Store(0)
		p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "rejected request"})
		if err != nil {
			t.Fatal(err)
		}
		p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []int32{4, 5} {
			fault.Store(mode)
			request := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ActorID: f.actor, RequestID: fmt.Sprintf("%s-reject-%d", p.Document.ID, mode), ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: p.Product.Instances[0].ID, Kind: "BODY"}}
			for range 2 {
				preview, err := f.service.PreviewCommand(t.Context(), p.Document.ID, request)
				if err == nil || preview.PreviewID != "" {
					t.Fatal("invalid RPC became persistable", preview, err)
				}
			}
			current, err := f.service.GetDocument(t.Context(), p.Document.ID)
			if err != nil || current.Document.VersionID != p.Document.VersionID || len(current.Product.Constraints) != 0 {
				t.Fatal("rejected request advanced Head", err)
			}
		}
	})
	for mode := int32(1); mode <= 3; mode++ {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			fault.Store(0)
			p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "deferred definition"})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(c workspace.CommandRequest) workspace.DocumentView {
				t.Helper()
				c.ActorID = f.actor
				if c.RequestID == "" {
					c.RequestID = p.Document.ID + p.Document.VersionID + c.Type
				}
				v, e := f.service.ApplyCommand(t.Context(), p.Document.ID, c)
				if e != nil {
					t.Fatal(e)
				}
				p = v
				return v
			}
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			poses := p.Product.Instances
			base := p.Document.VersionID
			command := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ActorID: f.actor, RequestID: p.Document.ID + "-deferred", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: poses[0].ID, Kind: "BODY"}}
			fault.Store(mode)
			calls.Store(0)
			preview, err := f.service.PreviewCommand(t.Context(), p.Document.ID, command)
			if err != nil || preview.PreviewID == "" || preview.EvaluationOutcome != "DEFINITION_ONLY" || preview.EvaluationFailure == nil || !preview.EvaluationFailure.Retryable || preview.EvaluationFailure.Phase != "SOLVING" {
				t.Fatal("definition candidate missing", preview, err)
			}
			if calls.Load() != 1 {
				t.Fatal("failure repeated by empty accepted-set solve", calls.Load())
			}
			current, err := f.service.GetDocument(t.Context(), p.Document.ID)
			if err != nil || current.Document.VersionID != base {
				t.Fatal("preview changed Head", err)
			}
			command.PreviewID = preview.PreviewID
			apply(command)
			if calls.Load() != 1 || !reflect.DeepEqual(p.Product.Instances, poses) || p.Product.Constraints[0].EvaluationStatus != "NOT_UPDATED" || p.Product.Constraints[0].Suppressed || p.Product.Constraints[0].EvaluationFailure == nil {
				t.Fatal("failed evidence adopted", p.Product, calls.Load())
			}
			saved := p.Document.VersionID
			apply(command)
			if p.Document.VersionID != saved {
				t.Fatal("retry generated another Revision")
			}
			if _, err = f.service.CreateProductRelease(t.Context(), p.Document.ID, workspace.CreateProductReleaseRequest{ActorID: f.actor, RequestID: saved + "-release", Name: "not ready"}); err == nil {
				t.Fatal("NotUpdated released as satisfied")
			}
			cold, err := f.service.GetDocument(t.Context(), p.Document.ID)
			if err != nil || cold.Product.Constraints[0].EvaluationFailure == nil {
				t.Fatal("cold failure lost", err)
			}
			fault.Store(0)
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			if p.Product.Constraints[0].EvaluationStatus != "VERIFIED" || p.Product.Constraints[0].EvaluationFailure != nil {
				t.Fatal("healthy update did not recover", p.Product)
			}
			// Editing Verified stores the new intent, not its old successful evaluation.
			fault.Store(mode)
			calls.Store(0)
			edit := workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", ActorID: f.actor, RequestID: saved + "-edit", TargetID: p.Product.Constraints[0].ID, FixMode: "RELATIVE"}
			preview, err = f.service.PreviewCommand(t.Context(), p.Document.ID, edit)
			if err != nil || preview.EvaluationOutcome != "DEFINITION_ONLY" {
				t.Fatal("edit candidate", preview, err)
			}
			edit.PreviewID = preview.PreviewID
			before := p.Product.Instances
			apply(edit)
			if p.Product.Constraints[0].FixMode != "RELATIVE" || p.Product.Constraints[0].EvaluationStatus != "NOT_UPDATED" || !reflect.DeepEqual(before, p.Product.Instances) || calls.Load() != 1 {
				t.Fatal("edited definition not atomic", p.Product, calls.Load())
			}
			fault.Store(0)
			apply(workspace.CommandRequest{Type: "UNDO"})
			if p.Product.Constraints[0].FixMode != "SPACE" {
				t.Fatal("undo definition")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			if p.Product.Constraints[0].FixMode != "RELATIVE" || p.Product.Constraints[0].EvaluationStatus != "NOT_UPDATED" {
				t.Fatal("redo failure outcome", p.Product)
			}
			// A failed addition beside an accepted definition preserves the
			// independent old evidence and all authoritative component poses.
			apply(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			before = p.Product.Instances
			fault.Store(mode)
			calls.Store(0)
			addition := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ActorID: f.actor, RequestID: saved + "-second", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: before[1].ID, Kind: "BODY"}}
			preview, err = f.service.PreviewCommand(t.Context(), p.Document.ID, addition)
			if err != nil || preview.EvaluationOutcome != "DEFINITION_ONLY" {
				t.Fatal("accepted-set failure lost pending definition", preview, err)
			}
			addition.PreviewID = preview.PreviewID
			wrong := addition
			wrong.FixMode = "RELATIVE"
			if _, err = f.service.ApplyCommand(t.Context(), p.Document.ID, wrong); err == nil {
				t.Fatal("definition candidate accepted changed intent")
			}
			apply(addition)
			if calls.Load() != 1 || !reflect.DeepEqual(before, p.Product.Instances) || p.Product.Constraints[0].EvaluationStatus != "VERIFIED" || p.Product.Constraints[1].EvaluationStatus != "NOT_UPDATED" {
				t.Fatal("accepted-set failure changed old solution", p.Product, calls.Load())
			}
			fault.Store(0)
		})
	}
}
