package workbenchconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogReferencesAndPositionOrdering(t *testing.T) {
	c, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Documents) != 2 || len(c.Commands) < 100 {
		t.Fatal("existing capabilities lost")
	}
	c.Placements = append(c.Placements, Placement{TabID: "PART_DESIGN.document", GroupID: "view-navigation", Order: -10})
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	bars := c.Toolbars()
	last := bars[len(bars)-1]
	if last.SortOrder != -10 || last.TabIDs[0] != "PART_DESIGN.document" || last.Items[0].CommandID != "view.fit" {
		t.Fatal("position did not preserve independent order")
	}
	for _, mutate := range []func(*Catalog){
		func(c *Catalog) { c.Commands = append(c.Commands, c.Commands[0]) },
		func(c *Catalog) { c.Groups[0].Commands[0].CommandID = "missing" },
		func(c *Catalog) { c.Placements[0].TabID = "missing" },
		func(c *Catalog) { c.Commands[0].IconKey = "missing" },
		func(c *Catalog) { c.Commands[0].EnabledWhen = Condition{Fact: "unknown", Equals: true} },
		func(c *Catalog) { c.Commands[0].EnabledWhen = Condition{Fact: "targetType", Equals: "UNREGISTERED"} },
		func(c *Catalog) { c.Contexts = nil },
		func(c *Catalog) { c.Icons["select"] = Icon{Resource: "https://invalid/icon.svg"} },
	} {
		c, err := Read()
		if err != nil {
			t.Fatal(err)
		}
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("invalid catalog published")
		}
	}
}

func TestCatalogIsDetachedFromStorage(t *testing.T) {
	// Returned objects may be changed by a caller without changing the embedded source.
	first, _ := Read()
	first.Commands[0].Name = "changed"
	second, _ := Read()
	if second.Commands[0].Name == "changed" {
		t.Fatal("catalog mutation escaped read")
	}
	encoded, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(encoded); err != nil {
		t.Fatal(err)
	}
}

// These presentation assertions moved from the retired database seed test to
// the sole catalog source; migration tests continue to prove history retention.
func TestSketchCatalogRetainsExistingCapabilities(t *testing.T) {
	c, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	commands := map[string]Command{}
	for _, command := range c.Commands {
		commands[command.ID] = command
	}
	for _, id := range []string{"sketch.slot", "sketch.edit.scale", "sketch.edit.quick_trim", "sketch.edit.rotate", "sketch.normal"} {
		if _, ok := commands[id]; ok {
			t.Fatalf("obsolete command %s", id)
		}
	}
	for _, id := range []string{"sketch.circle.three_point", "sketch.arc.three_point", "sketch.rectangle.center", "sketch.rectangle.oriented", "sketch.ellipse", "sketch.elliptical_arc", "sketch.spline.control", "sketch.constraint.collinear", "sketch.constraint.horizontal_distance", "sketch.constraint.vertical_distance", "sketch.constraint.major_radius", "sketch.constraint.minor_radius"} {
		if _, ok := commands[id]; !ok {
			t.Fatalf("missing sketch command %s", id)
		}
	}
	for id, text := range map[string]string{"sketch.point": "C 切换", "sketch.line": "Tab", "sketch.polyline": "T 切换", "sketch.spline": "拟合点"} {
		if !strings.Contains(commands[id].HelpText, text) {
			t.Fatalf("lost help for %s", id)
		}
	}
	edit, polygons, arcs := 0, 0, 0
	families := map[string]bool{}
	for _, group := range c.Groups {
		for _, item := range group.Commands {
			if item.GroupKey == "more" {
				t.Fatal("obsolete overflow group")
			}
			if group.ID == "sketch-edit" && strings.HasPrefix(item.CommandID, "sketch.") {
				edit++
			}
			if group.ID == "sketch-dimensional-constraints" {
				families[item.GroupKey] = true
			}
			if item.GroupKey == "variants:polygon" {
				polygons++
			}
			if group.ID == "sketch-profiles" && item.GroupKey == "variants:arc" {
				arcs++
			}
		}
	}
	if edit != 17 || len(families) != 3 || polygons != 2 || arcs != 3 {
		t.Fatalf("sketch placement drift edit=%d families=%d polygons=%d arcs=%d", edit, len(families), polygons, arcs)
	}
}
func TestCatalogRejectsPartialAndUnsafeConfiguration(t *testing.T) {
	for _, input := range [][]byte{[]byte(`{"schemaVersion":1,"schemaVersion":1}`), []byte(`{"schemaVersion":1,"unknown":true}`), append(append([]byte{}, builtin...), []byte(` {}`)...)} {
		if _, err := Parse(input); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	for _, mutate := range []func(*Catalog){
		func(c *Catalog) { c.Icons["select"] = Icon{Resource: "/assets/../private.svg"} },
		func(c *Catalog) { c.Icons["select"] = Icon{Elements: []Element{{Tag: "script"}}} },
		func(c *Catalog) { c.Commands[0].EnabledWhen = Condition{Fact: "busy", Equals: "true"} },
	} {
		c, _ := Read()
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}
