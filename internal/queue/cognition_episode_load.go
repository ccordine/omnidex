package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CognitionEpisode(
	ctx context.Context,
	episodeID cognition.EpisodeID,
) (CognitionEpisode, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return CognitionEpisode{}, fmt.Errorf("cognition episode read requires PostgreSQL and context")
	}
	if err := cognitionEpisodeIdentityValid(episodeID); err != nil {
		return CognitionEpisode{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CognitionEpisode{}, err
	}
	defer tx.Rollback(ctx)
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, episodeID, false)
	if err != nil {
		return CognitionEpisode{}, err
	}
	if !found {
		return CognitionEpisode{}, fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, episodeID)
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionEpisode{}, err
	}
	return episode, nil
}

func loadCognitionEpisodeTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	lock bool,
) (CognitionEpisode, bool, error) {
	query := `
		SELECT episodes.episode_id,episodes.job_id,episodes.generation,episodes.step_id,
		       episodes.created_attempt,episodes.created_worker_id,episodes.ledger_id,
		       episodes.working_set_id,episodes.scenario_id,episodes.scenario_sha256,
		       episodes.goal_json,episodes.completion_authority_json,episodes.action_catalog_json,episodes.current_revision,
		       episodes.runtime_budget_json,episodes.attested_brain_json,episodes.fact_authority_json,
		       episodes.current_revision_sha256,episodes.status,episodes.action_count,
		       episodes.total_cost,episodes.version,COALESCE(episodes.terminal_outcome,''),
		       episodes.created_at,episodes.updated_at,attempts.expires_at
		FROM cognition_episodes episodes
		JOIN job_step_attempts attempts ON attempts.job_id=episodes.job_id
		 AND attempts.generation=episodes.generation AND attempts.step_id=episodes.step_id
		 AND attempts.attempt=episodes.created_attempt AND attempts.worker_id=episodes.created_worker_id
		WHERE episodes.episode_id=$1`
	if lock {
		query += ` FOR UPDATE OF episodes`
	}
	var episode CognitionEpisode
	var goalJSON, completionJSON, catalogJSON, budgetJSON, brainJSON, factAuthorityJSON []byte
	var revision int64
	err := tx.QueryRow(ctx, query, episodeID).Scan(
		&episode.EpisodeID, &episode.Authority.JobID, &episode.Authority.Generation,
		&episode.Authority.StepID, &episode.Authority.Attempt, &episode.Authority.WorkerID,
		&episode.LedgerID, &episode.WorkingSetID, &episode.Scenario.ID, &episode.Scenario.SHA256,
		&goalJSON, &completionJSON, &catalogJSON, &revision, &budgetJSON, &brainJSON, &factAuthorityJSON,
		&episode.CurrentRevision.SHA256, &episode.Status,
		&episode.SuccessfulActions, &episode.TotalCost, &episode.Version, &episode.TerminalOutcome,
		&episode.CreatedAt, &episode.UpdatedAt, &episode.CreatedAttemptExpires,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CognitionEpisode{}, false, nil
	}
	if err != nil {
		return CognitionEpisode{}, false, fmt.Errorf("load cognition episode %q: %w", episodeID, err)
	}
	episode.CreatedAt = episode.CreatedAt.UTC()
	episode.UpdatedAt = episode.UpdatedAt.UTC()
	episode.CreatedAttemptExpires = episode.CreatedAttemptExpires.UTC()
	episode.CurrentRevision.EpisodeID = episode.EpisodeID
	episode.CurrentRevision.Number = uint64(revision)
	if err := json.Unmarshal(goalJSON, &episode.Goal); err != nil {
		return CognitionEpisode{}, false, fmt.Errorf("decode cognition episode goal: %w", err)
	}
	if err := json.Unmarshal(completionJSON, &episode.Completion); err != nil {
		return CognitionEpisode{}, false, fmt.Errorf("decode cognition completion authority: %w", err)
	}
	if err := json.Unmarshal(catalogJSON, &episode.ActionCatalog); err != nil {
		return CognitionEpisode{}, false, fmt.Errorf("decode cognition episode catalog: %w", err)
	}
	if err := json.Unmarshal(budgetJSON, &episode.Budget); err != nil {
		return CognitionEpisode{}, false, fmt.Errorf("decode cognition episode budget: %w", err)
	}
	if err := json.Unmarshal(brainJSON, &episode.AttestedBrain); err != nil {
		return CognitionEpisode{}, false, fmt.Errorf("decode cognition attested brain: %w", err)
	}
	if err := json.Unmarshal(factAuthorityJSON, &episode.FactAuthority); err != nil {
		return CognitionEpisode{}, false, fmt.Errorf("decode cognition fact authority: %w", err)
	}
	if err := validateLoadedCognitionEpisode(episode); err != nil {
		return CognitionEpisode{}, false, err
	}
	return episode, true, nil
}

func validateLoadedCognitionEpisode(episode CognitionEpisode) error {
	if err := cognitionEpisodeIdentityValid(episode.EpisodeID); err != nil {
		return err
	}
	if err := episode.Scenario.Validate(); err != nil {
		return fmt.Errorf("persisted cognition scenario: %w", err)
	}
	if err := episode.Goal.Validate(); err != nil {
		return fmt.Errorf("persisted cognition goal: %w", err)
	}
	if err := episode.Completion.Validate(); err != nil {
		return fmt.Errorf("persisted cognition completion authority: %w", err)
	}
	if err := episode.ActionCatalog.Validate(); err != nil {
		return fmt.Errorf("persisted cognition catalog: %w", err)
	}
	if err := episode.Budget.Validate(); err != nil {
		return fmt.Errorf("persisted cognition budget: %w", err)
	}
	if err := episode.AttestedBrain.Validate(); err != nil {
		return fmt.Errorf("persisted cognition attested brain: %w", err)
	}
	if err := episode.FactAuthority.Validate(); err != nil {
		return fmt.Errorf("persisted cognition fact authority: %w", err)
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(episode.AttestedBrain.Ref, episode.Budget); err != nil {
		return fmt.Errorf("persisted cognition budget brain binding: %w", err)
	}
	if err := episode.CurrentRevision.Validate(); err != nil {
		return fmt.Errorf("persisted cognition revision: %w", err)
	}
	return nil
}
