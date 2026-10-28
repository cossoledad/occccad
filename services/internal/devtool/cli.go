// Package devtool contains checkout-specific development operations. Native
// build graphs belong to CMake, Go and pnpm; process ownership belongs to Control.
package devtool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/occccad/occccad/internal/config"
)

type Options struct {
	Component, Profile, Scope, Level, Match, Base, Head, Target                         string
	Packages, Report, Reference, File, Document, Job, Recipe, Symbol, Diagnostic        string
	Purpose, Capability, Family, Layer, Case, Adapters, Stages, Label, Service          string
	Jobs, TestJobs, Samples, Attempt                                                    int
	Changed, Staged, JSON, Verbose, NoWeb, Yes, Replay, Browser, Live, Read, Check, Off bool
}

type App struct {
	Root     string
	Out, Err io.Writer
	Context  context.Context
}

func Main(args []string) error {
	root, err := config.ProjectRoot()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	jsonRequested := false
	for _, arg := range args {
		if arg == "--json" || arg == "--json=true" {
			jsonRequested = true
		}
		if arg == "--json=false" {
			jsonRequested = false
		}
	}
	if !jsonRequested {
		return (&App{Root: root, Out: os.Stdout, Err: os.Stderr, Context: ctx}).Run(args)
	}
	var output bytes.Buffer
	app := &App{Root: root, Out: &output, Err: os.Stderr, Context: ctx}
	err = app.Run(args)
	if json.Valid(bytes.TrimSpace(output.Bytes())) {
		_, writeErr := os.Stdout.Write(output.Bytes())
		if writeErr != nil {
			return writeErr
		}
		return err
	}
	status := "NOT_RUN"
	detail := "no execution result"
	if err != nil {
		status = "FAIL"
		detail = redact(err.Error())
		var environment *Blocked
		if errors.As(err, &environment) {
			status = "ENVIRONMENT_BLOCKED"
		}
	}
	writeErr := json.NewEncoder(os.Stdout).Encode(map[string]any{"schema": "occccad.command-result.v1", "status": status, "reason": detail})
	if writeErr != nil {
		return writeErr
	}
	return err
}
func positive(s string) (int, error) {
	if s == "" || s[0] < '1' || s[0] > '9' {
		return 0, fmt.Errorf("jobs must be a positive decimal integer: %q", s)
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("jobs must be a positive decimal integer: %q", s)
		}
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 {
		return 0, fmt.Errorf("jobs must be a positive integer: %q", s)
	}
	return n, nil
}
func (a *App) Run(args []string) error {
	if len(args) == 0 {
		return errors.New("use mise tasks ls; prepare with mise run setup --component tooling")
	}
	cmd := args[0]
	o := Options{Component: "all", Profile: "release", Level: "unit", Jobs: 4, TestJobs: 1, Samples: 5, Purpose: "validate", Adapters: "cpp,go,go-flags,web,web-catalog,integration", Stages: "static,interaction"}
	f := flag.NewFlagSet(cmd, flag.ContinueOnError)
	f.SetOutput(a.Err)
	add := func(names string) {
		for _, n := range strings.Fields(names) {
			switch n {
			case "component":
				f.StringVar(&o.Component, n, o.Component, "cpp, go, web, all or tooling")
			case "profile":
				f.StringVar(&o.Profile, n, o.Profile, "release, debug or sanitize")
			case "scope":
				f.StringVar(&o.Scope, n, "", "validation scope")
			case "level":
				f.StringVar(&o.Level, n, o.Level, "unit, integration, browser-mock, browser-live or full")
			case "match":
				f.StringVar(&o.Match, n, "", "discover matching Go/CTest cases or Web paths; omit empty leaves, fail an empty total")
			case "jobs":
				f.Func(n, "positive native build jobs; OCCCCAD_BUILD_JOBS; default 4", func(s string) error { v, e := positive(s); o.Jobs = v; return e })
			case "test-jobs":
				f.Func(n, "test concurrency; default 1", func(s string) error { v, e := positive(s); o.TestJobs = v; return e })
			case "target":
				f.StringVar(&o.Target, n, "", "real CMake target / loopback debug endpoint")
			case "packages":
				f.StringVar(&o.Packages, n, "", "comma-separated Go packages")
			case "report":
				f.StringVar(&o.Report, n, "", "owned execution report")
			case "base":
				f.StringVar(&o.Base, n, "", "local base revision; merge-base with head/HEAD")
			case "head":
				f.StringVar(&o.Head, n, "", "local head revision; excludes worktree")
			case "reference":
				f.StringVar(&o.Reference, n, "", "retained CAD_DIAGNOSTIC reference")
			case "file":
				f.StringVar(&o.File, n, "", "retained diagnostic file")
			case "diagnostic":
				f.StringVar(&o.Diagnostic, n, "", "exact recorded snapshot diagnostic ID for original replay")
			case "document":
				f.StringVar(&o.Document, n, "", "list recent document diagnostics")
			case "job":
				f.StringVar(&o.Job, n, "", "list recent job diagnostics")
			case "attempt":
				f.IntVar(&o.Attempt, n, 0, "recorded numerical attempt")
			case "recipe":
				f.StringVar(&o.Recipe, n, "", "property, rpc, selection, feature, diagnostic")
			case "symbol":
				f.StringVar(&o.Symbol, n, "", "literal search symbol")
			case "purpose":
				f.StringVar(&o.Purpose, n, o.Purpose, "validate, baseline, gaps, composition")
			case "capability":
				f.StringVar(&o.Capability, n, "", "stable capability ID")
			case "family":
				f.StringVar(&o.Family, n, "", "contract family")
			case "layer":
				f.StringVar(&o.Layer, n, "", "contract layer")
			case "case":
				f.StringVar(&o.Case, n, "", "stable case ID")
			case "adapters":
				f.StringVar(&o.Adapters, n, o.Adapters, "comma-separated evidence adapters")
			case "stages":
				f.StringVar(&o.Stages, n, o.Stages, "static,interaction,service,allocations")
			case "label":
				f.StringVar(&o.Label, n, "", "new measurement label")
			case "service":
				f.StringVar(&o.Service, n, "", "api, jobs or geometry")
			case "samples":
				f.IntVar(&o.Samples, n, 5, "measured samples (2..100)")
			default:
				bs := map[string]*bool{"changed": &o.Changed, "staged": &o.Staged, "json": &o.JSON, "verbose": &o.Verbose, "no-web": &o.NoWeb, "yes": &o.Yes, "replay": &o.Replay, "browser": &o.Browser, "live": &o.Live, "read": &o.Read, "check": &o.Check, "off": &o.Off}
				b, ok := bs[n]
				if !ok {
					panic(n)
				}
				f.BoolVar(b, n, false, n)
			}
		}
	}
	switch cmd {
	case "setup", "build":
		add("component profile jobs verbose")
	case "cpp:deps":
		add("profile jobs verbose")
	case "cpp:build":
		add("target profile jobs verbose")
	case "cpp:test":
		add("scope match profile test-jobs report verbose")
	case "go:build":
		add("profile jobs verbose")
	case "web:build":
		add("jobs verbose")
	case "go:check", "go:conformance":
		add("packages level match profile jobs test-jobs verbose")
	case "web:check":
		add("level match test-jobs verbose")
	case "plan", "check":
		add("scope changed staged base head level match profile jobs test-jobs json verbose")
	case "dev":
		add("profile jobs no-web verbose")
	case "clean":
		add("component profile")
	case "doctor":
		add("scope profile browser live json")
	case "data:reset":
		add("yes")
	case "docs:check":
		add("base json verbose")
	case "codegen":
		add("check jobs verbose")
	case "diagnose":
		add("reference file document job attempt diagnostic replay profile json")
	case "context":
		add("recipe symbol read json")
	case "benchmark", "benchmark:baseline":
		add("profile jobs samples stages label verbose")
	case "contract":
		add("purpose capability family layer case adapters profile jobs test-jobs verbose")
	case "debug:route":
		add("service target off")
	case "web:dev", "monitor":
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", f.Args())
	}
	var emptyFlag string
	f.Visit(func(v *flag.Flag) {
		if !oneOf(v.Name, "jobs", "test-jobs", "samples", "attempt", "changed", "staged", "json", "verbose", "no-web", "yes", "replay", "browser", "live", "read", "check", "off") && v.Value.String() == "" {
			emptyFlag = v.Name
		}
	})
	if emptyFlag != "" {
		return fmt.Errorf("--%s requires a non-empty value", emptyFlag)
	}
	explicitJobs := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == "jobs" {
			explicitJobs = true
		}
	})
	if f.Lookup("jobs") != nil && !explicitJobs {
		if v, ok := os.LookupEnv("OCCCCAD_BUILD_JOBS"); ok {
			n, e := positive(v)
			if e != nil {
				return e
			}
			o.Jobs = n
		}
	}
	if o.Jobs < 1 || o.TestJobs < 1 {
		return errors.New("--jobs and --test-jobs must be positive integers")
	}
	if !oneOf(o.Component, "cpp", "go", "web", "all", "tooling") {
		return fmt.Errorf("invalid component %q", o.Component)
	}
	if !oneOf(o.Profile, "release", "debug", "sanitize") {
		return fmt.Errorf("invalid profile %q", o.Profile)
	}
	if o.Scope != "" && !oneOf(o.Scope, "assembly", "geometry", "sketch", "workspace", "services", "web", "tooling", "docs", "all") {
		return fmt.Errorf("invalid scope %q", o.Scope)
	}
	if oneOf(cmd, "go:check", "go:conformance") && !oneOf(o.Level, "unit", "integration") {
		return fmt.Errorf("Go leaf supports only unit/integration levels")
	}
	if cmd == "web:check" && !oneOf(o.Level, "unit", "browser-mock", "browser-live") {
		return fmt.Errorf("Web leaf supports unit/browser-mock/browser-live levels")
	}
	if !oneOf(o.Level, "unit", "integration", "browser-mock", "browser-live", "full") {
		return fmt.Errorf("invalid level %q", o.Level)
	}
	switch cmd {
	case "setup":
		return a.Setup(o)
	case "build":
		return a.Build(o)
	case "cpp:deps":
		return a.CPPDeps(o)
	case "cpp:build":
		return a.CPPBuild(o)
	case "cpp:test":
		return a.CPPTest(o)
	case "go:build":
		return a.GoBuild(o)
	case "web:build":
		return a.WebBuild(o)
	case "go:check":
		return a.GoTest(o)
	case "go:conformance":
		return a.GoConformance(o)
	case "web:check":
		return a.WebTest(o)
	case "plan", "check":
		return a.Validate(o, cmd == "plan")
	case "doctor":
		return a.Doctor(o)
	case "clean":
		return a.Clean(o)
	case "dev":
		return a.Dev(o)
	case "web:dev":
		return a.raw(a.web(), "pnpm", "dev", "--strictPort")
	case "data:reset":
		return a.Reset(o)
	case "codegen":
		return a.Codegen(o)
	case "docs:check":
		return a.Docs(o)
	case "diagnose":
		return a.Diagnose(o)
	case "context":
		return a.TaskContext(o)
	case "benchmark", "benchmark:baseline":
		return a.Benchmark(o, cmd == "benchmark:baseline")
	case "contract":
		return a.Contract(o)
	case "monitor":
		binary := filepath.Join(a.Root, "build/services", o.Profile, "occccad-monitor")
		return syscall.Exec(binary, []string{binary}, os.Environ())
	case "debug:route":
		return a.DebugRoute(o)
	}
	return nil
}
func oneOf(s string, vs ...string) bool {
	for _, v := range vs {
		if s == v {
			return true
		}
	}
	return false
}
func (a *App) services() string { return filepath.Join(a.Root, "services") }
func (a *App) web() string      { return filepath.Join(a.Root, "web/apps/cad") }
func (a *App) emit(v any) error {
	e := json.NewEncoder(a.Out)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
