package queue

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/jackc/pgx/v5"
)

const (
	maxObjectiveCompletionSourceTypeBytes = 64
	maxObjectiveCompletionSourceRefBytes  = 2 << 10
	maxObjectiveCompletionExcerptBytes    = 8 << 10
	maxObjectiveCompletionSummaryBytes    = 2 << 10
)

func (r *Repository) CompleteStepWithEvidence(
	ctx context.Context,
	command CompleteStepEvidenceCommand,
) error {
	command, payloads, err := normalizeCompleteStepEvidenceCommand(command)
	if err != nil {
		return err
	}
	descriptor, err := describeLifecycleOperation(
		command.OperationID, LifecycleCompleteStep, command.CompleteStepCommand,
	)
	if err != nil {
		return err
	}
	return r.completeStep(ctx, command.CompleteStepCommand, descriptor, payloads)
}

func normalizeObjectiveCompletionEvidence(record evidence.Record, jobID, stepID int64) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	if record.JobID != jobID || record.StepID != stepID {
		return nil, fmt.Errorf("evidence owner disagrees with completion authority")
	}
	if record.ID != 0 || !record.CreatedAt.Equal(time.Time{}) {
		return nil, fmt.Errorf("new objective evidence cannot carry a persisted identity")
	}
	if record.Kind != evidence.KindObjectiveCitation {
		return nil, fmt.Errorf("completion evidence kind must be %q", evidence.KindObjectiveCitation)
	}
	for _, field := range []struct {
		name     string
		value    string
		maximum  int
		required bool
	}{
		{"source type", record.SourceType, maxObjectiveCompletionSourceTypeBytes, true},
		{"source reference", record.SourceRef, maxObjectiveCompletionSourceRefBytes, true},
		{"excerpt", record.Excerpt, maxObjectiveCompletionExcerptBytes, true},
		{"summary", record.Summary, maxObjectiveCompletionSummaryBytes, true},
	} {
		if err := validateExactObjectiveEvidenceText(field.name, field.value, field.maximum, field.required); err != nil {
			return nil, err
		}
	}
	if record.ToolName != "" || record.Command != "" || len(record.FilePaths) != 0 || len(record.Warnings) != 0 {
		return nil, fmt.Errorf("objective citation contains unrelated operation evidence fields")
	}
	decodedHash, err := hex.DecodeString(record.Hash)
	if err != nil || len(decodedHash) != 32 || record.Hash != strings.ToLower(record.Hash) {
		return nil, fmt.Errorf("objective citation requires an exact lowercase SHA-256")
	}
	if record.Confidence < 0 || record.Confidence > 1 {
		return nil, fmt.Errorf("objective citation confidence must be between 0 and 1")
	}
	if len(record.RequirementAuthorityBindings) == 0 || len(record.RequirementAuthorityBindings) > 4 {
		return nil, fmt.Errorf("objective citation requires between 1 and 4 requirement authority bindings")
	}
	seenBindings := make(map[string]struct{}, len(record.RequirementAuthorityBindings))
	for _, binding := range record.RequirementAuthorityBindings {
		if err := validateExactObjectiveEvidenceText("requirement authority binding", binding, 512, true); err != nil {
			return nil, err
		}
		if _, duplicate := seenBindings[binding]; duplicate {
			return nil, fmt.Errorf("objective citation requirement authority binding %q is duplicated", binding)
		}
		seenBindings[binding] = struct{}{}
	}
	if err := validateObjectiveCitationMetadata(record); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode objective citation: %w", err)
	}
	return payload, nil
}

func validateExactObjectiveEvidenceText(name, value string, maximum int, required bool) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("objective citation %s must be PostgreSQL-compatible UTF-8", name)
	}
	if required && (value == "" || value != strings.TrimSpace(value)) {
		return fmt.Errorf("objective citation %s must be one exact nonempty value", name)
	}
	if len(value) > maximum {
		return fmt.Errorf("objective citation %s exceeds the %d-byte limit", name, maximum)
	}
	return nil
}

func insertObjectiveCompletionEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	command CompleteStepCommand,
	payloads [][]byte,
) error {
	setPayload, err := json.Marshal(payloadsAsRawMessages(payloads))
	if err != nil {
		return fmt.Errorf("encode objective completion evidence set: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO step_completion_evidence_sets (
			operation_id, job_id, generation, step_id, attempt, worker_id,
			evidence_count, records_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb)
	`, command.OperationID, command.Authority.JobID, command.Authority.Generation,
		command.StepID, command.Authority.Attempt, command.Authority.WorkerID,
		len(payloads), string(setPayload)); err != nil {
		return fmt.Errorf("insert objective completion evidence set: %w", err)
	}
	for index, payload := range payloads {
		var record evidence.Record
		if err := json.Unmarshal(payload, &record); err != nil {
			return fmt.Errorf("decode objective evidence record %d: %w", index, err)
		}
		result, err := tx.Exec(ctx, `
			INSERT INTO evidence (
				job_id, step_id, kind, source_type, source_ref, payload_json,
				completion_operation_id, completion_evidence_index
			) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8)
		`, record.JobID, record.StepID, record.Kind, record.SourceType, record.SourceRef,
			string(payload), command.OperationID, index)
		if err != nil {
			return fmt.Errorf("insert objective completion evidence %d: %w", index, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("objective completion evidence %d was not inserted", index)
		}
	}
	return nil
}

func payloadsAsRawMessages(payloads [][]byte) []json.RawMessage {
	result := make([]json.RawMessage, len(payloads))
	for index := range payloads {
		result[index] = json.RawMessage(payloads[index])
	}
	return result
}

func requireObjectiveCompletionEvidenceReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID LifecycleOperationID,
	payloads [][]byte,
) error {
	setPayload, err := json.Marshal(payloadsAsRawMessages(payloads))
	if err != nil {
		return err
	}
	var count int
	var setMatches, rowsMatch bool
	if err := tx.QueryRow(ctx, `
		SELECT authority.evidence_count, authority.records_json=$2::jsonb,
		       COALESCE((
		           SELECT jsonb_agg(item.payload_json ORDER BY item.completion_evidence_index)
		           FROM evidence AS item
		           WHERE item.completion_operation_id=authority.operation_id
		       ),'[]'::jsonb)=$2::jsonb
		FROM step_completion_evidence_sets AS authority
		WHERE authority.operation_id=$1
	`, operationID, string(setPayload)).Scan(&count, &setMatches, &rowsMatch); err != nil {
		return fmt.Errorf("read objective completion evidence set: %w", err)
	}
	if count != len(payloads) || !setMatches || !rowsMatch {
		return lifecycleReplayStateError(operationID, "objective completion evidence set")
	}
	return nil
}
