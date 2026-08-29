package roleplay

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRoleplayUserCanonDeferredReceiptRejectsMissingAndPartialMaterialization(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	ctx := context.Background()
	world, viewpoint := bootstrapRoleplayChannel(
		t, pool, "user-canon-deferred", "Deferred canon", "Mara",
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
		ID: sceneID, WorldID: world.ID, Title: "Bell room",
		Description:    "A room holding one bronze bell.",
		ParticipantIDs: []string{viewpoint.ID},
	}); err != nil {
		t.Fatal(err)
	}
	const (
		messageID = int64(9101)
		jobID     = int64(9102)
		fact      = "The bronze bell cracked."
	)
	exact := insertNarratorRoleplayUserMessage(
		t, pool, messageID, "user-canon-deferred", "The bronze bell cracked.",
		UserContributionNarration,
	)
	preparation := prepareAndBindUncompletedTestTurn(
		t, pool, "user-canon-deferred", messageID, jobID, exact,
	)
	t.Run("event outside receipt", func(t *testing.T) {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		operationID := "lifecycle_operation_" + strings.Repeat("d", 64)
		factsJSON, _ := json.Marshal([]string{fact})
		recipientsJSON, _ := json.Marshal([]string{viewpoint.ID})
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_user_canon_completions (
				operation_id,preparation_id,world_id,source_message_id,
				persona_kind,actor_character_id,facts,knowledge_character_ids
			) VALUES ($1,$2,$3,$4,'narrator',NULL,$5::jsonb,$6::jsonb)
		`, operationID, preparation.PreparationID, world.ID, messageID,
			string(factsJSON), string(recipientsJSON)); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO roleplay_canon_events (
				id,world_id,source_message_id,content
			) VALUES ($1,$2,$3,$4)
		`, "rpe_"+strings.Repeat("d", 32), world.ID, messageID,
			"The bronze bell remained intact.")
		if err == nil || !strings.Contains(
			err.Error(), "exact receipt-backed user contribution",
		) {
			t.Fatalf("event outside exact user receipt error=%v", err)
		}
	})

	for _, testCase := range []struct {
		name                 string
		hexDigit             string
		insertEventKnowledge bool
		memoryContent        string
	}{
		{name: "missing events", hexDigit: "a"},
		{name: "missing memory", hexDigit: "b", insertEventKnowledge: true},
		{
			name: "mismatched memory", hexDigit: "c", insertEventKnowledge: true,
			memoryContent: "A different remembered event.",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			operationID := "lifecycle_operation_" + strings.Repeat(testCase.hexDigit, 64)
			factsJSON, _ := json.Marshal([]string{fact})
			recipientsJSON, _ := json.Marshal([]string{viewpoint.ID})
			if _, err := tx.Exec(ctx, `
				INSERT INTO roleplay_user_canon_completions (
					operation_id,preparation_id,world_id,source_message_id,
					persona_kind,actor_character_id,facts,knowledge_character_ids
				) VALUES ($1,$2,$3,$4,'narrator',NULL,$5::jsonb,$6::jsonb)
			`, operationID, preparation.PreparationID, world.ID, messageID,
				string(factsJSON), string(recipientsJSON)); err != nil {
				t.Fatal(err)
			}
			if testCase.insertEventKnowledge {
				eventID := "rpe_" + strings.Repeat(testCase.hexDigit, 32)
				knowledgeID := "rpk_" + strings.Repeat(testCase.hexDigit, 32)
				if _, err := tx.Exec(ctx, `
					INSERT INTO roleplay_canon_events (
						id,world_id,source_message_id,content
					) VALUES ($1,$2,$3,$4)
				`, eventID, world.ID, messageID, fact); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO roleplay_character_knowledge (
						id,world_id,character_id,canon_event_id
					) VALUES ($1,$2,$3,$4)
				`, knowledgeID, world.ID, viewpoint.ID, eventID); err != nil {
					t.Fatal(err)
				}
				if testCase.memoryContent != "" {
					memoryID := "rpm_" + strings.Repeat(testCase.hexDigit, 32)
					if _, err := tx.Exec(ctx, `
						INSERT INTO roleplay_character_memories (
							id,world_id,character_id,source_event_id,content
						) VALUES ($1,$2,$3,$4,$5)
					`, memoryID, world.ID, viewpoint.ID, eventID,
						testCase.memoryContent); err != nil {
						t.Fatal(err)
					}
				}
			}
			payload, err := json.Marshal(map[string]any{
				"operation_id": operationID, "step_id": jobID, "output": "response",
				"context_key": "", "context_value": "",
				"roleplay_responses": []any{},
				"roleplay_user_canon": map[string]any{
					"facts": []string{fact}, "knowledge_character_ids": []string{viewpoint.ID},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO lifecycle_operation_registry (
					operation_id,kind,command_sha256,command_payload
				) VALUES ($1,'complete_step',repeat('f',64),$2::jsonb)
			`, operationID, string(payload)); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO job_lifecycle_operations (
					operation_id,job_id,observed_generation,result_generation,step_id,
					kind,command_sha256,command_payload,
					result_job_status,result_step_status,result_job
				) VALUES (
					$1,$2,1,1,$2,'complete_step',repeat('f',64),$3::jsonb,
					'completed','completed',
					jsonb_build_object('id',$2::bigint,'current_generation',1,'status','completed')
				)
			`, operationID, jobID, string(payload)); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, `
				SET CONSTRAINTS roleplay_lifecycle_requires_user_canon_receipt IMMEDIATE
			`)
			if err == nil || !strings.Contains(
				err.Error(), "roleplay user canon lifecycle receipt differs from exact command",
			) {
				t.Fatalf("deferred %s materialization error=%v", testCase.name, err)
			}
		})
	}

	var completions int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM roleplay_user_canon_completions
	`).Scan(&completions); err != nil {
		t.Fatal(err)
	}
	if completions != 0 {
		t.Fatalf("rejected deferred receipts persisted %d rows", completions)
	}
}
