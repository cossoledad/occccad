package devtool

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type StepResult struct {
	Step
	Evidence
	ArtifactsBefore map[string]string `json:"artifactsBefore,omitempty"`
	ArtifactsAfter  map[string]string `json:"artifactsAfter,omitempty"`
	Environment     map[string]any    `json:"environment,omitempty"`
	NativeCommands  []map[string]any  `json:"nativeCommands,omitempty"`
	NativeArgv      []string          `json:"nativeArgv,omitempty"`
	ExitCode        int               `json:"exitCode"`
	Duration        float64           `json:"durationSeconds"`
	Log             string            `json:"log,omitempty"`
	Reproduce       string            `json:"reproduce"`
}
type Report struct {
	Schema      string       `json:"schema"`
	Status      string       `json:"status"`
	Plan        Plan         `json:"selection"`
	SourceAfter any          `json:"sourceAfter"`
	Build       any          `json:"build"`
	Duration    float64      `json:"durationSeconds"`
	Steps       []StepResult `json:"steps"`
	Artifact    string       `json:"artifact"`
}

func (a *App) Validate(o Options, planOnly bool) error {
	// Ninja owns test discovery outputs too. A C++ check refreshes its native
	// targets before selecting cases; read-only plan uses existing registration.
	deferCTest := !planOnly && o.Level == "unit" && oneOf(o.Scope, "assembly", "geometry", "sketch") && o.Match != ""
	p, e := a.makePlan(o, !deferCTest)
	if e != nil {
		return e
	}
	if planOnly {
		if o.JSON {
			return a.emit(p)
		}
		fmt.Fprintf(a.Out, "Plan scopes=%v level=%s (%d steps)\n", p.Scopes, p.Level, len(p.Steps))
		for _, s := range p.Steps {
			fmt.Fprintln(a.Out, s.Argv)
		}
		fmt.Fprintln(a.Out, "Outstanding evidence:", p.Outstanding)
		return nil
	}
	var resourceRelease func()
	if o.Level != "unit" {
		resourceRelease, e = lock(a.Context, filepath.Join(a.Root, "build/locks/resources.lock"), true)
		if e != nil {
			return e
		}
		defer resourceRelease()
	}
	start := time.Now()
	dir := filepath.Join(a.Root, "build/agent-check", start.UTC().Format("20060102T150405.000000000"))
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	report := Report{Schema: "occccad.validation-run.v2", Status: "NOT_RUN", Plan: p, Steps: []StepResult{}, Artifact: filepath.Join(dir, "report.json"), Build: map[string]any{"profile": o.Profile, "jobs": o.Jobs, "testJobs": o.TestJobs, "dependencyReceipt": fileHash(filepath.Join(a.buildDir(o), "dependencies.json"))}}
	failed := false
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	for i, step := range p.Steps {
		rec := StepResult{Step: step, Evidence: Evidence{Status: "NOT_RUN"}, Reproduce: shellJoin(step.Argv)}
		if failed {
			rec.Reason = "previous required step failed or was blocked"
			report.Steps = append(report.Steps, rec)
			continue
		}
		xml := filepath.Join(dir, fmt.Sprintf("%02d.xml", i))
		if step.Kind == "ctest" {
			rec.Argv = append(rec.Argv, "--report", xml)
		}
		if release == nil && (step.Kind == "ctest" || oneOf("worker", step.Resources...)) {
			release, e = a.buildLock(o, true)
			if e != nil {
				return e
			}
		}
		if deferCTest && step.Kind == "ctest" {
			local := o
			local.Scope = argument(step.Argv, "--scope")
			names, err := a.ctestCases(local)
			if err == nil && len(names) == 0 {
				err = fmt.Errorf("selected zero CTest cases for %q", o.Match)
			}
			if err != nil {
				rec.Status = "FAIL"
				rec.Reason = err.Error()
				rec.Log = filepath.Join(dir, fmt.Sprintf("%02d-%s.log", i, step.ID))
				if e = os.WriteFile(rec.Log, []byte(redact(rec.Reason)), 0600); e != nil {
					return e
				}
				report.Steps = append(report.Steps, rec)
				failed = true
				report.Status = rec.Status
				fmt.Fprintf(a.Err, "[%s] %s: %s; log: %s\n", rec.Status, step.ID, rec.Reason, rec.Log)
				continue
			}
			step.Expected = names
			rec.Expected = names
			report.Plan.Steps[i] = step
		}
		if e := a.Preflight(o, step.Resources); e != nil {
			rec.Status = "ENVIRONMENT_BLOCKED"
			rec.Reason = e.Error()
			rec.Log = filepath.Join(dir, fmt.Sprintf("%02d-%s.log", i, step.ID))
			if err := os.WriteFile(rec.Log, []byte(redact(e.Error())), 0600); err != nil {
				return err
			}
			report.Steps = append(report.Steps, rec)
			report.Status = rec.Status
			failed = true
			fmt.Fprintln(a.Err, rec.Reason, "log:", rec.Log)
			continue
		}
		if len(step.Resources) > 0 {
			rec.Environment = environmentIdentity()
		}
		if step.Kind != "build" {
			rec.ArtifactsBefore = a.stepArtifacts(o, step)
		}
		rec.Log = filepath.Join(dir, fmt.Sprintf("%02d-%s.log", i, step.ID))
		fmt.Fprintf(a.Err, "[run] %s; log: %s\n", step.ID, rec.Log)
		webCases := []CaseResult{}
		var protocolError error
		observe := func(line string) {
			if strings.HasPrefix(line, "OCCCCAD_NATIVE_COMMAND ") {
				if len(rec.NativeCommands) >= 1000 {
					protocolError = fmt.Errorf("native command count exceeds 1000")
					return
				}
				var command map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "OCCCCAD_NATIVE_COMMAND ")), &command) == nil {
					rec.NativeCommands = append(rec.NativeCommands, command)
				}
			}
			if strings.HasPrefix(line, "OCCCCAD_WEB_CASE ") {
				if len(webCases) >= 20000 {
					protocolError = fmt.Errorf("Web case count exceeds 20000")
					return
				}
				webCases = append(webCases, CaseResult{Name: strings.TrimPrefix(line, "OCCCCAD_WEB_CASE "), Status: "PASS"})
			}
		}
		var live io.Writer
		if o.Verbose {
			live = a.Err
		}
		before := time.Now()
		out, err := a.stream(rec.Cwd, append(os.Environ(), "OCCCCAD_DEV_EVIDENCE_STREAM=1"), rec.Log, live, observe, rec.Argv...)
		if err == nil {
			err = protocolError
		}
		if err != nil {
			rec.ExitCode = -1
			if code, ok := err.(*exec.ExitError); ok {
				rec.ExitCode = code.ExitCode()
			}
		}
		rec.Duration = time.Since(before).Seconds()
		switch step.Kind {
		case "go":
			rec.Evidence = GoEvidenceFile(rec.Log, err != nil, o.Match != "")
		case "ctest":
			rec.Evidence = CTestEvidence(xml, err != nil)
		case "node":
			rec.Evidence = NodeEvidence(out, err != nil)
		case "web":
			rec.Evidence = WebEvidence(out, err != nil)
			rec.Cases = webCases
		case "browser":
			rec.Evidence = browserEvidence(out, err != nil)
		default:
			rec.Status = "PASS"
			if err != nil {
				rec.Status = "FAIL"
			}
		}
		if err != nil && rec.Counts.Failed == 0 && (strings.Contains(out, "\nENVIRONMENT_BLOCKED:") || strings.HasPrefix(out, "ENVIRONMENT_BLOCKED:")) {
			rec.Status = "ENVIRONMENT_BLOCKED"
		}
		if len(step.Expected) > 0 {
			expectedEvidence(step, &rec.Evidence, o.Match)
		}
		if step.Task == "go:check" || step.Task == "go:conformance" {
			leaf := o
			for j := 4; j+1 < len(rec.Argv); j++ {
				if rec.Argv[j] == "--packages" {
					leaf.Packages = rec.Argv[j+1]
				}
				if rec.Argv[j] == "--level" {
					leaf.Level = rec.Argv[j+1]
				}
			}
			rec.NativeArgv, _ = goArgs(leaf)
		}
		if len(step.Resources) > 0 && hashJSON(rec.Environment) != hashJSON(environmentIdentity()) {
			rec.Status = "SOURCE_CHANGED"
			rec.Reason = "selected runtime configuration changed during validation"
		}
		if step.Kind != "build" {
			rec.ArtifactsAfter = a.stepArtifacts(o, step)
			if hashJSON(rec.ArtifactsBefore) != hashJSON(rec.ArtifactsAfter) {
				rec.Status = "SOURCE_CHANGED"
				rec.Reason = "related generated sources or native artifacts changed during execution"
			}
		}
		fmt.Fprintf(a.Err, "[%s] %s (%.2fs; passed=%d skipped=%d); log: %s\n", rec.Status, step.ID, rec.Duration, rec.Counts.Passed, rec.Counts.Skipped, rec.Log)
		if rec.Status != "PASS" && rec.Reason != "" {
			fmt.Fprintln(a.Err, "  reason:", redact(rec.Reason))
		}
		if !o.Verbose && rec.Status != "PASS" {
			fmt.Fprintln(a.Err, compact(redact(out)))
		}
		report.Steps = append(report.Steps, rec)
		if rec.Status != "PASS" {
			failed = true
			report.Status = rec.Status
		}
	}
	if len(p.Steps) > 0 && !failed {
		report.Status = "PASS"
	}
	report.SourceAfter, e = a.Source()
	if e != nil {
		return e
	}
	if hashJSON(p.Source) != hashJSON(report.SourceAfter) {
		report.Status = "SOURCE_CHANGED"
		failed = true
	}
	report.Duration = time.Since(start).Seconds()
	if e = writeJSON(report.Artifact, report); e != nil {
		return e
	}
	if o.JSON {
		if e = a.emit(report); e != nil {
			return e
		}
	} else {
		fmt.Fprintf(a.Out, "%s: requested %s scope; outstanding=%v; report: %s\n", report.Status, o.Level, p.Outstanding, report.Artifact)
	}
	if failed {
		return fmt.Errorf("validation %s; report: %s", report.Status, report.Artifact)
	}
	return nil
}
func browserEvidence(s string, failed bool) Evidence {
	r := Evidence{Status: "FAIL", Reason: "missing browser execution counts"}
	if failed {
		return r
	}
	for _, line := range strings.Split(s, "\n") {
		var n int
		if _, e := fmt.Sscanf(strings.TrimSpace(line), "%d passed", &n); e == nil && n > 0 {
			r.Status = "PASS"
			r.Reason = ""
			r.Counts.Started = n
			r.Counts.Passed = n
		}
	}
	return r
}
func shellJoin(args []string) string {
	r := []string{}
	for _, s := range args {
		r = append(r, "'"+strings.ReplaceAll(s, "'", "'\"'\"'")+"'")
	}
	return strings.Join(r, " ")
}
