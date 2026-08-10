package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CognitionRuntimeTerminalProgress(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.EpisodeProgress, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("cognition terminal progress requires PostgreSQL and context")
	}
	authority, err := cognitionRuntimeAuthority(binding)
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return nil, err
	} else if status != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "cognition terminal recovery actor is not running", nil)
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, binding.Episode.ID, false)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, binding.Episode.ID)
	}
	if err := cognitionAuthorityMatches(authority, episode); err != nil {
		return nil, err
	}
	if episode.Status == CognitionEpisodeCanceled {
		progress, err := loadCanceledCognitionProgressTx(ctx, tx, episode)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		copy := cloneCognitionEpisodeProgress(progress)
		return &copy, nil
	}
	progress, err := loadLatestTerminalCognitionProgressTx(ctx, tx, episode.EpisodeID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		if episode.Status != CognitionEpisodeActive {
			return nil, fmt.Errorf("%w: terminal cognition episode has no durable progress", ErrCognitionConflict)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := validateRecoveredCognitionProgressTx(ctx, tx, episode, *progress); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	copy := cloneCognitionEpisodeProgress(*progress)
	return &copy, nil
}

func validateRecoveredCognitionProgressTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	progress cognitionruntime.EpisodeProgress,
) error {
	if progress.Episode.ID != episode.EpisodeID || progress.Revision != episode.CurrentRevision ||
		progress.Completion == nil || progress.PublicOutcome == "" {
		return fmt.Errorf("%w: terminal cognition progress identity is incomplete", ErrCognitionConflict)
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, false)
	if err != nil || !found {
		return fmt.Errorf("%w: load recovered cognition graph: %v", ErrCognitionConflict, err)
	}
	if graph.Version != progress.GraphVersion || graph.Graph.SHA256 != progress.ObligationGraph.SHA256 {
		return fmt.Errorf("%w: terminal progress graph is not current", ErrCognitionConflict)
	}
	restored, err := cognition.RestoreObligationGraph(progress.ObligationGraph)
	if err != nil {
		return err
	}
	terminal, err := restored.TerminalStatus()
	if err != nil {
		return err
	}
	wantOutcome := CognitionEpisodeStatus("")
	switch progress.State {
	case cognitionruntime.ProgressCompleted:
		if terminal != cognition.ObligationGraphSatisfied || progress.Completion.Outcome != cognition.CompletionSatisfied {
			return fmt.Errorf("%w: recovered completion is not satisfied", ErrCognitionConflict)
		}
		wantOutcome = CognitionEpisodeCompleted
	case cognitionruntime.ProgressFailed:
		if terminal != cognition.ObligationGraphFailed || progress.Completion.Outcome != cognition.CompletionUnsatisfied {
			return fmt.Errorf("%w: recovered failure is not failed", ErrCognitionConflict)
		}
		wantOutcome = CognitionEpisodeFailed
	default:
		return fmt.Errorf("%w: active progress cannot enter terminal recovery", ErrCognitionConflict)
	}
	if episode.Status == CognitionEpisodeActive {
		terminalState, outcome, err := loadCognitionEnvironmentStateTx(ctx, tx, episode)
		if err != nil || !terminalState || outcome != progress.PublicOutcome {
			return fmt.Errorf("%w: pending terminal progress lacks exact environment state", ErrCognitionConflict)
		}
		return nil
	}
	if episode.Status != wantOutcome || episode.TerminalOutcome != progress.PublicOutcome {
		return fmt.Errorf("%w: sealed episode differs from terminal progress", ErrCognitionConflict)
	}
	seal, err := loadCognitionTerminalSealTx(ctx, tx, episode.EpisodeID)
	if err != nil {
		return err
	}
	_, completionSHA, err := cognitionJSON(*progress.Completion)
	if err != nil {
		return err
	}
	if seal.Outcome != wantOutcome || seal.FinalRevision != progress.Revision ||
		seal.ObligationGraphSHA256 != progress.ObligationGraph.SHA256 || seal.CompletionSHA256 != completionSHA {
		return fmt.Errorf("%w: terminal seal differs from durable progress", ErrCognitionConflict)
	}
	return nil
}
