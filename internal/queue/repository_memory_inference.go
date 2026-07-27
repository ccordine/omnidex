package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func inferMemoryCategories(kind, content string, tags []string) []string {
	out := []string{}
	add := func(category string) {
		category = normalizeMemoryCategory(category)
		if category == "" {
			return
		}
		for _, existing := range out {
			if existing == category {
				return
			}
		}
		out = append(out, category)
	}

	switch normalizeMemoryKind(kind) {
	case model.MemoryKindProcedural:
		add("strategy")
	case model.MemoryKindReference:
		add("reference")
	case model.MemoryKindPreference:
		add("preference")
		add("personal")
	case model.MemoryKindInstruction:
		add("instruction")
	}

	for _, tag := range cleanTags(tags) {
		if strings.HasPrefix(tag, "category:") {
			add(strings.TrimPrefix(tag, "category:"))
			continue
		}
		switch {
		case strings.HasPrefix(tag, "project:"):
			add("project")
		case strings.HasPrefix(tag, "session:"), strings.HasPrefix(tag, "channel:"):
			add("personal")
		case strings.HasPrefix(tag, "provider:"):
			add("integration")
		case strings.HasPrefix(tag, "query:"), tag == "research", tag == "web_search":
			add("research")
		case tag == "success-playbook", tag == "learned-skill":
			add("strategy")
		case isLanguageMemoryMarker(tag):
			add("language")
		case isDatabaseMemoryMarker(tag):
			add("database")
		case isInfrastructureMemoryMarker(tag):
			add("infrastructure")
		case isFrontendMemoryMarker(tag):
			add("frontend")
		}
	}

	text := strings.ToLower(content)
	if containsAnyText(text, "user prefers", "i prefer", "my name", "remember that i", "personal") {
		add("personal")
	}
	if containsAnyText(text, "project", "repository", "workspace", "schema", "codebase") {
		add("project")
	}
	if containsAnyText(text, "rust", "golang", "go lang", "javascript", "typescript", "php", "python", "zig", "react", "node", "vite") {
		add("language")
	}
	if containsAnyText(text, "postgres", "postgresql", "pgsql", "sql", "table", "column", "migration") {
		add("database")
	}
	if containsAnyText(text, "docker", "container", "compose", "kubernetes", "ollama", "gpu", "vulkan") {
		add("infrastructure")
	}
	if containsAnyText(text, "react", "vite", "frontend", "ui", "component", "css") {
		add("frontend")
	}
	if containsAnyText(text, "api", "endpoint", "http", "openai", "anthropic", "google", "hugging face") {
		add("integration")
	}
	if containsAnyText(text, "test", "verified", "verification", "build passed", "go test", "cargo test", "npm test") {
		add("verification")
	}
	if containsAnyText(text, "error", "failed", "blocker", "recovery", "fix", "troubleshoot") {
		add("troubleshooting")
	}
	if len(out) == 0 {
		add("general")
	}
	return out
}

func memoryCategoryTags(categories []string) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		if normalized := normalizeMemoryCategory(category); normalized != "" {
			out = append(out, "category:"+normalized)
		}
	}
	return out
}

func memoryCategoryFilters(tags []string) []string {
	out := []string{}
	for _, tag := range cleanTags(tags) {
		if strings.HasPrefix(tag, "category:") {
			if category := normalizeMemoryCategory(strings.TrimPrefix(tag, "category:")); category != "" {
				out = appendCleanTags(out, category)
			}
		}
	}
	return out
}

func normalizeMemoryCategory(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	category = strings.TrimPrefix(category, "category:")
	category = strings.ReplaceAll(category, "_", "-")
	category = strings.ReplaceAll(category, " ", "-")
	switch category {
	case "personal", "person", "user":
		return "personal"
	case "project", "codebase", "workspace", "repo", "repository":
		return "project"
	case "language", "languages", "programming-language":
		return "language"
	case "database", "db", "sql", "pgsql", "postgres", "postgresql":
		return "database"
	case "infrastructure", "infra", "docker", "container", "deployment", "devops":
		return "infrastructure"
	case "frontend", "ui", "react", "vite":
		return "frontend"
	case "integration", "api", "provider", "model-provider":
		return "integration"
	case "strategy", "procedural", "playbook", "skill":
		return "strategy"
	case "reference", "research", "documentation", "docs":
		return "research"
	case "preference", "instruction", "verification", "troubleshooting", "security", "general":
		return category
	default:
		if category == "" || len(category) > 40 {
			return ""
		}
		return category
	}
}

func appendCleanTags(base []string, values ...string) []string {
	out := append([]string(nil), base...)
	seen := map[string]struct{}{}
	for _, existing := range cleanTags(out) {
		seen[existing] = struct{}{}
	}
	for _, value := range cleanTags(values) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isLanguageMemoryMarker(tag string) bool {
	switch tag {
	case "go", "golang", "rust", "javascript", "typescript", "php", "python", "zig", "node", "nodejs", "react", "vite":
		return true
	default:
		return false
	}
}

func isDatabaseMemoryMarker(tag string) bool {
	switch tag {
	case "postgres", "postgresql", "pgsql", "sql", "database", "db", "schema", "migration":
		return true
	default:
		return false
	}
}

func isInfrastructureMemoryMarker(tag string) bool {
	switch tag {
	case "docker", "compose", "container", "containers", "kubernetes", "ollama", "vulkan", "gpu", "deployment":
		return true
	default:
		return false
	}
}

func isFrontendMemoryMarker(tag string) bool {
	switch tag {
	case "react", "vite", "frontend", "ui", "css", "component", "components":
		return true
	default:
		return false
	}
}

var ErrUnsupportedPipeline = errors.New("unsupported job pipeline")

func normalizePipeline(pipeline string) string {
	return strings.ToLower(strings.TrimSpace(pipeline))
}

func validatePipeline(pipeline string) (string, error) {
	normalized := normalizePipeline(pipeline)
	switch normalized {
	case model.PipelineAssistant,
		model.PipelineChat,
		model.PipelineCoding,
		model.PipelineStory,
		model.PipelineDataQuery,
		model.PipelineDataExplore,
		model.PipelineProjectDebugger,
		model.PipelineScrumCardLLM,
		model.PipelineScrum:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnsupportedPipeline, normalized)
	}
}

func isDataSourceQueryJob(metadataJSON []byte) bool {
	if len(metadataJSON) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(stringFromMetadata(payload["source"])) == "omni-data-source"
}

func stringFromMetadata(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func stepsForPipeline(pipeline string) []stepSeed {
	switch normalizePipeline(pipeline) {
	case model.PipelineAssistant:
		return []stepSeed{
			{action: "tooling", sortIndex: 5},
			{action: "workspace_scan", sortIndex: 6},
			{action: "tag", sortIndex: 7},
			{action: "retrieve", sortIndex: 8},
			{action: "plan", sortIndex: 20},
			{action: "web_search", sortIndex: 30},
			{action: "analyze", sortIndex: 40},
			{action: "assist", sortIndex: 50},
			{action: "verify", sortIndex: 60},
		}
	case model.PipelineCoding:
		return []stepSeed{
			{action: "coding_workflow", sortIndex: 5},
		}
	case model.PipelineChat:
		return []stepSeed{
			{action: "tooling", sortIndex: 5},
			{action: "workspace_scan", sortIndex: 6},
			{action: "tag", sortIndex: 7},
			{action: "retrieve", sortIndex: 8},
			{action: "plan", sortIndex: 20},
			{action: "web_search", sortIndex: 30},
			{action: "analyze", sortIndex: 40},
			{action: "roleplay", sortIndex: 50},
			{action: "verify", sortIndex: 60},
		}
	case model.PipelineDataQuery:
		return []stepSeed{
			{action: "data_source_query", sortIndex: 1},
		}
	case model.PipelineDataExplore:
		return []stepSeed{
			{action: "data_source_explore", sortIndex: 1},
		}
	case model.PipelineStory:
		return []stepSeed{
			{action: "tooling", sortIndex: 5},
			{action: "workspace_scan", sortIndex: 6},
			{action: "tag", sortIndex: 7},
			{action: "retrieve", sortIndex: 8},
			{action: "plan", sortIndex: 20},
			{action: "web_search", sortIndex: 30},
			{action: "analyze", sortIndex: 40},
			{action: "narrate", sortIndex: 50},
			{action: "verify", sortIndex: 60},
		}
	case model.PipelineProjectDebugger:
		return []stepSeed{{action: "project_debugger", sortIndex: 1}}
	case model.PipelineScrumCardLLM:
		return []stepSeed{{action: "scrum_card_llm", sortIndex: 1}}
	default:
		return nil
	}
}

func cleanTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func normalizeMemoryKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case model.MemoryKindProcedural:
		return model.MemoryKindProcedural
	case model.MemoryKindInstruction:
		return model.MemoryKindInstruction
	case model.MemoryKindPreference:
		return model.MemoryKindPreference
	case model.MemoryKindReference:
		return model.MemoryKindReference
	default:
		return model.MemoryKindEpisodic
	}
}

func vectorLiteral(values []float64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%f", value))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func stringOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func scanJob(row pgx.Row) (model.Job, error) {
	var job model.Job
	var result, errText *string
	if err := row.Scan(
		&job.ID,
		&job.Instruction,
		&job.Pipeline,
		&job.Status,
		&result,
		&errText,
		&job.Metadata,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	); err != nil {
		return model.Job{}, err
	}
	job.Result = stringOrEmpty(result)
	job.Error = stringOrEmpty(errText)
	if len(job.Metadata) == 0 {
		job.Metadata = []byte(`{}`)
	}
	return job, nil
}

func scanStep(row pgx.Row) (model.Step, error) {
	var step model.Step
	var workerID, output, errText *string
	if err := row.Scan(
		&step.ID,
		&step.JobID,
		&step.Action,
		&step.SortIndex,
		&step.Status,
		&workerID,
		&output,
		&errText,
		&step.StartedAt,
		&step.FinishedAt,
		&step.CreatedAt,
		&step.UpdatedAt,
	); err != nil {
		return model.Step{}, err
	}
	step.WorkerID = stringOrEmpty(workerID)
	step.Output = stringOrEmpty(output)
	step.Error = stringOrEmpty(errText)
	return step, nil
}

func scanStepContext(row pgx.Row) (model.StepContext, error) {
	var ctxValue model.StepContext
	if err := row.Scan(
		&ctxValue.ID,
		&ctxValue.StepID,
		&ctxValue.Key,
		&ctxValue.Value,
		&ctxValue.CreatedAt,
	); err != nil {
		return model.StepContext{}, err
	}
	return ctxValue, nil
}

func scanClaim(row pgx.Row) (model.Step, model.Job, error) {
	var step model.Step
	var job model.Job
	var stepWorker, stepOutput, stepError *string
	var jobResult, jobError *string
	if err := row.Scan(
		&step.ID,
		&step.JobID,
		&step.Action,
		&step.SortIndex,
		&step.Status,
		&stepWorker,
		&stepOutput,
		&stepError,
		&step.StartedAt,
		&step.FinishedAt,
		&step.CreatedAt,
		&step.UpdatedAt,
		&job.ID,
		&job.Instruction,
		&job.Pipeline,
		&job.Status,
		&jobResult,
		&jobError,
		&job.Metadata,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	); err != nil {
		return model.Step{}, model.Job{}, err
	}
	step.WorkerID = stringOrEmpty(stepWorker)
	step.Output = stringOrEmpty(stepOutput)
	step.Error = stringOrEmpty(stepError)
	job.Result = stringOrEmpty(jobResult)
	job.Error = stringOrEmpty(jobError)
	if len(job.Metadata) == 0 {
		job.Metadata = []byte(`{}`)
	}
	return step, job, nil
}
