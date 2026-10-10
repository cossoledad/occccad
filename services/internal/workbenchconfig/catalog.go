// Package workbenchconfig owns declarative document and command presentation.
// It contains no model state, evaluation, or executable browser code.
package workbenchconfig

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

//go:embed catalog.json
var builtin []byte

type Condition struct {
	Fact   string      `json:"fact,omitempty"`
	Equals any         `json:"equals,omitempty"`
	All    []Condition `json:"all,omitempty"`
	Any    []Condition `json:"any"`
	Not    *Condition  `json:"not,omitempty"`
}
type Document struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	HelpText string `json:"helpText"`
	IconKey  string `json:"iconKey"`
	Adapter  string `json:"adapter"`
}
type Element struct {
	Tag        string            `json:"tag"`
	Attributes map[string]string `json:"attributes"`
}
type Icon struct {
	ClassName string    `json:"className,omitempty"`
	Elements  []Element `json:"elements,omitempty"`
	Resource  string    `json:"resource,omitempty"`
}
type Command struct {
	Labels         map[string]string `json:"labels,omitempty"`
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	HelpText       string            `json:"helpText"`
	IconKey        string            `json:"iconKey"`
	Repeatable     bool              `json:"repeatable"`
	Implementation string            `json:"implementation"`
	VisibleWhen    Condition         `json:"visibleWhen"`
	EnabledWhen    Condition         `json:"enabledWhen"`
}
type CommandPlacement struct {
	CommandID string `json:"commandId"`
	GroupKey  string `json:"groupKey"`
	Order     int    `json:"order"`
}
type Group struct {
	Variants    map[string]string  `json:"variants,omitempty"`
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	StyleKey    string             `json:"styleKey"`
	Position    string             `json:"position"`
	Orientation string             `json:"orientation"`
	Commands    []CommandPlacement `json:"commands"`
}
type Tab struct {
	Domain    string `json:"domain"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ModeLabel string `json:"modeLabel"`
	Workbench string `json:"workbench"`
	Section   string `json:"section"`
	Order     int    `json:"order"`
}
type Context struct {
	ID   string    `json:"id"`
	When Condition `json:"when"`
	Tabs []string  `json:"tabs"`
}
type Placement struct {
	TabID   string `json:"tabId"`
	GroupID string `json:"groupId"`
	Order   int    `json:"order"`
}
type Catalog struct {
	SchemaVersion int             `json:"schemaVersion"`
	Documents     []Document      `json:"documents"`
	Icons         map[string]Icon `json:"icons"`
	Commands      []Command       `json:"commands"`
	Groups        []Group         `json:"groups"`
	Tabs          []Tab           `json:"tabs"`
	Contexts      []Context       `json:"contexts"`
	Placements    []Placement     `json:"placements"`
}

func Read() (Catalog, error) { return Parse(builtin) }
func Parse(data []byte) (Catalog, error) {
	var c Catalog
	if err := validateJSONKeys(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return c, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return Catalog{}, fmt.Errorf("catalog has trailing data")
	}
	if err := c.Validate(); err != nil {
		return Catalog{}, err
	}
	return c, nil
}

// JSON objects must not silently overwrite icon IDs or declaration fields.
func validateJSONKeys(d *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("catalog nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return fmt.Errorf("duplicate or invalid configuration key %q", key)
			}
			seen[key] = true
			if err = validateJSONKeys(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = validateJSONKeys(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected catalog delimiter")
	}
	_, err = d.Token()
	return err
}
func unique(kind string, ids []string) (map[string]bool, error) {
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return nil, fmt.Errorf("%s: empty or duplicate ID %q", kind, id)
		}
		seen[id] = true
	}
	return seen, nil
}
func (c Catalog) Validate() error {
	if len(c.Documents) == 0 || len(c.Contexts) == 0 || len(c.Tabs) == 0 {
		return fmt.Errorf("catalog missing document contexts or tabs")
	}
	if c.SchemaVersion != 1 {
		return fmt.Errorf("unsupported catalog schema %d", c.SchemaVersion)
	}
	docIDs, cmdIDs, groupIDs, tabIDs, contextIDs := []string{}, []string{}, []string{}, []string{}, []string{}
	for _, v := range c.Documents {
		docIDs = append(docIDs, v.ID)
	}
	for _, v := range c.Commands {
		cmdIDs = append(cmdIDs, v.ID)
	}
	for _, v := range c.Groups {
		groupIDs = append(groupIDs, v.ID)
	}
	for _, v := range c.Tabs {
		tabIDs = append(tabIDs, v.ID)
	}
	for _, v := range c.Contexts {
		contextIDs = append(contextIDs, v.ID)
	}
	for kind, ids := range map[string][]string{"document": docIDs, "command": cmdIDs, "group": groupIDs, "tab": tabIDs, "context": contextIDs} {
		if _, err := unique(kind, ids); err != nil {
			return err
		}
	}
	documents, _ := unique("document", docIDs)
	commands, _ := unique("command", cmdIDs)
	groups, _ := unique("group", groupIDs)
	tabs, _ := unique("tab", tabIDs)
	tabWorkbenches := map[string]string{}
	for _, tab := range c.Tabs {
		tabWorkbenches[tab.ID] = tab.Workbench
	}
	for _, v := range c.Documents {
		if v.Name == "" || v.Adapter == "" {
			return fmt.Errorf("document %s missing adapter/name", v.ID)
		}
		if _, ok := c.Icons[v.IconKey]; !ok {
			return fmt.Errorf("document %s missing icon %s", v.ID, v.IconKey)
		}
	}
	for _, v := range c.Commands {
		if v.Name == "" || v.HelpText == "" {
			return fmt.Errorf("command %s missing text", v.ID)
		}
		if v.Implementation != "handler" && v.Implementation != "tool" && v.Implementation != "form" {
			return fmt.Errorf("command %s missing implementation kind", v.ID)
		}
		if _, ok := c.Icons[v.IconKey]; !ok {
			return fmt.Errorf("command %s missing icon %s", v.ID, v.IconKey)
		}
		if err := validateCondition(v.VisibleWhen, 0, documents); err != nil {
			return err
		}
		if err := validateCondition(v.EnabledWhen, 0, documents); err != nil {
			return err
		}
	}
	allowedTags := map[string]bool{"path": true, "circle": true, "rect": true, "line": true, "polyline": true, "polygon": true, "ellipse": true}
	allowedAttributes := map[string]bool{"d": true, "cx": true, "cy": true, "r": true, "x": true, "y": true, "width": true, "height": true, "rx": true, "ry": true, "x1": true, "x2": true, "y1": true, "y2": true, "points": true, "className": true}
	for id, icon := range c.Icons {
		if icon.Resource != "" && (len(icon.Resource) < 8 || icon.Resource[:8] != "/assets/" || strings.Contains(icon.Resource, "..") || strings.ContainsAny(icon.Resource, "?#\\")) {
			return fmt.Errorf("icon %s: resource must be under /assets/", id)
		}
		for _, e := range icon.Elements {
			if !allowedTags[e.Tag] {
				return fmt.Errorf("icon %s: unsupported SVG element", id)
			}
			for a := range e.Attributes {
				if !allowedAttributes[a] {
					return fmt.Errorf("icon %s: unsupported SVG attribute %s", id, a)
				}
			}
		}
	}
	for _, tab := range c.Tabs {
		if tab.Name == "" || tab.Workbench == "" || tab.ModeLabel == "" || tab.Section == "" {
			return fmt.Errorf("tab %s missing presentation", tab.ID)
		}
	}
	for _, g := range c.Groups {
		if g.Name == "" {
			return fmt.Errorf("group %s missing name", g.ID)
		}
		if !map[string]bool{"top-left": true, "top-center": true, "top-right": true, "bottom-left": true, "bottom-center": true, "bottom-right": true}[g.Position] {
			return fmt.Errorf("group %s invalid position", g.ID)
		}
		ids := []string{}
		for _, p := range g.Commands {
			if strings.HasPrefix(p.GroupKey, "variants:") && g.Variants[p.GroupKey] == "" {
				return fmt.Errorf("group %s missing variant label %s", g.ID, p.GroupKey)
			}
			if !commands[p.CommandID] {
				return fmt.Errorf("group %s: missing command %s", g.ID, p.CommandID)
			}
			ids = append(ids, p.CommandID)
		}
		if _, err := unique("group command", ids); err != nil {
			return err
		}
	}
	for _, ctx := range c.Contexts {
		if err := validateCondition(ctx.When, 0, documents); err != nil {
			return err
		}
		if len(ctx.Tabs) == 0 {
			return fmt.Errorf("context %s has no tabs", ctx.ID)
		}
		if _, err := unique("context tabs", ctx.Tabs); err != nil {
			return err
		}
		for _, id := range ctx.Tabs {
			if tabWorkbenches[id] != tabWorkbenches[ctx.Tabs[0]] {
				return fmt.Errorf("context %s mixes workbench modes", ctx.ID)
			}
			if !tabs[id] {
				return fmt.Errorf("context %s: missing tab %s", ctx.ID, id)
			}
		}
	}
	ids := []string{}
	for _, p := range c.Placements {
		if !tabs[p.TabID] || !groups[p.GroupID] {
			return fmt.Errorf("missing placement reference %s/%s", p.TabID, p.GroupID)
		}
		ids = append(ids, p.TabID+"/"+p.GroupID)
	}
	_, err := unique("placement", ids)
	return err
}
func validateCondition(c Condition, depth int, documents map[string]bool) error {
	if depth > 16 {
		return fmt.Errorf("condition nesting exceeds limit")
	}
	if c.Fact != "" {
		allowed := map[string]bool{"hostType": true, "targetType": true, "sketchActive": true, "canEdit": true, "busy": true, "selectionKind": true, "selectionCount": true, "hasWorkingBody": true, "rootCanEdit": true, "isMock": true, "moveReceiptPending": true, "rootTarget": true, "motionActive": true}
		if !allowed[c.Fact] {
			return fmt.Errorf("unknown context fact %s", c.Fact)
		}
		isBoolean := c.Fact == "sketchActive" || c.Fact == "canEdit" || c.Fact == "busy" || c.Fact == "hasWorkingBody" || c.Fact == "rootCanEdit" || c.Fact == "isMock" || c.Fact == "moveReceiptPending" || c.Fact == "rootTarget" || c.Fact == "motionActive"
		if isBoolean {
			if _, ok := c.Equals.(bool); !ok {
				return fmt.Errorf("condition %s requires a boolean", c.Fact)
			}
		}
		if c.Fact == "hostType" || c.Fact == "targetType" || c.Fact == "selectionKind" {
			if _, ok := c.Equals.(string); !ok {
				return fmt.Errorf("condition %s requires a string", c.Fact)
			}
		}
		if c.Fact == "hostType" || c.Fact == "targetType" {
			if id, ok := c.Equals.(string); !ok || !documents[id] {
				return fmt.Errorf("condition %s references unregistered document type", c.Fact)
			}
		}
		if c.Fact == "selectionCount" {
			if _, ok := c.Equals.(float64); !ok {
				return fmt.Errorf("condition %s requires a number", c.Fact)
			}
		}
		if c.Equals == nil {
			return fmt.Errorf("condition %s missing equals", c.Fact)
		}
	}
	for _, children := range [][]Condition{c.All, c.Any} {
		for _, child := range children {
			if err := validateCondition(child, depth+1, documents); err != nil {
				return err
			}
		}
	}
	if c.Not != nil {
		return validateCondition(*c.Not, depth+1, documents)
	}
	return nil
}

// Toolbar is a projection for existing ribbon components, never a second source.
type Item struct {
	VariantLabel string `json:"variantLabel,omitempty"`
	CommandID    string `json:"commandId"`
	Name         string `json:"name"`
	HelpText     string `json:"helpText"`
	IconKey      string `json:"iconKey"`
	GroupKey     string `json:"groupKey"`
	SortOrder    int    `json:"sortOrder"`
	Repeatable   bool   `json:"repeatable"`
}
type Toolbar struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Workbench   string   `json:"workbench"`
	Position    string   `json:"position"`
	Orientation string   `json:"orientation"`
	StyleKey    string   `json:"styleKey"`
	SortOrder   int      `json:"sortOrder"`
	Items       []Item   `json:"items"`
	TabIDs      []string `json:"tabIds"`
	Section     string   `json:"section"`
}

func (c Catalog) Toolbars() []Toolbar {
	commands := map[string]Command{}
	tabs := map[string]Tab{}
	groups := map[string]Group{}
	for _, v := range c.Commands {
		commands[v.ID] = v
	}
	for _, v := range c.Tabs {
		tabs[v.ID] = v
	}
	for _, v := range c.Groups {
		groups[v.ID] = v
	}
	result := []Toolbar{}
	for _, placement := range c.Placements {
		g, tab := groups[placement.GroupID], tabs[placement.TabID]
		b := Toolbar{ID: g.ID, Name: g.Name, Position: g.Position, Orientation: g.Orientation, StyleKey: g.StyleKey, Workbench: tab.Workbench, SortOrder: placement.Order, Section: tab.Section, Items: []Item{}, TabIDs: []string{tab.ID}}
		for _, p := range g.Commands {
			v := commands[p.CommandID]
			b.Items = append(b.Items, Item{VariantLabel: g.Variants[p.GroupKey], CommandID: v.ID, Name: v.Name, HelpText: v.HelpText, IconKey: v.IconKey, GroupKey: p.GroupKey, SortOrder: p.Order, Repeatable: v.Repeatable})
		}
		sort.SliceStable(b.Items, func(i, j int) bool { return b.Items[i].SortOrder < b.Items[j].SortOrder })
		result = append(result, b)
	}
	return result
}
