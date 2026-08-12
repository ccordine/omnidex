package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

func insertCognitionProviderIdentityEvidenceBodyTx(
	ctx context.Context,
	tx pgx.Tx,
	evidence llm.ProviderIdentityEvidence,
) error {
	if err := evidence.Validate(); err != nil {
		return fmt.Errorf("%w: provider identity evidence is invalid: %v", ErrCognitionConflict, err)
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
		contentEncodingJSON, err := exactjson.Canonical(operation.ContentEncoding)
		if err != nil {
			return fmt.Errorf("encode provider identity content encoding %d: %w", index, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_provider_identity_evidence_operations (
				evidence_id,operation_index,operation,method,endpoint,request_disposition,
				request_sha256,request_bytes,request_body,http_status,disposition,
				response_complete,content_encoding_json,
				response_sha256,response_bytes,response_body
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (evidence_id,operation_index) DO NOTHING
		`, evidence.Ref.ID, index, operation.Operation, operation.Method, operation.Endpoint,
			operation.RequestDisposition, operation.RequestSHA256, operation.RequestBytes,
			operation.Request, operation.HTTPStatus, operation.Disposition,
			operation.ResponseComplete, string(contentEncodingJSON),
			operation.ResponseSHA256, operation.ResponseBytes, operation.ResponseCapture); err != nil {
			return fmt.Errorf("persist provider identity operation %d: %w", index, err)
		}
	}
	persisted, err := loadCognitionProviderIdentityEvidenceTx(ctx, tx, evidence.Ref.ID)
	if err != nil || !reflect.DeepEqual(persisted, evidence) {
		return fmt.Errorf("%w: content-addressed provider identity evidence changed: %v",
			ErrCognitionConflict, err)
	}
	return nil
}
