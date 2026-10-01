package modelcore

import (
	"math"
	"testing"
)

func TestSignedLengthExpressionUsesExistingStableAST(t *testing.T) {
	for _, source := range []string{"-2 mm", "-(Base + 1 mm)", "+Base * -2"} {
		expression, err := CompileExpression(source, map[string]ParameterBinding{"Base": {ParameterID: "stable-base", Dimension: LengthDimension}}, LengthDimension)
		if err != nil {
			t.Fatal(source, err)
		}
		base, _ := NewQuantity(1, "mm")
		value, err := EvaluateExpression(expression, map[string]Quantity{"stable-base": base})
		if err != nil || math.Abs(value.SIValue+0.002) > 1e-15 {
			t.Fatal(source, value, err)
		}
		rendered, err := FormatExpression(expression, map[string]string{"stable-base": "Renamed"})
		if err != nil {
			t.Fatal(err)
		}
		roundtrip, err := CompileExpression(rendered, map[string]ParameterBinding{"Renamed": {ParameterID: "stable-base", Dimension: LengthDimension}}, LengthDimension)
		if err != nil {
			t.Fatal(err)
		}
		again, err := EvaluateExpression(roundtrip, map[string]Quantity{"stable-base": base})
		if err != nil || again != value {
			t.Fatal(again, err)
		}
	}
	if _, err := CompileExpression("-2 deg", nil, LengthDimension); err == nil {
		t.Fatal("signed dimension error accepted")
	}
}
