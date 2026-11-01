package workspace

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
)

func TestProductParameterCommandsPreviewPublicationAndHistory(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "product-parameters.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	s := NewWithArtifacts(db, nil, nil)
	view, err := s.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PRODUCT", Name: "parameters", ActorID: access.DefaultUserID})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	run := func(request CommandRequest) {
		t.Helper()
		seq++
		request.ActorID = access.DefaultUserID
		request.RequestID = fmt.Sprintf("parameter-%d", seq)
		request.VersionID = view.Document.VersionID
		updated, err := s.ApplyCommand(t.Context(), view.Document.ID, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
		view = updated
	}
	run(CommandRequest{Type: "CREATE_PARAMETER", Name: "Width", Value: 10, Unit: "mm"})
	if len(view.Product.Parameters) != 1 || view.Product.Parameters[0].EvaluatedValue.SIValue != .01 {
		t.Fatal(view.Product.Parameters)
	}
	id := view.Product.Parameters[0].ParameterID
	previewInput := CommandRequest{Type: "EDIT_PARAMETER", ParameterID: id, Name: "Width", Value: 20, Unit: "mm", ActorID: access.DefaultUserID, VersionID: view.Document.VersionID, RequestID: "parameter-preview"}
	preview, err := s.PreviewCommand(t.Context(), view.Document.ID, previewInput)
	if err != nil || preview.PreviewID == "" || len(preview.ParameterCandidates) != 1 || preview.ParameterCandidates[0].EvaluatedValue.SIValue != .02 {
		t.Fatal(preview, err)
	}
	previewInput.PreviewID = preview.PreviewID
	run(previewInput)
	run(CommandRequest{Type: "UNDO"})
	if view.Product.Parameters[0].EvaluatedValue.SIValue != .01 {
		t.Fatal("undo", view.Product.Parameters)
	}
	run(CommandRequest{Type: "REDO"})
	if view.Product.Parameters[0].EvaluatedValue.SIValue != .02 {
		t.Fatal("redo", view.Product.Parameters)
	}
	run(CommandRequest{Type: "CREATE_PUBLICATION", TargetKind: "PARAMETER", PublicationType: "PARAMETER", TargetID: id})
	if len(view.Product.Publications) != 1 || view.Product.Publications[0].Resolution.Status != "CONNECTED" || view.Product.Publications[0].Resolution.Value.SIValue != .02 {
		t.Fatal("Product parameter publication", view.Product.Publications)
	}
	other, err := s.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PRODUCT", Name: "consumer", ActorID: access.DefaultUserID})
	if err != nil {
		t.Fatal(err)
	}
	source := view
	view = other
	run(CommandRequest{Type: "CREATE_PARAMETER", Name: "ImportedWidth", Value: 1, Unit: "mm"})
	run(CommandRequest{Type: "SET_PARAMETER_EXTERNAL", ParameterID: view.Product.Parameters[0].ParameterID, SourceDocumentID: source.Document.ID, PublicationID: source.Product.Publications[0].ID, ReferenceMode: "PINNED"})
	if view.Product.Parameters[0].EvaluatedValue.SIValue != .02 {
		t.Fatal("cross-document Product parameter", view.Product.Parameters)
	}
	run(CommandRequest{Type: "SET_PARAMETER_EXTERNAL", ParameterID: view.Product.Parameters[0].ParameterID, SourceDocumentID: source.Document.ID, PublicationID: source.Product.Publications[0].ID, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT"})
	consumer := view
	view = source
	run(CommandRequest{Type: "CREATE_PARAMETER", Name: "Unrelated", Value: 4, Unit: "mm"})
	consumer, err = s.GetDocument(t.Context(), consumer.Document.ID, access.DefaultUserID)
	if err != nil || len(consumer.ReferenceUpdates) != 1 || consumer.ReferenceUpdates[0].Status != "CURRENT" {
		t.Fatal("unrelated source revision invalidates port", consumer.ReferenceUpdates, err)
	}
	run(CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: id, Value: 30, Unit: "mm"})
	consumer, err = s.GetDocument(t.Context(), consumer.Document.ID, access.DefaultUserID)
	if err != nil || consumer.ReferenceUpdates[0].Status != "UPDATE_AVAILABLE" {
		t.Fatal("changed value is not pending", consumer.ReferenceUpdates, err)
	}
	view = consumer
	run(CommandRequest{Type: "UPDATE_REFERENCES"})
	if view.Product.Parameters[0].EvaluatedValue.SIValue != .03 {
		t.Fatal("Product external update", view.Product.Parameters)
	}
	run(CommandRequest{Type: "UNDO"})
	if view.Product.Parameters[0].EvaluatedValue.SIValue != .02 {
		t.Fatal("Product external update undo", view.Product.Parameters)
	}
}
