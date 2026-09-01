package assemblyline

import (
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestGroundedAnswerParagraphInventoryRequiresPositiveCandidates(t *testing.T) {
	t.Parallel()
	input := GroundedAnswerParagraphInventoryInput{
		ExactRequirement: "How long is the repair expected to take?",
		Context:          plainTextPromptObjectiveContext("The repair is scheduled."),
		Evidence: []GroundedEvidenceCapsule{{
			ID: "evidence_1", Text: "The inspection report records a two-day repair estimate.",
		}},
	}
	const paragraph = "The repair is expected to take two days."
	inventory, err := DecodeGroundedAnswerParagraphInventory(input, paragraph)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 1 || inventory.Candidates[0] != paragraph {
		t.Fatalf("positive inventory=%#v", inventory.Candidates)
	}
	if _, err := DecodeGroundedAnswerParagraphInventory(input, ""); err == nil {
		t.Fatal("empty grounded answer inventory was accepted")
	}
	inventory.Candidates = nil
	if err := inventory.ValidateFor(input); err == nil {
		t.Fatal("zero-candidate grounded answer inventory was accepted")
	}
	job, err := NewGroundedAnswerParagraphInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	maximum, err := PortableResponseMaximumBytesForJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if maximum != maxGroundedAnswerParagraphInventoryBytes {
		t.Fatalf("response maximum=%d, want %d", maximum, maxGroundedAnswerParagraphInventoryBytes)
	}
}

func TestRoleplayGroundedParagraphInventoryRequiresPositiveCandidates(t *testing.T) {
	t.Parallel()
	input := RoleplayGroundedResponseInput{
		ExactQuestion: "When does the eclipse begin?",
		RoleplayIdentity: RoleplayResponseIdentity{
			CharacterName: "Mara", Summary: "A careful astronomer who speaks plainly.",
		},
		RoleplayUserTurn: RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Ilan",
			ContributionKind: roleplay.UserContributionDialogue,
		},
		Context: plainTextPromptObjectiveContext("Mara repaired the observatory clock."),
		RealWorldEvidence: []GroundedEvidenceCapsule{{
			ID: "evidence_1", Text: "The almanac says the eclipse begins at midnight.",
		}},
	}
	const paragraph = "The eclipse begins at midnight."
	inventory, err := DecodeRoleplayGroundedParagraphInventory(input, paragraph)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 1 || inventory.Candidates[0] != paragraph {
		t.Fatalf("positive inventory=%#v", inventory.Candidates)
	}
	if _, err := DecodeRoleplayGroundedParagraphInventory(input, ""); err == nil {
		t.Fatal("empty roleplay grounded inventory was accepted")
	}
	inventory.Candidates = nil
	if err := inventory.ValidateFor(input); err == nil {
		t.Fatal("zero-candidate roleplay grounded inventory was accepted")
	}
	job, err := NewRoleplayGroundedParagraphInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	maximum, err := PortableResponseMaximumBytesForJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if maximum != maxRoleplayGroundedParagraphInventoryBytes {
		t.Fatalf("response maximum=%d, want %d", maximum, maxRoleplayGroundedParagraphInventoryBytes)
	}
}
