package odn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	defaultManagerMaxWorkers          = 4
	defaultManagerPlanTimeout         = 45 * time.Second
	defaultManagerReduceTimeout       = 45 * time.Second
	defaultManagerReduceContextBudget = 12000
)

type ManagerWorkerConfig struct {
	MaxWorkers          int
	PlanTimeout         time.Duration
	ReduceTimeout       time.Duration
	ReduceContextBudget int
	WorkerConfig        AgentCommandLoopConfig
}

type ManagerWorkerResult struct {
	Summary          string
	Tasks            []WorkerTask
	Workers          []WorkerResult
	Events           []Event
	Executed         int
	Blocked          int
	Failed           int
	Done             bool
	ReducedByLLM     bool
	CompactedContext bool
}

type WorkerTask struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Objective  string `json:"objective"`
	Acceptance string `json:"acceptance"`
}

type WorkerResult struct {
	Task   WorkerTask
	Result AgentCommandLoopResult
}

type managerPlanEnvelope struct {
	Tasks []WorkerTask `json:"tasks"`
}

func ExecuteManagerWorkerJob(ctx context.Context, session *Session, objective string, mode PermissionMode, in io.Reader, out io.Writer, client *OllamaClient, nextEventID func() string, runLogger *RunLogger) (ManagerWorkerResult, error) {
	return ExecuteManagerWorkerJobWithConfig(ctx, session, objective, mode, in, out, client, nextEventID, runLogger, DefaultManagerWorkerConfig())
}

func DefaultManagerWorkerConfig() ManagerWorkerConfig {
	return ManagerWorkerConfig{
		MaxWorkers:          defaultManagerMaxWorkers,
		PlanTimeout:         defaultManagerPlanTimeout,
		ReduceTimeout:       defaultManagerReduceTimeout,
		ReduceContextBudget: defaultManagerReduceContextBudget,
		WorkerConfig:        DefaultAgentCommandLoopConfig(),
	}
}

func ExecuteManagerWorkerJobWithConfig(ctx context.Context, session *Session, objective string, mode PermissionMode, in io.Reader, out io.Writer, client *OllamaClient, nextEventID func() string, runLogger *RunLogger, cfg ManagerWorkerConfig) (ManagerWorkerResult, error) {
	cfg = normalizeManagerWorkerConfig(cfg)
	result := ManagerWorkerResult{Events: make([]Event, 0, 64)}
	if client == nil {
		result.Blocked++
		result.Summary = "Manager blocked: no model is configured. Start without --no-ollama or set an Ollama endpoint/model."
		result.Events = append(result.Events, Event{
			ID:        nextEventID(),
			Type:      "manager_blocked",
			Summary:   "No model client configured for manager planning",
			Details:   map[string]string{"reason": "ollama_unavailable"},
			CreatedAt: nowUTC(),
		})
		return result, nil
	}

	tasks, planRaw, err := planWorkerTasks(ctx, client, session.WorkspacePath, objective, cfg)
	if err != nil {
		result.Failed++
		result.Summary = fmt.Sprintf("Manager failed during planning: %v", err)
		result.Events = append(result.Events, Event{
			ID:        nextEventID(),
			Type:      "manager_plan_failed",
			Summary:   "Manager could not produce a worker plan",
			Details:   map[string]string{"error": err.Error()},
			CreatedAt: nowUTC(),
		})
		return result, nil
	}
	result.Tasks = tasks
	result.Events = append(result.Events, Event{
		ID:      nextEventID(),
		Type:    "manager_plan_created",
		Summary: "Manager created worker plan",
		Details: map[string]string{
			"task_count": fmt.Sprintf("%d", len(tasks)),
			"raw_plan":   truncateOutput(planRaw),
		},
		CreatedAt: nowUTC(),
	})
	_ = runLogger.Log("manager", "plan_created", map[string]interface{}{"objective": objective, "raw_plan": planRaw, "task_count": len(tasks)})

	for _, task := range tasks {
		workerObjective := buildWorkerObjective(objective, task)
		workerResult, workerErr := ExecuteAgentCommandLoopWithConfig(ctx, session, workerObjective, mode, in, out, client, nextEventID, runLogger, cfg.WorkerConfig)
		if workerErr != nil {
			workerResult.FailedCount++
			workerResult.Summary = fmt.Sprintf("Worker %s failed: %v", task.ID, workerErr)
		}
		result.Workers = append(result.Workers, WorkerResult{Task: task, Result: workerResult})
		result.Events = append(result.Events, Event{
			ID:      nextEventID(),
			Type:    "worker_completed",
			Summary: "Worker completed assigned task",
			Details: map[string]string{
				"worker_id": task.ID,
				"role":      task.Role,
				"executed":  fmt.Sprintf("%d", workerResult.ExecutedCount),
				"blocked":   fmt.Sprintf("%d", workerResult.BlockedCount),
				"failed":    fmt.Sprintf("%d", workerResult.FailedCount),
				"done":      fmt.Sprintf("%t", workerResult.Done),
			},
			CreatedAt: nowUTC(),
		})

		result.Executed += workerResult.ExecutedCount
		result.Blocked += workerResult.BlockedCount
		result.Failed += workerResult.FailedCount
	}

	summary, reducedByLLM, compactedContext := reduceWorkerResults(ctx, client, objective, result.Workers, cfg)
	result.Summary = summary
	result.ReducedByLLM = reducedByLLM
	result.CompactedContext = compactedContext
	result.Done = result.Executed > 0 && result.Failed == 0
	result.Events = append(result.Events, Event{
		ID:      nextEventID(),
		Type:    "manager_reduced",
		Summary: "Manager reduced worker transcripts",
		Details: map[string]string{
			"reduced_by_llm": fmt.Sprintf("%t", reducedByLLM),
			"executed":       fmt.Sprintf("%d", result.Executed),
			"blocked":        fmt.Sprintf("%d", result.Blocked),
			"failed":         fmt.Sprintf("%d", result.Failed),
			"done":           fmt.Sprintf("%t", result.Done),
			"compacted":      fmt.Sprintf("%t", compactedContext),
		},
		CreatedAt: nowUTC(),
	})
	return result, nil
}

func normalizeManagerWorkerConfig(cfg ManagerWorkerConfig) ManagerWorkerConfig {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = defaultManagerMaxWorkers
	}
	if cfg.PlanTimeout <= 0 {
		cfg.PlanTimeout = defaultManagerPlanTimeout
	}
	if cfg.ReduceTimeout <= 0 {
		cfg.ReduceTimeout = defaultManagerReduceTimeout
	}
	if cfg.ReduceContextBudget <= 0 {
		cfg.ReduceContextBudget = defaultManagerReduceContextBudget
	}
	cfg.WorkerConfig = normalizeAgentCommandLoopConfig(cfg.WorkerConfig)
	return cfg
}

func planWorkerTasks(ctx context.Context, client *OllamaClient, workspacePath, objective string, cfg ManagerWorkerConfig) ([]WorkerTask, string, error) {
	planCtx, cancel := context.WithTimeout(ctx, cfg.PlanTimeout)
	defer cancel()
	resp, err := client.ChatRaw(planCtx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: buildManagerPlannerSystemPrompt(cfg.MaxWorkers)},
			{Role: "user", Content: fmt.Sprintf("Workspace: %s\nObjective:\n%s", workspacePath, strings.TrimSpace(objective))},
		},
		Options: map[string]interface{}{"temperature": 0, "num_predict": 600},
	})
	if err != nil {
		return nil, "", err
	}

	tasks, err := parseManagerPlan(resp.Content, cfg.MaxWorkers)
	if err != nil {
		return nil, resp.Content, err
	}
	return tasks, resp.Content, nil
}

func buildManagerPlannerSystemPrompt(maxWorkers int) string {
	return strings.Join(withMinimalOutputContract(
		"Role: manager.",
		"Split objective into bounded worker tasks.",
		"Tasks must be doable via terminal + observed output.",
		"Output JSON only. No markdown. No prose.",
		"Schema: {\"tasks\":[{\"id\":\"worker_1\",\"role\":\"workspace_researcher|web_researcher|coding_worker|test_worker|verifier\",\"objective\":\"...\",\"acceptance\":\"...\"}]}",
		fmt.Sprintf("Return between 1 and %d tasks.", maxWorkers),
		"Prefer fewer tasks.",
	), "\n")
}

func parseManagerPlan(raw string, maxWorkers int) ([]WorkerTask, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var envelope managerPlanEnvelope
	decoder := json.NewDecoder(strings.NewReader(clean))
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("parse manager plan JSON: %w", err)
	}
	if len(envelope.Tasks) == 0 {
		return nil, fmt.Errorf("manager plan contains no tasks")
	}
	if maxWorkers <= 0 {
		maxWorkers = defaultManagerMaxWorkers
	}
	if len(envelope.Tasks) > maxWorkers {
		envelope.Tasks = envelope.Tasks[:maxWorkers]
	}

	tasks := make([]WorkerTask, 0, len(envelope.Tasks))
	for index, task := range envelope.Tasks {
		task.ID = strings.TrimSpace(task.ID)
		task.Role = strings.TrimSpace(task.Role)
		task.Objective = strings.TrimSpace(task.Objective)
		task.Acceptance = strings.TrimSpace(task.Acceptance)
		if task.ID == "" {
			task.ID = fmt.Sprintf("worker_%d", index+1)
		}
		if task.Role == "" {
			task.Role = "worker"
		}
		if task.Objective == "" {
			return nil, fmt.Errorf("task %s has empty objective", task.ID)
		}
		if task.Acceptance == "" {
			task.Acceptance = "Return a DONE line grounded in observed command output."
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func buildWorkerObjective(overallObjective string, task WorkerTask) string {
	return strings.Join([]string{
		"Overall manager objective:",
		strings.TrimSpace(overallObjective),
		"",
		"Assigned worker:",
		task.ID,
		"",
		"Role:",
		task.Role,
		"",
		"Worker objective:",
		task.Objective,
		"",
		"Acceptance criteria:",
		task.Acceptance,
		"",
		"Use commands. Finish DONE only from observed output.",
	}, "\n")
}

func reduceWorkerResults(ctx context.Context, client *OllamaClient, objective string, workers []WorkerResult, cfg ManagerWorkerConfig) (string, bool, bool) {
	grounding, compacted := buildWorkerReductionGrounding(objective, workers, cfg)
	reduceCtx, cancel := context.WithTimeout(ctx, cfg.ReduceTimeout)
	defer cancel()

	resp, err := client.ChatRaw(reduceCtx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: MinimalOutputContract + " Role: reducer. Use only transcript facts. Missing evidence: say MISSING: <item>. Max 5 bullets. If input says CAVE MAN SUMMARY, treat it as the full bounded evidence set and do not invent omitted details."},
			{Role: "user", Content: grounding},
		},
		Options: map[string]interface{}{"temperature": 0, "num_predict": 400},
	})
	if err != nil {
		return deterministicWorkerSummary(objective, workers), false, compacted
	}
	return strings.TrimSpace(resp.Content), true, compacted
}

func buildWorkerReductionGrounding(objective string, workers []WorkerResult, cfg ManagerWorkerConfig) (string, bool) {
	budget := cfg.ReduceContextBudget
	if budget <= 0 {
		budget = defaultManagerReduceContextBudget
	}
	grounding := buildWorkerGrounding(objective, workers)
	if len(grounding) <= budget {
		return grounding, false
	}
	return buildCaveManWorkerGrounding(objective, workers, budget), true
}

func buildCaveManWorkerGrounding(objective string, workers []WorkerResult, budget int) string {
	if budget <= 0 {
		budget = defaultManagerReduceContextBudget
	}
	perObservationChars := 800
	if budget/4 < perObservationChars {
		perObservationChars = budget / 4
	}
	if perObservationChars < 160 {
		perObservationChars = 160
	}

	var b strings.Builder
	if !appendCaveManLine(&b, budget, "CAVE MAN SUMMARY. SOURCE IS WORKER TRANSCRIPTS. USE ONLY THESE FACTS.") {
		return b.String()
	}
	if !appendCaveManLine(&b, budget, "Objective: "+compactCaveManText(objective, 600)) {
		return b.String()
	}
	for _, worker := range workers {
		if !appendCaveManLine(&b, budget, fmt.Sprintf("Worker %s role=%s done=%t executed=%d blocked=%d failed=%d", worker.Task.ID, worker.Task.Role, worker.Result.Done, worker.Result.ExecutedCount, worker.Result.BlockedCount, worker.Result.FailedCount)) {
			return b.String()
		}
		if strings.TrimSpace(worker.Task.Objective) != "" {
			if !appendCaveManLine(&b, budget, "  objective: "+compactCaveManText(worker.Task.Objective, 320)) {
				return b.String()
			}
		}
		if strings.TrimSpace(worker.Result.Summary) != "" {
			if !appendCaveManLine(&b, budget, "  worker_summary: "+compactCaveManText(worker.Result.Summary, 320)) {
				return b.String()
			}
		}
		for _, obs := range worker.Result.Transcript {
			if !appendCaveManLine(&b, budget, fmt.Sprintf("  step=%d status=%s command=%q", obs.Step, obs.Status, compactCaveManText(obs.Command, 220))) {
				return b.String()
			}
			if strings.TrimSpace(obs.Stdout) != "" {
				if !appendCaveManLine(&b, budget, "    stdout facts: "+compactCaveManText(obs.Stdout, perObservationChars)) {
					return b.String()
				}
			}
			if strings.TrimSpace(obs.Stderr) != "" {
				if !appendCaveManLine(&b, budget, "    stderr facts: "+compactCaveManText(obs.Stderr, perObservationChars)) {
					return b.String()
				}
			}
			if strings.TrimSpace(obs.Error) != "" {
				if !appendCaveManLine(&b, budget, "    error: "+compactCaveManText(obs.Error, perObservationChars)) {
					return b.String()
				}
			}
		}
	}
	return b.String()
}

func buildWorkerGrounding(objective string, workers []WorkerResult) string {
	var b strings.Builder
	b.WriteString("Objective:\n")
	b.WriteString(strings.TrimSpace(objective))
	b.WriteString("\n\nWorker transcripts:\n")
	for _, worker := range workers {
		b.WriteString(fmt.Sprintf("\n[%s] role=%s objective=%s\n", worker.Task.ID, worker.Task.Role, worker.Task.Objective))
		b.WriteString(fmt.Sprintf("summary=%s executed=%d blocked=%d failed=%d done=%t\n", worker.Result.Summary, worker.Result.ExecutedCount, worker.Result.BlockedCount, worker.Result.FailedCount, worker.Result.Done))
		for _, obs := range worker.Result.Transcript {
			b.WriteString(fmt.Sprintf("- step=%d status=%s command=%q\n", obs.Step, obs.Status, obs.Command))
			if strings.TrimSpace(obs.Stdout) != "" {
				b.WriteString("  stdout:\n")
				b.WriteString(indentBlock(obs.Stdout))
			}
			if strings.TrimSpace(obs.Stderr) != "" {
				b.WriteString("  stderr:\n")
				b.WriteString(indentBlock(obs.Stderr))
			}
			if strings.TrimSpace(obs.Error) != "" {
				b.WriteString("  error:\n")
				b.WriteString(indentBlock(obs.Error))
			}
		}
	}
	return b.String()
}

func compactCaveManText(raw string, maxChars int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	marker := " ... [middle omitted] ... "
	if maxChars <= len(marker)+20 {
		return text[:maxChars]
	}
	head := (maxChars - len(marker)) / 2
	tail := maxChars - len(marker) - head
	return text[:head] + marker + text[len(text)-tail:]
}

func appendCaveManLine(b *strings.Builder, budget int, line string) bool {
	if budget <= 0 {
		b.WriteString(line)
		b.WriteByte('\n')
		return true
	}
	if b.Len() >= budget {
		return false
	}
	remaining := budget - b.Len()
	if len(line)+1 <= remaining {
		b.WriteString(line)
		b.WriteByte('\n')
		return true
	}
	marker := " ... [truncated to avoid context overflow]"
	if remaining <= len(marker)+1 {
		return false
	}
	limit := remaining - len(marker) - 1
	if limit <= 0 {
		return false
	}
	if limit > len(line) {
		limit = len(line)
	}
	b.WriteString(line[:limit])
	b.WriteString(marker)
	b.WriteByte('\n')
	return false
}

func deterministicWorkerSummary(objective string, workers []WorkerResult) string {
	var b strings.Builder
	b.WriteString("Manager completed worker run for objective: ")
	b.WriteString(strings.TrimSpace(objective))
	for _, worker := range workers {
		b.WriteString(fmt.Sprintf("\n- %s (%s): executed=%d blocked=%d failed=%d done=%t", worker.Task.ID, worker.Task.Role, worker.Result.ExecutedCount, worker.Result.BlockedCount, worker.Result.FailedCount, worker.Result.Done))
		if len(worker.Result.Transcript) > 0 {
			last := worker.Result.Transcript[len(worker.Result.Transcript)-1]
			b.WriteString(fmt.Sprintf("; last_command=%q", last.Command))
			if strings.TrimSpace(last.Stdout) != "" {
				b.WriteString("; stdout=")
				b.WriteString(truncateOutput(last.Stdout))
			}
		}
	}
	return b.String()
}

func indentBlock(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	var b bytes.Buffer
	for _, line := range lines {
		b.WriteString("    ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
