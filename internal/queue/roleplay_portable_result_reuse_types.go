package queue

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

const RoleplayPortableResultReuseReceiptSchemaV1 = "omnidex.roleplay-portable-result-reuse.v1"

// RoleplayPortableResultReuseRequest names one exact accepted semantic leaf
// that the current roleplay attempt would otherwise have to request again.
type RoleplayPortableResultReuseRequest struct {
	Authority model.StepAttemptAuthority
	Job       assemblyline.PortableJob
	Station   station.ID
}

// RoleplayPortableResultReuseReceipt is immutable provenance from the current
// attempt to the exact prior response evidence it reused.
type RoleplayPortableResultReuseReceipt struct {
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
	RoleplayAuthoritySHA256      string                     `json:"roleplay_authority_sha256"`
	CreatedAt                    time.Time                  `json:"created_at"`
}

// RoleplayPortableResultReuse returns the prior exact response as a result for
// the current content-addressed root job. The receipt carries the cross-attempt
// provenance; it does not grant station or workflow authority itself.
type RoleplayPortableResultReuse struct {
	Result  assemblyline.PortableResult        `json:"result"`
	Receipt RoleplayPortableResultReuseReceipt `json:"receipt"`
}

type roleplayPortableResultReuseRow struct {
	Receipt                RoleplayPortableResultReuseReceipt
	TargetPortablePayload  string
	TargetPortableEnvelope string
	RoleplayAuthority      string
}

type roleplayPortableResultReuseScanner interface {
	Scan(dest ...any) error
}

func scanRoleplayPortableResultReuse(
	scanner roleplayPortableResultReuseScanner,
	row *roleplayPortableResultReuseRow,
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
		&receipt.SourceResponseSHA256, &row.RoleplayAuthority,
		&receipt.RoleplayAuthoritySHA256, &receipt.CreatedAt,
	); err != nil {
		return err
	}
	if receipt.ID < 1 || receipt.CreatedAt.IsZero() {
		return fmt.Errorf("roleplay portable reuse receipt has no exact identity or time")
	}
	return nil
}
