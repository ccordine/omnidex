package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
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
		status, err := s.projectDebuggerStatus(r.Context(), project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, status)
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
	agentResolved, err := s.resolvedAgentsForProjectCard(r.Context(), project.ID, ScrumCard{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("resolve project debugger agent: %v", err))
		return
	}
	agentSystem := "omnidex"
	if v, ok := agentResolved["system"].(string); ok && strings.TrimSpace(v) != "" {
		agentSystem = strings.TrimSpace(v)
	}
	modelResolved, _, err := s.resolveModelConfig(project, ScrumCard{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("resolve project debugger model: %v", err))
		return
	}
	runtimeDefault, err := s.requiredDefaultLLMModel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("resolve default project debugger model: %v", err))
		return
	}
	analyzerModel, err := modelconfig.AnalyzerModel(modelResolved, runtimeDefault)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ticketModel, err := modelconfig.PlannerTicketModel(modelResolved, runtimeDefault)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metadata, err := projectdebugger.JobMetadata(project.ID, agentSystem, analyzerModel, ticketModel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	instruction := fmt.Sprintf("Analyze codebase for project %s", project.Name)
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
		Model:       analyzerModel,
		Status:      "running",
		StartedAt:   startedAt,
		Summary:     "Scanning project directory and backlog for issues, then creating backlog cards with planning tickets...",
	}
	if err := s.saveDebuggerLastRun(r.Context(), project, lastRun); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("job %d was queued but debugger status persistence failed: %v", job.ID, err))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":      job,
		"last_run": lastRun,
		"message":  fmt.Sprintf("Queued analysis job #%d", job.ID),
	})
}

func (s *Server) projectDebuggerStatus(ctx context.Context, project model.Project) (map[string]any, error) {
	lastRun, err := loadDebuggerLastRun(project.Settings)
	if err != nil {
		return nil, err
	}
	if lastRun.JobID > 0 && (lastRun.Status == "running" || lastRun.Status == "pending") && s.repo != nil {
		jobProjectID, err := s.repo.JobProjectID(ctx, lastRun.JobID)
		if err != nil {
			return nil, fmt.Errorf("load debugger job %d project authority: %w", lastRun.JobID, err)
		}
		if jobProjectID != project.ID {
			return nil, fmt.Errorf("debugger job %d belongs to project %d, expected %d", lastRun.JobID, jobProjectID, project.ID)
		}
		details, err := s.repo.GetJobDetails(ctx, lastRun.JobID)
		if err != nil {
			return nil, fmt.Errorf("load debugger job %d: %w", lastRun.JobID, err)
		}
		job := details.Job
		switch job.Status {
		case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
			model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
			lastRun.Status = job.Status
		default:
			return nil, fmt.Errorf("debugger job %d has unsupported status %q", job.ID, job.Status)
		}
		if strings.TrimSpace(job.Error) != "" {
			lastRun.Error = job.Error
		}
	}
	agentResolved, err := s.resolvedAgentsForProjectCard(ctx, project.ID, ScrumCard{})
	if err != nil {
		return nil, fmt.Errorf("resolve project debugger agent: %w", err)
	}
	return map[string]any{
		"last_run":     lastRun,
		"agent_config": agentResolved,
	}, nil
}

func loadDebuggerLastRun(settings json.RawMessage) (projectdebugger.LastRun, error) {
	if len(settings) == 0 {
		return projectdebugger.LastRun{}, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return projectdebugger.LastRun{}, fmt.Errorf("parse project debugger settings: %w", err)
	}
	raw, ok := payload[projectdebugger.SettingsKey]
	if !ok || len(raw) == 0 {
		return projectdebugger.LastRun{}, nil
	}
	run := projectdebugger.LastRun{}
	if err := json.Unmarshal(raw, &run); err != nil {
		return projectdebugger.LastRun{}, fmt.Errorf("parse %s: %w", projectdebugger.SettingsKey, err)
	}
	if run.JobID > 0 && run.ProjectID <= 0 {
		return projectdebugger.LastRun{}, fmt.Errorf("%s.project_id is required when job_id is set", projectdebugger.SettingsKey)
	}
	return run, nil
}

func (s *Server) saveDebuggerLastRun(ctx context.Context, project model.Project, run projectdebugger.LastRun) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("project debugger status requires a PostgreSQL repository")
	}
	if ctx == nil {
		return fmt.Errorf("project debugger status requires a context")
	}
	if project.ID <= 0 || run.JobID <= 0 || run.ProjectID != project.ID {
		return fmt.Errorf("project debugger status has invalid project or job authority")
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		if err := json.Unmarshal(project.Settings, &settings); err != nil {
			return fmt.Errorf("parse existing project settings: %w", err)
		}
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
