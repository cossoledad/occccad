package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

const typeUpdateDerived = "occccad://document/derived/update"

type derivedUpdatePayload struct {
	Measurements    *[]Measurement     `json:"measurements,omitempty"`
	SpatialAnalyses *[]SpatialAnalysis `json:"spatialAnalyses,omitempty"`
}

// Adapters prepare exact evidence outside the transaction. The shared command
// applies all accepted evidence atomically and uses the normal CAS/history path.
func applyDerivedUpdate(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var p derivedUpdatePayload
	var changes modelcore.ChangeSet
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, changes, err
	}
	apply := func(value any, handler func(json.RawMessage, json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error)) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		next, set, err := handler(raw, encoded)
		if err != nil {
			return err
		}
		raw = next
		changes.Changes = append(changes.Changes, set.Changes...)
		changes.ImpactSeeds = append(changes.ImpactSeeds, set.ImpactSeeds...)
		return nil
	}
	if p.Measurements != nil {
		if err := apply(*p.Measurements, applyMeasurements); err != nil {
			return nil, changes, err
		}
	}
	if p.SpatialAnalyses != nil {
		if err := apply(*p.SpatialAnalyses, applySpatialAnalyses); err != nil {
			return nil, changes, err
		}
	}
	return raw, changes, nil
}

func (s *Service) prepareDerivedUpdate(ctx context.Context, doc, kind string, raw json.RawMessage, r CommandRequest) (derivedUpdatePayload, error) {
	var p derivedUpdatePayload
	if r.TargetKind != "" && r.TargetKind != "ALL" && r.TargetKind != "MEASUREMENT" && r.TargetKind != "SPATIAL_ANALYSIS" {
		return p, fmt.Errorf("%w: DERIVED_TARGET_INVALID", ErrValidation)
	}
	if r.TargetKind == "" || r.TargetKind == "ALL" || r.TargetKind == "MEASUREMENT" {
		values, err := measurementValues(raw)
		if err != nil {
			return p, err
		}
		s.projectMeasurements(ctx, doc, r.VersionID, values)
		found := r.TargetID == ""
		candidate := raw
		for _, m := range values {
			if r.TargetID != "" && m.ID != r.TargetID {
				continue
			}
			found = true
			current := m.Status == "CURRENT"
			for _, status := range m.OutputCurrency {
				current = current && status == "CURRENT"
			}
			if current {
				continue
			}
			next, err := s.prepareMeasurements(ctx, doc, candidate, CommandRequest{Type: "UPDATE_MEASUREMENT", VersionID: r.VersionID, Measurement: &m})
			if err != nil {
				return p, err
			}
			encoded, _ := json.Marshal(next)
			candidate, _, err = applyMeasurements(candidate, encoded)
			if err != nil {
				return p, err
			}
		}
		if !found {
			return p, ErrNotFound
		}
		next, err := measurementValues(candidate)
		if err != nil {
			return p, err
		}
		clearMeasurementProjection(next)
		p.Measurements = &next
	}
	if kind == "PRODUCT" && (r.TargetKind == "" || r.TargetKind == "ALL" || r.TargetKind == "SPATIAL_ANALYSIS") {
		request := CommandRequest{Type: "UPDATE_SPATIAL_ANALYSES", VersionID: r.VersionID}
		if r.TargetID != "" {
			request.SpatialAnalysisIDs = []string{r.TargetID}
		}
		values, err := s.prepareSpatialAnalyses(ctx, doc, raw, request)
		if err != nil {
			return p, err
		}
		p.SpatialAnalyses = &values
	}
	return p, nil
}

func derivedOnly(changes modelcore.ChangeSet) bool {
	for _, c := range changes.Changes {
		if c.Target.SlotID != measurementSlot && c.Target.SlotID != "product.spatial-analyses" && c.Target.SlotID != "parameter.entity" {
			return false
		}
	}
	return len(changes.Changes) > 0
}
