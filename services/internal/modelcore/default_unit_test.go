package modelcore

import (
	"errors"
	"math"
	"testing"
)

func TestExpressionContextDefaultUnit(t *testing.T) {
	names := map[string]ParameterBinding{"p111": {ParameterID: "radius", Dimension: LengthDimension}, "r_1": {ParameterID: "radius", Dimension: LengthDimension}, "r_2": {ParameterID: "other", Dimension: LengthDimension}}
	radius, _ := NewQuantity(10, "mm")
	other, _ := NewQuantity(2, "mm")
	for _, test := range []struct {
		source, unit string
		want         float64
	}{{"p111 + 4", "mm", .014}, {"r_1 + 5", "mm", .015}, {"5 + r_1", "mm", .015}, {"(r_1 + 5) * 2", "mm", .030}, {"r_1 * 2", "mm", .020}, {"r_1 + 5 * 2", "mm", .020}, {"r_1 - 5", "mm", .005}, {"r_1 + -5", "mm", .005}, {"5 + 5", "mm", .010}, {"r_1 + 5 cm", "mm", .060}, {"r_1 + 5", "cm", .060}} {
		t.Run(test.source+test.unit, func(t *testing.T) {
			expression, err := CompileExpression(test.source, names, LengthDimension, test.unit)
			if err != nil {
				t.Fatal(err)
			}
			value, err := EvaluateExpression(expression, map[string]Quantity{"radius": radius, "other": other})
			if err != nil || math.Abs(value.SIValue-test.want) > 1e-12 {
				t.Fatalf("value=%+v err=%v want=%g", value, err, test.want)
			}
			if expression.SourceText != test.source {
				t.Fatal("lost source text")
			}
		})
	}
	for _, source := range []string{"r_1 + 5 deg", "r_1 / r_2 + 5", "r_1 * 5 mm"} {
		if _, err := CompileExpression(source, names, LengthDimension, "mm"); !errors.Is(err, ErrUnitMismatch) {
			t.Fatalf("%s: %v", source, err)
		}
	}
	if _, err := CompileExpression("r_1 + 5", names, LengthDimension); !errors.Is(err, ErrUnitMismatch) {
		t.Fatal("strict callers must retain explicit units")
	}
	angle, err := CompileExpression("30 + 5", nil, AngleDimension, "deg")
	if err != nil {
		t.Fatal(err)
	}
	q, err := EvaluateExpression(angle, nil)
	if err != nil || math.Abs(q.SIValue-35*math.Pi/180) > 1e-12 {
		t.Fatalf("angle=%+v %v", q, err)
	}
}
