package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const cognitionTerminalFailureReason = "The exact terminal environment state ended this cognition obligation."

func (r *Repository) FailCognitionRuntimeTerminal(
	ctx context.Context,
	command cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("cognition terminal failure requires PostgreSQL and context")
	}
	if command.Result.Outcome != cognition.CompletionUnsatisfied ||
		!command.EnvironmentTerminal || command.PublicOutcome == "" {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf(
			"%w: terminal failure requires an unsatisfied check and exact terminal environment", ErrCognitionConflict,
		)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	defer tx.Rollback(ctx)
	authority, replay, err := lockCognitionProgressAuthorityTx(
		ctx, tx, command, CognitionObligationFail,
	)
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	if replay != nil {
		if err := tx.Commit(ctx); err != nil {
			return cognitionruntime.EpisodeProgress{}, err
		}
		return *replay, nil
	}
	graph, err := cognition.RestoreObligationGraph(authority.Graph.Graph)
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	open := make([]cognition.Obligation, 0)
	for _, obligation := range authority.Graph.Graph.Obligations {
		if obligation.CreatedGeneration != authority.Graph.Graph.Generation ||
			obligation.Status == cognition.ObligationSatisfied || obligation.Status == cognition.ObligationFailed ||
			obligation.Status == cognition.ObligationSuperseded {
			continue
		}
		open = append(open, obligation)
		if err := graph.Transition(
			obligation.ID, authority.Graph.Graph.Generation, cognition.ObligationFailed,
		); err != nil {
			return cognitionruntime.EpisodeProgress{}, err
		}
	}
	if len(open) == 0 {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: terminal failure has no open obligations", ErrCognitionConflict)
	}
	after := graph.Snapshot()
	terminal, err := graph.TerminalStatus()
	if err != nil || terminal != cognition.ObligationGraphFailed || cognitionGraphHasOpenCurrentObligation(after) {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: terminal failure did not close the graph", ErrCognitionConflict)
	}
	if err := persistCognitionTerminalFailureTasksTx(ctx, tx, authority, open); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	record, err := insertCognitionObligationGraphTx(
		ctx, tx, authority.Episode.EpisodeID, authority.Graph.Version+1,
		authority.Descriptor, after, authority.Actor,
	)
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.Actor.JobID, false)
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	if err := requireCognitionGraphTaskProjectionTx(ctx, tx, header, after); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	root, exists := graph.Obligation(after.RootID)
	if !exists {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: failed graph root is missing", ErrCognitionConflict)
	}
	completion, err := cognition.NewCompletionResult(
		root.ID, root.CompletionCheck, authority.Episode.CurrentRevision,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	progress := cognitionProgress(
		authority.Episode, record, cognitionruntime.ProgressFailed, &completion, command.PublicOutcome,
	)
	if err := insertCognitionRuntimeProgressTx(
		ctx, tx, authority.Actor, authority.Descriptor, command, progress,
	); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	return progress, nil
}

func persistCognitionTerminalFailureTasksTx(
	ctx context.Context,
	tx pgx.Tx,
	authority cognitionProgressAuthority,
	obligations []cognition.Obligation,
) error {
	version := authority.Header.Version
	proof := taskstate.Ref{
		URI:     "cognition:episode/" + string(authority.Episode.EpisodeID) + "/terminal",
		Version: strconv.FormatUint(authority.Episode.CurrentRevision.Number, 10),
		Hash:    authority.Descriptor.SHA256, Relation: taskstate.RefVerifies,
	}
	for _, obligation := range obligations {
		wantStatus, err := cognitionTaskNodeStatus(obligation.Status)
		if err != nil {
			return err
		}
		commandID, err := cognitionTaskCommandID(
			authority.Descriptor.ID, "terminal-fail", string(obligation.ID),
		)
		if err != nil {
			return err
		}
		event, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, authority.Actor.JobID, authority.Actor.Generation,
			taskstate.TerminalFailNodeCommand{
				CommandID: commandID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
				NodeID: taskstate.NodeID(obligation.ID), Reason: cognitionTerminalFailureReason, Proof: proof,
			},
		)
		if err != nil {
			return fmt.Errorf("persist terminal cognition obligation %q: %w", obligation.ID, err)
		}
		if event.FromStatus != wantStatus ||
			event.ToStatus != taskstate.NodeFailed || event.Reason != cognitionTerminalFailureReason ||
			len(event.VerificationRefs) != 1 || event.VerificationRefs[0] != proof {
			return fmt.Errorf("%w: terminal failure event for %q is not exact", ErrCognitionConflict, obligation.ID)
		}
		version = event.Version
	}
	return nil
}
