package roleplay

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCharacterLibraryCarriesProfileAndMemoriesAcrossIsolatedWorldPlacements(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	sourceWorld, source := bootstrapRoleplayChannel(
		t, pool, "portable-source", "Source World", "Mira", 1101,
	)
	targetWorld, _ := bootstrapRoleplayChannel(
		t, pool, "portable-target", "Target World", "Guide", 1201,
	)
	profile, err := store.WritePersona(ctx, PersonaWriteRequest{
		CharacterID: source.ID,
		Sheet: PersonaSheet{
			Summary: "A patient clockmaker.", Voice: "Warm and precise.",
			Traits: []string{"observant"}, Goals: []string{"repair the beacon"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendCanonEvent(ctx, sourceWorld.ID, 1101, "Mira repaired the western beacon.")
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.GrantKnowledge(ctx, source.ID, event.ID); err != nil || !created {
		t.Fatalf("grant source knowledge: created=%t error=%v", created, err)
	}
	if _, err := store.AppendCharacterMemory(ctx, source.ID, event.ID, "I repaired the western beacon in Source World."); err != nil {
		t.Fatal(err)
	}

	placed, err := store.PlaceLibraryCharacter(ctx, targetWorld.ID, source.LibraryID)
	if err != nil {
		t.Fatal(err)
	}
	if placed.LibraryID != source.LibraryID || placed.ID == source.ID || placed.Name != source.Name {
		t.Fatalf("portable placement=%+v source=%+v", placed, source)
	}
	targetProfile, err := store.ProjectPersona(ctx, placed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if targetProfile.CharacterID != placed.ID || targetProfile.Revision != profile.Revision ||
		targetProfile.Sheet.Summary != profile.Sheet.Summary {
		t.Fatalf("target profile=%+v source profile=%+v", targetProfile, profile)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	memories, err := loadCharacterMemoriesTx(ctx, tx, targetWorld.ID, placed.ID)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].Content != "I repaired the western beacon in Source World." {
		t.Fatalf("portable memories=%+v", memories)
	}
	targetCanon, err := store.ProjectCharacterContext(ctx, placed.ID, MaxProjectionEvents)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetCanon.Facts) != 0 {
		t.Fatalf("source-world canon leaked into target world: %+v", targetCanon.Facts)
	}

	if _, err := store.PlaceLibraryCharacter(ctx, targetWorld.ID, source.LibraryID); !errors.Is(err, ErrSimulationConflict) {
		t.Fatalf("duplicate placement error=%v, want conflict", err)
	}
	page, err := store.ListLibraryCharactersPage(ctx, targetWorld.ID, MaxCharacterLibraryPageSize, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, character := range page.Items {
		if character.ID != source.LibraryID {
			continue
		}
		found = true
		if !character.PlacedInSelectedWorld || character.PlacementCount != 2 ||
			character.MemoryCount != 1 || character.Profile == nil ||
			character.Profile.Summary != profile.Sheet.Summary {
			t.Fatalf("library summary=%+v", character)
		}
	}
	if !found {
		t.Fatalf("library page omitted %s", source.LibraryID)
	}
}
