package automation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/assistant"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type fixedACL struct{ role access.Role }

func (a fixedACL) RequireDocument(_ context.Context, _, _ string, role access.Role) (access.Role, error) {
	if a.role.Level() < role.Level() {
		return "", access.ErrForbidden
	}
	return a.role, nil
}

type snapshotReader struct {
	snapshots map[string]workspace.ModelSnapshot
	head      string
	calls     int
}

func (r *snapshotReader) ReadModelSnapshot(_ context.Context, _, revision string) (workspace.ModelSnapshot, error) {
	r.calls++
	if revision == "" {
		revision = r.head
	}
	value, ok := r.snapshots[revision]
	if !ok {
		return value, workspace.ErrNotFound
	}
	return value, nil
}
func (*snapshotReader) PreviewCommand(context.Context, string, workspace.CommandRequest) (workspace.CommandPreview, error) {
	panic("unexpected evaluation")
}
func (*snapshotReader) ExecuteCommand(context.Context, string, workspace.CommandRequest) error {
	panic("unexpected mutation")
}
func (*snapshotReader) FindCommandReceipt(context.Context, string, string, string) (workspace.CommandReceipt, error) {
	return workspace.CommandReceipt{}, workspace.ErrNotFound
}
func (*snapshotReader) DiscardPreview(string, string, string) {}

func TestAutomationCatalogContractsAndValidation(t *testing.T) {
	catalog := automation.NewCatalog()
	again := automation.NewCatalog()
	if catalog.Digest != again.Digest {
		t.Fatal("catalog digest is unstable")
	}
	ops, err := catalog.Search("extrude", "PART", 10)
	if err != nil {
		t.Fatalf("discovery: %+v %v", ops, err)
	}
	found := map[string]bool{}
	for _, op := range ops {
		found[op.ID] = true
	}
	if !found["part.extrude"] || !found["command.create_solid_feature"] {
		t.Fatalf("missing matching operations: %+v", ops)
	}
	op, err := catalog.Describe("part.extrude")
	if err != nil || op.InputSchema["additionalProperties"] != false || op.OutputSchema == nil {
		t.Fatalf("operation has no checked contract: %+v %v", op, err)
	}
	valid := json.RawMessage(`{"bodyId":"body-stable","sketchId":"sketch-stable","length":12,"unit":"mm"}`)
	command, err := catalog.Compile(op.ID, 1, valid)
	if err != nil || command.Type != "PAD_SKETCH" || command.Length != 12 {
		t.Fatalf("compile: %+v %v", command, err)
	}
	for _, raw := range []string{`null`, `{"bodyId":"b"}`, `{"bodyId":"b","sketchId":"s","length":-1,"unit":"mm"}`, `{"bodyId":"b","sketchId":"s","length":12,"unit":"m"}`, `{"bodyId":"b","sketchId":"s","length":12,"unit":"mm","actorId":"forged"}`, string(valid) + ` {}`} {
		if _, err = catalog.Compile(op.ID, 1, json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid arguments accepted: %s", raw)
		}
	}
	if _, err = catalog.Compile(op.ID, 2, valid); err == nil {
		t.Fatal("unsupported schema accepted")
	}
	if _, err = catalog.Compile(op.ID, 1, json.RawMessage(`{"bodyId":"b","sketchId":"s","length":12,"length":99,"unit":"mm"}`)); err == nil {
		t.Fatal("duplicate quantity argument accepted")
	}
}

func TestAutomationBoundedQueryAndContextWithFiftyThousandNodes(t *testing.T) {
	document, revision, nextRevision, workspaceID, actor := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	part := workspace.PartModel{Units: "mm", ActiveBodyID: "body", Bodies: []workspace.PartBody{{ID: "body", Name: "Body"}}}
	for i := range 50000 {
		part.Features = append(part.Features, workspace.Feature{ID: fmt.Sprintf("feature-%06d", i), Name: "Repeated name", Type: "PAD", BodyID: "body", Length: 12})
	}
	part.Parameters = []modelcore.ParameterDefinition{{ParameterID: "length-parameter", OwnerFeatureID: "feature-000000", Key: "length_1", Label: "Length", DisplayUnit: "mm"}}
	raw, _ := json.Marshal(part)
	snapshot := workspace.ModelSnapshot{CommandBaseline: workspace.CommandBaseline{WorkspaceID: workspaceID, RevisionID: revision, Sequence: 1}, DocumentID: document, DocumentType: "PART", ModelHash: "hash", Model: raw}
	reader := &snapshotReader{snapshots: map[string]workspace.ModelSnapshot{revision: snapshot}, head: revision}
	service := automation.New(nil, reader, fixedACL{access.RoleOwner})
	query := automation.Query{Kind: "FEATURE", Limit: 3, Fields: []string{"name", "length"}}
	page, err := service.Query(t.Context(), actor, document, query)
	if err != nil || page.MatchedCount != 50000 || len(page.Items) != 3 || !page.HasMore || page.Complete || len(page.NextCursor) > 2048 {
		t.Fatalf("bounded query: %+v %v", page, err)
	}
	next := snapshot
	next.RevisionID = nextRevision
	next.Sequence = 2
	next.Model = json.RawMessage(`{"features":[]}`)
	reader.snapshots[nextRevision] = next
	reader.head = nextRevision
	query.Cursor = page.NextCursor
	page2, err := service.Query(t.Context(), actor, document, query)
	if err != nil || page2.Snapshot.RevisionID != revision || page2.Items[0].Ref.ID == page.Items[0].Ref.ID || page2.MatchedCount != 50000 || page2.Complete {
		t.Fatalf("cursor did not retain immutable snapshot: %+v %v", page2, err)
	}
	query.Name = "changed filter"
	if _, err = service.Query(t.Context(), actor, document, query); err == nil {
		t.Fatal("cursor accepted different filter")
	}
	parameterPage, err := service.Query(t.Context(), actor, document, automation.Query{Kind: "PARAMETER", BodyID: "body", RevisionID: revision, Fields: []string{"bodyId", "ownerId", "parameterAlias"}})
	if err != nil || parameterPage.MatchedCount != 1 || parameterPage.Items[0].Properties["ownerId"] != "feature-000000" {
		t.Fatalf("Body query lost formally owned parameters: %+v %v", parameterPage, err)
	}
	query = automation.Query{Kind: "FEATURE", ID: page.Items[0].Ref.ID, RevisionID: revision, Fields: []string{"name"}}
	exact, err := service.Query(t.Context(), actor, document, query)
	if err != nil || len(exact.Items) != 1 || !exact.Complete {
		t.Fatalf("exact read failed: %+v %v", exact, err)
	}
	host := assistant.NewHost(service, actor, document, false)
	packet, err := host.Context(t.Context(), "", []string{"part.extrude"}, 8192)
	if err != nil || packet.Bytes > 8192 || len(packet.Operations) != 1 {
		t.Fatalf("bounded context failed: %d %v", packet.Bytes, err)
	}
	if _, err = host.Context(t.Context(), "", []string{"sketch.create", "sketch.rectangle", "part.extrude", "parameter.set_literal", "parameter.set_expression"}, 2048); err == nil {
		t.Fatal("context exceeded budget without an explicit failure")
	}
	forged := host.Call(t.Context(), assistant.Call{ID: "forged", Name: "cad_context", Arguments: json.RawMessage(`{"documentId":"another-document"}`)})
	if !forged.IsError {
		t.Fatal("model replaced the host-bound document")
	}
	dispatched := host.Call(t.Context(), assistant.Call{ID: "read", Name: "cad_query_model", Arguments: json.RawMessage(`{"kind":"FEATURE","revisionId":"` + revision + `","limit":1}`)})
	if dispatched.IsError || len(dispatched.Content) > 8192 {
		t.Fatal("bounded tool dispatch failed", string(dispatched.Content))
	}
	result := host.Call(t.Context(), assistant.Call{ID: "call", Name: "cad_create_task", Arguments: json.RawMessage(`{}`)})
	if !result.IsError {
		t.Fatal("read-only model host admitted a mutation")
	}
	before := reader.calls
	denied := automation.New(nil, reader, fixedACL{access.RoleViewer})
	if _, err = denied.Prepare(t.Context(), actor, document, automation.ActionRequest{}); !errors.Is(err, access.ErrForbidden) || reader.calls != before {
		t.Fatal("mutation did not check authorization before model access", err)
	}
}

type sourceACL struct{ target string }

func (a sourceACL) RequireDocument(_ context.Context, document, _ string, role access.Role) (access.Role, error) {
	if document == a.target {
		return access.RoleOwner, nil
	}
	return "", access.ErrForbidden
}
func TestAutomationSourcePermissionsBeforeEvaluation(t *testing.T) {
	actor, target, source := uuid.NewString(), uuid.NewString(), uuid.NewString()
	reader := &snapshotReader{}
	service := automation.New(nil, reader, sourceACL{target: target})
	for _, operation := range []string{"command.insert_instance", "command.replace_instance", "command.set_parameter_external", "command.create_context_reference"} {
		args := map[string]any{}
		switch operation {
		case "command.insert_instance":
			args["referencedDocumentId"] = source
		case "command.replace_instance":
			args["instanceId"] = "instance"
			args["versionId"] = uuid.NewString()
			args["replacementVersionId"] = uuid.NewString()
			args["referencedDocumentId"] = source
		case "command.set_parameter_external":
			args = map[string]any{"parameterId": "parameter", "sourceDocumentId": source, "publicationId": "publication", "referenceMode": "FOLLOW_HEAD"}
		case "command.create_context_reference":
			args["sourceDocumentId"] = source
		}
		raw, _ := json.Marshal(args)
		_, err := service.Prepare(t.Context(), actor, target, automation.ActionRequest{OperationID: operation, Version: 1, Arguments: raw, RequestID: uuid.NewString(), ExpectedHead: workspace.CommandBaseline{WorkspaceID: uuid.NewString(), RevisionID: uuid.NewString(), Sequence: 1}})
		if !errors.Is(err, access.ErrForbidden) {
			t.Fatal("source permission was bypassed", operation, err)
		}
	}
	if reader.calls != 0 {
		t.Fatal("source authorization evaluated or read the target model")
	}
}
