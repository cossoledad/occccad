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
