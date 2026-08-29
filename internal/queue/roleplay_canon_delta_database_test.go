package queue

import (
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonDeltaFiltersHiddenWorldFactWithoutProjectingIt(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "canon-delta-hidden", Scope: model.ChannelScopeUser, Name: "Canon delta hidden",
		WorkspaceRoot: "/srv/workspaces/canon-delta-hidden", Mode: model.ChannelModeRoleplay,
	}, "Delta world", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world found=%t err=%v", found, err)
	}
	var sourceMessageID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO ai_channel_messages (channel_id,role,content)
		VALUES ($1,'assistant','A hidden accepted source.') RETURNING id
	`, channel.ID).Scan(&sourceMessageID); err != nil {
		t.Fatal(err)
	}
	const hiddenFact = "The archive key is beneath the western stair."
	if _, err := store.AppendCanonEvent(ctx, world.ID, sourceMessageID, hiddenFact); err != nil {
		t.Fatal(err)
	}
	projection, err := repository.ProjectRoleplayCharacterContext(
		ctx, channel.ID, channel.RoleplayViewpointCharacterID, roleplay.MaxProjectionEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Facts) != 0 {
		t.Fatalf("hidden world fact leaked to viewpoint projection: %#v", projection.Facts)
	}
	const newFact = "Rain began above the western stair."
	filtered, err := repository.FilterNewRoleplayCanonFacts(
		ctx, world.ID, []string{hiddenFact, newFact},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(filtered, []string{newFact}) {
		t.Fatalf("exact world-global delta=%v", filtered)
	}
}
