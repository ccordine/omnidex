package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestEncodeScrumAutoWorkSettingsRejectsEveryInvalidProjectAuthority(t *testing.T) {
	t.Parallel()
	for _, test := range invalidScrumAutoWorkProjectSettings() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := encodeScrumAutoWorkSettings(test.settings, DefaultScrumAutoWorkConfig())
			if err == nil {
				t.Fatalf("Scrum auto-work encoding accepted invalid settings %s", test.settings)
			}
		})
	}
}

func TestPostgresDirectScrumAutoWorkWritersRejectSeededInvalidSettings(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	// The current schema rejects these rows at storage time. Remove only those
	// isolated-test guards so each writer is also proven safe against corrupt or
	// pre-existing settings that reached its application boundary.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE projects
			DROP CONSTRAINT projects_current_model_config,
			DROP CONSTRAINT projects_retired_agent_config_absent,
			DROP CONSTRAINT projects_removed_scrum_auto_review_setting,
			DROP CONSTRAINT projects_removed_scrum_auto_play_through_setting
	`); err != nil {
		t.Fatal(err)
	}
	writers := []struct {
		name  string
		apply func(int64) error
	}{
		{
			name: "SetScrumAutoWorkConfig",
			apply: func(projectID int64) error {
				_, err := repository.SetScrumAutoWorkConfig(
					ctx,
					projectID,
					DefaultScrumAutoWorkConfig(),
				)
				return err
			},
		},
		{
			name: "ApplyProjectAutoWork",
			apply: func(projectID int64) error {
				_, err := repository.ApplyProjectAutoWork(ctx, ProjectAutoWorkCommand{
					ProjectID: projectID,
					Action:    ProjectAutoWorkPause,
				})
				return err
			},
		},
	}
	for _, writer := range writers {
		writer := writer
		t.Run(writer.name, func(t *testing.T) {
			for _, test := range invalidScrumAutoWorkProjectSettings() {
				test := test
				t.Run(test.name, func(t *testing.T) {
					project, err := repository.CreateProject(
						ctx,
						fmt.Sprintf("invalid-auto-work-%s-%s", writer.name, test.name),
						t.TempDir(),
						"",
					)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := pool.Exec(
						ctx,
						`UPDATE projects SET settings=$2::jsonb WHERE id=$1`,
						project.ID,
						string(test.settings),
					); err != nil {
						t.Fatal(err)
					}

					err = writer.apply(project.ID)
					if err == nil || !strings.Contains(err.Error(), test.want) {
						t.Fatalf("direct writer error=%v, want %q", err, test.want)
					}
					var unchanged bool
					if err := pool.QueryRow(
						ctx,
						`SELECT settings=$2::jsonb FROM projects WHERE id=$1`,
						project.ID,
						string(test.settings),
					).Scan(&unchanged); err != nil {
						t.Fatal(err)
					}
					if !unchanged {
						t.Fatal("rejected direct writer changed invalid project settings")
					}
				})
			}
		})
	}
}

type invalidScrumAutoWorkProjectSettingsCase struct {
	name     string
	settings json.RawMessage
	want     string
}

func invalidScrumAutoWorkProjectSettings() []invalidScrumAutoWorkProjectSettingsCase {
	return []invalidScrumAutoWorkProjectSettingsCase{
		{
			name: "malformed model config",
			settings: json.RawMessage(
				`{"model_config":[],"scrum_auto_work":{"enabled":false,"source_columns":["ready"]}}`,
			),
			want: "model config must be a JSON object",
		},
		{
			name: "unknown model route",
			settings: json.RawMessage(
				`{"model_config":{"retired_unknown_model_route":"model"},"scrum_auto_work":{"enabled":false,"source_columns":["ready"]}}`,
			),
			want: "unsupported field",
		},
		{
			name:     "planning chat",
			settings: json.RawMessage(`{"planning_chat":[]}`),
			want:     `project setting "planning_chat" was removed`,
		},
		{
			name:     "planning chat config",
			settings: json.RawMessage(`{"planning_chat_config":{}}`),
			want:     `project setting "planning_chat_config" was removed`,
		},
		{
			name:     "planning draft queue",
			settings: json.RawMessage(`{"planning_draft_queue":[]}`),
			want:     `project setting "planning_draft_queue" was removed`,
		},
		{
			name:     "Scrum auto play-through",
			settings: json.RawMessage(`{"scrum_auto_play_through":false}`),
			want:     `project setting "scrum_auto_play_through" was removed`,
		},
		{
			name:     "Scrum auto review",
			settings: json.RawMessage(`{"scrum_auto_review":false}`),
			want:     `project setting "scrum_auto_review" was removed`,
		},
		{
			name:     "retired agent authority",
			settings: json.RawMessage(`{"agent_config":{}}`),
			want:     `project setting "agent_config" was removed`,
		},
	}
}
