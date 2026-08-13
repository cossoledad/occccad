package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateArtifactDirectoryRejectsUnsafeTargets(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{string(filepath.Separator), workingDirectory, filepath.Dir(workingDirectory)} {
		if _, err := validateArtifactDirectory(target); err == nil {
			t.Fatalf("validateArtifactDirectory(%q) succeeded", target)
		}
	}
}

func TestValidateArtifactDirectoryRejectsSymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateArtifactDirectory(link); err == nil {
		t.Fatal("validateArtifactDirectory accepted a symlink")
	}
}

func TestResetArtifactDirectoryClearsAndRecreatesTarget(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "artifacts")
	if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "nested", "object"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resetArtifactDirectory(directory); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("artifact directory contains %d entries after reset", len(entries))
	}
}
