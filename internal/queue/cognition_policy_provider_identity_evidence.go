package queue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func insertCognitionProviderIdentityEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	evidence llm.ProviderIdentityEvidence,
) error {
	if result.ProviderIdentityEvidence == (llm.ProviderIdentityEvidenceRef{}) {
		if !reflect.DeepEqual(evidence, llm.ProviderIdentityEvidence{}) {
			return fmt.Errorf("%w: result without provider identity has raw evidence", ErrCognitionConflict)
		}
		return nil
	}
	if evidence.ValidateRequests(llm.ProviderIdentitySelection{
		Model: attempt.Brain.Model, NativeContextLimit: attempt.Brain.NativeContextLimit,
	}) != nil || evidence.Ref != result.ProviderIdentityEvidence {
		return fmt.Errorf("%w: provider identity evidence differs from terminal call", ErrCognitionConflict)
	}
	refJSON, err := exactjson.Canonical(evidence.Ref)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_provider_identity_evidence (
			evidence_id,manifest_sha256,total_bytes,ref_json,ref_sha256
		) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (evidence_id) DO NOTHING
	`, evidence.Ref.ID, evidence.Ref.SHA256, evidence.Ref.Bytes,
		string(refJSON), cognitionPayloadSHA(refJSON)); err != nil {
		return fmt.Errorf("persist provider identity evidence %q: %w", evidence.Ref.ID, err)
	}
	for index, operation := range evidence.Operations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_provider_identity_evidence_operations (
				evidence_id,operation_index,operation,method,endpoint,request_dispatched,
				request_sha256,request_bytes,request_body,http_status,disposition,
				response_complete,content_encoding_count,content_encoding,response_uncompressed,
				response_sha256,response_bytes,response_body
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
			ON CONFLICT (evidence_id,operation_index) DO NOTHING
		`, evidence.Ref.ID, index, operation.Operation, operation.Method, operation.Endpoint,
			operation.RequestDispatched, operation.RequestSHA256, operation.RequestBytes,
			operation.Request, operation.HTTPStatus, operation.Disposition,
			operation.ResponseComplete, operation.ContentEncodingCount,
			operation.ContentEncoding, operation.ResponseUncompressed,
			operation.ResponseSHA256, operation.ResponseBytes, operation.ResponseCapture); err != nil {
			return fmt.Errorf("persist provider identity operation %d: %w", index, err)
		}
	}
	persisted, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, evidence.Ref.ID)
	if err != nil || !reflect.DeepEqual(persisted, evidence) {
		return fmt.Errorf("%w: content-addressed provider identity evidence changed: %v",
			ErrCognitionConflict, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_policy_call_provider_identity_evidence (
			call_id,evidence_id,episode_id,job_id,generation,step_id,step_attempt,worker_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, attempt.ID, evidence.Ref.ID, attempt.ExpectedRevision.EpisodeID,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt,
		authority.WorkerID); err != nil {
		return fmt.Errorf("associate provider identity evidence with call %q: %w", attempt.ID, err)
	}
	return nil
}

func loadCognitionProviderIdentityEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	evidenceID string,
) (llm.ProviderIdentityEvidence, error) {
	var refJSON []byte
	var refSHA string
	if err := tx.QueryRow(ctx, `
		SELECT ref_json,ref_sha256 FROM cognition_provider_identity_evidence
		WHERE evidence_id=$1 FOR SHARE
	`, evidenceID).Scan(&refJSON, &refSHA); err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	var ref llm.ProviderIdentityEvidenceRef
	if err := cognitionDecodeExact(refJSON, &ref); err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	canonical, err := exactjson.Canonical(ref)
	if err != nil || !bytes.Equal(canonical, refJSON) || cognitionPayloadSHA(canonical) != refSHA {
		return llm.ProviderIdentityEvidence{}, fmt.Errorf("persisted provider identity ref changed")
	}
	rows, err := tx.Query(ctx, `
		SELECT operation,method,endpoint,request_dispatched,request_sha256,request_bytes,
		       request_body,http_status,disposition,response_complete,response_sha256,
		       content_encoding_count,content_encoding,response_uncompressed,
		       response_bytes,response_body
		FROM cognition_provider_identity_evidence_operations
		WHERE evidence_id=$1 ORDER BY operation_index
	`, evidenceID)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	defer rows.Close()
	operations := make([]llm.ProviderIdentityOperationEvidence, 0, 5)
	for rows.Next() {
		var operation llm.ProviderIdentityOperationEvidence
		if err := rows.Scan(&operation.Operation, &operation.Method, &operation.Endpoint,
			&operation.RequestDispatched, &operation.RequestSHA256, &operation.RequestBytes,
			&operation.Request, &operation.HTTPStatus, &operation.Disposition,
			&operation.ResponseComplete, &operation.ResponseSHA256,
			&operation.ContentEncodingCount, &operation.ContentEncoding,
			&operation.ResponseUncompressed, &operation.ResponseBytes,
			&operation.ResponseCapture); err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	value := llm.ProviderIdentityEvidence{
		Schema: llm.ProviderIdentityEvidenceSchemaV1, Ref: ref, Operations: operations,
	}
	if err := value.Validate(); err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	return value, nil
}

func validateLoadedCognitionProviderIdentityRefTx(
	ctx context.Context,
	tx pgx.Tx,
	callID string,
	ref llm.ProviderIdentityEvidenceRef,
) error {
	if ref == (llm.ProviderIdentityEvidenceRef{}) {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM cognition_policy_call_provider_identity_evidence WHERE call_id=$1
		)`, callID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%w: call without identity ref has an association", ErrCognitionConflict)
		}
		return nil
	}
	var refJSON []byte
	var refSHA string
	err := tx.QueryRow(ctx, `
		SELECT evidence.ref_json,evidence.ref_sha256
		FROM cognition_policy_call_provider_identity_evidence association
		JOIN cognition_provider_identity_evidence evidence
		  ON evidence.evidence_id=association.evidence_id
		WHERE association.call_id=$1
	`, callID).Scan(&refJSON, &refSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: terminal call lacks provider identity evidence", ErrCognitionConflict)
	}
	if err != nil {
		return err
	}
	var persisted llm.ProviderIdentityEvidenceRef
	if err := cognitionDecodeExact(refJSON, &persisted); err != nil {
		return err
	}
	canonical, err := exactjson.Canonical(persisted)
	if err != nil || !bytes.Equal(canonical, refJSON) || cognitionPayloadSHA(canonical) != refSHA ||
		persisted != ref {
		return fmt.Errorf("%w: provider identity association changed", ErrCognitionConflict)
	}
	return nil
}
