package devtool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type runtimeReceipt struct {
	Schema          string            `json:"schema"`
	RuntimeID       string            `json:"runtimeId"`
	Profile         string            `json:"profile"`
	Workspace       string            `json:"workspace"`
	BackendInputs   map[string]any    `json:"backendInputs"`
	Artifacts       map[string]string `json:"artifacts"`
	LoadedArtifacts map[string]string `json:"loadedArtifacts"`
	Environment     map[string]any    `json:"environment"`
	Frontend        map[string]any    `json:"frontend"`
}

func (a *App) backendInputs() (map[string]any, error) {
	files, e := a.git("ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if e != nil {
		return nil, e
	}
	hashes := map[string]string{}
	for _, p := range strings.Split(files, "\x00") {
		if !backendInput(p) {
			continue
		}
		hashes[p] = fileHash(filepath.Join(a.Root, p))
	}
	return map[string]any{"sha256": hashJSON(hashes), "files": len(hashes), "boundary": "backend Go/C++/Proto and compile inputs; excludes docs, tests, tooling and frontend HMR inputs"}, nil
}
func backendInput(p string) bool {
	if strings.Contains(p, "/tests/") || strings.HasSuffix(p, "_test.go") {
		return false
	}
	if strings.HasPrefix(p, "services/internal/devtool/") || strings.HasPrefix(p, "services/internal/testsupport/") || strings.HasPrefix(p, "services/cmd/occccad-dev/") {
		return false
	}
	if strings.HasPrefix(p, "services/") {
		return strings.HasSuffix(p, ".go") || oneOf(p, "services/go.mod", "services/go.sum")
	}
	if strings.HasPrefix(p, "proto/") {
		return strings.HasSuffix(p, ".proto")
	}
	if strings.HasPrefix(p, "kernel/") || strings.HasPrefix(p, "workers/") {
		return oneOf(filepath.Ext(p), ".cpp", ".cxx", ".h", ".hpp") || filepath.Base(p) == "CMakeLists.txt"
	}
	return oneOf(p, "CMakeLists.txt", "CMakePresets.json", "conanfile.py", "conan.lock") || strings.HasPrefix(p, "build-support/")
}
func runtimeConfiguration() map[string]any {
	identity := environmentIdentity()
	delete(identity, "envFile") // Effective values matter; comments and unrelated Vite settings do not.
	identity["worker"] = map[string]string{"path": os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN")}
	identity["control"] = map[string]string{
		"serverOverride": os.Getenv("OCCCCAD_SERVER_BIN"), "jobsOverride": os.Getenv("OCCCCAD_JOBS_BIN"),
		"workerMin": envValue("OCCCCAD_GEOMETRY_WORKER_MIN", "1"), "workerMax": envValue("OCCCCAD_GEOMETRY_WORKER_MAX", "8"),
		"geometryPerWorker": envValue("OCCCCAD_GEOMETRY_PER_WORKER", "2"), "firstWorkerPort": envValue("OCCCCAD_GEOMETRY_WORKER_FIRST_PORT", "51100"),
		"workerIdle": envValue("OCCCCAD_GEOMETRY_WORKER_IDLE", "5m"),
	}
	return identity
}
func copyExecutable(from, to string) error {
	info, e := os.Stat(from)
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return fmt.Errorf("not an executable: %s", from)
	}
	in, e := os.Open(from)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0500)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	closed := out.Close()
	if e != nil {
		return e
	}
	return closed
}

// Readers protect only the copy operation. The running Control, API, Jobs and
// every replacement Worker subsequently execute these immutable instance files.
func (a *App) snapshotRuntime(o Options, id string) (runtimeReceipt, string, error) {
	receipt := runtimeReceipt{Schema: "occccad.runtime-build.v3", RuntimeID: id, Profile: o.Profile, Workspace: a.Root, Artifacts: map[string]string{}, LoadedArtifacts: map[string]string{}, Environment: runtimeConfiguration(), Frontend: map[string]any{"managed": !o.NoWeb, "listen": envValue("OCCCCAD_WEB_LISTEN", "0.0.0.0:5173"), "mode": "Vite development/HMR; not a backend build identity"}}
	dir := filepath.Join(a.Root, "build/runtime/instances", id)
	cpp, e := a.buildLock(o, true)
	if e != nil {
		return receipt, dir, e
	}
	defer cpp()
	goLock, e := lock(a.Context, filepath.Join(a.Root, "build/locks/go-"+o.Profile+".lock"), true)
	if e != nil {
		return receipt, dir, e
	}
	defer goLock()
	if e = os.MkdirAll(dir, 0700); e != nil {
		return receipt, dir, e
	}
	sources := map[string]string{
		"occccad-control":         filepath.Join(a.Root, "build/services", o.Profile, "occccad-control"),
		"occccad-server":          envValue("OCCCCAD_SERVER_BIN", filepath.Join(a.Root, "build/services", o.Profile, "occccad-server")),
		"occccad-jobs":            envValue("OCCCCAD_JOBS_BIN", filepath.Join(a.Root, "build/services", o.Profile, "occccad-jobs")),
		"occccad_geometry_worker": os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"),
	}
	for name, from := range sources {
		to := filepath.Join(dir, name)
		if e = copyExecutable(from, to); e != nil {
			return receipt, dir, e
		}
		h := fileHash(from)
		if h != fileHash(to) {
			return receipt, dir, fmt.Errorf("artifact changed during runtime snapshot: %s", from)
		}
		receipt.Artifacts[from] = h
		receipt.LoadedArtifacts[to] = h
	}
	receipt.BackendInputs, e = a.backendInputs()
	return receipt, dir, e
}
