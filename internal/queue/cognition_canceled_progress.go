package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func loadCanceledCognitionProgressTx(
	ctx context.Context,
	tx pgx.Tx,
	episode CognitionEpisode,
) (cognitionruntime.EpisodeProgress, error) {
	if episode.Status != CognitionEpisodeCanceled || episode.TerminalOutcome == "" {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: episode is not exactly canceled", ErrCognitionConflict)
	}
	seal, err := loadCognitionTerminalSealTx(ctx, tx, episode.EpisodeID)
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	if seal.Outcome != CognitionEpisodeCanceled || seal.FinalRevision != episode.CurrentRevision {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: canceled seal differs from episode", ErrCognitionConflict)
	}
	graph, found, err := loadCurrentCognitionObligationGraphTx(ctx, tx, episode.EpisodeID, false)
	if err != nil || !found || graph.Graph.SHA256 != seal.ObligationGraphSHA256 {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: canceled graph differs from seal: %v", ErrCognitionConflict, err)
	}
	var completionRaw []byte
	var code cognitionruntime.CancellationCode
	var evidenceID string
	if err := tx.QueryRow(ctx, `
		SELECT seals.completion_json,cancellations.cancellation_code,cancellations.source_evidence_id
		FROM cognition_terminal_seals seals
		JOIN cognition_episode_cancellations cancellations ON cancellations.episode_id=seals.episode_id
		WHERE seals.episode_id=$1
	`, episode.EpisodeID).Scan(&completionRaw, &code, &evidenceID); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	var completion cognition.CompletionResult
	if err := json.Unmarshal(completionRaw, &completion); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	_, completionSHA, err := cognitionJSON(completion)
	if err != nil || completionSHA != seal.CompletionSHA256 {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("%w: canceled completion changed", ErrCognitionConflict)
	}
	cancellation := cognitionruntime.CancellationSeal{
		Episode: cognition.EpisodeRef{ID: episode.EpisodeID}, Code: code,
		SourceEvidenceID: evidenceID, TraceSHA256: seal.TraceSHA256,
	}
	if err := cancellation.Validate(); err != nil {
		return cognitionruntime.EpisodeProgress{}, err
	}
	progress := cognitionruntime.EpisodeProgress{
		Episode: cognition.EpisodeRef{ID: episode.EpisodeID}, State: cognitionruntime.ProgressCanceled,
		Revision: episode.CurrentRevision, GraphVersion: graph.Version,
		ObligationGraph: graph.Graph.Clone(), Completion: &completion,
		Cancellation: &cancellation, PublicOutcome: episode.TerminalOutcome,
	}
	return progress, nil
}
