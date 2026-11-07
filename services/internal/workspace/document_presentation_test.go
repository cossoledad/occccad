package workspace

import (
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
)

func TestAnnotationPresentationCASReopenAndHistoryIndependence(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "presentation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	s := NewWithArtifacts(db, nil, nil)
	view, err := s.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PRODUCT", Name: "presentation", ActorID: access.DefaultUserID})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	var originalActions int
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM occccad.domain_transactions WHERE workspace_id IN (SELECT id FROM occccad.workspaces WHERE document_id=$1)`, view.Document.ID).Scan(&originalActions); err != nil {
		t.Fatal(err)
	}
	// The test owns this synthetic result; geometry evaluation is covered by
	// Router integration. Presentation only validates the retained object ID.
	model := *view.Product
	model.Measurements = []Measurement{{ID: id}}
	raw, _ := json.Marshal(model)
	if _, err := db.Exec(t.Context(), `UPDATE occccad.document_versions SET model_json=$2 WHERE id=$1`, view.Document.VersionID, raw); err != nil {
		t.Fatal(err)
	}
	input := AnnotationLayout{Position: &AnnotationPosition{-30, 20, -10}}
	for _, raw := range []string{`{"position":[30,20]}`, `{"position":[1,2,3,4]}`} {
		var invalid AnnotationLayout
		if err := json.Unmarshal([]byte(raw), &invalid); err == nil {
			t.Fatal("a screen position was accepted as a world point", raw)
		}
	}
	for _, coordinate := range []float64{math.NaN(), math.Inf(1), 1e7} {
		_, err := s.SaveAnnotationLayout(t.Context(), view.Document.ID, id, AnnotationLayout{Position: &AnnotationPosition{0, coordinate, 0}})
		if !errors.Is(err, ErrValidation) {
			t.Fatal("invalid world coordinate accepted", coordinate, err)
		}
	}
	saved, err := s.SaveAnnotationLayout(t.Context(), view.Document.ID, id, input)
	if err != nil || saved.Version != 1 {
		t.Fatal(saved, err)
	}
	if _, err := s.SaveAnnotationLayout(t.Context(), view.Document.ID, id, input); !errors.Is(err, ErrPresentationConflict) {
		t.Fatal("late drag overwrote layout", err)
	}
	reopened := NewWithArtifacts(db, nil, nil)
	layouts, err := reopened.AnnotationLayouts(t.Context(), view.Document.ID)
	if err != nil || *layouts[id].Position != *input.Position {
		t.Fatal(layouts, err)
	}
	reset, err := s.SaveAnnotationLayout(t.Context(), view.Document.ID, id, AnnotationLayout{Version: 1})
	if err != nil || reset.Version != 2 || reset.Position != nil {
		t.Fatal(reset, err)
	}
	var head string
	var actions int
	if err := db.QueryRow(t.Context(), `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, view.Document.ID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM occccad.domain_transactions WHERE workspace_id IN (SELECT id FROM occccad.workspaces WHERE document_id=$1)`, view.Document.ID).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if head != view.Document.VersionID || actions != originalActions {
		t.Fatal("label position wrote modeling history", head, actions)
	}
	if _, err := s.SaveAnnotationLayout(t.Context(), view.Document.ID, uuid.NewString(), input); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown measurement accepted", err)
	}
}
