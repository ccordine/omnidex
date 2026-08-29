package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

func insertStationDiscoveryCapturesTx(
	ctx context.Context,
	tx pgx.Tx,
	openingID int64,
	evidence llm.ProviderIdentityEvidence,
) error {
	for index, operation := range evidence.Operations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO station_provider_discovery_captures (
				opening_id,operation_index,operation,method,endpoint,
				request_capture,request_sha256,request_bytes,
				response_capture,response_sha256,response_bytes,
				request_disposition,http_status,disposition,response_complete,
				content_encoding
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		`, openingID, index, string(operation.Operation), operation.Method, operation.Endpoint,
			operation.Request, operation.RequestSHA256, operation.RequestBytes,
			operation.ResponseCapture, operation.ResponseSHA256, operation.ResponseBytes,
			string(operation.RequestDisposition), operation.HTTPStatus, string(operation.Disposition),
			operation.ResponseComplete, mustCanonicalJSON(operation.ContentEncoding)); err != nil {
			return fmt.Errorf("persist station discovery capture %d: %w", index, err)
		}
	}
	return nil
}
