package roleplay

import (
	"errors"
	"testing"
)

func TestUserTurnSceneAuthorityRequiresCurrentCharacter(t *testing.T) {
	activeID := "rpc_11111111111111111111111111111111"
	personaID := "rpc_22222222222222222222222222222222"
	participants := []string{activeID, personaID}
	validCharacter := UserTurnAuthority{
		PersonaKind: UserPersonaCharacter, CharacterID: personaID,
	}
	if err := validateUserTurnSceneAuthority(validCharacter, participants); err != nil {
		t.Fatal(err)
	}
	if err := validateUserTurnSceneAuthority(UserTurnAuthority{
		PersonaKind: UserPersonaCharacter, CharacterID: activeID,
	}, participants); err != nil {
		t.Fatalf("active initiative character was rejected as the acting persona: %v", err)
	}
	if err := validateUserTurnSceneAuthority(UserTurnAuthority{
		PersonaKind: UserPersonaNarrator,
	}, participants); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []UserTurnAuthority{{
		PersonaKind: UserPersonaCharacter,
		CharacterID: "rpc_33333333333333333333333333333333",
	}} {
		if err := validateUserTurnSceneAuthority(invalid, participants); !errors.Is(err, ErrSimulationIllegal) {
			t.Fatalf("invalid persona %+v error=%v", invalid, err)
		}
	}
}
