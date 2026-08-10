package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func persistCognitionActionFailureTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episode CognitionEpisode,
	record CognitionActionRecord,
	failure cognition.ActionFailure,
) error {
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return err
	}
	schema, exists := episode.ActionCatalog.Schema(record.Action.Request.Kind)
	if !exists || schema.Ref() != record.Schema {
		return fmt.Errorf("cognition failure action schema is not registered")
	}
	mutation, err := cognitionstate.MapActionFailure(cognitionstate.ActionFailureInput{
		Ledger: ledger.MaterializedState(), ScopeNodeID: taskstate.NodeID(record.ObligationID),
		Binding:          cognitionstate.ActionBinding{Action: record.Action, Schema: schema},
		ExpectedRevision: record.ExpectedRevision, Failure: failure,
	})
	if err != nil {
		return err
	}
	if _, err := applyQueueOwnedTaskCommandTx(
		ctx, tx, record.Origin.JobID, record.Origin.Generation, mutation.Command(),
	); err != nil {
		return fmt.Errorf("persist cognition failure in task ledger: %w", err)
	}
	return nil
}

func persistCognitionObservationsTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episode CognitionEpisode,
	record CognitionActionRecord,
	transition cognition.Transition,
) (taskLedgerHeader, error) {
	schema, exists := episode.ActionCatalog.Schema(record.Action.Request.Kind)
	if !exists || schema.Ref() != record.Schema {
		return header, fmt.Errorf("cognition transition action schema is not registered")
	}
	binding := cognitionstate.ActionBinding{Action: record.Action, Schema: schema}
	for _, observation := range transition.Observations {
		var err error
		header, err = persistOneCognitionObservationTx(
			ctx, tx, header, record.Origin, record.ObligationID, observation, &binding,
		)
		if err != nil {
			return header, err
		}
	}
	return header, nil
}

func persistInitialCognitionObservationsTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	authority model.StepAttemptAuthority,
	obligationID cognition.ObligationID,
	observations []cognition.Observation,
) (taskLedgerHeader, error) {
	for _, observation := range observations {
		var err error
		header, err = persistOneCognitionObservationTx(
			ctx, tx, header, authority, obligationID, observation, nil,
		)
		if err != nil {
			return header, err
		}
	}
	return header, nil
}

func persistOneCognitionObservationTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	authority model.StepAttemptAuthority,
	obligationID cognition.ObligationID,
	observation cognition.Observation,
	action *cognitionstate.ActionBinding,
) (taskLedgerHeader, error) {
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return header, err
	}
	mutation, err := cognitionstate.MapEnvironmentObservation(cognitionstate.EnvironmentObservationInput{
		Ledger: ledger.MaterializedState(), ScopeNodeID: taskstate.NodeID(obligationID),
		Observation: observation, Action: action,
	})
	if err != nil {
		return header, err
	}
	event, err := applyQueueOwnedTaskCommandTx(
		ctx, tx, authority.JobID, authority.Generation, mutation.Command(),
	)
	if err != nil {
		return header, fmt.Errorf("persist cognition observation %q: %w", observation.ID, err)
	}
	header.Version = event.Version
	set, err := loadWorkingSetSnapshotTx(ctx, tx, header, authority.Generation, true)
	if err != nil {
		return header, err
	}
	attention, err := cognitionstate.BuildObservationRetention(set, obligationID, observation)
	if err != nil {
		return header, fmt.Errorf("retain cognition observation %q: %w", observation.ID, err)
	}
	for _, workingMutation := range attention {
		if _, err := applyWorkingSetCommandTx(
			ctx, tx, authority, workingMutation.Command(), workingMutation.Descriptor(),
		); err != nil {
			return header, fmt.Errorf("retain cognition observation %q: %w", observation.ID, err)
		}
	}
	return header, nil
}
