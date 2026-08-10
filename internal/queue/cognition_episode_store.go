package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) StartCognitionEpisode(
	ctx context.Context,
	command CognitionEpisodeStart,
	facts cognitionstate.FactAcceptanceAuthority,
) (CognitionEpisode, error) {
	if err := validateCognitionEpisodeStart(command); err != nil {
		return CognitionEpisode{}, err
	}
	if err := facts.Validate(); err != nil {
		return CognitionEpisode{}, fmt.Errorf("cognition episode fact authority: %w", err)
	}
	if r == nil || r.pool == nil || ctx == nil {
		return CognitionEpisode{}, fmt.Errorf("cognition episode requires PostgreSQL and context")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CognitionEpisode{}, err
	}
	defer tx.Rollback(ctx)
	if _, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, command.Authority); err != nil {
		return CognitionEpisode{}, err
	} else if stepStatus != model.StepStatusRunning {
		return CognitionEpisode{}, staleStepAttemptError(command.Authority, "cognition episode step is not running", nil)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, command.Authority.JobID, true)
	if err != nil {
		return CognitionEpisode{}, err
	}
	if existing, found, err := loadCognitionEpisodeTx(ctx, tx, command.EpisodeID, true); err != nil {
		return CognitionEpisode{}, err
	} else if found {
		if err := validateCognitionEpisodeStartReplayTx(ctx, tx, existing, command, facts.Reference()); err != nil {
			return CognitionEpisode{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CognitionEpisode{}, err
		}
		return existing, nil
	}
	set, err := loadWorkingSetSnapshotTx(ctx, tx, header, command.Authority.Generation, false)
	if err != nil {
		return CognitionEpisode{}, err
	}
	if set.Status != "active" {
		return CognitionEpisode{}, fmt.Errorf("cognition episode requires an active working set")
	}
	if err := createCognitionRootObligationTx(ctx, tx, header, command); err != nil {
		return CognitionEpisode{}, err
	}
	header, err = loadTaskLedgerHeaderTx(ctx, tx, command.Authority.JobID, false)
	if err != nil {
		return CognitionEpisode{}, err
	}
	goalJSON, goalSHA, err := cognitionJSON(command.Goal)
	if err != nil {
		return CognitionEpisode{}, err
	}
	catalogJSON, _, err := cognitionJSON(command.ActionCatalog)
	if err != nil {
		return CognitionEpisode{}, err
	}
	budgetJSON, budgetSHA, err := cognitionJSON(command.Budget)
	if err != nil {
		return CognitionEpisode{}, err
	}
	completionJSON, completionSHA, err := cognitionJSON(command.Completion)
	if err != nil {
		return CognitionEpisode{}, err
	}
	brainJSON, brainSHA, err := cognitionJSON(command.AttestedBrain)
	if err != nil {
		return CognitionEpisode{}, err
	}
	factAuthorityJSON, factAuthoritySHA, err := cognitionJSON(facts.Reference())
	if err != nil {
		return CognitionEpisode{}, err
	}
	factIdentityJSON, factIdentitySHA, err := cognitionJSON(
		cognitionFactAuthorityIdentity(facts.Reference()),
	)
	if err != nil {
		return CognitionEpisode{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_episodes (
			episode_id,schema_name,job_id,generation,step_id,created_attempt,created_worker_id,
			ledger_id,working_set_id,scenario_id,scenario_sha256,goal_json,goal_sha256,
			completion_authority_json,completion_authority_sha256,
			action_catalog_json,action_catalog_id,action_catalog_version,action_catalog_sha256,
			runtime_budget_json,runtime_budget_sha256,attested_brain_json,attested_brain_sha256,
			fact_authority_json,fact_authority_sha256,
			fact_authority_identity_json,fact_authority_identity_sha256,
			current_revision,current_revision_sha256,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,'active')
	`, command.EpisodeID, cognitionEpisodeSchemaV1, command.Authority.JobID, command.Authority.Generation,
		command.Authority.StepID, command.Authority.Attempt, command.Authority.WorkerID, header.ID, set.ID,
		command.Scenario.ID, command.Scenario.SHA256, string(goalJSON), goalSHA,
		string(completionJSON), completionSHA, string(catalogJSON),
		command.ActionCatalog.ID, command.ActionCatalog.Version, command.ActionCatalog.SHA256,
		string(budgetJSON), budgetSHA, string(brainJSON), brainSHA,
		string(factAuthorityJSON), factAuthoritySHA,
		string(factIdentityJSON), factIdentitySHA,
		int64(command.Transition.Current.Number), command.Transition.Current.SHA256); err != nil {
		return CognitionEpisode{}, fmt.Errorf("insert cognition episode %q: %w", command.EpisodeID, err)
	}
	if err := insertCognitionFactAuthorityTx(ctx, tx, command.EpisodeID, facts.Reference()); err != nil {
		return CognitionEpisode{}, err
	}
	if err := insertCognitionObligationProjectionTx(ctx, tx, command, header.ID); err != nil {
		return CognitionEpisode{}, err
	}
	graph, graphDescriptor, err := initialCognitionObligationGraph(command)
	if err != nil {
		return CognitionEpisode{}, err
	}
	if _, err := insertCognitionObligationGraphTx(
		ctx, tx, command.EpisodeID, 1, graphDescriptor, graph, command.Authority,
	); err != nil {
		return CognitionEpisode{}, err
	}
	if err := insertCognitionTransitionTx(
		ctx, tx, command.Authority, command.EpisodeID, command.Transition,
	); err != nil {
		return CognitionEpisode{}, err
	}
	header, err = persistInitialCognitionObservationsTx(
		ctx, tx, header, command.Authority, command.Root.ID, command.Transition.Observations,
	)
	if err != nil {
		return CognitionEpisode{}, err
	}
	if header, err = persistCognitionTransitionFactsTx(
		ctx, tx, header, command.Authority, command.EpisodeID,
		command.Root.ID, command.Transition, facts,
	); err != nil {
		return CognitionEpisode{}, err
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, command.EpisodeID, false)
	if err != nil || !found {
		return CognitionEpisode{}, fmt.Errorf("reload cognition episode %q: %w", command.EpisodeID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionEpisode{}, fmt.Errorf("commit cognition episode %q: %w", command.EpisodeID, err)
	}
	return episode, nil
}

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
