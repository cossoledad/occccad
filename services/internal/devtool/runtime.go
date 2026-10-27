package devtool

import (
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func (a *App) Clean(o Options) error {
	components := []string{o.Component}
	if o.Component == "all" {
		components = []string{"cpp", "go", "web", "tooling"}
	}
	// Resolve environment through the same Go loader for overlap checks only.
	if _, e := config.LoadEnvironment(); e != nil {
		return e
	}
	for _, c := range components {
		var dest, lockName string
		switch c {
		case "cpp":
			dest = a.buildDir(o)
			lockName = "cpp-" + o.Profile
		case "go":
			dest = filepath.Join(a.Root, "build/services", o.Profile)
			lockName = "go-" + o.Profile
		case "web":
			dest = filepath.Join(a.web(), "dist")
			lockName = "web"
		case "tooling":
			dest = filepath.Join(a.Root, "build/tools")
			lockName = "tooling"
		}
		if e := safeClean(a.Root, dest, []string{os.Getenv("OCCCCAD_DATA_DIR"), os.Getenv("OCCCCAD_LOG_DIR"), strings.TrimPrefix(os.Getenv("OCCCCAD_DATABASE_URL"), "sqlite:"), os.Getenv("OCCCCAD_TEST_RESOURCES_DIR")}); e != nil {
			return e
		}
		release, e := lock(a.Context, filepath.Join(a.Root, "build/locks/"+lockName+".lock"), false)
		if e != nil {
			return e
		}
		e = os.RemoveAll(dest)
		release()
		if e != nil {
			return e
		}
		fmt.Fprintln(a.Out, "Removed rebuildable output:", dest)
	}
	return nil
}
func inside(parent, child string) bool {
	r, e := filepath.Rel(parent, child)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
func safeClean(root, dest string, protected []string) error {
	if dest == "" || dest == root || !inside(root, dest) {
		return fmt.Errorf("unsafe clean path %q", dest)
	}
	for p := dest; inside(root, p) && p != root; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("clean refuses symlink: %s", p)
		}
	}
	for _, p := range protected {
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if resolved, e := filepath.EvalSymlinks(p); e == nil {
			p = resolved
		}
		if !filepath.IsAbs(p) {
			continue
		}
		if inside(p, dest) || inside(dest, p) {
			return fmt.Errorf("clean overlaps configured resource: %s", p)
		}
	}
	return nil
}
func (a *App) Dev(o Options) error {
	// One lifecycle owner per checkout; no PID/port killing. Acquire before build.
	release, e := tryLock(filepath.Join(a.Root, "build/locks/runtime.lock"))
	if e != nil {
		return e
	}
	defer release()
	resourceRelease, e := lock(a.Context, filepath.Join(a.Root, "build/locks/resources.lock"), true)
	if e != nil {
		return e
	}
	defer resourceRelease()
	if e = a.runtimeEnv(o); e != nil {
		return e
	}
	for _, address := range []string{envValue("OCCCCAD_APP_LISTEN", "0.0.0.0:8080"), envValue("OCCCCAD_CONTROL_LISTEN", "127.0.0.1:19090"), envValue("OCCCCAD_API_INTERNAL_LISTEN", "127.0.0.1:18080"), envValue("OCCCCAD_GEOMETRY_ROUTER_LISTEN", "127.0.0.1:51001")} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return blocked("configured port occupied; no process was killed: " + address)
		}
		listener.Close()
	}
	cp := o
	cp.Target = "occccad_geometry_worker"
	if e = a.CPPBuild(cp); e != nil {
		return e
	}
	if e = a.GoBuild(o); e != nil {
		return e
	}
	hold, e := a.buildLock(o, true)
	if e != nil {
		return e
	}
	defer hold()
	goRelease, e := lock(a.Context, filepath.Join(a.Root, "build/locks/go-"+o.Profile+".lock"), true)
	if e != nil {
		return e
	}
	defer goRelease()
	source, e := a.Source()
	if e != nil {
		return e
	}
	id := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	os.Setenv("OCCCCAD_RUNTIME_ID", id)
	os.Setenv("OCCCCAD_DEV_PROFILE", o.Profile)
	os.Setenv("OCCCCAD_DEV_WEB", fmt.Sprint(!o.NoWeb))
	artifacts := map[string]string{}
	for _, n := range []string{"occccad-control", "occccad-server", "occccad-jobs"} {
		p := filepath.Join(a.Root, "build/services", o.Profile, n)
		artifacts[p] = fileHash(p)
	}
	artifacts[os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN")] = fileHash(os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"))
	receipt := filepath.Join(a.Root, "build/runtime/identity.json")
	if e = writeJSON(receipt, map[string]any{"schema": "occccad.runtime-build.v2", "runtimeId": id, "source": source, "artifacts": artifacts, "profile": o.Profile, "workspace": a.Root, "environment": environmentIdentity()}); e != nil {
		return e
	}
	c := exec.CommandContext(a.Context, filepath.Join(a.Root, "build/services", o.Profile, "occccad-control"))
	c.Dir = a.services()
	c.Stdout = a.Out
	c.Stderr = a.Err
	c.Cancel = func() error { return c.Process.Signal(syscall.SIGTERM) }
	c.WaitDelay = 15 * time.Second
	e = c.Run()
	if a.Context.Err() != nil {
		return nil
	}
	return e
}
func tryLock(p string) (func(), error) {
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, blocked("another owned runtime/integration is active in this checkout; stop it first")
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func (a *App) Reset(o Options) error {
	if !o.Yes {
		return fmt.Errorf("data:reset requires --yes; destructive configured development resources only")
	}
	release, e := tryLock(filepath.Join(a.Root, "build/locks/runtime.lock"))
	if e != nil {
		return e
	}
	defer release()
	resourceRelease, e := tryLock(filepath.Join(a.Root, "build/locks/resources.lock"))
	if e != nil {
		return e
	}
	defer resourceRelease()
	if e = a.runtimeEnv(o); e != nil {
		return e
	}
	if writers := configuredWriters(a.Root, "/proc", os.Getenv("OCCCCAD_ENV_FILE"), os.Getenv("OCCCCAD_DATABASE_URL")); len(writers) > 0 {
		return fmt.Errorf("stop configured application writers before reset: %v", writers)
	}
	for _, addr := range []string{envValue("OCCCCAD_APP_LISTEN", "0.0.0.0:8080"), envValue("OCCCCAD_CONTROL_LISTEN", "127.0.0.1:19090"), envValue("OCCCCAD_API_INTERNAL_LISTEN", "127.0.0.1:18080"), envValue("OCCCCAD_GEOMETRY_ROUTER_LISTEN", "127.0.0.1:51001")} {
		conn, e := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if e == nil {
			conn.Close()
			return fmt.Errorf("stop configured process before reset: %s", addr)
		}
	}
	fmt.Fprintln(a.Err, "Reset configured development targets:", environmentIdentity())
	os.Setenv("OCCCCAD_ALLOW_DEV_RESET", "1")
	return a.native(a.services(), true, filepath.Join(a.Root, "build/services", o.Profile, "occccad-migrate"), "-reset-development-data")
}
func envValue(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func (a *App) DebugRoute(o Options) error {
	if !oneOf(o.Service, "api", "jobs", "geometry") {
		return fmt.Errorf("--service requires api, jobs or geometry")
	}
	if _, e := config.LoadEnvironment(); e != nil {
		return e
	}
	addr := envValue("OCCCCAD_CONTROL_LISTEN", "127.0.0.1:19090")
	if e := loopback(addr); e != nil {
		return e
	}
	method := "POST"
	if o.Off {
		method = "DELETE"
	} else if o.Service != "jobs" {
		if e := loopback(o.Target); e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(a.Context, method, "http://"+addr+"/control/debug/"+o.Service, strings.NewReader(fmt.Sprintf(`{"target":%q}`, o.Target)))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 5 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("debug route returned %s", res.Status)
	}
	return nil
}
func loopback(addr string) error {
	h, _, e := net.SplitHostPort(addr)
	if e != nil {
		return e
	}
	ip := net.ParseIP(h)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("management/debug address must be numeric loopback: %q", addr)
	}
	return nil
}

// Read-only reset guard for standalone services, including Jobs without a port.
// It reports matching writers; it never signals or adopts another process.
func configuredWriters(root, procRoot, envFile, dsn string) []string {
	entries, _ := os.ReadDir(procRoot)
	writers := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join(procRoot, entry.Name())
		exe, e := os.Readlink(filepath.Join(p, "exe"))
		if e != nil {
			continue
		}
		name := filepath.Base(strings.TrimSuffix(exe, " (deleted)"))
		if !oneOf(name, "occccad-control", "occccad-server", "occccad-jobs", "occccad_geometry_worker") {
			continue
		}
		raw, _ := os.ReadFile(filepath.Join(p, "environ"))
		values := map[string]string{}
		for _, kv := range strings.Split(string(raw), "\x00") {
			if k, v, ok := strings.Cut(kv, "="); ok {
				values[k] = v
			}
		}
		cwd, _ := os.Readlink(filepath.Join(p, "cwd"))
		if (envFile != "" && values["OCCCCAD_ENV_FILE"] == envFile) || (dsn != "" && values["OCCCCAD_DATABASE_URL"] == dsn) || inside(root, cwd) || inside(root, exe) {
			writers = append(writers, name+" pid="+entry.Name())
		}
	}
	return writers
}
