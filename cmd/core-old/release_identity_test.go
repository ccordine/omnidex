package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/version"
)

func TestCoreStartupRequiresEmbeddedBuildCommit(t *testing.T) {
	original := version.Commit
	t.Cleanup(func() { version.Commit = original })

	version.Commit = ""
	if err := validateReleaseIdentity(); err == nil {
		t.Fatal("core startup accepted a missing embedded build commit")
	}

	version.Commit = strings.Repeat("c", 40)
	if err := validateReleaseIdentity(); err != nil {
		t.Fatalf("core startup rejected an exact embedded build commit: %v", err)
	}
}

func TestCoreReleaseVerificationRequiresExactEmbeddedCommit(t *testing.T) {
	original := version.Commit
	t.Cleanup(func() { version.Commit = original })
	version.Commit = strings.Repeat("e", 40)

	got, err := verifyReleaseCommit(version.Commit)
	if err != nil || got != version.Commit {
		t.Fatalf("verifyReleaseCommit()=(%q, %v), want %q", got, err, version.Commit)
	}
	if _, err := verifyReleaseCommit(strings.Repeat("f", 40)); err == nil {
		t.Fatal("release verification accepted a different expected commit")
	}
	version.Commit = "invalid"
	if _, err := verifyReleaseCommit("invalid"); err == nil {
		t.Fatal("release verification accepted an invalid embedded commit")
	}
}
