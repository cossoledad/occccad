package automation

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workbenchconfig"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAutomationEveryCommandHasDiscoverableContract(t *testing.T) {
	catalog := NewCatalog()
	source, err := parser.ParseFile(token.NewFileSet(), "../workspace/legacy_commands.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]bool{}
	for _, decl := range source.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "adaptLegacyCommand" {
			continue
		}
		for _, statement := range fn.Body.List {
			sw, ok := statement.(*ast.SwitchStmt)
			if !ok {
				continue
			}
			for _, statement := range sw.Body.List {
				clause := statement.(*ast.CaseClause)
				for _, expr := range clause.List {
					literal, ok := expr.(*ast.BasicLit)
					if !ok {
						continue
					}
					name, _ := strconv.Unquote(literal.Value)
					actual[name] = true
					op, describeErr := catalog.Describe("command." + strings.ToLower(name))
					if describeErr != nil {
						t.Fatal(name, describeErr)
					}
					fields := map[string]bool{}
					for _, field := range op.Fields {
						fields[field.Name] = true
					}
					ast.Inspect(clause, func(node ast.Node) bool {
						selector, ok := node.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						owner, ok := selector.X.(*ast.Ident)
						if !ok || owner.Name != "request" {
							return true
						}
						field, ok := reflect.TypeOf(workspace.CommandRequest{}).FieldByName(selector.Sel.Name)
						if !ok {
							return true
						}
						key := strings.Split(field.Tag.Get("json"), ",")[0]
						// Host metadata and ephemeral native locators are deliberately
						// excluded. CREATE_BODY assigns its ID from requestId.
						if key == "-" || key == "" || key == "type" || key == "requestId" || key == "topologyId" || name == "CREATE_BODY" && key == "bodyId" {
							return true
						}
						if !fields[key] {
							t.Errorf("%s input %s missing from AI contract", name, key)
						}
						return true
					})
				}
			}
		}
	}
	for command := range actual {
		op, err := catalog.Describe("command." + strings.ToLower(command))
		if err != nil || op.CommandType != command {
			t.Fatalf("public command missing from AI catalog: %s %v", command, err)
		}
	}
	if len(actual) != len(commandContracts) {
		t.Fatalf("stale or duplicate command contracts: handlers=%d contracts=%d", len(actual), len(commandContracts))
	}
	presentation, err := workbenchconfig.Read()
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range presentation.Commands {
		op, err := catalog.Describe("ui." + command.ID)
		if err != nil || op.UICommand != command.ID || op.Effect != "UI_COMMAND" {
			t.Fatalf("missing UI command %s: %v", command.ID, err)
		}
	}
	for _, op := range catalog.ordered {
		raw, _ := json.Marshal(op)
		if len(raw) > 12<<10 {
			t.Fatalf("operation %s needs progressive type disclosure (%d bytes)", op.ID, len(raw))
		}
	}
	page, err := catalog.SearchPage("", "PART", 3, "")
	if err != nil || !page.HasMore || len(page.Items) != 3 {
		t.Fatal(page, err)
	}
	seen := map[string]bool{}
	for {
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate discovery page", item.ID)
			}
			seen[item.ID] = true
		}
		if !page.HasMore {
			break
		}
		page, err = catalog.SearchPage("", "PART", 3, page.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != page.MatchedCount {
		t.Fatal("not all commands discoverable", len(seen), page.MatchedCount)
	}
	if _, err = catalog.SearchPage("fillet", "PART", 3, NewCatalog().Digest+":fake"); err == nil {
		t.Fatal("cursor scope not checked")
	}
	t.Logf("Covered %d public domain command contracts and %d registered workbench commands", len(actual), len(presentation.Commands))
}
func TestAutomationNestedCommandContractsValidateRealDTOs(t *testing.T) {
	catalog := NewCatalog()
	valid := `{"sketchId":"sketch-stable","operations":[{"type":"ADD_ENTITY","entity":{"id":"circle-stable","kind":"CIRCLE","role":"PROFILE","center":{"x":0,"y":0},"radius":5}}]}`
	command, err := catalog.Compile("command.edit_sketch", 1, []byte(valid))
	if err != nil || command.Type != "EDIT_SKETCH" || command.Operations[0].Entity.Radius != 5 {
		t.Fatal(command, err)
	}
	for _, raw := range []string{
		`{"sketchId":"s","operations":[{"type":"ADD_ENTITY","entity":{"kind":"CIRCLE","radius":2,"radius":99}}]}`,
		`{"sketchId":"s","operations":[{"type":"ADD_ENTITY","actorId":"forged"}]}`,
		`{"origin":[0,0],"direction":[0,0,1]}`,
		`{"origin":[0,0,0],"direction":[0,0,1,2]}`,
	} {
		id := "command.edit_sketch"
		if strings.Contains(raw, "origin") {
			id = "command.create_datum_axis"
		}
		if _, err = catalog.Compile(id, 1, []byte(raw)); err == nil {
			t.Fatal("invalid nested contract accepted", raw)
		}
	}
	for _, name := range []string{"workspace.Feature", "workspace.SketchOperation", "workspace.SketchEntity", "workspace.AssemblyGeometryRef"} {
		schema, err := DescribeType(name)
		raw, _ := json.Marshal(schema)
		if err != nil || len(raw) > 12<<10 {
			t.Fatal(name, len(raw), err)
		}
	}
	if _, err = catalog.Compile("ui.part.fillet", 1, []byte(`{}`)); err == nil {
		t.Fatal("UI command entered domain pipeline")
	}
	pattern, err := catalog.Compile("command.create_pattern", 1, []byte(`{"feature":{"type":"SOLID_PATTERN","bodyId":"body","pattern":{"kind":"LINEAR","distribution":"FIXED_STEP","count":2,"spacing":20,"id":"pattern","origin":[0,0,0],"direction":[1,0,0],"source":{"bodyId":"body","featureId":"source"},"sourceKind":"BODY_STAGE"}}}`))
	if err != nil || pattern.Feature.Pattern.Kind != "LINEAR" {
		t.Fatal("embedded JSON fields must match the native DTO", pattern, err)
	}
	schema, err := DescribeType("workspace.FeaturePattern")
	if err != nil {
		t.Fatal(err)
	}
	fields := schema["properties"].(map[string]any)
	if fields["PatternDefinition"] != nil || fields["kind"] == nil {
		t.Fatal("embedded type was not flattened", schema)
	}
	dimension, err := DescribeType("modelcore.Dimension")
	if err != nil {
		t.Fatal(err)
	}
	if dimension["properties"].(map[string]any)["Length"].(map[string]any)["type"] != "integer" {
		t.Fatal("quantity exponents must be integer fields", dimension)
	}
	graph, err := DescribeType("modelcore.Quantity")
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := json.Marshal(modelcore.Quantity{SIValue: 0.012, Dimension: modelcore.LengthDimension})
	var values map[string]any
	_ = json.Unmarshal(actual, &values)
	if err = validateTyped(reflect.TypeOf(modelcore.Quantity{}), values, 0); err != nil {
		t.Fatal("DTO schema rejects its own canonical quantity types", graph, err)
	}
}

func TestAutomationCurrentPartCommandInputs(t *testing.T) {
	catalog := NewCatalog()
	cases := []struct {
		id, raw string
		check   func(workspace.CommandRequest) bool
	}{
		{"command.move_feature", `{"targetId":"hole","anchorFeatureId":"fillet","placement":"BEFORE","expectedFeatureDigest":"definition-proof"}`, func(r workspace.CommandRequest) bool {
			return r.TargetID == "hole" && r.AnchorFeatureID == "fillet" && r.Placement == "BEFORE" && r.ExpectedFeatureDigest == "definition-proof"
		}},
		{"command.reassign_sketch_support", `{"sketchId":"location","datumPlaneId":"datum-xz"}`, func(r workspace.CommandRequest) bool { return r.SketchID == "location" && r.DatumPlaneID == "datum-xz" }},
		{"command.create_modify_feature", `{"afterFeatureId":"base","versionId":"revision","geometryKey":"stage","feature":{"type":"FILLET","bodyId":"body","length":1}}`, func(r workspace.CommandRequest) bool {
			return r.AfterFeatureID == "base" && r.VersionID == "revision" && r.GeometryKey == "stage" && r.Feature.Type == "FILLET"
		}},
		{"command.create_boolean_feature", `{"bodyId":"body","operation":"REMOVE","tools":[{"bodyId":"tool","featureId":"stage"}],"booleanResult":{"mode":"EXPLICIT","regions":[{"boundaryUnique":true,"boundary":["proof"],"inputDigest":"input","cellDigest":"cell"}]}}`, func(r workspace.CommandRequest) bool {
			return r.BooleanResult != nil && r.BooleanResult.Mode == "EXPLICIT" && len(r.BooleanResult.Regions) == 1 && r.BooleanResult.Regions[0].CellDigest == "cell"
		}},
		{"command.create_solid_feature", `{"bodyId":"body","sketchId":"sketch","generator":"LINEAR_EXTRUDE","operation":"ADD","extent":"TO_PLANE","terminationPlaneId":"datum","profileSelection":{"mode":"REGIONS","regionIds":["region"]}}`, func(r workspace.CommandRequest) bool {
			return r.Extent == "TO_PLANE" && r.TerminationPlaneID == "datum" && r.ProfileSelection.Mode == "REGIONS" && reflect.DeepEqual(r.ProfileSelection.RegionIDs, []string{"region"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			r, e := catalog.Compile(tc.id, 1, json.RawMessage(tc.raw))
			if e != nil || !tc.check(r) {
				t.Fatal("current command fields lost", r, e)
			}
		})
	}
	for _, raw := range []string{`{"targetId":"hole","anchorFeatureId":"fillet","placement":"SIDEWAYS","expectedFeatureDigest":"proof"}`, `{"targetId":"hole","anchorFeatureId":"fillet","placement":"AFTER","expectedFeatureDigest":"proof","requestId":"forged"}`} {
		if _, e := catalog.Compile("command.move_feature", 1, json.RawMessage(raw)); e == nil {
			t.Fatal("invalid move contract accepted", raw)
		}
	}
	if _, e := catalog.Compile("command.reassign_sketch_support", 1, json.RawMessage(`{"sketchId":"sketch","targetKind":"FACE"}`)); e == nil || !strings.Contains(e.Error(), "UI_HANDOFF_REQUIRED") {
		t.Fatal("face support bypassed the authorized stage picker", e)
	}
	for _, name := range []string{"workspace.Feature", "workspace.CommandRequest"} {
		schema, e := DescribeType(name)
		if e != nil {
			t.Fatal(e)
		}
		properties := schema["properties"].(map[string]any)
		values := properties["extent"].(map[string]any)["enum"].([]string)
		if !slices.Contains(values, "TO_PLANE") {
			t.Fatal("stale extent schema", name, values)
		}
		if name == "workspace.Feature" {
			if !slices.Contains(values, "BLIND") || !slices.Contains(properties["type"].(map[string]any)["enum"].([]string), "HOLE") {
				t.Fatal("Hole schema missing")
			}
			if !reflect.DeepEqual(properties["holeKind"].(map[string]any)["enum"], []string{"SIMPLE", "COUNTERBORE", "COUNTERSINK"}) {
				t.Fatal("stale Hole kinds")
			}
		}
	}
}
