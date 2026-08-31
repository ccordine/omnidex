package queue

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/jackc/pgx/v5"
)

const cliChatSessionIdentitySchema = "omnidex.cli-chat-channel.v2"

var cliChatSessionTags = []string{"chat", "cli"}

// EnsureCLIChatSessionChannel atomically selects the one server-owned channel
// identity for an exact client directory and physical directory identity. It
// creates no messages, jobs, or workflow state.
func (r *Repository) EnsureCLIChatSessionChannel(
	ctx context.Context,
	workspaceRoot string,
	workspaceIdentity string,
) (model.Channel, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return model.Channel{}, fmt.Errorf("ensure CLI chat session requires PostgreSQL and context")
	}
	if err := model.ValidateChannelWorkspaceRoot(workspaceRoot); err != nil {
		return model.Channel{}, err
	}
	if err := projectroot.ValidateDirectoryIdentity(workspaceIdentity); err != nil {
		return model.Channel{}, fmt.Errorf("CLI workspace identity: %w", err)
	}
	expected := cliChatSessionChannel(workspaceRoot, workspaceIdentity)
	if err := expected.ValidateForCreate(); err != nil {
		return model.Channel{}, fmt.Errorf("derive CLI chat session channel: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Channel{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_channels (
			id, scope, name, tags, workspace_root, data_source_id,
			mode, roleplay_viewpoint_character_id
		)
		VALUES ($1, $2, $3, $4, $5, NULL, $6, NULL)
		ON CONFLICT (id) DO NOTHING
	`, expected.ID, expected.Scope, expected.Name, expected.Tags,
		expected.WorkspaceRoot, expected.Mode); err != nil {
		return model.Channel{}, err
	}
	channel, err := scanChannel(tx.QueryRow(ctx, `
		SELECT `+channelSelectColumns+`
		FROM ai_channels
		WHERE id=$1
		FOR SHARE
	`, expected.ID))
	if err != nil {
		return model.Channel{}, err
	}
	if err := requireExactCLIChatSessionChannel(expected, channel); err != nil {
		return model.Channel{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Channel{}, err
	}
	return channel, nil
}

func cliChatSessionChannel(workspaceRoot, workspaceIdentity string) model.Channel {
	digest := sha256.Sum256([]byte(
		cliChatSessionIdentitySchema + "\x00" + workspaceRoot + "\x00" + workspaceIdentity,
	))
	id := model.ChannelID(fmt.Sprintf("cli-chat-%x", digest[:]))
	return model.Channel{
		ID:            id,
		Scope:         model.ChannelScopeUser,
		Name:          "CLI chat " + string(id),
		Tags:          append([]string(nil), cliChatSessionTags...),
		WorkspaceRoot: workspaceRoot,
		Mode:          model.ChannelModeAssistant,
	}
}

func requireExactCLIChatSessionChannel(expected, actual model.Channel) error {
	if actual.ID != expected.ID || actual.Scope != expected.Scope ||
		actual.Name != expected.Name || !slices.Equal(actual.Tags, expected.Tags) ||
		actual.WorkspaceRoot != expected.WorkspaceRoot ||
		actual.Mode != expected.Mode || actual.DataSourceID != "" ||
		actual.RoleplayViewpointCharacterID != "" {
		return fmt.Errorf(
			"%w: channel %q differs from the exact server-derived binding",
			ErrCLIChatSessionConflict,
			expected.ID,
		)
	}
	return nil
}
