package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestLifecycleOperationArgsAcceptExactIdentityForRetry(t *testing.T) {
	id, err := queue.NewLifecycleOperationID("cli-retry-test", "41")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--operation-id", string(id), "41", "Continue exactly."},
		{"41", "--operation-id=" + string(id), "Continue exactly."},
	} {
		parsed, remaining, err := parseLifecycleOperationArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		if parsed != id || strings.Join(remaining, "|") != "41|Continue exactly." {
			t.Fatalf("parsed=%q remaining=%q", parsed, remaining)
		}
	}
}

func TestLifecycleOperationArgsGenerateOpaqueIdentityWithoutContentDerivation(t *testing.T) {
	first, remaining, err := parseLifecycleOperationArgs([]string{"41", "same content"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := parseLifecycleOperationArgs([]string{"41", "same content"})
	if err != nil {
		t.Fatal(err)
	}
	if first == second || strings.Join(remaining, "|") != "41|same content" {
		t.Fatalf("first=%q second=%q remaining=%q", first, second, remaining)
	}
	if _, err := queue.ParseLifecycleOperationID(string(first)); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleOperationArgsRejectInvalidOrAmbiguousIdentity(t *testing.T) {
	valid, err := queue.NewLifecycleOperationID("cli-invalid-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--operation-id"},
		{"--operation-id="},
		{"--operation-id", "invalid", "41"},
		{"--operation-id", string(valid), "--operation-id", string(valid), "41"},
	} {
		if _, _, err := parseLifecycleOperationArgs(args); err == nil {
			t.Fatalf("invalid args accepted: %q", args)
		}
	}
}
