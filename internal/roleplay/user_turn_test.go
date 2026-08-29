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
		{PersonaKind: UserPersonaKind("retired"), ContributionKind: UserContributionKind("retired")},
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
	unsupported := UserTurnAuthority{
		PersonaKind: UserPersonaKind("retired"), PersonaName: "No current persona",
		ContributionKind: UserContributionKind("retired"), ExactText: "Unsupported turn.",
	}
	if err := unsupported.Validate(); err == nil {
		t.Fatal("roleplay user-turn authority accepted unsupported persona and contribution kinds")
	}
}

func TestUserTurnOngoingActionContributionUsesOnlyTypedActionAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		turn              UserTurnAuthority
		want              bool
		exactContribution string
	}{
		{
			name: "character action",
			turn: exactCharacterUserTurn(UserContributionAction, []UserTurnPart{{
				Kind: UserTurnPartAction, Text: "I keep hauling the line.",
			}}),
			want: true, exactContribution: "[Action]\nI keep hauling the line.",
		},
		{
			name: "character action and dialogue excludes message",
			turn: exactCharacterUserTurn(UserContributionActionDialogue, []UserTurnPart{
				{Kind: UserTurnPartAction, Text: "I brace the door."},
				{Kind: UserTurnPartMessage, Text: "Run now!"},
			}),
			want: true, exactContribution: "[Action]\nI brace the door.",
		},
		{
			name: "structured character action",
			turn: exactCharacterUserTurn(UserContributionStructured, []UserTurnPart{
				{Kind: UserTurnPartEvent, Text: "The deck tilts."},
				{Kind: UserTurnPartAction, Text: "I catch the loose crate."},
			}),
			want: true, exactContribution: "[Action]\nI catch the loose crate.",
		},
		{
			name: "character dialogue",
			turn: exactCharacterUserTurn(UserContributionDialogue, []UserTurnPart{{
				Kind: UserTurnPartMessage, Text: "Hold fast.",
			}}),
		},
		{
			name: "structured event only",
			turn: exactCharacterUserTurn(UserContributionStructured, []UserTurnPart{{
				Kind: UserTurnPartEvent, Text: "The deck tilts.",
			}}),
		},
		{
			name: "narrator action-shaped narration",
			turn: UserTurnAuthority{
				PersonaKind: UserPersonaNarrator, PersonaName: NarratorPersonaName,
				ContributionKind: UserContributionNarration,
				Parts:            []UserTurnPart{{Kind: UserTurnPartAction, Text: "The gate opens."}},
				ExactText:        "[Action]\nThe gate opens.",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exact, got, err := test.turn.OngoingActionContribution()
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("action contribution=%t want %t", got, test.want)
			}
			if got && exact != test.exactContribution {
				t.Fatalf("exact action contribution=%q want %q", exact, test.exactContribution)
			}
			if !got && exact != "" {
				t.Fatalf("non-action contribution leaked %q", exact)
			}
		})
	}
}

func TestUserTurnCanonContributionExcludesTypedNarratorDirections(t *testing.T) {
	t.Parallel()
	direction := UserTurnAuthority{
		PersonaKind: UserPersonaNarrator, PersonaName: NarratorPersonaName,
		ContributionKind: UserContributionDirection,
		Parts:            []UserTurnPart{{Kind: UserTurnPartMessage, Text: "Make the confrontation brutal."}},
		ExactText:        "[Message]\nMake the confrontation brutal.",
	}
	if contribution, present, err := direction.CanonContribution(); err != nil || present || contribution != "" {
		t.Fatalf("direction canon contribution=%q present=%t error=%v", contribution, present, err)
	}

	mixed := UserTurnAuthority{
		PersonaKind: UserPersonaNarrator, PersonaName: NarratorPersonaName,
		ContributionKind: UserContributionNarrationDirection,
		Parts: []UserTurnPart{
			{Kind: UserTurnPartEvent, Text: "The north gate collapses."},
			{Kind: UserTurnPartMessage, Text: "Continue the violent siege."},
		},
		ExactText: "[Event]\nThe north gate collapses.\n\n[Message]\nContinue the violent siege.",
	}
	contribution, present, err := mixed.CanonContribution()
	if err != nil {
		t.Fatal(err)
	}
	if !present || contribution != "[Event]\nThe north gate collapses." {
		t.Fatalf("mixed canon contribution=%q present=%t", contribution, present)
	}
}

func exactCharacterUserTurn(
	kind UserContributionKind,
	parts []UserTurnPart,
) UserTurnAuthority {
	request := UserTurnRequest{
		PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID,
		ContributionKind: kind, Parts: parts,
	}
	exact, err := ComposeUserTurn(request)
	if err != nil {
		panic(err)
	}
	return UserTurnAuthority{
		PersonaKind: UserPersonaCharacter, CharacterID: testUserTurnCharacterID,
		PersonaName: "Gryph", PersonaSummary: "An artificer from afar.",
		ContributionKind: kind, Parts: append([]UserTurnPart(nil), parts...), ExactText: exact,
	}
}
