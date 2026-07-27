package worker

import (
	"context"
	"encoding/json"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/workspace"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func detectPackageManagers() []string {
	candidates := []string{"apt-get", "apk", "dnf", "yum", "pacman", "brew", "zypper", "rpm", "dpkg-query"}
	out := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate); err == nil {
			name := candidate
			switch name {
			case "dpkg-query":
				name = "dpkg"
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}

func resolvePackageManagers(job model.Job) []string {
	out := make([]string, 0, 4)
	seen := map[string]struct{}{}
	add := func(value string) {
		name := strings.TrimSpace(strings.ToLower(value))
		if name == "" {
			return
		}
		switch name {
		case "apt":
			name = "apt-get"
		case "dpkg-query":
			name = "dpkg"
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	for _, value := range metadataCSV(job.Metadata, "host_env_package_managers") {
		add(value)
	}
	add(metadataString(job.Metadata, "host_env_package_manager"))
	for _, value := range detectPackageManagers() {
		add(value)
	}
	return out
}

func primaryPackageManager(packageManagers []string) string {
	for _, manager := range packageManagers {
		name := strings.TrimSpace(manager)
		if name != "" {
			return name
		}
	}
	return ""
}

func buildInstallHint(packageManager string, tools []string) string {
	if packageManager == "" || len(tools) == 0 {
		return ""
	}
	joined := strings.Join(tools, " ")
	switch packageManager {
	case "apt-get":
		return "apt-get update && apt-get install -y " + joined
	case "dpkg":
		return "apt-get update && apt-get install -y " + joined
	case "apk":
		return "apk add --no-cache " + joined
	case "dnf":
		return "dnf install -y " + joined
	case "yum":
		return "yum install -y " + joined
	case "zypper":
		return "zypper install -y " + joined
	case "rpm":
		return "dnf install -y " + joined
	case "pacman":
		return "pacman -Sy --noconfirm " + joined
	case "brew":
		return "brew install " + joined
	default:
		return ""
	}
}

func buildInstallHints(packageManagers []string, tools []string) []string {
	if len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(packageManagers))
	seen := map[string]struct{}{}
	for _, manager := range packageManagers {
		hint := strings.TrimSpace(buildInstallHint(manager, tools))
		if hint == "" {
			continue
		}
		if _, ok := seen[hint]; ok {
			continue
		}
		seen[hint] = struct{}{}
		out = append(out, hint)
	}
	return out
}

func buildEnvironmentSummary(packageManager string, requiredTools, availableTools, missingTools []string, workspaceSvc *workspace.Service) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	workspaceRoot := ""
	if workspaceSvc != nil {
		workspaceRoot = workspaceSvc.Root()
	}

	commonTools := []string{"sh", "bash", "touch", "cat", "tee", "sed", "awk", "vim", "nano", "git", "go", "npm", "python3", "docker", "make"}
	availableCommon := make([]string, 0, len(commonTools))
	for _, tool := range commonTools {
		if _, err := exec.LookPath(tool); err == nil {
			availableCommon = append(availableCommon, tool)
		}
	}

	return strings.TrimSpace(strings.Join([]string{
		"env_os=" + runtime.GOOS,
		"env_arch=" + runtime.GOARCH,
		"env_shell=" + safeLine(os.Getenv("SHELL"), "unknown"),
		"env_user=" + safeLine(os.Getenv("USER"), "unknown"),
		"env_cwd=" + safeLine(cwd, "unknown"),
		"env_workspace_root=" + safeLine(workspaceRoot, "(unset)"),
		"env_package_manager=" + safeLine(packageManager, "(none)"),
		"env_required_tools=" + strings.Join(requiredTools, ","),
		"env_available_tools=" + strings.Join(availableTools, ","),
		"env_missing_tools=" + strings.Join(missingTools, ","),
		"env_common_tools_available=" + strings.Join(availableCommon, ","),
	}, "\n"))
}

func buildHostEnvironmentSummaryFromMetadata(job model.Job) string {
	lines := make([]string, 0, 16)
	orderedKeys := []string{
		"host_env_os",
		"host_env_arch",
		"host_env_kernel",
		"host_env_distro",
		"host_env_shell",
		"host_env_user",
		"host_env_identity",
		"host_env_cwd",
		"host_env_package_manager",
		"host_env_package_managers",
		"host_discovery_time",
		"host_clock_local",
		"host_clock_utc",
		"host_clock_tz",
		"host_clock_weekday",
		"host_clock_epoch",
	}
	for _, key := range orderedKeys {
		value := strings.TrimSpace(metadataString(job.Metadata, key))
		if value == "" {
			continue
		}
		lines = append(lines, key+"="+safeLine(value, "unknown"))
	}

	hostTools := metadataCSV(job.Metadata, "host_tools_available")
	if len(hostTools) > 0 {
		lines = append(lines, "host_tools_available="+strings.Join(hostTools, ","))
	}
	hostPackages := metadataCSV(job.Metadata, "host_packages_installed")
	if len(hostPackages) > 0 {
		lines = append(lines, "host_packages_installed="+strings.Join(hostPackages, ","))
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func hostToolSetFromMetadata(job model.Job) map[string]struct{} {
	raw := metadataCSV(job.Metadata, "host_tools_available")
	if len(raw) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		lower := strings.ToLower(strings.TrimSpace(value))
		if lower == "" {
			continue
		}
		set[lower] = struct{}{}
	}
	return set
}

func hostToolAvailable(tool string, hostTools map[string]struct{}) bool {
	if len(hostTools) == 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(tool))
	if name == "" {
		return false
	}
	if _, ok := hostTools[name]; ok {
		return true
	}

	aliases := map[string][]string{
		"python": {"python3"},
		"node":   {"nodejs"},
		"nodejs": {"node"},
	}
	for _, alias := range aliases[name] {
		if _, ok := hostTools[alias]; ok {
			return true
		}
	}
	return false
}

func metadataCSV(metadata json.RawMessage, key string) []string {
	raw := strings.TrimSpace(metadataString(metadata, key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	return dedupeStrings(values)
}

func currentTimeContextFromMetadata(job model.Job) string {
	now := time.Now()
	values := map[string]string{
		"host_clock_local":   now.Local().Format(time.RFC3339),
		"host_clock_utc":     now.UTC().Format(time.RFC3339),
		"host_clock_tz":      safeLine(now.Location().String(), "unknown"),
		"host_clock_weekday": now.Weekday().String(),
		"host_clock_epoch":   strconv.FormatInt(now.Unix(), 10),
		"host_env_user":      safeLine(metadataString(job.Metadata, "host_env_user"), safeLine(os.Getenv("USER"), "unknown")),
		"host_env_identity":  safeLine(metadataString(job.Metadata, "host_env_identity"), "unknown"),
	}

	orderedKeys := []string{
		"host_clock_local",
		"host_clock_utc",
		"host_clock_tz",
		"host_clock_weekday",
		"host_clock_epoch",
		"host_env_user",
		"host_env_identity",
	}
	for _, key := range orderedKeys {
		value := strings.TrimSpace(metadataString(job.Metadata, key))
		if value == "" {
			continue
		}
		values[key] = safeLine(value, "unknown")
	}
	lines := make([]string, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		lines = append(lines, key+"="+safeLine(values[key], "unknown"))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func anchorTimeSensitiveQuery(query string, job model.Job) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return query
	}
	if !isTimeSensitiveInstruction(job.Instruction) {
		return query
	}
	if dateAnchorPattern.MatchString(strings.ToLower(query)) {
		return query
	}
	anchor := searchDateAnchor(job)
	if anchor == "" {
		return query
	}
	return strings.TrimSpace(query + " as of " + anchor)
}

func searchDateAnchor(job model.Job) string {
	for _, key := range []string{"host_clock_local", "host_clock_utc"} {
		raw := strings.TrimSpace(metadataString(job.Metadata, key))
		if raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.Format("2006-01-02")
		}
		if len(raw) >= 10 {
			return raw[:10]
		}
	}
	return time.Now().Format("2006-01-02")
}

func sanitizeSearchQueryArtifacts(query string) string {
	clean := strings.TrimSpace(query)
	if clean == "" {
		return ""
	}
	clean = searchPromptArtifactPattern.ReplaceAllString(clean, " ")
	clean = duplicateAsOfPattern.ReplaceAllString(clean, "as of")
	clean = strings.Join(strings.Fields(clean), " ")
	clean = strings.TrimSpace(clean)
	clean = strings.Trim(clean, "\"'`")
	return clean
}

func (s *Service) rewriteNeedInputAutonomous(
	ctx context.Context,
	stepID int64,
	job model.Job,
	contexts map[string]string,
	blockedDraft string,
	question string,
) string {
	prompt := buildAutonomousRewritePrompt(job, contexts, blockedDraft, question)

	responseFallback := s.specialistModel(job, specialist.RoleResponseSpecialist, s.models.Response)
	modelName := s.pickThinkingModel(job, contexts, metadataModel(job, "model_response", responseFallback))
	rewrite, err := s.llmGenerateWithTrace(ctx, stepID, "response_need_input_rewrite", modelName, prompt)
	if err != nil {
		return defaultAutonomousResponse(job.Instruction)
	}
	rewrite = strings.TrimSpace(rewrite)
	if rewrite == "" {
		return defaultAutonomousResponse(job.Instruction)
	}
	if _, ok := extractNeedInputQuestion(rewrite); ok {
		return defaultAutonomousResponse(job.Instruction)
	}
	return rewrite
}

func defaultAutonomousResponse(instruction string) string {
	lower := strings.ToLower(strings.TrimSpace(instruction))
	if strings.Contains(lower, "create") && (strings.Contains(lower, "file") || strings.Contains(lower, "document")) {
		return "Proceeding with sensible defaults in this environment: using `touch test` to create a file named `test`."
	}
	return "Proceeding with sensible defaults based on the current environment and available tools."
}

func buildAutonomousRewritePrompt(job model.Job, contexts map[string]string, blockedDraft, question string) string {
	sections := []string{
		antiRoleplayInstructionForPipeline(job.Pipeline),
		"Rewrite the blocked draft into a direct autonomous response for the user.",
		"Return only the final user-facing response.",
		"Do not mention rewriting, drafts, internal process, or tool policy.",
		"Do not ask follow-up questions.",
		"Use sensible defaults inferred from tooling/environment context.",
		"State assumptions briefly and continue.",
		"Do not include a Sources section.",
		promptTrustBoundaryInstruction(),
		promptUserAnchor("start", job.Instruction, ""),
	}
	if shouldIncludeFileDefaultHint(job.Instruction, question) {
		sections = append(sections,
			"If a file/document is requested but filename is missing, default to `test`.",
			"If a text editor choice is missing, prefer shell-safe defaults (`touch` + `cat`).",
		)
	}
	sections = append(sections,
		promptBlock("USER_INSTRUCTION", job.Instruction),
		promptBlock("BLOCKED_DRAFT", trimForBudget(blockedDraft, 1600)),
		promptBlock("BLOCKING_QUESTION", question),
		promptBlock("TOOLING", trimForBudget(contexts["tooling"], 1200)),
		promptBlock("ENVIRONMENT", trimForBudget(contexts["environment"], 1200)),
		promptBlock("ANALYZER", trimForBudget(contexts["analyzer"], 1200)),
		promptUserAnchor("end", job.Instruction, ""),
		"Final grounding check: produce the response for AUTHORITATIVE_USER_INSTRUCTION_END.",
	)
	return strings.Join(sections, "\n\n")
}

func shouldIncludeFileDefaultHint(instruction, question string) bool {
	combined := strings.ToLower(strings.TrimSpace(instruction + " " + question))
	if combined == "" {
		return false
	}
	tokens := []string{
		"file",
		"filename",
		"file name",
		"document",
		"doc",
		"text editor",
		"editor",
	}
	for _, token := range tokens {
		if strings.Contains(combined, token) {
			return true
		}
	}
	return false
}

func metadataValue(metadata json.RawMessage, key string) (any, bool) {
	if len(metadata) == 0 {
		return nil, false
	}

	var parsed map[string]any
	if err := json.Unmarshal(metadata, &parsed); err != nil {
		return nil, false
	}

	value, ok := parsed[key]
	return value, ok
}

func specialistRoleForJob(job model.Job, defaultRoleID string) string {
	roleID := strings.TrimSpace(metadataString(job.Metadata, "specialist_role_id"))
	if roleID != "" {
		return roleID
	}
	return strings.TrimSpace(defaultRoleID)
}

func (s *Service) specialistModel(job model.Job, defaultRoleID string, fallback string) string {
	roleID := specialistRoleForJob(job, defaultRoleID)
	if roleID != "" && s.models.Specialist != nil {
		if value, ok := s.models.Specialist[roleID]; ok {
			if clean := strings.TrimSpace(value); clean != "" {
				return clean
			}
		}
	}
	return strings.TrimSpace(fallback)
}

func (s *Service) pickThinkingModel(job model.Job, contexts map[string]string, fallback string) string {
	level := s.resolveReasoningLevel(job, contexts)
	explicitLevel := strings.ToLower(strings.TrimSpace(metadataString(job.Metadata, "reasoning_level")))
	if level == "deep" && strings.TrimSpace(s.models.Reasoning) != "" {
		return s.models.Reasoning
	}
	if level == "fast" && strings.TrimSpace(s.models.Fast) != "" {
		if explicitLevel == "fast" || explicitLevel == "light" || explicitLevel == "low" || strings.TrimSpace(fallback) == "" {
			return s.models.Fast
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return s.models.Default
}

func (s *Service) resolveReasoningLevel(job model.Job, contexts map[string]string) string {
	raw := strings.ToLower(strings.TrimSpace(metadataString(job.Metadata, "reasoning_level")))
	switch raw {
	case "deep", "complex", "high":
		return "deep"
	case "fast", "light", "low":
		return "fast"
	}

	if s.isComplexTask(job.Instruction, contexts) {
		return "deep"
	}
	return "fast"
}

func (s *Service) isComplexTask(instruction string, contexts map[string]string) bool {
	normalized := strings.ToLower(strings.TrimSpace(instruction))
	if len(normalized) > 260 {
		return true
	}
	if complexityKeywordPattern.MatchString(normalized) {
		return true
	}

	totalContext := len(strings.TrimSpace(contexts["retrieval"])) + len(strings.TrimSpace(contexts["web_search"]))
	return totalContext > s.contextBudget
}

func shouldRetrySameModelAfterCreateEOF(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(err.Error()))
	if value == "" {
		return false
	}
	return strings.Contains(value, "ollama create failed") && strings.Contains(value, "eof")
}

func hasSufficientRetrievedContext(retrieval string, minChars int) bool {
	clean := strings.TrimSpace(retrieval)
	if clean == "" {
		return false
	}

	normalized := strings.ToLower(clean)
	if strings.Contains(normalized, "no relevant memory found") {
		return false
	}

	if len(clean) >= minChars {
		return true
	}

	return len(bracketedMatchPattern.FindAllString(clean, -1)) >= 2
}

type inferredMemory struct {
	Procedural  []string `json:"procedural"`
	Instruction []string `json:"instruction"`
	Preference  []string `json:"preference"`
}

func memoryCandidateStatusForInference(kind string, confidence float64, groundedInInstruction bool, autopromote bool) string {
	if !autopromote {
		return model.MemoryCandidateStatusCandidate
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case model.MemoryKindPreference:
		if groundedInInstruction && confidence >= 0.9 {
			return model.MemoryCandidateStatusApproved
		}
	}
	return model.MemoryCandidateStatusCandidate
}
