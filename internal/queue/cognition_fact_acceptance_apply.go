package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func persistCognitionTransitionFactsTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	obligationID cognition.ObligationID,
	transition cognition.Transition,
	facts cognitionstate.FactAcceptanceAuthority,
) (taskLedgerHeader, error) {
	restored, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return header, err
	}
	preLedger := restored.MaterializedState()
	callOrdinal, err := cognitionTransitionCallOrdinalTx(ctx, tx, episodeID, transition)
	if err != nil {
		return header, err
	}
	materialization, err := newCognitionAcceptedFactMaterialization(
		episodeID, obligationID, transition, facts, preLedger, callOrdinal,
	)
	if err != nil {
		return header, fmt.Errorf("plan cognition accepted facts: %w", err)
	}
	for _, member := range materialization.Members {
		event, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, authority.JobID, authority.Generation, member.Command,
		)
		if err != nil {
			return header, fmt.Errorf("persist cognition accepted fact: %w", err)
		}
		header.Version = event.Version
		if err := insertCognitionAcceptedFactTx(ctx, tx, member.Fact); err != nil {
			return header, err
		}
	}
	if err := insertCognitionAcceptedFactMaterializationTx(
		ctx, tx, authority, materialization,
	); err != nil {
		return header, err
	}
	return header, nil
}

func cognitionTransitionCallOrdinalTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	transition cognition.Transition,
) (uint64, error) {
	if transition.ActionID == "" {
		return 0, nil
	}
	var ordinal int64
	if err := tx.QueryRow(ctx, `
		SELECT snapshots.call_ordinal
		FROM cognition_actions actions
		JOIN cognition_runtime_snapshots snapshots
		  ON snapshots.snapshot_sha256=actions.snapshot_sha256
		WHERE actions.episode_id=$1 AND actions.action_id=$2
	`, episodeID, transition.ActionID).Scan(&ordinal); err != nil || ordinal < 1 {
		return 0, fmt.Errorf("load accepted-fact transition call ordinal: %w", err)
	}
	return uint64(ordinal), nil
}

func acceptedFactCommandAuthority(
	command taskstate.AddEntryCommand,
	transition cognition.Transition,
) (cognitionstate.FactAcceptancePolicyRef, []cognition.EvidenceRef, error) {
	var metadata struct {
		Policy cognitionstate.FactAcceptancePolicyRef `json:"acceptance_policy"`
	}
	if err := json.Unmarshal(command.Metadata.Bytes(), &metadata); err != nil || metadata.Policy.Validate() != nil {
		return cognitionstate.FactAcceptancePolicyRef{}, nil,
			fmt.Errorf("%w: accepted fact policy metadata is invalid", ErrCognitionConflict)
	}
	available := make(map[taskstate.Ref]cognition.EvidenceRef, len(transition.Observations))
	for _, observation := range transition.Observations {
		ref := observation.EvidenceRef()
		available[cognitionEvidenceTaskRefs([]cognition.EvidenceRef{ref})[0]] = ref
	}
	evidence := make([]cognition.EvidenceRef, len(command.Refs))
	for index, ref := range command.Refs {
		value, exists := available[ref]
		if !exists {
			return cognitionstate.FactAcceptancePolicyRef{}, nil,
				fmt.Errorf("%w: accepted fact cites evidence outside its transition", ErrCognitionConflict)
		}
		evidence[index] = value
	}
	return metadata.Policy, evidence, nil
}
