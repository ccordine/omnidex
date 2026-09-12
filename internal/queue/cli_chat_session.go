package queue

import (
	"context"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/jackc/pgx/v5"
)

var cliChatSessionTags = []string{"chat", "cli"}

// Bootstrap is a short, code-owned allocation transaction. A constant lock
// serializes selection by actual workspace values without hashing a path or
// imposing PostgreSQL's narrower B-tree key bound on accepted workspace paths.
const cliChatBootstrapLockID int64 = 1869442666

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
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Channel{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, cliChatBootstrapLockID); err != nil {
		return model.Channel{}, fmt.Errorf("lock CLI chat bootstrap: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT `+channelSelectColumns+`
		FROM ai_channels
		WHERE workspace_root=$1 AND cli_workspace_identity=$2
		LIMIT 2
		FOR SHARE
	`, workspaceRoot, workspaceIdentity)
	if err != nil {
		return model.Channel{}, err
	}
	channels, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Channel, error) {
		return scanChannel(row)
	})
	if err != nil {
		return model.Channel{}, err
	}
	if len(channels) > 1 {
		return model.Channel{}, fmt.Errorf("%w: multiple CLI channels share one workspace binding", ErrCLIChatSessionConflict)
	}
	if len(channels) == 1 {
		channel := channels[0]
		if err := requireExactCLIChatSessionChannel(cliChatSessionChannel(channel.ID, workspaceRoot), channel); err != nil {
			return model.Channel{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return model.Channel{}, err
		}
		return channel, nil
	}
	id, err := projectroot.NewCLIChatChannelID()
	if err != nil {
		return model.Channel{}, err
	}
	expected := cliChatSessionChannel(id, workspaceRoot)
	if err := expected.ValidateForCreate(); err != nil {
		return model.Channel{}, fmt.Errorf("create CLI chat session channel: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_channels (
			id, scope, name, tags, workspace_root, data_source_id,
			mode, roleplay_viewpoint_character_id, cli_workspace_identity
		)
		VALUES ($1, $2, $3, $4, $5, NULL, $6, NULL, $7)
	`, expected.ID, expected.Scope, expected.Name, expected.Tags,
		expected.WorkspaceRoot, expected.Mode, workspaceIdentity); err != nil {
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

func cliChatSessionChannel(id model.ChannelID, workspaceRoot string) model.Channel {
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
			"%w: channel %q differs from the stored server binding",
			ErrCLIChatSessionConflict,
			expected.ID,
		)
	}
	return nil
}
