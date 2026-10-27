package devtool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func environmentIdentity() map[string]any {
	c := config.Load()
	db := map[string]any{}
	if strings.HasPrefix(c.DatabaseURL, "sqlite:") {
		db["backend"] = "sqlite"
		db["path"] = strings.TrimPrefix(c.DatabaseURL, "sqlite:")
	} else if u, e := url.Parse(c.DatabaseURL); e == nil {
		db["backend"] = "postgres"
		db["host"] = u.Hostname()
		db["port"] = u.Port()
		db["database"] = strings.TrimPrefix(u.Path, "/")
	}
	return map[string]any{"envFile": map[string]string{"path": os.Getenv("OCCCCAD_ENV_FILE"), "sha256": fileHash(os.Getenv("OCCCCAD_ENV_FILE"))}, "configurationSHA256": hashJSON(map[string]any{"application": c, "s3": []string{os.Getenv("OCCCCAD_ARTIFACT_BACKEND"), os.Getenv("OCCCCAD_S3_ENDPOINT"), os.Getenv("OCCCCAD_S3_BUCKET"), os.Getenv("OCCCCAD_S3_ACCESS_KEY"), os.Getenv("OCCCCAD_S3_SECRET_KEY")}}), "listeners": map[string]string{"app": envValue("OCCCCAD_APP_LISTEN", "0.0.0.0:8080"), "control": envValue("OCCCCAD_CONTROL_LISTEN", "127.0.0.1:19090"), "router": envValue("OCCCCAD_GEOMETRY_ROUTER_LISTEN", "127.0.0.1:51001"), "api": envValue("OCCCCAD_API_INTERNAL_LISTEN", "127.0.0.1:18080")}, "database": db, "dataDirectory": c.DataDirectory, "logDirectory": c.LogDirectory, "artifactBackend": envValue("OCCCCAD_ARTIFACT_BACKEND", "LOCAL"), "bucket": os.Getenv("OCCCCAD_S3_BUCKET"), "worker": map[string]string{"path": os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"), "sha256": fileHash(os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"))}}
}
func (a *App) Doctor(o Options) error {
	rows := []map[string]any{}
	add := func(n string, e error, prepare string) {
		status, detail := "PASS", "ready"
		if e != nil {
			status = "ENVIRONMENT_BLOCKED"
			detail = redact(e.Error())
		}
		rows = append(rows, map[string]any{"name": n, "status": status, "detail": detail, "prepare": prepare})
	}
	for _, tool := range []string{"git", "go", "node", "pnpm", "cmake", "ninja", "conan", "ccache", "protoc", "protoc-gen-go", "protoc-gen-go-grpc"} {
		_, e := exec.LookPath(tool)
		add(tool, e, "mise install --locked")
	}
	if o.Scope == "" || oneOf(o.Scope, "all", "assembly", "geometry", "sketch") {
		add("dependencies", a.verifyDeps(o), "mise run cpp:deps --profile "+o.Profile)
		out, err := a.capture(a.Root, nil, "ninja", "-C", a.buildDir(o), "-n", "occccad_geometry_worker", "occcad_geometry_scenarios", "occccad_naming_artifact_tests")
		if err == nil && strings.Contains(out, "Re-running CMake") {
			rows = append(rows, map[string]any{"name": "native-freshness", "status": "NOT_RUN", "required": false, "detail": "Ninja dry run requests a configure/glob check; freshness needs an actual build/check. --live separately verifies loaded code and artifacts."})
		} else {
			if err == nil && !strings.Contains(out, "no work to do") {
				err = blocked("native inputs changed or fixture producers missing")
			}
			add("native-freshness", err, "mise run cpp:build --profile "+o.Profile)
		}
		for _, binary := range []string{"workers/geometry/occccad_geometry_worker", "kernel/occt/tests/occcad_geometry_scenarios", "workers/geometry/occccad_naming_artifact_tests"} {
			info, e := os.Stat(filepath.Join(a.buildDir(o), binary))
			if e == nil && info.Mode()&0111 == 0 {
				e = fmt.Errorf("not executable")
			}
			add(filepath.Base(binary), e, "mise run cpp:build --target "+filepath.Base(binary)+" --profile "+o.Profile)
		}
	}
	if e := a.runtimeEnv(o); e != nil {
		add("environment", e, "select an existing OCCCCAD_ENV_FILE")
	} else if o.Scope == "" || o.Scope == "all" || o.Live || o.Scope == "services" {
		exe, _ := os.Executable()
		ctx, cancel := context.WithTimeout(a.Context, 10*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, exe, "doctor:resources")
		c.Dir = a.Root
		out, err := c.Output()
		var checks []map[string]any
		if json.Unmarshal(out, &checks) == nil {
			rows = append(rows, checks...)
		} else {
			add("resources", err, "mise run dev --no-web")
		}
	}
	if o.Live {
		add("running-build", a.RunningIdentity(o), "restart mise run dev")
	}
	if o.Browser {
		script := `const {chromium}=require('@playwright/test');chromium.launch({headless:true,args:['--use-gl=angle','--use-angle=swiftshader','--enable-unsafe-swiftshader']}).then(b=>b.close()).catch(e=>{console.error(String(e));process.exit(2)})`
		_, e := a.capture(a.web(), nil, "node", "-e", script)
		add("existing-chromium", e, "install Chromium explicitly; see docs/development/environment.md")
	}
	failed := false
	for _, r := range rows {
		if r["status"] == "FAIL" || r["status"] == "ENVIRONMENT_BLOCKED" {
			failed = true
		}
	}
	status := "PASS"
	if failed {
		status = "ENVIRONMENT_BLOCKED"
	}
	if o.JSON {
		a.emit(map[string]any{"schema": "occccad.development-doctor.v2", "status": status, "checks": rows, "environment": environmentIdentity()})
	} else {
		for _, r := range rows {
			fmt.Fprintf(a.Out, "%s %s: %s\n", r["status"], r["name"], r["detail"])
			if r["status"] != "PASS" {
				fmt.Fprintln(a.Out, "  prepare:", r["prepare"])
			}
		}
	}
	if failed {
		return blocked("ENVIRONMENT_BLOCKED: doctor found missing preparation/resources")
	}
	return nil
}
func (a *App) RunningIdentity(o Options) error { return a.runningIdentityAt(o, "/proc") }
func (a *App) runningIdentityAt(o Options, procRoot string) error {
	var saved struct {
		Source             any
		Artifacts          map[string]string
		RuntimeID          string `json:"runtimeId"`
		Profile, Workspace string
		Environment        map[string]any
	}
	if e := loadJSON(filepath.Join(a.Root, "build/runtime/identity.json"), &saved); e != nil {
		return blocked("no runtime receipt")
	}
	source, e := a.Source()
	if e != nil {
		return e
	}
	if hashJSON(source) != hashJSON(saved.Source) || saved.Profile != o.Profile || saved.Workspace != a.Root {
		return blocked("runtime source/profile/workspace changed")
	}
	if hashJSON(saved.Environment) != hashJSON(environmentIdentity()) {
		return blocked("configured runtime resources or Worker changed")
	}
	if len(saved.Artifacts) < 4 {
		return blocked("runtime receipt lacks mandatory binaries")
	}
	names := map[string]bool{}
	for p := range saved.Artifacts {
		names[filepath.Base(p)] = true
	}
	for _, n := range []string{"occccad-control", "occccad-server", "occccad-jobs", "occccad_geometry_worker"} {
		if !names[n] {
			return blocked("runtime receipt missing " + n)
		}
	}
	seen := map[string]bool{}
	for p, h := range saved.Artifacts {
		if fileHash(p) != h {
			return blocked("disk artifact updated since startup: " + p)
		}
	}
	entries, e := os.ReadDir(procRoot)
	if e != nil {
		return blocked("loaded binary verification requires Linux /proc")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join(procRoot, entry.Name())
		env, e := os.ReadFile(filepath.Join(p, "environ"))
		if e != nil || !strings.Contains("\x00"+string(env), "\x00OCCCCAD_RUNTIME_ID="+saved.RuntimeID+"\x00") {
			continue
		}
		exe, e := os.Readlink(filepath.Join(p, "exe"))
		if e != nil {
			continue
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		if expected, ok := saved.Artifacts[exe]; ok {
			if fileHash(filepath.Join(p, "exe")) != expected {
				return blocked("loaded process binary is stale: " + entry.Name())
			}
			seen[exe] = true
		}
	}
	for p := range saved.Artifacts {
		if !seen[p] {
			return blocked("recorded runtime process not running: " + p)
		}
	}
	return nil
}

func (a *App) Preflight(o Options, resources []string) error {
	if len(resources) == 0 {
		return nil
	}
	if e := a.runtimeEnv(o); e != nil {
		return blocked(e.Error())
	}
	if oneOf("worker", resources...) {
		p := os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN")
		info, e := os.Stat(p)
		if e != nil || info.Mode()&0111 == 0 {
			return blocked("selected Worker missing; run mise run cpp:build --target occccad_geometry_worker --profile " + o.Profile)
		}
	}
	if oneOf("database", resources...) || oneOf("artifact-store", resources...) {
		exe, _ := os.Executable()
		out, err := a.capture(a.Root, nil, exe, "doctor:resources")
		var rows []map[string]any
		if json.Unmarshal([]byte(out), &rows) != nil {
			return blocked("resource checker could not run: " + fmt.Sprint(err))
		}
		for _, row := range rows {
			if row["status"] != "PASS" {
				return blocked(fmt.Sprint(row["name"]) + ": " + fmt.Sprint(row["detail"]))
			}
		}
	}
	if oneOf("live-stack", resources...) {
		if e := a.RunningIdentity(o); e != nil {
			return e
		}
	}
	if oneOf("browser", resources...) {
		_, e := a.capture(a.web(), nil, "node", "-e", `const {chromium}=require('@playwright/test');require('fs').accessSync(chromium.executablePath())`)
		if e != nil {
			return blocked("existing Playwright browser unavailable; prepare it explicitly; no automatic download")
		}
	}
	return nil
}
