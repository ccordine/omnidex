package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertCognitionProviderGenerationEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.ProviderGenerationEvidence,
) error {
	if result.ProviderGenerationEvidence == (cognitionpolicy.ProviderGenerationEvidenceRef{}) {
		return nil
	}
	refJSON, err := exactjson.Canonical(evidence.Ref)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_policy_provider_generation_evidence (
			evidence_id,call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			generation_sha256,generation_bytes,ref_json,ref_sha256,generation_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, evidence.Ref.ID, attempt.ID, attempt.ExpectedRevision.EpisodeID,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID,
		evidence.Ref.SHA256, evidence.Ref.Bytes, string(refJSON), cognitionPayloadSHA(refJSON),
		string(evidence.Generation))
	if err != nil {
		return fmt.Errorf("persist untrusted cognition provider evidence %q: %w", evidence.Ref.ID, err)
	}
	return nil
}
