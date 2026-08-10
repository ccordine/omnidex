package queue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func validateCognitionResponseEvidence(
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.ModelResponseEvidence,
) error {
	if result.ResponseBytes == 0 {
		if !reflect.DeepEqual(evidence, cognitionpolicy.ModelResponseEvidence{}) {
			return fmt.Errorf("%w: empty policy response has evidence bytes", ErrCognitionConflict)
		}
		return nil
	}
	if err := evidence.Validate(); err != nil {
		return err
	}
	if evidence.CallID != attempt.ID || evidence.Ref != result.ResponseEvidence ||
		len(evidence.Content) != result.ResponseBytes {
		return fmt.Errorf("%w: policy response evidence differs from its terminal result", ErrCognitionConflict)
	}
	return nil
}

func validateLoadedCognitionResponseEvidence(
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence *cognitionpolicy.ModelResponseEvidence,
) error {
	if result.ResponseBytes == 0 {
		if evidence != nil {
			return fmt.Errorf("%w: empty persisted policy response has evidence", ErrCognitionConflict)
		}
		return nil
	}
	if evidence == nil {
		return fmt.Errorf("%w: persisted policy result lacks response evidence", ErrCognitionConflict)
	}
	return validateCognitionResponseEvidence(attempt, result, *evidence)
}

func insertCognitionResponseEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence cognitionpolicy.ModelResponseEvidence,
) error {
	if result.ResponseBytes == 0 {
		return nil
	}
	refJSON, err := exactjson.Canonical(evidence.Ref)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_policy_response_evidence (
			evidence_id,call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			response_sha256,response_bytes,ref_json,ref_sha256,content
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, evidence.Ref.ID, attempt.ID, attempt.ExpectedRevision.EpisodeID,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID,
		evidence.Ref.SHA256, evidence.Ref.Bytes, string(refJSON), cognitionPayloadSHA(refJSON),
		evidence.Content)
	if err != nil {
		return fmt.Errorf("persist exact cognition model response %q: %w", evidence.Ref.ID, err)
	}
	return nil
}

func loadCognitionResponseEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	callID string,
	lock bool,
) (*cognitionpolicy.ModelResponseEvidence, error) {
	query := `SELECT ref_json,ref_sha256,content FROM cognition_policy_response_evidence WHERE call_id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var refJSON []byte
	var refSHA string
	var content []byte
	err := tx.QueryRow(ctx, query, callID).Scan(&refJSON, &refSHA, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ref cognitionpolicy.ModelResponseEvidenceRef
	if err := cognitionDecodeExact(refJSON, &ref); err != nil {
		return nil, fmt.Errorf("decode exact cognition response evidence ref: %w", err)
	}
	canonical, err := exactjson.Canonical(ref)
	if err != nil || !bytes.Equal(canonical, refJSON) || cognitionPayloadSHA(canonical) != refSHA {
		return nil, fmt.Errorf("%w: persisted response evidence reference changed", ErrCognitionConflict)
	}
	evidence := cognitionpolicy.ModelResponseEvidence{Ref: ref, CallID: callID, Content: content}
	if err := evidence.Validate(); err != nil {
		return nil, fmt.Errorf("%w: persisted response evidence: %v", ErrCognitionConflict, err)
	}
	return &evidence, nil
}
