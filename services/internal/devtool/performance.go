package devtool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

func (a *App) Benchmark(o Options, baseline bool) error {
	if o.Samples < 2 || o.Samples > 100 {
		return fmt.Errorf("--samples must be 2..100")
	}
	if baseline {
		out, e := a.capture(a.services(), nil, "go", "test", "-p", strconv.Itoa(o.Jobs), "./internal/workspace", "-run", "^$", "-bench", "Benchmark(ProfileBuilder|VisualizationManifest)", "-benchmem", "-count="+strconv.Itoa(o.Samples))
		dir := filepath.Join(a.Root, "build/performance")
		os.MkdirAll(dir, 0700)
		log := filepath.Join(dir, "go-workspace.txt")
		if err := os.WriteFile(log, []byte(redact(out)), 0600); err != nil {
			return err
		}
		if e != nil {
			return fmt.Errorf("Go benchmarks failed; log %s: %w", log, e)
		}
		for _, name := range []string{"ProfileBuilder", "VisualizationManifest"} {
			if len(regexp.MustCompile(`(?m)^Benchmark`+name+`\S*\s+\d+\s+[0-9.eE+-]+\s+ns/op`).FindAllString(out, -1)) < o.Samples {
				return fmt.Errorf("Benchmark%s missing measured samples; %s", name, log)
			}
		}
		for _, target := range []string{"occcad_assembly_solver_benchmark", "occcad_assembly_interaction_benchmark"} {
			cp := o
			cp.Target = target
			if e = a.CPPBuild(cp); e != nil {
				return e
			}
		}
	}
	if e := a.runtimeEnv(o); e != nil {
		return e
	}
	hold, e := a.buildLock(o, true)
	if e != nil {
		return e
	}
	defer hold()
	args := []string{"node", filepath.Join(a.Root, "kernel/assembly/tests/run-performance.mjs"), "--profile", o.Profile, "--samples", strconv.Itoa(o.Samples), "--stages", o.Stages, "--jobs", strconv.Itoa(o.Jobs)}
	if o.Label != "" {
		args = append(args, "--label", o.Label)
	}
	return a.native(a.Root, true, args...)
}
