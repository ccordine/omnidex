package queue

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

const ObjectivePortableResultReuseReceiptSchemaV1 = "omnidex.objective-portable-result-reuse.v1"

// ObjectivePortableResultReuseRequest names one exact accepted semantic leaf
// that the current step attempt would otherwise have to request again.
type ObjectivePortableResultReuseRequest struct {
	Authority model.StepAttemptAuthority
	Job       assemblyline.PortableJob
	Station   station.ID
}

// ObjectivePortableResultReuseReceipt is immutable provenance from the current
// attempt to the exact prior response evidence it reused.
type ObjectivePortableResultReuseReceipt struct {
	ID                           int64                      `json:"id"`
	Schema                       string                     `json:"schema"`
	TargetAuthority              model.StepAttemptAuthority `json:"target_authority"`
	Station                      station.ID                 `json:"station"`
	RootWorkID                   string                     `json:"root_work_id"`
	TargetWorkKind               assemblyline.WorkKind      `json:"target_work_kind"`
	TargetPortablePayloadSHA256  string                     `json:"target_portable_payload_sha256"`
	TargetPortableEnvelopeSHA256 string                     `json:"target_portable_envelope_sha256"`
	SourceAuthority              model.StepAttemptAuthority `json:"source_authority"`
	SourceGapOpeningID           int64                      `json:"source_gap_opening_id"`
	SourceGapOutcomeID           int64                      `json:"source_gap_outcome_id"`
	SourceWorkID                 string                     `json:"source_work_id"`
	SourcePortableEnvelopeSHA256 string                     `json:"source_portable_envelope_sha256"`
	SourceCallReceiptSHA256      string                     `json:"source_call_receipt_sha256"`
	SourceResponseSHA256         string                     `json:"source_response_sha256"`
	ObjectiveAuthoritySHA256     string                     `json:"objective_authority_sha256"`
	CreatedAt                    time.Time                  `json:"created_at"`
}

// ObjectivePortableResultReuse returns the prior exact response as a result for
// the current content-addressed root job. The receipt carries the cross-attempt
// provenance; it does not grant station or workflow authority itself.
type ObjectivePortableResultReuse struct {
	Result  assemblyline.PortableResult         `json:"result"`
	Receipt ObjectivePortableResultReuseReceipt `json:"receipt"`
}

type objectivePortableResultReuseRow struct {
	Receipt                ObjectivePortableResultReuseReceipt
	TargetPortablePayload  string
	TargetPortableEnvelope string
	ObjectiveAuthority     string
}

type objectivePortableResultReuseScanner interface {
	Scan(dest ...any) error
}

func scanObjectivePortableResultReuse(
	scanner objectivePortableResultReuseScanner,
	row *objectivePortableResultReuseRow,
) error {
	receipt := &row.Receipt
	if err := scanner.Scan(
		&receipt.ID, &receipt.Schema,
		&receipt.TargetAuthority.JobID, &receipt.TargetAuthority.Generation,
		&receipt.TargetAuthority.StepID, &receipt.TargetAuthority.Attempt,
		&receipt.TargetAuthority.WorkerID, &receipt.Station, &receipt.RootWorkID,
		&receipt.TargetWorkKind, &row.TargetPortablePayload,
		&receipt.TargetPortablePayloadSHA256, &row.TargetPortableEnvelope,
		&receipt.TargetPortableEnvelopeSHA256,
		&receipt.SourceAuthority.JobID, &receipt.SourceAuthority.Generation,
		&receipt.SourceAuthority.StepID, &receipt.SourceAuthority.Attempt,
		&receipt.SourceAuthority.WorkerID, &receipt.SourceGapOpeningID,
		&receipt.SourceGapOutcomeID, &receipt.SourceWorkID,
		&receipt.SourcePortableEnvelopeSHA256, &receipt.SourceCallReceiptSHA256,
		&receipt.SourceResponseSHA256, &row.ObjectiveAuthority,
		&receipt.ObjectiveAuthoritySHA256, &receipt.CreatedAt,
	); err != nil {
		return err
	}
	if receipt.ID < 1 || receipt.CreatedAt.IsZero() {
		return fmt.Errorf("objective portable reuse receipt has no exact identity or time")
	}
	return nil
}
