package main

import "testing"

func TestParseQueuedTurnInput(t *testing.T) {
	message, ok := parseQueuedTurnInput("\tfollow up with test details")
	if !ok || message != "follow up with test details" {
		t.Fatalf("expected tab-prefixed line to queue, got ok=%t message=%q", ok, message)
	}

	message, ok = parseQueuedTurnInput("\t   ")
	if ok || message != "" {
		t.Fatalf("expected empty tab-prefixed line to be ignored, got ok=%t message=%q", ok, message)
	}

	message, ok = parseQueuedTurnInput("follow up without tab")
	if ok || message != "" {
		t.Fatalf("expected non-tab line not to queue, got ok=%t message=%q", ok, message)
	}
}
