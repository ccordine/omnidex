package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (store *Store) Seal(
	ctx context.Context,
	command cognitionruntime.SealCommand,
) (cognitionruntime.TerminalSeal, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.TerminalSeal{}, fmt.Errorf("cognition terminal store is uninitialized")
	}
	authority, err := queueAuthority(command.Binding)
	if err != nil {
		return cognitionruntime.TerminalSeal{}, err
	}
	outcome := queue.CognitionEpisodeFailed
	if command.Outcome == cognitionruntime.TerminalCompleted {
		outcome = queue.CognitionEpisodeCompleted
	} else if command.Outcome != cognitionruntime.TerminalFailed {
		return cognitionruntime.TerminalSeal{}, fmt.Errorf("unregistered cognition terminal outcome %q", command.Outcome)
	}
	seal, err := store.repository.SealCognitionEpisode(ctx, queue.CognitionTerminalCommand{
		Authority: authority, EpisodeID: command.Binding.Episode.ID, Outcome: outcome,
		GraphVersion: command.GraphVersion, Completion: command.Completion.Clone(),
		ObligationGraph: command.ObligationGraph.Clone(), PublicOutcome: command.PublicOutcome,
		ExpectedRevision: command.Revision,
	})
	if err != nil {
		return cognitionruntime.TerminalSeal{}, err
	}
	mapped := cognitionruntime.TerminalSeal{
		Episode: command.Binding.Episode, Outcome: command.Outcome,
		Revision: seal.FinalRevision, TraceSHA256: seal.TraceSHA256,
	}
	if err := mapped.ValidateFor(command); err != nil {
		return cognitionruntime.TerminalSeal{}, err
	}
	return mapped, nil
}
