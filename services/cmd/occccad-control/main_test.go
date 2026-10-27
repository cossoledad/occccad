package main

import (
	"path/filepath"
	"testing"
)

func TestResolveDataDirectoryUsesManagedServiceWorkingDirectory(t *testing.T) {
	services := filepath.Join(string(filepath.Separator), "workspace", "services")
	if got, want := resolveDataDirectory(services, "./data"), filepath.Join(services, "data"); got != want {
		t.Fatalf("relative data directory resolved to %q, want %q", got, want)
	}
	absolute := filepath.Join(string(filepath.Separator), "var", "lib", "occccad")
	if got := resolveDataDirectory(services, absolute); got != absolute {
		t.Fatalf("absolute data directory changed to %q", got)
	}
}

func TestManagementRequiresLoopback(t *testing.T) {
	for _, bad := range []string{"0.0.0.0:19090", "example.org:19090", "localhost:19090", "[::]:19090", ""} {
		if requireLoopback(bad) == nil {
			t.Fatalf("accepted non-loopback %q", bad)
		}
	}
	for _, good := range []string{"127.0.0.1:19090", "[::1]:19090"} {
		if err := requireLoopback(good); err != nil {
			t.Fatal(err)
		}
	}
}
