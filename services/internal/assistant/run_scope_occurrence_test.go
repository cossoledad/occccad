package assistant

import (
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

type scopeOccurrenceFixture struct {
	s                          *Service
	domain                     *workspace.Service
	actor, part, body          string
	root, rootRevision         string
	acceptedRevision, partHead string
	path                       []string
}

func newScopeOccurrenceFixture(t *testing.T) scopeOccurrenceFixture {
	t.Helper()
	s, domain, part, actor := assistantFixture(t, "http://127.0.0.1:1")
	apply := func(document string, request workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
		if err != nil {
			t.Fatal(err)
		}
		request.ActorID, request.RequestID, request.ExpectedHead = actor, uuid.NewString(), &snapshot.CommandBaseline
		preview, err := domain.PreviewCommand(t.Context(), document, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
		request.PreviewID = preview.PreviewID
		view, err := domain.ApplyCommand(t.Context(), document, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
		return view
	}
	createProduct := func() workspace.DocumentView {
		t.Helper()
		v, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Type: "PRODUCT", Name: "Frozen occurrence scope " + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// Ordinary circular sketch/extrusion gives the accepted source a genuine
	// production Worker definition, not a synthetic revision/model payload.
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: part.Part.ActiveBodyID, Plane: "XY"})
	sketch := part.Part.Features[len(part.Part.Features)-1].ID
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 5}}}})
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: part.Part.ActiveBodyID, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 35})
	accepted := part.Document.VersionID
	child := createProduct()
	child = apply(child.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	leaf := child.Product.Instances[0].ID
	child = apply(child.Document.ID, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: leaf, ReferenceMode: "PINNED"})
	root := createProduct()
	root = apply(root.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: child.Document.ID})
	branch := root.Product.Instances[0].ID
	root = apply(root.Document.ID, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: branch, ReferenceMode: "PINNED"})
	// The caller still owns the source and can read its newer Head. ACL alone
	// therefore cannot detect a forged definition/revision for the occurrence.
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "RENAME_BODY", BodyID: part.Part.ActiveBodyID, Name: "New source Head after pinning"})
	if part.Document.VersionID == accepted {
		t.Fatal("source update did not create a distinct immutable revision")
	}
	f := scopeOccurrenceFixture{s: s, domain: domain, actor: actor, part: part.Document.ID, body: part.Part.ActiveBodyID, root: root.Document.ID, rootRevision: root.Document.VersionID, acceptedRevision: accepted, partHead: part.Document.VersionID, path: []string{branch, leaf}}
	observed, err := s.automation.QueryOccurrence(t.Context(), actor, f.root, automation.OccurrenceQuery{RevisionID: f.rootRevision, InstanceIDs: f.path, Kind: "BODY", Limit: 1})
	if err != nil || observed.SourceDocumentID != f.part || observed.SourceRevisionID != accepted || observed.ReferenceMode != "PINNED" {
		t.Fatal("root does not preserve the intended historical/PINNED definition", observed, err)
	}
	return f
}

func (f scopeOccurrenceFixture) leafScope() FrozenScope {
	return FrozenScope{RootDocumentID: f.root, RootRevisionID: f.rootRevision, DocumentID: f.part, RevisionID: f.acceptedRevision, WorkingBodyID: f.body, InstancePath: append([]string(nil), f.path...), Selections: []SelectedReference{}, Complete: true}
}

func TestAssistantFrozenOccurrenceScopeRequiresAcceptedSourceRevision(t *testing.T) {
	f := newScopeOccurrenceFixture(t)
	valid := f.leafScope()
	frozen, writable, err := f.s.freezeScope(t.Context(), f.actor, f.part, &valid, "")
	if err != nil || writable || frozen.RevisionID != f.acceptedRevision || frozen.RootRevisionID != f.rootRevision {
		t.Fatal("accepted historical definition must remain readable and read-only", frozen, writable, err)
	}
	forged := f.leafScope()
	forged.RevisionID = f.partHead
	_, writable, err = f.s.freezeScope(t.Context(), f.actor, f.part, &forged, "")
	if err == nil || writable || asFault(err).Code != "AI_SCOPE_INVALID" {
		t.Fatal("client replaced PINNED source revision with writable source Head", writable, err)
	}
}

func TestAssistantFrozenSelectionOccurrenceRequiresRootSourceIdentity(t *testing.T) {
	f := newScopeOccurrenceFixture(t)
	unrelated, err := f.domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: f.actor, Type: "PART", Name: "Readable unrelated selection source " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	validRef := SelectedReference{Kind: "BODY", ID: f.body, BodyID: f.body, SourceDocumentID: f.part, SourceVersionID: f.acceptedRevision, InstancePath: append([]string(nil), f.path...)}
	for _, definition := range []string{"root-product", "historical-leaf-part"} {
		t.Run(definition, func(t *testing.T) {
			scope, document := f.leafScope(), f.part
			if definition == "root-product" {
				scope = FrozenScope{RootDocumentID: f.root, RootRevisionID: f.rootRevision, DocumentID: f.root, RevisionID: f.rootRevision, Complete: true}
				document = f.root
			}
			scope.Selections, scope.SelectionCount = []SelectedReference{validRef}, 1
			if _, _, e := f.s.freezeScope(t.Context(), f.actor, document, &scope, ""); e != nil {
				t.Fatal("root-bound accepted occurrence selection rejected", e)
			}
			cases := []struct {
				name string
				ref  SelectedReference
			}{
				{"same-source-new-head", validRef},
				{"readable-unrelated-source", SelectedReference{Kind: "BODY", ID: unrelated.Part.ActiveBodyID, BodyID: unrelated.Part.ActiveBodyID, SourceDocumentID: unrelated.Document.ID, SourceVersionID: unrelated.Document.VersionID, InstancePath: append([]string(nil), f.path...)}},
			}
			cases[0].ref.SourceVersionID = f.partHead
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					forged := scope
					forged.Selections = []SelectedReference{tc.ref}
					if _, _, e := f.s.freezeScope(t.Context(), f.actor, document, &forged, ""); e == nil || asFault(e).Code != "AI_SCOPE_INVALID" {
						t.Fatal("selection path accepted an independent source/revision based only on its ACL", e)
					}
				})
			}
		})
	}
}
