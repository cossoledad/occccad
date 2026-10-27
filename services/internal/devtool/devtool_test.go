package devtool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "services"), 0755)
	os.MkdirAll(filepath.Join(root, "web/apps/cad"), 0755)
	return &App{Root: root, Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, Context: context.Background()}
}
func put(t *testing.T, root, p, s string) {
	t.Helper()
	p = filepath.Join(root, p)
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), 0644); e != nil {
		t.Fatal(e)
	}
}
func gitFixture(t *testing.T, a *App, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-c", "user.name=Fixture", "-c", "user.email=fixture@invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	c.Dir = a.Root
	out, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, out)
	}
	return strings.TrimSpace(string(out))
}
func initGit(t *testing.T, a *App) {
	gitFixture(t, a, "init", "-q")
	put(t, a.Root, "services/internal/workspace/a.go", "package workspace\n")
	gitFixture(t, a, "add", ".")
	gitFixture(t, a, "commit", "-qm", "base")
}
func TestChangedPathsPreserveRenameUntrackedDeletedNewlines(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	put(t, a.Root, "web/name with\nnewline.ts", "export {};")
	gitFixture(t, a, "mv", "services/internal/workspace/a.go", "old renamed.go")
	paths, e := a.Changed(Options{})
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"services/internal/workspace/a.go", "old renamed.go", "web/name with\nnewline.ts"} {
		if !oneOf(p, paths...) {
			t.Fatalf("missing %q: %q", p, paths)
		}
	}
	staged, e := a.Changed(Options{Staged: true})
	if e != nil || oneOf("web/name with\nnewline.ts", staged...) {
		t.Fatal(staged, e)
	}
	gitFixture(t, a, "commit", "-qam", "renamed")
	os.Remove(filepath.Join(a.Root, "old renamed.go"))
	paths, e = a.Changed(Options{})
	if e != nil || !oneOf("old renamed.go", paths...) {
		t.Fatal(paths, e)
	}
}
func TestCommitRangeAndMissingHistory(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	base := gitFixture(t, a, "rev-parse", "HEAD")
	put(t, a.Root, "services/internal/workspace/a.go", "package workspace\n// committed\n")
	gitFixture(t, a, "commit", "-qam", "change")
	put(t, a.Root, "web/untracked.ts", "x")
	paths, e := a.Changed(Options{Base: base, Head: "HEAD"})
	if e != nil || len(paths) != 1 {
		t.Fatal(paths, e)
	}
	paths, e = a.Changed(Options{Base: base})
	if e != nil || len(paths) != 2 {
		t.Fatal(paths, e)
	}
	if _, e = a.Changed(Options{Base: "missing"}); e == nil {
		t.Fatal("missing history accepted")
	}
	if _, e = a.Changed(Options{Head: "HEAD"}); e == nil {
		t.Fatal("head without base")
	}
}
func TestSourceIncludesUntrackedDeletionAndExcludesOutputs(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	before, _ := a.Source()
	put(t, a.Root, "new.go", "package p")
	after, _ := a.Source()
	if hashJSON(before) == hashJSON(after) {
		t.Fatal("untracked missing")
	}
	put(t, a.Root, "build/new.log", "output")
	gitFixture(t, a, "add", "build/new.log")
	latest, _ := a.Source()
	if before["sha256"] == after["sha256"] || latest["sha256"] != after["sha256"] {
		t.Fatal("report self-invalidated identity")
	}
	os.Remove(filepath.Join(a.Root, "services/internal/workspace/a.go"))
	latest, _ = a.Source()
	if latest["sha256"] == after["sha256"] {
		t.Fatal("deletion missing")
	}
}
func TestPositiveBudgetsAndIllegalCLI(t *testing.T) {
	for _, s := range []string{"0", "-1", "x", "", "1.5"} {
		if _, e := positive(s); e == nil {
			t.Fatal("accepted", s)
		}
	}
	for _, args := range [][]string{{"build", "--jobs", "0"}, {"build", "--jobs"}, {"build", "--profile", "wrong"}, {"build", "--component", "bad"}, {"plan", "--level", "x"}, {"clean", "--yes"}, {"dev", "--wat"}, {"build", "junk"}, {"data:reset"}} {
		if e := fixture(t).Run(args); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	t.Setenv("OCCCCAD_BUILD_JOBS", "bad")
	if e := fixture(t).Run([]string{"build", "--component", "tooling"}); e == nil {
		t.Fatal("bad env budget")
	}
	if e := fixture(t).Run([]string{"setup", "--component", "tooling", "--jobs", "2"}); e != nil {
		t.Fatal("explicit should override env:", e)
	}
}
func TestGoBudgetCacheAndMatchPropagation(t *testing.T) {
	o := Options{Jobs: 8, TestJobs: 1, Packages: "./internal/control", Level: "integration", Match: "^TestReal$"}
	argv, e := goArgs(o)
	if e != nil {
		t.Fatal(e)
	}
	s := strings.Join(argv, " ")
	for _, part := range []string{"-p 1", "-parallel 1", "-tags=integration", "-count=1", "-run ^TestReal$"} {
		if !strings.Contains(s, part) {
			t.Fatal(argv, part)
		}
	}
	prep := strings.Join(goCompileArgs(o, "/owned output"), " ")
	if !strings.Contains(prep, "-p 8") || !strings.Contains(prep, "-tags=integration") {
		t.Fatal(prep)
	}
	o.Level = "unit"
	argv, _ = goArgs(o)
	if oneOf("-count=1", argv...) || oneOf("-tags=integration", argv...) {
		t.Fatal("unit cache disabled / integration included")
	}
	o.Packages = "--bad"
	if _, e = goArgs(o); e == nil {
		t.Fatal("package injection")
	}
}
func events(v ...GoEvent) string {
	lines := []string{}
	for _, e := range v {
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}
	return strings.Join(lines, "\n")
}
func TestGoEvidenceZeroInterruptedSkippedAndCached(t *testing.T) {
	if GoEvidence("", false, false).Status != "FAIL" {
		t.Fatal("zero match")
	}
	run := GoEvent{Action: "run", Test: "TestReal"}
	pass := GoEvent{Action: "pass", Test: "TestReal"}
	if GoEvidence(events(run), false, false).Status != "FAIL" {
		t.Fatal("missing terminal")
	}
	skip := GoEvent{Action: "skip", Test: "TestReal/child"}
	if GoEvidence(events(run, pass, GoEvent{Action: "run", Test: skip.Test}, skip), false, false).Status != "ENVIRONMENT_BLOCKED" {
		t.Fatal("parent hid skip")
	}
	raw := events(run, GoEvent{Action: "output", Test: run.Test, Output: "NOT_APPLICABLE: selected backend"}, GoEvent{Action: "skip", Test: run.Test})
	if GoEvidence(raw, false, true).Status != "ENVIRONMENT_BLOCKED" {
		t.Fatal("required selection skipped")
	}
	raw = events(run, pass, GoEvent{Action: "output", Output: "ok fixture (cached)"})
	if r := GoEvidence(raw, false, false); r.Status != "PASS" || !r.Cached {
		t.Fatal(r)
	}
	if GoEvidence(raw, true, false).Status != "FAIL" {
		t.Fatal("process failure hidden")
	}
}
func TestCTestEvidenceRequiresFreshActualCases(t *testing.T) {
	a := fixture(t)
	p := filepath.Join(a.Root, "report.xml")
	if CTestEvidence(p, false).Status != "FAIL" {
		t.Fatal("missing xml")
	}
	for _, c := range []struct{ body, status string }{{"<testsuite/>", "FAIL"}, {"<testsuite><testcase name=\"native\"><skipped/></testcase></testsuite>", "ENVIRONMENT_BLOCKED"}, {"<testsuite><testcase name=\"native\"/></testsuite>", "PASS"}, {"<testsuite><testcase name=\"native\"><failure>bad</failure></testcase></testsuite>", "FAIL"}, {"broken", "FAIL"}} {
		os.WriteFile(p, []byte(c.body), 0600)
		if r := CTestEvidence(p, false); r.Status != c.status {
			t.Fatal(c, r)
		}
	}
}
func TestWebZeroMatchAndProcessFailure(t *testing.T) {
	for _, s := range []string{"", "PASS", "web scenarios PASS (0/20; x)"} {
		if WebEvidence(s, false).Status != "FAIL" {
			t.Fatal(s)
		}
	}
	s := "web scenarios PASS (3/20; x)"
	if r := WebEvidence(s, false); r.Status != "PASS" || r.Counts.Passed != 3 {
		t.Fatal(r)
	}
	if WebEvidence(s, true).Status != "FAIL" {
		t.Fatal("process failure")
	}
}
func TestRedactionAndBoundedUTF8(t *testing.T) {
	t.Setenv("OCCCCAD_ADMIN_PASSWORD", "private-demo")
	s := redact("postgres://actor:secret@example.test/db token=abc private-demo")
	for _, secret := range []string{"secret", "abc", "private-demo"} {
		if strings.Contains(s, secret) {
			t.Fatal(s)
		}
	}
	s = compact("fatal start " + strings.Repeat("数据", 20000) + " failure end")
	if len(s) > 16384 || !strings.HasPrefix(s, "fatal start") || !strings.HasSuffix(s, "failure end") {
		t.Fatal("bad bounded output")
	}
}
func TestCleanRejectsSymlinkAndResourceOverlap(t *testing.T) {
	a := fixture(t)
	dest := filepath.Join(a.Root, "build/cmake/debug")
	for _, p := range []string{a.Root, "/", filepath.Dir(a.Root)} {
		if safeClean(a.Root, p, nil) == nil {
			t.Fatal(p)
		}
	}
	if safeClean(a.Root, dest, []string{dest + "/data.db"}) == nil {
		t.Fatal("overlap")
	}
	os.MkdirAll(filepath.Dir(dest), 0755)
	os.Symlink(t.TempDir(), dest)
	if safeClean(a.Root, dest, nil) == nil {
		t.Fatal("symlink")
	}
	os.Remove(dest)
	if e := safeClean(a.Root, dest, []string{filepath.Join(a.Root, "services/data")}); e != nil {
		t.Fatal(e)
	}
}
func TestBuildLockCancellationAndOneWriter(t *testing.T) {
	a := fixture(t)
	p := filepath.Join(a.Root, "build/locks/test.lock")
	release, e := lock(a.Context, p, false)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if unlock, e := lock(ctx, p, false); e == nil {
		unlock()
		t.Fatal("concurrent writer")
	}
	release()
	r, e := lock(a.Context, p, false)
	if e != nil {
		t.Fatal("cancel leaked lock", e)
	}
	r()
	r, e = tryLock(p)
	if e != nil {
		t.Fatal(e)
	}
	defer r()
	if r, e := tryLock(p); e == nil {
		r()
		t.Fatal("duplicate runtime accepted")
	}
}
func TestDiagnosticTraversalFractionalIDsAndNoNumericalReplay(t *testing.T) {
	a := fixture(t)
	logs := filepath.Join(a.Root, "logs")
	put(t, logs, "debug/cad-diagnostics/document/2026.10.27.001.json", `{"schema":"occccad.operation-diagnostic.v1","input":{"immutable":true}}`)
	p, e := resolveDiagnostic(logs, "[CAD_DIAGNOSTIC document/2026.10.27.001]")
	if e != nil || !strings.HasSuffix(p, "2026.10.27.001.json") {
		t.Fatal(p, e)
	}
	for _, s := range []string{"../escape", "document/..", "a/b/c"} {
		if _, e = resolveDiagnostic(logs, s); e == nil {
			t.Fatal("escape", s)
		}
	}
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(logs, "debug/cad-diagnostics/symlink"))
	put(t, outside, "x.json", "{}")
	if _, e = resolveDiagnostic(logs, "symlink/x"); e == nil {
		t.Fatal("symlink escape")
	}
	d, h, e := readDiagnostic(p)
	if e != nil || len(h) != 64 || d["schema"] == nil {
		t.Fatal(d, h, e)
	}
}
func TestProfileJobsDoNotEnterSemanticIdentity(t *testing.T) {
	a := fixture(t)
	if a.buildDir(Options{Profile: "debug", Jobs: 1}) != a.buildDir(Options{Profile: "debug", Jobs: 8}) {
		t.Fatal("jobs changed directory")
	}
	if a.buildDir(Options{Profile: "debug"}) == a.buildDir(Options{Profile: "release"}) {
		t.Fatal("profile collision")
	}
	if buildType("sanitize") != "Debug" {
		t.Fatal("sanitize config")
	}
}
