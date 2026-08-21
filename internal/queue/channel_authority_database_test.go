package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresChannelAuthorityIsTypedPaginatedAndSingular(t *testing.T) {
	ctx := context.Background()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "channel-authority", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
		Name: "Channel authority", Tags: []string{"user-channel"}, WorkspaceRoot: "/srv/workspaces/channel-authority",
	})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := channel
	duplicate.ProjectID = 0
	if _, err := repository.CreateChannel(ctx, duplicate); !strings.Contains(errString(err), ErrChannelAlreadyExists.Error()) {
		t.Fatalf("duplicate create error=%v", err)
	}
	for _, content := range []string{"first", "second", "third"} {
		if _, err := insertChannelMessageForTest(t, repository, channel.ID, model.ChannelMessageRoleUser, content); err != nil {
			t.Fatal(err)
		}
	}
	page, err := repository.ListChannelMessages(ctx, channel.ID, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore || page.NextBeforeID == nil || len(page.Messages) != 2 ||
		page.Messages[0].Content != "second" || page.Messages[1].Content != "third" {
		t.Fatalf("first page=%+v", page)
	}
	older, err := repository.ListChannelMessages(ctx, channel.ID, 2, page.NextBeforeID)
	if err != nil {
		t.Fatal(err)
	}
	if older.HasMore || len(older.Messages) != 1 || older.Messages[0].Content != "first" {
		t.Fatalf("older page=%+v", older)
	}
	if _, _, err := repository.EnqueueChannelTurn(ctx, channel.ID, "first exact turn"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.EnqueueChannelTurn(ctx, channel.ID, "forbidden concurrent turn"); !strings.Contains(errString(err), ErrChannelTurnActive.Error()) {
		t.Fatalf("concurrent turn error=%v", err)
	}
	_, roleErr := pool.Exec(ctx, `
		INSERT INTO ai_channel_messages(channel_id,role,content)
		VALUES ($1,'tool','forbidden')
	`, channel.ID)
	if roleErr == nil ||
		!strings.Contains(roleErr.Error(), "ai_channel_messages_role_check") &&
			!strings.Contains(roleErr.Error(), "ai_channel_messages_content_check") {
		t.Fatalf("invalid role error=%v", roleErr)
	}
}

func TestPostgresChannelWorkspaceBindingRefusesExistingUnboundChannelsAtomically(t *testing.T) {
	ctx := context.Background()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "070")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channels(id,scope,name,tags)
		VALUES ('unbound-channel','user','Unbound channel',ARRAY['user-channel'])
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "071"))
	if err == nil || !strings.Contains(err.Error(), "channel unbound-channel has no explicit durable binding") {
		t.Fatalf("migration error=%v", err)
	}
	var bindingColumns, ledgerCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='ai_channels'
		  AND column_name IN ('project_id','workspace_root')
	`).Scan(&bindingColumns); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE filename='071_channel_workspace_binding.sql'`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if bindingColumns != 0 || ledgerCount != 0 {
		t.Fatalf("rejected migration changed authority: columns=%d ledger=%d", bindingColumns, ledgerCount)
	}
}

func TestPostgresChannelAuthorityRejectsLegacyInternalInventoryAtomically(t *testing.T) {
	ctx := context.Background()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "068")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channels(id,name,tags)
		VALUES ('thought_legacy','Rejected thought',ARRAY['thought-channel'])
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "069"))
	if err == nil || !strings.Contains(err.Error(), "rejected internal channel thought_legacy still exists") {
		t.Fatalf("migration error=%v", err)
	}
	var scopeColumn, rowCount, ledgerCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='ai_channels' AND column_name='scope'
	`).Scan(&scopeColumn); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_channels WHERE id='thought_legacy'`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE filename='069_channel_authority.sql'`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if scopeColumn != 0 || rowCount != 1 || ledgerCount != 0 {
		t.Fatalf("rejected migration changed authority: scope=%d row=%d ledger=%d", scopeColumn, rowCount, ledgerCount)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
