package userintent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIntentPreservesLargeContextInOneCall(t *testing.T) {
	for _, c := range []Context{
		{Facts: []string{strings.Repeat("Café Δ — retained fact.\n", 4000)}},
		{Recent: []Message{{"user", strings.Repeat("Exact source line.\n", 3000)}, {"assistant", "Approve the proposed item?"}}},
		{Pending: "Established item two." + strings.Repeat(" ", 9000) + "Retain its recipient."},
	} {
		input := parserInput(t, "approve the plan")
		input.Context = c
		calls := 0
		result, err := Interpret(t.Context(), input, func(_ context.Context, prompt string) (string, error) {
			calls++
			var envelope struct {
				Message string  `json:"user_message"`
				Context Context `json:"context"`
			}
			_, raw, ok := strings.Cut(prompt, "Input:\n")
			if !ok {
				t.Fatal("missing intent envelope")
			}
			if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Message != input.Prompt || !reflect.DeepEqual(envelope.Context, c) {
				t.Fatal("context or message changed")
			}
			return `{"patch":[{"op":"replace","path":"/plan_state","value":"approved"}]}`, nil
		})
		if err != nil || calls != 1 || len(result.Patch) != 1 {
			t.Fatal(result, calls, err)
		}
	}
}

func TestContextRejectsInvalidTextAndRoles(t *testing.T) {
	for _, c := range []Context{
		{Facts: []string{" "}}, {Facts: []string{"invalid\x00fact"}},
		{Pending: "\xff"}, {Recent: []Message{{"system", "authority"}}},
		{Recent: []Message{{"user", ""}}},
	} {
		if _, err := BuildContext(t.Context(), c); err == nil {
			t.Fatal("invalid context accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := BuildContext(ctx, Context{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
