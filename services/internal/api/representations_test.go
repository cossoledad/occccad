//go:build integration

package api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/visualdelta"
	"github.com/occccad/occccad/internal/workspace"
)

func TestVisualDeltaPreservesBytesAuthorizationAndVariantETag(t *testing.T) {
	db := testsupport.OpenDevelopmentDatabase(t)
	local, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	objects := artifact.NewService(db, local, staging)
	domain := workspace.NewWithArtifacts(db, nil, objects)
	actor := "00000000-0000-7000-8000-000000000001"
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PART", Name: fmt.Sprintf("visual delta %d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	baseBytes := make([]byte, 64<<10)
	for i := range baseBytes {
		baseBytes[i] = byte(i*17 + i/97)
	}
	targetBytes := append([]byte("changed GLB metadata and alignment"), baseBytes...)
	base, err := objects.Put(t.Context(), artifact.KindGLB, "model/gltf-binary", bytes.NewReader(baseBytes))
	if err != nil {
		t.Fatal(err)
	}
	target, err := objects.Put(t.Context(), artifact.KindGLB, "model/gltf-binary", bytes.NewReader(targetBytes))
	if err != nil {
		t.Fatal(err)
	}
	unreferenced, err := objects.Put(t.Context(), artifact.KindGLB, "model/gltf-binary", bytes.NewReader(bytes.Repeat([]byte("private"), 4096)))
	if err != nil {
		t.Fatal(err)
	}
	baseKey, targetKey := view.Document.ID+":delta-base", view.Document.ID+":delta-target"
	for key, object := range map[string]string{baseKey: base.ID, targetKey: target.ID} {
		_, err = db.Exec(t.Context(), `INSERT INTO occccad.geometry_artifacts(geometry_key,geometry_id,evaluator_version,occt_version,units,bbox_json,topology_json,volume) VALUES($1,$1,'fixture','fixture','mm','{}','{}',0)`, key)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(t.Context(), `INSERT INTO occccad.geometry_representations(geometry_key,role,schema_version,object_id) VALUES($1,'VISUAL',2,$2)`, key, object)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Only this test's own snapshots are authored as authorization fixtures.
	model := *view.Part
	model.Bodies = append([]workspace.PartBody(nil), model.Bodies...)
	model.Bodies[0].GeometryKey = baseKey
	raw, _ := json.Marshal(model)
	if _, err = db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$2 WHERE id=$1`, view.Document.VersionID, raw); err != nil {
		t.Fatal(err)
	}
	baseVersion := view.Document.VersionID
	view, err = domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: view.Document.ID + "-body", Type: "CREATE_BODY"})
	if err != nil {
		t.Fatal(err)
	}
	model = *view.Part
	model.Bodies = append([]workspace.PartBody(nil), model.Bodies...)
	model.Bodies[1].GeometryKey = targetKey
	raw, _ = json.Marshal(model)
	if _, err = db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$2 WHERE id=$1`, view.Document.VersionID, raw); err != nil {
		t.Fatal(err)
	}
	server := &Server{database: db, workspace: domain, access: access.New(db), artifacts: objects}
	basePath := fmt.Sprintf("/api/documents/%s/representations/%s?versionId=%s&bodyId=%s", view.Document.ID, base.ID, baseVersion, model.Bodies[0].ID)
	get := func(user, baseURL, accept, etag string, preview bool) *httptest.ResponseRecorder {
		query := url.Values{"versionId": {view.Document.VersionID}, "bodyId": {model.Bodies[1].ID}, "base": {baseURL}}
		if preview {
			query.Set("previewId", "expired")
		}
		r := httptest.NewRequest("GET", "/representation?"+query.Encode(), nil)
		r.SetPathValue("documentID", view.Document.ID)
		r.SetPathValue("objectID", target.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: user}))
		r.Header.Set("Accept", accept)
		r.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		server.downloadRepresentation(w, r)
		return w
	}
	response := get(actor, basePath, visualdelta.ContentType, "", false)
	if response.Code != 200 || response.Header().Get("Content-Type") != visualdelta.ContentType {
		t.Fatalf("delta %d %s", response.Code, response.Body.String())
	}
	patch := response.Body.Bytes()
	if len(patch) >= len(targetBytes)/4 {
		t.Fatalf("no useful delta: %d", len(patch))
	}
	if string(patch[:8]) != "OCCDLT01" || int(binary.LittleEndian.Uint32(patch[72:])) != len(targetBytes) {
		t.Fatal("invalid header")
	}
	var reconstructed []byte
	cursor := 80
	for range binary.LittleEndian.Uint32(patch[76:80]) {
		op := patch[cursor]
		length := int(binary.LittleEndian.Uint32(patch[cursor+1:]))
		cursor += 5
		if op == 1 {
			reconstructed = append(reconstructed, patch[cursor:cursor+length]...)
			cursor += length
			continue
		}
		offset := int(binary.LittleEndian.Uint32(patch[cursor:]))
		cursor += 4
		start := len(reconstructed)
		reconstructed = append(reconstructed, baseBytes[offset:offset+length]...)
		if op == 2 {
			delta := binary.LittleEndian.Uint32(patch[cursor:])
			cursor += 4
			for i := start; i < len(reconstructed); i += 4 {
				binary.LittleEndian.PutUint32(reconstructed[i:], binary.LittleEndian.Uint32(reconstructed[i:])+delta)
			}
		}
	}
	if !bytes.Equal(reconstructed, targetBytes) || cursor != len(patch) {
		t.Fatal("delta changed display bytes")
	}
	etag := response.Header().Get("ETag")
	if etag == `"`+target.SHA256+`"` {
		t.Fatal("different encodings share a strong ETag")
	}
	if got := get(actor, basePath, visualdelta.ContentType, etag, false); got.Code != 304 || got.Body.Len() != 0 {
		t.Fatal("delta ETag ignored")
	}
	if got := get(actor, basePath, visualdelta.ContentType, `"`+target.SHA256+`"`, false); got.Code != 200 {
		t.Fatal("full ETag reused for delta")
	}
	if got := get(actor, basePath, visualdelta.ContentType+";q=0, model/gltf-binary", "", false); got.Code != 200 || !bytes.Equal(got.Body.Bytes(), targetBytes) {
		t.Fatal("full encoding fallback failed")
	}
	visualDeltaSlot <- struct{}{}
	busy := get(actor, basePath, visualdelta.ContentType, "", false)
	<-visualDeltaSlot
	if busy.Code != 200 || !bytes.Equal(busy.Body.Bytes(), targetBytes) {
		t.Fatal("busy encoder must use full immutable bytes")
	}
	for _, tc := range []struct {
		user, base string
		preview    bool
		status     int
	}{
		{actor, "https://example.com/private", false, 400},
		{actor, fmt.Sprintf("/api/documents/%s/representations/%s?versionId=%s", view.Document.ID, unreferenced.ID, view.Document.VersionID), false, 404},
		{actor, fmt.Sprintf("/api/documents/foreign/representations/%s", base.ID), false, 400},
		{actor, basePath + "&bodyId=wrong", false, 200}, // duplicate query uses first authorized Body
		{actor, fmt.Sprintf("/api/documents/%s/representations/%s?versionId=%s&bodyId=wrong", view.Document.ID, base.ID, baseVersion), false, 404},
		{"00000000-0000-7000-8000-000000000099", basePath, false, 403},
		{actor, basePath, true, 404},
	} {
		if got := get(tc.user, tc.base, visualdelta.ContentType, "", tc.preview); got.Code != tc.status {
			t.Fatalf("scope %q %d != %d", tc.base, got.Code, tc.status)
		}
	}
}

func TestRepresentationDownloadAuthorizationAndSnapshot(t *testing.T) {
	url := os.Getenv("OCCCCAD_DATABASE_URL")
	if url == "" {
		t.Skip("requires configured development database")
	}
	db, err := database.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	local, developmentStaging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	objects := artifact.NewService(db, local, developmentStaging)
	domain := workspace.NewWithArtifacts(db, nil, objects)
	actor := "00000000-0000-7000-8000-000000000001"
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PART", Name: fmt.Sprintf("artifact HTTP %d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := objects.Put(t.Context(), artifact.KindGLB, "model/gltf-binary", bytes.NewReader([]byte("unreferenced")))
	if err != nil {
		t.Fatal(err)
	}
	ref := view.Artifacts[view.Part.Bodies[0].GeometryKey].Representations["VISUAL"]
	server := &Server{database: db, workspace: domain, access: access.New(db), artifacts: objects}
	bodyScope := ""
	get := func(user, object, version, etag string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/representation?versionId="+version+"&bodyId="+bodyScope, nil)
		request.SetPathValue("documentID", view.Document.ID)
		request.SetPathValue("objectID", object)
		request = request.WithContext(access.WithPrincipal(request.Context(), access.User{ID: user}))
		request.Header.Set("If-None-Match", etag)
		response := httptest.NewRecorder()
		server.downloadRepresentation(response, request)
		return response
	}
	result := get(actor, ref.ObjectID, view.Document.VersionID, "")
	if result.Code != 200 || result.Body.Len() != int(ref.Size) || result.Header().Get("ETag") != `"`+ref.Digest+`"` {
		t.Fatalf("download status=%d body=%s", result.Code, result.Body.String())
	}
	if cached := get(actor, ref.ObjectID, view.Document.VersionID, result.Header().Get("ETag")); cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatal("ETag not honored")
	}
	if result := get(actor, unrelated.ID, view.Document.VersionID, ""); result.Code != 404 {
		t.Fatalf("unrelated object exposed: %d", result.Code)
	}
	if result := get("00000000-0000-7000-8000-000000000099", ref.ObjectID, view.Document.VersionID, ""); result.Code != 403 {
		t.Fatalf("unauthorized object exposed: %d", result.Code)
	}
	if result := get(actor, ref.ObjectID, "00000000-0000-7000-8000-000000000099", ""); result.Code != 404 {
		t.Fatalf("foreign revision accepted: %d", result.Code)
	}

	view, err = domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: view.Document.ID + "-body", Type: "CREATE_BODY"})
	if err != nil {
		t.Fatal(err)
	}
	authorityIDs := make(map[string]string)
	for _, role := range []string{"BREP", "NAMING"} {
		kind := artifact.KindBREP
		contentType := "application/vnd.opencascade.brep"
		if role == "NAMING" {
			kind = artifact.KindTopologyManifest
			contentType = "application/x-protobuf"
		}
		data := []byte("authorization fixture " + role)
		object, e := objects.Put(t.Context(), kind, contentType, bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		authorityIDs[role] = object.ID
		_, e = db.Exec(t.Context(), `INSERT INTO occccad.geometry_representations(geometry_key,role,schema_version,object_id) VALUES($1,$2,2,$3)`, view.Part.Bodies[1].GeometryKey, role, object.ID)
		if e != nil {
			t.Fatal(e)
		}
		bodyScope = view.Part.Bodies[1].ID
		if response := get(actor, object.ID, view.Document.VersionID, ""); response.Code != 200 || !bytes.Equal(response.Body.Bytes(), data) {
			t.Fatalf("%s download: %d %s", role, response.Code, response.Body.String())
		}
		bodyScope = view.Part.Bodies[0].ID
		if response := get(actor, object.ID, view.Document.VersionID, ""); response.Code != 404 {
			t.Fatalf("cross Body %s download accepted: %d", role, response.Code)
		}
	}
	bodyScope = ""
	partSnapshot := view
	// Follow nested frozen snapshots, including after the root Head changes.
	child := view
	for depth := 0; depth < 2; depth++ {
		product, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PRODUCT", Name: fmt.Sprintf("artifact parent %d", depth)})
		if err != nil {
			t.Fatal(err)
		}
		view, err = domain.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: "insert-" + product.Document.ID, Type: "INSERT_INSTANCE", ReferencedDocumentID: child.Document.ID})
		if err != nil {
			t.Fatal(err)
		}
		if response := get(actor, ref.ObjectID, view.Document.VersionID, ""); response.Code != 200 {
			t.Fatalf("nested depth %d download: %d %s", depth, response.Code, response.Body.String())
		}
		child = view
	}
	historical := view.Document.VersionID
	if _, err := domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: actor, RequestID: "undo-" + view.Document.ID, Type: "UNDO"}); err != nil {
		t.Fatal(err)
	}
	if response := get(actor, ref.ObjectID, historical, ""); response.Code != 200 {
		t.Fatalf("historical download: %d", response.Code)
	}
	if response := get(actor, ref.ObjectID, "", ""); response.Code != 404 {
		t.Fatalf("removed occurrence still exposes artifact: %d", response.Code)
	}
	// Seed the derived failure display in this test's own Part snapshot. Its
	// successful source representations remain stored, but only Visual is readable.
	view = partSnapshot
	body := view.Part.Bodies[1]
	// Mutate this test's own snapshot with the shared JSON model, so the
	// authorization fixture runs on both supported database dialects.
	model := *view.Part
	model.Bodies = append([]workspace.PartBody(nil), model.Bodies...)
	model.Bodies[1].DisplayFallback = &workspace.BodyDisplayFallback{GeometryKey: body.GeometryKey, SourceVersionID: view.Document.VersionID}
	model.Bodies[1].GeometryKey = ""
	raw, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$2 WHERE id=$1`, view.Document.VersionID, raw); err != nil {
		t.Fatal(err)
	}
	bodyScope = body.ID
	visual := view.Artifacts[body.GeometryKey].Representations["VISUAL"]
	if response := get(actor, visual.ObjectID, view.Document.VersionID, ""); response.Code != 200 {
		t.Fatalf("failure display unavailable: %d %s", response.Code, response.Body.String())
	}
	for role, objectID := range authorityIDs {
		if response := get(actor, objectID, view.Document.VersionID, ""); response.Code != 404 {
			t.Fatalf("fallback exposed %s: %d", role, response.Code)
		}
	}
}
