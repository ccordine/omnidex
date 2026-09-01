package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonPresenceUsesOpaqueBinaryChoice(t *testing.T) {
	input := roleplayCanonPresenceTestInput()
	prompt, err := BuildRoleplayCanonFactPresencePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Answer with A or B.") ||
		strings.Contains(prompt, RoleplayCanonContributionEstablishesFact) ||
		strings.Contains(prompt, RoleplayCanonContributionEstablishesNoFact) {
		t.Fatalf("canon presence prompt crossed the opaque-choice boundary: %s", prompt)
	}
	present, err := DecodeRoleplayCanonFactPresenceResult(input, "A")
	if err != nil || present.Relation != RoleplayCanonContributionEstablishesFact {
		t.Fatalf("present relation=%+v error=%v", present, err)
	}
	absent, err := DecodeRoleplayCanonFactPresenceResult(input, "B")
	if err != nil || absent.Relation != RoleplayCanonContributionEstablishesNoFact {
		t.Fatalf("absent relation=%+v error=%v", absent, err)
	}
	if _, err := DecodeRoleplayCanonFactPresenceResult(
		input,
		RoleplayCanonContributionEstablishesNoFact,
	); err == nil {
		t.Fatal("canon presence accepted a code-owned relation as model output")
	}
}

func TestRoleplayCanonInventoryRequiresPositiveFactLines(t *testing.T) {
	input := roleplayCanonPresenceTestInput()
	raw := "Mara locks the observatory door.\nMara pockets the brass key."
	inventory, err := DecodeRoleplayCanonFactInventory(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 2 || inventory.Candidates[0] != "Mara locks the observatory door." ||
		inventory.Candidates[1] != "Mara pockets the brass key." {
		t.Fatalf("canon inventory=%+v", inventory)
	}
	if _, err := DecodeRoleplayCanonFactInventory(input, ""); err == nil {
		t.Fatal("canon inventory accepted an empty response")
	}
}

func roleplayCanonPresenceTestInput() RoleplayCanonExtractionInput {
	return RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind:                  RoleplayCanonSourceUserContribution,
			AttributedPersonaName: "Mara",
			ExactContribution:     "I lock the observatory door and pocket the brass key.",
			PersonaKind:           roleplay.UserPersonaCharacter,
			ContributionKind:      roleplay.UserContributionDialogue,
		},
		Context: ObjectiveContext{},
	}
}
