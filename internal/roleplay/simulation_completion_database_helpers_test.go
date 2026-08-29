package roleplay

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type preparedRoleplayTestResponse struct {
	Position              int      `json:"position"`
	CharacterID           string   `json:"character_id"`
	Output                string   `json:"output"`
	Facts                 []string `json:"facts"`
	KnowledgeCharacterIDs []string `json:"knowledge_character_ids"`
}

func completePreparedRoleplayTestJobTx(
	ctx context.Context,
	tx pgx.Tx,
	authority SimulationTurnAuthority,
	jobID int64,
) error {
	const workerID = "roleplay-current-state-test"
	responses := make([]preparedRoleplayTestResponse, len(authority.ResponderRoutes))
	outputs := make([]string, len(authority.ResponderRoutes))
	for index, route := range authority.ResponderRoutes {
		output := fmt.Sprintf("Prepared response %d.", index+1)
		responses[index] = preparedRoleplayTestResponse{
			Position: route.Position, CharacterID: route.CharacterID, Output: output,
			Facts: []string{}, KnowledgeCharacterIDs: []string{},
		}
		outputs[index] = output
	}
	if len(responses) == 0 {
		return fmt.Errorf("prepared roleplay test job has no response route")
	}

	operationDigest := sha256.Sum256([]byte(fmt.Sprintf(
		"roleplay-current-state-test\x00%s\x00%d", authority.PreparationID, jobID,
	)))
	operationID := fmt.Sprintf("lifecycle_operation_%x", operationDigest[:])
	command := map[string]any{
		"operation_id":       operationID,
		"step_id":            jobID,
		"output":             strings.Join(outputs, "\n\n"),
		"context_key":        "objective_result",
		"context_value":      "roleplay-current-state-test",
		"roleplay_responses": responses,
	}
	_, requiresUserCanon, err := authority.UserTurn.CanonContribution()
	if err != nil {
		return err
	}
	if requiresUserCanon {
		command["roleplay_user_canon"] = map[string]any{
			"facts": []string{}, "knowledge_character_ids": []string{},
		}
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("encode prepared roleplay completion: %w", err)
	}
	commandDigest := sha256.Sum256(payload)
	commandSHA256 := fmt.Sprintf("%x", commandDigest[:])

	stepID := jobID
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,status
		) VALUES ($1,1,$2,1,$3,'active')
	`, jobID, stepID, workerID); err != nil {
		return fmt.Errorf("insert prepared roleplay test attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status='running',worker_id=$2,current_attempt=1,
		    started_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1
	`, stepID, workerID); err != nil {
		return fmt.Errorf("start prepared roleplay test step: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_step_attempts
		SET status='completed',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=1 AND step_id=$2 AND attempt=1
	`, jobID, stepID); err != nil {
		return fmt.Errorf("complete prepared roleplay test attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status='completed',output=$2,finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1
	`, stepID, command["output"]); err != nil {
		return fmt.Errorf("complete prepared roleplay test step: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status='completed',result=$2,completed_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1
	`, jobID, command["output"]); err != nil {
		return fmt.Errorf("complete prepared roleplay test job: %w", err)
	}

	var contextID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO step_contexts (step_id,key,value)
		VALUES ($1,'objective_result','roleplay-current-state-test')
		RETURNING id
	`, stepID).Scan(&contextID); err != nil {
		return fmt.Errorf("insert prepared roleplay test context: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO lifecycle_operation_registry (
			operation_id,kind,command_sha256,command_payload
		) VALUES ($1,'complete_step',$2,$3::jsonb)
	`, operationID, commandSHA256, string(payload)); err != nil {
		return fmt.Errorf("register prepared roleplay test completion: %w", err)
	}
	resultJob, err := json.Marshal(map[string]any{
		"id": jobID, "current_generation": 1, "status": "completed",
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_lifecycle_operations (
			operation_id,job_id,observed_generation,result_generation,
			step_id,step_context_id,kind,command_sha256,command_payload,
			result_job_status,result_step_status,result_job
		) VALUES (
			$1,$2,1,1,$3,$4,'complete_step',$5,$6::jsonb,
			'completed','completed',$7::jsonb
		)
	`, operationID, jobID, stepID, contextID, commandSHA256,
		string(payload), string(resultJob)); err != nil {
		return fmt.Errorf("insert prepared roleplay test lifecycle: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO step_completion_evidence_sets (
			operation_id,job_id,generation,step_id,attempt,worker_id,evidence_count,records_json
		) VALUES ($1,$2,1,$3,1,$4,0,'[]'::jsonb)
	`, operationID, jobID, stepID, workerID); err != nil {
		return fmt.Errorf("insert prepared roleplay test evidence set: %w", err)
	}
	if requiresUserCanon {
		if _, err := AppendUserTurnCanonTx(
			ctx, tx, operationID, authority.PreparationID, authority.ChannelID,
			authority.UserMessageID, []string{}, []string{},
		); err != nil {
			return fmt.Errorf("append prepared roleplay test user canon: %w", err)
		}
	}
	for _, response := range responses {
		var messageID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO ai_channel_messages (channel_id,role,content)
			VALUES ($1,'assistant',$2)
			RETURNING id
		`, authority.ChannelID, response.Output).Scan(&messageID); err != nil {
			return fmt.Errorf("insert prepared roleplay test response: %w", err)
		}
		if _, err := AppendTurnCanonTx(
			ctx, tx, operationID, response.Position, authority.ChannelID,
			messageID, response.CharacterID, []string{}, []string{},
		); err != nil {
			return fmt.Errorf("append prepared roleplay test turn canon: %w", err)
		}
		if _, err := AppendOngoingActionResolutionTx(
			ctx, tx, operationID, response.Position, nil, nil,
		); err != nil {
			return fmt.Errorf("append prepared roleplay test ongoing action: %w", err)
		}
	}
	return nil
}
