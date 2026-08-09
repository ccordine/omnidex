package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type scrumChannelQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *Repository) LoadScrumChannelOperation(
	ctx context.Context,
	request ScrumChannelOperationRequest,
) (ScrumChannelOperationResult, bool, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("PostgreSQL and context are required for Scrum channel replay")
	}
	descriptor, err := describeScrumChannelOperation(request)
	if err != nil {
		return ScrumChannelOperationResult{}, false, err
	}
	found, err := requireRegisteredScrumChannelIdentity(ctx, r.pool, descriptor)
	if err != nil || !found {
		return ScrumChannelOperationResult{}, false, err
	}
	result, found, err := loadScrumChannelOperation(ctx, r.pool, descriptor)
	if err != nil {
		return ScrumChannelOperationResult{}, false, err
	}
	if !found {
		return ScrumChannelOperationResult{}, false, fmt.Errorf(
			"registered Scrum channel operation %q has no immutable result",
			descriptor.Request.OperationID,
		)
	}
	return result, true, nil
}

func requireRegisteredScrumChannelIdentity(
	ctx context.Context,
	query scrumChannelQueryRower,
	descriptor scrumChannelOperationDescriptor,
) (bool, error) {
	var kind LifecycleOperationKind
	var commandSHA string
	var payloadMatches bool
	err := query.QueryRow(ctx, `
		SELECT kind, command_sha256, command_payload=$2::jsonb
		FROM lifecycle_operation_registry
		WHERE operation_id=$1
	`, descriptor.Request.OperationID, string(descriptor.Payload)).Scan(&kind, &commandSHA, &payloadMatches)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read Scrum channel lifecycle identity: %w", err)
	}
	if kind != LifecycleScrumChannel || commandSHA != descriptor.SHA256 || !payloadMatches {
		return false, fmt.Errorf(
			"%w: operation ID %q is reserved for different kind, scope, or command content",
			ErrLifecycleOperationConflict,
			descriptor.Request.OperationID,
		)
	}
	return true, nil
}

func loadScrumChannelOperation(
	ctx context.Context,
	query scrumChannelQueryRower,
	descriptor scrumChannelOperationDescriptor,
) (ScrumChannelOperationResult, bool, error) {
	var action, agent string
	var jobJSON, cardJSON []byte
	var jobID int64
	err := query.QueryRow(ctx, `
		SELECT job_id, result_action, result_agent, result_job, result_card
		FROM scrum_channel_operations
		WHERE operation_id=$1 AND kind=$2 AND command_sha256=$3
		  AND command_payload=$4::jsonb
	`, descriptor.Request.OperationID, LifecycleScrumChannel, descriptor.SHA256, string(descriptor.Payload)).Scan(
		&jobID, &action, &agent, &jobJSON, &cardJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScrumChannelOperationResult{}, false, nil
	}
	if err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("read Scrum channel operation result: %w", err)
	}
	var job model.Job
	var card DBScrumCard
	if err := json.Unmarshal(jobJSON, &job); err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("decode Scrum channel result job: %w", err)
	}
	if err := json.Unmarshal(cardJSON, &card); err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("decode Scrum channel result card: %w", err)
	}
	sanitizeScrumCardFields(&card)
	if job.ID != jobID || card.ID != descriptor.Request.CardID ||
		card.ProjectID != descriptor.Request.ProjectID || card.JobID != strconv.FormatInt(jobID, 10) {
		return ScrumChannelOperationResult{}, false, fmt.Errorf(
			"Scrum channel operation %q contains inconsistent result authority",
			descriptor.Request.OperationID,
		)
	}
	return ScrumChannelOperationResult{
		Card: card, Job: job, Action: action, Agent: agent,
	}, true, nil
}

func insertScrumChannelOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor scrumChannelOperationDescriptor,
	command ScrumChannelOperationCommand,
	result ScrumChannelOperationResult,
) error {
	jobJSON, err := json.Marshal(result.Job)
	if err != nil {
		return fmt.Errorf("encode Scrum channel result job: %w", err)
	}
	cardJSON, err := json.Marshal(result.Card)
	if err != nil {
		return fmt.Errorf("encode Scrum channel result card: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO scrum_channel_operations (
			operation_id, project_id, card_id, kind, command_sha256, command_payload,
			effect_kind, job_id, result_action, result_agent, result_job, result_card
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11::jsonb,$12::jsonb)
	`, descriptor.Request.OperationID, descriptor.Request.ProjectID, descriptor.Request.CardID,
		LifecycleScrumChannel, descriptor.SHA256, string(descriptor.Payload), command.Effect.Kind,
		result.Job.ID, result.Action, result.Agent, string(jobJSON), string(cardJSON))
	if err != nil {
		return fmt.Errorf("record Scrum channel operation %q: %w", descriptor.Request.OperationID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("Scrum channel operation %q recorded %d rows; expected 1", descriptor.Request.OperationID, tag.RowsAffected())
	}
	return nil
}
