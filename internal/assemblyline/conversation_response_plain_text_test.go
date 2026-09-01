package assemblyline

import (
	"strings"
	"testing"
)

func TestConversationResponseAcceptsOrdinarySemanticTextWithoutWrapperParsing(t *testing.T) {
	input := ConversationResponseInput{
		Kind:             ObjectiveKindAnswer,
		ExactInstruction: "Explain the result in the most useful form.",
	}
	tests := map[string]string{
		"markdown source":  "```go\nfunc answer() int { return 42 }\n```",
		"JSON-shaped text": `{"answer":"ready","items":[1,2]}`,
		"quoted prose":     `"ready"`,
		"outer whitespace": "  An intentionally indented answer.\n",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			decision, err := DecodeConversationResponseDecision(input, raw)
			if err != nil {
				t.Fatalf("decode ordinary response: %v", err)
			}
			if decision.Schema != ConversationResponseSchemaV1 {
				t.Fatalf("schema = %q", decision.Schema)
			}
			if decision.Text != raw {
				t.Fatalf("text changed across boundary: got %q want %q", decision.Text, raw)
			}
		})
	}
}

func TestConversationResponseStillRejectsInvalidOrUnboundedTransportText(t *testing.T) {
	input := ConversationResponseInput{
		Kind:             ObjectiveKindAnswer,
		ExactInstruction: "Answer this question.",
	}
	for name, raw := range map[string]string{
		"empty":        " \n\t ",
		"NUL":          "answer\x00tail",
		"too large":    strings.Repeat("x", maxConversationResponseTextBytes+1),
		"invalid UTF8": string([]byte{0xff}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeConversationResponseDecision(input, raw); err == nil {
				t.Fatal("expected conversation response transport validation failure")
			}
		})
	}
}

func TestStrictSemanticLeafStillRejectsStructuredChoiceResponse(t *testing.T) {
	first, err := NewOpaqueModelChoice("first", "internal-first")
	if err != nil {
		t.Fatalf("first choice: %v", err)
	}
	second, err := NewOpaqueModelChoice("second", "internal-second")
	if err != nil {
		t.Fatalf("second choice: %v", err)
	}
	choices := []OpaqueModelChoice{first, second}
	if _, err := DecodeOpaqueModelChoice(`{"choice":"A"}`, choices); err == nil {
		t.Fatal("opaque selection accepted a JSON response packet")
	}
}
