package valuecopy

import (
	"math"
	"testing"
)

func TestCloneSeparatesNestedModelValuesAndPreservesIntegers(t *testing.T) {
	n := int64(math.MaxInt64)
	v := struct {
		Values   map[string][]*int64
		Optional *float64
	}{Values: map[string][]*int64{"key": {&n}}}
	frozen, err := Clone(v)
	if err != nil {
		t.Fatal(err)
	}
	*frozen.Values["key"][0] = 7
	frozen.Values["key"] = append(frozen.Values["key"], &n)
	if n != math.MaxInt64 || len(v.Values["key"]) != 1 || frozen.Optional != nil {
		t.Fatal("freeze polluted original or optional field")
	}
	if _, err := Clone(math.NaN()); err == nil {
		t.Fatal("accepted non-finite input")
	}
	type node struct{ Next *node }
	chain := &node{}
	for i := 0; i < 200; i++ {
		chain = &node{Next: chain}
	}
	if _, err := Clone(chain); err != nil {
		t.Fatal("valid deep expression-shaped model rejected", err)
	}
	cyclic := &node{}
	cyclic.Next = cyclic
	if _, err := Clone(cyclic); err == nil {
		t.Fatal("accepted cyclic value")
	}
}
