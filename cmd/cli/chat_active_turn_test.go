package main

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

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

	message, ok = parseQueuedTurnInput("\t  preserve queued authority\t ")
	if !ok || message != "  preserve queued authority\t " {
		t.Fatalf("queued authority changed, got ok=%t message=%q", ok, message)
	}

	message, ok = parseQueuedTurnInput("\t\tsecond tab belongs to authority")
	if !ok || message != "\tsecond tab belongs to authority" {
		t.Fatalf("queued authority control byte handling changed, got ok=%t message=%q", ok, message)
	}
}

func TestValidateSameJobControlRejectsSuccessorAuthority(t *testing.T) {
	if err := validateSameJobControl("feedback", 17, model.Job{ID: 17}); err != nil {
		t.Fatal(err)
	}
	if err := validateSameJobControl("feedback", 17, model.Job{ID: 18}); err == nil {
		t.Fatal("CLI accepted a successor job as feedback authority")
	}
}
