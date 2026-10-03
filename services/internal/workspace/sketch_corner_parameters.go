package workspace

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// Corner candidates use the same checked, unit-aware source as their persisted
// primary dimension. The client's numeric preview is never authoritative when
// a source is supplied.
func resolveSketchCornerOperationValues(model PartModel, operations []SketchOperation) ([]SketchOperation, error) {
	result := append([]SketchOperation(nil), operations...)
	for i, op := range result {
		if (op.Type != "FILLET_ENTITIES" && op.Type != "CHAMFER_ENTITIES") || op.ParameterSource == nil {
			continue
		}
		sourceText := strings.TrimSpace(*op.ParameterSource)
		if literal, err := strconv.ParseFloat(sourceText, 64); err == nil {
			if !finite(literal) {
				return nil, fmt.Errorf("%w: corner length must be finite", ErrValidation)
			}
			unit := model.Units
			if unit == "" {
				unit = "mm"
			}
			sourceText = strconv.FormatFloat(literal, 'g', -1, 64) + " " + unit
			op.ParameterSource = &sourceText
		}
		dimension, _ := modelcore.NewQuantity(1, "mm")
		source, err := compileSketchDimensionSource(model, sourceText, dimension.Dimension)
		if err != nil {
			return nil, err
		}
		var quantity modelcore.Quantity
		if source.Literal != nil {
			quantity = *source.Literal
		} else {
			values := map[string]modelcore.Quantity{}
			for _, parameter := range model.Parameters {
				if parameter.EvaluatedValue != nil {
					values[parameter.ParameterID] = *parameter.EvaluatedValue
				}
			}
			quantity, err = modelcore.EvaluateExpression(*source.Expression, values)
			if err != nil {
				return nil, fmt.Errorf("%w: corner primary dimension: %w", ErrValidation, err)
			}
		}
		value := quantity.SIValue * 1000
		if !finite(value) || value <= 0 {
			return nil, fmt.Errorf("%w: corner primary length must be positive", ErrValidation)
		}
		if op.Type == "FILLET_ENTITIES" {
			op.Value = &value
		} else {
			op.ChamferFirst = value
		}
		result[i] = op
	}
	return result, nil
}
