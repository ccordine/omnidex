package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

func validateSemanticEpisodeTraceProjection(
	episode SealedEpisode,
	records []queue.CognitionSealedTraceRecord,
	traceSHA256 string,
) error {
	if err := episode.Validate(); err != nil {
		return fmt.Errorf("embedded sealed episode: %w", err)
	}
	trace := episode.Manifest.Trace
	terminal := trace[len(trace)-1]
	if err := validateSemanticEpisodeTerminal(
		terminal, episode.Manifest.FinalRevision, episode.Manifest.Outcome, traceSHA256,
	); err != nil {
		return err
	}
	production := make([]TraceEntry, 0, len(records))
	for index, entry := range trace {
		var discriminator struct {
			Schema string `json:"schema"`
		}
		if err := json.Unmarshal(entry.Payload.Bytes(), &discriminator); err != nil {
			return fmt.Errorf("decode sealed episode trace entry %d discriminator: %w", index+1, err)
		}
		isProductionID := strings.HasPrefix(entry.ID, "production-")
		isProductionPayload := discriminator.Schema == ProductionTraceRecordSchemaV1
		if isProductionID != isProductionPayload {
			return fmt.Errorf("sealed episode production trace wrapper identity is inconsistent")
		}
		if isProductionID {
			production = append(production, entry)
		}
	}
	if len(production) != len(records) {
		return fmt.Errorf("sealed episode production trace wrappers are not reverse-complete")
	}
	for index, record := range records {
		if err := validateSemanticEpisodeProductionEntry(production[index], record); err != nil {
			return fmt.Errorf("sealed episode production wrapper %d: %w", index+1, err)
		}
	}
	return nil
}

func validateSemanticEpisodeTerminal(
	entry TraceEntry,
	revision cognition.WorldRevision,
	outcome Outcome,
	traceSHA256 string,
) error {
	if entry.Kind != TraceTerminal || entry.ID != "terminal-"+traceSHA256 ||
		entry.Revision == nil || *entry.Revision != revision {
		return fmt.Errorf("sealed episode terminal does not bind the exact production trace")
	}
	var value oracleTerminalTrace
	if err := decodeStrictJSON(
		entry.Payload.Bytes(), &value, "sealed episode terminal payload",
	); err != nil {
		return err
	}
	if value.Revision != revision || value.PublicOutcome != outcome.PublicOutcome ||
		value.GoalSatisfied != outcome.GoalSatisfied {
		return fmt.Errorf("sealed episode terminal payload differs from its public outcome")
	}
	return nil
}

func validateSemanticEpisodeProductionEntry(
	entry TraceEntry,
	record queue.CognitionSealedTraceRecord,
) error {
	wrapper := productionRecordPayload{
		Schema: ProductionTraceRecordSchemaV1, Kind: record.Kind,
		CallOrdinal: record.CallOrdinal, Phase: record.Phase, Sequence: record.Sequence,
		ID: record.ID, SHA256: record.SHA256,
		Payload: append(json.RawMessage(nil), record.Payload...),
	}
	payload, err := traceJSONObject(wrapper)
	if err != nil {
		return err
	}
	digest, err := digestJSON(struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		SHA  string `json:"sha"`
	}{record.Kind, record.ID, record.SHA256})
	if err != nil {
		return err
	}
	if entry.Kind != productionTraceKind(record.Kind) ||
		entry.ID != "production-"+digest || entry.Revision != nil ||
		!bytes.Equal(entry.Payload.Bytes(), payload.Bytes()) {
		return fmt.Errorf("does not equal its exact queue source wrapper")
	}
	return nil
}
