package worker

import "github.com/gryph/omnidex/internal/roleplay"

func roleplayGenerationFixture(suffix string) roleplay.CharacterGenerationConfig {
	return roleplay.CharacterGenerationConfig{
		Schema:             roleplay.CharacterGenerationConfigSchemaV2,
		LibraryCharacterID: "rpl_" + suffix,
		Revision:           1,
	}
}
