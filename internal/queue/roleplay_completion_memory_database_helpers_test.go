package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func assertRoleplayCompletionMemoryAndVisibility(
	t *testing.T,
	repository *Repository,
	store *roleplay.Store,
	channel model.Channel,
	worldID string,
	witnessID string,
	eventID string,
	fact string,
	command CompleteStepCommand,
) {
	t.Helper()
	ctx := t.Context()
	witnessProjection, err := repository.ProjectRoleplayCharacterContext(
		ctx, channel.ID, model.RoleplayCharacterID(witnessID), roleplay.MaxProjectionEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(witnessProjection.Facts) != 0 {
		t.Fatalf("non-viewpoint participant received hidden knowledge=%#v", witnessProjection)
	}
	reopenedStore, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	viewpointNarrative, _, err := reopenedStore.ProjectSimulationNarrative(
		ctx, worldID, string(channel.RoleplayViewpointCharacterID),
	)
	if err != nil {
		t.Fatal(err)
	}
	witnessNarrative, _, err := reopenedStore.ProjectSimulationNarrative(ctx, worldID, witnessID)
	if err != nil {
		t.Fatal(err)
	}
	if len(viewpointNarrative.Memories) != 1 || viewpointNarrative.Memories[0] != fact ||
		len(witnessNarrative.Memories) != 0 {
		t.Fatalf("cross-session memory isolation viewpoint=%v witness=%v",
			viewpointNarrative.Memories, witnessNarrative.Memories)
	}
	var sourceRole string
	if err := repository.pool.QueryRow(ctx, `
		SELECT message.role
		FROM roleplay_canon_events AS event
		JOIN ai_channel_messages AS message ON message.id=event.source_message_id
		WHERE event.id=$1
	`, eventID).Scan(&sourceRole); err != nil {
		t.Fatal(err)
	}
	if sourceRole != string(model.ChannelMessageRoleAssistant) {
		t.Fatalf("canon source role=%q", sourceRole)
	}
	if _, err := repository.pool.Exec(ctx, `
		ALTER TABLE roleplay_character_memories
		DISABLE TRIGGER roleplay_character_memories_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		DELETE FROM roleplay_character_memories WHERE source_event_id=$1
	`, eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		ALTER TABLE roleplay_character_memories
		ENABLE TRIGGER roleplay_character_memories_immutable
	`); err != nil {
		t.Fatal(err)
	}
	completion := CompleteStepEvidenceCommand{CompleteStepCommand: command, Evidence: nil}
	if err := repository.CompleteStepWithEvidence(ctx, completion); err == nil ||
		!strings.Contains(err.Error(), "inconsistent roleplay completion character memories") {
		t.Fatalf("tampered roleplay memory replay error=%v", err)
	}
	if _, err := store.AppendCharacterMemory(
		ctx, string(channel.RoleplayViewpointCharacterID), eventID, fact,
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
		t.Fatalf("restored exact roleplay completion replay: %v", err)
	}
	assertRoleplayCompletionReceiptImmutability(t, repository, command, eventID)
}

func assertRoleplayCompletionReceiptImmutability(
	t *testing.T,
	repository *Repository,
	command CompleteStepCommand,
	eventID string,
) {
	t.Helper()
	ctx := t.Context()
	for _, mutation := range []struct {
		statement string
		arguments []any
	}{
		{`UPDATE roleplay_turn_completions SET facts='[]'::jsonb WHERE operation_id=$1`, []any{command.OperationID}},
		{`DELETE FROM roleplay_turn_completions WHERE operation_id=$1`, []any{command.OperationID}},
		{`TRUNCATE roleplay_turn_completions`, nil},
	} {
		if _, err := repository.pool.Exec(ctx, mutation.statement, mutation.arguments...); err == nil ||
			!strings.Contains(err.Error(), "immutable") {
			t.Fatalf("mutable roleplay completion receipt statement=%q error=%v", mutation.statement, err)
		}
	}
	if _, err := repository.pool.Exec(ctx, `
		ALTER TABLE roleplay_character_knowledge
		DISABLE TRIGGER roleplay_character_knowledge_immutable
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		DELETE FROM roleplay_character_knowledge WHERE canon_event_id=$1
	`, eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		ALTER TABLE roleplay_character_knowledge
		ENABLE TRIGGER roleplay_character_knowledge_immutable
	`); err != nil {
		t.Fatal(err)
	}
	completion := CompleteStepEvidenceCommand{CompleteStepCommand: command, Evidence: nil}
	if err := repository.CompleteStepWithEvidence(ctx, completion); err == nil ||
		!strings.Contains(err.Error(), "inconsistent roleplay completion character knowledge") {
		t.Fatalf("tampered roleplay completion replay error=%v", err)
	}
}
