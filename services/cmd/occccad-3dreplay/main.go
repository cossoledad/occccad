// occccad-3dreplay executes immutable numerical snapshots with the production solver.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, workspace.AssemblyReplayFileBudget+1))
	if err == nil && len(data) > workspace.AssemblyReplayFileBudget {
		return nil, fmt.Errorf("replay file exceeds byte budget")
	}
	return data, err
}

func run() error {
	address := flag.String("worker", "127.0.0.1:51001", "Geometry Worker or Router address")
	binary := flag.String("worker-binary", "", "start a local production Worker for offline numerical replay")
	output := flag.String("out", "", "new output file (default stdout); never overwrites input or an existing result")
	mode := flag.String("mode", "accepted", "accepted, accepted-pending, all-driving, subsystem, original")
	pending := flag.String("constraints", "", "comma-separated NotUpdated IDs for accepted-pending")
	target := flag.String("target", "", "target constraint for original/subsystem")
	diagnostic := flag.String("diagnostic", "", "original diagnostic ID")
	include := flag.Bool("include-unverified", false, "select all resolvable NotUpdated definitions (accepted-pending)")
	overrides := flag.String("overrides", "", "JSON AssemblyReplayOptions patch: initialGuesses, angleBranches, maxIterations, maxPreferenceIterations, wallClockMs")
	timeout := flag.Duration("timeout", 30*time.Second, "overall execution budget (does not replace a recorded per-stage budget)")
	trace := flag.Bool("trace", false, "capture bounded residual/Jacobian evaluation trajectory")
	matrices := flag.Bool("matrices", false, "include final null-space matrices and traced Jacobians")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: occccad-3dreplay [-worker-binary path | -worker address] [-mode mode] [-out newfile] input.3dreplay")
		return fmt.Errorf("exactly one input file is required")
	}
	input, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		return err
	}
	if *output != "" {
		out, err := filepath.Abs(*output)
		if err != nil {
			return err
		}
		if out == input {
			return fmt.Errorf("output must differ from the original snapshot")
		}
	}
	data, err := readBounded(input)
	if err != nil {
		return err
	}
	options := workspace.AssemblyReplayOptions{Mode: *mode, TargetConstraintID: *target, DiagnosticID: *diagnostic, Trace: *trace, FullMatrices: *matrices}
	if *pending != "" {
		options.ConstraintIDs = strings.Split(*pending, ",")
	}
	if *include {
		options.Mode = "accepted-pending"
	}
	if *overrides != "" {
		raw, err := readBounded(*overrides)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &options); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	var client *geometry.Client
	var close func()
	if *binary != "" {
		client, close, err = geometry.OpenReplayWorker(ctx, *binary)
	} else {
		client, err = geometry.Open(*address)
		close = func() { _ = client.Close() }
	}
	if err != nil {
		return err
	}
	defer close()
	result, err := workspace.ReplayAssemblyDiagnosticWithOptions(ctx, client, data, options)
	if err != nil {
		return err
	}
	if *output != "" {
		f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(result)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	} else {
		fmt.Println(string(result))
	}
	return nil
}
