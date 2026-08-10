package cognitionruntime

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

func recoveredCanceledProgress(binding Binding, progress EpisodeProgress) (StepResult, error) {
	if progress.Episode != binding.Episode || progress.Revision.EpisodeID != binding.Episode.ID ||
		progress.Revision.Validate() != nil || progress.GraphVersion == 0 ||
		progress.Completion == nil || progress.Cancellation == nil ||
		progress.Completion.Outcome != cognition.CompletionUnsatisfied ||
		progress.Completion.Revision != progress.Revision ||
		progress.Completion.ObligationID != progress.ObligationGraph.RootID ||
		progress.Cancellation.Episode != binding.Episode || progress.Cancellation.Validate() != nil {
		return StepResult{Binding: binding}, fmt.Errorf("%w: canceled progress identity is invalid", ErrInvalidProgress)
	}
	if !utf8.ValidString(progress.PublicOutcome) || strings.TrimSpace(progress.PublicOutcome) == "" ||
		strings.ContainsRune(progress.PublicOutcome, 0) || len(progress.PublicOutcome) > cognition.MaxPublicOutcomeBytes {
		return StepResult{Binding: binding}, fmt.Errorf("%w: canceled progress outcome is invalid", ErrInvalidProgress)
	}
	graph, err := cognition.RestoreObligationGraph(progress.ObligationGraph)
	if err != nil {
		return StepResult{Binding: binding}, fmt.Errorf("%w: canceled graph: %v", ErrInvalidProgress, err)
	}
	root, exists := graph.Obligation(progress.ObligationGraph.RootID)
	if !exists || progress.Completion.ValidateFor(root, progress.Revision, root.SupportingRefs) != nil {
		return StepResult{Binding: binding}, fmt.Errorf("%w: canceled root completion is invalid", ErrInvalidProgress)
	}
	completion := progress.Completion.Clone()
	cancellation := *progress.Cancellation
	return StepResult{
		State: StepEpisodeCanceled, Binding: binding, Revision: progress.Revision,
		Completion: &completion, Cancellation: &cancellation,
	}, nil
}
