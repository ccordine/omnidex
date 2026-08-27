package roleplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserPersonaSceneMigrationRejectsContradictoryRetainedTurnAtomically(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	ctx := t.Context()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, first := bootstrapRoleplayChannel(
		t, pool, "persona-upgrade", "Persona upgrade world", "First",
	)
	second, err := store.CreateCharacter(ctx, world.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range []Character{first, second} {
		writeTestPersona(t, store, character.ID, character.Name+" retained participant.")
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Retained persona scene",
		Description:    "A turn is retained while its persona leaves the cast.",
		ParticipantIDs: []string{first.ID, second.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	insertHistoricalCharacterActionMessage(
		t, pool, world.ChannelID, 821, second.ID, "I leave before preparation.",
	)
	if _, err := store.UpdateCurrentScene(ctx, SceneUpdate{
		SceneID: sceneID, WorldID: world.ID, ExpectedRevision: scene.Revision,
		Title:          "Retained persona scene",
		Description:    "A turn is retained while its persona leaves the cast.",
		ParticipantIDs: []string{first.ID},
	}); err != nil {
		t.Fatal(err)
	}

	var beforeDefinition string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_functiondef(
		    'validate_roleplay_user_turn_insert()'::regprocedure
		)
	`).Scan(&beforeDefinition); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(
		"..", "..", "migrations", "150_roleplay_user_persona_scene_authority.sql",
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err == nil || !strings.Contains(
		err.Error(), "retained character turn without exact prepared scene authority",
	) {
		t.Fatalf("contradictory retained persona migration error=%v", err)
	}
	var afterDefinition string
	var retainedTurns int
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_functiondef(
		           'validate_roleplay_user_turn_insert()'::regprocedure
		       ),
		       (SELECT COUNT(*) FROM roleplay_user_turns WHERE user_message_id=821)
	`).Scan(&afterDefinition, &retainedTurns); err != nil {
		t.Fatal(err)
	}
	if afterDefinition != beforeDefinition || retainedTurns != 1 {
		t.Fatalf(
			"rejected persona migration definitionChanged=%t retainedTurns=%d",
			afterDefinition != beforeDefinition, retainedTurns,
		)
	}
}
