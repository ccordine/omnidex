package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const MaxChannelSessionMessages = 200

// ChannelSessionSnapshot is one server-authoritative read of the persisted
// channel conversation and its sole active turn, when one exists.
type ChannelSessionSnapshot struct {
	Channel    model.Channel
	Transcript model.ChannelMessagePage
	ActiveJob  *model.JobDetails
}

func (r *Repository) ChannelSessionSnapshot(
	ctx context.Context,
	channelID model.ChannelID,
	messageLimit int,
) (ChannelSessionSnapshot, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ChannelSessionSnapshot{}, fmt.Errorf("channel session snapshot requires PostgreSQL and context")
	}
	if err := channelID.Validate(); err != nil {
		return ChannelSessionSnapshot{}, err
	}
	if messageLimit < 1 || messageLimit > MaxChannelSessionMessages {
		return ChannelSessionSnapshot{}, fmt.Errorf(
			"channel session message limit must be between 1 and %d",
			MaxChannelSessionMessages,
		)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ChannelSessionSnapshot{}, fmt.Errorf("begin channel %q session snapshot: %w", channelID, err)
	}
	defer tx.Rollback(context.Background())

	channel, err := scanChannel(tx.QueryRow(ctx, `
		SELECT `+channelSelectColumns+`
		FROM ai_channels
		WHERE id=$1
	`, channelID))
	if err != nil {
		return ChannelSessionSnapshot{}, err
	}
	transcript, err := listChannelMessages(ctx, tx, channelID, messageLimit, nil)
	if err != nil {
		return ChannelSessionSnapshot{}, fmt.Errorf("read channel %q session transcript: %w", channelID, err)
	}
	activeJob, err := activeChannelJobDetailsTx(ctx, tx, channel, transcript)
	if err != nil {
		return ChannelSessionSnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChannelSessionSnapshot{}, fmt.Errorf("commit channel %q session snapshot: %w", channelID, err)
	}
	return ChannelSessionSnapshot{
		Channel: channel, Transcript: transcript, ActiveJob: activeJob,
	}, nil
}

func activeChannelJobDetailsTx(
	ctx context.Context,
	tx pgx.Tx,
	channel model.Channel,
	transcript model.ChannelMessagePage,
) (*model.JobDetails, error) {
	rows, err := tx.Query(ctx, `
		SELECT id
		FROM jobs
		WHERE metadata->>'channel_id'=$1
		  AND status IN ('pending','running','waiting_input')
		ORDER BY id DESC
		LIMIT 2
	`, channel.ID)
	if err != nil {
		return nil, fmt.Errorf("read channel %q active job authority: %w", channel.ID, err)
	}
	activeIDs := make([]int64, 0, 2)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan channel %q active job authority: %w", channel.ID, err)
		}
		if id < 1 {
			rows.Close()
			return nil, fmt.Errorf("channel %q has an invalid active job identity", channel.ID)
		}
		activeIDs = append(activeIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate channel %q active job authority: %w", channel.ID, err)
	}
	rows.Close()
	if len(activeIDs) > 1 {
		return nil, fmt.Errorf(
			"channel %q has multiple active jobs %d and %d",
			channel.ID, activeIDs[0], activeIDs[1],
		)
	}
	if len(activeIDs) == 0 {
		if err := requireTranscriptActiveTurn(transcript.Messages, nil, 0); err != nil {
			return nil, fmt.Errorf("channel %q session authority: %w", channel.ID, err)
		}
		return nil, nil
	}

	details, err := currentJobDetailsTx(ctx, tx, activeIDs[0])
	if err != nil {
		return nil, fmt.Errorf("read channel %q active job %d: %w", channel.ID, activeIDs[0], err)
	}
	userMessageID, err := validateActiveChannelJob(channel, details)
	if err != nil {
		return nil, err
	}
	if err := requireTranscriptActiveTurn(transcript.Messages, &details.Job, userMessageID); err != nil {
		return nil, fmt.Errorf("channel %q active job %d: %w", channel.ID, details.Job.ID, err)
	}
	return &details, nil
}

func validateActiveChannelJob(channel model.Channel, details model.JobDetails) (int64, error) {
	job := details.Job
	if job.ID < 1 || job.Pipeline != model.PipelineChat || job.CurrentGeneration < 1 {
		return 0, fmt.Errorf("channel %q active job has invalid identity, pipeline, or generation", channel.ID)
	}
	switch job.Status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
	default:
		return 0, fmt.Errorf("channel %q active job %d has non-active status %q", channel.ID, job.ID, job.Status)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		return 0, fmt.Errorf("decode channel %q active job %d binding: %w", channel.ID, job.ID, err)
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return 0, fmt.Errorf("channel %q active job %d binding: %w", channel.ID, job.ID, err)
	}
	if err := model.ValidateChannelWorkspaceRoot(binding.ClientCWD); err != nil {
		return 0, fmt.Errorf("channel %q active job %d client_cwd: %w", channel.ID, job.ID, err)
	}
	if binding.ChannelID != channel.ID || binding.ClientCWD != channel.WorkspaceRoot ||
		binding.ChannelMode != channel.Mode || binding.DataSourceID != channel.DataSourceID ||
		binding.RoleplayViewpointCharacterID != channel.RoleplayViewpointCharacterID {
		return 0, fmt.Errorf("channel %q active job %d differs from immutable channel authority", channel.ID, job.ID)
	}
	if len(details.Steps) == 0 {
		return 0, fmt.Errorf("channel %q active job %d has no current steps", channel.ID, job.ID)
	}
	openSteps := 0
	for _, step := range details.Steps {
		if step.ID < 1 || step.JobID != job.ID || step.Generation != job.CurrentGeneration ||
			step.SupersededAtGeneration != nil {
			return 0, fmt.Errorf("channel %q active job %d has contradictory current step %d", channel.ID, job.ID, step.ID)
		}
		switch step.Status {
		case model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting:
			openSteps++
		case model.StepStatusCompleted:
		default:
			return 0, fmt.Errorf(
				"channel %q active job %d has incompatible step %d status %q",
				channel.ID, job.ID, step.ID, step.Status,
			)
		}
	}
	if openSteps == 0 {
		return 0, fmt.Errorf("channel %q active job %d has no open current step", channel.ID, job.ID)
	}
	return binding.ChannelUserMessageID, nil
}

func requireTranscriptActiveTurn(
	messages []model.ChannelMessage,
	activeJob *model.Job,
	activeUserMessageID int64,
) error {
	activeTurns := 0
	for index, message := range messages {
		if message.Turn == nil {
			continue
		}
		switch message.Turn.Status {
		case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
			activeTurns++
			if activeJob == nil || message.Turn.JobID != activeJob.ID ||
				message.Turn.Status != activeJob.Status || message.Role != model.ChannelMessageRoleUser ||
				message.ID != activeUserMessageID || message.Content != activeJob.Instruction ||
				index != len(messages)-1 {
				return fmt.Errorf("transcript active turn differs from current job authority")
			}
		}
	}
	if activeJob == nil && activeTurns != 0 {
		return fmt.Errorf("transcript exposes an active turn without an active job")
	}
	if activeJob != nil && activeTurns != 1 {
		return fmt.Errorf("transcript contains %d active turns for current job %d", activeTurns, activeJob.ID)
	}
	return nil
}
