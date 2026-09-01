package assemblyline

import (
	"strings"
	"testing"
)

func TestRoleplayOngoingActionRelationUsesOnlyOpaqueChoice(t *testing.T) {
	previous := "Mira is carrying a lantern through the tunnel."
	input := RoleplayOngoingActionRelationInput{
		CharacterName:         "Mira",
		Source:                RoleplayOngoingActionSourceAssistantResponse,
		ExactContribution:     "Mira keeps walking, holding the lantern ahead of her.",
		PreviousOngoingAction: &previous,
	}
	prompt, err := BuildRoleplayOngoingActionRelationPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{previous, "A. ", "B. ", "C. ", "Answer with A or B or C."} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("relation prompt is missing %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"previous_ongoing_action", "assistant_response", "NONE", "preserve",
		"byte-for-byte", "JSON", "schema", "unchanged", "replacement",
		"Return only",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("relation prompt leaked %q:\n%s", forbidden, prompt)
		}
	}

	relation, err := DecodeRoleplayOngoingActionRelation(input, "B")
	if err != nil {
		t.Fatal(err)
	}
	if relation != RoleplayOngoingActionUnchanged {
		t.Fatalf("relation = %q, want code-owned unchanged value", relation)
	}
	if _, err := DecodeRoleplayOngoingActionRelation(input, "unchanged"); err == nil {
		t.Fatal("internal relation label was accepted as model output")
	}
}

func TestRoleplayOngoingActionRelationWithoutPreviousHasTwoChoices(t *testing.T) {
	input := RoleplayOngoingActionRelationInput{
		CharacterName:     "Mira",
		Source:            RoleplayOngoingActionSourceUserAction,
		ExactContribution: "I start crossing the bridge.",
	}
	prompt, err := BuildRoleplayOngoingActionRelationPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "A. ") || !strings.Contains(prompt, "B. ") ||
		strings.Contains(prompt, "C. ") {
		t.Fatalf("relation prompt does not expose exactly two opaque choices:\n%s", prompt)
	}
	relation, err := DecodeRoleplayOngoingActionRelation(input, "B")
	if err != nil {
		t.Fatal(err)
	}
	if relation != RoleplayOngoingActionReplacement {
		t.Fatalf("relation = %q, want code-owned replacement value", relation)
	}
}

func TestRoleplayOngoingActionValueIsOrdinaryPlainText(t *testing.T) {
	input := RoleplayOngoingActionValueInput{
		CharacterName:     "Mira",
		Source:            RoleplayOngoingActionSourceUserAction,
		ExactContribution: "I start crossing the bridge.",
	}
	prompt, err := BuildRoleplayOngoingActionValuePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"previous", "NONE", "JSON", "schema", "Return", "Answer with",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("value prompt leaked %q:\n%s", forbidden, prompt)
		}
	}

	const raw = "Crossing the bridge."
	value, err := DecodeRoleplayOngoingActionValue(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if value != raw {
		t.Fatalf("value = %q, want exact ordinary response %q", value, raw)
	}
	const jsonShaped = `{"ongoing_action":"Crossing the bridge."}`
	value, err = DecodeRoleplayOngoingActionValue(input, jsonShaped)
	if err != nil {
		t.Fatalf("JSON-shaped ordinary action text: %v", err)
	}
	if value != jsonShaped {
		t.Fatalf("JSON-shaped action text changed: got %q want %q", value, jsonShaped)
	}
}

func TestRoleplayOngoingActionPortableBoundariesAreSplit(t *testing.T) {
	relationJob, err := NewRoleplayOngoingActionRelationJob(RoleplayOngoingActionRelationInput{
		CharacterName:     "Mira",
		Source:            RoleplayOngoingActionSourceUserAction,
		ExactContribution: "I stop walking.",
	})
	if err != nil {
		t.Fatal(err)
	}
	valueJob, err := NewRoleplayOngoingActionValueJob(RoleplayOngoingActionValueInput{
		CharacterName:     "Mira",
		Source:            RoleplayOngoingActionSourceUserAction,
		ExactContribution: "I start crossing the bridge.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if relationJob.Kind != WorkRoleplayOngoingActionRelation {
		t.Fatalf("relation kind = %q", relationJob.Kind)
	}
	if valueJob.Kind != WorkRoleplayOngoingActionValue {
		t.Fatalf("value kind = %q", valueJob.Kind)
	}
	for _, job := range []PortableJob{relationJob, valueJob} {
		prompt, err := RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		for _, structured := range []string{"character_name", "exact_contribution", `{"`} {
			if strings.Contains(prompt, structured) {
				t.Fatalf("%s rendered structured model context %q:\n%s", job.Kind, structured, prompt)
			}
		}
		framing, err := PortableResponseFramingForJob(job)
		if err != nil {
			t.Fatal(err)
		}
		if framing != PortableResponseFramingSingleLine {
			t.Fatalf("%s framing = %q", job.Kind, framing)
		}
	}
	maximum, err := PortableResponseMaximumBytesForJob(relationJob)
	if err != nil {
		t.Fatal(err)
	}
	if maximum != 1 {
		t.Fatalf("relation maximum = %d, want one opaque ID byte", maximum)
	}
}
