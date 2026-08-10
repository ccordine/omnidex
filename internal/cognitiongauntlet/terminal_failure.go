package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func deriveTerminalFailureTrace(episode SealedEpisode) (FailureTrace, error) {
	code := episode.Manifest.Outcome.FailureCode
	if code == "" {
		return FailureTrace{}, nil
	}
	if code != string(cognitionruntime.CancellationPolicyFailure) &&
		code != string(cognitionruntime.CancellationRunBudgetExhausted) {
		return FailureTrace{}, nil
	}
	var eventID string
	for _, entry := range episode.Manifest.Trace {
		if entry.Kind != TraceFailure {
			continue
		}
		var record productionRecordPayload
		if err := decodeTracePayload(entry.Payload, &record, "terminal failure record"); err != nil {
			return FailureTrace{}, err
		}
		if record.Kind != "cancellation_evidence" {
			continue
		}
		var evidence cognitionruntime.CancellationEvidence
		if err := decodeStrictJSON(record.Payload, &evidence, "terminal cancellation evidence"); err != nil {
			return FailureTrace{}, err
		}
		if err := evidence.Validate(); err != nil || string(evidence.Code) != code || record.ID != evidence.ID {
			return FailureTrace{}, fmt.Errorf("terminal cancellation evidence changed its sealed failure authority")
		}
		if eventID != "" {
			return FailureTrace{}, fmt.Errorf("terminal cancellation has duplicate sealed failure evidence")
		}
		eventID = entry.ID
	}
	if eventID == "" {
		return FailureTrace{}, fmt.Errorf("terminal cancellation lacks sealed failure evidence")
	}
	if code == string(cognitionruntime.CancellationPolicyFailure) {
		return FailureTrace{PolicyRejected: true, PolicyFailureEventID: eventID}, nil
	}
	return FailureTrace{BudgetExhausted: true, BudgetEventID: eventID}, nil
}
