package queue

import (
	"fmt"
	"strings"
	"testing"
)

func TestPostgresScrumChannelCleanStartAppliesOnlyToEmptyAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	bundle := loadMigrationBundleThroughPrefix(t, "089")
	if err := repository.EnsureSchema(t.Context(), bundle); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(t.Context(), bundle); err != nil {
		t.Fatalf("retry clean-start migration bundle: %v", err)
	}
	assertAppliedMigrationCount(t, pool, scrumChannelRelationMigration, 1)

	for _, relation := range []string{"scrum_channel_operations", "scrum_flow_events"} {
		var present bool
		if err := pool.QueryRow(t.Context(), `SELECT to_regclass($1) IS NOT NULL`, relation).Scan(&present); err != nil {
			t.Fatal(err)
		}
		if present {
			t.Fatalf("retired relation %s remains after clean start", relation)
		}
	}
	var retiredColumns int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='scrum_cards'
		  AND column_name IN (
		    'chat','planning_chat','console_log',
		    'agent_stream_chat_cursor','agent_stream_console_cursor'
		  )
	`).Scan(&retiredColumns); err != nil {
		t.Fatal(err)
	}
	if retiredColumns != 0 {
		t.Fatalf("retired Scrum columns remaining=%d", retiredColumns)
	}
}

func TestPostgresScrumChannelCleanStartRejectsNonemptyLegacyAuthorityAtomically(t *testing.T) {
	for _, test := range []struct {
		name string
		seed func(*testing.T, *Repository)
		want string
	}{
		{
			name: "card",
			seed: func(t *testing.T, repository *Repository) {
				projectID := insertCleanStartProject(t, repository, "card")
				if _, err := repository.pool.Exec(t.Context(), `
					INSERT INTO scrum_cards(id,project_id,title) VALUES('legacy-card',$1,'Legacy')
				`, projectID); err != nil {
					t.Fatal(err)
				}
			},
			want: "scrum_cards=1, scrum_channel_operations=0, scrum_flow_events=0, scrum_channel_registry=0",
		},
		{
			name: "flow event",
			seed: func(t *testing.T, repository *Repository) {
				projectID := insertCleanStartProject(t, repository, "flow")
				if _, err := repository.pool.Exec(t.Context(), `
					INSERT INTO scrum_flow_events(project_id,card_id,event_type)
					VALUES($1,'missing-card','conversation')
				`, projectID); err != nil {
					t.Fatal(err)
				}
			},
			want: "scrum_cards=0, scrum_channel_operations=0, scrum_flow_events=1, scrum_channel_registry=0",
		},
		{
			name: "registry",
			seed: func(t *testing.T, repository *Repository) {
				operationID := "lifecycle_operation_" + strings.Repeat("1", 64)
				if _, err := repository.pool.Exec(t.Context(), `
					INSERT INTO lifecycle_operation_registry(operation_id,kind,command_sha256,command_payload)
					VALUES($1,'scrum_channel_message',$2,jsonb_build_object('operation_id',$1::text))
				`, operationID, strings.Repeat("2", 64)); err != nil {
					t.Fatal(err)
				}
			},
			want: "scrum_cards=0, scrum_channel_operations=0, scrum_flow_events=0, scrum_channel_registry=1",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "088")); err != nil {
				t.Fatal(err)
			}
			test.seed(t, repository)

			err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "089"))
			if err == nil || !strings.Contains(err.Error(), "migration 089 reset required") ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("migration error=%v, want reset counts %q", err, test.want)
			}
			var oldRelation, oldColumn bool
			if err := pool.QueryRow(t.Context(), `
				SELECT to_regclass('scrum_channel_operations') IS NOT NULL,
				       EXISTS (
				         SELECT 1 FROM information_schema.columns
				         WHERE table_schema=current_schema() AND table_name='scrum_cards' AND column_name='chat'
				       )
			`).Scan(&oldRelation, &oldColumn); err != nil {
				t.Fatal(err)
			}
			if !oldRelation || !oldColumn {
				t.Fatalf("rejected migration partially committed relation=%t chat_column=%t", oldRelation, oldColumn)
			}
		})
	}
}

func insertCleanStartProject(t *testing.T, repository *Repository, suffix string) int64 {
	t.Helper()
	var projectID int64
	if err := repository.pool.QueryRow(t.Context(), `
		INSERT INTO projects(location,name) VALUES($1,$2) RETURNING id
	`, fmt.Sprintf("/tmp/scrum-clean-start-%s", suffix), "Clean start").Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	return projectID
}
