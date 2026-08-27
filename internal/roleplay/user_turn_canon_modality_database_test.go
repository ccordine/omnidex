package roleplay

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRoleplayUserCanonDirectionHasNoReceiptAuthority(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	ctx := context.Background()
	world, viewpoint := bootstrapRoleplayChannel(
		t, pool, "user-canon-direction", "Direction world", "Mara",
	)
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	writeTestPersona(t, store, viewpoint.ID, "A careful witness.")
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Quiet hall",
		Description:    "A hall awaiting the next fictional response.",
		ParticipantIDs: []string{viewpoint.ID},
	}); err != nil {
		t.Fatal(err)
	}
	const (
		messageID = int64(9201)
		jobID     = int64(9202)
	)
	exact := insertNarratorRoleplayUserMessage(
		t, pool, messageID, world.ChannelID, "Continue toward the archway.",
		UserContributionDirection,
	)
	preparation := prepareAndBindTestTurn(
		t, pool, world.ChannelID, messageID, jobID, exact,
	)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	operationID := "lifecycle_operation_" + strings.Repeat("e", 64)
	if _, err := AppendUserTurnCanonTx(
		ctx, tx, operationID, preparation.PreparationID,
		world.ChannelID, messageID, []string{}, []string{},
	); err == nil || !strings.Contains(
		err.Error(), "source differs from frozen turn authority",
	) {
		t.Fatalf("direction user canon append error=%v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_user_canon_completions (
			operation_id,preparation_id,world_id,source_message_id,
			persona_kind,actor_character_id,facts,knowledge_character_ids
		) VALUES ($1,$2,$3,$4,'narrator',NULL,'[]'::jsonb,'[]'::jsonb)
	`, operationID, preparation.PreparationID, world.ID, messageID); err == nil ||
		!strings.Contains(err.Error(), "differs from frozen user-turn authority") {
		t.Fatalf("direct direction canon receipt error=%v", err)
	}
}

func TestRoleplayUserCanonModalityPredicateMatrix(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	rows, err := pool.Query(t.Context(), `
		SELECT label,roleplay_user_turn_requires_canon(persona,contribution,parts)
		FROM (VALUES
			('character', 'character','dialogue',
			 '[{"kind":"message","text":"I speak."}]'::jsonb,TRUE),
			('legacy narration', 'narrator','narration','[]'::jsonb,TRUE),
			('event', 'narrator','narration',
			 '[{"kind":"event","text":"Rain begins."}]'::jsonb,TRUE),
			('mixed action', 'narrator','narration_direction',
			 '[{"kind":"message","text":"Continue."},{"kind":"action","text":"The door opens."}]'::jsonb,TRUE),
			('direction', 'narrator','direction',
			 '[{"kind":"message","text":"Continue."}]'::jsonb,FALSE),
			('command', 'narrator','command','[]'::jsonb,FALSE),
			('legacy', 'legacy_untyped','legacy_untyped','[]'::jsonb,FALSE),
			('message only', 'narrator','narration_direction',
			 '[{"kind":"message","text":"Only direction."}]'::jsonb,FALSE)
		) AS fixture(label,persona,contribution,parts,want)
		WHERE roleplay_user_turn_requires_canon(persona,contribution,parts)
		      IS DISTINCT FROM want
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var mismatches []string
	for rows.Next() {
		var label string
		var got bool
		if err := rows.Scan(&label, &got); err != nil {
			t.Fatal(err)
		}
		mismatches = append(mismatches, label)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 0 {
		t.Fatalf("roleplay user canon modality mismatches=%v", mismatches)
	}
}
