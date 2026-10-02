package workspace

import (
	"encoding/json"
	"fmt"
	"math"
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

func assemblyQuantityParameter(c AssemblyConstraint) *modelcore.ParameterDefinition {
	return c.QuantityParameter
}

type assemblyQuantitySpec struct {
	prefix, label, unit string
	dimension           modelcore.Dimension
}

func assemblyQuantityInputs(kind string, expression *string, key string, offsetExpression *string, offsetKey string) (*string, string, error) {
	if offsetExpression != nil || offsetKey != "" {
		return nil, "", fmt.Errorf("%w: unsupported experimental Offset inputs; use Quantity", ErrValidation)
	}
	return expression, key, nil
}

func assemblyQuantitySpecification(c AssemblyConstraint) (assemblyQuantitySpec, bool) {
	if c.Kind == "DISTANCE" {
		return assemblyQuantitySpec{"offset:", "Offset", "mm", modelcore.LengthDimension}, true
	}
	if c.Kind == "ANGLE" && (c.AngleRelation == "" || c.AngleRelation == "FREE" || c.AngleRelation == "DIRECTED") {
		return assemblyQuantitySpec{"angle:", "Angle", "rad", modelcore.AngleDimension}, true
	}
	return assemblyQuantitySpec{}, false
}

// One AST/evaluator for length and angle, bound to same-Product stable IDs.
// Literals enter in Worker units (mm/rad); Quantity itself retains SI values.
func editAssemblyQuantity(model *ProductModel, index int, expression *string, key string, literal float64) error {
	c := &model.Constraints[index]
	spec, supported := assemblyQuantitySpecification(*c)
	if !supported {
		if expression != nil || key != "" {
			return fmt.Errorf("%w: relation has no editable Quantity parameter", ErrValidation)
		}
		// A legal relation switch drops its now-inapplicable parameter atomically.
		c.QuantityParameter = nil
		return nil
	}
	parameter := modelcore.ParameterDefinition{ParameterID: spec.prefix + c.ID, Key: spec.label + "_" + parameterKeyFragment(c.ID),
		Label: spec.label, ValueType: modelcore.ValueQuantity, Dimension: spec.dimension, DisplayUnit: spec.unit, Role: "DRIVING", PropertySlot: "assembly-constraint.entity"}
	if existing := assemblyQuantityParameter(*c); existing != nil {
		raw, _ := json.Marshal(existing)
		if err := json.Unmarshal(raw, &parameter); err != nil {
			return err
		}
	}
	if key != "" {
		parameter.Key = key
	}
	if !validParameterKey(parameter.Key) {
		return fmt.Errorf("%w: Quantity key must be an ASCII identifier", ErrValidation)
	}
	c.QuantityParameter, c.DefinitionVersion = &parameter, 2
	if expression != nil && strings.TrimSpace(*expression) != "" {
		names := map[string]modelcore.ParameterBinding{}
		for _, other := range model.Constraints {
			if p := assemblyQuantityParameter(other); p != nil {
				names[p.Key] = modelcore.ParameterBinding{ParameterID: p.ParameterID, Dimension: p.Dimension}
			}
		}
		compiled, err := modelcore.CompileExpression(*expression, names, spec.dimension)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrValidation, err)
		}
		parameter.Source = modelcore.ValueSource{Expression: &compiled}
	} else if expression != nil || parameter.Source.Expression == nil {
		q, err := modelcore.NewQuantity(literal, spec.unit)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrValidation, err)
		}
		parameter.Source = modelcore.ValueSource{Literal: &q}
	}
	return resolveAssemblyQuantities(model)
}

func resolveAssemblyQuantities(model *ProductModel) error {
	// Reuse the existing Quantity evaluator, dimension/dependency checks and
	// cycle gate; the temporary input has parameters only, no Part/CAD bodies.
	input := PartModel{}
	keys := map[string]string{}
	for _, c := range model.Constraints {
		if p := assemblyQuantityParameter(c); p != nil {
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
		parameter := assemblyQuantityParameter(*c)
		if parameter == nil {
			continue
		}
		spec, supported := assemblyQuantitySpecification(*c)
		if !supported || parameter.ParameterID != spec.prefix+c.ID || !parameter.Dimension.Equal(spec.dimension) {
			return fmt.Errorf("%w: invalid assembly Quantity ownership/dimension", ErrValidation)
		}
		p := byID[parameter.ParameterID]
		value, err := quantityInDisplayUnit(*p.EvaluatedValue, spec.unit)
		if err != nil || !finite(value) {
			return fmt.Errorf("%w: assembly Quantity must evaluate to finite %s", ErrValidation, spec.dimension.Semantic)
		}
		if c.Kind == "ANGLE" {
			if value < 0 || value > 2*math.Pi {
				return fmt.Errorf("%w: angle Quantity must be in [0, 2pi]", ErrValidation)
			}
			if value == 2*math.Pi {
				value = 0
			} // Static canonical angle; AST/literal stays intact.
		}
		c.Value = value
		c.QuantityParameter = &p
		if err := validateInstanceConstraintReferences(*c); err != nil {
			return err
		}
	}
	return nil
}
