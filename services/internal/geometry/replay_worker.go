package geometry

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"google.golang.org/grpc"
)

// OpenReplayWorker runs the production numerical service locally. No database,
// S3, BREP or online support resolver participates; all inputs come from the file.
func OpenReplayWorker(ctx context.Context, binary string) (*Client, func(), error) {
	if _, err := config.LoadEnvironment(); err != nil {
		return nil, nil, err
	}
	directory, err := os.MkdirTemp(os.Getenv("OCCCCAD_TEST_RESOURCES_DIR"), "numerical-replay-")
	if err != nil {
		return nil, nil, err
	}
	return openLocalWorker(ctx, binary, directory, filepath.Join(directory, "logs"),
		func() { _ = os.RemoveAll(directory) })
}

// OpenDevelopmentWorker uses the application's selected binary, data, logs and
// LOCAL/S3 staging. Its process is scoped to the caller; shared data is retained.
func OpenDevelopmentWorker(ctx context.Context, options ...grpc.DialOption) (*Client, func(), error) {
	if _, err := config.LoadEnvironment(); err != nil {
		return nil, nil, err
	}
	configuration := config.Load()
	store, local, err := artifact.OpenConfigured(ctx, configuration.DataDirectory)
	if err != nil {
		return nil, nil, err
	}
	options = append([]grpc.DialOption{ArtifactStaging(store, local)}, options...)
	return openLocalWorker(ctx, os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"),
		configuration.DataDirectory, configuration.LogDirectory, func() {}, options...)
}

func openLocalWorker(ctx context.Context, binary, directory, logs string, cleanup func(), options ...grpc.DialOption) (*Client, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	address := listener.Addr().String()
	_ = listener.Close()
	identity := "local-" + rand.Text()
	command := exec.CommandContext(ctx, binary)
	command.Env = append([]string(nil), os.Environ()...)
	replacements := map[string]string{"OCCCCAD_GEOMETRY_WORKER_LISTEN": address, "OCCCCAD_DATA_DIR": directory, "OCCCCAD_LOG_DIR": logs, "OCCCCAD_GEOMETRY_WORKER_ID": identity}
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
		cleanup()
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
			cleanup()
		})
	}
	client, err = Open(address, options...)
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
		pong, pingErr := client.Ping(pingCtx)
		err = pingErr
		if pingErr == nil && pong.GetWorkerId() != identity {
			err = fmt.Errorf("local Worker ownership mismatch")
		}
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
