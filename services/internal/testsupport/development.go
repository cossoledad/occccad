// Package testsupport uses the application's configured development resources.
package testsupport

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

func Environment(t *testing.T) config.Config {
	t.Helper()
	if !integrationEnabled {
		t.Fatal("configured application resources require go test -tags=integration or mise run check --level integration")
	}
	if _, err := config.LoadEnvironment(); err != nil {
		t.Fatal(err)
	}
	return config.Load()
}

// OpenDevelopmentDatabase opens exactly the database used by mise run dev.
// Fixtures migrate it without resetting data or selecting another database.
func OpenDevelopmentDatabase(t *testing.T) *database.Pool {
	t.Helper()
	db, err := database.Open(t.Context(), Environment(t).DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

// OpenArtifactStore shares the application's LOCAL/S3 backend and credentials.
func OpenArtifactStore(t *testing.T) artifact.Store {
	t.Helper()
	store, _, err := artifact.OpenConfigured(t.Context(), Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// Run loads the same environment as application startup before tests read it.
func Run(m *testing.M) int {
	if !integrationEnabled {
		return m.Run()
	}
	if _, err := config.LoadEnvironment(); err != nil {
		fmt.Fprintln(os.Stderr, "development environment:", err)
		return 1
	}
	if err := os.MkdirAll(os.Getenv("OCCCCAD_TEST_RESOURCES_DIR"), 0700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func PrepareAssemblyFixtures(t *testing.T) {
	t.Helper()
	Environment(t)
	directory := os.Getenv("OCCCCAD_ASSEMBLY_FIXTURE_DIR")
	worker := os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN")
	binary := filepath.Clean(filepath.Join(filepath.Dir(worker), "..", "..", "kernel", "occt", "tests", "occcad_geometry_scenarios"))
	producer, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("analytic fixture generator missing: %s", binary)
	}
	complete := true
	for _, name := range []string{"sphere-r6", "sphere-r3", "sphere-r075", "cylinder-r6-h12", "cylinder-r3-h12", "cone-r9-r3-h12", "cone-r6-r2-h8"} {
		for _, extension := range []string{".brep", ".step"} {
			if input, err := os.Stat(filepath.Join(directory, name+extension)); err != nil || input.ModTime().Before(producer.ModTime()) {
				complete = false
			}
		}
	}
	if complete {
		return
	}
	command := exec.CommandContext(t.Context(), binary, "--gtest_filter=AssemblyExactSupport.ExportAnalyticRouterFixtures")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("analytic fixture generation failed: %v\n%s", err, output)
	}
}

func PrepareNamingFixture(t *testing.T) string {
	t.Helper()
	Environment(t)
	path := os.Getenv("OCCCCAD_TEST_NAMING_FIXTURE")
	worker := os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN")
	binary := filepath.Join(filepath.Dir(worker), "occccad_naming_artifact_tests")
	producer, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("native naming producer missing: %s; build occccad_naming_artifact_tests", binary)
	}
	if fixture, err := os.Stat(path); err == nil && !fixture.ModTime().Before(producer.ModTime()) {
		return path
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), binary, "--gtest_filter=NamingArtifact.InternsEvidenceAndKeepsExplicitBodyTips")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native naming fixture failed: %v\n%s", err, output)
	}
	return path
}
