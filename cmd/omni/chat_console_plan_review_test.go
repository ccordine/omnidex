package main

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/term"
)

func TestChatConsolePlanReviewUsesAlternateScreenAndRestoresPrompt(t *testing.T) {
	t.Parallel()

	console, output := planReviewTestConsole(t)
	if err := console.ShowPlanReview("PLAN REVIEW\n> 1. [pending] ✓ safe\x1b[31m text\n"); err != nil {
		t.Fatalf("ShowPlanReview() error = %v", err)
	}
	if !console.reviewActive || !console.reviewInput.ReviewEnabled() {
		t.Fatal("ShowPlanReview() did not activate display and input together")
	}
	if err := console.SetPrompt("you #73> "); err != nil {
		t.Fatalf("SetPrompt() during review error = %v", err)
	}
	if err := console.HidePlanReview(); err != nil {
		t.Fatalf("HidePlanReview() error = %v", err)
	}
	if console.reviewActive || console.reviewInput.ReviewEnabled() {
		t.Fatal("HidePlanReview() retained display or input mode")
	}
	rendered := output.String()
	for _, required := range []string{
		"\x1b[?1049h", "\x1b[?25l", "\x1b[H\x1b[2J",
		`safe\u001b[31m text`, "\x1b[?25h\x1b[?1049l",
	} {
		if !strings.Contains(rendered, required) {
			t.Errorf("terminal output omitted %q: %q", required, rendered)
		}
	}
	if strings.Contains(rendered, "safe\x1b[31m text") {
		t.Fatal("model/server text injected a terminal escape into the trusted review frame")
	}
	if console.prompt != "you #73> " {
		t.Fatalf("restored prompt authority = %q, want job prompt", console.prompt)
	}
}

func TestChatConsolePersistsOrdinaryOutputAroundActiveReview(t *testing.T) {
	t.Parallel()

	console, output := planReviewTestConsole(t)
	if err := console.ShowPlanReview("PLAN REVIEW\n> one\n"); err != nil {
		t.Fatal(err)
	}
	before := output.Len()
	if err := console.WriteError("[omni] persisted job state\n"); err != nil {
		t.Fatalf("WriteError() during review error = %v", err)
	}
	segment := output.String()[before:]
	firstExit := strings.Index(segment, "\x1b[?25h\x1b[?1049l")
	message := strings.Index(segment, "[omni] persisted job state")
	reenter := strings.LastIndex(segment, "\x1b[?1049h\x1b[?25l")
	redraw := strings.LastIndex(segment, "PLAN REVIEW")
	if firstExit < 0 || message <= firstExit || reenter <= message || redraw <= reenter {
		t.Fatalf("review/output ordering is not transcript-preserving: %q", segment)
	}
	if !console.reviewActive || !console.reviewInput.ReviewEnabled() {
		t.Fatal("ordinary output unexpectedly ended plan review")
	}
}

func planReviewTestConsole(t *testing.T) (*chatConsole, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	router, err := newPlanReviewInputRouter(newPlanReviewTestSource(strings.NewReader("")))
	if err != nil {
		t.Fatal(err)
	}
	terminal := term.NewTerminal(
		terminalReadWriter{reader: router, writer: &output},
		"you> ",
	)
	return &chatConsole{
		terminal: terminal, reviewInput: router, output: &output,
		terminalFD: -1, prompt: "you> ",
	}, &output
}
