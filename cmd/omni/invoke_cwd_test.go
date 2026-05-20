package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyInvocationCWDFromEnvChangesOmniWorkingDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	target := t.TempDir()
	t.Setenv(invokeCWDEnv, target)

	if err := applyInvocationCWDFromEnv(); err != nil {
		t.Fatalf("applyInvocationCWDFromEnv: %v", err)
	}

	got, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd after chdir: %v", err)
	}
	if got != target {
		t.Fatalf("cwd=%q want=%q", got, target)
	}
}

func TestApplyInvocationCWDFromEnvRejectsInvalidOmniWorkingDirectory(t *testing.T) {
	t.Setenv(invokeCWDEnv, filepath.Join(t.TempDir(), "missing"))

	if err := applyInvocationCWDFromEnv(); err == nil {
		t.Fatal("expected error for invalid invocation cwd")
	}
}
