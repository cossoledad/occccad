package devtool

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func diagnosticBytes(p string) ([]byte, error) {
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 8*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(data) > 8*1024*1024 {
		return nil, fmt.Errorf("diagnostic exceeds 8 MiB")
	}
	return data, nil
}
func readDiagnostic(p string) (map[string]any, string, error) {
	data, e := diagnosticBytes(p)
	if e != nil {
		return nil, "", e
	}
	var d map[string]any
	e = json.Unmarshal(data, &d)
	if e != nil || d == nil {
		return nil, "", fmt.Errorf("diagnostic must be a JSON object")
	}
	return d, fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
func resolveDiagnostic(logs, ref string) (string, error) {
	job := strings.Contains(ref, "CAD_JOB_DIAGNOSTIC ")
	m := regexp.MustCompile(`\bCAD_(?:JOB_)?DIAGNOSTIC\s+([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)(?:\s|$|\])`).FindStringSubmatch(ref)
	if m != nil {
		ref = m[1]
	}
	parts := strings.Split(ref, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("expected documentId/diagnosticId")
	}
	for _, s := range parts {
		if !safeID.MatchString(s) || s == "." || s == ".." {
			return "", fmt.Errorf("invalid diagnostic identity")
		}
	}
	dir := "cad-diagnostics"
	if job {
		dir = "job-diagnostics"
	}
	base := filepath.Join(logs, "debug", dir)
	p := filepath.Join(base, parts[0], parts[1]+".json")
	resolved, e := filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	base, e = filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	if !inside(base, resolved) {
		return "", fmt.Errorf("diagnostic path escapes configured archive")
	}
	return resolved, nil
}
func (a *App) Diagnose(o Options) error {
	if e := a.runtimeEnv(o); e != nil {
		return e
	}
	if o.Document != "" || o.Job != "" {
		owner := o.Document
		kind := "CAD_DIAGNOSTIC"
		if o.Job != "" {
			if owner != "" {
				return fmt.Errorf("choose document or job")
			}
			owner = o.Job
			kind = "CAD_JOB_DIAGNOSTIC"
		}
		if !safeID.MatchString(owner) || oneOf(owner, ".", "..") {
			return fmt.Errorf("invalid archive owner identity")
		}
		folder := "cad-diagnostics"
		if o.Job != "" {
			folder = "job-diagnostics"
		}
		paths, e := filepath.Glob(filepath.Join(os.Getenv("OCCCCAD_LOG_DIR"), "debug", folder, owner, "*.meta.json"))
		if e != nil {
			return e
		}
		sort.Sort(sort.Reverse(sort.StringSlice(paths)))
		rows := []any{}
		for _, p := range paths[:min(5, len(paths))] {
			ref := kind + " " + owner + "/" + strings.TrimSuffix(filepath.Base(p), ".meta.json")
			p, e = resolveDiagnostic(os.Getenv("OCCCCAD_LOG_DIR"), ref)
			if e != nil {
				return e
			}
			d, h, e := readDiagnostic(p)
			if e != nil {
				return e
			}
			rows = append(rows, map[string]any{"reference": ref, "stage": d["stage"], "mode": d["mode"], "failure": d["failure"], "sha256": h})
		}
		return a.emit(map[string]any{"schema": "occccad.diagnostic-list.v1", "owner": owner, "entries": rows})
	}
	if (o.Reference == "") == (o.File == "") {
		return fmt.Errorf("choose exactly one --reference or --file")
	}
	p := o.File
	var e error
	if p == "" {
		p, e = resolveDiagnostic(os.Getenv("OCCCCAD_LOG_DIR"), o.Reference)
	} else {
		p, e = filepath.Abs(p)
	}
	if e != nil {
		return e
	}
	d, h, e := readDiagnostic(p)
	if e != nil {
		return e
	}
	schema, _ := d["schema"].(string)
	if !oneOf(schema, "occccad.operation-diagnostic.v1", "occccad.job-diagnostic.v1", "occccad.3dreplay.v1") {
		return fmt.Errorf("unsupported diagnostic schema")
	}
	result := map[string]any{"schema": "occccad.diagnostic-inspection.v1", "source": p, "sha256": h, "boundary": "recorded numerical input only; no domain transaction replay or Head advancement"}
	for _, k := range []string{"documentId", "baseRevisionId", "resultRevisionId", "outcome", "jobId", "operationDiagnosticId", "jobType", "attempt", "inputObjectId", "versionId", "inputDigest", "mode", "stage", "failure"} {
		result[k] = d[k]
	}
	attempts, _ := d["assemblyAttempts"].([]any)
	if schema == "occccad.3dreplay.v1" {
		attempts = []any{d}
	}
	result["recordedAttempts"] = len(attempts)
	missing := []any{}
	for _, k := range []string{"assemblyMissing", "metadataMissing", "missing"} {
		if v, ok := d[k]; ok {
			arr, ok := v.([]any)
			if !ok {
				return fmt.Errorf("invalid missing-input evidence")
			}
			missing = append(missing, arr...)
		}
	}
	result["missing"] = missing
	if len(attempts) == 0 {
		result["replayStatus"] = "NO_RECORDED_NUMERICAL_ATTEMPT"
		if o.Replay {
			return fmt.Errorf("no recorded numerical attempt; full operation replay unavailable")
		}
		return a.emit(result)
	}
	if o.Attempt < 0 || o.Attempt >= len(attempts) {
		return fmt.Errorf("recorded attempt index out of range")
	}
	recorded, ok := attempts[o.Attempt].(map[string]any)
	if !ok || recorded["schema"] != "occccad.3dreplay.v1" || (recorded["request"] == nil && recorded["snapshot"] == nil) {
		return fmt.Errorf("recorded numerical input missing or unsupported")
	}
	selectionRequired := false
	if snapshot, ok := recorded["snapshot"].(map[string]any); ok {
		ids := []string{}
		if rows, ok := snapshot["attempts"].([]any); ok {
			for _, row := range rows {
				if attempt, ok := row.(map[string]any); ok {
					if id, ok := attempt["diagnosticId"].(string); ok && id != "" {
						ids = append(ids, id)
					}
				}
			}
		}
		result["availableDiagnosticIds"] = ids
		selectionRequired = o.Diagnostic == ""
		if !selectionRequired && !oneOf(o.Diagnostic, ids...) {
			return fmt.Errorf("diagnostic ID is not a recorded original attempt")
		}
	} else if o.Diagnostic != "" {
		return fmt.Errorf("--diagnostic requires a complete snapshot with recorded original attempts")
	}
	// Snapshot digests cover RawMessage members, including their key order and
	// exact numeric tokens. Never round-trip retained input through interface{}.
	raw, e := diagnosticBytes(p)
	if e != nil {
		return e
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != h {
		return fmt.Errorf("diagnostic source changed during inspection")
	}
	var retained json.RawMessage = raw
	if schema != "occccad.3dreplay.v1" {
		var object map[string]json.RawMessage
		var rows []json.RawMessage
		if json.Unmarshal(raw, &object) != nil || json.Unmarshal(object["assemblyAttempts"], &rows) != nil || o.Attempt >= len(rows) {
			return fmt.Errorf("recorded numerical attempt changed")
		}
		retained = rows[o.Attempt]
	}
	var compactRetained bytes.Buffer
	if e = json.Compact(&compactRetained, retained); e != nil {
		return e
	}
	dir := filepath.Join(a.Root, "build/diagnostics", h[:20], strconv.Itoa(o.Attempt))
	input := filepath.Join(dir, "input.3dreplay")
	if b, e := os.ReadFile(input); e == nil {
		var existing bytes.Buffer
		if json.Compact(&existing, b) != nil || !bytes.Equal(existing.Bytes(), compactRetained.Bytes()) {
			return fmt.Errorf("derived immutable replay input changed")
		}
	} else {
		if e = writeJSON(input, retained); e != nil {
			return e
		}
	}
	output := filepath.Join(dir, "result-"+time.Now().Format("20060102T150405.000000000")+".3dreplay")
	argv := []string{filepath.Join(a.Root, "build/services", o.Profile, "occccad-3dreplay"), "-worker-binary", os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"), "-mode", "original", "-out", output, input}
	if o.Diagnostic != "" {
		argv = append(argv[:len(argv)-1], "-diagnostic", o.Diagnostic, input)
	}
	result["replayStatus"] = "AVAILABLE"
	if selectionRequired {
		result["replayStatus"] = "SELECTION_REQUIRED"
	}
	result["input"] = input
	result["command"] = argv
	if o.Replay {
		if selectionRequired {
			a.emit(result)
			return fmt.Errorf("original snapshot replay requires --diagnostic from availableDiagnosticIds")
		}
		hold, e := a.buildLock(o, true)
		if e != nil {
			return e
		}
		defer hold()
		out, err := a.capture(a.services(), nil, argv...)
		log := output + ".log"
		if e = os.WriteFile(log, []byte(redact(out)), 0600); e != nil {
			return e
		}
		result["log"] = log
		result["output"] = output
		result["replayStatus"] = "EXECUTED"
		if err != nil {
			result["replayStatus"] = "EXECUTION_FAILED"
			result["executionError"] = strings.TrimSpace(compact(redact(out)))
		}
		if observed, _, e := readDiagnostic(output); e == nil {
			result["observedOutcome"] = observed["outcome"]
		}
		if err != nil {
			a.emit(result)
			return fmt.Errorf("numerical replay failed; log: %s: %w", log, err)
		}
	}
	return a.emit(result)
}
