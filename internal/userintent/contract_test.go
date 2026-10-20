package userintent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSuppliedStateContractIsExactEvidence(t *testing.T) {
	input := parserInput(t, "Approve and start.")
	input.Contract = json.RawMessage(`{"properties":{"items":{"type":"array"}},"description":"Typed caller state."}`)
	calls := 0
	_, err := Interpret(t.Context(), input, func(_ context.Context, prompt string) (string, error) {
		calls++
		var envelope struct {
			Contract json.RawMessage `json:"state_contract"`
			Snapshot json.RawMessage `json:"server_snapshot"`
			Message  string          `json:"user_message"`
		}
		at := strings.Index(prompt, "Input:\n")
		if at < 0 {
			t.Fatal("missing input")
		}
		if err := json.Unmarshal([]byte(prompt[at+7:]), &envelope); err != nil {
			t.Fatal(err)
		}
		if string(envelope.Contract) != string(input.Contract) || string(envelope.Snapshot) != string(input.Snapshot) || envelope.Message != input.Prompt {
			t.Fatal("changed contract/snapshot/message")
		}
		return `{"patch":[]}`, nil
	})
	if err != nil || calls != 1 {
		t.Fatal(calls, err)
	}
	for _, bad := range []string{`[]`, `{"broken":`, strings.Repeat(" ", 33<<10)} {
		input.Contract = json.RawMessage(bad)
		_, err := Interpret(t.Context(), input, func(context.Context, string) (string, error) { t.Fatal("invalid contract dispatched"); return "", nil })
		if err == nil {
			t.Fatal("invalid contract accepted")
		}
	}
}
