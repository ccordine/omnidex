package codex

import (
	"context"
	"errors"
	"github.com/gryph/omnidex/internal/codingagent"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeExecutionDocker(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	script := `#!/bin/sh
set -eu
printf '%s\n' "$1" >> "$DEPENDENCY_TEST_LOG"
case "$1" in
 info) [ "${DEPENDENCY_TEST_STOPPED:-}" != 1 ] || [ -f "$DEPENDENCY_TEST_LOG.started" ] ;;
 context) echo unix:///run/user/1000/docker.sock ;;
 --user) touch "$DEPENDENCY_TEST_LOG.started" ;;
 run) echo container ;;
 exec) [ "${DEPENDENCY_TEST_UNREADY:-}" != 1 ] ;;
 port) echo 127.0.0.1:54321 ;;
 ps) echo container ;;
 rm) [ "${DEPENDENCY_TEST_CLEANUP_FAIL:-}" != 1 ] ;;
 *) exit 1 ;;
esac
`
	for _, name := range []string{"docker", "systemctl"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(root, "commands")
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	t.Setenv("DEPENDENCY_TEST_LOG", log)
	t.Setenv("DATABASE_URL", "postgres://installed-ledger")
	t.Setenv("REDIS_URL", "redis://installed-cache")
	t.Setenv("OLLAMA_BASE_URL", "")
	return log
}

func TestExecutionDependenciesStartExposeAndClean(t *testing.T) {
	log := fakeExecutionDocker(t)
	t.Setenv("DEPENDENCY_TEST_STOPPED", "1")
	dependencies, err := prepareExecutionDependencies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	environment := strings.Join(dependencies.environment, "\n")
	for _, key := range []string{"DATABASE_URL=postgres://postgres:", "OMNI_TEST_DATABASE_URL=postgres://postgres:", "REDIS_URL=redis://:", "DATABASE_SCHEMA=omnidex_runtime"} {
		if !strings.Contains(environment, key) {
			t.Fatalf("missing child endpoint %s", key)
		}
	}
	if strings.Contains(environment, "installed-ledger") || strings.Contains(environment, "installed-cache") {
		t.Fatal("runtime state leaked into child endpoints")
	}
	if os.Getenv("DATABASE_URL") != "postgres://installed-ledger" {
		t.Fatal("mutated launching process")
	}
	if err := dependencies.close(); err != nil {
		t.Fatal(err)
	}
	if err := dependencies.close(); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(commands), "run\n") != 2 || strings.Count(string(commands), "rm\n") != 2 || !strings.Contains(string(commands), "--user\n") {
		t.Fatalf("incorrect startup/cleanup: %s", commands)
	}
}

func TestExecutionDependenciesReadinessCancellationCleansPartialStart(t *testing.T) {
	log := fakeExecutionDocker(t)
	t.Setenv("DEPENDENCY_TEST_UNREADY", "1")
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	dependencies, err := prepareExecutionDependencies(ctx)
	if dependencies != nil || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "postgres readiness") {
		t.Fatalf("readiness did not fail closed: %v", err)
	}
	commands, _ := os.ReadFile(log)
	if strings.Count(string(commands), "run\n") != 1 || strings.Count(string(commands), "rm\n") != 1 {
		t.Fatalf("partial resources not cleaned: %s", commands)
	}
}

func TestExecutionDependenciesCleanupFailureIsReturned(t *testing.T) {
	fakeExecutionDocker(t)
	dependencies, err := prepareExecutionDependencies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEPENDENCY_TEST_CLEANUP_FAIL", "1")
	if err := dependencies.close(); err == nil || !strings.Contains(err.Error(), "cleanup omnidex-execution-") {
		t.Fatalf("cleanup failure hidden: %v", err)
	}
}

func TestExecutionDependenciesAbsentConfigurationDoesNotCreateResources(t *testing.T) {
	log := fakeExecutionDocker(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	dependencies, err := prepareExecutionDependencies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := dependencies.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("created unconfigured services")
	}
}

func TestNativeLaunchFailureLeavesExistingDependenciesAlone(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			log := fakeExecutionDocker(t)
			client, err := New(filepath.Join(t.TempDir(), "missing-codex"), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Run(t.Context(), codingagent.Assignment{JobID: 1, Generation: 1, Root: t.TempDir(), Role: role, Route: codingagent.Route{Provider: "codex", Model: "fixed"}, Task: &codingagent.Task{ID: "fixture", Goal: "Verify this task."}}, nil)
			if err == nil {
				t.Fatal("missing executable succeeded")
			}
			if commands, err := os.ReadFile(log); !os.IsNotExist(err) {
				t.Fatalf("launch failure created or removed dependency resources: %s %v", commands, err)
			}
		})
	}
}

func TestNativeCancellationLeavesExistingDependenciesAlone(t *testing.T) {
	log := fakeExecutionDocker(t)
	command := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nsleep 20\n"), 0700); err != nil {
		t.Fatal(err)
	}
	client, err := New(command, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = client.Run(ctx, codingagent.Assignment{JobID: 1, Generation: 1, Root: t.TempDir(), Role: codingagent.Execute, Route: codingagent.Route{Provider: "codex", Model: "fixed"}, Task: &codingagent.Task{ID: "fixture", Goal: "Verify this task."}}, nil)
	if err == nil {
		t.Fatal("cancelled native session succeeded")
	}
	if commands, err := os.ReadFile(log); !os.IsNotExist(err) {
		t.Fatalf("cancellation created or removed dependency resources: %s %v", commands, err)
	}
}
