package cognitionruntime

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

type TerminalOutcome string

const (
	TerminalCompleted TerminalOutcome = "completed"
	TerminalFailed    TerminalOutcome = "failed"
)

type SealCommand struct {
	Binding         Binding                           `json:"binding"`
	Outcome         TerminalOutcome                   `json:"outcome"`
	Revision        cognition.WorldRevision           `json:"revision"`
	GraphVersion    uint64                            `json:"graph_version"`
	ObligationGraph cognition.ObligationGraphSnapshot `json:"obligation_graph"`
	Completion      cognition.CompletionResult        `json:"completion"`
	PublicOutcome   string                            `json:"public_outcome"`
}

type TerminalSeal struct {
	Episode     cognition.EpisodeRef    `json:"episode"`
	Outcome     TerminalOutcome         `json:"outcome"`
	Revision    cognition.WorldRevision `json:"revision"`
	TraceSHA256 string                  `json:"trace_sha256"`
}

func sealCommand(binding Binding, progress EpisodeProgress) (SealCommand, error) {
	if progress.Completion == nil {
		return SealCommand{}, fmt.Errorf("%w: terminal progress has no completion", ErrInvalidSeal)
	}
	outcome := TerminalOutcome("")
	switch progress.State {
	case ProgressCompleted:
		outcome = TerminalCompleted
	case ProgressFailed:
		outcome = TerminalFailed
	default:
		return SealCommand{}, fmt.Errorf("%w: progress is not terminal", ErrInvalidSeal)
	}
	command := SealCommand{
		Binding: binding, Outcome: outcome, Revision: progress.Revision,
		GraphVersion: progress.GraphVersion, ObligationGraph: progress.ObligationGraph.Clone(),
		Completion: progress.Completion.Clone(), PublicOutcome: progress.PublicOutcome,
	}
	if err := command.Validate(); err != nil {
		return SealCommand{}, err
	}
	return command, nil
}

func (seal TerminalSeal) ValidateFor(command SealCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	if seal.Episode != command.Binding.Episode || seal.Outcome != command.Outcome ||
		seal.Revision != command.Revision || !validSHA256(seal.TraceSHA256) {
		return fmt.Errorf("%w: terminal seal does not bind the exact command", ErrInvalidSeal)
	}
	return nil
}

func (command SealCommand) Validate() error {
	if err := command.Binding.Validate(); err != nil {
		return err
	}
	if err := command.Revision.Validate(); err != nil || command.Revision.EpisodeID != command.Binding.Episode.ID {
		return fmt.Errorf("%w: final revision does not bind the episode", ErrInvalidSeal)
	}
	if command.GraphVersion == 0 {
		return fmt.Errorf("%w: graph version must be positive", ErrInvalidSeal)
	}
	if err := command.ObligationGraph.Validate(); err != nil {
		return fmt.Errorf("%w: final obligation graph is invalid", ErrInvalidSeal)
	}
	if !utf8.ValidString(command.PublicOutcome) || strings.TrimSpace(command.PublicOutcome) == "" ||
		strings.ContainsRune(command.PublicOutcome, 0) || len(command.PublicOutcome) > cognition.MaxPublicOutcomeBytes {
		return fmt.Errorf("%w: public outcome must be exact bounded text", ErrInvalidSeal)
	}
	graph, err := cognition.RestoreObligationGraph(command.ObligationGraph)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSeal, err)
	}
	status, err := graph.TerminalStatus()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSeal, err)
	}
	root, exists := graph.Obligation(command.ObligationGraph.RootID)
	if !exists {
		return fmt.Errorf("%w: final root is missing", ErrInvalidSeal)
	}
	if err := command.Completion.ValidateFor(root, command.Revision, root.SupportingRefs); err != nil {
		return fmt.Errorf("%w: root completion: %v", ErrInvalidSeal, err)
	}
	if command.Outcome == TerminalCompleted &&
		(status != cognition.ObligationGraphSatisfied || command.Completion.Outcome != cognition.CompletionSatisfied) {
		return fmt.Errorf("%w: completed outcome requires a satisfied graph", ErrInvalidSeal)
	}
	if command.Outcome == TerminalCompleted &&
		(root.Completion == nil || !reflect.DeepEqual(*root.Completion, command.Completion)) {
		return fmt.Errorf("%w: graph root does not retain the sealed completion", ErrInvalidSeal)
	}
	if command.Outcome == TerminalFailed &&
		(status != cognition.ObligationGraphFailed || command.Completion.Outcome != cognition.CompletionUnsatisfied) {
		return fmt.Errorf("%w: failed outcome requires a failed graph", ErrInvalidSeal)
	}
	if command.Outcome != TerminalCompleted && command.Outcome != TerminalFailed {
		return fmt.Errorf("%w: terminal outcome %q is not registered", ErrInvalidSeal, command.Outcome)
	}
	return nil
}

func (command SealCommand) Clone() SealCommand {
	command.ObligationGraph = command.ObligationGraph.Clone()
	command.Completion = command.Completion.Clone()
	return command
}
