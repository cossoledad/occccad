package control

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestManagedProcessStopsOwnedGroupAndDoesNotSignalCompletedPID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, e := StartManagedProcess(ctx, "owned-fixture", "/bin/sh", t.TempDir(), []string{"-c", "sleep 30 & wait"}, os.Environ())
	if e != nil {
		t.Fatal(e)
	}
	if !p.Running() || p.instance == "" {
		t.Fatal("missing live instance")
	}
	if e = p.Stop(time.Second); e != nil {
		t.Fatal(e)
	}
	if p.Running() {
		t.Fatal("process still running")
	}
	if e = p.Stop(time.Millisecond); e != nil {
		t.Fatal(e)
	}
}
func TestManagedProcessCleansDescendantAfterLeaderExits(t *testing.T) {
	dir := t.TempDir()
	script := "sleep 30 >/dev/null 2>&1 & echo $! > child.pid\n"
	p, e := StartManagedProcess(context.Background(), "short-leader", "/bin/sh", dir, []string{"-c", script}, os.Environ())
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-p.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("leader did not exit")
	}
	if e = p.Stop(time.Second); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "child.pid"))
	if e != nil {
		t.Fatal(e)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	stat, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if e == nil {
		index := strings.LastIndex(string(stat), ")")
		fields := strings.Fields(string(stat)[index+1:])
		if len(fields) > 0 && fields[0] != "Z" {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatal("owned descendant left alive")
		}
	}
}
func TestManagedProcessStartFailureIsExplicit(t *testing.T) {
	if p, e := StartManagedProcess(context.Background(), "missing", "/missing/occccad-executable", t.TempDir(), nil, nil); e == nil || p != nil {
		t.Fatal(p, e)
	}
}
