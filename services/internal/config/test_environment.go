package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LoadTestEnvironment shares the normal env-file precedence and anchors test
// paths at the checkout root, regardless of the package's working directory.
func LoadTestEnvironment() error {
	envFile, err := LoadProjectEnv()
	if err != nil {
		return err
	}
	if envFile != "" {
		absolute, err := filepath.Abs(envFile)
		if err != nil {
			return err
		}
		if err := os.Setenv("OCCCCAD_ENV_FILE", absolute); err != nil {
			return err
		}
	}
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
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
		"OCCCCAD_TEST_GEOMETRY_WORKER":        filepath.Join(root, "build", "cmake", strings.ToLower(value("OCCCCAD_BUILD_TYPE", "Release")), "workers", "geometry", "occccad_geometry_worker"),
	}
	for key, fallback := range paths {
		if err := os.Setenv(key, resolve(value(key, fallback))); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(resource, 0700); err != nil {
		return err
	}
	dsn := value("OCCCCAD_TEST_DATABASE_URL", "sqlite:"+filepath.Join(resource, "occccad_test.db"))
	if strings.HasPrefix(dsn, "sqlite:") {
		dsn = "sqlite:" + resolve(strings.TrimPrefix(dsn, "sqlite:"))
	}
	return os.Setenv("OCCCCAD_TEST_DATABASE_URL", dsn)
}
