package assemblyline

import (
	"fmt"
	"testing"
)

func TestOpaqueModelChoiceIDsCoverBoundedCodeOwnedSets(t *testing.T) {
	choices := make([]OpaqueModelChoice, maxOpaqueModelChoices)
	for index := range choices {
		choice, err := NewOpaqueModelChoice(
			fmt.Sprintf("Candidate %d", index+1),
			fmt.Sprintf("internal-value-%d", index+1),
		)
		if err != nil {
			t.Fatalf("choice %d: %v", index, err)
		}
		choices[index] = choice
	}
	for index, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 255: "IV"} {
		if got := opaqueModelChoiceID(index); got != want {
			t.Fatalf("choice ID %d = %q, want %q", index, got, want)
		}
		decoded, err := DecodeOpaqueModelChoice(want, choices)
		if err != nil {
			t.Fatalf("decode choice ID %q: %v", want, err)
		}
		if decoded != fmt.Sprintf("internal-value-%d", index+1) {
			t.Fatalf("decoded choice %d = %q", index, decoded)
		}
	}
	maximum, err := opaqueModelChoiceResponseMaximum(choices)
	if err != nil {
		t.Fatalf("choice response maximum: %v", err)
	}
	if maximum != 2 {
		t.Fatalf("choice response maximum = %d, want 2", maximum)
	}
}

func TestResolveSoleOpaqueModelChoiceUsesCodeOwnedValue(t *testing.T) {
	choice, err := NewOpaqueModelChoice("The only available candidate", "internal-only-value")
	if err != nil {
		t.Fatalf("new choice: %v", err)
	}
	value, resolved, err := ResolveSoleOpaqueModelChoice([]OpaqueModelChoice{choice})
	if err != nil {
		t.Fatalf("resolve sole choice: %v", err)
	}
	if !resolved || value != "internal-only-value" {
		t.Fatalf("sole choice = (%q, %t), want internal-only-value and true", value, resolved)
	}
}
