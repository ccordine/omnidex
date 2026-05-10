package main

import "testing"

func TestDefaultProjectScopedSessionID(t *testing.T) {
	a := defaultProjectScopedSessionID("/home/gryph/Projects/ai/omnidex")
	b := defaultProjectScopedSessionID("/home/gryph/Projects/ai/omnidex")
	c := defaultProjectScopedSessionID("/home/gryph/Projects/ai/another-repo")

	if a != b {
		t.Fatalf("expected deterministic session id per project, got %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different session ids for different project paths")
	}
	if a == "" {
		t.Fatalf("expected non-empty session id")
	}
}

func TestNormalizeSessionSlug(t *testing.T) {
	got := normalizeSessionSlug("Omni Dex__Project!!!")
	want := "omni-dex-project"
	if got != want {
		t.Fatalf("normalizeSessionSlug=%q want %q", got, want)
	}
}
