package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

const semanticProductionTraceAuthoritySchemaV2 = "omnidex.cognition-trace-authority.v2"
const semanticReplayTraceHeaderSchemaV1 = "omnidex.replay-production-trace-header.v1"

type semanticReplayTraceHeader struct {
	Schema            string                      `json:"schema"`
	EpisodeID         cognition.EpisodeID         `json:"episode_id"`
	TraceSHA256       string                      `json:"trace_sha256"`
	Seal              queue.CognitionTerminalSeal `json:"seal"`
	GraphVersion      uint64                      `json:"graph_version"`
	GraphSHA256       string                      `json:"graph_sha256"`
	LedgerVersion     uint64                      `json:"ledger_version"`
	WorkingSetVersion uint64                      `json:"working_set_version"`
	EpisodeStartedAt  time.Time                   `json:"episode_started_at"`
	SealedAt          time.Time                   `json:"sealed_at"`
}

func semanticReplayHeader(value queue.CognitionSealedTracePage) semanticReplayTraceHeader {
	return semanticReplayTraceHeader{
		Schema: semanticReplayTraceHeaderSchemaV1, EpisodeID: value.EpisodeID,
		TraceSHA256: value.TraceSHA256, Seal: value.Seal,
		GraphVersion: value.GraphVersion, GraphSHA256: value.GraphSHA256,
		LedgerVersion: value.LedgerVersion, WorkingSetVersion: value.WorkingSetVersion,
		EpisodeStartedAt: value.EpisodeStartedAt, SealedAt: value.SealedAt,
	}
}

func (value semanticReplayTraceHeader) page(records []queue.CognitionSealedTraceRecord) (
	queue.CognitionSealedTracePage,
	error,
) {
	page := queue.CognitionSealedTracePage{
		Schema: queue.CognitionSealedTraceSchemaV2, EpisodeID: value.EpisodeID,
		TraceSHA256: value.TraceSHA256, Seal: value.Seal,
		GraphVersion: value.GraphVersion, GraphSHA256: value.GraphSHA256,
		LedgerVersion: value.LedgerVersion, WorkingSetVersion: value.WorkingSetVersion,
		EpisodeStartedAt: value.EpisodeStartedAt, SealedAt: value.SealedAt,
		TotalRecords: len(records), Offset: 0, NextOffset: -1,
		Records: append([]queue.CognitionSealedTraceRecord(nil), records...),
	}
	if value.Schema != semanticReplayTraceHeaderSchemaV1 ||
		validateProductionTraceHeader(page, value.EpisodeID) != nil ||
		len(records) < 2 || len(records) > queue.MaxCognitionTraceRecords {
		return queue.CognitionSealedTracePage{}, fmt.Errorf("embedded production trace header is invalid")
	}
	for index, record := range records {
		if validateProductionTraceRecord(record, index) != nil {
			return queue.CognitionSealedTracePage{}, fmt.Errorf("embedded production trace record is invalid")
		}
	}
	return page, nil
}

type semanticProductionTraceAuthority struct {
	Schema         string                          `json:"schema"`
	EpisodeID      cognition.EpisodeID             `json:"episode_id"`
	Revision       cognition.WorldRevision         `json:"revision"`
	GraphVersion   uint64                          `json:"graph_version"`
	GraphSHA256    string                          `json:"graph_sha256"`
	LedgerVersion  uint64                          `json:"ledger_version"`
	WorkingVersion uint64                          `json:"working_set_version"`
	Records        []semanticProductionTraceRecord `json:"records"`
}

type semanticProductionTraceRecord struct {
	Kind        string `json:"kind"`
	CallOrdinal int64  `json:"call_ordinal"`
	Phase       int    `json:"phase"`
	Sequence    int64  `json:"sequence"`
	ID          string `json:"id"`
	SHA256      string `json:"sha256"`
}

func validateSemanticProductionTraceDigest(trace productionTrace) error {
	if err := queue.VerifyCognitionSealedTraceRecordOrder(trace.Records); err != nil {
		return fmt.Errorf("production cognition trace order: %w", err)
	}
	records := make([]semanticProductionTraceRecord, len(trace.Records))
	for index, record := range trace.Records {
		records[index] = semanticProductionTraceRecord{
			Kind: record.Kind, CallOrdinal: record.CallOrdinal, Phase: record.Phase,
			Sequence: record.Sequence, ID: record.ID, SHA256: record.SHA256,
		}
	}
	authority := semanticProductionTraceAuthority{
		Schema:    semanticProductionTraceAuthoritySchemaV2,
		EpisodeID: trace.Header.EpisodeID, Revision: trace.Header.Seal.FinalRevision,
		GraphVersion: trace.Header.GraphVersion, GraphSHA256: trace.Header.GraphSHA256,
		LedgerVersion:  trace.Header.LedgerVersion,
		WorkingVersion: trace.Header.WorkingSetVersion, Records: records,
	}
	raw, err := json.Marshal(authority)
	if err != nil {
		return fmt.Errorf("encode exact production cognition trace authority: %w", err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != trace.Header.TraceSHA256 {
		return fmt.Errorf("production cognition trace digest differs from its exact pages")
	}
	return nil
}
