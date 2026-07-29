package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

const scrumAutoReviewKey = "scrum_auto_review"

const scrumPlayReviewing = "reviewing"

type ScrumAutoReviewConfig struct {
	Enabled      bool   `json:"enabled"`
	BounceColumn string `json:"bounce_column"`
}

var scrumAutoReviewReadyPattern = regexp.MustCompile(`(?i)"ready_for_review"\s*:\s*(true|false)`)
var scrumAutoReviewStatusPattern = regexp.MustCompile(`(?im)^SCRUM_REVIEW:\s*(ready|not_ready|pass|fail)\s*$`)

func defaultScrumAutoReviewConfig() ScrumAutoReviewConfig {
	return ScrumAutoReviewConfig{
		Enabled:      false,
		BounceColumn: "assigned",
	}
}

func validateScrumAutoReviewConfig(cfg ScrumAutoReviewConfig) (ScrumAutoReviewConfig, error) {
	if strings.TrimSpace(cfg.BounceColumn) == "" {
		cfg.BounceColumn = "assigned"
	}
	cfg.BounceColumn = normalizeScrumColumn(cfg.BounceColumn)
	switch cfg.BounceColumn {
	case "assigned", "in_progress", "ready":
		return cfg, nil
	default:
		return ScrumAutoReviewConfig{}, fmt.Errorf("unsupported Scrum auto-review bounce column %q", cfg.BounceColumn)
	}
}

func loadScrumAutoReviewConfig(settings json.RawMessage) (ScrumAutoReviewConfig, error) {
	cfg := defaultScrumAutoReviewConfig()
	if len(bytes.TrimSpace(settings)) == 0 {
		return cfg, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return ScrumAutoReviewConfig{}, fmt.Errorf("decode project settings for Scrum auto-review: %w", err)
	}
	raw, ok := payload[scrumAutoReviewKey]
	if !ok || len(raw) == 0 {
		return cfg, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ScrumAutoReviewConfig{}, fmt.Errorf("Scrum auto-review config must be an object")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ScrumAutoReviewConfig{}, fmt.Errorf("decode Scrum auto-review config: %w", err)
	}
	return validateScrumAutoReviewConfig(cfg)
}

func (s *Server) saveScrumAutoReviewConfig(ctx context.Context, project model.Project, cfg ScrumAutoReviewConfig) error {
	if s == nil || s.repo == nil || project.ID <= 0 {
		return fmt.Errorf("postgres repository and project are required to save Scrum auto-review config")
	}
	validated, err := validateScrumAutoReviewConfig(cfg)
	if err != nil {
		return err
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		if err := json.Unmarshal(project.Settings, &settings); err != nil {
			return fmt.Errorf("decode project %d settings: %w", project.ID, err)
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings[scrumAutoReviewKey] = validated
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, project.ID, patch)
	return err
}

func isScrumAutoReviewJob(metadata json.RawMessage) bool {
	if len(metadata) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return false
	}
	v, ok := payload["scrum_auto_review"]
	if !ok {
		return false
	}
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "on", "yes", "1":
			return true
		}
	}
	return false
}

func buildScrumAutoReviewInstruction(board ScrumBoard, card ScrumCard) string {
	lines := []string{
		"Independent scrum review — fresh agent eyes on work that just landed in Review.",
		"",
		"Your job is to verify the implementation against the card guide before humans review it.",
		"Inspect actual project changes (git diff, modified files, tests) — do not trust claims without evidence.",
		"",
		"Card: " + card.Title,
	}
	if strings.TrimSpace(board.ProjectDirectory) != "" {
		lines = append(lines, "Project directory: "+board.ProjectDirectory)
	}
	lines = appendScrumCardContextLines(lines, card)
	lines = append(lines,
		"",
		"Checklist:",
		"1. Compare changes to the card description, checklist, test criteria, and recipe/guide.",
		"2. Confirm acceptance criteria are met with evidence (files, tests, behavior).",
		"3. Flag gaps, regressions, or scope drift.",
		"",
		"Respond with JSON only (no markdown fences):",
		`{"ready_for_review":true|false,"summary":"brief verdict","gaps":["specific gap"],"recommendations":["next step"]}`,
		"",
		"Or emit a single line: SCRUM_REVIEW: ready  OR  SCRUM_REVIEW: not_ready",
	)
	return strings.Join(lines, "\n")
}

type scrumAutoReviewVerdict struct {
	Ready           bool
	Summary         string
	Gaps            []string
	Recommendations []string
}

func parseScrumAutoReviewVerdict(output string) (scrumAutoReviewVerdict, bool) {
	output = strings.TrimSpace(output)
	if output == "" {
		return scrumAutoReviewVerdict{}, false
	}
	if match := scrumAutoReviewStatusPattern.FindStringSubmatch(output); len(match) > 1 {
		status := strings.ToLower(match[1])
		return scrumAutoReviewVerdict{
			Ready:   status == "ready" || status == "pass",
			Summary: strings.TrimSpace(output),
		}, true
	}
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start >= 0 && end > start {
		var payload struct {
			ReadyForReview  bool     `json:"ready_for_review"`
			Summary         string   `json:"summary"`
			Gaps            []string `json:"gaps"`
			Recommendations []string `json:"recommendations"`
		}
		if err := json.Unmarshal([]byte(output[start:end+1]), &payload); err == nil {
			return scrumAutoReviewVerdict{
				Ready:           payload.ReadyForReview,
				Summary:         strings.TrimSpace(payload.Summary),
				Gaps:            payload.Gaps,
				Recommendations: payload.Recommendations,
			}, true
		}
	}
	if match := scrumAutoReviewReadyPattern.FindStringSubmatch(output); len(match) > 1 {
		return scrumAutoReviewVerdict{
			Ready:   strings.EqualFold(match[1], "true"),
			Summary: trimText(output, 400),
		}, true
	}
	return scrumAutoReviewVerdict{}, false
}

func trimText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "…"
}

func (s *Server) scrumAutoReviewMetadata(ctx context.Context, board ScrumBoard, card ScrumCard, projectID int64) ([]byte, error) {
	instance := agentconfig.Config{}
	if len(card.AgentConfig) > 0 {
		var err error
		instance, err = agentconfig.FromJSON(card.AgentConfig)
		if err != nil {
			return nil, fmt.Errorf("parse Scrum card agent configuration: %w", err)
		}
	}
	raw, _, err := s.scrumPlayMetadata(ctx, board, card, projectID, instance)
	if err != nil {
		return nil, err
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parse Scrum auto-review metadata: %w", err)
	}
	meta["scrum_auto_review"] = true
	meta["scrum_return_column"] = "review"
	delete(meta, "scrum_raw_play")
	raw, err = json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *Server) startScrumAutoReview(r *http.Request, board ScrumBoard, projectID int64, card ScrumCard) (ScrumCard, error) {
	if card.PlayState == scrumPlayReviewing {
		return card, nil
	}
	instruction := buildScrumAutoReviewInstruction(board, card)
	metadata, err := s.scrumAutoReviewMetadata(r.Context(), board, card, projectID)
	if err != nil {
		return ScrumCard{}, err
	}
	if s.repo == nil {
		return ScrumCard{}, fmt.Errorf("auto review requires queue mode")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	job, err := s.repo.EnqueueJob(ctx, instruction, model.PipelineScrum, metadata)
	cancel()
	if err != nil {
		return ScrumCard{}, err
	}
	card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Auto-review job #%d queued — checking changes against card guide", job.ID))
	card.JobID = fmt.Sprintf("%d", job.ID)
	card.Column = "review"
	card.PlayState = scrumPlayReviewing
	card.QueueOrder = 0
	return s.persistScrumCard(r, projectID, card)
}

func (s *Server) maybeStartScrumAutoReview(r *http.Request, projectID int64, board ScrumBoard, card ScrumCard, fromColumn string) (ScrumCard, error) {
	automation, err := s.scrumAutomationSettings(r.Context(), projectID)
	if err != nil {
		return ScrumCard{}, err
	}
	cfg := automation.AutoReview
	if !cfg.Enabled {
		return card, nil
	}
	if normalizeScrumColumn(card.Column) != "review" {
		return card, nil
	}
	fromColumn = normalizeScrumColumn(fromColumn)
	if fromColumn == "review" {
		return card, nil
	}
	if card.PlayState == scrumPlayReviewing || card.PlayState == scrumPlayRunning {
		return card, nil
	}
	return s.startScrumAutoReview(r, board, projectID, card)
}

func (s *Server) finishScrumAutoReview(r *http.Request, projectID int64, card ScrumCard, job model.JobDetails) (ScrumCard, bool, error) {
	return s.finishScrumAutoReviewFromContext(r.Context(), projectID, card, job)
}

func (s *Server) finishScrumAutoReviewFromContext(ctx context.Context, projectID int64, card ScrumCard, job model.JobDetails) (ScrumCard, bool, error) {
	automation, err := s.scrumAutomationSettings(ctx, projectID)
	if err != nil {
		return card, false, err
	}
	cfg := automation.AutoReview
	bounceColumn := cfg.BounceColumn

	output := collectScrumAgentOutput(job)
	verdict, ok := parseScrumAutoReviewVerdict(output)
	jobID := strings.TrimSpace(card.JobID)
	if !ok {
		reason := "Auto-review returned no explicit verdict"
		switch job.Job.Status {
		case model.JobStatusFailed, model.JobStatusCanceled:
			reason = fmt.Sprintf("Auto-review job %s", job.Job.Status)
			if detail := strings.TrimSpace(job.Job.Error); detail != "" {
				reason += ": " + detail
			}
		case model.JobStatusCompleted:
		default:
			return card, false, nil
		}
		card = syncScrumAutoReviewOutput(card, job)
		card.PlayState = ""
		card.QueueOrder = 0
		card.JobID = ""
		card.Column = "error"
		card = appendScrumChannelEvent(card, "error", reason)
		verdict = scrumAutoReviewVerdict{Ready: false, Summary: reason}
		if err := s.recordScrumAutoReviewFlowEvent(ctx, projectID, card, jobID, verdict); err != nil {
			return card, false, err
		}
		return card, true, nil
	}

	card = syncScrumAutoReviewOutput(card, job)

	card.PlayState = ""
	card.QueueOrder = 0
	card.JobID = ""
	if verdict.Ready {
		card.Column = "review"
		note := "Auto-review passed — ready for human review"
		if verdict.Summary != "" {
			note += ": " + verdict.Summary
		}
		card = appendScrumChannelEvent(card, "system", note)
		if len(verdict.Gaps) > 0 {
			card = appendScrumChannelEvent(card, "assistant", "Review notes:\n- "+strings.Join(verdict.Gaps, "\n- "))
		}
	} else {
		card.Column = bounceColumn
		note := fmt.Sprintf("Auto-review bounced card to %s — not ready for Review", scrumReviewColumnLabel(bounceColumn))
		if verdict.Summary != "" {
			note += ": " + verdict.Summary
		}
		card = appendScrumChannelEvent(card, "system", note)
		detail := strings.TrimSpace(strings.Join(append(verdict.Gaps, verdict.Recommendations...), "\n- "))
		if detail != "" {
			card = appendScrumChannelEvent(card, "assistant", "Fix before re-review:\n- "+detail)
		}
	}

	if err := s.recordScrumAutoReviewFlowEvent(ctx, projectID, card, jobID, verdict); err != nil {
		return card, false, err
	}
	return card, true, nil
}

func syncScrumAutoReviewOutput(card ScrumCard, job model.JobDetails) ScrumCard {
	if synced, ok := syncRunningJobChannelChat(card, job); ok {
		card = synced
	}
	if synced, ok := syncRunningJobConsoleLog(card, job); ok {
		card = synced
	}
	return card
}

func (s *Server) recordScrumAutoReviewFlowEvent(ctx context.Context, projectID int64, card ScrumCard, jobID string, verdict scrumAutoReviewVerdict) error {
	if s == nil || s.repo == nil || projectID <= 0 {
		return fmt.Errorf("postgres repository and project are required to record Scrum auto-review outcome")
	}
	payload, err := json.Marshal(map[string]any{
		"ready":   verdict.Ready,
		"summary": verdict.Summary,
		"job_id":  strings.TrimSpace(jobID),
	})
	if err != nil {
		return fmt.Errorf("encode Scrum auto-review outcome: %w", err)
	}
	event := scrumFlowEventReviewGateFailed
	if verdict.Ready {
		event = scrumFlowEventReviewGatePassed
	}
	if err := s.repo.RecordScrumFlowEvent(
		ctx, projectID, card.ID, event,
		"review", card.Column, scrumPlayReviewing, card.PlayState, payload,
	); err != nil {
		return fmt.Errorf("record Scrum auto-review outcome for card %q: %w", card.ID, err)
	}
	return nil
}

func scrumReviewColumnLabel(column string) string {
	switch normalizeScrumColumn(column) {
	case "assigned":
		return "Assigned"
	case "in_progress":
		return "In Progress"
	case "ready":
		return "Ready"
	default:
		return column
	}
}
