package worker

import (
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayUserCanonRecipientsComeOnlyFromTypedFrozenAuthority(t *testing.T) {
	const (
		maraID = "rpc_11111111111111111111111111111111"
		ivoID  = "rpc_22222222222222222222222222222222"
	)
	preparation := roleplay.SimulationTurnAuthority{
		ParticipantCharacterIDs: []string{maraID, ivoID},
		UserTurn: roleplay.UserTurnAuthority{
			PersonaKind: roleplay.UserPersonaCharacter, CharacterID: ivoID,
			PersonaName: "Ivo", PersonaSummary: "A watch officer.",
			ContributionKind: roleplay.UserContributionDialogue, ExactText: "The gate is sealed.",
		},
	}
	character, err := newRoleplayUserCanonCompletion(
		preparation, []string{"Ivo stated that the gate is sealed."},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(character.KnowledgeCharacterIDs, []model.RoleplayCharacterID{
		model.RoleplayCharacterID(ivoID),
	}) {
		t.Fatalf("character recipients=%v", character.KnowledgeCharacterIDs)
	}

	preparation.UserTurn = roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind: roleplay.UserContributionNarration, ExactText: "The gate is sealed.",
	}
	narrator, err := newRoleplayUserCanonCompletion(
		preparation, []string{"The gate became sealed."},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(narrator.KnowledgeCharacterIDs, []model.RoleplayCharacterID{
		model.RoleplayCharacterID(maraID), model.RoleplayCharacterID(ivoID),
	}) {
		t.Fatalf("narrator recipients=%v", narrator.KnowledgeCharacterIDs)
	}
	empty, err := newRoleplayUserCanonCompletion(preparation, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty.KnowledgeCharacterIDs) != 0 {
		t.Fatalf("empty-fact recipients=%#v", empty)
	}
}
