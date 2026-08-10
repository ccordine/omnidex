package cognitionruntime

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

type CompletionRequest struct {
	Binding             Binding                  `json:"binding"`
	SnapshotSHA256      string                   `json:"snapshot_sha256"`
	Goal                cognition.GoalExpression `json:"goal"`
	Revision            cognition.WorldRevision  `json:"revision"`
	Obligation          cognition.Obligation     `json:"obligation"`
	EvidenceRefs        []cognition.EvidenceRef  `json:"evidence_refs"`
	EnvironmentTerminal bool                     `json:"environment_terminal"`
	PublicOutcome       string                   `json:"public_outcome,omitempty"`
}

type CompletionCommand struct {
	Binding                Binding                           `json:"binding"`
	SnapshotSHA256         string                            `json:"snapshot_sha256"`
	GraphVersion           uint64                            `json:"graph_version"`
	ObligationGraph        cognition.ObligationGraphSnapshot `json:"obligation_graph"`
	CompletionEvidenceRefs []cognition.EvidenceRef           `json:"completion_evidence_refs"`
	Result                 cognition.CompletionResult        `json:"result"`
	EnvironmentTerminal    bool                              `json:"environment_terminal"`
	PublicOutcome          string                            `json:"public_outcome,omitempty"`
}

type EpisodeProgressState string

const (
	ProgressActive    EpisodeProgressState = "active"
	ProgressCompleted EpisodeProgressState = "completed"
	ProgressFailed    EpisodeProgressState = "failed"
	ProgressCanceled  EpisodeProgressState = "canceled"
)

type EpisodeProgress struct {
	Episode         cognition.EpisodeRef              `json:"episode"`
	State           EpisodeProgressState              `json:"state"`
	Revision        cognition.WorldRevision           `json:"revision"`
	GraphVersion    uint64                            `json:"graph_version"`
	ObligationGraph cognition.ObligationGraphSnapshot `json:"obligation_graph"`
	Completion      *cognition.CompletionResult       `json:"completion,omitempty"`
	Cancellation    *CancellationSeal                 `json:"cancellation,omitempty"`
	PublicOutcome   string                            `json:"public_outcome,omitempty"`
}

func completionRequest(prepared PreparedSnapshot, binding Binding) CompletionRequest {
	return CompletionRequest{
		Binding: binding, SnapshotSHA256: prepared.Snapshot.SHA256(),
		Goal: prepared.Snapshot.Goal(), Revision: prepared.Snapshot.CurrentRevision(),
		Obligation:          prepared.Snapshot.CurrentObligation(),
		EvidenceRefs:        append([]cognition.EvidenceRef{}, prepared.CompletionEvidenceRefs...),
		EnvironmentTerminal: prepared.EnvironmentTerminal, PublicOutcome: prepared.PublicOutcome,
	}
}

func completionCommand(prepared PreparedSnapshot, binding Binding, result cognition.CompletionResult) CompletionCommand {
	return CompletionCommand{
		Binding: binding, SnapshotSHA256: prepared.Snapshot.SHA256(), GraphVersion: prepared.GraphVersion,
		ObligationGraph:        prepared.ObligationGraph.Clone(),
		CompletionEvidenceRefs: append([]cognition.EvidenceRef{}, prepared.CompletionEvidenceRefs...),
		Result:                 result.Clone(),
		EnvironmentTerminal:    prepared.EnvironmentTerminal, PublicOutcome: prepared.PublicOutcome,
	}
}

func validateCompletionResult(prepared PreparedSnapshot, result cognition.CompletionResult) error {
	current := prepared.Snapshot.CurrentObligation()
	if err := result.ValidateFor(
		current, prepared.Snapshot.CurrentRevision(), prepared.CompletionEvidenceRefs,
	); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProgress, err)
	}
	return nil
}

func validateProgress(
	prepared PreparedSnapshot,
	result cognition.CompletionResult,
	progress EpisodeProgress,
) error {
	if progress.Episode.ID != prepared.Snapshot.CurrentRevision().EpisodeID ||
		progress.Revision != prepared.Snapshot.CurrentRevision() || progress.GraphVersion < prepared.GraphVersion {
		return fmt.Errorf("%w: progress does not bind the prepared episode revision", ErrInvalidProgress)
	}
	if err := progress.ObligationGraph.Validate(); err != nil {
		return fmt.Errorf("%w: graph: %v", ErrInvalidProgress, err)
	}
	if progress.ObligationGraph.Generation != prepared.ObligationGraph.Generation {
		return fmt.Errorf("%w: progress changed the obligation generation", ErrInvalidProgress)
	}
	graph, err := cognition.RestoreObligationGraph(progress.ObligationGraph)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProgress, err)
	}
	terminal, err := graph.TerminalStatus()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProgress, err)
	}
	switch progress.State {
	case ProgressActive:
		if terminal != cognition.ObligationGraphRunning || progress.Completion != nil || progress.PublicOutcome != "" ||
			progress.GraphVersion <= prepared.GraphVersion {
			return fmt.Errorf("%w: active progress must advance to one nonterminal graph", ErrInvalidProgress)
		}
		if err := validateSatisfiedProjection(progress.ObligationGraph, result); err != nil {
			return err
		}
		if countActiveObligations(progress.ObligationGraph) != 1 {
			return fmt.Errorf("%w: active progress requires exactly one active obligation", ErrInvalidProgress)
		}
		return nil
	case ProgressCompleted:
		if terminal != cognition.ObligationGraphSatisfied || !prepared.EnvironmentTerminal ||
			progress.GraphVersion <= prepared.GraphVersion {
			return fmt.Errorf("%w: completed progress requires a terminal environment and satisfied graph", ErrInvalidProgress)
		}
		return validateTerminalProgress(prepared, result, progress, cognition.CompletionSatisfied)
	case ProgressFailed:
		if terminal != cognition.ObligationGraphFailed || !prepared.EnvironmentTerminal ||
			progress.GraphVersion <= prepared.GraphVersion {
			return fmt.Errorf("%w: failed progress requires a terminal environment and failed graph", ErrInvalidProgress)
		}
		return validateTerminalProgress(prepared, result, progress, cognition.CompletionUnsatisfied)
	default:
		return fmt.Errorf("%w: state %q is not registered", ErrInvalidProgress, progress.State)
	}
}

func countActiveObligations(graph cognition.ObligationGraphSnapshot) int {
	count := 0
	for _, obligation := range graph.Obligations {
		if obligation.Status == cognition.ObligationActive {
			count++
		}
	}
	return count
}

func validateSatisfiedProjection(graph cognition.ObligationGraphSnapshot, result cognition.CompletionResult) error {
	for _, obligation := range graph.Obligations {
		if obligation.ID == result.ObligationID && obligation.Status == cognition.ObligationSatisfied &&
			obligation.Completion != nil && reflect.DeepEqual(*obligation.Completion, result) {
			return nil
		}
	}
	return fmt.Errorf("%w: graph does not retain the exact satisfied completion", ErrInvalidProgress)
}

func validateTerminalProgress(
	prepared PreparedSnapshot,
	evaluated cognition.CompletionResult,
	progress EpisodeProgress,
	outcome cognition.CompletionOutcome,
) error {
	if progress.Completion == nil || progress.PublicOutcome != prepared.PublicOutcome ||
		progress.Completion.Outcome != outcome || progress.Completion.ObligationID != progress.ObligationGraph.RootID ||
		progress.Completion.Revision != prepared.Snapshot.CurrentRevision() {
		return fmt.Errorf("%w: terminal completion or public outcome is not exact", ErrInvalidProgress)
	}
	var root cognition.Obligation
	for _, obligation := range progress.ObligationGraph.Obligations {
		if obligation.ID == progress.ObligationGraph.RootID {
			root = obligation
			break
		}
	}
	if err := progress.Completion.ValidateFor(root, progress.Revision, root.SupportingRefs); err != nil {
		return fmt.Errorf("%w: terminal root completion: %v", ErrInvalidProgress, err)
	}
	if outcome == cognition.CompletionSatisfied &&
		(root.Completion == nil || !reflect.DeepEqual(*root.Completion, *progress.Completion)) {
		return fmt.Errorf("%w: final graph does not retain the terminal completion", ErrInvalidProgress)
	}
	if outcome == cognition.CompletionSatisfied && !reflect.DeepEqual(*progress.Completion, evaluated) {
		return fmt.Errorf("%w: satisfied terminal result differs from the evaluated result", ErrInvalidProgress)
	}
	return nil
}
