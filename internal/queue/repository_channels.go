package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const channelSelectColumns = `
	id, scope, name, tags, project_id, workspace_root, data_source_id,
	mode, roleplay_viewpoint_character_id, created_at, updated_at
`

type channelRowScanner interface {
	Scan(dest ...any) error
}

func scanChannel(row channelRowScanner) (model.Channel, error) {
	var channel model.Channel
	var dataSourceID *string
	var roleplayViewpointID *string
	err := row.Scan(
		&channel.ID, &channel.Scope, &channel.Name, &channel.Tags,
		&channel.ProjectID, &channel.WorkspaceRoot, &dataSourceID,
		&channel.Mode, &roleplayViewpointID,
		&channel.CreatedAt, &channel.UpdatedAt,
	)
	if err != nil {
		return model.Channel{}, err
	}
	if dataSourceID != nil {
		channel.DataSourceID = model.DataSourceID(*dataSourceID)
	}
	if roleplayViewpointID != nil {
		channel.RoleplayViewpointCharacterID = model.RoleplayCharacterID(*roleplayViewpointID)
	}
	if err := channel.ValidateStored(); err != nil {
		return model.Channel{}, fmt.Errorf("invalid stored channel %q: %w", channel.ID, err)
	}
	return channel, nil
}

func (r *Repository) CreateChannel(ctx context.Context, channel model.Channel) (model.Channel, error) {
	if channel.Mode != model.ChannelModeAssistant {
		return model.Channel{}, fmt.Errorf("assistant channel creation requires exact assistant mode")
	}
	return r.createChannel(ctx, channel, nil)
}

type roleplayChannelBootstrap struct {
	worldID, worldName         string
	viewpointID, viewpointName string
}

func (r *Repository) CreateRoleplayChannel(
	ctx context.Context,
	channel model.Channel,
	worldName, viewpointName string,
) (model.Channel, error) {
	if channel.Mode != model.ChannelModeRoleplay {
		return model.Channel{}, fmt.Errorf("roleplay channel creation requires exact roleplay mode")
	}
	if channel.RoleplayViewpointCharacterID != "" {
		return model.Channel{}, fmt.Errorf("roleplay viewpoint identity is server-resolved and must be omitted on create")
	}
	worldID, err := roleplay.NewWorldIdentity()
	if err != nil {
		return model.Channel{}, err
	}
	viewpointID, err := roleplay.NewCharacterIdentity()
	if err != nil {
		return model.Channel{}, err
	}
	channel.RoleplayViewpointCharacterID = model.RoleplayCharacterID(viewpointID)
	return r.createChannel(ctx, channel, &roleplayChannelBootstrap{
		worldID: worldID, worldName: worldName,
		viewpointID: viewpointID, viewpointName: viewpointName,
	})
}

func (r *Repository) createChannel(
	ctx context.Context,
	channel model.Channel,
	roleplayBootstrap *roleplayChannelBootstrap,
) (model.Channel, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return model.Channel{}, fmt.Errorf("create channel requires PostgreSQL and context")
	}
	if err := channel.ValidateForCreate(); err != nil {
		return model.Channel{}, err
	}
	channel.Tags = append([]string(nil), channel.Tags...)
	if channel.Tags == nil {
		channel.Tags = []string{}
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Channel{}, err
	}
	defer tx.Rollback(ctx)
	metadata, err := json.Marshal(map[string]string{"client_cwd": channel.WorkspaceRoot})
	if err != nil {
		return model.Channel{}, err
	}
	projectID, err := resolveProjectID(ctx, tx, metadata)
	if err != nil {
		return model.Channel{}, fmt.Errorf("resolve channel project binding: %w", err)
	}
	if projectID == nil || *projectID < 1 {
		return model.Channel{}, fmt.Errorf("channel project binding was not resolved")
	}
	out, err := scanChannel(tx.QueryRow(ctx, `
		INSERT INTO ai_channels (
			id, scope, name, tags, project_id, workspace_root, data_source_id,
			mode, roleplay_viewpoint_character_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+channelSelectColumns,
		channel.ID, channel.Scope, channel.Name, channel.Tags, *projectID, channel.WorkspaceRoot,
		nullableDataSourceID(channel.DataSourceID), channel.Mode,
		nullableRoleplayCharacterID(channel.RoleplayViewpointCharacterID),
	))
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return model.Channel{}, fmt.Errorf("%w: %s", ErrChannelAlreadyExists, channel.ID)
		}
		return model.Channel{}, err
	}
	if roleplayBootstrap != nil {
		if channel.Mode != model.ChannelModeRoleplay ||
			string(channel.RoleplayViewpointCharacterID) != roleplayBootstrap.viewpointID {
			return model.Channel{}, fmt.Errorf("roleplay channel bootstrap authority does not match channel binding")
		}
		if _, _, err := roleplay.BootstrapWorldTx(
			ctx, tx, string(channel.ID), roleplayBootstrap.worldID, roleplayBootstrap.worldName,
			roleplayBootstrap.viewpointID, roleplayBootstrap.viewpointName,
		); err != nil {
			return model.Channel{}, fmt.Errorf("bootstrap roleplay channel authority: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Channel{}, err
	}
	return out, nil
}

func nullableRoleplayCharacterID(id model.RoleplayCharacterID) any {
	if id == "" {
		return nil
	}
	return string(id)
}

func nullableDataSourceID(id model.DataSourceID) any {
	if id == "" {
		return nil
	}
	return string(id)
}

func (r *Repository) GetChannel(ctx context.Context, id model.ChannelID) (model.Channel, error) {
	if err := id.Validate(); err != nil {
		return model.Channel{}, err
	}
	return scanChannel(r.pool.QueryRow(ctx, `
		SELECT `+channelSelectColumns+`
		FROM ai_channels WHERE id = $1
	`, id))
}

func (r *Repository) ListChannels(ctx context.Context, scope model.ChannelScope, limit, offset int) ([]model.Channel, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		return nil, fmt.Errorf("channel list limit must be between 1 and 200")
	}
	if offset < 0 {
		return nil, fmt.Errorf("channel list offset must be nonnegative")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+channelSelectColumns+`
		FROM ai_channels WHERE scope = $1
		ORDER BY updated_at DESC, id ASC LIMIT $2 OFFSET $3
	`, scope, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []model.Channel{}
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (r *Repository) ListChannelMessages(ctx context.Context, channelID model.ChannelID, limit int, beforeID *int64) (model.ChannelMessagePage, error) {
	if err := channelID.Validate(); err != nil {
		return model.ChannelMessagePage{}, err
	}
	if limit <= 0 || limit > 200 {
		return model.ChannelMessagePage{}, fmt.Errorf("channel message limit must be between 1 and 200")
	}
	if beforeID != nil && *beforeID < 1 {
		return model.ChannelMessagePage{}, fmt.Errorf("channel message cursor must be positive")
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_channels WHERE id=$1)`, channelID).Scan(&exists); err != nil {
		return model.ChannelMessagePage{}, err
	}
	if !exists {
		return model.ChannelMessagePage{}, pgx.ErrNoRows
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, channel_id, role, content, created_at
		FROM ai_channel_messages
		WHERE channel_id=$1 AND ($3::bigint IS NULL OR id<$3)
		ORDER BY id DESC LIMIT $2
	`, channelID, limit+1, beforeID)
	if err != nil {
		return model.ChannelMessagePage{}, err
	}
	defer rows.Close()
	messages := []model.ChannelMessage{}
	for rows.Next() {
		var message model.ChannelMessage
		if err := rows.Scan(&message.ID, &message.ChannelID, &message.Role, &message.Content, &message.CreatedAt); err != nil {
			return model.ChannelMessagePage{}, err
		}
		if err := model.ValidateChannelMessage(message.Role, message.Content); err != nil {
			return model.ChannelMessagePage{}, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return model.ChannelMessagePage{}, err
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	var nextBeforeID *int64
	if hasMore && len(messages) > 0 {
		value := messages[0].ID
		nextBeforeID = &value
	}
	return model.ChannelMessagePage{Messages: messages, NextBeforeID: nextBeforeID, HasMore: hasMore}, nil
}
