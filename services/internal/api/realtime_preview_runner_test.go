package api

import (
	"testing"
	"time"
)

func TestPreviewRunnerRetainsOnlyLatestPending(t *testing.T) {
	client := &realtimeClient{previewSlots: make(chan struct{}, 1)}
	client.previewSlots <- struct{}{}
	started, release, latest := make(chan struct{}), make(chan struct{}), make(chan struct{})
	runner := &previewRunner{running: true, pending: func() { close(started); <-release }}
	go client.runPreviewRunner(runner)
	<-started
	client.mu.Lock()
	runner.pending = func() { t.Error("superseded pending executed") }
	runner.pending = func() { close(latest) }
	client.mu.Unlock()
	close(release)
	select {
	case <-latest:
	case <-time.After(time.Second):
		t.Fatal("latest input was not evaluated")
	}
	deadline := time.Now().Add(time.Second)
	for {
		client.mu.Lock()
		stopped := !runner.running && len(client.previewSlots) == 0
		client.mu.Unlock()
		if stopped {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runner did not release capacity")
		}
		time.Sleep(time.Millisecond)
	}
}
