package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocateScriptUnderRootsPrefersFirstMatch(t *testing.T) {
	tmp := t.TempDir()
	rootA := filepath.Join(tmp, "a")
	rootB := filepath.Join(tmp, "b")
	if err := os.MkdirAll(filepath.Join(rootA, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir rootA: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rootB, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir rootB: %v", err)
	}

	pathA := filepath.Join(rootA, "scripts", "build-core.sh")
	pathB := filepath.Join(rootB, "scripts", "build-core.sh")
	if err := os.WriteFile(pathA, []byte("#!/usr/bin/env bash\n"), 0o644); err != nil {
		t.Fatalf("write pathA: %v", err)
	}
	if err := os.WriteFile(pathB, []byte("#!/usr/bin/env bash\n"), 0o644); err != nil {
		t.Fatalf("write pathB: %v", err)
	}

	got := locateScriptUnderRoots([]string{rootA, rootB}, "scripts/build-core.sh")
	if got != pathA {
		t.Fatalf("locateScriptUnderRoots()=%q, want %q", got, pathA)
	}
}

func TestFindManagedScriptPathUsesConfiguredRuntimeDir(t *testing.T) {
	tmp := t.TempDir()
	runtimeRoot := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir runtime scripts: %v", err)
	}
	expected := filepath.Join(runtimeRoot, "scripts", "build-core.sh")
	if err := os.WriteFile(expected, []byte("#!/usr/bin/env bash\n"), 0o644); err != nil {
		t.Fatalf("write build script: %v", err)
	}

	otherCWD := filepath.Join(tmp, "cwd")
	if err := os.MkdirAll(otherCWD, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	previousCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(otherCWD); err != nil {
		t.Fatalf("chdir to other cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousCWD)
	})

	t.Setenv(omniRuntimeDirEnv, runtimeRoot)
	got, err := findManagedScriptPath("scripts/build-core.sh")
	if err != nil {
		t.Fatalf("findManagedScriptPath returned error: %v", err)
	}
	if got != expected {
		t.Fatalf("findManagedScriptPath()=%q, want %q", got, expected)
	}
}

func TestFindManagedScriptPathReturnsErrorWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(omniRuntimeDirEnv, filepath.Join(tmp, "missing-root"))

	previousCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousCWD)
	})

	if _, err := findManagedScriptPath("update.sh"); err == nil {
		t.Fatalf("expected error when managed script cannot be found")
	}
}

func TestResolveUpdateArgsAutoPrefixesRepoCWD(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteFile := func(path string) {
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWriteFile(filepath.Join(repoRoot, "go.mod"))
	mustWriteFile(filepath.Join(repoRoot, "docker-compose.yml"))
	mustWriteFile(filepath.Join(repoRoot, "update.sh"))
	if err := os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})

	got := resolveUpdateArgs([]string{"--no-build"})
	if len(got) < 3 {
		t.Fatalf("resolveUpdateArgs len=%d, want >=3", len(got))
	}
	if got[0] != "--prefix" || got[1] != repoRoot {
		t.Fatalf("resolveUpdateArgs prefix=%v, want [--prefix %s ...]", got[:2], repoRoot)
	}
}

func TestResolveUpdateArgsPreservesExplicitPrefix(t *testing.T) {
	got := resolveUpdateArgs([]string{"--prefix", "/tmp/target", "--no-build"})
	if len(got) < 2 || got[0] != "--prefix" || got[1] != "/tmp/target" {
		t.Fatalf("resolveUpdateArgs unexpectedly changed explicit prefix: %v", got)
	}
}
