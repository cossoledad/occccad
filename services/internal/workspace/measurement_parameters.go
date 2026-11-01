package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// Evaluate freshness against the candidate's final poses, before publishing any
// parameter value. The snapshot is scoped to this call and cannot observe Head.
func (s *Service) refreshProductMeasurementParameters(ctx context.Context, documentID, sourceRevisionID, revisionID string, model *ProductModel) error {
	raw, err := json.Marshal(model)
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, measurementSnapshotKey{}, measurementSnapshot{documentID, sourceRevisionID, "PRODUCT", raw})
	s.projectMeasurements(ctx, documentID, sourceRevisionID, model.Measurements)
	return resolveProductParameters(model, revisionID)
}

var ErrDerivedInputUpdateRequired = fmt.Errorf("%w: DERIVED_INPUT_UPDATE_REQUIRED", ErrValidation)

func clearMeasurementProjection(values []Measurement) {
	for i := range values {
		values[i].Status = ""
		values[i].OutputCurrency = nil
		values[i].DisplayAnchors = nil
	}
}

func blockedParameterOwners(model PartModel) map[string]bool {
	out := map[string]bool{}
	for _, p := range model.Parameters {
		if p.OwnerFeatureID != "" && p.Source.Produced == nil && p.Source.Expression != nil && p.CurrencyStatus == "BLOCKED" {
			out[p.OwnerFeatureID] = true
		}
	}
	return out
}

func derivedBlockedFeatures(model PartModel, graph *modelcore.DependencyGraph) map[string]bool {
	seeds := []modelcore.DependencyKey{}
	for _, p := range model.Parameters {
		if (p.Source.Produced != nil || p.Source.Expression != nil) && p.CurrencyStatus != "CURRENT" && p.CurrencyStatus != "" {
			seeds = append(seeds, modelcore.DependencyKey("parameter:"+p.ParameterID))
		}
	}
	out := map[string]bool{}
	for _, key := range graph.DirtyClosure(seeds) {
		if strings.HasPrefix(string(key), "feature:") {
			out[strings.TrimPrefix(string(key), "feature:")] = true
		}
	}
	return out
}

func measurementBodyOrder(model PartModel, graph *modelcore.DependencyGraph) []int {
	ranks := map[string]int{}
	for rank, key := range graph.TopologicalOrder() {
		ranks[string(key)] = rank
	}
	order := make([]int, len(model.Bodies))
	for i := range order {
		order[i] = i
	}
	last := func(id string) int {
		value := ranks["body:"+id]
		for _, f := range model.Features {
			if f.BodyID == id && ranks["feature:"+f.ID] > value {
				value = ranks["feature:"+f.ID]
			}
		}
		return value
	}
	sort.SliceStable(order, func(i, j int) bool { return last(model.Bodies[order[i]].ID) < last(model.Bodies[order[j]].ID) })
	return order
}

func measurementParameterID(id, slot string) string { return "parameter:" + id + ":output:" + slot }

// A Part's owner frame is fixed. Unchanged intrinsic inputs also preserve its
// coordinate outputs. This pure gate runs between Body stages, before any
// downstream geometry is allowed to consume accepted measurement values.
func refreshPartMeasurementCurrency(model *PartModel) {
	keys := map[string]string{}
	for _, body := range model.Bodies {
		if !body.Consumed {
			keys[body.ID] = body.GeometryKey
		}
	}
	for i := range model.Measurements {
		m := &model.Measurements[i]
		m.Status = "UNKNOWN"
		m.OutputCurrency = map[string]string{}
		if m.Result == nil {
			m.Status = "MISSING"
			continue
		}
		if m.Result.EvaluatorVersion != measurementEvaluatorVersion || m.Result.Policy != measurementPolicy {
			continue
		}
		found, current := false, true
		for _, read := range m.Result.Reads {
			if read.Facet == modelcore.IntrinsicGeometry {
				found = true
				key, exists := keys[read.EntityID]
				if !exists || key == "" || key != read.Digest {
					current = false
				}
			}
		}
		if found {
			m.Status = "STALE"
			if current {
				m.Status = "CURRENT"
			}
		}
		for key := range m.Result.Values {
			m.OutputCurrency[key] = m.Status
		}
	}
}

func partConsumesMeasurementOutputs(model PartModel) bool {
	derived := map[modelcore.DependencyKey]bool{}
	for _, p := range model.Parameters {
		if p.Source.Produced != nil {
			derived[modelcore.DependencyKey("parameter:"+p.ParameterID)] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, p := range model.Parameters {
			key := modelcore.DependencyKey("parameter:" + p.ParameterID)
			if derived[key] || p.Source.Expression == nil {
				continue
			}
			for _, read := range p.Source.Expression.Reads {
				if derived[read] {
					derived[key] = true
					changed = true
					break
				}
			}
		}
	}
	for _, p := range model.Parameters {
		if p.OwnerFeatureID != "" && p.Source.Produced == nil && derived[modelcore.DependencyKey("parameter:"+p.ParameterID)] {
			return true
		}
	}
	return false
}

// Output definitions refer to the producer. Values are rebuildable evidence,
// never editable literal sources. Keep vanished slots unavailable for consumers.
func syncMeasurementParameters(parameters *[]modelcore.ParameterDefinition, measurements []Measurement) error {
	owners := map[string]bool{}
	for _, m := range measurements {
		owners[m.ID] = true
	}
	filtered := (*parameters)[:0]
	for _, p := range *parameters {
		if source := p.Source.Produced; source != nil && source.Kind == "MEASUREMENT" && !owners[source.EntityID] {
			continue
		}
		filtered = append(filtered, p)
	}
	*parameters = filtered
	for _, m := range measurements {
		if m.Result == nil {
			continue
		}
		keys := []string{}
		for key := range m.Result.Values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			id := measurementParameterID(m.ID, key)
			index := -1
			for i, p := range *parameters {
				if p.ParameterID == id {
					index = i
					break
				}
			}
			quantity, unit, err := measurementQuantity(key, m.Result.Values[key])
			if err != nil {
				return err
			}
			if index < 0 {
				*parameters = append(*parameters, modelcore.ParameterDefinition{ParameterID: id, OwnerFeatureID: m.ID, PropertySlot: key, Lifecycle: "ANALYSIS_OUTPUT", Key: nextParameterAlias(*parameters, key), Label: measurementOutputLabel(key), ValueType: modelcore.ValueQuantity, Dimension: quantity.Dimension, DisplayUnit: unit, Role: "OUTPUT", Source: modelcore.ValueSource{Produced: &modelcore.ParameterProducer{Kind: "MEASUREMENT", EntityID: m.ID, OutputSlot: key}}})
			}
		}
		for i := range *parameters {
			p := &(*parameters)[i]
			source := p.Source.Produced
			if source == nil || source.Kind != "MEASUREMENT" || source.EntityID != m.ID {
				continue
			}
			p.EvaluatedValue = nil
			p.CurrencyStatus = "MISSING"
			if value, ok := m.Result.Values[source.OutputSlot]; ok {
				quantity, _, err := measurementQuantity(source.OutputSlot, value)
				if err != nil {
					return err
				}
				p.EvaluatedValue = &quantity
				p.CurrencyStatus = m.Status
				if status := m.OutputCurrency[source.OutputSlot]; status != "" {
					p.CurrencyStatus = status
				}
				// A persisted snapshot contains accepted evidence. Live projections
				// and candidate validation recheck actual source fingerprints.
				if p.CurrencyStatus == "" {
					p.CurrencyStatus = "CURRENT"
				}
			}
		}
	}
	return nil
}

func rejectRemovedMeasurementConsumers(parameters []modelcore.ParameterDefinition, before, after []Measurement) error {
	remaining := map[string]bool{}
	for _, m := range after {
		remaining[m.ID] = true
	}
	removed := map[modelcore.DependencyKey]bool{}
	for _, p := range parameters {
		if source := p.Source.Produced; source != nil && source.Kind == "MEASUREMENT" && !remaining[source.EntityID] {
			removed[modelcore.DependencyKey("parameter:"+p.ParameterID)] = true
		}
	}
	for _, p := range parameters {
		if p.Source.Expression != nil {
			for _, read := range p.Source.Expression.Reads {
				if removed[read] {
					return fmt.Errorf("%w: MEASUREMENT_OUTPUT_IN_USE by %s", ErrValidation, p.ParameterID)
				}
			}
		}
	}
	return nil
}

func measurementOutputLabel(key string) string {
	labels := map[string]string{"distanceMm": "最小距离", "angleRad": "夹角", "lengthMm": "长度", "perimeterMm": "周长", "areaMm2": "面积", "volumeMm3": "体积", "radiusMm": "半径", "diameterMm": "直径", "majorRadiusMm": "长半径", "minorRadiusMm": "短半径", "arcAngleRad": "圆弧角度", "xMm": "X 坐标", "yMm": "Y 坐标", "zMm": "Z 坐标", "deltaXMm": "X 分量", "deltaYMm": "Y 分量", "deltaZMm": "Z 分量", "sizeXMm": "X 尺寸", "sizeYMm": "Y 尺寸", "sizeZMm": "Z 尺寸"}
	if label := labels[key]; label != "" {
		return label
	}
	return key
}
