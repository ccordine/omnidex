package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

// Current code knows the post-157 modality rule. This bridge lets it publish
// one receipt through the genuine pre-157 database authority, then restores
// the exact direction binding before the migration runs. It is test-only and
// leaves the accepted lifecycle command and receipt untouched.
func installHistoricalDirectionCompletionBridge(
	t *testing.T,
	repository *Repository,
	job model.Job,
) func() {
	t.Helper()
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	var preparation, turnParts []byte
	var contributionKind string
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT preparation.result,user_turn.contribution_kind,user_turn.parts
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_user_turns AS user_turn
		  ON user_turn.user_message_id=preparation.user_message_id
		 AND user_turn.channel_id=preparation.channel_id
		 AND user_turn.world_id=preparation.world_id
		WHERE preparation.operation_id=$1
	`, binding.RoleplaySimulationPreparationID).Scan(
		&preparation, &contributionKind, &turnParts,
	); err != nil {
		t.Fatal(err)
	}
	setHistoricalCompletionBridge(
		t, repository, job.ID, binding.RoleplaySimulationPreparationID,
		binding.ChannelUserMessageID,
	)
	restored := false
	return func() {
		if restored {
			t.Fatal("historical direction completion bridge restored twice")
		}
		restored = true
		tx, err := repository.pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(t.Context())
		for _, statement := range historicalBridgeDropTriggers {
			if _, err := tx.Exec(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.Exec(t.Context(), `
			UPDATE jobs SET metadata=$2::jsonb WHERE id=$1
		`, job.ID, string(job.Metadata)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(t.Context(), `
			UPDATE roleplay_simulation_turn_preparations SET result=$2::jsonb
			WHERE operation_id=$1
		`, binding.RoleplaySimulationPreparationID, string(preparation)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(t.Context(), `
			UPDATE roleplay_user_turns SET contribution_kind=$2,parts=$3::jsonb
			WHERE user_message_id=$1
		`, binding.ChannelUserMessageID, contributionKind, string(turnParts)); err != nil {
			t.Fatal(err)
		}
		for _, statement := range historicalBridgeCreateTriggers {
			if _, err := tx.Exec(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.Exec(t.Context(),
			`DROP FUNCTION roleplay_user_turn_requires_canon(TEXT,TEXT,JSONB)`,
		); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}

func setHistoricalCompletionBridge(
	t *testing.T,
	repository *Repository,
	jobID int64,
	preparationID string,
	userMessageID int64,
) {
	t.Helper()
	tx, err := repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	for _, statement := range historicalBridgeDropTriggers {
		if _, err := tx.Exec(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE roleplay_user_turns
		SET contribution_kind='narration',parts='[]'::jsonb
		WHERE user_message_id=$1
	`, userMessageID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE jobs SET metadata=jsonb_set(
		    jsonb_set(metadata,'{roleplay_user_turn,contribution_kind}',
		              '"narration"'::jsonb),
		    '{roleplay_user_turn,parts}','[]'::jsonb
		) WHERE id=$1
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		UPDATE roleplay_simulation_turn_preparations SET result=jsonb_set(
		    jsonb_set(result,'{user_turn,contribution_kind}',
		              '"narration"'::jsonb),
		    '{user_turn,parts}','[]'::jsonb
		) WHERE operation_id=$1
	`, preparationID); err != nil {
		t.Fatal(err)
	}
	for _, statement := range historicalBridgeCreateTriggers {
		if _, err := tx.Exec(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(t.Context(), `
		CREATE FUNCTION roleplay_user_turn_requires_canon(TEXT,TEXT,JSONB)
		RETURNS BOOLEAN AS $function$
		    SELECT $2 NOT IN ('command','legacy_untyped');
		$function$ LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

var historicalBridgeDropTriggers = []string{
	`DROP TRIGGER jobs_chat_turn_binding_immutable ON jobs`,
	`DROP TRIGGER roleplay_simulation_preparations_immutable
	 ON roleplay_simulation_turn_preparations`,
	`DROP TRIGGER roleplay_user_turns_immutable ON roleplay_user_turns`,
}

var historicalBridgeCreateTriggers = []string{
	`CREATE TRIGGER roleplay_user_turns_immutable
	 BEFORE UPDATE OR DELETE ON roleplay_user_turns
	 FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation()`,
	`CREATE TRIGGER roleplay_simulation_preparations_immutable
	 BEFORE UPDATE OR DELETE ON roleplay_simulation_turn_preparations
	 FOR EACH ROW EXECUTE FUNCTION reject_roleplay_append_authority_mutation()`,
	`CREATE TRIGGER jobs_chat_turn_binding_immutable
	 BEFORE UPDATE OF pipeline,metadata ON jobs
	 FOR EACH ROW EXECUTE FUNCTION reject_chat_turn_binding_update()`,
}

func assertHistoricalDirectionCanonVisibility(
	t *testing.T,
	repository *Repository,
	worldID string,
	messageID int64,
	fact string,
	recipients []model.RoleplayCharacterID,
) {
	t.Helper()
	var events int
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM roleplay_canon_events
		WHERE world_id=$1 AND source_message_id=$2 AND content=$3
	`, worldID, messageID, fact).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("historical direction canon event count=%d want 1", events)
	}
	for _, recipient := range recipients {
		var knowledge, memory int
		if err := repository.pool.QueryRow(t.Context(), `
			SELECT
			 (SELECT COUNT(*) FROM roleplay_canon_events AS event
			  JOIN roleplay_character_knowledge AS knowledge
			    ON knowledge.world_id=event.world_id AND knowledge.canon_event_id=event.id
			  WHERE event.world_id=$1 AND event.source_message_id=$2
			    AND event.content=$3 AND knowledge.character_id=$4),
			 (SELECT COUNT(*) FROM roleplay_canon_events AS event
			  JOIN roleplay_character_memories AS memory
			    ON memory.world_id=event.world_id AND memory.source_event_id=event.id
			  WHERE event.world_id=$1 AND event.source_message_id=$2
			    AND event.content=$3 AND memory.character_id=$4 AND memory.content=$3)
		`, worldID, messageID, fact, recipient).Scan(&knowledge, &memory); err != nil {
			t.Fatal(err)
		}
		if knowledge != 1 || memory != 1 {
			t.Fatalf(
				"historical direction recipient %s knowledge/memory=%d/%d",
				recipient, knowledge, memory,
			)
		}
	}
}
