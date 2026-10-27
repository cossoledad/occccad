package devtool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	p, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func registryFixture(t *testing.T, a *App) {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(repoRoot(t), "dev/validation.json"))
	if e != nil {
		t.Fatal(e)
	}
	put(t, a.Root, "dev/validation.json", string(b))
}
func TestRoutingLocalSharedUnknownAndDocsActuallyExecute(t *testing.T) {
	for _, c := range []struct {
		path, scope string
		expanded    bool
	}{{"docs/new.md", "docs", false}, {"kernel/assembly/src/x.cpp", "assembly", false}, {"kernel/occt/src/x.cpp", "geometry", false}, {"workers/geometry/sketch/src/x.cpp", "sketch", false}, {"services/internal/workspace/x.go", "workspace", false}, {"web/apps/cad/src/x.ts", "web", false}, {"proto/new.proto", "", true}, {"services/internal/database/postgres_migrations/new.sql", "", true}, {"unknown name\nfile", "", true}} {
		t.Run(c.path, func(t *testing.T) {
			a := fixture(t)
			initGit(t, a)
			registryFixture(t, a)
			gitFixture(t, a, "add", ".")
			gitFixture(t, a, "commit", "-qm", "registry")
			put(t, a.Root, c.path, "new")
			p, e := a.MakePlan(Options{Changed: true, Level: "unit", Profile: "release", Jobs: 4, TestJobs: 1})
			if e != nil {
				t.Fatal(e)
			}
			if c.expanded {
				if len(p.Scopes) != 8 || len(p.Outstanding) == 0 {
					t.Fatal(p)
				}
			} else if len(p.Scopes) != 1 || p.Scopes[0] != c.scope {
				t.Fatal(p.Scopes)
			}
			if c.scope == "docs" && (len(p.Steps) != 1 || p.Steps[0].Task != "docs:check") {
				t.Fatal("docs-only did not select audit", p.Steps)
			}
		})
	}
}
func TestPlanReadOnlyExplicitProfileBudgetMatchAndOutstanding(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	registryFixture(t, a)
	t.Setenv("OCCCCAD_ENV_FILE", filepath.Join(a.Root, "nonexistent.env"))
	o := Options{Scope: "assembly", Level: "unit", Match: "DirectedAngle", Profile: "debug", Jobs: 7, TestJobs: 1}
	p, e := a.MakePlan(o)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(a.Root, "build")); !os.IsNotExist(e) {
		t.Fatal("plan wrote build output")
	}
	if !oneOf("integration", p.Outstanding...) {
		t.Fatal("risk evidence hidden")
	}
	for _, s := range p.Steps {
		joined := strings.Join(s.Argv, " ")
		if !strings.Contains(joined, "--profile debug") {
			t.Fatal(s)
		}
		if s.Kind == "build" && !strings.Contains(joined, "--jobs 7") {
			t.Fatal(s)
		}
		if s.Kind == "ctest" && !strings.Contains(joined, "--match DirectedAngle") {
			t.Fatal(s)
		}
	}
	o.Scope = "workspace"
	o.Level = "integration"
	p, e = a.MakePlan(o)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range p.Steps {
		if s.Task == "go:check" {
			joined := strings.Join(s.Argv, " ")
			for _, want := range []string{"--jobs 7", "--profile debug", "--test-jobs 1", "--match DirectedAngle"} {
				if !strings.Contains(joined, want) {
					t.Fatal(joined, want)
				}
			}
		}
	}
	o.TestJobs = 2
	if _, e = a.MakePlan(o); e == nil {
		t.Fatal("parallel shared database accepted")
	}
	o.TestJobs = 1
	o.Scope = ""
	if _, e = a.MakePlan(o); e == nil {
		t.Fatal("implicit match scope accepted")
	}
}
func TestRegistryNeverRecursesIntoCheck(t *testing.T) {
	var registry Registry
	if e := loadJSON(filepath.Join(repoRoot(t), "dev/validation.json"), &registry); e != nil {
		t.Fatal(e)
	}
	for _, s := range registry.Steps {
		if oneOf(s.Task, "check", "plan", "setup", "dev") {
			t.Fatal("recursive or resource-starting leaf", s)
		}
		p := filepath.Join(repoRoot(t), "dev/tasks", strings.ReplaceAll(s.Task, ":", "/"))
		if info, e := os.Stat(p); e != nil || info.IsDir() {
			if _, e = os.Stat(filepath.Join(p, "_default")); e != nil {
				t.Fatal("missing task", s.Task)
			}
		}
	}
}
func fakeExecutable(t *testing.T, a *App, name, script string) {
	t.Helper()
	dir := filepath.Join(a.Root, "fake tools")
	put(t, dir, name, "#!/bin/sh\n"+script)
	os.Chmod(filepath.Join(dir, name), 0755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
func TestGoAndWebOnlyDoNotTouchCPP(t *testing.T) {
	a := fixture(t)
	fakeExecutable(t, a, "go", "printf 'native Go build\\n'\n")
	fakeExecutable(t, a, "pnpm", "printf 'native Web build\\n'\n")
	for _, component := range []string{"go", "web"} {
		if e := a.Build(Options{Component: component, Profile: "release", Jobs: 7}); e != nil {
			t.Fatal(e)
		}
	}
	out := a.Err.(*bytes.Buffer).String()
	if strings.Contains(out, "conan") || strings.Contains(out, "cmake") {
		t.Fatal(out)
	}
	if !strings.Contains(out, `"-p","7"`) {
		t.Fatal("Go budget absent", out)
	}
}
func TestBenchmarkFailuresAndZeroSamplesNeverBuildNative(t *testing.T) {
	for _, script := range []string{"printf 'compiler failed\\n'; exit 7\n", "printf 'PASS\\n'\n"} {
		a := fixture(t)
		fakeExecutable(t, a, "go", script)
		e := a.Benchmark(Options{Samples: 2, Jobs: 4, Profile: "release"}, true)
		if e == nil {
			t.Fatal("unmeasured benchmark accepted")
		}
		b, _ := os.ReadFile(filepath.Join(a.Root, "build/performance/go-workspace.txt"))
		if !strings.Contains(string(b), "compiler failed") && !strings.Contains(e.Error(), "missing measured samples") {
			t.Fatal(e, string(b))
		}
		if strings.Contains(a.Err.(*bytes.Buffer).String(), "cmake") {
			t.Fatal("built native after failed Go evidence")
		}
	}
}
func TestConanManifestRequiresPresentFilesDespiteSuccessReceipt(t *testing.T) {
	a := fixture(t)
	p := filepath.Join(a.Root, "package")
	put(t, p, "conanmanifest.txt", "1234\nlib/libcad.a: abc\n")
	if verifyPackage(p) == nil {
		t.Fatal("missing cached file accepted")
	}
	put(t, p, "lib/libcad.a", "binary")
	if e := verifyPackage(p); e != nil {
		t.Fatal(e)
	}
	os.Remove(filepath.Join(p, "lib/libcad.a"))
	if verifyPackage(p) == nil {
		t.Fatal("stale manifest accepted")
	}
}
func TestRecipeLockProfileCompilerAndGeneratedOutputsInvalidate(t *testing.T) {
	a := fixture(t)
	root := repoRoot(t)
	for _, name := range []string{"conanfile.py", "conan.lock", "build-support/conan/profiles/linux-gcc15-release", "build-support/conan/profiles/linux-gcc15-debug"} {
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		put(t, a.Root, name, string(b))
	}
	for _, name := range []string{"conan", "cmake", "ninja"} {
		fakeExecutable(t, a, name, "printf 'fixed tool version\\n'\n")
	}
	fakeExecutable(t, a, "g++-15", "case \"$1\" in -dumpfullversion) printf '15.2.0\\n';; -dumpmachine) printf 'x86_64-linux-gnu\\n';; esac\n")
	o := Options{Profile: "release", Jobs: 1}
	id, e := a.depIdentity(o)
	if e != nil {
		t.Fatal(e)
	}
	pkg := filepath.Join(a.Root, "cache")
	put(t, pkg, "conanmanifest.txt", "1234\ninclude/a.h: abcd\n")
	put(t, pkg, "include/a.h", "header")
	output := a.toolchain(o)
	put(t, a.Root, strings.TrimPrefix(output, a.Root+"/"), "generated")
	receipt := depsReceipt{Schema: dependenciesSchema, Identity: id, Packages: []string{pkg}, Outputs: map[string]string{output: fileHash(output)}}
	if e = writeJSON(filepath.Join(a.buildDir(o), "dependencies.json"), receipt); e != nil {
		t.Fatal(e)
	}
	if e = a.verifyDeps(o); e != nil {
		t.Fatal(e)
	}
	o.Jobs = 8
	t.Setenv("OCCCCAD_DATABASE_URL", "sqlite:/unrelated/runtime.db")
	if latest, _ := a.depIdentity(o); latest != id {
		t.Fatal("jobs/runtime entered compilation identity")
	}
	for _, name := range []string{"conanfile.py", "conan.lock", "build-support/conan/profiles/linux-gcc15-release"} {
		p := filepath.Join(a.Root, name)
		b, _ := os.ReadFile(p)
		os.WriteFile(p, append(b, '\n', '#', 'x'), 0644)
		if a.verifyDeps(o) == nil {
			t.Fatal("stale receipt accepted", name)
		}
		os.WriteFile(p, b, 0644)
	}
	os.Remove(output)
	if a.verifyDeps(o) == nil {
		t.Fatal("missing generated output accepted")
	}
	os.WriteFile(output, []byte("generated"), 0644)
	os.Remove(filepath.Join(pkg, "include/a.h"))
	if a.verifyDeps(o) == nil {
		t.Fatal("missing cached header accepted")
	}
	fakeExecutable(t, a, "g++-15", "printf '14.1.0\\n'\n")
	if _, e = a.depIdentity(o); e == nil {
		t.Fatal("compiler/profile mismatch accepted")
	}
}
func TestProfileCannotLieAboutCompiler(t *testing.T) {
	a := fixture(t)
	p := filepath.Join(a.Root, "profile")
	put(t, a.Root, "profile", "[settings]\nos=Linux\narch=x86_64\ncompiler=gcc\ncompiler.version=14\ncompiler.cppstd=17\ncompiler.libcxx=libstdc++11\nbuild_type=Release\n")
	if validateProfile(p, "Release") == nil {
		t.Fatal("fake compiler accepted")
	}
}
func TestSourceChangesDuringValidationCannotReportPass(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	r := Registry{Steps: []Registration{{ID: "fixture", Task: "go:check", Kind: "go", Scopes: []string{"services"}, Levels: []string{"unit"}, Packages: []string{"./fixture"}}}}
	writeJSON(filepath.Join(a.Root, "dev/validation.json"), r)
	fakeExecutable(t, a, "mise", "printf 'package fixture\\n' > "+shellJoin([]string{filepath.Join(a.Root, "changed.go")})+"\nprintf '%s\\n' '{\"Action\":\"run\",\"Test\":\"TestFixture\"}' '{\"Action\":\"pass\",\"Test\":\"TestFixture\"}'\n")
	e := a.Validate(Options{Scope: "services", Level: "unit", Profile: "release", Jobs: 4, TestJobs: 1}, false)
	if e == nil || !strings.Contains(e.Error(), "SOURCE_CHANGED") {
		t.Fatal(e)
	}
	paths, _ := filepath.Glob(filepath.Join(a.Root, "build/agent-check/*/report.json"))
	var report Report
	if len(paths) != 1 || loadJSON(paths[0], &report) != nil || report.Status != "SOURCE_CHANGED" {
		t.Fatal(paths, report.Status)
	}
}
func TestCleanSelectionIsNotTestSuccess(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	writeJSON(filepath.Join(a.Root, "dev/validation.json"), Registry{})
	gitFixture(t, a, "add", ".")
	gitFixture(t, a, "commit", "-qm", "registry")
	if e := a.Validate(Options{Changed: true, Level: "unit", Profile: "release", Jobs: 4, TestJobs: 1, JSON: true}, false); e != nil {
		t.Fatal(e)
	}
	var report Report
	if e := json.Unmarshal(a.Out.(*bytes.Buffer).Bytes(), &report); e != nil || report.Status != "NOT_RUN" {
		t.Fatal(report, e)
	}
}
func TestLoadedRuntimeRequiresSameInstanceResourcesAndBinaries(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	o := Options{Profile: "release"}
	t.Setenv("OCCCCAD_DATABASE_URL", "sqlite:/owned/app.db")
	source, _ := a.Source()
	artifacts := map[string]string{}
	proc := filepath.Join(a.Root, "build/proc")
	paths := []string{}
	for i, name := range []string{"occccad-control", "occccad-server", "occccad-jobs", "occccad_geometry_worker"} {
		p := filepath.Join(a.Root, "build/bin", name)
		put(t, a.Root, "build/bin/"+name, "v1")
		artifacts[p] = fileHash(p)
		paths = append(paths, p)
		pid := fmt.Sprint(i + 1)
		put(t, proc, pid+"/environ", "OCCCCAD_RUNTIME_ID=instance-1\x00")
		os.Symlink(p, filepath.Join(proc, pid, "exe"))
	}
	writeJSON(filepath.Join(a.Root, "build/runtime/identity.json"), map[string]any{"source": source, "artifacts": artifacts, "runtimeId": "instance-1", "profile": "release", "workspace": a.Root, "environment": environmentIdentity()})
	if e := a.runningIdentityAt(o, proc); e != nil {
		t.Fatal(e)
	}
	put(t, proc, "1/environ", "OCCCCAD_RUNTIME_ID=older-instance\x00")
	if a.runningIdentityAt(o, proc) == nil {
		t.Fatal("old process instance accepted")
	}
	put(t, proc, "1/environ", "OCCCCAD_RUNTIME_ID=instance-1\x00")
	t.Setenv("OCCCCAD_APP_LISTEN", "127.0.0.1:29000")
	if a.runningIdentityAt(o, proc) == nil {
		t.Fatal("changed listeners accepted")
	}
	os.Unsetenv("OCCCCAD_APP_LISTEN")
	os.WriteFile(paths[0], []byte("v2"), 0644)
	if a.runningIdentityAt(o, proc) == nil {
		t.Fatal("updated disk artifact accepted")
	}
	os.WriteFile(paths[0], []byte("v1"), 0644)
	os.WriteFile(paths[0]+" (deleted)", []byte("old inode"), 0644)
	os.Remove(filepath.Join(proc, "1/exe"))
	os.Symlink(paths[0]+" (deleted)", filepath.Join(proc, "1/exe"))
	if a.runningIdentityAt(o, proc) == nil {
		t.Fatal("old loaded inode accepted")
	}
}
func diagnosticEnvironment(t *testing.T, a *App) {
	t.Helper()
	put(t, a.Root, "fixture.env", "")
	t.Setenv("OCCCCAD_ENV_FILE", filepath.Join(a.Root, "fixture.env"))
	for k, v := range map[string]string{"OCCCCAD_DATA_DIR": filepath.Join(a.Root, "data"), "OCCCCAD_LOG_DIR": filepath.Join(a.Root, "logs"), "OCCCCAD_DATABASE_URL": "sqlite:" + filepath.Join(a.Root, "app.db"), "OCCCCAD_TEST_RESOURCES_DIR": filepath.Join(a.Root, "resources"), "OCCCCAD_GEOMETRY_WORKER_BIN": filepath.Join(a.Root, "worker")} {
		t.Setenv(k, v)
	}
}
func TestDiagnosticExportsImmutableRecordedAttemptAndRejectsTamper(t *testing.T) {
	a := fixture(t)
	diagnosticEnvironment(t, a)
	p := filepath.Join(a.Root, "input.json")
	writeJSON(p, map[string]any{"schema": "occccad.operation-diagnostic.v1", "documentId": "retained", "assemblyAttempts": []any{map[string]any{"schema": "occccad.3dreplay.v1", "request": map[string]any{"immutable": "recorded only"}}}})
	before := fileHash(p)
	if e := a.Diagnose(Options{File: p, Profile: "release"}); e != nil {
		t.Fatal(e)
	}
	var result map[string]any
	if e := json.Unmarshal(a.Out.(*bytes.Buffer).Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if result["replayStatus"] != "AVAILABLE" || fileHash(p) != before {
		t.Fatal(result)
	}
	exported := result["input"].(string)
	if b, _ := os.ReadFile(exported); !strings.Contains(string(b), "recorded only") {
		t.Fatal(string(b))
	}
	os.WriteFile(exported, []byte("{}"), 0644)
	if a.Diagnose(Options{File: p, Profile: "release"}) == nil {
		t.Fatal("changed immutable export accepted")
	}
}
func TestRetainedDomainInputDoesNotClaimNumericalReplayAndJobsOwnArchive(t *testing.T) {
	a := fixture(t)
	diagnosticEnvironment(t, a)
	p := filepath.Join(a.Root, "domain.json")
	writeJSON(p, map[string]any{"schema": "occccad.operation-diagnostic.v1", "input": map[string]any{"retained": true}})
	if e := a.Diagnose(Options{File: p, Profile: "release"}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(a.Out.(*bytes.Buffer).String(), "NO_RECORDED_NUMERICAL_ATTEMPT") {
		t.Fatal(a.Out)
	}
	if a.Diagnose(Options{File: p, Profile: "release", Replay: true}) == nil {
		t.Fatal("replayed unrecorded input")
	}
	logs := filepath.Join(a.Root, "logs")
	put(t, logs, "debug/job-diagnostics/job/1.2.3.json", `{"schema":"occccad.job-diagnostic.v1"}`)
	if p, e := resolveDiagnostic(logs, "CAD_JOB_DIAGNOSTIC job/1.2.3"); e != nil || !strings.Contains(p, "job-diagnostics") {
		t.Fatal(p, e)
	}
}

func TestDiagnosticExportPreservesDigestCoveredOrderAndNumericTokens(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		a := fixture(t)
		diagnosticEnvironment(t, a)
		replay := `{"schema":"occccad.3dreplay.v1","snapshot":{"z":9007199254740993,"a":1e-7,"ordered":{"z":2,"a":1}}}`
		content := replay
		if wrapped {
			content = `{"schema":"occccad.operation-diagnostic.v1","assemblyAttempts":[` + replay + `]}`
		}
		p := filepath.Join(a.Root, "retained.json")
		os.WriteFile(p, []byte(content), 0600)
		if e := a.Diagnose(Options{File: p, Profile: "release"}); e != nil {
			t.Fatal(e)
		}
		var result map[string]any
		if e := json.Unmarshal(a.Out.(*bytes.Buffer).Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		data, e := os.ReadFile(result["input"].(string))
		var compact bytes.Buffer
		if e != nil || json.Compact(&compact, data) != nil || compact.String() != replay {
			t.Fatal("changed digest-covered retained input", string(data), e)
		}
	}
}
func TestContextRoutesAndBoundedPackets(t *testing.T) {
	a := fixture(t)
	text := strings.Repeat("target "+strings.Repeat("数据", 1500)+"\n", 30)
	put(t, a.Root, "source.go", text)
	writeJSON(filepath.Join(a.Root, "dev/recipes.json"), map[string]Recipe{"property": {Goal: "fixture", Sources: [][2]string{{"source.go", "target"}}}})
	if e := a.TaskContext(Options{Recipe: "property", Read: true}); e != nil {
		t.Fatal(e)
	}
	var result map[string]any
	if json.Unmarshal(a.Out.(*bytes.Buffer).Bytes(), &result) != nil {
		t.Fatal(a.Out)
	}
	metrics := result["metrics"].(map[string]any)
	if metrics["returnedSourceBytes"].(float64) > 5000 || metrics["modelTokens"] != nil {
		t.Fatal(metrics)
	}
	if a.TaskContext(Options{Recipe: "typo"}) == nil {
		t.Fatal("unknown recipe accepted")
	}
	paths, _ := filepath.Glob(filepath.Join(a.Root, "build/agent-workflow/*.json"))
	if len(paths) != 1 {
		t.Fatal(paths)
	}
}
func TestNodeEvidenceNeedsActualCountAndNoSkips(t *testing.T) {
	if NodeEvidence("PASS", false).Status != "FAIL" {
		t.Fatal("zero count")
	}
	raw := "# tests 2\n# pass 1\n# fail 0\n# skipped 1\n"
	if NodeEvidence(raw, false).Status != "ENVIRONMENT_BLOCKED" {
		t.Fatal("hidden skip")
	}
	if NodeEvidence(strings.ReplaceAll(raw, "# skipped 1", "# skipped 0"), true).Status != "FAIL" {
		t.Fatal("hidden process failure")
	}
}

func TestRetireGeneratedReferencesPreservesConfiguration(t *testing.T) {
	a := fixture(t)
	put(t, a.Root, ".env", "keep configured resources")
	put(t, a.Root, "CMakeUserPresets.json", `{"version":4,"vendor":{"conan":{}},"include":["build/cmake/release/build/Release/generators/CMakePresets.json"]}`)
	os.Symlink(filepath.Join(a.Root, "build/cmake/release/compile_commands.json"), filepath.Join(a.Root, "compile_commands.json"))
	if e := a.retireGeneratedReferences(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Lstat(filepath.Join(a.Root, "compile_commands.json")); !os.IsNotExist(e) {
		t.Fatal("legacy link remained")
	}
	b, _ := os.ReadFile(filepath.Join(a.Root, ".env"))
	if string(b) != "keep configured resources" {
		t.Fatal("configuration overwritten")
	}
	put(t, a.Root, "CMakeUserPresets.json", `{"version":4,"vendor":{"conan":{}},"include":["custom.json"]}`)
	if e := a.retireGeneratedReferences(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(a.Root, "CMakeUserPresets.json")); e != nil {
		t.Fatal("user presets removed")
	}
}
func TestResetRequiresYesAndDetectsStandaloneWritersWithoutPorts(t *testing.T) {
	a := fixture(t)
	if a.Reset(Options{}) == nil {
		t.Fatal("reset without yes accepted")
	}
	proc := filepath.Join(a.Root, "build/proc")
	put(t, proc, "42/environ", "OCCCCAD_ENV_FILE=/selected/app.env\x00")
	os.Symlink("/tmp/go-build/occccad-jobs", filepath.Join(proc, "42/exe"))
	os.Symlink(a.services(), filepath.Join(proc, "42/cwd"))
	writers := configuredWriters(a.Root, proc, "/selected/app.env", "")
	if len(writers) != 1 {
		t.Fatal("standalone jobs not guarded", writers)
	}
	if writers = configuredWriters(t.TempDir(), proc, "/different/env", ""); len(writers) != 0 {
		t.Fatal("unrelated workspace included", writers)
	}
}

func TestCodegenUsesTemporaryOutputAndDetectsAddedDeletedChangedFiles(t *testing.T) {
	a := fixture(t)
	proto := `syntax="proto3";package fixture;option go_package="github.com/occccad/occccad/gen/fixture;fixture";message Ping{string name=1;}`
	put(t, a.Root, "proto/fixture/ping.proto", proto)
	if e := a.Codegen(Options{}); e != nil {
		t.Fatal("prepare pinned tools with mise install:", e)
	}
	generated := filepath.Join(a.services(), "gen/fixture/ping.pb.go")
	before := fileHash(generated)
	if before == "MISSING" {
		t.Fatal("generator produced no Go source")
	}
	if e := a.Codegen(Options{Check: true}); e != nil {
		t.Fatal(e)
	}
	put(t, a.Root, "proto/fixture/ping.proto", strings.Replace(proto, "string name=1", "string name=2", 1))
	if a.Codegen(Options{Check: true}) == nil {
		t.Fatal("content drift accepted")
	}
	if fileHash(generated) != before {
		t.Fatal("check rewrote workspace")
	}
	if e := a.Codegen(Options{}); e != nil {
		t.Fatal(e)
	}
	put(t, a.Root, "services/gen/fixture/unexpected.pb.go", "unexpected generated file")
	if a.Codegen(Options{Check: true}) == nil {
		t.Fatal("extra file accepted")
	}
	if e := a.Codegen(Options{}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(a.services(), "gen/fixture/unexpected.pb.go")); !os.IsNotExist(e) {
		t.Fatal("stale output not removed")
	}
	os.RemoveAll(filepath.Join(a.services(), "gen"))
	if a.Codegen(Options{Check: true}) == nil {
		t.Fatal("deleted outputs accepted")
	}
	if e := a.Codegen(Options{}); e != nil {
		t.Fatal("missing directory not repaired:", e)
	}
}
func TestEmptyOptionsNeverFallBackToWholeBuildOrScope(t *testing.T) {
	for _, args := range [][]string{{"cpp:build", "--target", ""}, {"cpp:build", "--target", "--help"}, {"check", "--scope", ""}, {"check", "--scope", "assembly", "--match", ""}, {"plan", "--base", ""}} {
		if e := fixture(t).Run(args); e == nil {
			t.Fatal("empty/option target silently accepted", args)
		}
	}
}
func TestDiagnosticBudgetAndSchemaDoNotTrustFileMetadata(t *testing.T) {
	a := fixture(t)
	p := filepath.Join(a.Root, "diagnostic.json")
	for _, content := range []string{"[]", "broken", strings.Repeat(" ", 8*1024*1024+1)} {
		os.WriteFile(p, []byte(content), 0600)
		if _, _, e := readDiagnostic(p); e == nil {
			t.Fatal("invalid/oversize diagnostic accepted")
		}
	}
	os.WriteFile(p, []byte(`{"schema":"fixture"}`), 0600)
	_, sha, e := readDiagnostic(p)
	if e != nil || sha != fileHash(p) {
		t.Fatal("digest is not the read input", sha, e)
	}
}
func TestMachineSourceIdentityNeverDependsOnDiagnosticRedaction(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	put(t, a.Root, "proto/occccad/owner.proto", "syntax=\"proto3\";")
	gitFixture(t, a, "add", ".")
	gitFixture(t, a, "commit", "-qm", "identity")
	before, e := a.Source()
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("OCCCCAD_ADMIN_PASSWORD", "occccad")
	after, e := a.Source()
	if e != nil || hashJSON(before) != hashJSON(after) {
		t.Fatal("credential redaction changed machine paths/source identity", before, after, e)
	}
	if !strings.Contains(redact("password=occccad"), "<redacted>") {
		t.Fatal("diagnostics were not redacted")
	}
}

func TestSnapshotReplayRequiresExactRecordedDiagnosticSelection(t *testing.T) {
	a := fixture(t)
	diagnosticEnvironment(t, a)
	p := filepath.Join(a.Root, "snapshot.json")
	put(t, a.Root, "snapshot.json", `{"schema":"occccad.3dreplay.v1","snapshot":{"attempts":[{"diagnosticId":"recorded/exact"}]}}`)
	if e := a.Diagnose(Options{File: p, Profile: "release", Replay: true}); e == nil || !strings.Contains(e.Error(), "--diagnostic") {
		t.Fatal("silently selected original attempt", e)
	}
	if e := a.Diagnose(Options{File: p, Profile: "release", Diagnostic: "absent"}); e == nil {
		t.Fatal("unrecorded diagnostic selected")
	}
	a.Out = &bytes.Buffer{}
	if e := a.Diagnose(Options{File: p, Profile: "release", Diagnostic: "recorded/exact"}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(a.Out.(*bytes.Buffer).String(), `"-diagnostic"`) || !strings.Contains(a.Out.(*bytes.Buffer).String(), `"AVAILABLE"`) {
		t.Fatal(a.Out)
	}
}

func TestGoLeafRejectsZeroMatchesAndRedactsOnlyDiagnosticFields(t *testing.T) {
	a := fixture(t)
	fakeExecutable(t, a, "go", "exit 0\n")
	if e := a.GoTest(Options{Packages: "./internal/fixture", Level: "unit", Jobs: 4, TestJobs: 1, Match: "Nothing"}); e == nil || !strings.Contains(e.Error(), "zero Go tests") {
		t.Fatal("native zero selection silently succeeded", e)
	}
	t.Setenv("OCCCCAD_ADMIN_PASSWORD", "pass")
	raw := events(GoEvent{Action: "run", Package: "fixture/pass", Test: "TestCase"}, GoEvent{Action: "output", Package: "fixture/pass", Test: "TestCase", Output: "secret pass postgres://actor:credential@host/db"}, GoEvent{Action: "pass", Package: "fixture/pass", Test: "TestCase"})
	out := redactGoOutput(raw)
	if r := GoEvidence(out, false, true); r.Status != "PASS" || r.Counts.Passed != 1 || r.Cases[0].Package != "fixture/pass" {
		t.Fatal("redaction corrupted machine verdict", out, r)
	}
	if strings.Contains(out, "secret pass") || strings.Contains(out, "actor:credential") {
		t.Fatal("diagnostic leaked", out)
	}
}
