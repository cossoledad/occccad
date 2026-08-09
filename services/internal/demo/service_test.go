package demo

import "testing"

func TestGeometryKeyIsDeterministicAndParameterSensitive(t *testing.T) {
	t.Parallel()
	first := geometryKey(100, 60, 40)
	second := geometryKey(100, 60, 40)
	changed := geometryKey(100, 60, 41)
	if first != second {
		t.Fatalf("same feature model produced different keys: %q != %q", first, second)
	}
	if first == changed {
		t.Fatal("different pad length produced the same geometry key")
	}
	if len(first) != len("sha256:")+64 {
		t.Fatalf("unexpected GeometryKey length: %d", len(first))
	}
}
