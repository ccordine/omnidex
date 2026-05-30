package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
)

const (
	scrumOutcomeClassifierTimeout  = 20 * time.Second
	scrumOutcomeClassifierMaxChars = 3500
	scrumOutcomeClassifierMinConf  = 0.55
)

type scrumOutcomeClassification struct {
	Outcome    ScrumManagerOutcome `json:"outcome"`
	Confidence float64             `json:"confidence"`
	Reason     string              `json:"reason"`
	RealError  bool                `json:"real_error"`
}

func scrumJobStatusTerminal(status string) bool {
	switch status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		return true
	default:
		return false
	}
}

func normalizeScrumOutcomeClassification(raw ScrumManagerOutcome) ScrumManagerOutcome {
	switch ScrumManagerOutcome(strings.ToLower(strings.TrimSpace(string(raw)))) {
	case ScrumOutcomeSuccess, ScrumOutcomeFailed, ScrumOutcomeBlocked, ScrumOutcomeInProgress, ScrumOutcomePaused:
		return ScrumManagerOutcome(strings.ToLower(strings.TrimSpace(string(raw))))
	default:
		return ""
	}
}

func parseScrumOutcomeClassification(raw string) (scrumOutcomeClassification, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return scrumOutcomeClassification{}, false
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return scrumOutcomeClassification{}, false
	}
	var payload struct {
		Outcome    string  `json:"outcome"`
		Confidence float64 `json:"confidence"`
		Reason     string  `json:"reason"`
		RealError  bool    `json:"real_error"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &payload); err != nil {
		return scrumOutcomeClassification{}, false
	}
	outcome := normalizeScrumOutcomeClassification(ScrumManagerOutcome(payload.Outcome))
	if outcome == "" {
		return scrumOutcomeClassification{}, false
	}
	confidence := payload.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return scrumOutcomeClassification{
		Outcome:    outcome,
		Confidence: confidence,
		Reason:     strings.TrimSpace(payload.Reason),
		RealError:  payload.RealError,
	}, true
}

func scrumOutcomeClassifierSystemPrompt() string {
	return strings.Join([]string{
		"You classify scrum agent run results from sentiment and substance, not keyword matching.",
		"Read whether the agent actually finished useful work, hit a real error, needs the user, or can retry.",
		"Outcomes:",
		"- success: task finished; deliverable ready for human review",
		"- failed: real failure in this run; meaningful work did not complete",
		"- blocked: waiting on user input, credentials, approval, or external dependency",
		"- in_progress: agent clearly expects more work in the same session (rare when the job already stopped)",
		"- paused: transient infra/tooling issue; safe to retry (rate limit, agent unavailable, bridge down)",
		"real_error is true only when this run genuinely failed — not when the agent mentions past errors, warnings, or hypothetical failures.",
		"If job_status is failed/canceled but the agent output reads like a successful handoff, prefer success when work clearly completed.",
		"If job_status is completed but the agent apologizes for failing or asks for missing credentials, prefer failed/blocked/paused.",
		"Respond with JSON only (no markdown fences):",
		`{"outcome":"success|failed|blocked|in_progress|paused","confidence":0.0,"reason":"one short sentence","real_error":false}`,
	}, "\n")
}

func buildScrumOutcomeClassifierUserPrompt(job model.JobDetails, baseline ScrumManagerOutcome) string {
	output := strings.TrimSpace(collectScrumAgentOutput(job))
	if len(output) > scrumOutcomeClassifierMaxChars {
		output = output[len(output)-scrumOutcomeClassifierMaxChars:]
	}
	cardTitle := scrumCardTitleFromMetadata(job.Job.Metadata)
	lines := []string{
		"job_status: " + strings.TrimSpace(job.Job.Status),
		"heuristic_outcome: " + string(baseline),
	}
	if cardTitle != "" {
		lines = append(lines, "card_title: "+cardTitle)
	}
	if instruction := strings.TrimSpace(job.Job.Instruction); instruction != "" {
		if len(instruction) > 400 {
			instruction = instruction[:400] + "…"
		}
		lines = append(lines, "task_instruction: "+instruction)
	}
	lines = append(lines, "", "agent_output:", output)
	return strings.Join(lines, "\n")
}

func scrumCardTitleFromMetadata(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	title, _ := meta["scrum_card_title"].(string)
	return strings.TrimSpace(title)
}

func (s *Server) scrumOutcomeClassifierModel() string {
	return firstNonEmpty(s.ollamaTaggingModel, s.ollamaDefaultModel, "qwen3:4b-thinking")
}

func (s *Server) scrumOutcomeLLMChat(ctx context.Context, system, user string, meta llmContextTelemetryMeta) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scrumOutcomeClassifierTimeout)
	defer cancel()
	modelName := s.scrumOutcomeClassifierModel()
	promptChars := llmPromptCharCount(system, user)
	client := s.ollamaClientWithTimeout(scrumOutcomeClassifierTimeout)
	if client != nil {
		generated, err := client.Chat(ctx, modelName, strings.TrimSpace(system), strings.TrimSpace(user))
		s.recordLLMContextUsage(ctx, llmContextSourceOutcomeClassifier, modelName, "ollama", meta, promptChars, promptChars, false, 0, err)
		return generated, err
	}
	prompt := strings.TrimSpace(system + "\n\n" + user)
	generated, err := s.llmClient.Generate(ctx, modelName, prompt)
	s.recordLLMContextUsage(ctx, llmContextSourceOutcomeClassifier, modelName, s.llmProviderName(), meta, promptChars, len(prompt), false, 0, err)
	return generated, err
}

func (s *Server) classifyScrumAgentOutcome(ctx context.Context, job model.JobDetails, baseline ScrumManagerOutcome) (scrumOutcomeClassification, bool) {
	if s.llmClient == nil {
		return scrumOutcomeClassification{}, false
	}
	if !scrum.IsScrumJob(job.Job.Metadata) && !scrum.IsScrumRawPlay(job.Job.Metadata) {
		return scrumOutcomeClassification{}, false
	}
	if strings.TrimSpace(collectScrumAgentOutput(job)) == "" {
		return scrumOutcomeClassification{}, false
	}
	raw, err := s.scrumOutcomeLLMChat(ctx, scrumOutcomeClassifierSystemPrompt(), buildScrumOutcomeClassifierUserPrompt(job, baseline), llmContextTelemetryMeta{
		CardID: scrumCardTitleFromMetadata(job.Job.Metadata),
		Metadata: map[string]any{
			"job_id":   job.Job.ID,
			"baseline": string(baseline),
		},
	})
	if err != nil {
		return scrumOutcomeClassification{}, false
	}
	classified, ok := parseScrumOutcomeClassification(raw)
	if !ok || classified.Confidence < scrumOutcomeClassifierMinConf {
		return scrumOutcomeClassification{}, false
	}
	return classified, true
}

func scrumBaselinePlayOutcome(job model.JobDetails) ScrumManagerOutcome {
	outcome := resolveScrumManagerOutcome(job)
	if job.Job.Status == model.JobStatusCompleted && outcome == ScrumOutcomeInProgress {
		outcome = ScrumOutcomeSuccess
	}
	return outcome
}

func (s *Server) resolveScrumPlayOutcome(ctx context.Context, job model.JobDetails) (ScrumManagerOutcome, string) {
	baseline := scrumBaselinePlayOutcome(job)
	if !scrumJobStatusTerminal(job.Job.Status) {
		return baseline, ""
	}
	classified, ok := s.classifyScrumAgentOutcome(ctx, job, baseline)
	if !ok {
		return baseline, ""
	}
	note := fmt.Sprintf("outcome scan (%s): %s", classified.Outcome, classified.Reason)
	if classified.RealError && classified.Outcome != ScrumOutcomeSuccess {
		note += " (real error)"
	}
	if outcome, ok := stabilizeCompletedScrumOutcome(job, baseline, classified); ok {
		return outcome, note + " (kept completed job ready for review)"
	}
	return classified.Outcome, note
}

func stabilizeCompletedScrumOutcome(job model.JobDetails, baseline ScrumManagerOutcome, classified scrumOutcomeClassification) (ScrumManagerOutcome, bool) {
	if job.Job.Status == model.JobStatusCompleted && baseline == ScrumOutcomeSuccess && classified.Outcome == ScrumOutcomeInProgress {
		return baseline, true
	}
	return classified.Outcome, false
}
