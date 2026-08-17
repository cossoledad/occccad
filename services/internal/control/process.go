package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type ManagedProcess struct {
	name    string
	command *exec.Cmd
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
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
	command.Env = environment
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	process := &ManagedProcess{name: name, command: command, done: make(chan struct{})}
	go func() {
		err := command.Wait()
		process.mu.Lock()
		process.waitErr = err
		process.mu.Unlock()
		close(process.done)
	}()
	slog.Info("managed process started", "service", name, "pid", command.Process.Pid)
	return process, nil
}

func (process *ManagedProcess) Stop(timeout time.Duration) error {
	if process == nil || process.command == nil || process.command.Process == nil {
		return nil
	}
	_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGTERM)
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
		_ = syscall.Kill(-process.command.Process.Pid, syscall.SIGKILL)
		<-process.done
	}
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
}

func (writer *logWriter) Write(data []byte) (int, error) {
	message := string(data)
	if writer.level >= slog.LevelError {
		slog.Error("service output", "service", writer.service, "message", message)
	} else {
		slog.Info("service output", "service", writer.service, "message", message)
	}
	return len(data), nil
}

var _ io.Writer = (*logWriter)(nil)
