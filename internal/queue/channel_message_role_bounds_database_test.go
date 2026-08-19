package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresChannelMessageRoleBoundsAcceptExactMaxima(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "117")); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "role-bounds", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant, Name: "Role bounds",
		WorkspaceRoot: "/srv/workspaces/role-bounds",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		role model.ChannelMessageRole
		size int
	}{
		{role: model.ChannelMessageRoleUser, size: model.MaxFreeFormTurnBytes},
		{role: model.ChannelMessageRoleAssistant, size: model.MaxChannelContentBytes},
	} {
		if _, err := insertChannelMessageForTest(t, repository, channel.ID, test.role, strings.Repeat("x", test.size)); err != nil {
			t.Fatalf("role %q maximum rejected: %v", test.role, err)
		}
	}
}

func TestPostgresChannelMessageRoleBoundsRejectMaxPlusOne(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "117")); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "role-overflow", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant, Name: "Role overflow",
		WorkspaceRoot: "/srv/workspaces/role-overflow",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		role model.ChannelMessageRole
		size int
	}{
		{role: model.ChannelMessageRoleUser, size: model.MaxFreeFormTurnBytes + 1},
		{role: model.ChannelMessageRoleAssistant, size: model.MaxChannelContentBytes + 1},
	} {
		if _, err := insertChannelMessageForTest(t, repository, channel.ID, test.role, strings.Repeat("x", test.size)); err == nil ||
			!strings.Contains(err.Error(), "ai_channel_messages_content_check") {
			t.Fatalf("role %q overflow error=%v", test.role, err)
		}
	}
}

func TestPostgresChannelMessageRoleBoundsRefuseInvalidHistoryAtomically(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "071")); err != nil {
		t.Fatal(err)
	}
	channel := model.Channel{ID: "role-history"}
	var projectID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/srv/workspaces/role-history','Role history')
		RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channels(id,scope,name,tags,project_id,workspace_root)
		VALUES($1,'user','Role history','{}'::text[],$2,'/srv/workspaces/role-history')
	`, channel.ID, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := insertChannelMessageForTest(
		t, repository, channel.ID, model.ChannelMessageRoleUser,
		strings.Repeat("x", model.MaxFreeFormTurnBytes+1),
	); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "072"))
	if err == nil || !strings.Contains(err.Error(), "outside its typed role contract") {
		t.Fatalf("migration error=%v", err)
	}
	var broadConstraint, ledgerCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_constraint
		WHERE conrelid='ai_channel_messages'::regclass
		  AND conname='ai_channel_messages_content_check'
		  AND pg_get_constraintdef(oid,true) LIKE '%65536%'
	`).Scan(&broadConstraint); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM schema_migrations
		WHERE filename='072_channel_message_role_bounds.sql'
	`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if broadConstraint != 1 || ledgerCount != 0 {
		t.Fatalf("rejected migration changed authority: broad_constraint=%d ledger=%d", broadConstraint, ledgerCount)
	}
}
