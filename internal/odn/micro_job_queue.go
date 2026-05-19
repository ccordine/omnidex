package odn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	defaultMicroJobMaxJobs     = 12
	defaultMicroJobPlanTimeout = 45 * time.Second
)

type MicroJobQueueConfig struct {
	MaxJobs               int
	PlanTimeout           time.Duration
	DisableProjectProfile bool
	ProjectProfileConfig  ProjectRunProfileConfig
}

type MicroJob struct {
	ID         string `json:"id"`
	Objective  string `json:"objective"`
	Acceptance string `json:"acceptance"`
}

type MicroJobQueueResult struct {
	Jobs           []MicroJob
	Results        []MicroJobResult
	ProjectProfile ProjectRunProfile
	Done           bool
	Summary        string
}

type MicroJobResult struct {
	Job      MicroJob
	Done     bool
	Error    string
	Command  string
	ExitCode int
	Answer   string
}

type microJobPlanEnvelope struct {
	Jobs []MicroJob `json:"jobs"`
}

func DefaultMicroJobQueueConfig() MicroJobQueueConfig {
	return MicroJobQueueConfig{
		MaxJobs:               defaultMicroJobMaxJobs,
		PlanTimeout:           defaultMicroJobPlanTimeout,
		DisableProjectProfile: false,
		ProjectProfileConfig:  ProjectRunProfileConfig{},
	}
}

func ExecuteMicroJobQueue(ctx context.Context, objective, workspacePath string, client CommandDecisionClient, stdout, stderr io.Writer, cfg MicroJobQueueConfig) (MicroJobQueueResult, error) {
	cfg = normalizeMicroJobQueueConfig(cfg)
	if strings.TrimSpace(objective) == "" {
		return MicroJobQueueResult{}, fmt.Errorf("micro job objective is required")
	}
	if client == nil {
		return MicroJobQueueResult{}, fmt.Errorf("llm client is required")
	}

	profile := ProjectRunProfile{}
	if !cfg.DisableProjectProfile {
		var err error
		profile, err = BuildProjectRunProfile(ctx, workspacePath, client, cfg.ProjectProfileConfig)
		if err != nil {
			return MicroJobQueueResult{}, fmt.Errorf("build project run profile: %w", err)
		}
	}

	jobs, err := planMicroJobs(ctx, objective, workspacePath, profile, client, cfg)
	if err != nil {
		return MicroJobQueueResult{}, err
	}

	result := MicroJobQueueResult{Jobs: jobs, ProjectProfile: profile}
	completed := []MicroJobResult{}
	for _, job := range jobs {
		jobPrompt := buildMicroJobPrompt(objective, workspacePath, profile, job, completed)
		commandResult, execErr := RunStructuredCommandDecision(ctx, jobPrompt, client, stdout, stderr)
		jobResult := MicroJobResult{
			Job:      job,
			Done:     execErr == nil,
			Command:  commandResult.Command,
			ExitCode: commandResult.ExitCode,
			Answer:   commandResult.Answer,
		}
		if execErr != nil {
			jobResult.Error = execErr.Error()
			result.Results = append(result.Results, jobResult)
			result.Done = false
			result.Summary = fmt.Sprintf("Stopped at %s: %s", job.ID, jobResult.Error)
			return result, nil
		}
		result.Results = append(result.Results, jobResult)
		completed = append(completed, jobResult)
	}
	result.Done = len(result.Results) == len(result.Jobs)
	result.Summary = fmt.Sprintf("Completed %d/%d micro job(s).", len(result.Results), len(result.Jobs))
	return result, nil
}

func normalizeMicroJobQueueConfig(cfg MicroJobQueueConfig) MicroJobQueueConfig {
	if cfg.MaxJobs <= 0 {
		cfg.MaxJobs = defaultMicroJobMaxJobs
	}
	if cfg.PlanTimeout <= 0 {
		cfg.PlanTimeout = defaultMicroJobPlanTimeout
	}
	cfg.ProjectProfileConfig = normalizeProjectRunProfileConfig(cfg.ProjectProfileConfig)
	return cfg
}

func planMicroJobs(ctx context.Context, objective, workspacePath string, profile ProjectRunProfile, client CommandDecisionClient, cfg MicroJobQueueConfig) ([]MicroJob, error) {
	planCtx, cancel := context.WithTimeout(ctx, cfg.PlanTimeout)
	defer cancel()

	resp, err := client.ChatRaw(planCtx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: buildMicroJobPlannerPrompt(cfg.MaxJobs)},
			{Role: "user", Content: fmt.Sprintf("Workspace: %s\nProject run profile:\n%s\nObjective:\n%s", strings.TrimSpace(workspacePath), formatProjectRunProfile(profile), strings.TrimSpace(objective))},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"jobs": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id":         map[string]interface{}{"type": "string"},
							"objective":  map[string]interface{}{"type": "string"},
							"acceptance": map[string]interface{}{"type": "string"},
						},
						"required": []string{"id", "objective", "acceptance"},
					},
				},
			},
			"required": []string{"jobs"},
		},
		Options: map[string]interface{}{"temperature": 0},
	})
	if err != nil {
		return nil, err
	}
	return parseMicroJobPlan(resp.Content, cfg.MaxJobs)
}

func buildMicroJobPlannerPrompt(maxJobs int) string {
	return strings.Join(withMinimalOutputContract(
		"Role: manager-manager.",
		"Break the objective into tiny sequential terminal-verifiable jobs.",
		"Use the project run profile to choose run, build, and test jobs.",
		"Each job must be independently actionable by one command loop.",
		"Each acceptance must be checkable from command stdout, filesystem state, HTTP response, or test output.",
		"Prefer many tiny steps over broad tasks.",
		"Output JSON only.",
		"Schema: {\"jobs\":[{\"id\":\"job_1\",\"objective\":\"...\",\"acceptance\":\"...\"}]}",
		fmt.Sprintf("Return between 1 and %d jobs.", maxJobs),
	), "\n")
}

func parseMicroJobPlan(raw string, maxJobs int) ([]MicroJob, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var envelope microJobPlanEnvelope
	if err := json.Unmarshal([]byte(clean), &envelope); err != nil {
		return nil, fmt.Errorf("parse micro job plan JSON: %w", err)
	}
	if len(envelope.Jobs) == 0 {
		return nil, fmt.Errorf("micro job plan contains no jobs")
	}
	if maxJobs <= 0 {
		maxJobs = defaultMicroJobMaxJobs
	}
	if len(envelope.Jobs) > maxJobs {
		envelope.Jobs = envelope.Jobs[:maxJobs]
	}

	jobs := make([]MicroJob, 0, len(envelope.Jobs))
	for index, job := range envelope.Jobs {
		job.ID = strings.TrimSpace(job.ID)
		job.Objective = strings.TrimSpace(job.Objective)
		job.Acceptance = strings.TrimSpace(job.Acceptance)
		if job.ID == "" {
			job.ID = fmt.Sprintf("job_%d", index+1)
		}
		if job.Objective == "" {
			return nil, fmt.Errorf("micro job %s objective is empty", job.ID)
		}
		if job.Acceptance == "" {
			return nil, fmt.Errorf("micro job %s acceptance is empty", job.ID)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func buildMicroJobPrompt(overallObjective, workspacePath string, profile ProjectRunProfile, job MicroJob, completed []MicroJobResult) string {
	previous := make([]string, 0, len(completed))
	for _, item := range completed {
		previous = append(previous, fmt.Sprintf("%s done=%t exit=%d command=%q answer=%q", item.Job.ID, item.Done, item.ExitCode, item.Command, item.Answer))
	}
	return strings.Join([]string{
		"Overall objective:",
		strings.TrimSpace(overallObjective),
		"",
		"Workspace:",
		strings.TrimSpace(workspacePath),
		"",
		"Project run profile:",
		formatProjectRunProfile(profile),
		"",
		"Completed micro jobs:",
		strings.Join(previous, "\n"),
		"",
		"Current micro job:",
		job.ID,
		"",
		"Objective:",
		job.Objective,
		"",
		"Acceptance:",
		job.Acceptance,
		"",
		"Complete only this micro job. Use terminal evidence. Stop after this job is accepted.",
	}, "\n")
}
