package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestProjectSettingsRejectRetiredAgentAuthority(t *testing.T) {
	t.Parallel()
	if err := validateProjectSettings(json.RawMessage(`{"agent_config":{"agent_system":"codex"}}`)); err == nil {
		t.Fatal("project settings accepted retired agent authority")
	}
}

func TestProjectSettingsRejectRetiredScrumAutoPlayThrough(t *testing.T) {
	t.Parallel()
	err := validateProjectSettings(json.RawMessage(`{"scrum_auto_play_through":false}`))
	if err == nil || !strings.Contains(err.Error(), `project setting "scrum_auto_play_through" was removed`) {
		t.Fatalf("project settings accepted retired Scrum auto play-through authority: %v", err)
	}
}

func TestPostgresProjectUpdateRejectsRetiredScrumAutoPlayThroughAtomically(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(
		ctx, "Retired Scrum auto play-through", t.TempDir(), "",
	)
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"scrum_auto_play_through":true}`)
	_, err = repository.UpdateProjectAtRevision(
		ctx,
		project.ID,
		project.UpdatedAt,
		model.ProjectPatch{Settings: &settings},
	)
	if err == nil || !strings.Contains(err.Error(), `project setting "scrum_auto_play_through" was removed`) {
		t.Fatalf("revision-bound project update accepted retired Scrum auto play-through authority: %v", err)
	}
	retained, err := repository.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(retained.Settings) != `{}` || !retained.UpdatedAt.Equal(project.UpdatedAt) {
		t.Fatalf(
			"rejected retired-setting update changed project settings/revision=%s/%s",
			retained.Settings,
			retained.UpdatedAt,
		)
	}
}

func TestProjectSettingsRejectRetiredModelRoute(t *testing.T) {
	t.Parallel()
	err := validateProjectSettings(json.RawMessage(
		`{"model_config":{"web_claim_evidence_review_model":"retired"}}`,
	))
	if err == nil || !strings.Contains(err.Error(), "unsupported field") {
		t.Fatalf("project settings accepted retired model route: %v", err)
	}
}
