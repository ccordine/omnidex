package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func waitForPausedInference(
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	episodeID cognition.EpisodeID,
	authority model.StepAttemptAuthority,
	boundary inferenceBoundary,
	checkpointPath string,
	child *offlineChildProcess,
) (PausedInferenceCheckpoint, error) {
	if ctx == nil || pool == nil || repository == nil || boundary.Validate() != nil ||
		child == nil || child.pid() == 0 {
		return PausedInferenceCheckpoint{}, fmt.Errorf("paused inference wait authority is incomplete")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return PausedInferenceCheckpoint{}, ctx.Err()
		case <-ticker.C:
			if err := child.signal(syscall.Signal(0)); err != nil {
				return PausedInferenceCheckpoint{}, fmt.Errorf("inference child exited before its boundary: %w", err)
			}
			episode, err := repository.CognitionEpisode(ctx, episodeID)
			if err != nil {
				return PausedInferenceCheckpoint{}, err
			}
			count, err := durableInferenceBoundaryCount(ctx, pool, episode, boundary.Kind)
			if err != nil {
				return PausedInferenceCheckpoint{}, err
			}
			if count > int64(boundary.Count) {
				return PausedInferenceCheckpoint{}, fmt.Errorf("inference child crossed its public durable boundary")
			}
			if count != int64(boundary.Count) {
				continue
			}
			if _, err := os.Stat(checkpointPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return PausedInferenceCheckpoint{}, err
			}
			checkpoint, err := LoadPausedInferenceCheckpoint(checkpointPath)
			if err != nil {
				return PausedInferenceCheckpoint{}, err
			}
			if checkpoint.Boundary != boundary ||
				checkpoint.PreCall.Bound.Attempt != bindingAttemptRef(authority) {
				return PausedInferenceCheckpoint{}, fmt.Errorf("paused inference checkpoint changed its durable authority")
			}
			if err := requireDurablePausedProjection(ctx, repository, authority, checkpoint); err != nil {
				return PausedInferenceCheckpoint{}, err
			}
			return checkpoint, nil
		}
	}
}

func durableInferenceBoundaryCount(
	ctx context.Context,
	pool *pgxpool.Pool,
	episode queue.CognitionEpisode,
	kind inferenceBoundaryKind,
) (int64, error) {
	switch kind {
	case inferenceBoundaryActions:
		return episode.SuccessfulActions, nil
	case inferenceBoundaryDecisions:
		var count int64
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM cognition_policy_calls
			WHERE episode_id=$1 AND status='accepted'
		`, episode.EpisodeID).Scan(&count); err != nil {
			return 0, fmt.Errorf("count durable cognition policy decisions: %w", err)
		}
		return count, nil
	default:
		return 0, fmt.Errorf("durable inference boundary kind is not registered")
	}
}

func requireDurablePausedProjection(
	ctx context.Context,
	repository *queue.Repository,
	authority model.StepAttemptAuthority,
	checkpoint PausedInferenceCheckpoint,
) error {
	var cursor int64
	for {
		page, err := repository.ListContextProjectionSummaries(
			ctx, authority.JobID, authority.Generation, cursor, 100,
		)
		if err != nil {
			return err
		}
		for _, summary := range page {
			if summary.ProjectionID != string(checkpoint.PreCall.Bound.Projection.ID) {
				continue
			}
			if summary.Authority.StepAttemptAuthority != authority ||
				summary.RenderedSHA256 != checkpoint.PreCall.ProjectionRenderedSHA256 {
				return fmt.Errorf("paused projection changed its durable attempt or rendered content")
			}
			return nil
		}
		if len(page) == 0 {
			return fmt.Errorf("paused inference checkpoint lacks its public durable projection")
		}
		cursor = page[len(page)-1].RecordID
	}
}

func bindingAttemptRef(authority model.StepAttemptAuthority) cognition.AttemptRef {
	return cognition.AttemptRef{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID,
	}
}
