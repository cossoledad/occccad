package modelcore

import (
	"math"
	"testing"
)

func TestGeometricQuantityUnitsAndDimensions(t *testing.T) {
	for _, c := range []struct {
		unit      string
		si        float64
		dimension Dimension
	}{{"mm2", 1e-6, AreaDimension}, {"cm2", 1e-4, AreaDimension}, {"in2", .00064516, AreaDimension}, {"mm3", 1e-9, VolumeDimension}, {"cm3", 1e-6, VolumeDimension}, {"in3", .000016387064, VolumeDimension}} {
		q, err := NewQuantity(1, c.unit)
		if err != nil || math.Abs(q.SIValue-c.si) > 1e-15 || !q.Dimension.Equal(c.dimension) {
			t.Fatal(c, q, err)
		}
	}
	expression, err := CompileExpression("2 mm * 3 mm", nil, AreaDimension, "mm2")
	if err != nil {
		t.Fatal(err)
	}
	value, err := EvaluateExpression(expression, nil)
	if err != nil || math.Abs(value.SIValue-6e-6) > 1e-15 {
		t.Fatal(value, err)
	}
	if _, err := CompileExpression("2 mm + 3 mm2", nil, AreaDimension, "mm2"); err == nil {
		t.Fatal("mixed dimensions accepted")
	}
}
