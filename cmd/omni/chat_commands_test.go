package main

import (
	"strings"
	"testing"
)

func TestParseChatCommandPreservesOrdinaryAndControlText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input     string
		name      string
		text      string
		isCommand bool
	}{
		{input: "preserve this exact ordinary turn", text: "preserve this exact ordinary turn"},
		{input: "/status", name: "status", isCommand: true},
		{input: "/interrupt", name: "interrupt", isCommand: true},
		{input: "/interrupt because I need to inspect it", name: "interrupt", text: "because I need to inspect it", isCommand: true},
		{input: "/redirect  preserve leading space ", name: "redirect", text: " preserve leading space ", isCommand: true},
		{input: "/cancel stop this objective", name: "cancel", text: "stop this objective", isCommand: true},
		{input: "/exit", name: "exit", isCommand: true},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			name, text, command := parseChatCommand(test.input)
			if name != test.name || text != test.text || command != test.isCommand {
				t.Fatalf(
					"parseChatCommand(%q) = (%q, %q, %t), want (%q, %q, %t)",
					test.input, name, text, command,
					test.name, test.text, test.isCommand,
				)
			}
		})
	}
}

func TestChatCommandsRejectMissingOrDiscardedArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		message string
	}{
		{input: "/redirect", message: "/redirect requires exact redirection text"},
		{input: "/redirect   ", message: "/redirect requires exact redirection text"},
		{input: "/cancel", message: "/cancel requires a reason"},
		{input: "/cancel   ", message: "/cancel requires a reason"},
		{input: "/status ignored", message: "/status does not accept arguments"},
		{input: "/exit ignored", message: "/exit does not accept arguments"},
		{input: "/help ignored", message: "/help does not accept arguments"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			quit, err := (&chatSession{}).acceptInput(test.input, false)
			if quit || err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("acceptInput(%q) = quit %t, error %v; want %q", test.input, quit, err, test.message)
			}
		})
	}
}

func TestExitCommandClosesOnlyTheClientSession(t *testing.T) {
	t.Parallel()

	quit, err := (&chatSession{}).acceptInput("/exit", false)
	if err != nil || !quit {
		t.Fatalf("/exit = quit %t, error %v; want quit without error", quit, err)
	}
}
