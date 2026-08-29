package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const maxStationAttemptEvidenceCalls = 64

// StationAttemptCallEvidence is the immutable model-call leaf retained for one
// exact step attempt. It is intentionally internal execution evidence, not an
// API projection or a source of model authority.
type StationAttemptCallEvidence struct {
	OpeningID int64
	WorkKind  assemblyline.WorkKind
	Payload   string
	Prompt    string
	Response  string
}

// StationAttemptCallEvidence loads every completed provider call for one exact
// running attempt. Partial gaps, missing receipts, provider failures, and
// missing llm_call_evidence fail loudly so callers cannot manufacture
// completion counters from local events. A stored response has no transition
// authority. Only a separately established code-proven defect may authorize the
// specifically bounded target-tree replacement or staged compiler repair.
func (r *Repository) StationAttemptCallEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) ([]StationAttemptCallEvidence, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("station attempt evidence requires PostgreSQL and context")
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin exact station attempt evidence read: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := r.AuthorizeStepAttemptTransaction(ctx, tx, authority); err != nil {
		return nil, err
	}

	var openings, outcomes, calls, receipts, evidence int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(DISTINCT gap.id),COUNT(DISTINCT outcome.id),
		       COUNT(DISTINCT call.id),COUNT(DISTINCT receipt.id),
		       COUNT(DISTINCT exact.id)
		FROM station_gap_openings AS gap
		LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=gap.id
		LEFT JOIN station_call_openings AS call ON call.gap_opening_id=gap.id
		LEFT JOIN station_call_receipts AS receipt ON receipt.opening_id=call.id
		LEFT JOIN llm_call_evidence AS exact ON exact.station_call_opening_id=call.id
		WHERE gap.job_id=$1 AND gap.generation=$2 AND gap.step_id=$3
		  AND gap.step_attempt=$4 AND gap.worker_id=$5
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID).Scan(
		&openings, &outcomes, &calls, &receipts, &evidence,
	); err != nil {
		return nil, fmt.Errorf("count exact station attempt evidence: %w", err)
	}
	if openings > maxStationAttemptEvidenceCalls {
		return nil, fmt.Errorf(
			"station attempt evidence has %d calls above the %d-call bound",
			openings, maxStationAttemptEvidenceCalls,
		)
	}
	if outcomes != openings || calls != openings || receipts != openings || evidence != openings {
		return nil, fmt.Errorf(
			"station attempt evidence is incomplete: gaps=%d outcomes=%d calls=%d receipts=%d evidence=%d",
			openings, outcomes, calls, receipts, evidence,
		)
	}

	rows, err := tx.Query(ctx, `
		SELECT call.id,gap.work_kind,gap.portable_payload,gap.prompt,
		       outcome.response,outcome.error,exact.system_prompt,exact.response,
		       outcome.status,receipt.status,exact.status
		FROM station_gap_openings AS gap
		JOIN station_gap_outcomes AS outcome ON outcome.opening_id=gap.id
		JOIN station_call_openings AS call ON call.gap_opening_id=gap.id
		JOIN station_call_receipts AS receipt ON receipt.opening_id=call.id
		JOIN llm_call_evidence AS exact ON exact.station_call_opening_id=call.id
		WHERE gap.job_id=$1 AND gap.generation=$2 AND gap.step_id=$3
		  AND gap.step_attempt=$4 AND gap.worker_id=$5
		ORDER BY gap.id
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("load exact station attempt evidence: %w", err)
	}
	defer rows.Close()

	result := make([]StationAttemptCallEvidence, 0, openings)
	for rows.Next() {
		var item StationAttemptCallEvidence
		var outcomeResponse, outcomeError, exactResponse *string
		var exactPrompt, outcomeStatus, receiptStatus, evidenceStatus string
		if err := rows.Scan(
			&item.OpeningID, &item.WorkKind, &item.Payload, &item.Prompt,
			&outcomeResponse, &outcomeError, &exactPrompt, &exactResponse,
			&outcomeStatus, &receiptStatus, &evidenceStatus,
		); err != nil {
			return nil, fmt.Errorf("scan exact station attempt evidence: %w", err)
		}
		if receiptStatus != "succeeded" || evidenceStatus != string(LLMEvidenceSucceeded) ||
			exactResponse == nil || exactPrompt != item.Prompt {
			return nil, fmt.Errorf("station call %d lacks one successful exact terminal evidence chain", item.OpeningID)
		}
		switch StationGapStatus(outcomeStatus) {
		case StationGapResolved:
			if outcomeResponse == nil || *outcomeResponse != *exactResponse || outcomeError != nil {
				return nil, fmt.Errorf("station call %d resolved outcome differs from its exact response", item.OpeningID)
			}
		case StationGapFailed:
			if outcomeResponse != nil || outcomeError == nil || *outcomeError == "" {
				return nil, fmt.Errorf("station call %d rejected outcome lacks exact failure authority", item.OpeningID)
			}
		default:
			return nil, fmt.Errorf("station call %d has unregistered gap status %q", item.OpeningID, outcomeStatus)
		}
		item.Response = *exactResponse
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exact station attempt evidence: %w", err)
	}
	if len(result) != openings {
		return nil, fmt.Errorf("station attempt evidence changed during exact read")
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit exact station attempt evidence read: %w", err)
	}
	return result, nil
}
