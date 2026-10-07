package geometry

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OpenReplayWorker runs the production numerical service locally. No database,
// S3, BREP or online support resolver participates; all inputs come from the file.
func OpenReplayWorker(ctx context.Context, binary string) (*Client, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	address := listener.Addr().String()
	_ = listener.Close()
	directory, err := os.MkdirTemp("", "occccad-numerical-replay-")
	if err != nil {
		return nil, nil, err
	}
	command := exec.CommandContext(ctx, binary)
	command.Env = append([]string(nil), os.Environ()...)
	replacements := map[string]string{"OCCCCAD_GEOMETRY_WORKER_LISTEN": address, "OCCCCAD_DATA_DIR": directory, "OCCCCAD_LOG_DIR": filepath.Join(directory, "logs"), "OCCCCAD_LOG_LEVEL": "warn"}
	filtered := command.Env[:0]
	for _, entry := range command.Env {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := replacements[key]; !ok {
			filtered = append(filtered, entry)
		}
	}
	command.Env = filtered
	for key, value := range replacements {
		command.Env = append(command.Env, key+"="+value)
	}
	command.Stderr = os.Stderr
	if err = command.Start(); err != nil {
		_ = os.RemoveAll(directory)
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var once sync.Once
	var client *Client
	close := func() {
		once.Do(func() {
			if client != nil {
				_ = client.Close()
			}
			_ = command.Process.Kill()
			<-done
			_ = os.RemoveAll(directory)
		})
	}
	client, err = Open(address)
	if err != nil {
		close()
		return nil, nil, err
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		_, err = client.Ping(pingCtx)
		cancel()
		if err == nil {
			return client, close, nil
		}
		select {
		case <-ctx.Done():
			close()
			return nil, nil, ctx.Err()
		case <-deadline.C:
			close()
			return nil, nil, fmt.Errorf("replay Worker did not start: %w", err)
		case processErr := <-done:
			// Return the consumed completion so the common cleanup can join it.
			done <- processErr
			close()
			return nil, nil, fmt.Errorf("replay Worker exited: %v", processErr)
		case <-ticker.C:
		}
	}
}
