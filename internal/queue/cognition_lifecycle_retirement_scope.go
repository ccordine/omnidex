package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func loadLifecycleCognitionScopeTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	stepIDs []int64,
) ([]CognitionEpisode, error) {
	allowed := make(map[int64]struct{}, len(stepIDs))
	for _, stepID := range stepIDs {
		if stepID <= 0 {
			return nil, fmt.Errorf("cognition lifecycle scope contains an invalid step ID")
		}
		allowed[stepID] = struct{}{}
	}
	rows, err := tx.Query(ctx, `
		SELECT episode_id,step_id FROM cognition_episodes
		WHERE job_id=$1 AND generation=$2 AND status='active'
		ORDER BY episode_id FOR UPDATE
	`, jobID, generation)
	if err != nil {
		return nil, err
	}
	type identity struct {
		episode cognition.EpisodeID
		stepID  int64
	}
	identities := make([]identity, 0)
	for rows.Next() {
		var item identity
		if err := rows.Scan(&item.episode, &item.stepID); err != nil {
			rows.Close()
			return nil, err
		}
		if _, exists := allowed[item.stepID]; !exists {
			rows.Close()
			return nil, fmt.Errorf(
				"%w: active cognition episode %q is outside lifecycle step scope",
				ErrCognitionConflict, item.episode,
			)
		}
		identities = append(identities, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	episodes := make([]CognitionEpisode, 0, len(identities))
	for _, identity := range identities {
		episode, found, err := loadCognitionEpisodeTx(ctx, tx, identity.episode, false)
		if err != nil || !found {
			return nil, fmt.Errorf("%w: locked cognition episode disappeared: %v", ErrCognitionConflict, err)
		}
		if episode.Status != CognitionEpisodeActive || episode.Authority.JobID != jobID ||
			episode.Authority.Generation != generation || episode.Authority.StepID != identity.stepID {
			return nil, fmt.Errorf("%w: lifecycle cognition scope changed", ErrCognitionConflict)
		}
		episodes = append(episodes, episode)
	}
	return episodes, nil
}

func validateLifecycleCognitionStateTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
) error {
	journal, found, err := loadCognitionEnvironmentJournalTx(
		ctx, tx, cognition.EpisodeRef{ID: episode.EpisodeID}, true,
	)
	if err != nil {
		return err
	}
	if found && (journal.Current != episode.CurrentRevision || journal.Terminal || journal.TerminalReceipt != nil) {
		return fmt.Errorf("%w: lifecycle cannot relabel terminal or unreconciled environment truth", ErrCognitionConflict)
	}
	terminal, _, err := loadCognitionEnvironmentStateTx(ctx, tx, episode)
	if err != nil {
		return err
	}
	if terminal {
		return fmt.Errorf("%w: lifecycle cannot relabel terminal environment truth", ErrCognitionConflict)
	}
	if err := requireNoUnresolvedCognitionActionTx(ctx, tx, episode.EpisodeID); err != nil {
		return err
	}
	var startedCalls, terminalProgress int
	if err := tx.QueryRow(ctx, `
		SELECT
		 (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1 AND status='started'),
		 (SELECT COUNT(*) FROM cognition_episode_progress
		  WHERE episode_id=$1 AND state IN ('completed','failed'))
	`, episode.EpisodeID).Scan(&startedCalls, &terminalProgress); err != nil {
		return err
	}
	if startedCalls != 0 {
		return fmt.Errorf("%w: lifecycle cognition episode has an indeterminate policy call", ErrCognitionConflict)
	}
	if terminalProgress != 0 {
		return fmt.Errorf("%w: lifecycle cognition episode has pending terminal progress", ErrCognitionConflict)
	}
	restored, err := cognition.RestoreObligationGraph(graph.Graph)
	if err != nil {
		return err
	}
	graphStatus, err := restored.TerminalStatus()
	if err != nil || graphStatus != cognition.ObligationGraphRunning {
		return fmt.Errorf("%w: lifecycle cognition graph is already terminal: %v", ErrCognitionConflict, err)
	}
	return nil
}

func lifecycleCancellationCode(kind LifecycleOperationKind) (cognitionruntime.CancellationCode, string, error) {
	switch kind {
	case LifecycleCancelJob:
		return cognitionruntime.CancellationJobCanceled,
			"The owning job was canceled by code lifecycle authority.", nil
	case LifecycleReplanJob:
		return cognitionruntime.CancellationGenerationRetired,
			"The owning job generation was superseded by code lifecycle authority.", nil
	default:
		return "", "", fmt.Errorf("unregistered cognition lifecycle operation %q", kind)
	}
}
