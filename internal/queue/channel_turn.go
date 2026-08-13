package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/jackc/pgx/v5"
)

type channelTurnMetadata struct {
	ChannelID            model.ChannelID    `json:"channel_id"`
	SessionID            string             `json:"session_id"`
	ChannelUserMessageID int64              `json:"channel_user_message_id"`
	ProjectID            int64              `json:"project_id"`
	ClientCWD            string             `json:"client_cwd"`
	ModelConfig          modelconfig.Config `json:"model_config"`
}

// EnqueueChannelTurn atomically records the exact user message and creates the
// single authoritative chat job that will answer it.
func (r *Repository) EnqueueChannelTurn(
	ctx context.Context,
	channelID model.ChannelID,
	instruction string,
) (model.ChannelMessage, model.Job, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel turn requires PostgreSQL and context")
	}
	if err := channelID.Validate(); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, instruction); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	defer tx.Rollback(ctx)
	var scope model.ChannelScope
	var projectID int64
	var workspaceRoot string
	var projectLocation string
	var projectSettings json.RawMessage
	if err := tx.QueryRow(ctx, `
		SELECT channel.scope, channel.project_id, channel.workspace_root,
		       project.location, project.settings
		FROM ai_channels AS channel
		JOIN projects AS project ON project.id=channel.project_id
		WHERE channel.id=$1
		FOR UPDATE OF channel, project
	`, channelID).Scan(
		&scope, &projectID, &workspaceRoot, &projectLocation, &projectSettings,
	); err == pgx.ErrNoRows {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q does not exist", channelID)
	} else if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if scope != model.ChannelScopeUser {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q is not a user conversation", channelID)
	}
	if projectID < 1 {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q has no authoritative project binding", channelID)
	}
	if err := model.ValidateChannelWorkspaceRoot(workspaceRoot); err != nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q workspace binding: %w", channelID, err)
	}
	if projectLocation != workspaceRoot {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf(
			"channel %q project location %q differs from workspace binding %q",
			channelID, projectLocation, workspaceRoot,
		)
	}
	modelSnapshot, err := modelconfig.FromSettingsJSON(projectSettings)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf(
			"channel %q project model config: %w", channelID, err,
		)
	}
	var activeJobID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM jobs
		WHERE metadata->>'channel_id'=$1
		  AND status IN ('pending','running','waiting_input')
		ORDER BY id DESC LIMIT 1
	`, channelID).Scan(&activeJobID)
	if err == nil {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("%w: job %d", ErrChannelTurnActive, activeJobID)
	}
	if err != pgx.ErrNoRows {
		return model.ChannelMessage{}, model.Job{}, err
	}

	var message model.ChannelMessage
	err = tx.QueryRow(ctx, `
		INSERT INTO ai_channel_messages (channel_id, role, content)
		SELECT id, 'user', $2 FROM ai_channels WHERE id = $1
		RETURNING id, channel_id, role, content, created_at
	`, channelID, instruction).Scan(
		&message.ID, &message.ChannelID, &message.Role, &message.Content, &message.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return model.ChannelMessage{}, model.Job{}, fmt.Errorf("channel %q does not exist", channelID)
	}
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	metadata, err := marshalChannelTurnMetadata(
		channelID, message.ID, projectID, workspaceRoot, modelSnapshot,
	)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	job, err := r.enqueueChannelJobTx(ctx, tx, instruction, metadata)
	if err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_channels SET updated_at = NOW() WHERE id = $1`, channelID); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ChannelMessage{}, model.Job{}, err
	}
	return message, job, nil
}

func (r *Repository) enqueueChannelJobTx(
	ctx context.Context,
	tx pgx.Tx,
	instruction string,
	metadata []byte,
) (model.Job, error) {
	if len(metadata) == 0 {
		return model.Job{}, fmt.Errorf("channel job metadata is required")
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(metadata, &binding); err != nil {
		return model.Job{}, fmt.Errorf("decode channel job metadata: %w", err)
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return model.Job{}, err
	}
	return r.enqueueJobWithStepsTx(
		ctx, tx, instruction, model.PipelineChat, metadata, conversationObjectiveSteps(),
	)
}

func marshalChannelTurnMetadata(
	channelID model.ChannelID,
	messageID int64,
	projectID int64,
	workspaceRoot string,
	modelSnapshot modelconfig.Config,
) ([]byte, error) {
	binding := channelTurnMetadata{
		ChannelID: channelID, SessionID: "channel:" + string(channelID),
		ChannelUserMessageID: messageID, ProjectID: projectID, ClientCWD: workspaceRoot,
		ModelConfig: modelSnapshot,
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return nil, err
	}
	return json.Marshal(binding)
}

func validateChannelTurnMetadata(binding channelTurnMetadata) error {
	if err := binding.ChannelID.Validate(); err != nil {
		return err
	}
	if binding.ChannelUserMessageID < 1 || binding.ProjectID < 1 ||
		binding.SessionID != "channel:"+string(binding.ChannelID) {
		return fmt.Errorf("channel job metadata requires exact channel, message, and project identities")
	}
	if err := model.ValidateChannelWorkspaceRoot(binding.ClientCWD); err != nil {
		return fmt.Errorf("channel job metadata workspace binding: %w", err)
	}
	raw, err := json.Marshal(binding.ModelConfig)
	if err != nil {
		return fmt.Errorf("encode channel model snapshot: %w", err)
	}
	validated, err := modelconfig.FromJSON(raw)
	if err != nil {
		return fmt.Errorf("channel model snapshot: %w", err)
	}
	if !maps.Equal(binding.ModelConfig, validated) {
		return fmt.Errorf("channel model snapshot is not exact")
	}
	return nil
}
