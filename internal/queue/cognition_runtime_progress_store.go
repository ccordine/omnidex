package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const cognitionRuntimeProgressSchemaV1 = "omnidex.cognition-runtime-progress.v1"

func describeCognitionRuntimeProgress(
	kind CognitionObligationCommandKind,
	command cognitionruntime.CompletionCommand,
) (cognitionObligationDescriptor, error) {
	if kind != CognitionObligationSatisfy && kind != CognitionObligationFail {
		return cognitionObligationDescriptor{}, fmt.Errorf("unregistered cognition progress kind %q", kind)
	}
	raw, _, err := cognitionJSON(struct {
		Schema  string                             `json:"schema"`
		Kind    CognitionObligationCommandKind     `json:"kind"`
		Command cognitionruntime.CompletionCommand `json:"command"`
	}{cognitionRuntimeProgressSchemaV1, kind, command})
	if err != nil {
		return cognitionObligationDescriptor{}, err
	}
	digest := sha256.Sum256(raw)
	value := hex.EncodeToString(digest[:])
	return cognitionObligationDescriptor{
		ID: "cognition_graph_command_" + value, SHA256: value, Kind: kind, Raw: raw,
	}, nil
}

func loadCognitionRuntimeProgressReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor cognitionObligationDescriptor,
	command cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, bool, error) {
	var commandJSON, progressJSON []byte
	var commandSHA string
	err := tx.QueryRow(ctx, `
		SELECT command_json,command_sha256,progress_json
		FROM cognition_episode_progress WHERE command_id=$1
	`, descriptor.ID).Scan(&commandJSON, &commandSHA, &progressJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return cognitionruntime.EpisodeProgress{}, false, nil
	}
	if err != nil {
		return cognitionruntime.EpisodeProgress{}, false, err
	}
	var persisted cognitionruntime.CompletionCommand
	var progress cognitionruntime.EpisodeProgress
	if err := json.Unmarshal(commandJSON, &persisted); err != nil {
		return progress, false, err
	}
	if err := json.Unmarshal(progressJSON, &progress); err != nil {
		return progress, false, err
	}
	_, expectedCommandSHA, err := cognitionJSON(command)
	if err != nil || commandSHA != expectedCommandSHA || !reflect.DeepEqual(persisted, command) {
		return progress, false, fmt.Errorf("%w: cognition progress replay changed content", ErrCognitionConflict)
	}
	return cloneCognitionEpisodeProgress(progress), true, nil
}

func insertCognitionRuntimeProgressTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	descriptor cognitionObligationDescriptor,
	command cognitionruntime.CompletionCommand,
	progress cognitionruntime.EpisodeProgress,
) error {
	commandJSON, commandSHA, err := cognitionJSON(command)
	if err != nil {
		return err
	}
	progressJSON, progressSHA, err := cognitionJSON(progress)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_episode_progress (
			command_id,episode_id,source_snapshot_sha256,input_graph_version,
			output_graph_version,state,command_json,command_sha256,progress_json,
			progress_sha256,job_id,generation,step_id,actor_attempt,actor_worker_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, descriptor.ID, command.Binding.Episode.ID, command.SnapshotSHA256,
		int64(command.GraphVersion), int64(progress.GraphVersion), progress.State,
		string(commandJSON), commandSHA, string(progressJSON), progressSHA,
		authority.JobID, authority.Generation, authority.StepID, authority.Attempt, authority.WorkerID)
	if err != nil {
		return fmt.Errorf("insert cognition episode progress: %w", err)
	}
	return nil
}

func loadLatestTerminalCognitionProgressTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
) (*cognitionruntime.EpisodeProgress, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT progress_json FROM cognition_episode_progress
		WHERE episode_id=$1 AND state IN ('completed','failed')
		ORDER BY output_graph_version DESC LIMIT 1
	`, episodeID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var progress cognitionruntime.EpisodeProgress
	if err := json.Unmarshal(raw, &progress); err != nil {
		return nil, fmt.Errorf("decode terminal cognition progress: %w", err)
	}
	copy := cloneCognitionEpisodeProgress(progress)
	return &copy, nil
}

func cloneCognitionEpisodeProgress(progress cognitionruntime.EpisodeProgress) cognitionruntime.EpisodeProgress {
	progress.ObligationGraph = progress.ObligationGraph.Clone()
	if progress.Completion != nil {
		completion := progress.Completion.Clone()
		progress.Completion = &completion
	}
	if progress.Cancellation != nil {
		cancellation := *progress.Cancellation
		progress.Cancellation = &cancellation
	}
	return progress
}
