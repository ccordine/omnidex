package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertCognitionProviderResponseCaptureTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.ProviderResponseCaptureEvidence,
) error {
	if result.ProviderResponseCapture == (cognitionpolicy.ProviderResponseCaptureEvidenceRef{}) {
		return nil
	}
	refJSON, err := exactjson.Canonical(evidence.Ref)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_policy_provider_response_captures (
			evidence_id,call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			capture_sha256,capture_bytes,ref_json,ref_sha256,content
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, evidence.Ref.ID, attempt.ID, attempt.ExpectedRevision.EpisodeID,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID,
		evidence.Ref.SHA256, evidence.Ref.Bytes, string(refJSON), cognitionPayloadSHA(refJSON),
		evidence.Content)
	if err != nil {
		return fmt.Errorf("persist exact provider response capture %q: %w", evidence.Ref.ID, err)
	}
	return nil
}
