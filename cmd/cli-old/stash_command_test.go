package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseStashArgsDefaults(t *testing.T) {
	opts, help, err := parseStashArgs(nil)
	if err != nil {
		t.Fatalf("parseStashArgs returned error: %v", err)
	}
	if help {
		t.Fatalf("expected help=false")
	}
	if opts.Action != stashActionPush {
		t.Fatalf("action=%q, want %q", opts.Action, stashActionPush)
	}
	if opts.TrackedOnly {
		t.Fatalf("expected TrackedOnly=false")
	}
	if opts.IncludeAll {
		t.Fatalf("expected IncludeAll=false")
	}
}

func TestParseStashArgsPopWithRef(t *testing.T) {
	opts, help, err := parseStashArgs([]string{"--prefix", "/tmp/repo", "--pop", "stash@{1}"})
	if err != nil {
		t.Fatalf("parseStashArgs returned error: %v", err)
	}
	if help {
		t.Fatalf("expected help=false")
	}
	if opts.Prefix != "/tmp/repo" {
		t.Fatalf("prefix=%q, want /tmp/repo", opts.Prefix)
	}
	if opts.Action != stashActionPop {
		t.Fatalf("action=%q, want %q", opts.Action, stashActionPop)
	}
	if opts.Ref != "stash@{1}" {
		t.Fatalf("ref=%q, want stash@{1}", opts.Ref)
	}
}

func TestParseStashArgsRejectsMultipleActions(t *testing.T) {
	if _, _, err := parseStashArgs([]string{"--list", "--apply"}); err == nil {
		t.Fatalf("expected multiple action error")
	}
}

func TestParseStashArgsRejectsTrackedOnlyWithAll(t *testing.T) {
	if _, _, err := parseStashArgs([]string{"--tracked-only", "--all"}); err == nil {
		t.Fatalf("expected incompatible flags error")
	}
}

func TestResolveStashTargetPrefersExplicitPrefix(t *testing.T) {
	t.Setenv(omniRuntimeDirEnv, "/tmp/runtime")

	targetRoot := t.TempDir()
	got, err := resolveStashTarget(targetRoot)
	if err != nil {
		t.Fatalf("resolveStashTarget returned error: %v", err)
	}
	want, _ := filepath.Abs(targetRoot)
	if got != filepath.Clean(want) {
		t.Fatalf("resolveStashTarget()=%q, want %q", got, filepath.Clean(want))
	}
}

func TestResolveStashTargetUsesRepoCWDBeforeRuntimeDir(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteFile := func(path string) {
		t.Helper()
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
		t.Fatalf("chdir repo: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})

	t.Setenv(omniRuntimeDirEnv, filepath.Join(t.TempDir(), "runtime"))

	got, err := resolveStashTarget("")
	if err != nil {
		t.Fatalf("resolveStashTarget returned error: %v", err)
	}
	want, _ := filepath.Abs(repoRoot)
	if got != filepath.Clean(want) {
		t.Fatalf("resolveStashTarget()=%q, want %q", got, filepath.Clean(want))
	}
}

func TestResolveStashTargetFallsBackToRuntimeDir(t *testing.T) {
	cwd := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})

	runtimeRoot := t.TempDir()
	t.Setenv(omniRuntimeDirEnv, runtimeRoot)

	got, err := resolveStashTarget("")
	if err != nil {
		t.Fatalf("resolveStashTarget returned error: %v", err)
	}
	want, _ := filepath.Abs(runtimeRoot)
	if got != filepath.Clean(want) {
		t.Fatalf("resolveStashTarget()=%q, want %q", got, filepath.Clean(want))
	}
}

func TestBuildGitStashInvocationPushDefaults(t *testing.T) {
	now := time.Date(2026, time.February, 15, 14, 30, 0, 0, time.UTC)
	got := buildGitStashInvocation("/tmp/repo", stashOptions{}, now)
	want := []string{
		"-C", "/tmp/repo", "stash", "push",
		"--include-untracked",
		"--message", "omni stash 2026-02-15T14:30:00Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildGitStashInvocation()=%v, want %v", got, want)
	}
}

func TestBuildGitStashInvocationActions(t *testing.T) {
	listArgs := buildGitStashInvocation("/tmp/repo", stashOptions{Action: stashActionList}, time.Now())
	if !reflect.DeepEqual(listArgs, []string{"-C", "/tmp/repo", "stash", "list"}) {
		t.Fatalf("list invocation=%v", listArgs)
	}

	popArgs := buildGitStashInvocation("/tmp/repo", stashOptions{Action: stashActionPop, Ref: "stash@{2}"}, time.Now())
	if !reflect.DeepEqual(popArgs, []string{"-C", "/tmp/repo", "stash", "pop", "stash@{2}"}) {
		t.Fatalf("pop invocation=%v", popArgs)
	}

	applyArgs := buildGitStashInvocation("/tmp/repo", stashOptions{Action: stashActionApply}, time.Now())
	if !reflect.DeepEqual(applyArgs, []string{"-C", "/tmp/repo", "stash", "apply"}) {
		t.Fatalf("apply invocation=%v", applyArgs)
	}
}
