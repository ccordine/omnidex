package roleplay

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCharacterLibraryMigrationBackfillsAnExistingPopulatedWorld(t *testing.T) {
	pool, _ := openRoleplayTestPoolWithMigrations(t, []string{
		"117_roleplay_canon_authority.sql",
		"118_roleplay_simulation_authority.sql",
	})
	ctx := context.Background()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	const (
		worldID     = "rpw_11111111111111111111111111111111"
		characterID = "rpc_22222222222222222222222222222222"
	)
	for _, statement := range []string{
		`INSERT INTO ai_channels (id,mode,roleplay_viewpoint_character_id)
		 VALUES ('existing-world','roleplay','` + characterID + `')`,
		`INSERT INTO roleplay_worlds (id,channel_id,name)
		 VALUES ('` + worldID + `','existing-world','Existing World')`,
		`INSERT INTO roleplay_characters (id,world_id,name)
		 VALUES ('` + characterID + `','` + worldID + `','Mira')`,
		`INSERT INTO roleplay_character_personas (world_id,character_id,summary,voice,traits,goals)
		 VALUES ('` + worldID + `','` + characterID + `','An existing traveler.','Quiet.','["patient"]','["remember"]')`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "122_roleplay_character_library.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("upgrade populated roleplay schema: %v", err)
	}
	var libraryID, profileSummary string
	if err := pool.QueryRow(ctx, `
		SELECT character.library_character_id,profile.summary
		FROM roleplay_characters AS character
		JOIN roleplay_character_profiles AS profile
		  ON profile.library_character_id=character.library_character_id
		WHERE character.id=$1
	`, characterID).Scan(&libraryID, &profileSummary); err != nil {
		t.Fatal(err)
	}
	if libraryID != "rpl_b657698b565585c3c432121b2905953b" || profileSummary != "An existing traveler." {
		t.Fatalf("backfill library=%q profile=%q", libraryID, profileSummary)
	}
	if _, err := pool.Exec(ctx, `UPDATE roleplay_characters SET name='Changed' WHERE id=$1`, characterID); err == nil {
		t.Fatal("post-upgrade character binding accepted an inexact library name")
	}
}

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
