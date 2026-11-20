package workspace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func FuzzCompactAssociationOrdinals(f *testing.F) {
	for _, input := range []string{"null", "[]", "[[],[0,128,4294967295],[]]", "[[4294967296]]", "[[01]]", "[[2,null],null]", " [ [ 0 ] ] ", "[[1],]", "[[1.0]]", "[[1e2]]", "[[1,2],[-1]]"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		var got associationOrdinals
		var want [][]uint32
		actual, expected := json.Unmarshal([]byte(input), &got), json.Unmarshal([]byte(input), &want)
		if (actual == nil) != (expected == nil) || actual == nil && !reflect.DeepEqual([][]uint32(got), want) {
			t.Fatalf("ordinal decode %q: got %v (%v), want %v (%v)", input, got, actual, want, expected)
		}
	})
}
