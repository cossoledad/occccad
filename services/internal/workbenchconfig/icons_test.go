package workbenchconfig

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestIconCompositionAndFlatWireCatalog(t *testing.T) {
	c, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Parse(encoded)
	if err != nil || !reflect.DeepEqual(c.Icons, decoded.Icons) {
		t.Fatalf("flat wire round trip: %v", err)
	}
	// Optional empty maps elsewhere in Catalog normalize to nil through
	// omitempty. Compare their actual wire representation, not Go allocation.
	roundTrip, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(encoded, roundTrip) {
		t.Fatalf("wire catalog changed after decoding: %v", err)
	}
	seen := map[string]bool{}
	for _, key := range []string{"pad", "pocket", "hole", "fillet", "chamfer", "draft", "shell", "sweep", "loft", "surface-extrude", "surface-revolve", "surface-sweep", "surface-loft"} {
		icon, ok := c.Icons[key]
		if !ok {
			t.Fatalf("missing semantic glyph %s", key)
		}
		shape, _ := json.Marshal(icon.Elements)
		if seen[string(shape)] {
			t.Fatalf("modeling operations collapse to the same glyph: %s", key)
		}
		seen[string(shape)] = true
	}
	for _, raw := range []string{
		`{"schemaVersion":2,"symbols":{},"icons":{}}`,
		`{"schemaVersion":1,"schemaVersion":1,"symbols":{},"icons":{}}`,
		`{"schemaVersion":1,"symbols":{},"icons":{"x":{"family":"solid","parts":["missing"]}}}`,
		`{"schemaVersion":1,"symbols":{},"icons":{"x":{"family":"unknown","elements":[{"tag":"path","attributes":{"d":"M1 1"}}]}}}`,
		`{"schemaVersion":1,"symbols":{},"icons":{"x":{"family":"solid"}}}`,
		`{"schemaVersion":1,"symbols":{},"icons":{}} {}`,
	} {
		if _, err := compileIcons([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid icon source: %s", raw)
		}
	}
}
