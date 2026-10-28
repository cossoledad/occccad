package devtool

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/occccad/occccad/internal/control"
)

// One command, one owned process group. This is also used for short machine
// queries; lifecycle ownership and cancellation never depend on output pipes.
func (a *App) execute(cwd string, env []string, stdout, stderr io.Writer, argv ...string) error {
	if e := a.Context.Err(); e != nil {
		return e
	}
	token := make([]byte, 16)
	if _, e := rand.Read(token); e != nil {
		return e
	}
	instance := hex.EncodeToString(token)
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = cwd
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.WaitDelay = time.Second
	if env == nil {
		env = os.Environ()
	}
	c.Env = []string{}
	lineage := ""
	for _, s := range env {
		if strings.HasPrefix(s, "OCCCCAD_EXECUTION_LINEAGE=") {
			lineage = strings.TrimPrefix(s, "OCCCCAD_EXECUTION_LINEAGE=")
			continue
		}
		if !strings.HasPrefix(s, "OCCCCAD_PROCESS_INSTANCE=") {
			c.Env = append(c.Env, s)
		}
	}
	c.Env = append(c.Env, "OCCCCAD_PROCESS_INSTANCE="+instance)
	// Nested leaf tasks get their own launch identity, while retaining the
	// ancestor chain so cancelling the outer check can clean every owned group.
	if lineage == "" {
		lineage = ":"
	}
	if !strings.HasSuffix(lineage, ":") {
		lineage += ":"
	}
	c.Env = append(c.Env, "OCCCCAD_EXECUTION_LINEAGE="+lineage+instance+":")
	c.Stdout = stdout
	c.Stderr = stderr
	if e := c.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	grace := 750 * time.Millisecond
	if filepath.Base(argv[0]) == "occccad-control" {
		grace = 15 * time.Second
	}
	var err error
	select {
	case err = <-done:
	case <-a.Context.Done():
		control.SignalOwnedProcessGroup(c.Process.Pid, instance, syscall.SIGTERM)
		select {
		case err = <-done:
		case <-time.After(grace):
			control.SignalOwnedProcessGroup(c.Process.Pid, instance, syscall.SIGKILL)
			// A group containing an unverified member must not be signalled as a
			// whole. The Process handle still identifies our own live leader.
			_ = c.Process.Kill()
			select {
			case err = <-done:
			case <-time.After(2 * time.Second):
				return fmt.Errorf("owned command did not stop after cancellation; verify process identity/permissions")
			}
		}
	}
	// A successful leader may leave descendants with closed/redirected pipes.
	// Confirm identity even after Wait, and escalate if descendants ignore TERM.
	orphaned := false
	groups := control.OwnedProcessGroups(instance)
	if len(groups) > 0 {
		orphaned = true
		signal := func(sig syscall.Signal) bool {
			owned := false
			for _, group := range control.OwnedProcessGroups(instance) {
				if control.SignalOwnedProcessGroup(group, instance, sig) {
					owned = true
				}
			}
			return owned
		}
		signal(syscall.SIGTERM)
		deadline := time.Now().Add(750 * time.Millisecond)
		for time.Now().Before(deadline) && signal(syscall.SIGTERM) {
			time.Sleep(25 * time.Millisecond)
		}
		signal(syscall.SIGKILL)
		// Signal delivery is asynchronous. Wait for live owned members to leave
		// /proc before reporting cleanup complete; zombies have no listeners.
		deadline = time.Now().Add(time.Second)
		for len(control.OwnedProcessGroups(instance)) > 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if len(control.OwnedProcessGroups(instance)) > 0 {
			return fmt.Errorf("owned process cleanup incomplete after SIGKILL")
		}
	}
	if e := a.Context.Err(); e != nil {
		return e
	}
	if err == nil && orphaned {
		return fmt.Errorf("command left owned descendants; process group cleaned")
	}
	return err
}

type cappedBuffer struct {
	sync.Mutex
	data     []byte
	limit    int
	exceeded bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	if len(p) > b.limit-len(b.data) {
		b.exceeded = true
		return 0, fmt.Errorf("machine output exceeds %d bytes", b.limit)
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *cappedBuffer) String() string { b.Lock(); defer b.Unlock(); return string(b.data) }

const maxLogLine = 1024 * 1024
const maxLogTail = 32 * 1024

// A line is held only until its newline so split writes cannot reveal a secret.
// Oversize lines fail explicitly instead of persisting a partially scrubbed value.
type streamLog struct {
	sync.Mutex
	file    io.Writer
	live    io.Writer
	observe func(string)
	pending []byte
	tail    []byte
	err     error
	discard bool
	cancel  context.CancelFunc
}

func safeLogLine(line string) string {
	var event map[string]json.RawMessage
	if json.Unmarshal([]byte(line), &event) == nil && event["Action"] != nil {
		return strings.TrimSuffix(redactGoOutput(line), "\n")
	}
	return redact(line)
}
func (s *streamLog) line(raw string) {
	if s.observe != nil {
		s.observe(raw)
	}
	clean := safeLogLine(raw) + "\n"
	if _, e := io.WriteString(s.file, clean); e != nil && s.err == nil {
		s.err = e
		if s.cancel != nil {
			s.cancel()
		}
	}
	if s.live != nil {
		if _, e := io.WriteString(s.live, clean); e != nil && s.err == nil {
			s.err = e
			if s.cancel != nil {
				s.cancel()
			}
		}
	}
	s.tail = append(s.tail, clean...)
	if len(s.tail) > maxLogTail {
		s.tail = append([]byte(nil), s.tail[len(s.tail)-maxLogTail:]...)
	}
}
func (s *streamLog) Write(p []byte) (int, error) {
	s.Lock()
	defer s.Unlock()
	for _, b := range p {
		if b == '\n' || b == '\r' {
			if !s.discard {
				s.line(string(s.pending))
			}
			s.pending = s.pending[:0]
			s.discard = false
			continue
		}
		if s.discard {
			continue
		}
		if len(s.pending) == maxLogLine {
			s.pending = nil
			s.discard = true
			s.line("[output line exceeds 1 MiB; omitted before persistence]")
			if s.err == nil {
				s.err = fmt.Errorf("output line exceeds 1 MiB")
				if s.cancel != nil {
					s.cancel()
				}
			}
			continue
		}
		s.pending = append(s.pending, b)
	}
	return len(p), nil
}
func (s *streamLog) close() {
	s.Lock()
	defer s.Unlock()
	if len(s.pending) > 0 {
		s.line(string(s.pending))
		s.pending = nil
	}
}

func (a *App) stream(cwd string, env []string, log string, live io.Writer, observe func(string), argv ...string) (string, error) {
	if e := os.MkdirAll(filepath.Dir(log), 0700); e != nil {
		return "", e
	}
	f, e := os.OpenFile(log, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	defer f.Close()
	ctx, cancel := context.WithCancel(a.Context)
	defer cancel()
	runner := *a
	runner.Context = ctx
	writer := &streamLog{file: f, live: live, observe: observe, cancel: cancel}
	finished := make(chan struct{})
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-finished:
				return
			case <-ticker.C:
				fmt.Fprintf(a.Err, "[running] %s; log: %s\n", filepath.Base(argv[0]), log)
			}
		}
	}()
	err := runner.execute(cwd, env, writer, writer, argv...)
	close(finished)
	<-progressDone
	writer.close()
	if writer.err != nil {
		err = writer.err
	}
	return string(writer.tail), err
}
func (a *App) logPath(name string) string {
	return filepath.Join(a.Root, "build/logs", name+"-"+time.Now().UTC().Format("20060102T150405.000000000")+".log")
}

// Scan a bounded machine query without retaining line-sized scanner buffers.
func scanLines(raw string, visit func(string)) {
	s := bufio.NewScanner(strings.NewReader(raw))
	s.Buffer(make([]byte, 4096), 16*1024*1024)
	for s.Scan() {
		visit(s.Text())
	}
}
