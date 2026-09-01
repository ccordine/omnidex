package main

import (
	"os"
	"testing"
)

func TestApplyInvocationCWDFromEnv(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(original)
	}()

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

func TestApplyInvocationCWDFromEnvInvalid(t *testing.T) {
	t.Setenv(invokeCWDEnv, "/path/that/does/not/exist")
	if err := applyInvocationCWDFromEnv(); err == nil {
		t.Fatal("expected error for invalid invocation cwd")
	}
}
