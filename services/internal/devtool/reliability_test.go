package devtool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecutorCancelsOwnedDescendantsEvenAfterLeaderExit(t *testing.T) {
	foreign := exec.Command("/bin/sleep", "60")
	if e := foreign.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { foreign.Process.Kill(); foreign.Wait() }()
	for _, leaderExits := range []bool{false, true} {
		t.Run(fmt.Sprint(leaderExits), func(t *testing.T) {
			a := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.Context = ctx
			pidfile := filepath.Join(a.Root, "child.pid")
			script := `trap '' TERM
 (trap '' TERM; exec sleep 60) >/dev/null 2>&1 &
 echo $! > "$1"
 `
			if leaderExits {
				script += "exit 0\n"
			} else {
				script += "wait\n"
				go func() { time.Sleep(150 * time.Millisecond); cancel() }()
			}
			started := time.Now()
			err := a.execute(a.Root, nil, &bytes.Buffer{}, &bytes.Buffer{}, "/bin/sh", "-c", script, "fixture", pidfile)
			if err == nil {
				t.Fatal("cancellation/orphan returned success")
			}
			if time.Since(started) > 4*time.Second {
				t.Fatal("cleanup not bounded", err)
			}
			b, e := os.ReadFile(pidfile)
			if e != nil {
				t.Fatal(e)
			}
			pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			for i := 0; i < 50; i++ {
				stat, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
				if os.IsNotExist(e) {
					break
				}
				text := string(stat)
				if index := strings.LastIndex(text, ")"); index >= 0 && strings.HasPrefix(strings.TrimSpace(text[index+1:]), "Z ") {
					break
				}
				if i == 49 {
					syscall.Kill(pid, syscall.SIGKILL)
					t.Fatal("owned ignoring descendant remains")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if e := foreign.Process.Signal(syscall.Signal(0)); e != nil {
				t.Fatal("foreign process killed", e)
			}
		})
	}
}
func TestStreamingLogsPersistRedactedProgressBeforeExitAndBoundMemory(t *testing.T) {
	a := fixture(t)
	t.Setenv("OCCCCAD_ADMIN_PASSWORD", "fragment-secret")
	log := filepath.Join(a.Root, "build/live.log")
	live := &cappedBuffer{limit: 1024 * 1024}
	done := make(chan error, 1)
	go func() {
		_, e := a.stream(a.Root, nil, log, live, nil, "/bin/sh", "-c", `printf 'first phase\n'; sleep .3; printf 'fragment-'; sleep .1; printf 'secret postgres://actor:credential@host/db token=abc\n'; sleep .3`)
		done <- e
	}()
	for i := 0; i < 100; i++ {
		b, _ := os.ReadFile(log)
		if strings.Contains(string(b), "first phase") {
			break
		}
		if i == 99 {
			t.Fatal("no streaming progress")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-done:
		t.Fatal("command ended before progress observation")
	default:
	}
	if !strings.Contains(live.String(), "first phase") {
		t.Fatal("verbose not live")
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(log)
	for _, secret := range []string{"fragment-secret", "actor:credential", "token=abc"} {
		if strings.Contains(string(b), secret) || strings.Contains(live.String(), secret) {
			t.Fatal("unscrubbed output", secret)
		}
	}
	tail, e := a.stream(a.Root, nil, log, nil, nil, "/bin/sh", "-c", `i=0; while [ "$i" -lt 2500 ]; do printf 'bounded log memory while preserving complete output 1234567890\n'; i=$((i+1)); done`)
	b, _ = os.ReadFile(log)
	if e != nil || len(tail) > maxLogTail || len(b) < 100000 {
		t.Fatal("output not bounded/complete", len(tail), len(b), e)
	}
	_, e = a.stream(a.Root, nil, log, nil, nil, "head", "-c", strconv.Itoa(maxLogLine+1), "/dev/zero")
	if e == nil {
		t.Fatal("oversize line accepted")
	}
	b, _ = os.ReadFile(log)
	if len(b) > 1024 {
		t.Fatal("oversize raw data persisted")
	}
}
func TestSnapshotReleasesBuildLocksAndBackendIdentityIgnoresDocsAndHMR(t *testing.T) {
	a := fixture(t)
	initGit(t, a)
	diagnosticEnvironment(t, a)
	for _, n := range []string{"occccad-control", "occccad-server", "occccad-jobs"} {
		put(t, a.Root, "build/services/release/"+n, "#!/bin/sh\nexit 0\n")
		os.Chmod(filepath.Join(a.Root, "build/services/release", n), 0755)
	}
	put(t, a.Root, "worker", "#!/bin/sh\nexit 0\n")
	os.Chmod(filepath.Join(a.Root, "worker"), 0755)
	before, _ := a.backendInputs()
	receipt, dir, e := a.snapshotRuntime(Options{Profile: "release"}, "fixture-instance")
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"cpp-release", "go-release"} {
		release, e := tryLock(filepath.Join(a.Root, "build/locks", name+".lock"))
		if e != nil {
			t.Fatal("snapshot retained lock", e)
		}
		release()
	}
	put(t, a.Root, "README.md", "docs changed")
	put(t, a.Root, "web/apps/cad/src/hmr.tsx", "frontend changed")
	put(t, a.Root, "services/internal/control/owner_test.go", "test inputs changed")
	after, _ := a.backendInputs()
	if hashJSON(before) != hashJSON(after) {
		t.Fatal("unrelated inputs invalidate backend")
	}
	put(t, a.Root, "services/internal/control/owner.go", "package control\n")
	after, _ = a.backendInputs()
	if hashJSON(before) == hashJSON(after) {
		t.Fatal("backend change missed")
	}
	originalHash := fileHash(filepath.Join(dir, "occccad_geometry_worker"))
	put(t, a.Root, "worker", "disk replacement")
	if fileHash(filepath.Join(dir, "occccad_geometry_worker")) != originalHash || receipt.Artifacts[filepath.Join(a.Root, "worker")] == fileHash(filepath.Join(a.Root, "worker")) {
		t.Fatal("running snapshot follows disk mutation")
	}
}
func TestMatchDiscoveryOmitsEmptyLeavesAndRequiresPlannedCases(t *testing.T) {
	a := fixture(t)
	os.MkdirAll(filepath.Join(a.Root, "tests/go"), 0700)
	registryFixture(t, a)
	put(t, a.Root, "services/owner/owner_test.go", `package owner; import "testing"; func TestSelected(t *testing.T){}; func TestOther(t *testing.T){}`)
	list := map[string]any{"Dir": filepath.Join(a.Root, "services/owner"), "ImportPath": "fixture/owner", "TestGoFiles": []string{"owner_test.go"}}
	writeJSON(filepath.Join(a.Root, "list.json"), list)
	fakeExecutable(t, a, "go", "case \"$*\" in *empty*) printf '%s\\n' '{\"Dir\":\"/empty\",\"ImportPath\":\"empty\"}';; *) cat "+shellJoin([]string{filepath.Join(a.Root, "list.json")})+";; esac\n")
	p := Plan{Steps: []Step{{ID: "hit", Task: "go:check", Kind: "go", Argv: []string{"--packages", "./owner", "--level", "unit"}}, {ID: "empty", Task: "go:conformance", Kind: "go", Argv: []string{"--packages", "./empty", "--level", "unit"}}}}
	if e := a.resolveMatches(&p, Options{Scope: "services", Level: "unit", Match: "Selected"}); e != nil {
		t.Fatal(e)
	}
	if len(p.Steps) != 1 || len(p.Steps[0].Expected) != 1 || p.Steps[0].Expected[0] != "fixture/owner/TestSelected" {
		t.Fatal(p)
	}
	verdict := Evidence{Status: "PASS", Cases: []CaseResult{{Package: "fixture/owner", Name: "TestOther", Status: "PASS"}}}
	expectedEvidence(p.Steps[0], &verdict, "Selected")
	if verdict.Status != "FAIL" {
		t.Fatal("disappeared case accepted")
	}
	if e := a.resolveMatches(&p, Options{Scope: "services", Level: "unit", Match: "Missing"}); e == nil {
		t.Fatal("zero set accepted")
	}
	verdict = Evidence{Status: "PASS", Cases: []CaseResult{{Package: "fixture", Name: "TestRoot", Status: "PASS"}, {Package: "fixture", Name: "TestRoot/Else", Status: "PASS"}}}
	expectedEvidence(Step{Kind: "go", Expected: []string{"fixture/TestRoot"}}, &verdict, "TestRoot/Absent")
	if verdict.Status != "FAIL" {
		t.Fatal("empty subtest selection accepted")
	}
}
func TestCTestNamingOwnershipAndUnknownRegisteredCaseFails(t *testing.T) {
	a := fixture(t)
	registryFixture(t, a)
	put(t, a.Root, "build/cmake/linux-gcc15-x86_64/release/CTestTestfile.cmake", "# fixture\n")
	data := map[string]any{"tests": []map[string]string{{"name": "worker/NamingArtifact.One"}, {"name": "geometry/Geometry.One"}, {"name": "assembly/Assembly.One"}}}
	writeJSON(filepath.Join(a.Root, "ctest.json"), data)
	fakeExecutable(t, a, "ctest", "cat "+shellJoin([]string{filepath.Join(a.Root, "ctest.json")})+"\n")
	names, e := a.ctestCases(Options{Scope: "geometry", Match: "NamingArtifact", Profile: "release"})
	if e != nil || len(names) != 1 || names[0] != "worker/NamingArtifact.One" {
		t.Fatal(names, e)
	}
	data["tests"] = []map[string]string{{"name": "unowned/Case"}}
	writeJSON(filepath.Join(a.Root, "ctest.json"), data)
	if _, e = a.ctestInventory(Options{Profile: "release"}); e == nil {
		t.Fatal("unowned CTest case hidden")
	}
}
func TestStreamPreservesMachineJSONVerdictsWhenSecretsOverlapNames(t *testing.T) {
	t.Setenv("OCCCCAD_ADMIN_PASSWORD", "pass")
	var file bytes.Buffer
	w := &streamLog{file: &file}
	w.Write([]byte(`{"Action":"pass","Package":"fixture/pass","Test":"TestOne","Output":"password=private"}` + "\n"))
	w.close()
	var event GoEvent
	if e := json.Unmarshal(bytes.TrimSpace(file.Bytes()), &event); e != nil || event.Action != "pass" || event.Package != "fixture/pass" || strings.Contains(event.Output, "private") {
		t.Fatal(file.String(), e)
	}
}

func TestGoRunAlternativesPreserveNativeRootAndSubtestSemantics(t *testing.T) {
	pattern := "^TestRoot$/Wanted$|^TestOther$/Present$|^TestPlain$"
	for _, root := range []string{"TestRoot", "TestOther", "TestPlain"} {
		if ok, e := goRootMatches(pattern, root); e != nil || !ok {
			t.Fatal(root, ok, e)
		}
	}
	for _, name := range []string{"TestRoot/Wanted", "TestOther/Present", "TestPlain"} {
		if !goRunMatches(pattern, name) {
			t.Fatal("lost native alternative", name)
		}
	}
	if goRunMatches(pattern, "TestRoot/Else") {
		t.Fatal("subtest filter ignored")
	}
	e := Evidence{Status: "PASS", Cases: []CaseResult{{Package: "fixture", Name: "TestPlain", Status: "PASS"}, {Package: "fixture", Name: "TestOther", Status: "PASS"}, {Package: "fixture", Name: "TestOther/Else", Status: "PASS"}}}
	expectedEvidence(Step{Kind: "go", Expected: []string{"fixture/TestPlain", "fixture/TestOther"}}, &e, pattern)
	if e.Status != "FAIL" {
		t.Fatal("another alternative hid missing selected subtest")
	}
}

func TestExecutorCleansDetachedOwnedGroupAfterSuccessfulLeader(t *testing.T) {
	a := fixture(t)
	pidfile := filepath.Join(a.Root, "detached.pid")
	err := a.execute(a.Root, nil, &bytes.Buffer{}, &bytes.Buffer{}, "/bin/sh", "-c", `setsid /bin/sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 60' fixture "$1" >/dev/null 2>&1 & sleep .1; exit 0`, "fixture", pidfile)
	if err == nil {
		t.Fatal("detached owned descendant treated as success")
	}
	b, e := os.ReadFile(pidfile)
	if e != nil {
		t.Fatal(e)
	}
	pid := strings.TrimSpace(string(b))
	stat, e := os.ReadFile(filepath.Join("/proc", pid, "stat"))
	if e == nil {
		end := strings.LastIndex(string(stat), ")")
		if end < 0 || !strings.HasPrefix(strings.TrimSpace(string(stat)[end+1:]), "Z ") {
			p, _ := strconv.Atoi(pid)
			syscall.Kill(p, syscall.SIGKILL)
			t.Fatal("detached descendant survived")
		}
	}
}

func TestCTestUnbuiltPlaceholderIsBlockedOnlyInItsOwningScope(t *testing.T) {
	a := fixture(t)
	registryFixture(t, a)
	put(t, a.Root, "build/cmake/linux-gcc15-x86_64/release/CTestTestfile.cmake", "# fixture\n")
	writeJSON(filepath.Join(a.Root, "ctest.json"), map[string]any{"tests": []map[string]string{{"name": "occccad_naming_artifact_tests_NOT_BUILT"}, {"name": "assembly/Existing.Case"}}})
	fakeExecutable(t, a, "ctest", "cat "+shellJoin([]string{filepath.Join(a.Root, "ctest.json")})+"\n")
	names, e := a.ctestCases(Options{Profile: "release", Scope: "assembly"})
	if e != nil || len(names) != 1 {
		t.Fatal("unrelated unbuilt target blocked scoped discovery", names, e)
	}
	if _, e = a.ctestCases(Options{Profile: "release", Scope: "geometry"}); e == nil || !strings.Contains(e.Error(), "--target occccad_naming_artifact_tests") {
		t.Fatal("unbuilt required cases hidden", e)
	}
}

func TestExecutorCleansNestedLaunchIdentityWithoutKillingAncestorOrSibling(t *testing.T) {
	a := fixture(t)
	pidfile := filepath.Join(a.Root, "nested.pid")
	foreign := exec.Command("/bin/sleep", "60")
	if e := foreign.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { foreign.Process.Kill(); foreign.Wait() }()
	err := a.execute(a.Root, nil, &bytes.Buffer{}, &bytes.Buffer{}, "/bin/sh", "-c", `OCCCCAD_PROCESS_INSTANCE=new-nested-identity setsid /bin/sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 60' fixture "$1" >/dev/null 2>&1 & sleep .1; exit 0`, "fixture", pidfile)
	if err == nil {
		t.Fatal("nested command descendant escaped cleanup")
	}
	b, e := os.ReadFile(pidfile)
	if e != nil {
		t.Fatal(e)
	}
	pid := strings.TrimSpace(string(b))
	stat, e := os.ReadFile(filepath.Join("/proc", pid, "stat"))
	if e == nil {
		end := strings.LastIndex(string(stat), ")")
		if end < 0 || !strings.HasPrefix(strings.TrimSpace(string(stat)[end+1:]), "Z ") {
			p, _ := strconv.Atoi(pid)
			syscall.Kill(p, syscall.SIGKILL)
			t.Fatal("nested identity survived outer cleanup")
		}
	}
	if e := foreign.Process.Signal(syscall.Signal(0)); e != nil {
		t.Fatal("sibling/ancestor was signalled", e)
	}
}
