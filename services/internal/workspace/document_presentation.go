package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/database"
)

var ErrPresentationConflict = errors.New("PRESENTATION_CONFLICT")

// AnnotationPosition is a point in the owning document's model frame, in mm.
// A two-dimensional screen location is not a valid annotation position.
type AnnotationPosition [3]float64

func (p *AnnotationPosition) UnmarshalJSON(raw []byte) error {
	var coordinates []float64
	if err := json.Unmarshal(raw, &coordinates); err != nil {
		return err
	}
	if len(coordinates) != 3 {
		return fmt.Errorf("%w: annotation position requires three coordinates", ErrValidation)
	}
	copy(p[:], coordinates)
	return nil
}

type AnnotationLayout struct {
	Version  int64               `json:"version"`
	Position *AnnotationPosition `json:"position"`
}

func (s *Service) annotationMeasurementIDs(ctx context.Context, doc string) (map[string]bool, error) {
	var raw []byte
	if err := s.database.QueryRow(ctx, `SELECT v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1 AND d.deleted_at IS NULL`, doc).Scan(&raw); err != nil {
		return nil, err
	}
	values, err := measurementValues(raw)
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, m := range values {
		ids[m.ID] = true
	}
	return ids, nil
}

func (s *Service) AnnotationLayouts(ctx context.Context, doc string) (map[string]AnnotationLayout, error) {
	ids, err := s.annotationMeasurementIDs(ctx, doc)
	if err != nil {
		return nil, err
	}
	rows, err := s.database.Query(ctx, `SELECT object_id,version,payload FROM occccad.document_presentation WHERE document_id=$1 AND namespace='measurement.label.world'`, doc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]AnnotationLayout{}
	for rows.Next() {
		var id string
		var raw []byte
		var layout AnnotationLayout
		if err := rows.Scan(&id, &layout.Version, &raw); err != nil {
			return nil, err
		}
		if !ids[id] {
			continue
		}
		if err := json.Unmarshal(raw, &layout.Position); err != nil {
			return nil, err
		}
		out[id] = layout
	}
	return out, rows.Err()
}

// Presentation has an independent CAS and never writes a Workspace Revision,
// ChangeSet or history action. A reset is versioned too, to reject late drags.
func (s *Service) SaveAnnotationLayout(ctx context.Context, doc, id string, input AnnotationLayout) (AnnotationLayout, error) {
	if input.Version < 0 || input.Version == math.MaxInt64 {
		return AnnotationLayout{}, fmt.Errorf("%w: invalid presentation version", ErrValidation)
	}
	if input.Position != nil {
		for _, v := range input.Position {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
				return AnnotationLayout{}, fmt.Errorf("%w: invalid annotation position", ErrValidation)
			}
		}
	}
	ids, err := s.annotationMeasurementIDs(ctx, doc)
	if err != nil {
		return AnnotationLayout{}, err
	}
	if !ids[id] {
		return AnnotationLayout{}, ErrNotFound
	}
	raw, err := json.Marshal(input.Position)
	if err != nil {
		return AnnotationLayout{}, err
	}
	var version int64
	if input.Version == 0 {
		err = s.database.QueryRow(ctx, `INSERT INTO occccad.document_presentation(document_id,namespace,object_id,version,payload) VALUES($1,'measurement.label.world',$2,1,$3) ON CONFLICT(document_id,namespace,object_id) DO NOTHING RETURNING version`, doc, id, raw).Scan(&version)
	} else {
		err = s.database.QueryRow(ctx, `UPDATE occccad.document_presentation SET payload=$3,version=version+1 WHERE document_id=$1 AND namespace='measurement.label.world' AND object_id=$2 AND version=$4 RETURNING version`, doc, id, raw, input.Version).Scan(&version)
	}
	if errors.Is(err, database.ErrNoRows) {
		return AnnotationLayout{}, ErrPresentationConflict
	}
	if err != nil {
		return AnnotationLayout{}, err
	}
	return AnnotationLayout{Version: version, Position: input.Position}, nil
}
