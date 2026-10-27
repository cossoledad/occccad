package devtool

import (
	"fmt"
	"github.com/occccad/occccad/internal/config"
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
	a.recordNative(cwd, prep)
	out, e := a.capture(cwd, offlineGo(), prep...)
	if e != nil {
		fmt.Fprint(a.Err, redact(out))
		return fmt.Errorf("Go test preparation failed; run mise run setup --component go if modules are missing: %w", e)
	}
	if o.Level == "integration" {
		if e = a.runtimeEnv(o); e != nil {
			return e
		}
	}
	a.recordNative(cwd, args)
	out, e = a.capture(cwd, offlineGo(), args...)
	// Parse machine verdicts before redacting diagnostic text. In particular,
	// a password that also occurs in a package name must not alter its identity.
	verdict := GoEvidence(out, e != nil, o.Match != "")
	fmt.Fprint(a.Out, redactGoOutput(out))
	if e != nil {
		return e
	}
	if verdict.Counts.Started == 0 {
		return fmt.Errorf("selected zero Go tests")
	}
	return nil
}
func (a *App) CPPTest(o Options) error {
	if !oneOf(o.Scope, "assembly", "geometry", "sketch") {
		return fmt.Errorf("cpp:test requires assembly, geometry or sketch scope")
	}
	filter := "^" + o.Scope + "/"
	if o.Scope == "assembly" {
		filter = "^(assembly|assembly-corpus)/"
	}
	if o.Match != "" {
		filter += ".*(" + o.Match + ")"
	}
	args := []string{"ctest", "--test-dir", a.buildDir(o), "--no-tests=error", "--output-on-failure", "--parallel", strconv.Itoa(o.TestJobs), "-R", filter}
	if o.Report != "" {
		args = append(args, "--output-junit", o.Report)
	}
	if e := a.runtimeEnv(o); e != nil {
		return e
	}
	if e := os.MkdirAll(os.Getenv("OCCCCAD_TEST_RESOURCES_DIR"), 0700); e != nil {
		return e
	}
	return a.raw(a.Root, args...)
}
func (a *App) WebTest(o Options) error {
	args := []string{"pnpm", "test", "--"}
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
	return a.raw(a.web(), args...)
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
