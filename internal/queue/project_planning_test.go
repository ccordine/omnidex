package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrumcardllm"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateProjectPlanningConfig(t *testing.T) {
	config, err := validateProjectPlanningConfig(model.ProjectPlanningConfig{Model: " qwen ", ReasoningMode: "Thinking"})
	if err != nil || config.Model != "qwen" || config.ReasoningMode != "thinking" {
		t.Fatalf("config=%+v err=%v", config, err)
	}
	if _, err := validateProjectPlanningConfig(model.ProjectPlanningConfig{ReasoningMode: "maybe"}); err == nil {
		t.Fatal("unknown reasoning mode must fail")
	}
}

func TestValidateProjectPlanningDraft(t *testing.T) {
	draft := model.ProjectPlanningDraft{ID: " draft ", Title: " Card ", Column: "backlog", Source: "chat", Checklist: []string{" test "}}
	if err := validateProjectPlanningDraft(&draft, 42); err != nil {
		t.Fatal(err)
	}
	if draft.ProjectID != 42 || draft.ID != "draft" || draft.Checklist[0] != "test" {
		t.Fatalf("draft=%+v", draft)
	}
	for _, invalid := range []model.ProjectPlanningDraft{
		{ID: "x", Title: "Card", Column: "invented", Source: "chat"},
		{ID: "x", Title: "Card", Column: "backlog", Source: "chat", Checklist: []string{""}},
		{ID: "x", Title: "Card", Column: "backlog", Source: "chat", Status: "added"},
	} {
		if err := validateProjectPlanningDraft(&invalid, 42); err == nil {
			t.Fatalf("draft %+v must fail", invalid)
		}
	}
}

func TestNormalizePlanningDraftIDsRejectsDuplicates(t *testing.T) {
	if _, err := normalizePlanningDraftIDs([]string{"draft_1", " draft_1 "}); err == nil {
		t.Fatal("duplicate draft IDs must fail")
	}
	if _, err := normalizePlanningDraftIDs([]string{""}); err == nil {
		t.Fatal("empty draft ID must fail")
	}
}

func TestValidateDebuggerCardJSONIsStrict(t *testing.T) {
	if err := validateDebuggerCardJSON(
		json.RawMessage(`[{"text":"verify","done":false}]`),
		json.RawMessage(`["internal/app.go"]`),
		json.RawMessage(`["bug","analysis"]`),
	); err != nil {
		t.Fatal(err)
	}
	for _, checklist := range []json.RawMessage{
		json.RawMessage(`[]`),
		json.RawMessage(`[{"text":"","done":false}]`),
		json.RawMessage(`[{"text":"verify","done":true}]`),
		json.RawMessage(`[{"text":"verify","done":false,"legacy":true}]`),
	} {
		if err := validateDebuggerCardJSON(checklist, json.RawMessage(`[]`), json.RawMessage(`["analysis"]`)); err == nil {
			t.Fatalf("checklist %s must fail", checklist)
		}
	}
}

func TestProjectSettingsRejectRemovedPlanningState(t *testing.T) {
	for _, key := range removedProjectPlanningSettingKeys {
		raw := json.RawMessage(`{"` + key + `":[]}`)
		if err := validateProjectSettings(raw); err == nil {
			t.Fatalf("removed setting %q must fail", key)
		}
	}
	if err := validateProjectSettings(json.RawMessage(`{"model_config":{}}`)); err != nil {
		t.Fatal(err)
	}
	if err := validateProjectSettings(json.RawMessage(`null`)); err == nil {
		t.Fatal("null settings must fail")
	}
}

func TestProjectPlanningMigrationOwnsDurableState(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "021_project_planning_state.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS project_planning_configs",
		"CREATE TABLE IF NOT EXISTS project_planning_messages",
		"CREATE TABLE IF NOT EXISTS project_planning_drafts",
		"RAISE EXCEPTION",
		"- 'planning_chat'",
		"- 'planning_chat_config'",
		"- 'planning_draft_queue'",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("planning migration missing %q", required)
		}
	}
}

func TestProjectPlanningRepositoryRoundTrip(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL planning state tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	repo := New(pool)
	if err := repo.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	location := fmt.Sprintf("/tmp/omni-planning-test-%d", time.Now().UnixNano())
	project, err := repo.CreateProject(ctx, "Planning test", location, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = repo.DeleteProject(cleanupContext, project.ID)
	})

	config := model.ProjectPlanningConfig{Model: "planner:test", ReasoningMode: "thinking"}
	commit, err := repo.CommitProjectPlanningResponse(ctx, project.ID, ProjectPlanningCommit{
		Config: config, UserMessage: "Plan this", AssistantMessage: "Here is the plan",
		Drafts: []model.ProjectPlanningDraft{{ID: "draft_1", Title: "Implement it", Column: "backlog", Source: "plan", Checklist: []string{"test it"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(commit.Messages.Messages) != 2 || len(commit.Drafts) != 1 {
		t.Fatalf("commit=%+v", commit)
	}
	storedConfig, err := repo.GetProjectPlanningConfig(ctx, project.ID)
	if err != nil || storedConfig.Model != config.Model || storedConfig.ReasoningMode != config.ReasoningMode {
		t.Fatalf("config=%+v err=%v", storedConfig, err)
	}

	mutation, err := repo.MutateProjectPlanningDrafts(ctx, project.ID, ProjectPlanningDraftMutation{Action: "add", DraftID: "draft_1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mutation.Cards) != 1 || len(mutation.Drafts) != 1 || mutation.Drafts[0].Status != "added" || mutation.Drafts[0].CardID != mutation.Cards[0].ID {
		t.Fatalf("mutation=%+v", mutation)
	}
	var checklist []map[string]any
	if err := json.Unmarshal(mutation.Cards[0].Checklist, &checklist); err != nil {
		t.Fatal(err)
	}
	if len(checklist) != 1 || checklist[0]["text"] != "test it" || checklist[0]["done"] != false {
		t.Fatalf("checklist=%+v", checklist)
	}
	if err := repo.UpdateProjectSetting(ctx, project.ID, "debugger_last_run", json.RawMessage(`{"status":"completed"}`)); err != nil {
		t.Fatal(err)
	}
	updatedProject, err := repo.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(updatedProject.Settings, &settings); err != nil {
		t.Fatal(err)
	}
	var lastRun struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(settings["debugger_last_run"], &lastRun); err != nil || lastRun.Status != "completed" {
		t.Fatalf("last_run=%+v err=%v settings=%s", lastRun, err, updatedProject.Settings)
	}
	debuggerCard, debuggerJob, err := repo.CreateProjectDebuggerCardJob(ctx, project.ID, ProjectDebuggerCardInput{
		Title:       "Debugger finding",
		Description: "Evidence-backed finding",
		Column:      "backlog",
		Checklist:   json.RawMessage(`[{"text":"verify","done":false}]`),
		RefFiles:    json.RawMessage(`["internal/example.go"]`),
		Tags:        json.RawMessage(`["bug","analysis"]`),
		CardPrompt:  "Title: Debugger finding",
		TicketModel: "planner:test",
		Ticket: scrumcardllm.TicketRequest{
			Prompt: "Draft a plan", CardPrompt: "Title: Debugger finding", PlanningMode: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if debuggerJob.ID <= 0 || debuggerCard.TicketJobID != fmt.Sprint(debuggerJob.ID) {
		t.Fatalf("debugger card=%+v job=%+v", debuggerCard, debuggerJob)
	}
}
