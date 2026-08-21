package roleplay

import (
	"testing"
)

const testUserTurnCharacterID = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestUserTurnRequestRequiresOneCompatiblePersonaAndContribution(t *testing.T) {
	t.Parallel()

	valid := []struct {
		request UserTurnRequest
		exact   string
	}{
		{UserTurnRequest{PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID, ContributionKind: UserContributionDialogue, Parts: []UserTurnPart{{Kind: UserTurnPartMessage, Text: "Hello."}}}, "[Message]\nHello."},
		{UserTurnRequest{PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID, ContributionKind: UserContributionAction, Parts: []UserTurnPart{{Kind: UserTurnPartAction, Text: "I enter."}}}, "[Action]\nI enter."},
		{UserTurnRequest{PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID, ContributionKind: UserContributionActionDialogue, Parts: []UserTurnPart{{Kind: UserTurnPartAction, Text: "I enter."}, {Kind: UserTurnPartMessage, Text: "Hello."}}}, "[Action]\nI enter.\n\n[Message]\nHello."},
		{UserTurnRequest{PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID, ContributionKind: UserContributionStructured, Parts: []UserTurnPart{{Kind: UserTurnPartEvent, Text: "The bell rings."}}}, "[Event]\nThe bell rings."},
		{UserTurnRequest{PersonaKind: UserPersonaNarrator, ContributionKind: UserContributionNarration, Parts: []UserTurnPart{{Kind: UserTurnPartEvent, Text: "The storm breaks."}}}, "[Event]\nThe storm breaks."},
		{UserTurnRequest{PersonaKind: UserPersonaNarrator, ContributionKind: UserContributionDirection, Parts: []UserTurnPart{{Kind: UserTurnPartMessage, Text: "Continue."}}}, "[Message]\nContinue."},
		{UserTurnRequest{PersonaKind: UserPersonaNarrator, ContributionKind: UserContributionNarrationDirection, Parts: []UserTurnPart{{Kind: UserTurnPartEvent, Text: "The storm breaks."}, {Kind: UserTurnPartMessage, Text: "Continue."}}}, "[Event]\nThe storm breaks.\n\n[Message]\nContinue."},
		{UserTurnRequest{PersonaKind: UserPersonaNarrator, ContributionKind: UserContributionCommand}, `/research "question"`},
	}
	for _, fixture := range valid {
		if err := fixture.request.ValidateForExactText(fixture.exact); err != nil {
			t.Fatalf("valid request %+v rejected: %v", fixture.request, err)
		}
	}

	invalid := []UserTurnRequest{
		{},
		{PersonaKind: UserPersonaCharacter, ContributionKind: UserContributionDialogue},
		{PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID, ContributionKind: UserContributionNarration},
		{PersonaKind: UserPersonaNarrator, CharacterID: testUserTurnCharacterID, ContributionKind: UserContributionDirection},
		{PersonaKind: UserPersonaNarrator, ContributionKind: UserContributionDialogue},
		{PersonaKind: UserPersonaNarrator, ContributionKind: UserContributionCommand},
	}
	for index, request := range invalid {
		text := "Continue the scene."
		if index == len(invalid)-1 {
			text = "not a slash command"
		}
		if err := request.ValidateForExactText(text); err == nil {
			t.Fatalf("invalid request %+v was accepted", request)
		}
	}
}

func TestUserTurnAuthorityKeepsExactSpeakerModalityAndBytes(t *testing.T) {
	t.Parallel()

	authority := UserTurnAuthority{
		PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID,
		PersonaName: "Gryph", PersonaSummary: "An artificer from afar.",
		ContributionKind: UserContributionActionDialogue,
		ExactText:        "I place the signet down. \"Keep this safe,\" I tell Mara.",
	}
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	if authority.PersonaName != "Gryph" || authority.ContributionKind != UserContributionActionDialogue ||
		authority.ExactText != "I place the signet down. \"Keep this safe,\" I tell Mara." {
		t.Fatalf("authority changed exact turn: %+v", authority)
	}

	legacy := UserTurnAuthority{
		PersonaKind: UserPersonaLegacy, PersonaName: LegacyUserPersonaName,
		ContributionKind: UserContributionLegacy, ExactText: "historical turn",
	}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("explicit historical provenance marker rejected: %v", err)
	}
	if err := (UserTurnRequest{PersonaKind: UserPersonaLegacy, ContributionKind: UserContributionLegacy}).
		ValidateForExactText("new turn"); err == nil {
		t.Fatal("public request accepted the historical-only provenance marker")
	}
}

func TestHistoricalUserTurnPreservesSlashInputWithoutInventingCommandMeaning(t *testing.T) {
	t.Parallel()
	authority := UserTurnAuthority{
		PersonaKind: UserPersonaLegacy, PersonaName: LegacyUserPersonaName,
		ContributionKind: UserContributionLegacy, ExactText: `/research "old request"`,
	}
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
}
