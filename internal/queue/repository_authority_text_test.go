package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
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

func TestChannelMessageValidationPreservesExactValidAuthority(t *testing.T) {
	exact := "  Preserve this exact channel message.\n\t"
	if err := validateChannelMessageContent(exact); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"invalid UTF-8": "invalid-" + string([]byte{0xff}),
		"NUL":           "invalid-\x00message",
		"blank":         " \n\t ",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateChannelMessageContent(content); err == nil {
				t.Fatal("expected explicit channel-message validation failure")
			}
		})
	}
}

func TestChannelMessageRoleRejectsClientAuthorityExpansion(t *testing.T) {
	for _, role := range []model.ChannelMessageRole{model.ChannelMessageRoleUser, model.ChannelMessageRoleAssistant} {
		if err := role.Validate(); err != nil {
			t.Fatalf("role %q: %v", role, err)
		}
	}
	for _, role := range []string{"", " user ", "system", "tool", "unknown"} {
		if err := model.ChannelMessageRole(role).Validate(); err == nil {
			t.Fatalf("role %q was accepted", role)
		}
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
