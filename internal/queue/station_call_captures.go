package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

func insertStationCallCapturesTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
	result llm.PreparedGeneration,
) error {
	if result.ProviderResponseDisposition != "" &&
		result.ProviderResponseDisposition != llm.ProviderResponseTransportError {
		if _, err := tx.Exec(ctx, `
			INSERT INTO station_call_response_captures (
				opening_id,capture,capture_sha256,captured_bytes
			) VALUES ($1,$2,$3,$4)
		`, openingID, result.ProviderResponseCapture,
			result.ProviderResponseCaptureSHA256, result.ProviderResponseCapturedBytes); err != nil {
			return fmt.Errorf("persist station call response capture: %w", err)
		}
	}
	for index, operation := range result.ProviderIdentityEvidence.Operations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO station_call_identity_captures (
				opening_id,operation_index,operation,method,endpoint,
				request_capture,request_sha256,request_bytes,
				response_capture,response_sha256,response_bytes
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		`, openingID, index, string(operation.Operation), operation.Method, operation.Endpoint,
			operation.Request, operation.RequestSHA256, operation.RequestBytes,
			operation.ResponseCapture, operation.ResponseSHA256, operation.ResponseBytes); err != nil {
			return fmt.Errorf("persist station call identity capture %d: %w", index, err)
		}
	}
	return nil
}
