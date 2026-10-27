package devtool

import (
	"fmt"
	"os"
	"path/filepath"
)

func (a *App) Contract(o Options) error {
	if o.Purpose != "validate" {
		if e := a.runtimeEnv(o); e != nil {
			return e
		}
	}
	args := []string{filepath.Join(a.Root, "tests/assembly-contract/runner.mjs"), o.Purpose, "--profile", o.Profile, "--jobs", fmt.Sprint(o.Jobs), "--test-jobs", fmt.Sprint(o.TestJobs)}
	for k, v := range map[string]string{"capability": o.Capability, "family": o.Family, "layer": o.Layer, "case": o.Case, "adapters": o.Adapters} {
		if v != "" {
			args = append(args, "--"+k, v)
		}
	}
	return a.rawEnv(a.Root, os.Environ(), append([]string{"node"}, args...)...)
}
