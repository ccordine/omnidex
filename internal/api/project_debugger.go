package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectdebugger"
)

func (s *Server) handleProjectDebugger(w http.ResponseWriter, r *http.Request, projectID int64, action string) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require database")
		return
	}
	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	switch action {
	case "debugger", "debugger/":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.projectDebuggerStatus(r.Context(), project))
	case "debugger/run":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleProjectDebuggerRun(w, r, project)
	default:
		writeError(w, http.StatusNotFound, "project debugger action not found")
	}
}

func (s *Server) handleProjectDebuggerRun(w http.ResponseWriter, r *http.Request, project model.Project) {
	agentResolved, _ := s.resolvedAgentsForProjectCard(r.Context(), project.ID, ScrumCard{})
	agentSystem := "omnidex"
	if v, ok := agentResolved["system"].(string); ok && strings.TrimSpace(v) != "" {
		agentSystem = strings.TrimSpace(v)
	}
	modelResolved, _ := s.resolveModelConfig(project, ScrumCard{})
	modelName := firstNonEmpty(
		modelResolved.Get("planner_model"),
		modelResolved.Get("default_model"),
		s.ollamaDefaultModel,
		"qwen3:4b-thinking",
	)
	metadata, err := projectdebugger.JobMetadata(project.ID, agentSystem, modelName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	instruction := fmt.Sprintf("Run debugger scan for project %s", project.Name)
	job, err := s.repo.EnqueueJob(r.Context(), instruction, projectdebugger.Pipeline(), metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	startedAt := time.Now().UTC().Format(time.RFC3339)
	lastRun := projectdebugger.LastRun{
		JobID:       job.ID,
		ProjectID:   project.ID,
		AgentSystem: agentSystem,
		Model:       modelName,
		Status:      "running",
		StartedAt:   startedAt,
		Summary:     "Scanning project for bugs and quality issues…",
	}
	if err := s.saveDebuggerLastRun(r.Context(), project, lastRun); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":      job,
		"last_run": lastRun,
		"message":  fmt.Sprintf("Queued debugger job #%d", job.ID),
	})
}

func (s *Server) projectDebuggerStatus(ctx context.Context, project model.Project) map[string]any {
	lastRun := loadDebuggerLastRun(project.Settings)
	if lastRun.JobID > 0 && (lastRun.Status == "running" || lastRun.Status == "pending") && s.repo != nil {
		if details, err := s.repo.GetJobDetails(ctx, lastRun.JobID); err == nil {
			job := details.Job
			switch job.Status {
			case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
				lastRun.Status = job.Status
			default:
				lastRun.Status = "running"
			}
			if strings.TrimSpace(job.Error) != "" {
				lastRun.Error = job.Error
			}
		}
	}
	agentResolved, _ := s.resolvedAgentsForProjectCard(ctx, project.ID, ScrumCard{})
	return map[string]any{
		"last_run":     lastRun,
		"agent_config": agentResolved,
	}
}

func loadDebuggerLastRun(settings json.RawMessage) projectdebugger.LastRun {
	if len(settings) == 0 {
		return projectdebugger.LastRun{}
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return projectdebugger.LastRun{}
	}
	raw, ok := payload[projectdebugger.SettingsKey]
	if !ok || len(raw) == 0 {
		return projectdebugger.LastRun{}
	}
	run := projectdebugger.LastRun{}
	_ = json.Unmarshal(raw, &run)
	return run
}

func (s *Server) saveDebuggerLastRun(ctx context.Context, project model.Project, run projectdebugger.LastRun) error {
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings[projectdebugger.SettingsKey] = run
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, project.ID, patch)
	return err
}
