package devtool

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

func (a *App) Benchmark(o Options, baseline bool) error {
	if o.Samples < 2 || o.Samples > 100 {
		return fmt.Errorf("--samples must be 2..100")
	}
	if baseline {
		dir := filepath.Join(a.Root, "build/performance")
		log := filepath.Join(dir, "go-workspace-"+time.Now().UTC().Format("20060102T150405.000000000")+".txt")
		argv := []string{"go", "test", "-p", strconv.Itoa(o.Jobs), "./internal/workspace", "-run", "^$", "-bench", "Benchmark(ProfileBuilder|VisualizationManifest)", "-benchmem", "-count=" + strconv.Itoa(o.Samples)}
		a.recordNative(a.services(), argv)
		fmt.Fprintln(a.Err, "[run] Go benchmarks; log:", log)
		var live io.Writer
		if o.Verbose {
			live = a.Err
		}
		counts := map[string]int{}
		observe := func(line string) {
			for _, name := range []string{"ProfileBuilder", "VisualizationManifest"} {
				if regexp.MustCompile(`^Benchmark` + name + `\S*\s+\d+\s+[0-9.eE+-]+\s+ns/op`).MatchString(line) {
					counts[name]++
				}
			}
		}
		_, e := a.stream(a.services(), nil, log, live, observe, argv...)
		if e != nil {
			return fmt.Errorf("Go benchmarks failed; log %s: %w", log, e)
		}
		for _, name := range []string{"ProfileBuilder", "VisualizationManifest"} {
			if counts[name] < o.Samples {
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
