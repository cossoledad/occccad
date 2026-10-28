package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ManagedProcess struct {
	name     string
	instance string
	command  *exec.Cmd
	done     chan struct{}
	mu       sync.Mutex
	waitErr  error
}

func StartManagedProcess(ctx context.Context, name, executable, workingDirectory string, arguments []string, environment []string) (*ManagedProcess, error) {
	return startManagedProcess(ctx, name, executable, workingDirectory, arguments, environment,
		&logWriter{service: name, level: slog.LevelInfo},
		&logWriter{service: name, level: slog.LevelInfo})
}

// StartManagedProcessPassthrough keeps a native service's own console format.
// The service remains fully lifecycle-managed; only stdout/stderr bypass the
// control process's structured log envelope.
func StartManagedProcessPassthrough(ctx context.Context, name, executable, workingDirectory string, arguments []string, environment []string) (*ManagedProcess, error) {
	return startManagedProcess(ctx, name, executable, workingDirectory, arguments, environment,
		os.Stdout, os.Stderr)
}

func startManagedProcess(ctx context.Context, name, executable, workingDirectory string, arguments []string, environment []string, stdout, stderr io.Writer) (*ManagedProcess, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = workingDirectory
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	instance := hex.EncodeToString(token)
	if environment == nil {
		environment = os.Environ()
	}
	command.Env = []string{}
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "OCCCCAD_PROCESS_INSTANCE=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "OCCCCAD_PROCESS_INSTANCE="+instance)
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 5 * time.Second
	process := &ManagedProcess{name: name, instance: instance, command: command, done: make(chan struct{})}
	started := make(chan error, 1)
	go func() {
		// Linux Pdeathsig follows the creating thread. Keep it alive for the
		// child's lifetime so Go thread retirement cannot terminate a Worker.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := command.Start(); err != nil {
			started <- err
			return
		}
		started <- nil
		err := command.Wait()
		process.signalGroup(syscall.SIGTERM)
		process.mu.Lock()
		process.waitErr = err
		process.mu.Unlock()
		close(process.done)
	}()
	if err := <-started; err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	slog.Info("managed process started", "service", name, "pid", command.Process.Pid)
	return process, nil
}

func (process *ManagedProcess) Stop(timeout time.Duration) error {
	if process == nil || process.command == nil || process.command.Process == nil {
		return nil
	}
	process.signalGroup(syscall.SIGTERM)
	select {
	case <-process.done:
		err := process.Err()
		if err != nil {
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				return err
			}
		}
	case <-time.After(timeout):
		process.signalGroup(syscall.SIGKILL)
		<-process.done
	}
	deadline := time.Now().Add(timeout)
	for process.signalGroup(syscall.SIGTERM) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	process.signalGroup(syscall.SIGKILL)
	slog.Info("managed process stopped", "service", process.name)
	return nil
}

func (process *ManagedProcess) Done() <-chan struct{} { return process.done }
func (process *ManagedProcess) PID() int {
	if process == nil || process.command == nil || process.command.Process == nil {
		return 0
	}
	return process.command.Process.Pid
}
func (process *ManagedProcess) Running() bool {
	if process == nil {
		return false
	}
	select {
	case <-process.done:
		return false
	default:
		return true
	}
}
func (process *ManagedProcess) Err() error {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.waitErr
}

type logWriter struct {
	service string
	level   slog.Level
	mu      sync.Mutex
	pending strings.Builder
}

func (writer *logWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.pending.Write(data)
	text := writer.pending.String()
	index := strings.LastIndex(text, "\n")
	if index < 0 {
		return len(data), nil
	}
	message := config.Redact(text[:index+1])
	writer.pending.Reset()
	writer.pending.WriteString(text[index+1:])
	if writer.level >= slog.LevelError {
		slog.Error("service output", "service", writer.service, "message", message)
	} else {
		slog.Info("service output", "service", writer.service, "message", message)
	}
	return len(data), nil
}

var _ io.Writer = (*logWriter)(nil)

// A process group can outlive its original leader. Confirm the per-launch
// token in /proc before signalling descendants, including after leader exit.
func (process *ManagedProcess) signalGroup(signal syscall.Signal) bool {
	return SignalOwnedProcessGroup(process.command.Process.Pid, process.instance, signal)
}

// SignalOwnedProcessGroup only signals a live group whose members retain the
// per-launch identity. The group may outlive its leader; reused PIDs are rejected.
func SignalOwnedProcessGroup(pid int, instance string, signal syscall.Signal) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	owned := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join("/proc", entry.Name())
		stat, err := os.ReadFile(filepath.Join(p, "stat"))
		if err != nil {
			continue
		}
		index := strings.LastIndex(string(stat), ")")
		if index < 0 {
			continue
		}
		fields := strings.Fields(string(stat)[index+1:])
		if len(fields) < 3 || fields[2] != strconv.Itoa(pid) || fields[0] == "Z" {
			continue
		}
		env, err := os.ReadFile(filepath.Join(p, "environ"))
		if err != nil || !ownsProcess(env, instance) {
			return false
		}
		owned = true
	}
	if owned {
		_ = syscall.Kill(-pid, signal)
	}
	return owned
}

// OwnedProcessGroups finds finite-command descendants that created their own
// process group. The inherited launch identity survives a leader's early exit.
func OwnedProcessGroups(instance string) []int {
	if instance == "" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	groups := map[int]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join("/proc", entry.Name())
		env, err := os.ReadFile(filepath.Join(p, "environ"))
		if err != nil || !ownsProcess(env, instance) {
			continue
		}
		stat, err := os.ReadFile(filepath.Join(p, "stat"))
		if err != nil {
			continue
		}
		end := strings.LastIndex(string(stat), ")")
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(stat)[end+1:])
		if len(fields) < 3 || fields[0] == "Z" {
			continue
		}
		group, err := strconv.Atoi(fields[2])
		if err == nil && group > 1 {
			groups[group] = true
		}
	}
	result := []int{}
	for group := range groups {
		result = append(result, group)
	}
	return result
}

func ownsProcess(env []byte, instance string) bool {
	if instance == "" {
		return false
	}
	text := "\x00" + string(env)
	if strings.Contains(text, "\x00OCCCCAD_PROCESS_INSTANCE="+instance+"\x00") {
		return true
	}
	key := "\x00OCCCCAD_EXECUTION_LINEAGE="
	i := strings.Index(text, key)
	if i < 0 {
		return false
	}
	value := text[i+len(key):]
	if end := strings.IndexByte(value, 0); end >= 0 {
		value = value[:end]
	}
	return strings.Contains(value, ":"+instance+":")
}
