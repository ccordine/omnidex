package api

import (
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

func loadScrumAutoReviewConfig(settings json.RawMessage) ScrumAutoReviewConfig {
	cfg := defaultScrumAutoReviewConfig()
	if len(settings) == 0 {
		return cfg
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return cfg
	}
	raw, ok := payload[scrumAutoReviewKey]
	if !ok || len(raw) == 0 {
		return cfg
	}
	_ = json.Unmarshal(raw, &cfg)
	cfg.BounceColumn = normalizeScrumColumn(firstNonEmpty(cfg.BounceColumn, "assigned"))
	if cfg.BounceColumn == "" || cfg.BounceColumn == "review" || cfg.BounceColumn == "done" {
		cfg.BounceColumn = "assigned"
	}
	return cfg
}

func (s *Server) scrumAutoReviewConfig(ctx context.Context, projectID int64) ScrumAutoReviewConfig {
	if s.repo == nil || projectID <= 0 {
		return defaultScrumAutoReviewConfig()
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return defaultScrumAutoReviewConfig()
	}
	return loadScrumAutoReviewConfig(project.Settings)
}

func (s *Server) saveScrumAutoReviewConfig(ctx context.Context, project model.Project, cfg ScrumAutoReviewConfig) error {
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	if cfg.BounceColumn == "" {
		cfg.BounceColumn = "assigned"
	}
	settings[scrumAutoReviewKey] = cfg
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
		instance = agentconfig.FromJSON(card.AgentConfig)
	}
	raw, _, err := s.scrumPlayMetadata(ctx, board, card, projectID, instance)
	if err != nil {
		return nil, err
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return raw, nil
	}
	meta["scrum_auto_review"] = true
	meta["review_always"] = "on"
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
	job, err := s.repo.EnqueueJob(ctx, instruction, "scrum", metadata)
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
	cfg := s.scrumAutoReviewConfig(r.Context(), projectID)
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

func (s *Server) finishScrumAutoReview(r *http.Request, projectID int64, card ScrumCard, job model.JobDetails) (ScrumCard, bool) {
	return s.finishScrumAutoReviewFromContext(r.Context(), projectID, card, job)
}

func (s *Server) finishScrumAutoReviewFromContext(ctx context.Context, projectID int64, card ScrumCard, job model.JobDetails) (ScrumCard, bool) {
	cfg := s.scrumAutoReviewConfig(ctx, projectID)
	bounceColumn := cfg.BounceColumn
	if bounceColumn == "" {
		bounceColumn = "assigned"
	}

	output := collectScrumAgentOutput(job)
	verdict, ok := parseScrumAutoReviewVerdict(output)
	if !ok {
		switch job.Job.Status {
		case model.JobStatusCompleted:
			verdict = scrumAutoReviewVerdict{Ready: true, Summary: "Auto-review completed without explicit verdict — keeping in Review"}
		case model.JobStatusFailed, model.JobStatusCanceled:
			verdict = scrumAutoReviewVerdict{Ready: false, Summary: "Auto-review job did not complete — bouncing for another pass"}
		default:
			return card, false
		}
	}

	if synced, syncOK := syncRunningJobChannelChat(card, job); syncOK {
		card = synced
	}
	if synced, syncOK := syncRunningJobConsoleLog(card, job); syncOK {
		card = synced
	}

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

	if s.repo != nil && projectID > 0 {
		payload, _ := json.Marshal(map[string]any{
			"ready":   verdict.Ready,
			"summary": verdict.Summary,
			"job_id":  strings.TrimSpace(card.JobID),
		})
		event := scrumFlowEventReviewGateFailed
		if verdict.Ready {
			event = scrumFlowEventReviewGatePassed
		}
		_ = s.repo.RecordScrumFlowEvent(
			ctx, projectID, card.ID, event,
			"review", card.Column, scrumPlayReviewing, card.PlayState, payload,
		)
	}
	return card, true
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
