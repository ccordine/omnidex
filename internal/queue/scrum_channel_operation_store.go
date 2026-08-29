package queue

import (
	"context"
	"errors"
	"fmt"

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
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("begin Scrum channel replay: %w", err)
	}
	defer rollbackTx(ctx, tx, "Scrum channel replay")
	found, err := requireRegisteredScrumChannelIdentity(ctx, tx, descriptor)
	if err != nil || !found {
		return ScrumChannelOperationResult{}, false, err
	}
	result, found, err := loadScrumChannelOperation(ctx, tx, descriptor)
	if err != nil {
		return ScrumChannelOperationResult{}, false, err
	}
	if !found {
		return ScrumChannelOperationResult{}, false, fmt.Errorf(
			"registered Scrum channel operation %q has no immutable result",
			descriptor.Request.OperationID,
		)
	}
	if err := tx.Commit(ctx); err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("commit Scrum channel replay read: %w", err)
	}
	return result, true, nil
}

func requireRegisteredScrumChannelIdentity(
	ctx context.Context,
	query scrumChannelOperationQueryer,
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
	tx pgx.Tx,
	descriptor scrumChannelOperationDescriptor,
) (ScrumChannelOperationResult, bool, error) {
	var action string
	var jobID int64
	err := tx.QueryRow(ctx, `
		SELECT job_id,result_action
		FROM scrum_channel_operations
		WHERE operation_id=$1
	`, descriptor.Request.OperationID).Scan(
		&jobID, &action,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScrumChannelOperationResult{}, false, nil
	}
	if err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("read Scrum channel operation result: %w", err)
	}
	card, err := lockScrumCardTx(ctx, tx, descriptor.Request.ProjectID, descriptor.Request.CardID)
	if err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("load live Scrum channel replay target: %w", err)
	}
	job, err := scanJob(tx.QueryRow(ctx, `
		SELECT id,instruction,pipeline,status,result,error,metadata,current_generation,
		       created_at,updated_at,completed_at
		FROM jobs WHERE id=$1 AND project_id=$2
	`, jobID, descriptor.Request.ProjectID))
	if err != nil {
		return ScrumChannelOperationResult{}, false, fmt.Errorf("load current Scrum operation job: %w", err)
	}
	messages, messageStart, err := loadScrumCardMessageTail(
		ctx, tx, card.ProjectID, card.ID, card.ChannelMessageCount, 50, MaxScrumChannelPageBytes,
	)
	if err != nil {
		return ScrumChannelOperationResult{}, false, err
	}
	return ScrumChannelOperationResult{
		OperationID: descriptor.Request.OperationID,
		Card:        card, Messages: messages, MessageStart: messageStart, MessageTotal: card.ChannelMessageCount,
		Job: job, Action: action,
	}, true, nil
}

type scrumChannelOperationQueryer interface {
	scrumChannelQueryRower
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func insertScrumChannelOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor scrumChannelOperationDescriptor,
	command ScrumChannelOperationCommand,
	result ScrumChannelOperationResult,
) error {
	effectOperationID := descriptor.Request.OperationID
	if command.Effect.Kind != ScrumChannelStartJob {
		var err error
		effectOperationID, err = scrumChannelEffectOperationID(command)
		if err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO scrum_channel_operations (
			operation_id,project_id,card_id,effect_kind,effect_operation_id,job_id,result_action
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, descriptor.Request.OperationID, descriptor.Request.ProjectID, descriptor.Request.CardID,
		command.Effect.Kind, effectOperationID, result.Job.ID, result.Action)
	if err != nil {
		return fmt.Errorf("record Scrum channel operation %q: %w", descriptor.Request.OperationID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("Scrum channel operation %q recorded %d rows; expected 1", descriptor.Request.OperationID, tag.RowsAffected())
	}
	return nil
}
