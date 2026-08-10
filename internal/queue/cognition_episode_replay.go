package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/jackc/pgx/v5"
)

func validateCognitionEpisodeStartReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	existing CognitionEpisode,
	command CognitionEpisodeStart,
	factAuthority cognitionstate.FactAcceptanceAuthorityRef,
) error {
	goalJSON, _, err := cognitionJSON(command.Goal)
	if err != nil {
		return err
	}
	existingGoal, _, err := cognitionJSON(existing.Goal)
	if err != nil {
		return err
	}
	completionJSON, _, err := cognitionJSON(command.Completion)
	if err != nil {
		return err
	}
	existingCompletion, _, err := cognitionJSON(existing.Completion)
	if err != nil {
		return err
	}
	catalogJSON, _, err := cognitionJSON(command.ActionCatalog)
	if err != nil {
		return err
	}
	existingCatalog, _, err := cognitionJSON(existing.ActionCatalog)
	if err != nil {
		return err
	}
	budgetJSON, _, err := cognitionJSON(command.Budget)
	if err != nil {
		return err
	}
	existingBudget, _, err := cognitionJSON(existing.Budget)
	if err != nil {
		return err
	}
	brainJSON, _, err := cognitionJSON(command.AttestedBrain)
	if err != nil {
		return err
	}
	existingBrain, _, err := cognitionJSON(existing.AttestedBrain)
	if err != nil {
		return err
	}
	if existing.Authority.JobID != command.Authority.JobID ||
		existing.Authority.Generation != command.Authority.Generation ||
		existing.Authority.StepID != command.Authority.StepID ||
		existing.Scenario != command.Scenario ||
		string(goalJSON) != string(existingGoal) ||
		string(completionJSON) != string(existingCompletion) || string(catalogJSON) != string(existingCatalog) ||
		string(budgetJSON) != string(existingBudget) || string(brainJSON) != string(existingBrain) ||
		!reflect.DeepEqual(existing.FactAuthority, factAuthority) {
		return fmt.Errorf("%w: cognition episode start identity changed", ErrCognitionConflict)
	}
	if err := requireExactInitialCognitionTransitionTx(ctx, tx, command); err != nil {
		return err
	}
	return requireExactInitialCognitionGraphTx(ctx, tx, command)
}

func requireExactInitialCognitionTransitionTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionEpisodeStart,
) error {
	wantRaw, wantSHA, err := cognitionJSON(command.Transition)
	if err != nil {
		return err
	}
	var gotRaw, gotSHA string
	if err := tx.QueryRow(ctx, `
		SELECT transition_json,transition_sha256 FROM cognition_transitions
		WHERE episode_id=$1 AND revision=1 AND action_id IS NULL
	`, command.EpisodeID).Scan(&gotRaw, &gotSHA); err != nil {
		return fmt.Errorf("load cognition initial transition replay: %w", err)
	}
	if gotSHA != wantSHA || gotRaw != string(wantRaw) {
		return fmt.Errorf("%w: cognition initial transition changed", ErrCognitionConflict)
	}
	return nil
}

func requireExactInitialCognitionGraphTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionEpisodeStart,
) error {
	wantGraph, wantDescriptor, err := initialCognitionObligationGraph(command)
	if err != nil {
		return err
	}
	wantRaw, _, err := cognitionJSON(wantGraph)
	if err != nil {
		return err
	}
	var gotID, gotSHA, gotKind, gotRaw, gotGraphSHA string
	if err := tx.QueryRow(ctx, `
		SELECT command_id,command_sha256,command_kind,graph_json,graph_sha256
		FROM cognition_obligation_graphs WHERE episode_id=$1 AND graph_version=1
	`, command.EpisodeID).Scan(&gotID, &gotSHA, &gotKind, &gotRaw, &gotGraphSHA); err != nil {
		return fmt.Errorf("load cognition initial obligation replay: %w", err)
	}
	if gotID != wantDescriptor.ID || gotSHA != wantDescriptor.SHA256 ||
		gotKind != string(CognitionObligationInitial) || gotRaw != string(wantRaw) ||
		gotGraphSHA != wantGraph.SHA256 {
		return fmt.Errorf("%w: cognition initial obligation graph changed", ErrCognitionConflict)
	}
	return nil
}
