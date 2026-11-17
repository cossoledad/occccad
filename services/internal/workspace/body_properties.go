package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/geometry"
)

// Derived properties are read-only projections of a qualified frozen BREP.
// They never enter Artifact/Revision persistence or normal Feature evaluation.
func (s *Service) bodyProperties(ctx context.Context, key string) (TopologyElementProperties, error) {
	a, err := s.loadArtifact(ctx, key)
	if err != nil {
		return TopologyElementProperties{}, err
	}
	if a.GeometryDomain != "SOLID_SET" || a.ValidationPolicy != geometryValidationPolicy {
		return TopologyElementProperties{}, fmt.Errorf("%w: solid Body properties required", ErrValidation)
	}
	ref, err := s.brepArtifactReference(ctx, key)
	if err != nil {
		return TopologyElementProperties{}, err
	}
	data, err := preparedResult(ctx, &s.prepared, "@body-properties", struct {
		GeometryID, Policy, SourceOCCT string
		BREP                           geometry.ArtifactReference
	}{a.GeometryID, measurementEvaluatorVersion, a.OCCTVersion, ref}, func() ([]byte, error) {
		input := geometry.AnalysisGeometry{ID: "properties", GeometryID: a.GeometryID, BRep: ref, Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}
		measured, e := s.worker.MeasureGeometry(ctx, newID("body-properties"), "ITEM", []geometry.AnalysisGeometry{input}, [][]geometry.MeasurementElement{{{InputID: "properties", Kind: "BODY", Mode: "GEOMETRY"}}})
		if e != nil {
			return nil, e
		}
		if !measured.Complete {
			return nil, fmt.Errorf("%w: %s", ErrValidation, measured.Diagnostic)
		}
		return json.Marshal(measured)
	})
	if err != nil {
		return TopologyElementProperties{}, err
	}
	var measured geometry.MeasurementResult
	if err = json.Unmarshal(data, &measured); err != nil {
		return TopologyElementProperties{}, err
	}
	properties := map[string]any{"lengthUnit": "mm"}
	for field, name := range map[string]string{"volumeMm3": "volume", "areaMm2": "area"} {
		if value, ok := measured.Values[field]; ok {
			properties[name] = value
		}
	}
	if value, ok := measured.Anchors["centroid"]; ok {
		properties["centroid"] = value
	}
	return TopologyElementProperties{GeometryKey: key, GeometryID: a.GeometryID, Kind: "BODY", GeometryType: "SOLID_SET", BBox: a.BBox, Properties: properties, WorkerID: a.WorkerID, OCCTVersion: a.OCCTVersion}, nil
}
