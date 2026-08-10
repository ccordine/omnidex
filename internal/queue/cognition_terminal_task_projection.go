package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func requireCognitionGraphTaskProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	graph cognition.ObligationGraphSnapshot,
) error {
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return err
	}
	nodes := make(map[taskstate.NodeID]taskstate.Node, len(graph.Obligations))
	for _, node := range ledger.Nodes() {
		nodes[node.ID] = node
	}
	for _, obligation := range graph.Obligations {
		if obligation.CreatedGeneration != graph.Generation || obligation.Status == cognition.ObligationSuperseded {
			continue
		}
		node, found := nodes[taskstate.NodeID(obligation.ID)]
		if !found {
			return fmt.Errorf("%w: obligation %q has no Task Ledger node", ErrCognitionConflict, obligation.ID)
		}
		want, err := cognitionTaskNodeStatus(obligation.Status)
		if err != nil {
			return err
		}
		if node.Status != want {
			return fmt.Errorf(
				"%w: obligation %q status %q disagrees with Task Ledger %q",
				ErrCognitionConflict, obligation.ID, obligation.Status, node.Status,
			)
		}
	}
	return nil
}

func cognitionTaskNodeStatus(status cognition.ObligationStatus) (taskstate.NodeStatus, error) {
	switch status {
	case cognition.ObligationProposed:
		return taskstate.NodePending, nil
	case cognition.ObligationReady:
		return taskstate.NodeReady, nil
	case cognition.ObligationBlocked:
		return taskstate.NodeBlocked, nil
	case cognition.ObligationActive:
		return taskstate.NodeActive, nil
	case cognition.ObligationSatisfied:
		return taskstate.NodeDone, nil
	case cognition.ObligationFailed:
		return taskstate.NodeFailed, nil
	default:
		return "", fmt.Errorf("unregistered current cognition obligation status %q", status)
	}
}

func cancelCognitionObligationNodesTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	episodeID cognition.EpisodeID,
	graph cognition.ObligationGraphSnapshot,
	jobID, jobGeneration int64,
	commandAuthority string,
) (taskLedgerHeader, error) {
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return header, err
	}
	for _, obligation := range graph.Obligations {
		if obligation.CreatedGeneration != graph.Generation ||
			obligation.Status == cognition.ObligationSuperseded {
			continue
		}
		node, found := ledger.Node(taskstate.NodeID(obligation.ID))
		if !found {
			return header, fmt.Errorf("%w: cancellation obligation %q has no Task Ledger node", ErrCognitionConflict, obligation.ID)
		}
		if node.Status == taskstate.NodeDone || node.Status == taskstate.NodeFailed || node.Status == taskstate.NodeCanceled {
			continue
		}
		commandID, err := cognitionTaskCommandID(
			string(episodeID), "terminal-cancel", commandAuthority, string(obligation.ID), graph.SHA256,
		)
		if err != nil {
			return header, err
		}
		event, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, jobID, jobGeneration,
			taskstate.TransitionNodeCommand{
				CommandID: commandID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
				NodeID: node.ID, To: taskstate.NodeCanceled,
				Reason: "The cognition episode was canceled by code authority.",
			},
		)
		if err != nil {
			return header, err
		}
		header.Version = event.Version
		ledger, err = restoreTaskLedgerTx(ctx, tx, header)
		if err != nil {
			return header, err
		}
	}
	return header, nil
}
