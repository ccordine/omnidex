package roleplay

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCharacterProjectionCarriesOnlyProjectedKnowledge(t *testing.T) {
	projection, err := newCharacterProjection(
		World{ID: "rpw_11111111111111111111111111111111", Name: "Harbor"},
		Character{ID: "rpc_22222222222222222222222222222222", Name: "Mara"},
		[]projectedEvent{
			{ID: "rpe_33333333333333333333333333333333", content: "The west gate is open."},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Authority != AuthorityCharacterKnowledge {
		t.Fatalf("authority=%q", projection.Authority)
	}
	if len(projection.Facts) != 1 || projection.Facts[0].Content != "The west gate is open." {
		t.Fatalf("facts=%#v", projection.Facts)
	}

	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"ordinal", "source_message_id", "canon_event_count"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("character projection leaked %q: %s", forbidden, encoded)
		}
	}
	if len(projection.Fingerprint) != 64 {
		t.Fatalf("fingerprint=%q", projection.Fingerprint)
	}

	repeated, err := newCharacterProjection(
		World{ID: projection.WorldID, Name: projection.WorldName},
		Character{ID: projection.CharacterID, Name: projection.CharacterName},
		[]projectedEvent{
			{ID: projection.Facts[0].EventID, content: projection.Facts[0].Content},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Fingerprint != projection.Fingerprint {
		t.Fatalf("projection fingerprint changed: %q != %q", repeated.Fingerprint, projection.Fingerprint)
	}
}

func TestRoleplayAuthorityNamespacesRemainDistinct(t *testing.T) {
	if AuthorityFictionalCanon == AuthorityCharacterKnowledge {
		t.Fatal("fictional canon and character knowledge must be distinct authorities")
	}
	if AuthorityFictionalCanon != "FICTIONAL_CANON" {
		t.Fatalf("canon authority=%q", AuthorityFictionalCanon)
	}
	if AuthorityCharacterKnowledge != "CHARACTER_KNOWLEDGE" {
		t.Fatalf("knowledge authority=%q", AuthorityCharacterKnowledge)
	}
}
