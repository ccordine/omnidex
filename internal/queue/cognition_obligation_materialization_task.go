package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func persistCognitionMaterializationTaskDiffTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episode CognitionEpisode,
	materialization cognition.ObligationMaterialization,
	after cognition.ObligationGraphSnapshot,
) (taskLedgerHeader, error) {
	version := header.Version
	applyEvent := func(suffix string, command taskstate.Command) (taskstate.Event, error) {
		event, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, episode.Authority.JobID, episode.Authority.Generation, command,
		)
		if err != nil {
			return taskstate.Event{}, fmt.Errorf("persist cognition materialization %s: %w", suffix, err)
		}
		version = event.Version
		return event, nil
	}
	apply := func(suffix string, command taskstate.Command) error {
		_, err := applyEvent(suffix, command)
		return err
	}
	commandID := func(suffix string) (taskstate.CommandID, error) {
		return cognitionTaskCommandID(string(episode.EpisodeID), materialization.ID, suffix)
	}
	metadata, err := materializationTaskMetadata(episode, materialization)
	if err != nil {
		return header, err
	}
	spec := materialization.Spec
	addID, err := commandID("add-child")
	if err != nil {
		return header, err
	}
	stepID := episode.Authority.StepID
	if err := apply("child", taskstate.AddNodeCommand{
		CommandID: addID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID: taskstate.NodeID(spec.ID), ParentID: taskstate.NodeID(spec.ParentID),
		Kind: taskstate.NodeObjective, Title: "Cognition obligation " + string(spec.ID),
		Priority: 80, CreatedStepID: &stepID,
		AcceptanceCriteria: []string{
			"completion-check:" + string(spec.CompletionCheck.ID) + "@" + spec.CompletionCheck.Version,
		}, Metadata: metadata,
	}); err != nil {
		return header, err
	}
	decomposeID, err := commandID("decompose-child")
	if err != nil {
		return header, err
	}
	if err := apply("child decomposition", taskstate.AddEdgeCommand{
		CommandID: decomposeID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID:   cognitionEdgeID(episode.EpisodeID, spec.ParentID, spec.ID, taskstate.EdgeDecomposes),
		Kind: taskstate.EdgeDecomposes,
		From: taskstate.NodeID(spec.ParentID), To: taskstate.NodeID(spec.ID),
	}); err != nil {
		return header, err
	}
	readyID, err := commandID("ready-child")
	if err != nil {
		return header, err
	}
	readyEvent, err := applyEvent("child readiness", taskstate.PromoteReadyNodesCommand{
		CommandID: readyID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
	})
	if err != nil {
		return header, err
	}
	if len(readyEvent.NodeIDs) != 1 || readyEvent.NodeIDs[0] != taskstate.NodeID(spec.ID) {
		return header, fmt.Errorf(
			"%w: obligation materialization readied nodes other than its exact child",
			ErrCognitionConflict,
		)
	}
	dependencyID, err := commandID("parent-depends-on-child")
	if err != nil {
		return header, err
	}
	if err := apply("parent dependency", taskstate.AddEdgeCommand{
		CommandID: dependencyID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID:   cognitionEdgeID(episode.EpisodeID, spec.ParentID, spec.ID, taskstate.EdgeDependsOn),
		Kind: taskstate.EdgeDependsOn,
		From: taskstate.NodeID(spec.ParentID), To: taskstate.NodeID(spec.ID),
	}); err != nil {
		return header, err
	}
	blockID, err := commandID("block-parent")
	if err != nil {
		return header, err
	}
	if err := apply("parent block", taskstate.TransitionNodeCommand{
		CommandID: blockID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		NodeID: taskstate.NodeID(spec.ParentID), To: taskstate.NodeBlocked,
		Reason: "A code-authorized cognition prerequisite is active.",
	}); err != nil {
		return header, err
	}
	activateID, err := commandID("activate-child")
	if err != nil {
		return header, err
	}
	if err := apply("child activation", taskstate.TransitionNodeCommand{
		CommandID: activateID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		NodeID: taskstate.NodeID(spec.ID), To: taskstate.NodeActive,
	}); err != nil {
		return header, err
	}
	if err := insertCognitionObligationProjectionRecordTx(
		ctx, tx, episode.EpisodeID, episode.Authority.JobID,
		episode.Authority.Generation, after.Generation, header.ID, spec,
	); err != nil {
		return header, err
	}
	if err := insertCognitionObligationDependenciesTx(
		ctx, tx, episode.EpisodeID, spec.ParentID, []cognition.ObligationID{spec.ID},
	); err != nil {
		return header, err
	}
	if err := requireMaterializedTaskStatuses(ctx, tx, header.ID, materialization, after); err != nil {
		return header, err
	}
	header.Version = version
	return header, nil
}

func requireMaterializedTaskStatuses(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	materialization cognition.ObligationMaterialization,
	after cognition.ObligationGraphSnapshot,
) error {
	var parentStatus, childStatus taskstate.NodeStatus
	if err := tx.QueryRow(ctx, `
		SELECT parent.status,child.status FROM task_nodes parent,task_nodes child
		WHERE parent.ledger_id=$1 AND parent.id=$2 AND child.ledger_id=$1 AND child.id=$3
	`, ledgerID, materialization.ActiveObligationID, materialization.Spec.ID).
		Scan(&parentStatus, &childStatus); err != nil {
		return err
	}
	if parentStatus != taskstate.NodeBlocked || childStatus != taskstate.NodeActive {
		return fmt.Errorf("%w: Task Ledger materialization lifecycle diverged", ErrCognitionConflict)
	}
	parentFound, childFound := false, false
	for _, obligation := range after.Obligations {
		if obligation.ID == materialization.ActiveObligationID && obligation.Status != cognition.ObligationBlocked {
			return fmt.Errorf("%w: materialized parent is not blocked", ErrCognitionConflict)
		}
		if obligation.ID == materialization.ActiveObligationID {
			parentFound = true
		}
		if obligation.ID == materialization.Spec.ID && obligation.Status != cognition.ObligationActive {
			return fmt.Errorf("%w: materialized child is not active", ErrCognitionConflict)
		}
		if obligation.ID == materialization.Spec.ID {
			childFound = true
		}
	}
	if !parentFound || !childFound {
		return fmt.Errorf("%w: materialized graph omitted parent or child", ErrCognitionConflict)
	}
	return nil
}
