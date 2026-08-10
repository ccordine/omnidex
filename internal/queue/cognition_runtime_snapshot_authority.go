package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func loadCognitionSnapshotAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	command CognitionRuntimeSnapshotCommand,
) (taskLedgerHeader, CognitionEpisode, CognitionObligationGraphRecord, error) {
	header, err := loadTaskLedgerHeaderTx(ctx, tx, command.Authority.JobID, true)
	if err != nil {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, err
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, command.EpisodeID, true)
	if err != nil {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, err
	}
	if !found {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, fmt.Errorf(
			"%w: %s", ErrCognitionEpisodeNotFound, command.EpisodeID,
		)
	}
	if err := cognitionAuthorityMatches(command.Authority, episode); err != nil {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, err
	}
	if episode.Status != CognitionEpisodeActive {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, ErrCognitionTerminal
	}
	if err := requireNoUnresolvedCognitionActionTx(ctx, tx, episode.EpisodeID); err != nil {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, err
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, true)
	if err != nil {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, err
	}
	if !found {
		return taskLedgerHeader{}, CognitionEpisode{}, CognitionObligationGraphRecord{}, fmt.Errorf(
			"%w: cognition episode has no current generation graph", ErrCognitionConflict,
		)
	}
	return header, episode, graph, nil
}

func loadCognitionEnvironmentStateTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
) (bool, string, error) {
	var terminal bool
	var outcome string
	if err := tx.QueryRow(ctx, `
		SELECT terminal,public_outcome FROM cognition_transitions
		WHERE episode_id=$1 AND revision=$2 AND current_revision_sha256=$3
	`, episode.EpisodeID, int64(episode.CurrentRevision.Number), episode.CurrentRevision.SHA256).
		Scan(&terminal, &outcome); err != nil {
		return false, "", fmt.Errorf("load cognition environment state: %w", err)
	}
	if !terminal && outcome != "" {
		// A nonterminal transition may expose a bounded public outcome, but the
		// runtime only carries terminal outcomes into completion authority.
		outcome = ""
	}
	return terminal, outcome, nil
}

func cognitionEvidenceTaskRef(ref cognition.EvidenceRef) string {
	return "cognition:episode/" + string(ref.Revision.EpisodeID) + "/observation/" + string(ref.ObservationID)
}
