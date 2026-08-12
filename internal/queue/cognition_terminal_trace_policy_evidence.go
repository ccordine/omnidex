package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

const (
	cognitionTracePolicyEvidenceSchemaV1  = "omnidex.cognition-policy-evidence-trace.v1"
	cognitionTraceEvidenceModelResponse   = "model_response"
	cognitionTraceEvidenceProviderReturn  = "provider_generation"
	cognitionTraceEvidenceProviderCapture = "provider_response_capture"
)

type cognitionTracePolicyEvidence struct {
	Schema              string `json:"schema"`
	EvidenceKind        string `json:"evidence_kind"`
	CallID              string `json:"call_id"`
	EvidenceID          string `json:"evidence_id"`
	ReferenceJSONSHA256 string `json:"reference_json_sha256"`
	ContentSHA256       string `json:"content_sha256"`
	Bytes               int    `json:"bytes"`
}

func (value cognitionTracePolicyEvidence) validate() error {
	if value.Schema != cognitionTracePolicyEvidenceSchemaV1 ||
		(value.EvidenceKind != cognitionTraceEvidenceModelResponse &&
			value.EvidenceKind != cognitionTraceEvidenceProviderReturn &&
			value.EvidenceKind != cognitionTraceEvidenceProviderCapture) ||
		!taskLedgerExact(value.CallID) || !taskLedgerExact(value.EvidenceID) ||
		!cognitionDigestPattern.MatchString(value.ReferenceJSONSHA256) ||
		!cognitionDigestPattern.MatchString(value.ContentSHA256) || value.Bytes < 0 ||
		(value.Bytes == 0 && value.EvidenceKind != cognitionTraceEvidenceProviderCapture) {
		return fmt.Errorf("%w: cognition policy evidence trace is invalid", ErrCognitionConflict)
	}
	return nil
}

func cognitionPolicyEvidenceTracePayload(value cognitionTracePolicyEvidence) ([]byte, error) {
	if err := value.validate(); err != nil {
		return nil, err
	}
	raw, err := exactjson.Canonical(value)
	if err != nil {
		return nil, fmt.Errorf("encode cognition policy evidence trace: %w", err)
	}
	return raw, nil
}

func appendCognitionPolicyEvidenceTraceRecordsTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	records []cognitionTraceRecord,
) ([]cognitionTraceRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT evidence.evidence_kind,evidence.evidence_id,evidence.call_id,
		       evidence.ref_sha256,evidence.content_sha256,
		       content_bytes,snapshots.call_ordinal
		FROM (
			SELECT 'model_response'::text AS evidence_kind,evidence.evidence_id,
			       evidence.call_id,evidence.ref_sha256,
			       evidence.response_sha256 AS content_sha256,
			       evidence.response_bytes AS content_bytes,evidence.episode_id
			FROM cognition_policy_response_evidence evidence WHERE evidence.episode_id=$1
			UNION ALL
			SELECT 'provider_generation',evidence.evidence_id,evidence.call_id,
			       evidence.ref_sha256,evidence.generation_sha256,
			       evidence.generation_bytes,evidence.episode_id
			FROM cognition_policy_provider_generation_evidence evidence WHERE evidence.episode_id=$1
			UNION ALL
			SELECT 'provider_response_capture',evidence.evidence_id,evidence.call_id,
			       evidence.ref_sha256,evidence.capture_sha256,
			       evidence.capture_bytes,evidence.episode_id
			FROM cognition_policy_provider_response_captures evidence WHERE evidence.episode_id=$1
		) evidence
		JOIN cognition_policy_calls calls ON calls.call_id=evidence.call_id
		JOIN cognition_runtime_snapshots snapshots ON snapshots.snapshot_sha256=calls.snapshot_sha256
		ORDER BY snapshots.call_ordinal,evidence.evidence_kind,evidence.evidence_id
	`, episode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var value cognitionTracePolicyEvidence
		var ordinal int64
		value.Schema = cognitionTracePolicyEvidenceSchemaV1
		if err := rows.Scan(
			&value.EvidenceKind, &value.EvidenceID, &value.CallID,
			&value.ReferenceJSONSHA256, &value.ContentSHA256, &value.Bytes, &ordinal,
		); err != nil {
			return nil, err
		}
		raw, err := cognitionPolicyEvidenceTracePayload(value)
		if err != nil {
			return nil, err
		}
		kind := "policy_response_evidence"
		if value.EvidenceKind == cognitionTraceEvidenceProviderReturn {
			kind = "policy_provider_generation_evidence"
		} else if value.EvidenceKind == cognitionTraceEvidenceProviderCapture {
			kind = "policy_provider_response_capture"
		}
		records = append(records, cognitionTraceRecord{
			Kind: kind, CallOrdinal: ordinal, Phase: 32,
			ID: value.EvidenceID, SHA256: cognitionPayloadSHA(raw),
		})
	}
	return records, rows.Err()
}

func loadCognitionPolicyEvidenceTracePayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	record cognitionTraceRecord,
) ([]byte, error) {
	value := cognitionTracePolicyEvidence{Schema: cognitionTracePolicyEvidenceSchemaV1}
	query := `SELECT call_id,ref_sha256,response_sha256,response_bytes
		FROM cognition_policy_response_evidence WHERE episode_id=$1 AND evidence_id=$2`
	value.EvidenceKind = cognitionTraceEvidenceModelResponse
	if record.Kind == "policy_provider_generation_evidence" {
		query = `SELECT call_id,ref_sha256,generation_sha256,generation_bytes
			FROM cognition_policy_provider_generation_evidence WHERE episode_id=$1 AND evidence_id=$2`
		value.EvidenceKind = cognitionTraceEvidenceProviderReturn
	} else if record.Kind == "policy_provider_response_capture" {
		query = `SELECT call_id,ref_sha256,capture_sha256,capture_bytes
			FROM cognition_policy_provider_response_captures WHERE episode_id=$1 AND evidence_id=$2`
		value.EvidenceKind = cognitionTraceEvidenceProviderCapture
	}
	value.EvidenceID = record.ID
	if err := tx.QueryRow(ctx, query, episode, record.ID).Scan(
		&value.CallID, &value.ReferenceJSONSHA256, &value.ContentSHA256, &value.Bytes,
	); err != nil {
		return nil, fmt.Errorf("load sealed cognition policy evidence metadata: %w", err)
	}
	raw, err := cognitionPolicyEvidenceTracePayload(value)
	if err != nil || cognitionPayloadSHA(raw) != record.SHA256 {
		return nil, fmt.Errorf("%w: sealed cognition policy evidence metadata changed", ErrCognitionConflict)
	}
	return raw, nil
}
