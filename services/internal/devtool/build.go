package devtool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const dependenciesSchema = "occccad.dependencies.v1"

type depsReceipt struct {
	Compiler, CompileEnvironment string
	Schema                       string            `json:"schema"`
	Identity                     string            `json:"identity"`
	Packages                     []string          `json:"packages"`
	Outputs                      map[string]string `json:"outputs"`
}

func buildType(p string) string {
	if p == "release" {
		return "Release"
	}
	return "Debug"
}
func (a *App) buildDir(o Options) string {
	return filepath.Join(a.Root, "build/cmake/linux-gcc15-x86_64", o.Profile)
}
func (a *App) generators(o Options) string {
	return filepath.Join(a.buildDir(o), "build", buildType(o.Profile), "generators")
}
func (a *App) toolchain(o Options) string {
	return filepath.Join(a.generators(o), "conan_toolchain.cmake")
}
func (a *App) profile(o Options) string {
	return filepath.Join(a.Root, "build-support/conan/profiles/linux-gcc15-"+strings.ToLower(buildType(o.Profile)))
}
func (a *App) depIdentity(o Options) (string, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", blocked("checked-in Conan profiles require Linux x86_64; add a verified native profile for this host")
	}
	version, e := a.capture(a.Root, nil, "g++-15", "-dumpfullversion")
	if e != nil {
		return "", blocked("missing g++-15; install GCC/G++ 15, pkg-config and X11/OpenGL development headers using your system package manager")
	}
	arch, e := a.capture(a.Root, nil, "g++-15", "-dumpmachine")
	if e != nil {
		return "", e
	}
	if !strings.HasPrefix(version, "15.") || !strings.HasPrefix(arch, "x86_64-") {
		return "", blocked("real compiler does not match linux-gcc15 x86_64 Conan profile")
	}
	if e := validateProfile(a.profile(o), buildType(o.Profile)); e != nil {
		return "", e
	}
	if e := validateProfile(a.profile(Options{Profile: "release"}), "Release"); e != nil {
		return "", e
	}
	inputs := map[string]string{"compiler": strings.TrimSpace(version), "arch": strings.TrimSpace(arch), "compilerBinary": fileHash("/usr/bin/g++-15"), "host": fileHash(a.profile(o)), "build": fileHash(a.profile(Options{Profile: "release"})), "recipe": fileHash(filepath.Join(a.Root, "conanfile.py")), "lock": fileHash(filepath.Join(a.Root, "conan.lock"))}
	for _, tool := range []string{"conan", "cmake", "ninja"} {
		v, e := a.capture(a.Root, nil, tool, "--version")
		if e != nil {
			return "", blocked("missing " + tool + "; run mise install --locked")
		}
		inputs[tool] = strings.TrimSpace(v)
	}
	// Compiler flags and Conan configuration affect package semantics. Runtime
	// credentials, jobs and log settings deliberately do not participate.
	for _, k := range []string{"CC", "CXX", "CFLAGS", "CXXFLAGS", "LDFLAGS", "CONAN_HOME", "CONAN_DEFAULT_PROFILE"} {
		inputs[k] = os.Getenv(k)
	}
	return hashJSON(inputs), nil
}
func (a *App) verifyDeps(o Options) error {
	id, e := a.depIdentity(o)
	if e != nil {
		return e
	}
	var r depsReceipt
	if loadJSON(filepath.Join(a.buildDir(o), "dependencies.json"), &r) != nil || (r.Identity != id || r.Schema != dependenciesSchema) {
		return blocked("dependency identity missing or changed; run mise run cpp:deps --profile " + o.Profile)
	}
	if len(r.Packages) == 0 || len(r.Outputs) == 0 {
		return blocked("incomplete dependency receipt; run mise run cpp:deps --profile " + o.Profile)
	}
	for p, h := range r.Outputs {
		if fileHash(p) != h {
			return blocked("missing/changed generated dependency output " + p + "; run mise run cpp:deps --profile " + o.Profile)
		}
	}
	for _, p := range r.Packages {
		if e := verifyPackage(p); e != nil {
			return blocked("Conan cache incomplete: " + e.Error() + "; run mise run cpp:deps --profile " + o.Profile)
		}
	}
	return nil
}
func (a *App) CPPDeps(o Options) error {
	unlock, e := a.buildLock(o, false)
	if e != nil {
		return e
	}
	defer unlock()
	// Conan writes share one local cache; serialize project-driven installs.
	home := os.Getenv("CONAN_HOME")
	if home == "" {
		h, _ := os.UserHomeDir()
		home = filepath.Join(h, ".conan2")
	}
	release, e := lock(a.Context, filepath.Join(home, "occccad-install.lock"), false)
	if e != nil {
		return e
	}
	defer release()
	if e = a.checkCachedCompiler(o); e != nil {
		return e
	}
	var previous depsReceipt
	if loadJSON(filepath.Join(a.buildDir(o), "dependencies.json"), &previous) == nil && previous.Compiler != "" && (previous.Compiler != fileHash("/usr/bin/g++-15") || previous.CompileEnvironment != compileEnvironment()) {
		return blocked("compiler or compile environment changed; clean this cpp profile explicitly and rerun cpp:deps")
	}
	id, e := a.depIdentity(o)
	if e != nil {
		return e
	}
	b := a.buildDir(o)
	if e = os.MkdirAll(b, 0700); e != nil {
		return e
	}
	graph := filepath.Join(b, "conan-graph.json")
	args := []string{"conan", "install", a.Root, "-of", b, "-pr:h", a.profile(o), "-pr:b", a.profile(Options{Profile: "release"}), "--lockfile", filepath.Join(a.Root, "conan.lock"), "--build=missing", "-c:h", "tools.build:jobs=" + strconv.Itoa(o.Jobs), "-c:b", "tools.build:jobs=" + strconv.Itoa(o.Jobs), "--format=json", "--out-file", graph}
	if e = a.native(a.Root, o.Verbose, args...); e != nil {
		return e
	}
	var g struct {
		Graph struct {
			Nodes map[string]struct {
				PackageFolder string `json:"package_folder"`
			} `json:"nodes"`
		} `json:"graph"`
	}
	if e = loadJSON(graph, &g); e != nil {
		return e
	}
	r := depsReceipt{Compiler: fileHash("/usr/bin/g++-15"), CompileEnvironment: compileEnvironment(), Schema: dependenciesSchema, Identity: id, Outputs: map[string]string{}}
	for _, n := range g.Graph.Nodes {
		if n.PackageFolder != "" {
			r.Packages = append(r.Packages, n.PackageFolder)
		}
	}
	sort.Strings(r.Packages)
	e = filepath.WalkDir(a.generators(o), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".cmake") || strings.HasSuffix(p, ".json")) {
			r.Outputs[p] = fileHash(p)
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range regexp.MustCompile(`set\([^ ]*_PACKAGE_FOLDER_[A-Z]+ "([^"]+)"\)`).FindAllStringSubmatch(string(b), -1) {
				r.Packages = appendUnique(r.Packages, m[1])
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(b, "dependencies.json"), r); e != nil {
		return e
	}
	if e = a.configure(o); e != nil {
		return e
	}
	return a.retireGeneratedReferences()
}
func (a *App) configure(o Options) error {
	if e := a.checkCachedCompiler(o); e != nil {
		return e
	}
	if e := a.native(a.Root, o.Verbose, "cmake", "--preset", "dev-"+o.Profile); e != nil {
		return e
	}
	return writeJSON(filepath.Join(a.buildDir(o), "configuration.json"), map[string]string{"preset": fileHash(filepath.Join(a.Root, "CMakePresets.json"))})
}

var targetName = regexp.MustCompile(`^[A-Za-z0-9_./+][A-Za-z0-9_./+-]*$`)

func (a *App) CPPBuild(o Options) (result error) {
	started := time.Now()
	timing := map[string]any{"profile": o.Profile, "target": o.Target, "jobs": o.Jobs}
	defer func() {
		timing["totalSeconds"] = time.Since(started).Seconds()
		timing["success"] = result == nil
		p := a.logPath("cpp-build") + ".json"
		if e := writeJSON(p, timing); e != nil && result == nil {
			result = e
		}
		b, _ := json.Marshal(timing)
		fmt.Fprintf(a.Err, "OCCCCAD_BUILD_TIMING %s; report: %s\n", b, p)
	}()
	if o.Target != "" && !targetName.MatchString(o.Target) {
		return fmt.Errorf("invalid CMake target %q", o.Target)
	}
	unlock, e := a.buildLock(o, false)
	timing["lockWaitSeconds"] = time.Since(started).Seconds()
	if e != nil {
		return e
	}
	defer unlock()
	checking := time.Now()
	e = a.verifyDeps(o)
	timing["dependencyCheckSeconds"] = time.Since(checking).Seconds()
	if e != nil {
		return e
	}
	var configuration map[string]string
	loadJSON(filepath.Join(a.buildDir(o), "configuration.json"), &configuration)
	if e = a.checkCachedCompiler(o); e != nil {
		return e
	}
	if _, e = os.Stat(filepath.Join(a.buildDir(o), "build.ninja")); e != nil || configuration["preset"] != fileHash(filepath.Join(a.Root, "CMakePresets.json")) || cmakeCacheValue(filepath.Join(a.buildDir(o), "CMakeCache.txt"), "CMAKE_BUILD_TYPE") != buildType(o.Profile) {
		configuring := time.Now()
		if e = a.configure(o); e != nil {
			return e
		}
		timing["configureSeconds"] = time.Since(configuring).Seconds()
	}
	args := []string{"cmake", "--build", a.buildDir(o), "--parallel", strconv.Itoa(o.Jobs)}
	if o.Target != "" {
		args = append(args, "--target", o.Target)
	}
	building := time.Now()
	result = a.native(a.Root, o.Verbose, args...)
	timing["nativeBuildSeconds"] = time.Since(building).Seconds()
	return result
}
func (a *App) GoBuild(o Options) error {
	release, e := lock(a.Context, filepath.Join(a.Root, "build/locks/go-"+o.Profile+".lock"), false)
	if e != nil {
		return e
	}
	defer release()
	genRelease, e := lock(a.Context, filepath.Join(a.Root, "build/locks/codegen.lock"), true)
	if e != nil {
		return e
	}
	defer genRelease()
	args := []string{"go", "build", "-mod=readonly", "-p", strconv.Itoa(o.Jobs), "-o", filepath.Join(a.Root, "build/services", o.Profile) + string(filepath.Separator)}
	if o.Profile != "release" {
		args = append(args, "-gcflags=all=-N -l")
	}
	args = append(args, "./cmd/...")
	return a.nativeEnv(a.services(), offlineGo(), o.Verbose, args...)
}
func (a *App) Build(o Options) error {
	for _, c := range []string{"cpp", "go", "web"} {
		if o.Component != c && o.Component != "all" {
			continue
		}
		var e error
		switch c {
		case "cpp":
			e = a.CPPBuild(o)
		case "go":
			e = a.GoBuild(o)
		case "web":
			e = a.WebBuild(o)
		}
		if e != nil {
			return e
		}
	}
	if o.Component == "tooling" {
		return fmt.Errorf("tooling is prepared by mise run setup --component tooling")
	}
	return nil
}
func (a *App) Setup(o Options) error {
	if _, e := os.Stat(filepath.Join(a.Root, ".env")); os.IsNotExist(e) {
		fmt.Fprintln(a.Err, "No .env. Copy .env.example to .env and select application resources before running dev/integration. Existing configuration is never overwritten.")
	}
	for _, c := range []string{"go", "web", "cpp"} {
		if o.Component != c && o.Component != "all" {
			continue
		}
		var e error
		switch c {
		case "go":
			e = a.native(a.services(), o.Verbose, "go", "mod", "download")
			if e == nil {
				e = a.native(filepath.Join(a.Root, "tests/go"), o.Verbose, "go", "mod", "download")
			}
		case "web":
			release, err := lock(a.Context, filepath.Join(a.Root, "build/locks/web.lock"), false)
			if err != nil {
				return err
			}
			e = a.native(filepath.Join(a.Root, "web"), o.Verbose, "pnpm", "install", "--frozen-lockfile")
			release()
		case "cpp":
			e = a.CPPDeps(o)
		}
		if e != nil {
			return e
		}
	}
	return nil
}

func verifyPackage(p string) error {
	b, e := os.ReadFile(filepath.Join(p, "conanmanifest.txt"))
	if e != nil {
		return e
	}
	for _, line := range strings.Split(string(b), "\n")[1:] {
		index := strings.LastIndex(line, ": ")
		if index < 0 {
			continue
		}
		f := filepath.Join(p, line[:index])
		if _, e = os.Lstat(f); e != nil {
			return e
		}
	}
	return nil
}

func (a *App) WebBuild(o Options) error {
	release, e := lock(a.Context, filepath.Join(a.Root, "build/locks/web.lock"), false)
	if e != nil {
		return e
	}
	defer release()
	return a.native(a.web(), o.Verbose, "pnpm", "build")
}

func validateProfile(path, bt string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return blocked("missing Conan profile: " + path)
	}
	section := ""
	values := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
		}
		if section == "[settings]" {
			if k, v, ok := strings.Cut(line, "="); ok {
				values[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	for k, want := range map[string]string{"os": "Linux", "arch": "x86_64", "compiler": "gcc", "compiler.version": "15", "compiler.cppstd": "17", "compiler.libcxx": "libstdc++11", "build_type": bt} {
		if values[k] != want {
			return blocked("Conan profile does not match real GCC 15 x86_64 contract: " + k + "=" + values[k])
		}
	}
	return nil
}

// Retire only references known to have been generated by the previous runner.
// User-authored presets and links to another checkout are never removed.
func (a *App) retireGeneratedReferences() error {
	p := filepath.Join(a.Root, "CMakeUserPresets.json")
	var old map[string]any
	if loadJSON(p, &old) == nil {
		vendor, _ := old["vendor"].(map[string]any)
		_, conan := vendor["conan"]
		includes, _ := old["include"].([]any)
		owned := conan && len(vendor) == 1 && len(includes) > 0
		for _, v := range includes {
			path, ok := v.(string)
			if !ok || !regexp.MustCompile(`^build/cmake/(release|debug)/build/(Release|Debug)/generators/CMakePresets\.json$`).MatchString(path) {
				owned = false
			}
		}
		for k := range old {
			if !oneOf(k, "version", "vendor", "include") {
				owned = false
			}
		}
		if owned {
			if e := os.Remove(p); e != nil {
				return e
			}
		}
	}
	p = filepath.Join(a.Root, "compile_commands.json")
	if target, e := os.Readlink(p); e == nil {
		if !filepath.IsAbs(target) {
			target = filepath.Join(a.Root, target)
		}
		if oneOf(filepath.Clean(target), filepath.Join(a.Root, "build/cmake/release/compile_commands.json"), filepath.Join(a.Root, "build/cmake/debug/compile_commands.json")) {
			return os.Remove(p)
		}
	}
	return nil
}

func compileEnvironment() string {
	values := map[string]string{}
	for _, k := range []string{"CC", "CXX", "CFLAGS", "CXXFLAGS", "LDFLAGS"} {
		values[k] = os.Getenv(k)
	}
	return hashJSON(values)
}
func cmakeCacheValue(path, key string) string {
	b, e := os.ReadFile(path)
	if e != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, key+":") {
			_, v, _ := strings.Cut(line, "=")
			return v
		}
	}
	return ""
}
func (a *App) checkCachedCompiler(o Options) error {
	compiler := cmakeCacheValue(filepath.Join(a.buildDir(o), "CMakeCache.txt"), "CMAKE_CXX_COMPILER")
	if compiler != "" && fileHash(compiler) != fileHash("/usr/bin/g++-15") {
		return blocked("cached compiler differs from the verified profile; run mise run clean --component cpp --profile " + o.Profile + ", then mise run cpp:deps --profile " + o.Profile)
	}
	return nil
}
