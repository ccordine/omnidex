package cognitionruntime

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

// VerifyEpisodeProgress reconstructs the exact prepared runtime authority and
// reuses the production progress validator. It performs no persistence or
// environment work.
func VerifyEpisodeProgress(
	snapshot cognition.RuntimeSnapshot,
	command CompletionCommand,
	progress EpisodeProgress,
) error {
	prepared := PreparedSnapshot{
		Snapshot: snapshot, ObligationGraph: command.ObligationGraph.Clone(),
		GraphVersion: command.GraphVersion,
		CompletionEvidenceRefs: append(
			[]cognition.EvidenceRef(nil), command.CompletionEvidenceRefs...,
		),
		EnvironmentTerminal: command.EnvironmentTerminal,
		PublicOutcome:       command.PublicOutcome,
	}
	if snapshot.SHA256() != command.SnapshotSHA256 {
		return fmt.Errorf("%w: progress snapshot authority changed", ErrInvalidProgress)
	}
	if err := prepared.ValidateFor(command.Binding); err != nil {
		return err
	}
	if err := validateCompletionResult(prepared, command.Result); err != nil {
		return err
	}
	expected := completionCommand(prepared, command.Binding, command.Result)
	if !reflect.DeepEqual(expected, command) {
		return fmt.Errorf("%w: progress command differs from prepared authority", ErrInvalidProgress)
	}
	return validateProgress(prepared, command.Result, progress)
}
