package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) AdvanceCognitionRuntimeSatisfied(
	ctx context.Context,
	command cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("cognition satisfaction requires PostgreSQL and context")
	}
	if command.Result.Outcome != cognition.CompletionSatisfied {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: satisfaction requires a satisfied result", ErrCognitionConflict)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	defer tx.Rollback(ctx)
	authority, replay, err := lockCognitionProgressAuthorityTx(
		ctx, tx, command, CognitionObligationSatisfy,
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
	before := graph.Snapshot()
	current, exists := graph.Obligation(command.Result.ObligationID)
	if !exists {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf(
			"%w: satisfied obligation is absent", ErrCognitionConflict,
		)
	}
	missing := missingCognitionCompletionEvidence(current.SupportingRefs, command.Result.EvidenceRefs)
	if len(missing) > 0 {
		if err := graph.AddSupportingEvidence(
			command.Result.ObligationID, before.Generation, missing,
		); err != nil {
			return cognitionruntime.EpisodeProgress{}, err
		}
		if err := insertCognitionObligationSupportingRefsTx(
			ctx, tx, authority.Episode.EpisodeID, command.Result.ObligationID, missing,
		); err != nil {
			return cognitionruntime.EpisodeProgress{}, err
		}
	}
	dependents := directCognitionCompletionDependents(before, command.Result.ObligationID)
	for _, dependentID := range dependents {
		dependent, exists := graph.Obligation(dependentID)
		if !exists {
			return cognitionruntime.EpisodeProgress{}, fmt.Errorf(
				"%w: completion dependent %q is absent", ErrCognitionConflict, dependentID,
			)
		}
		missing = missingCognitionCompletionEvidence(dependent.SupportingRefs, command.Result.EvidenceRefs)
		if len(missing) > 0 {
			if err := graph.AddSupportingEvidence(dependentID, before.Generation, missing); err != nil {
				return cognitionruntime.EpisodeProgress{}, err
			}
			if err := insertCognitionObligationSupportingRefsTx(
				ctx, tx, authority.Episode.EpisodeID, dependentID, missing,
			); err != nil {
				return cognitionruntime.EpisodeProgress{}, err
			}
		}
	}
	if err := graph.Satisfy(command.Result.ObligationID, before.Generation, command.Result); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	afterSatisfaction := graph.Snapshot()
	terminal, err := graph.TerminalStatus()
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	nextID := cognition.ObligationID("")
	state := cognitionruntime.ProgressActive
	publicOutcome := ""
	var completion *cognition.CompletionResult
	if terminal == cognition.ObligationGraphSatisfied {
		if !command.EnvironmentTerminal || command.PublicOutcome == "" ||
			cognitionGraphHasOpenCurrentObligation(afterSatisfaction) {
			return cognitionruntime.EpisodeProgress{}, fmt.Errorf(
				"%w: satisfied root requires an exact terminal environment and no open obligations", ErrCognitionConflict,
			)
		}
		state, publicOutcome = cognitionruntime.ProgressCompleted, command.PublicOutcome
		root, exists := graph.Obligation(afterSatisfaction.RootID)
		if !exists || root.Completion == nil {
			return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: satisfied root has no completion", ErrCognitionConflict)
		}
		value := root.Completion.Clone()
		completion = &value
	} else {
		nextID, err = firstReadyCognitionObligation(afterSatisfaction)
		if err != nil {
			return cognitionruntime.EpisodeProgress{}, err
		}
		if err := graph.Transition(nextID, before.Generation, cognition.ObligationActive); err != nil {
			return cognitionruntime.EpisodeProgress{}, err
		}
	}
	if err := persistCognitionSatisfiedProgressTaskTx(
		ctx, tx, authority, command, before, afterSatisfaction, nextID,
	); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	after := graph.Snapshot()
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
	handoffEvidence := command.Result.EvidenceRefs
	if len(dependents) == 0 {
		handoffEvidence = []cognition.EvidenceRef{}
	}
	if err := persistCognitionCompletionHandoffTx(
		ctx, tx, authority, header, command.Result.ObligationID,
		dependents, handoffEvidence,
	); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	if err := requireCognitionGraphTaskProjectionTx(ctx, tx, header, after); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	progress := cognitionProgress(authority.Episode, record, state, completion, publicOutcome)
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

func missingCognitionCompletionEvidence(
	existing []cognition.EvidenceRef,
	result []cognition.EvidenceRef,
) []cognition.EvidenceRef {
	known := make(map[cognition.EvidenceRef]struct{}, len(existing))
	for _, ref := range existing {
		known[ref] = struct{}{}
	}
	missing := make([]cognition.EvidenceRef, 0, len(result))
	for _, ref := range result {
		if _, found := known[ref]; found {
			continue
		}
		known[ref] = struct{}{}
		missing = append(missing, ref)
	}
	return missing
}

func firstReadyCognitionObligation(
	graph cognition.ObligationGraphSnapshot,
) (cognition.ObligationID, error) {
	for _, obligation := range graph.Obligations {
		if obligation.CreatedGeneration == graph.Generation && obligation.Status == cognition.ObligationReady {
			return obligation.ID, nil
		}
	}
	return "", fmt.Errorf("%w: satisfied obligation left no deterministic ready successor", ErrCognitionConflict)
}

func persistCognitionSatisfiedProgressTaskTx(
	ctx context.Context,
	tx pgx.Tx,
	authority cognitionProgressAuthority,
	command cognitionruntime.CompletionCommand,
	before, after cognition.ObligationGraphSnapshot,
	nextID cognition.ObligationID,
) error {
	version := authority.Header.Version
	apply := func(label string, value taskstate.Command) error {
		event, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, authority.Actor.JobID, authority.Actor.Generation, value,
		)
		if err != nil {
			return fmt.Errorf("persist cognition progress %s: %w", label, err)
		}
		version = event.Version
		return nil
	}
	commandID := func(suffix string) (taskstate.CommandID, error) {
		return cognitionTaskCommandID(authority.Descriptor.ID, suffix)
	}
	if err := persistSatisfiedCognitionObligationTx(
		authority.Episode, command.Result, before, after, &version, apply, commandID,
	); err != nil {
		return err
	}
	if nextID == "" {
		return nil
	}
	id, err := commandID("activate-" + string(nextID))
	if err != nil {
		return err
	}
	return apply("deterministic activation", taskstate.TransitionNodeCommand{
		CommandID: id, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		NodeID: taskstate.NodeID(nextID), To: taskstate.NodeActive,
	})
}
