package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func insertLLMCallOutcomeTx(
	ctx context.Context,
	tx pgx.Tx,
	callID int64,
	status LLMCallOutcomeStatus,
	validationError string,
) (LLMCallOutcome, error) {
	var outcome LLMCallOutcome
	var message *string
	if err := tx.QueryRow(ctx, `
		INSERT INTO llm_call_outcomes (call_evidence_id,status,validation_error)
		VALUES ($1,$2,$3)
		RETURNING call_evidence_id,status,validation_error,created_at
	`, callID, string(status), optionalExactText(validationError)).Scan(
		&outcome.CallEvidenceID, &outcome.Status, &message, &outcome.CreatedAt,
	); err != nil {
		return LLMCallOutcome{}, fmt.Errorf("record LLM call outcome: %w", err)
	}
	if message != nil {
		outcome.ValidationError = *message
	}
	return outcome, nil
}

func readLLMCallOutcomeTx(ctx context.Context, tx pgx.Tx, callID int64) (LLMCallOutcome, bool, error) {
	var outcome LLMCallOutcome
	var message *string
	err := tx.QueryRow(ctx, `
		SELECT call_evidence_id,status,validation_error,created_at
		FROM llm_call_outcomes WHERE call_evidence_id=$1
	`, callID).Scan(&outcome.CallEvidenceID, &outcome.Status, &message, &outcome.CreatedAt)
	if err == pgx.ErrNoRows {
		return LLMCallOutcome{}, false, nil
	}
	if err != nil {
		return LLMCallOutcome{}, false, fmt.Errorf("read LLM call outcome: %w", err)
	}
	if message != nil {
		outcome.ValidationError = *message
	}
	return outcome, true, nil
}
