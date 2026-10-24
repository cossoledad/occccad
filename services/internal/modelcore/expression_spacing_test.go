package modelcore

import (
	"math"
	"reflect"
	"testing"
)

func TestCompactExpressionInputPreservesUnitsAndBindings(t *testing.T) {
	pairs := [][2]string{{"len/2", "len / 2"}, {"(len+2 mm)/2", "( len + 2 mm ) / 2"}, {"-(-len)/2", "- ( - len ) / 2"}, {"len*1e-2", "len * 1e-2"}, {"(len/2)+1e-3 m", "( len / 2 ) + 1e-3 m"}}
	for _, p := range pairs {
		a, e := CompileExpression(p[0], map[string]ParameterBinding{"len": {ParameterID: "stable-length", Dimension: LengthDimension}}, LengthDimension, "mm")
		if e != nil {
			t.Fatal(p, e)
		}
		b, e := CompileExpression(p[1], map[string]ParameterBinding{"len": {ParameterID: "stable-length", Dimension: LengthDimension}}, LengthDimension, "mm")
		if e != nil {
			t.Fatal(p, e)
		}
		if a.SourceText != p[0] || b.SourceText != p[1] || !reflect.DeepEqual(a.CheckedAST, b.CheckedAST) || !reflect.DeepEqual(a.Reads, b.Reads) {
			t.Fatalf("spacing changed AST/reads/source: %+v %+v", a, b)
		}
		q, _ := NewQuantity(20, "mm")
		x, e := EvaluateExpression(a, map[string]Quantity{"stable-length": q})
		if e != nil {
			t.Fatal(e)
		}
		y, e := EvaluateExpression(b, map[string]Quantity{"stable-length": q})
		if e != nil || math.Abs(x.SIValue-y.SIValue) > 1e-15 || !x.Dimension.Equal(LengthDimension) {
			t.Fatal(x, y, e)
		}
	}
}
