package devtool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Rule struct {
	Prefix   string   `json:"prefix"`
	Suffix   string   `json:"suffix"`
	Scopes   []string `json:"scopes"`
	Escalate bool     `json:"escalate"`
	Reason   string   `json:"reason"`
}
type Registration struct {
	ID, Task, Kind                               string
	Scopes, Levels, Resources, Targets, Packages []string
}
type Registry struct {
	Rules []Rule
	Steps []Registration
}
type Step struct {
	ID        string   `json:"id"`
	Task      string   `json:"task"`
	Argv      []string `json:"argv"`
	Cwd       string   `json:"cwd"`
	Kind      string   `json:"kind"`
	Resources []string `json:"resources,omitempty"`
}
type Plan struct {
	Schema      string   `json:"schema"`
	Paths       []string `json:"paths"`
	Scopes      []string `json:"scopes"`
	Level       string   `json:"level"`
	Reasons     []string `json:"reasons"`
	Outstanding []string `json:"outstandingEvidence"`
	Steps       []Step   `json:"steps"`
	Source      any      `json:"source"`
	Comparison  string   `json:"comparison"`
}

func (a *App) git(args ...string) (string, error) {
	return a.capture(a.Root, nil, append([]string{"git"}, args...)...)
}
func (a *App) Changed(o Options) ([]string, error) {
	if o.Head != "" && o.Base == "" {
		return nil, fmt.Errorf("--head requires --base")
	}
	if o.Staged && (o.Base != "" || o.Head != "") {
		return nil, fmt.Errorf("--staged cannot combine with --base/--head")
	}
	paths := map[string]bool{}
	diff := func(args ...string) error {
		out, e := a.git(append([]string{"diff", "--name-status", "-z", "--find-renames"}, args...)...)
		if e != nil {
			return fmt.Errorf("git diff failed: %s: %w", out, e)
		}
		fields := strings.Split(out, "\x00")
		for i := 0; i < len(fields) && fields[i] != ""; {
			status := fields[i]
			i++
			count := 1
			if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
				count = 2
			}
			for j := 0; j < count; j++ {
				if i >= len(fields) {
					return fmt.Errorf("invalid NUL-delimited git diff")
				}
				paths[fields[i]] = true
				i++
			}
		}
		return nil
	}
	if o.Base != "" {
		head := o.Head
		if head == "" {
			head = "HEAD"
		}
		baseHash, e := a.git("rev-parse", "--verify", "--end-of-options", o.Base+"^{commit}")
		if e != nil {
			return nil, fmt.Errorf("missing local base reference %q", o.Base)
		}
		headHash, e := a.git("rev-parse", "--verify", "--end-of-options", head+"^{commit}")
		if e != nil {
			return nil, fmt.Errorf("missing local head reference %q", head)
		}
		base, e := a.git("merge-base", strings.TrimSpace(baseHash), strings.TrimSpace(headHash))
		if e != nil {
			return nil, fmt.Errorf("missing local reference/history for merge-base %q %q: %w", o.Base, head, e)
		}
		if e = diff(strings.TrimSpace(base), strings.TrimSpace(headHash)); e != nil {
			return nil, e
		}
	}
	if o.Head == "" {
		var e error
		if o.Staged {
			e = diff("--cached", "HEAD")
		} else {
			e = diff("HEAD")
		}
		if e != nil {
			return nil, e
		}
		if !o.Staged {
			out, e := a.git("ls-files", "--others", "--exclude-standard", "-z")
			if e != nil {
				return nil, e
			}
			for _, p := range strings.Split(out, "\x00") {
				if p != "" {
					paths[p] = true
				}
			}
		}
	}
	result := []string{}
	for p := range paths {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}
func (a *App) Source() (map[string]any, error) {
	out, e := a.git("ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if e != nil {
		return nil, e
	}
	inputs := map[string]string{}
	for _, p := range strings.Split(out, "\x00") {
		if !sourceInput(p) {
			continue
		}
		info, e := os.Lstat(filepath.Join(a.Root, p))
		if os.IsNotExist(e) {
			inputs[p] = "DELETED"
			continue
		}
		if e != nil {
			return nil, e
		}
		if info.Mode().IsRegular() {
			inputs[p] = fileHash(filepath.Join(a.Root, p))
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(filepath.Join(a.Root, p))
			if e != nil {
				return nil, e
			}
			inputs[p] = "symlink:" + target
		}
	}
	head, e := a.git("rev-parse", "HEAD")
	if e != nil {
		return nil, e
	}
	dirty, e := a.git("status", "--porcelain=v1", "-z")
	if e != nil {
		return nil, e
	}
	return map[string]any{"sha256": hashJSON(inputs), "files": len(inputs), "commit": strings.TrimSpace(head), "dirty": len(sourceStatus(dirty)) > 0, "worktree": sourceStatus(dirty), "boundary": "tracked and untracked owned inputs; excludes generated Go, build outputs, logs and secret configuration"}, nil
}
func (a *App) MakePlan(o Options) (Plan, error) {
	p := Plan{Schema: "occccad.validation-plan.v1", Level: o.Level, Paths: []string{}, Scopes: []string{}, Reasons: []string{}, Outstanding: []string{}, Steps: []Step{}, Comparison: "worktree: staged + unstaged + untracked; rename includes both paths"}
	if o.Head != "" {
		p.Comparison = "merge-base(base, head)..head; worktree excluded"
	} else if o.Base != "" {
		p.Comparison = "merge-base(base, HEAD)..HEAD plus staged/unstaged/untracked"
	}
	var r Registry
	if e := loadJSON(filepath.Join(a.Root, "dev/validation.json"), &r); e != nil {
		return p, e
	}
	scopes := map[string]bool{}
	if o.Scope != "" {
		if !oneOf(o.Scope, "assembly", "geometry", "sketch", "workspace", "services", "web", "tooling", "docs", "all") {
			return p, fmt.Errorf("invalid scope %q", o.Scope)
		}
		scopes[o.Scope] = true
	}
	if o.Changed || o.Base != "" || o.Head != "" || o.Staged || o.Scope == "" {
		paths, e := a.Changed(o)
		if e != nil {
			return p, e
		}
		p.Paths = paths
		for _, path := range paths {
			found := false
			for _, rule := range r.Rules {
				if strings.HasPrefix(path, rule.Prefix) && strings.HasSuffix(path, rule.Suffix) {
					found = true
					if o.Scope == "" {
						for _, s := range rule.Scopes {
							scopes[s] = true
						}
					}
					if rule.Escalate {
						p.Outstanding = appendUnique(p.Outstanding, "integration", "browser-live")
						p.Reasons = appendUnique(p.Reasons, rule.Reason)
					}
					break
				}
			}
			if !found {
				if o.Scope == "" {
					scopes["all"] = true
				}
				p.Reasons = appendUnique(p.Reasons, "unknown ownership: "+path)
				p.Outstanding = appendUnique(p.Outstanding, "integration", "browser-live")
			}
		}
	}
	if scopes["all"] {
		scopes = map[string]bool{"assembly": true, "geometry": true, "sketch": true, "workspace": true, "services": true, "web": true, "tooling": true, "docs": true}
	}
	for s := range scopes {
		p.Scopes = append(p.Scopes, s)
	}
	sort.Strings(p.Scopes)
	if o.Match != "" {
		if o.Scope == "" || len(p.Scopes) != 1 || oneOf(o.Scope, "all", "docs", "tooling") || o.Level == "full" {
			return p, fmt.Errorf("--match requires one explicit filterable scope and a specific level")
		}
		if oneOf(o.Level, "browser-mock", "browser-live") {
			return p, fmt.Errorf("--match unsupported for browser levels; use native Playwright filters")
		}
	}
	if o.Level != "unit" && o.TestJobs != 1 {
		return p, fmt.Errorf("integration/browser/full require --test-jobs 1 for shared application resources")
	}
	if o.Level == "unit" && (scopes["assembly"] || scopes["geometry"] || scopes["sketch"] || scopes["workspace"] || scopes["services"]) {
		p.Outstanding = appendUnique(p.Outstanding, "integration")
	}
	for _, reg := range r.Steps {
		selected := false
		for _, s := range reg.Scopes {
			if scopes[s] {
				selected = true
			}
		}
		if !selected {
			continue
		}
		if o.Level != "full" && !oneOf(o.Level, reg.Levels...) {
			continue
		}
		args := []string{"mise", "run", "--quiet", reg.Task}
		switch reg.Task {
		case "cpp:build":
			for _, target := range reg.Targets {
				sa := append(append([]string{}, args...), "--target", target, "--profile", o.Profile, "--jobs", fmt.Sprint(o.Jobs))
				p.Steps = append(p.Steps, Step{ID: reg.ID + "-" + target, Task: reg.Task, Kind: "build", Cwd: a.Root, Argv: sa})
			}
			continue
		case "cpp:test":
			args = append(args, "--scope", reg.Scopes[0], "--profile", o.Profile, "--test-jobs", fmt.Sprint(o.TestJobs))
			if o.Match != "" {
				args = append(args, "--match", o.Match)
			}
		case "go:check", "go:conformance":
			args = append(args, "--profile", o.Profile, "--packages", strings.Join(reg.Packages, ","), "--level", reg.Levels[0], "--jobs", fmt.Sprint(o.Jobs), "--test-jobs", fmt.Sprint(o.TestJobs))
			if o.Match != "" {
				args = append(args, "--match", o.Match)
			}
		case "codegen":
			args = append(args, "--check", "--jobs", fmt.Sprint(o.Jobs))
		case "web:build":
			args = append(args, "--jobs", fmt.Sprint(o.Jobs))
		case "web:check":
			args = append(args, "--level", reg.Levels[0], "--test-jobs", fmt.Sprint(o.TestJobs))
			if o.Match != "" {
				args = append(args, "--match", o.Match)
			}
		}
		p.Steps = append(p.Steps, Step{ID: reg.ID, Task: reg.Task, Kind: reg.Kind, Resources: reg.Resources, Cwd: a.Root, Argv: args})
	}
	if len(p.Scopes) > 0 && len(p.Steps) == 0 {
		return p, fmt.Errorf("no validation registered for scopes %v at level %s", p.Scopes, o.Level)
	}
	sort.SliceStable(p.Steps, func(i, j int) bool { return p.Steps[i].Kind == "build" && p.Steps[j].Kind != "build" })
	source, e := a.Source()
	p.Source = source
	return p, e
}
func appendUnique(s []string, values ...string) []string {
	for _, v := range values {
		if !oneOf(v, s...) {
			s = append(s, v)
		}
	}
	return s
}

func (a *App) Artifacts(o Options) map[string]string {
	paths := []string{}
	_ = filepath.WalkDir(filepath.Join(a.services(), "gen"), func(p string, d os.DirEntry, e error) error {
		if e == nil && !d.IsDir() && strings.HasSuffix(p, ".pb.go") {
			paths = append(paths, p)
		}
		return nil
	})
	for _, p := range []string{"workers/geometry/occccad_geometry_worker", "workers/geometry/occccad_naming_artifact_tests", "kernel/occt/tests/occcad_geometry_scenarios", "kernel/assembly/tests/occcad_assembly_solver_scenarios", "kernel/assembly/tests/occcad_assembly_solver_corpus", "workers/geometry/sketch/tests/occcad_sketch_solver_scenarios"} {
		paths = append(paths, filepath.Join(a.buildDir(o), p))
	}
	if p := os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"); p != "" {
		paths = append(paths, p)
	}
	result := map[string]string{}
	for _, p := range paths {
		result[p] = fileHash(p)
	}
	return result
}

func sourceInput(p string) bool {
	return p != "" && !strings.HasPrefix(p, "build/") && !strings.HasPrefix(p, "services/gen/") && !strings.Contains(p, "/node_modules/") && !strings.Contains(p, "/dist/") && !oneOf(p, ".env", ".env.local", "CMakeUserPresets.json", "compile_commands.json") && !strings.HasSuffix(p, ".log") && !strings.HasSuffix(p, ".local") && !strings.Contains(p, "/.env.")
}
func sourceStatus(raw string) []string {
	parts := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	result := []string{}
	for i := 0; i < len(parts); i++ {
		row := parts[i]
		if len(row) < 3 {
			continue
		}
		p := row[3:]
		rename := strings.ContainsAny(row[:2], "RC")
		old := ""
		if rename && i+1 < len(parts) {
			i++
			old = parts[i]
		}
		if sourceInput(p) || (old != "" && sourceInput(old)) {
			result = append(result, row)
			if rename {
				result = append(result, old)
			}
		}
	}
	return result
}
