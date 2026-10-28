package devtool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type Blocked struct{ Reason string }

func (e *Blocked) Error() string { return "ENVIRONMENT_BLOCKED: " + e.Reason }
func blocked(s string) *Blocked  { return &Blocked{s} }
func (a *App) capture(cwd string, env []string, argv ...string) (string, error) {
	b := &cappedBuffer{limit: 16 * 1024 * 1024}
	err := a.execute(cwd, env, b, b, argv...)
	if b.exceeded {
		return b.String(), fmt.Errorf("machine output exceeds 16 MiB")
	}
	return b.String(), err
}

// Redaction happens before output reaches disk or the terminal. Scrub both
// configured secrets and unknown URL credentials/tokens in child diagnostics.
var urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+(?::[^\s/@]*)?@`)
var tokenField = regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key)["']?\s*[:=]\s*["']?)[^\s,"'}]+`)

func redact(s string) string {
	// Scrub labelled values before configured short secrets can alter labels
	// such as "password", making them unrecognizable to the field scrubber.
	s = tokenField.ReplaceAllString(s, "${1}<redacted>")
	s = urlCredentials.ReplaceAllString(s, "${1}<redacted>@")
	return config.Redact(s)
}
func (a *App) native(cwd string, verbose bool, argv ...string) error {
	return a.nativeEnv(cwd, nil, verbose, argv...)
}
func (a *App) nativeEnv(cwd string, env []string, verbose bool, argv ...string) error {
	a.recordNative(cwd, argv)
	log := a.logPath(filepath.Base(argv[0]))
	fmt.Fprintf(a.Err, "[run] %s; log: %s\n", filepath.Base(argv[0]), log)
	var live io.Writer
	if verbose {
		live = a.Err
	}
	out, err := a.stream(cwd, env, log, live, nil, argv...)
	if err != nil {
		fmt.Fprintln(a.Err, compact(out))
		return fmt.Errorf("%s: %w; log: %s", argv[0], err, log)
	}
	fmt.Fprintf(a.Err, "%s completed; log: %s\n", argv[0], log)
	return nil
}

func compact(s string) string {
	const max = 16000
	if len(s) > max {
		return strings.ToValidUTF8(s[:7000]+"\n... see full log ...\n"+s[len(s)-8000:], "")
	}
	return s
}
func fileHash(p string) string {
	f, e := os.Open(p)
	if e != nil {
		return "MISSING"
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "UNREADABLE"
	}
	return hex.EncodeToString(h.Sum(nil))
}
func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func writeJSON(p string, v any) error {
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(p, append(b, '\n'), 0600)
}
func loadJSON(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}

// Locks are outside their protected directories. Cancellation releases the
// descriptor, never unlinks a lock another process could still hold.
func lock(ctx context.Context, p string, shared bool) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return nil, err
	}
	f, e := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	op := syscall.LOCK_EX
	if shared {
		op = syscall.LOCK_SH
	}
	started := time.Now()
	announced := false
	for {
		e = syscall.Flock(int(f.Fd()), op|syscall.LOCK_NB)
		if e == nil {
			return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		if e != syscall.EWOULDBLOCK {
			f.Close()
			return nil, e
		}
		if !announced {
			fmt.Fprintf(os.Stderr, "[lock] waiting for %s (%s): another build/clean or validation owns this path; finish or cancel that command, then retry. dev runs from a snapshot and does not hold build locks.\n", p, map[bool]string{true: "reader", false: "writer"}[shared])
			announced = true
		}
		if time.Since(started) >= 15*time.Second {
			f.Close()
			return nil, blocked("lock busy after 15s: " + p + "; finish/cancel the build or validation holding it, then retry")
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func (a *App) buildLock(o Options, shared bool) (func(), error) {
	return lock(a.Context, filepath.Join(a.Root, "build/locks/cpp-"+o.Profile+".lock"), shared)
}

func (a *App) raw(cwd string, argv ...string) error { return a.rawEnv(cwd, nil, argv...) }
func (a *App) rawEnv(cwd string, env []string, argv ...string) error {
	a.recordNative(cwd, argv)
	log := a.logPath(filepath.Base(argv[0]))
	fmt.Fprintf(a.Err, "[run] %s; log: %s\n", filepath.Base(argv[0]), log)
	_, err := a.stream(cwd, env, log, a.Out, nil, argv...)
	return err
}

func (a *App) recordNative(cwd string, argv []string) {
	b, _ := json.Marshal(map[string]any{"cwd": cwd, "argv": argv})
	fmt.Fprintln(a.Err, "OCCCCAD_NATIVE_COMMAND "+string(b))
}
func offlineGo() []string {
	env := []string{}
	for _, s := range os.Environ() {
		if !strings.HasPrefix(s, "GOPROXY=") {
			env = append(env, s)
		}
	}
	return append(env, "GOPROXY=off")
}
