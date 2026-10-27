package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ProjectRoot follows the checkout, not the location of a selected env file.
func ProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	executable, _ := os.Executable()
	_, source, _, _ := runtime.Caller(0)
	for _, start := range []string{cwd, filepath.Dir(executable), filepath.Dir(source)} {
		for directory := start; directory != ""; directory = filepath.Dir(directory) {
			if _, err := os.Stat(filepath.Join(directory, "services", "go.mod")); err == nil {
				return filepath.Clean(directory), nil
			}
			if parent := filepath.Dir(directory); parent == directory {
				break
			}
		}
	}
	// Standalone packaged services may have no source checkout. Their explicit
	// env file (or working directory) remains the resource anchor.
	if file := os.Getenv("OCCCCAD_ENV_FILE"); file != "" {
		absolute, err := filepath.Abs(file)
		return filepath.Dir(absolute), err
	}
	return cwd, nil
}

// LoadEnvironment is shared by application processes and integration tests.
// Runtime resources refer to the same development environment, regardless of
// the caller's working directory. Generated inputs are not a second database.
func LoadEnvironment() (string, error) {
	envFile, err := LoadProjectEnv()
	if err != nil {
		return "", err
	}
	if envFile != "" {
		envFile, err = filepath.Abs(envFile)
		if err != nil {
			return "", err
		}
		if err := os.Setenv("OCCCCAD_ENV_FILE", envFile); err != nil {
			return "", err
		}
	}
	root, err := ProjectRoot()
	if err != nil {
		return "", err
	}
	resolve := func(path string) string {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		return filepath.Clean(path)
	}
	resource := resolve(value("OCCCCAD_TEST_RESOURCES_DIR", "build/test-resources"))
	paths := map[string]string{
		"OCCCCAD_TEST_RESOURCES_DIR":          resource,
		"OCCCCAD_ASSEMBLY_FIXTURE_DIR":        filepath.Join(resource, "analytic-fixtures"),
		"OCCCCAD_TEST_IMPORT_BREP":            filepath.Join(resource, "import-repair.brep"),
		"OCCCCAD_TEST_CURVED_GLB":             filepath.Join(resource, "curved.glb"),
		"OCCCCAD_TEST_GLB_FIXTURE":            filepath.Join(resource, "geometry.glb"),
		"OCCCCAD_TEST_ASSEMBLY_REPLAY_OUTPUT": filepath.Join(resource, "assembly-replay.3dreplay"),
		"OCCCCAD_TEST_NAMING_FIXTURE":         filepath.Join(resource, "naming-v2.pb"),
		"OCCCCAD_GEOMETRY_WORKER_BIN":         filepath.Join(root, "build", "cmake", "linux-gcc15-x86_64", value("OCCCCAD_DEV_PROFILE", strings.ToLower(value("OCCCCAD_BUILD_TYPE", "Release"))), "workers", "geometry", "occccad_geometry_worker"),
	}
	for key, fallback := range paths {
		if err := os.Setenv(key, resolve(value(key, fallback))); err != nil {
			return "", err
		}
	}
	// Keep the established services-relative meaning of ./data and ./logs,
	// resolving it once for Control, standalone processes and tests alike.
	servicesRoot := root
	if _, err := os.Stat(filepath.Join(root, "services", "go.mod")); err == nil {
		servicesRoot = filepath.Join(root, "services")
	}
	for key, fallback := range map[string]string{"OCCCCAD_DATA_DIR": "./data", "OCCCCAD_LOG_DIR": "./logs"} {
		path := value(key, fallback)
		if !filepath.IsAbs(path) {
			path = filepath.Join(servicesRoot, path)
		}
		if err := os.Setenv(key, filepath.Clean(path)); err != nil {
			return "", err
		}
	}
	dsn := Load().DatabaseURL
	if strings.HasPrefix(dsn, "sqlite:") {
		dsn = "sqlite:" + resolve(strings.TrimPrefix(dsn, "sqlite:"))
	}
	return envFile, os.Setenv("OCCCCAD_DATABASE_URL", dsn)
}
