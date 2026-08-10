package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

type durableRuntimeCheckpoint struct {
	EpisodeID         cognition.EpisodeID          `json:"episode_id"`
	Revision          cognition.WorldRevision      `json:"revision"`
	EpisodeStatus     queue.CognitionEpisodeStatus `json:"episode_status"`
	EpisodeVersion    uint64                       `json:"episode_version"`
	SuccessfulActions int64                        `json:"successful_actions"`
	TotalCost         int64                        `json:"total_cost"`
	ActionCatalogSHA  string                       `json:"action_catalog_sha256"`
	GraphVersion      uint64                       `json:"graph_version"`
	GraphSHA256       string                       `json:"graph_sha256"`
	GraphGeneration   uint64                       `json:"graph_generation"`
	LedgerSHA256      string                       `json:"ledger_sha256"`
	WorkingSetID      string                       `json:"working_set_id"`
	WorkingSetVersion uint64                       `json:"working_set_version"`
	WorkingSetStatus  string                       `json:"working_set_status"`
	WorkingSetItems   int                          `json:"working_set_items"`
	WorkingSetSHA256  string                       `json:"working_set_sha256"`
}

func captureRuntimeCheckpoint(
	ctx context.Context,
	repository *queue.Repository,
	episode cognition.EpisodeID,
) (string, error) {
	current, err := repository.CognitionEpisode(ctx, episode)
	if err != nil {
		return "", err
	}
	graph, err := repository.CognitionObligationGraph(ctx, episode)
	if err != nil {
		return "", err
	}
	ledger, err := repository.TaskLedger(ctx, current.Authority.JobID)
	if err != nil {
		return "", err
	}
	working, err := repository.CurrentWorkingSet(ctx, current.Authority.JobID)
	if err != nil {
		return "", err
	}
	catalogSHA, err := digestJSON(current.ActionCatalog)
	if err != nil {
		return "", fmt.Errorf("hash durable cognition action catalog: %w", err)
	}
	ledgerSHA, err := digestJSON(ledger)
	if err != nil {
		return "", fmt.Errorf("hash durable cognition Task Ledger: %w", err)
	}
	workingSHA, err := digestJSON(working)
	if err != nil {
		return "", fmt.Errorf("hash durable cognition Working Set: %w", err)
	}
	checkpoint := durableRuntimeCheckpoint{
		EpisodeID: episode, Revision: current.CurrentRevision, EpisodeStatus: current.Status,
		EpisodeVersion: current.Version, SuccessfulActions: current.SuccessfulActions,
		TotalCost: current.TotalCost, ActionCatalogSHA: catalogSHA, GraphVersion: graph.Version,
		GraphSHA256: graph.Graph.SHA256, GraphGeneration: graph.Graph.Generation,
		LedgerSHA256: ledgerSHA,
		WorkingSetID: string(working.ID), WorkingSetVersion: working.Version,
		WorkingSetStatus: string(working.Status), WorkingSetItems: len(working.Items),
		WorkingSetSHA256: workingSHA,
	}
	digest, err := digestJSON(checkpoint)
	if err != nil {
		return "", fmt.Errorf("hash durable cognition restart checkpoint: %w", err)
	}
	return digest, nil
}

func validateRestartSchedule(schedule []uint32, maximum int) error {
	for _, cycle := range schedule {
		if int(cycle) >= maximum {
			return fmt.Errorf("restart cycle %d is outside the frozen runtime-cycle budget", cycle)
		}
	}
	return nil
}
