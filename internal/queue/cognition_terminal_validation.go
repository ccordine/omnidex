package queue

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

func validateCognitionTerminalCommand(command CognitionTerminalCommand) error {
	if err := validateStepAttemptAuthority(command.Authority); err != nil {
		return err
	}
	if err := cognitionEpisodeIdentityValid(command.EpisodeID); err != nil {
		return err
	}
	if !validCognitionTerminalStatus(command.Outcome) {
		return fmt.Errorf("unregistered cognition terminal outcome %q", command.Outcome)
	}
	if err := command.ExpectedRevision.Validate(); err != nil {
		return fmt.Errorf("cognition terminal revision: %w", err)
	}
	if command.ExpectedRevision.EpisodeID != command.EpisodeID {
		return fmt.Errorf("cognition terminal revision does not bind the episode")
	}
	if err := command.Completion.Validate(); err != nil {
		return err
	}
	if err := command.ObligationGraph.Validate(); err != nil {
		return err
	}
	if command.GraphVersion == 0 ||
		command.Completion.Revision != command.ExpectedRevision ||
		command.Completion.ObligationID != command.ObligationGraph.RootID {
		return fmt.Errorf("%w: terminal command authority disagrees with its graph or completion", ErrCognitionConflict)
	}
	if !utf8.ValidString(command.PublicOutcome) || command.PublicOutcome != strings.TrimSpace(command.PublicOutcome) ||
		command.PublicOutcome == "" || strings.ContainsRune(command.PublicOutcome, 0) ||
		len(command.PublicOutcome) > cognition.MaxPublicOutcomeBytes {
		return fmt.Errorf("cognition terminal public outcome must be exact bounded UTF-8")
	}
	graph, err := cognition.RestoreObligationGraph(command.ObligationGraph)
	if err != nil {
		return err
	}
	status, err := graph.TerminalStatus()
	if err != nil {
		return err
	}
	switch command.Outcome {
	case CognitionEpisodeCompleted:
		if command.Completion.Outcome != cognition.CompletionSatisfied || status != cognition.ObligationGraphSatisfied {
			return fmt.Errorf("%w: completed episode requires a satisfied code-owned graph", ErrCognitionConflict)
		}
		for _, obligation := range command.ObligationGraph.Obligations {
			if obligation.CreatedGeneration == command.ObligationGraph.Generation &&
				obligation.Status != cognition.ObligationSatisfied {
				return fmt.Errorf("%w: completed episode retains open obligation %q", ErrCognitionConflict, obligation.ID)
			}
		}
	case CognitionEpisodeFailed:
		if command.Completion.Outcome != cognition.CompletionUnsatisfied || status != cognition.ObligationGraphFailed {
			return fmt.Errorf("%w: failed episode requires a failed graph and unsatisfied check", ErrCognitionConflict)
		}
		if cognitionGraphHasOpenCurrentObligation(command.ObligationGraph) {
			return fmt.Errorf("%w: failed episode retains open obligations", ErrCognitionConflict)
		}
	case CognitionEpisodeCanceled:
		if command.Completion.Outcome != cognition.CompletionUnsatisfied {
			return fmt.Errorf("%w: canceled episode cannot claim satisfaction", ErrCognitionConflict)
		}
	}
	return nil
}

func cognitionGraphHasOpenCurrentObligation(graph cognition.ObligationGraphSnapshot) bool {
	for _, obligation := range graph.Obligations {
		if obligation.CreatedGeneration != graph.Generation {
			continue
		}
		if obligation.Status != cognition.ObligationSatisfied && obligation.Status != cognition.ObligationFailed &&
			obligation.Status != cognition.ObligationSuperseded {
			return true
		}
	}
	return false
}
