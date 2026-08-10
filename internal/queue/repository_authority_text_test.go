package queue

import (
	"strings"
	"testing"
)

func TestJobInstructionValidationPreservesExactValidAuthority(t *testing.T) {
	exact := "  Preserve this exact instruction.\n"
	if err := validateJobInstruction(exact); err != nil {
		t.Fatal(err)
	}
	for name, instruction := range map[string]string{
		"invalid UTF-8": "invalid-" + string([]byte{0xff}),
		"NUL":           "invalid-\x00instruction",
		"blank":         " \n\t ",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateJobInstruction(instruction); err == nil {
				t.Fatal("expected explicit instruction validation failure")
			}
		})
	}
}

func TestCancelReasonValidationIsExactAfterWhitespaceNormalization(t *testing.T) {
	got, err := validateCancelReason("  deliberate cancellation  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "deliberate cancellation" {
		t.Fatalf("cancel reason=%q", got)
	}
	for name, reason := range map[string]string{
		"invalid UTF-8": "invalid-" + string([]byte{0xff}),
		"NUL":           "invalid-\x00reason",
		"blank":         strings.Repeat(" ", 3),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateCancelReason(reason); err == nil {
				t.Fatal("expected explicit cancel-reason validation failure")
			}
		})
	}
}
