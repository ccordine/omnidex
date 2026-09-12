package queue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func requireRoleplayPersonaValueFreshness(t *testing.T, store *roleplay.Store, prepared roleplay.SimulationTurnAuthority, jobID int64) {
	t.Helper()
	ctx := context.Background()
	viewpoint := prepared.NarrativeProjection.Viewpoint
	original := roleplay.PersonaSheet{
		Summary: viewpoint.Summary, Voice: viewpoint.Voice,
		Traits: append([]string{}, viewpoint.Traits...), Goals: append([]string{}, viewpoint.Goals...),
	}
	changed := original
	changed.Voice = "Speak quietly."
	updated, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
		CharacterID: prepared.ActiveCharacterID, ExpectedRevision: 1, Sheet: changed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadFreshSimulationTurnForJob(ctx, prepared.PreparationID, jobID); !errors.Is(err, roleplay.ErrSimulationStaleRevision) ||
		!strings.Contains(err.Error(), "responding character") {
		t.Fatalf("changed persona bytes were not identified before narrative work: %v", err)
	}
	if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
		CharacterID: prepared.ActiveCharacterID, ExpectedRevision: updated.Revision, Sheet: original,
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadFreshSimulationTurnForJob(ctx, prepared.PreparationID, jobID)
	if err != nil || !reflect.DeepEqual(loaded, prepared) {
		t.Fatalf("restoring exact persona content did not retain accepted preparation: error=%v\nwant=%#v\ngot=%#v", err, prepared, loaded)
	}
}

func requireRoleplayProjectionValues(t *testing.T, store *roleplay.Store, channel model.Channel, world roleplay.World, content string) {
	t.Helper()
	ctx := context.Background()
	visible, err := store.ProjectChannelCharacterContext(ctx, string(channel.ID), string(channel.RoleplayViewpointCharacterID), 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := visible.Validate(); err != nil || len(visible.Facts) != 1 || visible.Facts[0].Content != content {
		t.Fatalf("character projection differs from its actual source: %#v / %v", visible, err)
	}
	canon, err := store.ProjectCanonContext(ctx, world.ID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := canon.Validate(); err != nil || !reflect.DeepEqual(canon.Facts, visible.Facts) || canon.Authority != roleplay.AuthorityFictionalCanon {
		t.Fatalf("canon projection differs from its actual source: %#v / %v", canon, err)
	}
	other, err := store.CreateCharacter(ctx, world.ID, "Other observer")
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := store.ProjectChannelCharacterContext(ctx, string(channel.ID), other.ID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := hidden.Validate(); err != nil || len(hidden.Facts) != 0 || hidden.Authority != roleplay.AuthorityCharacterKnowledge {
		t.Fatalf("ungranted facts leaked into another character: %#v / %v", hidden, err)
	}
	for _, value := range []any{visible, canon, hidden} {
		raw, err := json.Marshal(value)
		if err != nil || strings.Contains(string(raw), "fingerprint") {
			t.Fatalf("context projection retained a fingerprint: %s / %v", raw, err)
		}
	}
}
