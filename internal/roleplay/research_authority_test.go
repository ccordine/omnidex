package roleplay

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestResearchTurnAuthorityKeepsRealWorldAndFictionalNamespacesSeparate(t *testing.T) {
	t.Parallel()
	question := "How does yeast make bread rise?"
	digest := sha256.Sum256([]byte(question))
	authority := ResearchTurnAuthority{
		Schema:               ResearchTurnAuthoritySchemaV1,
		PreparationID:        "rpt_11111111111111111111111111111111",
		ChannelID:            "story-channel",
		UserMessageID:        7,
		WorldID:              "rpw_22222222222222222222222222222222",
		SceneID:              "rps_33333333333333333333333333333333",
		SceneRevision:        4,
		CharacterID:          "rpc_44444444444444444444444444444444",
		Capability:           CapabilityWebResearch,
		CapabilityGrantID:    "rpg_55555555555555555555555555555555",
		Question:             question,
		QuestionSHA256:       hex.EncodeToString(digest[:]),
		NarrativeFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Authority:            AuthorityRealWorld,
		CapabilityIssuedAt:   time.Unix(1, 0).UTC(),
		CreatedAt:            time.Unix(2, 0).UTC(),
	}
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	authority.Authority = AuthorityFictionalCanon
	if err := authority.Validate(); err == nil {
		t.Fatal("real-world research authority was accepted as fictional canon")
	}
}
