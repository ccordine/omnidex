package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/jackc/pgx/v5"
)

func captureLiveStaleDurableState(
	ctx context.Context,
	database *offlinePromotionDatabase,
	episode SealedEpisode,
) (LiveStaleDurableState, error) {
	if ctx == nil || database == nil {
		return LiveStaleDurableState{}, fmt.Errorf("live stale durable-state authority is incomplete")
	}
	if err := episode.Validate(); err != nil {
		return LiveStaleDurableState{}, err
	}
	trace, err := readProductionTrace(ctx, database.repository, episode.Manifest.EpisodeID)
	if err != nil {
		return LiveStaleDurableState{}, err
	}
	ref := cognition.EpisodeRef{ID: episode.Manifest.EpisodeID}
	hostStore, err := labyrinthhost.NewStoreInSchema(database.hostAdminPool, database.hostSchema)
	if err != nil {
		return LiveStaleDurableState{}, err
	}
	host, err := hostStore.Episode(ctx, ref)
	if err != nil {
		return LiveStaleDurableState{}, err
	}
	var hostReceipts int
	query := `SELECT COUNT(*) FROM ` +
		pgx.Identifier{database.hostSchema, "action_receipts"}.Sanitize() +
		` WHERE episode_id=$1`
	if err := database.hostAdminPool.QueryRow(ctx, query, ref.ID).Scan(&hostReceipts); err != nil {
		return LiveStaleDurableState{}, fmt.Errorf("count durable Labyrinth receipts: %w", err)
	}
	state := LiveStaleDurableState{
		Episode: ref, TraceSHA256: trace.Header.TraceSHA256, SealSHA256: episode.SealSHA256,
		GraphVersion: trace.Header.GraphVersion, LedgerVersion: trace.Header.LedgerVersion,
		WorkingSetVersion: trace.Header.WorkingSetVersion, HostReceipts: hostReceipts,
		HostCurrent: host.Current, HostTerminal: host.Terminal,
	}
	for _, record := range trace.Records {
		switch record.Kind {
		case "policy_result":
			state.PolicyResults++
		case "policy_abandonment":
			state.PolicyAbandonments++
		case "reconciliation_receipt":
			state.ReconciliationReceipts++
		case "action":
			state.ActionRecords++
		case "working_set_event":
			state.WorkingSetEvents++
		case "episode_progress":
			state.ProgressRecords++
		}
	}
	if host.Current != episode.Manifest.FinalRevision || host.Terminal != episode.Manifest.Outcome.Terminal {
		return LiveStaleDurableState{}, fmt.Errorf("live stale host state differs from the sealed episode")
	}
	return state, state.Validate()
}
