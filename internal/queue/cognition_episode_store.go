package queue

import (
	"context"
	"fmt"

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
		if err := validateCognitionEpisodeStartReplayTx(ctx, tx, existing, command, facts); err != nil {
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
	brainJSON, brainSHA, err := cognitionJSON(command.BrainBootstrap.AttestedBrain)
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
	if err := insertCognitionEpisodeBootstrapEvidenceTx(
		ctx, tx, command.EpisodeID, command.BrainBootstrap,
	); err != nil {
		return CognitionEpisode{}, err
	}
	if err := persistCognitionProviderProcessActivationTx(
		ctx, tx, command.Authority, CognitionEpisode{
			EpisodeID: command.EpisodeID, Authority: command.Authority,
			AttestedBrain: command.BrainBootstrap.AttestedBrain,
			Status:        CognitionEpisodeActive,
		}, command.ProviderProcessActivation, "",
	); err != nil {
		return CognitionEpisode{}, err
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
