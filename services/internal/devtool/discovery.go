package devtool

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ctestCase struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

func (a *App) ctestInventory(o Options) ([]ctestCase, error) {
	root := a.buildDir(o)
	if _, e := os.Stat(filepath.Join(root, "CTestTestfile.cmake")); e != nil {
		return nil, blocked("CTest discovery requires a configured/built " + o.Profile + " profile; prepare the smallest cpp:build target")
	}
	hold, e := a.buildLock(o, true)
	if e != nil {
		return nil, e
	}
	defer hold()
	// Even ctest --show-only writes Testing/Temporary/LastTest.log. Read its
	// generated definitions in a disposable tree, leaving the real build intact.
	query, e := os.MkdirTemp("", "occccad-ctest-query-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(query)
	e = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name != "CTestTestfile.cmake" && name != "CTestCustom.cmake" && !strings.HasSuffix(name, "_include.cmake") && !strings.HasSuffix(name, "_tests.cmake") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		dest := filepath.Join(query, rel)
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0600)
	})
	if e != nil {
		return nil, e
	}
	out, e := a.capture(a.Root, nil, "ctest", "--test-dir", query, "--show-only=json-v1")
	if e != nil {
		return nil, blocked(fmt.Sprintf("CTest discovery failed: %v; check mise install --locked and the smallest cpp:build target for %s: %s", e, o.Profile, compact(redact(out))))
	}
	var data struct {
		Tests []ctestCase `json:"tests"`
	}
	if e = json.Unmarshal([]byte(out), &data); e != nil {
		return nil, fmt.Errorf("invalid CTest inventory: %w", e)
	}
	var r Registry
	if e = loadJSON(filepath.Join(a.Root, "dev/validation.json"), &r); e != nil {
		return nil, e
	}
	for _, c := range data.Tests {
		if strings.HasSuffix(c.Name, "_NOT_BUILT") {
			target := strings.TrimSuffix(c.Name, "_NOT_BUILT")
			if len(ctestTargetScopes(r, target)) == 0 {
				return nil, fmt.Errorf("CTest placeholder has no validation build registration: %s", target)
			}
			continue // A GoogleTest discovery placeholder is not an executed case.
		}
		owners := 0
		for scope, prefixes := range r.CTestScopes {
			registered := false
			for _, step := range r.Steps {
				if step.Task == "cpp:test" && oneOf(scope, step.Scopes...) {
					registered = true
				}
			}
			if !registered {
				continue
			}
			for _, p := range prefixes {
				if strings.HasPrefix(c.Name, p) {
					owners++
				}
			}
		}
		if owners != 1 {
			return nil, fmt.Errorf("CTest case %q has %d validation owners; update dev/validation.json ctestScopes", c.Name, owners)
		}
	}
	return data.Tests, nil
}
func (a *App) ctestCases(o Options) ([]string, error) {
	var r Registry
	if e := loadJSON(filepath.Join(a.Root, "dev/validation.json"), &r); e != nil {
		return nil, e
	}
	prefixes := r.CTestScopes[o.Scope]
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no CTest ownership for scope %s", o.Scope)
	}
	filter, e := regexp.Compile(o.Match)
	if e != nil {
		return nil, e
	}
	all, e := a.ctestInventory(o)
	if e != nil {
		return nil, e
	}
	names := []string{}
	for _, c := range all {
		if strings.HasSuffix(c.Name, "_NOT_BUILT") {
			target := strings.TrimSuffix(c.Name, "_NOT_BUILT")
			if oneOf(o.Scope, ctestTargetScopes(r, target)...) {
				return nil, blocked("CTest cases not discovered yet; run mise run cpp:build --target " + target + " --profile " + o.Profile)
			}
			continue
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(c.Name, prefix) && (o.Match == "" || filter.MatchString(c.Name) || filter.MatchString(strings.TrimPrefix(c.Name, prefix))) {
				names = append(names, c.Name)
				break
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

func ctestTargetScopes(r Registry, target string) []string {
	scopes := []string{}
	for _, step := range r.Steps {
		if step.Task == "cpp:build" && oneOf("unit", step.Levels...) && oneOf(target, step.Targets...) {
			scopes = appendUnique(scopes, step.Scopes...)
		}
	}
	return scopes
}
func exactCTestFilter(names []string) string {
	escaped := []string{}
	for _, n := range names {
		escaped = append(escaped, regexp.QuoteMeta(n))
	}
	return "^(" + strings.Join(escaped, "|") + ")$"
}

// Go -run separates regex components at unescaped top-level slashes. Discovery
// lists top-level tests without compiling or running resource-owning TestMain.
func splitRunPattern(pattern string, delimiter rune) []string {
	parts := []string{}
	start, brackets, parens := 0, 0, 0
	escape := false
	for i, c := range pattern {
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		switch c {
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case '(':
			if brackets == 0 {
				parens++
			}
		case ')':
			if brackets == 0 && parens > 0 {
				parens--
			}
		case delimiter:
			if brackets == 0 && parens == 0 {
				parts = append(parts, pattern[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, pattern[start:])
}
func goRunParts(pattern string) []string { return splitRunPattern(pattern, '/') }
func goRunAlternatives(pattern string) [][]string {
	result := [][]string{}
	for _, p := range splitRunPattern(pattern, '|') {
		result = append(result, goRunParts(p))
	}
	return result
}
func goRootMatches(pattern, name string) (bool, error) {
	for _, parts := range goRunAlternatives(pattern) {
		r, e := regexp.Compile(parts[0])
		if e != nil {
			return false, e
		}
		if r.MatchString(name) {
			return true, nil
		}
	}
	return false, nil
}
func goRunMatches(pattern, name string) bool {
	names := strings.Split(name, "/")
	for _, parts := range goRunAlternatives(pattern) {
		if len(names) < len(parts) {
			continue
		}
		matched := true
		for i, p := range parts {
			r, e := regexp.Compile(p)
			if e != nil || !r.MatchString(names[i]) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func (a *App) goCases(step *Step, o Options) ([]string, error) {
	cwd := a.services()
	if step.Task == "go:conformance" {
		cwd = filepath.Join(a.Root, "tests/go")
	}
	packages := argument(step.Argv, "--packages")
	args := []string{"go", "list", "-mod=readonly", "-json"}
	if argument(step.Argv, "--level") == "integration" {
		args = append(args, "-tags=integration")
	}
	args = append(args, strings.Split(packages, ",")...)
	out, e := a.capture(cwd, offlineGo(), args...)
	if e != nil {
		return nil, blocked(fmt.Sprintf("Go discovery failed: %v; check mise install --locked and setup --component go: %s", e, compact(redact(out))))
	}
	decoder := json.NewDecoder(strings.NewReader(out))
	names := []string{}
	selectedPackages := []string{}
	for {
		var p struct {
			Dir, ImportPath           string
			TestGoFiles, XTestGoFiles []string
		}
		e = decoder.Decode(&p)
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		for _, file := range append(p.TestGoFiles, p.XTestGoFiles...) {
			tree, e := parser.ParseFile(token.NewFileSet(), filepath.Join(p.Dir, file), nil, parser.ParseComments)
			if e != nil {
				return nil, e
			}
			for _, d := range tree.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				name := fn.Name.Name
				if name == "TestMain" || !(strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Example")) {
					continue
				}
				matched, err := goRootMatches(o.Match, name)
				if err != nil {
					return nil, err
				}
				if matched {
					names = appendUnique(names, p.ImportPath+"/"+name)
					rel, err := filepath.Rel(cwd, p.Dir)
					if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
						return nil, fmt.Errorf("discovered package is outside module: %s", p.Dir)
					}
					selectedPackages = appendUnique(selectedPackages, "./"+filepath.ToSlash(rel))
				}
			}
		}
	}
	sort.Strings(names)
	for i, arg := range step.Argv {
		if arg == "--packages" && i+1 < len(step.Argv) {
			step.Argv[i+1] = strings.Join(selectedPackages, ",")
		}
	}
	return names, nil
}
func (a *App) webCases(match string) ([]string, error) {
	names := []string{}
	err := filepath.WalkDir(filepath.Join(a.web(), "src"), func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && strings.HasSuffix(p, ".scenario.mjs") && strings.Contains(strings.ToLower(p), strings.ToLower(match)) {
			rel, e := filepath.Rel(a.web(), p)
			if e != nil {
				return e
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}
func argument(args []string, key string) string {
	for i, v := range args {
		if v == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func (a *App) resolveMatches(p *Plan, o Options) error {
	if o.Match == "" {
		return nil
	}
	steps := []Step{}
	matched := 0
	for _, step := range p.Steps {
		var names []string
		var e error
		switch step.Kind {
		case "go":
			names, e = a.goCases(&step, o)
		case "ctest":
			local := o
			local.Scope = argument(step.Argv, "--scope")
			names, e = a.ctestCases(local)
		case "web":
			names, e = a.webCases(o.Match)
		default:
			steps = append(steps, step)
			continue
		}
		if e != nil {
			return e
		}
		if len(names) == 0 {
			p.Reasons = appendUnique(p.Reasons, "no matching cases in "+step.ID+"; leaf omitted")
			continue
		}
		step.Expected = names
		matched += len(names)
		steps = append(steps, step)
	}
	if matched == 0 {
		return fmt.Errorf("selected zero tests for --scope %s --level %s --match %q", o.Scope, o.Level, o.Match)
	}
	p.Steps = steps
	return nil
}
func expectedEvidence(step Step, e *Evidence, match string) {
	actual := map[string]bool{}
	for _, c := range e.Cases {
		key := c.Name
		if step.Kind == "go" {
			key = c.Package + "/" + strings.Split(c.Name, "/")[0]
		}
		actual[key] = true
	}
	for _, n := range step.Expected {
		if !actual[n] {
			e.Status = "FAIL"
			e.Reason = "planned case disappeared or was not executed: " + n
			return
		}
	}
	if step.Kind == "go" && match != "" {
		for _, expected := range step.Expected {
			found := false
			for _, c := range e.Cases {
				if c.Package+"/"+strings.Split(c.Name, "/")[0] == expected && goRunMatches(match, c.Name) && c.Status != "NOT_RUN" {
					found = true
					break
				}
			}
			if !found {
				e.Status = "FAIL"
				e.Reason = "selected zero matching Go tests/subtests for " + expected
				return
			}
		}
	}
}
