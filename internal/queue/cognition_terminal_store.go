package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) SealCognitionEpisode(
	ctx context.Context,
	command CognitionTerminalCommand,
) (CognitionTerminalSeal, error) {
	if err := validateCognitionTerminalCommand(command); err != nil {
		return CognitionTerminalSeal{}, err
	}
	if ctx == nil || r == nil || r.pool == nil {
		return CognitionTerminalSeal{}, fmt.Errorf("cognition terminal seal requires PostgreSQL and context")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	defer tx.Rollback(ctx)
	seal, err := sealCognitionEpisodeTx(ctx, tx, command)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionTerminalSeal{}, err
	}
	return seal, nil
}

func sealCognitionEpisodeTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionTerminalCommand,
) (CognitionTerminalSeal, error) {
	if _, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, command.Authority); err != nil {
		return CognitionTerminalSeal{}, err
	} else if stepStatus != model.StepStatusRunning {
		return CognitionTerminalSeal{}, staleStepAttemptError(command.Authority, "cognition terminal actor is not running", nil)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, command.Authority.JobID, true)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, command.EpisodeID, true)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	if !found {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, command.EpisodeID)
	}
	if episode.Status != CognitionEpisodeActive {
		return requireCognitionTerminalReplayTx(ctx, tx, episode, command)
	}
	if err := cognitionAuthorityMatches(command.Authority, episode); err != nil {
		return CognitionTerminalSeal{}, err
	}
	if episode.CurrentRevision != command.ExpectedRevision {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: terminal command observed a stale world revision", ErrCognitionConflict)
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, command.EpisodeID, true)
	if err != nil || !found {
		return CognitionTerminalSeal{}, fmt.Errorf("load terminal cognition graph: %w", err)
	}
	graphJSON, _, _ := cognitionJSON(graph.Graph)
	commandGraphJSON, _, _ := cognitionJSON(command.ObligationGraph)
	if graph.Version != command.GraphVersion || string(graphJSON) != string(commandGraphJSON) {
		return CognitionTerminalSeal{}, fmt.Errorf("%w: terminal obligation graph is not current", ErrCognitionConflict)
	}
	if err := requireNoUnresolvedCognitionActionTx(ctx, tx, command.EpisodeID); err != nil {
		return CognitionTerminalSeal{}, err
	}
	if err := requireCognitionTerminalEnvironmentTx(ctx, tx, episode, command); err != nil {
		return CognitionTerminalSeal{}, err
	}
	if command.Outcome == CognitionEpisodeCanceled {
		header, err = cancelCognitionObligationNodesTx(
			ctx, tx, header, command.EpisodeID, command.ObligationGraph,
			command.Authority.JobID, command.Authority.Generation, cognitionTerminalAuthorityWorker,
		)
		if err != nil {
			return CognitionTerminalSeal{}, err
		}
	} else if err := requireCognitionGraphTaskProjectionTx(ctx, tx, header, command.ObligationGraph); err != nil {
		return CognitionTerminalSeal{}, err
	}
	workingEvent, err := closeCognitionTerminalWorkingSetTx(
		ctx, tx, header, command.Authority.JobID, command.Authority.Generation,
		command.EpisodeID, command.Outcome, command.ExpectedRevision,
		command.ObligationGraph, &command.Authority, nil,
	)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	traceJSON, traceSHA, err := buildCognitionTraceAuthorityTx(
		ctx, tx, episode, graph, header.Version, workingEvent.Version,
	)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	if err := persistCognitionTerminalSealTx(
		ctx, tx, command.EpisodeID, command.Outcome, command.ExpectedRevision,
		command.Completion, command.ObligationGraph, header.Version, workingEvent.Version,
		traceJSON, traceSHA, command.PublicOutcome, workerCognitionTerminalAuthority(command.Authority),
	); err != nil {
		return CognitionTerminalSeal{}, err
	}
	seal, err := loadCognitionTerminalSealTx(ctx, tx, command.EpisodeID)
	if err != nil {
		return CognitionTerminalSeal{}, err
	}
	return seal, nil
}

func requireNoUnresolvedCognitionActionTx(ctx context.Context, tx pgx.Tx, episode cognition.EpisodeID) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cognition_actions WHERE episode_id=$1 AND status IN ('prepared','dispatched'))`, episode).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: cognition episode has an unresolved action", ErrCognitionConflict)
	}
	return nil
}

func requireCognitionTerminalEnvironmentTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	command CognitionTerminalCommand,
) error {
	if command.Outcome == CognitionEpisodeCanceled {
		return nil
	}
	var terminal bool
	var outcome string
	if err := tx.QueryRow(ctx, `
		SELECT terminal,public_outcome FROM cognition_transitions
		WHERE episode_id=$1 AND revision=$2
	`, episode.EpisodeID, int64(episode.CurrentRevision.Number)).Scan(&terminal, &outcome); err != nil {
		return err
	}
	if !terminal || outcome != command.PublicOutcome {
		return fmt.Errorf("%w: completion is not confirmed by the current environment transition", ErrCognitionConflict)
	}
	return nil
}
