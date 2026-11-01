package workspace

import (
	"encoding/json"

	"github.com/occccad/occccad/internal/modelcore"
)

func measurementGraph(measurements []Measurement, parameters []modelcore.ParameterDefinition, source func(MeasurementSupport) []modelcore.DependencyKey) ([]modelcore.DependencyNode, []modelcore.DependencyEdge) {
	nodes := []modelcore.DependencyNode{}
	edges := []modelcore.DependencyEdge{}
	for _, m := range measurements {
		key := modelcore.DependencyKey("measurement:" + m.ID)
		raw, _ := json.Marshal(struct {
			Kind     string
			Supports []MeasurementSupport
			Evidence *MeasurementEvidence
		}{m.Kind, m.Supports, m.Result})
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 2, Type: "MEASUREMENT", CanonicalInput: raw, Derived: true})
		for _, support := range m.Supports {
			for _, dependency := range source(support) {
				edges = append(edges, modelcore.DependencyEdge{Source: dependency, Target: key, Kind: modelcore.ReadGeometry})
			}
		}
	}
	for _, p := range parameters {
		if producer := p.Source.Produced; producer != nil && producer.Kind == "MEASUREMENT" {
			edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("measurement:" + producer.EntityID), Target: modelcore.DependencyKey("parameter:" + p.ParameterID), Kind: modelcore.ReadValue})
		}
	}
	return nodes, edges
}

func partMeasurementGraph(model PartModel) ([]modelcore.DependencyNode, []modelcore.DependencyEdge) {
	return measurementGraph(model.Measurements, model.Parameters, func(support MeasurementSupport) []modelcore.DependencyKey {
		bodyID := support.BodyID
		if support.Selection != nil {
			bodyID = support.Selection.SourceBodyID
		}
		key := modelcore.DependencyKey("body:" + bodyID)
		for _, f := range model.Features {
			if f.BodyID == bodyID {
				key = modelcore.DependencyKey("feature:" + f.ID)
			}
		}
		return []modelcore.DependencyKey{key}
	})
}

func productMeasurementGraph(model ProductModel) ([]modelcore.DependencyNode, []modelcore.DependencyEdge) {
	return measurementGraph(model.Measurements, model.Parameters, func(support MeasurementSupport) []modelcore.DependencyKey {
		if support.InstancePath == nil || len(support.InstancePath.Segments) == 0 {
			return nil
		}
		keys := []modelcore.DependencyKey{modelcore.DependencyKey("instance:" + support.InstancePath.Segments[0].InstanceID)}
		for _, b := range model.ContextBindings {
			if b.OwningInstancePath.Canonical == support.InstancePath.Canonical {
				keys = append(keys, modelcore.DependencyKey("context-binding:"+b.ID))
			}
		}
		return keys
	})
}
