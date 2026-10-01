package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const selectedPlaneNormalV1 = "SELECTED_PLANE_NORMAL_V1"

// No endpoint swaps or sign guesses here: the numeric adapter evaluates the
// selected-plane contract directly, including the Undefined alignment union.
func compileAssemblyOffset(c AssemblyConstraint, value *geometry.AssemblyConstraint, first, second geometry.AssemblyGeometry) error {
	if c.Kind != "DISTANCE" {
		return nil
	}
	if c.DistanceRelation == selectedPlaneNormalV1 && first.Kind != "PLANE" && second.Kind != "PLANE" {
		return fmt.Errorf("%w: signed offset requires an exact plane", ErrValidation)
	}
	if (c.DistanceRelation == "" || c.DistanceRelation == "UNSIGNED") && c.Value < 0 {
		return fmt.Errorf("%w: unsigned offset must be nonnegative", ErrValidation)
	}
	value.DistanceRelation = c.DistanceRelation
	if c.Mode == "MEASURED" && first.Kind == "PLANE" && second.Kind == "PLANE" {
		// Valid constant distance requires parallel planes of EITHER sense.
		// Keep the Driving direction intent in the definition, not in this
		// measurement-only residual. No endpoint/sign conversion is involved.
		value.DirectionRelation = "UNORIENTED"
	}
	return nil
}

// Constraint-owned Quantity parameters: references are checked stable IDs, not
// component names or evaluated floats. This is not a Configuration/Rule system.
func editOffsetParameter(model *ProductModel, index int, expression *string, key string, literalMM float64) error {
	c := &model.Constraints[index]
	if c.Kind != "DISTANCE" {
		if expression != nil || key != "" {
			return fmt.Errorf("%w: Offset parameters require DISTANCE", ErrValidation)
		}
		return nil
	}
	parameter := modelcore.ParameterDefinition{ParameterID: "offset:" + c.ID, Key: "Offset_" + parameterKeyFragment(c.ID),
		Label: "Offset", ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Role: "DRIVING", PropertySlot: "assembly-constraint.entity"}
	if c.OffsetParameter != nil {
		raw, _ := json.Marshal(c.OffsetParameter)
		if err := json.Unmarshal(raw, &parameter); err != nil {
			return err
		}
	}
	if key != "" {
		parameter.Key = key
	}
	if !validParameterKey(parameter.Key) {
		return fmt.Errorf("%w: offset key must be an ASCII identifier", ErrValidation)
	}
	c.OffsetParameter = &parameter
	if expression != nil && strings.TrimSpace(*expression) != "" {
		names := map[string]modelcore.ParameterBinding{}
		for _, other := range model.Constraints {
			if p := other.OffsetParameter; p != nil {
				names[p.Key] = modelcore.ParameterBinding{ParameterID: p.ParameterID, Dimension: p.Dimension}
			}
		}
		compiled, err := modelcore.CompileExpression(*expression, names, modelcore.LengthDimension)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrValidation, err)
		}
		parameter.Source = modelcore.ValueSource{Expression: &compiled}
	} else if expression != nil || parameter.Source.Expression == nil {
		q, err := modelcore.NewQuantity(literalMM, "mm")
		if err != nil {
			return fmt.Errorf("%w: %w", ErrValidation, err)
		}
		parameter.Source = modelcore.ValueSource{Literal: &q}
	}
	return resolveOffsetParameters(model)
}

func resolveOffsetParameters(model *ProductModel) error {
	// Reuse the existing Quantity evaluator, dimension/dependency checks and
	// cycle gate; the temporary input has parameters only, no Part/CAD bodies.
	input := PartModel{}
	keys := map[string]string{}
	for _, c := range model.Constraints {
		if p := c.OffsetParameter; p != nil {
			input.Parameters = append(input.Parameters, *p)
			keys[p.ParameterID] = p.Key
		}
	}
	if len(input.Parameters) == 0 {
		return nil
	}
	if err := validateAndResolvePartParameters(&input); err != nil {
		return err
	}
	byID := map[string]modelcore.ParameterDefinition{}
	for _, p := range input.Parameters {
		if p.Source.Expression != nil {
			// Copy before presenting renamed keys; never mutate an old snapshot.
			e := *p.Source.Expression
			text, err := modelcore.FormatExpression(e, keys)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrValidation, err)
			}
			e.SourceText = text
			p.Source.Expression = &e
		}
		byID[p.ParameterID] = p
	}
	for i := range model.Constraints {
		c := &model.Constraints[i]
		if c.OffsetParameter == nil {
			continue
		}
		if c.Kind != "DISTANCE" || c.OffsetParameter.ParameterID != "offset:"+c.ID || !c.OffsetParameter.Dimension.Equal(modelcore.LengthDimension) {
			return fmt.Errorf("%w: invalid Offset parameter ownership/dimension", ErrValidation)
		}
		p := byID[c.OffsetParameter.ParameterID]
		value, err := quantityInDisplayUnit(*p.EvaluatedValue, "mm")
		if err != nil || !finite(value) {
			return fmt.Errorf("%w: Offset must evaluate to finite length", ErrValidation)
		}
		c.Value, c.OffsetParameter = value, &p
		if err := validateInstanceConstraintReferences(*c); err != nil {
			return err
		}
	}
	return nil
}
