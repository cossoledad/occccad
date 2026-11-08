//go:build integration

package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func TestPartDependencyReadAuthorizationAndSnapshot(t *testing.T) {
	db := testsupport.OpenDevelopmentDatabase(t)
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, nil, artifact.NewService(db, store, staging))
	v, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PART", Name: "Part dependency read authorization"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = domain.ApplyCommand(t.Context(), v.Document.ID, workspace.CommandRequest{ActorID: access.DefaultUserID, RequestID: v.Document.ID + "-sketch", Type: "CREATE_SKETCH", Plane: "XY"})
	if err != nil {
		t.Fatal(err)
	}
	featureID := v.Part.Features[0].ID
	v, err = domain.ApplyCommand(t.Context(), v.Document.ID, workspace.CommandRequest{ActorID: access.DefaultUserID, RequestID: v.Document.ID + "-parameter", Type: "CREATE_PARAMETER", Name: "read_probe", Value: 1, Unit: "mm"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PRODUCT", Name: "Private dependency context"})
	if err != nil {
		t.Fatal(err)
	}
	viewer := uuid.NewString()
	if _, err = db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name,status,platform_role) VALUES($1,$2,'Dependency viewer','ACTIVE','MEMBER')`, viewer, viewer+"@example.test"); err != nil {
		t.Fatal(err)
	}
	acl := access.New(db)
	if _, err = acl.UpsertGrant(t.Context(), "DOCUMENT", v.Document.ID, access.DefaultUserID, access.ShareRequest{SubjectType: "USER", SubjectID: viewer, Role: access.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	s := &Server{database: db, workspace: domain, access: acl}
	read := func(actor string, input workspace.PartDependencyRequest) *httptest.ResponseRecorder {
		data, _ := json.Marshal(input)
		r := httptest.NewRequest("POST", "/part-dependencies", bytes.NewReader(data))
		r.SetPathValue("documentID", v.Document.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: actor}))
		w := httptest.NewRecorder()
		s.partDependencies(w, r)
		return w
	}
	request := workspace.PartDependencyRequest{VersionID: v.Document.VersionID}
	w := read(viewer, request)
	if w.Code != 200 {
		t.Fatal("Viewer read", w.Code, w.Body.String())
	}
	var graph workspace.PartDependencies
	if err = json.Unmarshal(w.Body.Bytes(), &graph); err != nil || graph.DocumentID != v.Document.ID || graph.VersionID != v.Document.VersionID || graph.Digest == "" || len(graph.Nodes) == 0 {
		t.Fatal("partial graph", err, graph)
	}
	readComparison := func(actor string, input workspace.FeatureRepairRequest) *httptest.ResponseRecorder {
		data, _ := json.Marshal(input)
		r := httptest.NewRequest("POST", "/feature-repair-context", bytes.NewReader(data))
		r.SetPathValue("documentID", v.Document.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: actor}))
		w := httptest.NewRecorder()
		s.featureRepairContext(w, r)
		return w
	}
	comparisonRequest := workspace.FeatureRepairRequest{PartDependencyRequest: request, FeatureID: featureID}
	comparisonResponse := readComparison(viewer, comparisonRequest)
	if comparisonResponse.Code != 200 {
		t.Fatal("Viewer comparison", comparisonResponse.Code, comparisonResponse.Body.String())
	}
	var comparison workspace.FeatureRepairContext
	if err = json.Unmarshal(comparisonResponse.Body.Bytes(), &comparison); err != nil || comparison.Snapshot == nil || comparison.Snapshot.SketchFrame == nil {
		t.Fatal("historical sketch read", err, comparison)
	}
	if unauthorized := readComparison(uuid.NewString(), comparisonRequest); unauthorized.Code != 403 {
		t.Fatal("unauthorized comparison", unauthorized.Code)
	}
	comparisonRequest.RootProductDocumentID = root.Document.ID
	comparisonRequest.RootProductRevisionID = root.Document.VersionID
	if private := readComparison(viewer, comparisonRequest); private.Code != 403 {
		t.Fatal("private comparison scope", private.Code)
	}
	if w = read(uuid.NewString(), request); w.Code != 403 {
		t.Fatal("unauthorized graph exposed", w.Code)
	}
	request.VersionID = "old"
	if w = read(viewer, request); w.Code == 200 {
		t.Fatal("stale graph exposed")
	}
	request.VersionID = v.Document.VersionID
	request.ContextVariantID = "orphan"
	if w = read(viewer, request); w.Code == 200 {
		t.Fatal("partial scope accepted")
	}
	request.ContextVariantID = ""
	request.RootProductDocumentID = root.Document.ID
	request.RootProductRevisionID = root.Document.VersionID
	if w = read(viewer, request); w.Code != 403 {
		t.Fatal("private root context bypassed", w.Code, w.Body.String())
	}
	final, err := domain.GetDocument(t.Context(), v.Document.ID, access.DefaultUserID)
	if err != nil || final.Document.VersionID != v.Document.VersionID {
		t.Fatal("read changed Head", err)
	}
}
