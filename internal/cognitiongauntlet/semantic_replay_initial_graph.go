package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) verifyInitialObligationGraph(
	recordID string,
	graph cognition.ObligationGraphSnapshot,
) error {
	check, err := state.completionAuthority.Resolve(state.goal)
	if err != nil {
		return fmt.Errorf("resolve semantic initial root completion: %w", err)
	}
	rootID, err := cognition.DeriveObligationID(
		state.trace.Header.EpisodeID, cognition.InitialObligationGeneration,
		"", state.goal, check,
	)
	if err != nil {
		return err
	}
	root := cognition.ObligationSpec{
		ID: rootID, Desired: state.goal,
		DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{},
		CompletionCheck: check,
	}
	if err := queue.VerifyCognitionInitialObligationTraceAuthority(
		state.trace.Header.EpisodeID, state.initialActor, root, recordID, graph,
	); err != nil {
		return fmt.Errorf("verify semantic initial obligation graph: %w", err)
	}
	return nil
}
