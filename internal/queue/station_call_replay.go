package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// StationCallReplayPoint is one immutable, previously opened provider call
// together with the rendered portable job that authorized it. It is read-only
// benchmark input; replaying it never reclaims the historical step attempt.
type StationCallReplayPoint struct {
	Call StationCallOpening
	Gap  StationGapOpening
}

// ReadStationCallReplayPoint returns one exact historical station call and
// its immutable portable-job boundary from a repeatable read-only snapshot.
func (r *Repository) ReadStationCallReplayPoint(
	ctx context.Context,
	openingID int64,
) (StationCallReplayPoint, error) {
	if r == nil || r.pool == nil {
		return StationCallReplayPoint{}, fmt.Errorf("station replay read requires PostgreSQL")
	}
	if openingID < 1 {
		return StationCallReplayPoint{}, fmt.Errorf("station replay call opening ID must be positive")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return StationCallReplayPoint{}, fmt.Errorf("begin station replay read: %w", err)
	}
	defer tx.Rollback(ctx)
	point, err := loadStationCallReplayPointTx(ctx, tx, openingID)
	if err != nil {
		return StationCallReplayPoint{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationCallReplayPoint{}, fmt.Errorf("commit station replay read: %w", err)
	}
	return point, nil
}

// FindLatestStationCallReplayPoint selects the latest immutable opening for
// one historical job/work kind. The caller must still record the returned
// opening ID in benchmark evidence, so later replays remain exact.
func (r *Repository) FindLatestStationCallReplayPoint(
	ctx context.Context,
	jobID int64,
	workKind string,
) (StationCallReplayPoint, error) {
	if r == nil || r.pool == nil {
		return StationCallReplayPoint{}, fmt.Errorf("station replay read requires PostgreSQL")
	}
	workKind = strings.TrimSpace(workKind)
	if jobID < 1 || workKind == "" || len(workKind) > 128 {
		return StationCallReplayPoint{}, fmt.Errorf("station replay job and work kind are invalid")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return StationCallReplayPoint{}, fmt.Errorf("begin station replay search: %w", err)
	}
	defer tx.Rollback(ctx)
	var openingID int64
	if err := tx.QueryRow(ctx, `
		SELECT calls.id
		FROM station_call_openings AS calls
		JOIN station_gap_openings AS gaps ON gaps.id=calls.gap_opening_id
		WHERE calls.job_id=$1 AND gaps.work_kind=$2
		ORDER BY calls.id DESC
		LIMIT 1
	`, jobID, workKind).Scan(&openingID); err != nil {
		return StationCallReplayPoint{}, fmt.Errorf(
			"find latest station replay opening for job %d work kind %q: %w",
			jobID, workKind, err,
		)
	}
	point, err := loadStationCallReplayPointTx(ctx, tx, openingID)
	if err != nil {
		return StationCallReplayPoint{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StationCallReplayPoint{}, fmt.Errorf("commit station replay search: %w", err)
	}
	return point, nil
}

func loadStationCallReplayPointTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
) (StationCallReplayPoint, error) {
	call, err := loadStationCallReplayOpeningTx(ctx, tx, openingID)
	if err != nil {
		return StationCallReplayPoint{}, fmt.Errorf("load station replay call opening: %w", err)
	}
	gap, err := loadStationCallReplayGapTx(ctx, tx, call.GapOpeningID)
	if err != nil {
		return StationCallReplayPoint{}, fmt.Errorf("load station replay gap opening: %w", err)
	}
	if call.GapOpeningID != gap.ID || call.JobID != gap.JobID ||
		call.Generation != gap.Generation || call.StepID != gap.StepID ||
		call.StepAttempt != gap.StepAttempt || call.WorkerID != gap.WorkerID ||
		call.GapID != gap.GapID {
		return StationCallReplayPoint{}, fmt.Errorf("station replay opening differs from its durable gap authority")
	}
	return StationCallReplayPoint{Call: call, Gap: gap}, nil
}

func loadStationCallReplayOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
) (StationCallOpening, error) {
	var opening StationCallOpening
	err := scanStationCallOpening(tx.QueryRow(ctx, `
		SELECT id,gap_opening_id,discovery_receipt_id,job_id,generation,step_id,step_attempt,worker_id,gap_id,
			protocol,tokenizer_profile,provider_method,provider_endpoint,
			wire_request,wire_request_sha256,wire_request_bytes,expectation,
			expectation_sha256,observation_challenge,model,context_tokens,max_input_tokens,
			max_output_tokens,output_limit_mode,model_input,model_input_sha256,model_input_bytes,
			model_input_token_upper_bound,created_at
		FROM station_call_openings WHERE id=$1
	`, openingID), &opening)
	return opening, err
}

func loadStationCallReplayGapTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
) (StationGapOpening, error) {
	var opening StationGapOpening
	err := scanStationGapOpening(tx.QueryRow(ctx, `
		SELECT id,job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,
			projection_envelope,projection_sha256,semantic_uncertainty_contract,
			semantic_uncertainty_contract_sha256,context_tokens,max_output_tokens,
			output_limit_mode,created_at
		FROM station_gap_openings WHERE id=$1
	`, openingID), &opening)
	return opening, err
}
