package devtool

import (
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"io"
	"os"

	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func (a *App) runtimeEnv(o Options) error {
	os.Setenv("OCCCCAD_DEV_PROFILE", o.Profile)
	if e := os.Setenv("OCCCCAD_BUILD_TYPE", buildType(o.Profile)); e != nil {
		return e
	}
	_, e := config.LoadEnvironment()
	return e
}
func goArgs(o Options) ([]string, error) {
	if o.Packages == "" {
		return nil, fmt.Errorf("--packages is required")
	}
	if o.Match != "" {
		if _, e := regexp.Compile(o.Match); e != nil {
			return nil, e
		}
	}
	args := []string{"go", "test", "-json", "-p", strconv.Itoa(min(o.Jobs, o.TestJobs)), "-parallel", strconv.Itoa(o.TestJobs), "-timeout=15m"}
	if o.Level == "integration" {
		if o.TestJobs != 1 {
			return nil, fmt.Errorf("shared integration tests require --test-jobs 1")
		}
		args = append(args, "-tags=integration", "-count=1")
	}
	if o.Match != "" {
		args = append(args, "-run", o.Match)
	}
	for _, p := range strings.Split(o.Packages, ",") {
		if !regexp.MustCompile(`^\./[a-zA-Z0-9_./-]+$`).MatchString(p) {
			return nil, fmt.Errorf("invalid Go package %q", p)
		}
		args = append(args, p)
	}
	return args, nil
}
func (a *App) GoTest(o Options) error { return a.goTestAt(o, a.services()) }
func goCompileArgs(o Options, output string) []string {
	args := []string{"go", "test", "-c", "-mod=readonly", "-p", strconv.Itoa(o.Jobs), "-o", output + string(filepath.Separator)}
	if o.Level == "integration" {
		args = append(args, "-tags=integration")
	}
	return append(args, strings.Split(o.Packages, ",")...)
}
func (a *App) goTestAt(o Options, cwd string) error {
	release, e := lock(a.Context, filepath.Join(a.Root, "build/locks/codegen.lock"), true)
	if e != nil {
		return e
	}
	defer release()
	args, e := goArgs(o)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Join(a.Root, "build"), 0700); e != nil {
		return e
	}
	temp, e := os.MkdirTemp(filepath.Join(a.Root, "build"), "go-tests-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	prep := goCompileArgs(o, temp)
	if e = a.nativeEnv(cwd, offlineGo(), o.Verbose, prep...); e != nil {
		return fmt.Errorf("Go test preparation failed; setup --component go if modules are missing: %w", e)
	}
	if o.Level == "integration" {
		hold, e := a.buildLock(o, true)
		if e != nil {
			return e
		}
		defer hold()
		if e = a.runtimeEnv(o); e != nil {
			return e
		}
	}
	a.recordNative(cwd, args)
	log := a.logPath("go-test")
	fmt.Fprintf(a.Err, "[run] Go tests; log: %s\n", log)
	var live io.Writer
	if os.Getenv("OCCCCAD_DEV_EVIDENCE_STREAM") == "1" {
		live = a.Out
	} else if o.Verbose {
		live = a.Err
	}
	out, err := a.stream(cwd, offlineGo(), log, live, nil, args...)
	verdict := GoEvidenceFile(log, err != nil, o.Match != "")
	fmt.Fprintf(a.Err, "[%s] Go tests passed=%d skipped=%d; log: %s\n", verdict.Status, verdict.Counts.Passed, verdict.Counts.Skipped, log)
	if err != nil {
		if live == nil {
			fmt.Fprintln(a.Err, compact(out))
		}
		return err
	}
	if verdict.Status == "ENVIRONMENT_BLOCKED" {
		return blocked("Go tests skipped required evidence; log: " + log)
	}
	if verdict.Status != "PASS" {
		return fmt.Errorf("%s; log: %s", verdict.Reason, log)
	}
	return nil
}
func (a *App) CPPTest(o Options) error {
	if !oneOf(o.Scope, "assembly", "geometry", "sketch") {
		return fmt.Errorf("cpp:test requires assembly, geometry or sketch scope")
	}
	hold, e := a.buildLock(o, true)
	if e != nil {
		return e
	}
	defer hold()
	names, e := a.ctestCases(o)
	if e != nil {
		return e
	}
	if len(names) == 0 {
		return fmt.Errorf("selected zero CTest cases")
	}
	filter := exactCTestFilter(names)
	args := []string{"ctest", "--test-dir", a.buildDir(o), "--no-tests=error", "--output-on-failure", "--parallel", strconv.Itoa(o.TestJobs), "-R", filter}
	report := o.Report
	if report == "" {
		report = a.logPath("ctest") + ".xml"
	}
	if e = os.MkdirAll(filepath.Dir(report), 0700); e != nil {
		return e
	}
	args = append(args, "--output-junit", report)
	if e := a.runtimeEnv(o); e != nil {
		return e
	}
	if e := os.MkdirAll(os.Getenv("OCCCCAD_TEST_RESOURCES_DIR"), 0700); e != nil {
		return e
	}
	if os.Getenv("OCCCCAD_DEV_EVIDENCE_STREAM") == "1" {
		e = a.raw(a.Root, args...)
	} else {
		e = a.native(a.Root, o.Verbose, args...)
	}
	verdict := CTestEvidence(report, e != nil)
	expectedEvidence(Step{Kind: "ctest", Expected: names}, &verdict, o.Match)
	if e != nil {
		return e
	}
	if verdict.Status == "ENVIRONMENT_BLOCKED" {
		return blocked("required CTest evidence skipped; report: " + report)
	}
	if verdict.Status != "PASS" {
		return fmt.Errorf("CTest %s: %s; report: %s", verdict.Status, verdict.Reason, report)
	}
	return nil
}
func (a *App) WebTest(o Options) error {
	args := []string{"pnpm", "test", "--", "--verbose"}
	if o.Match != "" {
		args = append(args, o.Match)
	}
	if o.Level == "browser-mock" || o.Level == "browser-live" {
		args = []string{"pnpm", "exec", "playwright", "test", "--workers", strconv.Itoa(o.TestJobs)}
		if o.Level == "browser-live" {
			if e := a.runtimeEnv(o); e != nil {
				return e
			}
			args = append(args, "--config", "playwright.live.config.ts")
		}
	}
	if os.Getenv("OCCCCAD_DEV_EVIDENCE_STREAM") == "1" {
		return a.raw(a.web(), args...)
	}
	return a.native(a.web(), o.Verbose, args...)
}
func (a *App) Codegen(o Options) error {
	unlock, e := lock(a.Context, filepath.Join(a.Root, "build/locks/codegen.lock"), false)
	if e != nil {
		return e
	}
	defer unlock()
	if e = os.MkdirAll(filepath.Join(a.Root, "build"), 0700); e != nil {
		return e
	}
	temp, e := os.MkdirTemp(filepath.Join(a.Root, "build"), "codegen-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	files := []string{}
	e = filepath.WalkDir(filepath.Join(a.Root, "proto"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".proto") {
			files = append(files, p)
		}
		return nil
	})
	if e != nil {
		return e
	}
	args := []string{"protoc", "--proto_path=" + filepath.Join(a.Root, "proto"), "--go_out=" + temp, "--go_opt=module=github.com/occccad/occccad/gen", "--go-grpc_out=" + temp, "--go-grpc_opt=module=github.com/occccad/occccad/gen"}
	args = append(args, files...)
	if e = a.native(a.Root, o.Verbose, args...); e != nil {
		return e
	}
	expected := map[string]string{}
	actual := map[string]string{}
	walk := func(root string, m map[string]string) error {
		return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				if p == root && os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".pb.go") {
				r, _ := filepath.Rel(root, p)
				m[r] = fileHash(p)
			}
			return nil
		})
	}
	if e = walk(temp, expected); e != nil {
		return e
	}
	if e = walk(filepath.Join(a.services(), "gen"), actual); e != nil {
		return e
	}
	if o.Check {
		if hashJSON(actual) != hashJSON(expected) {
			return fmt.Errorf("generated Go drift (added/deleted/content changed); run mise run codegen")
		}
		fmt.Fprintln(a.Out, "Go generated sources match pinned generators")
		return nil
	}
	for p := range actual {
		if _, ok := expected[p]; !ok {
			if e = os.Remove(filepath.Join(a.services(), "gen", p)); e != nil {
				return e
			}
		}
	}
	for p := range expected {
		b, e := os.ReadFile(filepath.Join(temp, p))
		if e != nil {
			return e
		}
		dest := filepath.Join(a.services(), "gen", p)
		if e = os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
			return e
		}
		if e = os.WriteFile(dest, b, 0644); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) GoConformance(o Options) error { return a.goTestAt(o, filepath.Join(a.Root, "tests/go")) }
