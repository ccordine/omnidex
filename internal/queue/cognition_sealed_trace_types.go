package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	CognitionSealedTraceSchemaV2  = "omnidex.cognition-sealed-trace-page.v2"
	MaxCognitionTracePageSize     = 32
	maxCognitionTracePayloadBytes = 2 * 1024 * 1024
	maxCognitionTracePageBytes    = 16 * 1024 * 1024
)

type CognitionTracePageRequest struct {
	Offset int
	Limit  int
}

type CognitionSealedTraceRecord struct {
	Kind        string          `json:"kind"`
	CallOrdinal int64           `json:"call_ordinal"`
	Phase       int             `json:"phase"`
	Sequence    int64           `json:"sequence"`
	ID          string          `json:"id"`
	SHA256      string          `json:"sha256"`
	Payload     json.RawMessage `json:"payload"`
}

type CognitionSealedTracePage struct {
	Schema            string                       `json:"schema"`
	EpisodeID         cognition.EpisodeID          `json:"episode_id"`
	TraceSHA256       string                       `json:"trace_sha256"`
	Seal              CognitionTerminalSeal        `json:"seal"`
	GraphVersion      uint64                       `json:"graph_version"`
	GraphSHA256       string                       `json:"graph_sha256"`
	LedgerVersion     uint64                       `json:"ledger_version"`
	WorkingSetVersion uint64                       `json:"working_set_version"`
	EpisodeStartedAt  time.Time                    `json:"episode_started_at"`
	SealedAt          time.Time                    `json:"sealed_at"`
	TotalRecords      int                          `json:"total_records"`
	Offset            int                          `json:"offset"`
	NextOffset        int                          `json:"next_offset"`
	Records           []CognitionSealedTraceRecord `json:"records"`
}

func validateCognitionTracePageRequest(request CognitionTracePageRequest) error {
	if request.Offset < 0 || request.Limit < 1 || request.Limit > MaxCognitionTracePageSize {
		return fmt.Errorf("cognition trace page requires a nonnegative offset and limit between 1 and %d", MaxCognitionTracePageSize)
	}
	return nil
}

func (trace cognitionTraceAuthority) validate() error {
	if trace.Schema != cognitionTraceAuthoritySchemaV2 || trace.EpisodeID == "" ||
		trace.Revision.EpisodeID != trace.EpisodeID || trace.Revision.Validate() != nil ||
		trace.GraphVersion == 0 || !cognitionDigestPattern.MatchString(trace.GraphSHA256) ||
		trace.LedgerVersion == 0 || len(trace.Records) < 2 || len(trace.Records) > maxCognitionTraceRecords {
		return fmt.Errorf("%w: sealed cognition trace authority is invalid", ErrCognitionConflict)
	}
	seen := make(map[string]struct{}, len(trace.Records))
	for index, record := range trace.Records {
		if !validCognitionTraceKind(record.Kind) || record.CallOrdinal < 0 ||
			record.Phase < 1 || record.Phase > 100 || record.Sequence < 0 ||
			!taskLedgerExact(record.ID) || !cognitionDigestPattern.MatchString(record.SHA256) {
			return fmt.Errorf("%w: sealed cognition trace record %d is invalid", ErrCognitionConflict, index)
		}
		key := record.Kind + "\x00" + record.ID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%w: sealed cognition trace record %d is duplicated", ErrCognitionConflict, index)
		}
		seen[key] = struct{}{}
		if index > 0 && cognitionTraceRecordLess(record, trace.Records[index-1]) {
			return fmt.Errorf("%w: sealed cognition trace record order changed", ErrCognitionConflict)
		}
	}
	return nil
}

func validCognitionTraceKind(kind string) bool {
	switch kind {
	case "accepted_decision_recovery", "action", "action_event", "belief_revision", "cancellation_evidence", "context_projection",
		"episode_progress", "episode_progress_command", "obligation_graph",
		"lifecycle_retirement",
		"plan_revision", "policy_abandonment", "policy_attempt", "policy_result", "policy_timing", "reconciliation_command",
		"policy_provider_generation_evidence", "policy_provider_response_capture", "policy_response_evidence",
		"provider_process_observation", "reconciliation_receipt", "runtime_snapshot", "transition",
		"working_set_event", "working_set_snapshot":
		return true
	default:
		return false
	}
}

func cognitionTraceRecordLess(left, right cognitionTraceRecord) bool {
	if left.CallOrdinal != right.CallOrdinal {
		return left.CallOrdinal < right.CallOrdinal
	}
	if left.Phase != right.Phase {
		return left.Phase < right.Phase
	}
	if left.Sequence != right.Sequence {
		return left.Sequence < right.Sequence
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.ID < right.ID
}

func taskLedgerExact(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsRune(value, 0)
}
