package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/specialist"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type StructuredToolInventory struct {
	TerminalTools  []string                          `json:"terminal_tools"`
	Skills         []string                          `json:"skills,omitempty"`
	PublicSources  []string                          `json:"public_sources"`
	LLMRoles       []string                          `json:"llm_roles"`
	SpecialistTeam []StructuredSpecialistTeamSummary `json:"specialist_team"`
	ShellRules     []string                          `json:"shell_rules"`
}

type StructuredSpecialistTeamSummary struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Scope               string   `json:"scope"`
	Authority           string   `json:"authority"`
	AllowedTools        []string `json:"allowed_tools,omitempty"`
	CanDelegateTo       []string `json:"can_delegate_to,omitempty"`
	ContextContribution string   `json:"context_contribution"`
	MemoryPermissions   []string `json:"memory_permissions,omitempty"`
}

func buildStructuredToolInventory() StructuredToolInventory {
	return StructuredToolInventory{
		TerminalTools: discoveredTerminalTools(),
		Skills:        discoveredSkillNames(),
		PublicSources: []string{
			"wttr.in",
			"news.google.com/rss/search",
			"duckduckgo.com/html",
			"go.dev/dl/?mode=json",
		},
		LLMRoles: []string{
			"command_planner",
			"shell_execution_specialist",
			"final_responder",
			"memory_retriever",
			"memory_reviewer",
			"web_researcher",
			"workspace_researcher",
			"subtask_executor",
			"verifier",
		},
		SpecialistTeam: structuredSpecialistTeamSummary(specialist.DefaultTeam()),
		ShellRules: []string{
			"single fresh bash shell per command",
			"working directory does not persist between commands",
			"use absolute paths or cd within the same command",
			"for Thailand current time use TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'",
			"stdout stderr and exit code are observed after execution",
		},
	}
}

func structuredSpecialistTeamSummary(profiles []specialist.TeamProfile) []StructuredSpecialistTeamSummary {
	out := make([]StructuredSpecialistTeamSummary, 0, len(profiles))
	for _, profile := range profiles {
		tools := make([]string, 0, len(profile.AllowedTools))
		for _, grant := range profile.AllowedTools {
			if strings.TrimSpace(grant.Name) == "" {
				continue
			}
			tools = append(tools, grant.Name)
		}
		permissions := []string{}
		if profile.Memory.CanRead {
			permissions = append(permissions, "read")
		}
		if profile.Memory.CanCreate {
			permissions = append(permissions, "create")
		}
		if profile.Memory.CanUpdate {
			permissions = append(permissions, "update")
		}
		if profile.Memory.CanDeprioritize {
			permissions = append(permissions, "deprioritize")
		}
		out = append(out, StructuredSpecialistTeamSummary{
			ID:                  profile.Role.ID,
			Name:                profile.Role.Name,
			Scope:               profile.Role.Scope,
			Authority:           profile.Authority,
			AllowedTools:        limitStrings(tools, 12),
			CanDelegateTo:       limitStrings(profile.CanDelegateTo, 12),
			ContextContribution: profile.ContextContribution,
			MemoryPermissions:   permissions,
		})
	}
	return out
}

func discoveredTerminalTools() []string {
	candidates := []string{
		"bash", "sh", "curl", "python3", "sed", "awk", "grep", "rg", "jq", "date", "uname",
		"cat", "find", "ls", "pwd", "mkdir", "touch", "tee", "git", "go", "npm", "npx", "node",
		"docker", "ps", "pgrep", "xdg-open", "firefox", "wmctrl", "xdotool",
	}
	tools := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, tool := range candidates {
		if _, err := exec.LookPath(tool); err == nil && !seen[tool] {
			tools = append(tools, tool)
			seen[tool] = true
		}
	}
	sort.Strings(tools)
	return tools
}

func discoveredSkillNames() []string {
	root := findStructuredSkillsRoot()
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	skills := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err == nil {
			skills = append(skills, name)
		}
	}
	sort.Strings(skills)
	return skills
}

func findStructuredSkillsRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(wd, "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		next := filepath.Dir(wd)
		if next == wd {
			return ""
		}
		wd = next
	}
}

type StructuredMemoryRecord struct {
	Turn        int    `json:"turn"`
	Role        string `json:"role"`
	NotPrompt   bool   `json:"not_prompt"`
	MemoryStyle string `json:"memory_style"`
	MemoryNote  string `json:"memory_note"`
}

func recentStructuredCapabilityMemories(memories []SessionMemory) []SessionMemory {
	if len(memories) == 0 {
		return nil
	}
	start := 0
	if len(memories) > maxConversationHistoryMessages {
		start = len(memories) - maxConversationHistoryMessages
	}
	out := []SessionMemory{}
	for _, memory := range memories[start:] {
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		kind := strings.TrimSpace(memory.Kind)
		if kind == "" {
			kind = "capability"
		}
		if kind != "capability" {
			continue
		}
		out = append(out, SessionMemory{
			Kind:      kind,
			Content:   truncateStructuredObservation(memory.Content),
			Tags:      sortedCopy(memory.Tags),
			CreatedAt: memory.CreatedAt,
		})
	}
	return out
}

func recentStructuredSessionMemories(memories []SessionMemory) []SessionMemory {
	if len(memories) == 0 {
		return nil
	}
	start := 0
	if len(memories) > maxConversationHistoryMessages {
		start = len(memories) - maxConversationHistoryMessages
	}
	out := []SessionMemory{}
	for _, memory := range memories[start:] {
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		kind := strings.TrimSpace(memory.Kind)
		if kind == "" {
			kind = "episodic"
		}
		out = append(out, SessionMemory{
			Kind:      kind,
			Content:   truncateStructuredObservation(memory.Content),
			Tags:      sortedCopy(memory.Tags),
			CreatedAt: memory.CreatedAt,
		})
	}
	return out
}

func recentStructuredMemoryRecords(history []Message) []StructuredMemoryRecord {
	recent := recentStructuredConversation(history)
	if len(recent) == 0 {
		return nil
	}
	out := make([]StructuredMemoryRecord, 0, len(recent))
	for i, msg := range recent {
		content := sanitizeStructuredReferenceHistoryContent(msg.Role, msg.Content)
		if strings.TrimSpace(content) == "" {
			continue
		}
		out = append(out, StructuredMemoryRecord{
			Turn:        i + 1,
			Role:        msg.Role,
			NotPrompt:   true,
			MemoryStyle: "terse_reference_only",
			MemoryNote:  compactStructuredMemoryNote(content),
		})
	}
	return out
}

func sanitizeStructuredReferenceHistoryContent(role, content string) string {
	content = strings.TrimSpace(content)
	if content == "" || strings.TrimSpace(role) != "assistant" {
		return content
	}
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if structuredReferenceHistoryLineIsOperationalState(line) {
			continue
		}
		kept = append(kept, line)
	}
	clean := strings.TrimSpace(strings.Join(kept, "\n"))
	if clean == "" {
		return "prior assistant response omitted operational loop state"
	}
	return clean
}

func structuredReferenceHistoryLineIsOperationalState(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return false
	}
	needles := []string{
		"forbidden_commands",
		"forbidden command",
		"blocked command",
		"loop blocker",
		"last blocker",
		"anti_loop:",
		"progression_gate",
		"structured_command_loop_blocked",
		"repeated command exhausted",
		"command repeats a previous failed command",
		"pending objectives:",
		"command:",
		"last command exit code:",
		"attempts:",
		"stdout:",
		"stderr:",
		"answer:",
		"status:",
		"stopped:",
		"stopped: structured command loop exhausted",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func compactStructuredMemoryNote(content string) string {
	note := strings.Join(strings.Fields(content), " ")
	if len(note) <= 320 {
		return note
	}
	return note[:320] + " [truncated]"
}

func recentStructuredConversation(history []Message) []Message {
	if len(history) == 0 {
		return nil
	}
	start := 0
	if len(history) > maxConversationHistoryMessages {
		start = len(history) - maxConversationHistoryMessages
	}
	out := make([]Message, 0, len(history)-start)
	for _, msg := range history[start:] {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		out = append(out, Message{
			Role:      role,
			Content:   truncateStructuredObservation(content),
			CreatedAt: msg.CreatedAt,
		})
	}
	return out
}

func currentWorkingDirectoryForStructuredPrompt() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func structuredPromptWorkingDirectory(workingDirectory string) string {
	if strings.TrimSpace(workingDirectory) != "" {
		return strings.TrimSpace(workingDirectory)
	}
	return currentWorkingDirectoryForStructuredPrompt()
}

func realCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" {
			count++
		}
	}
	return count
}

func successfulCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode == 0 {
			count++
		}
	}
	return count
}

func failedCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode != 0 {
			count++
		}
	}
	return count
}

func truncateStructuredObservation(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= defaultStructuredObservationChars {
		return trimmed
	}
	return trimmed[:defaultStructuredObservationChars] + "\n[truncated]"
}

func truncateStructuredTimelineValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= 400 {
		return trimmed
	}
	return trimmed[:400] + "..."
}

func ParseStructuredCommandPayload(raw string) (StructuredCommandPayload, error) {
	var decoded struct {
		Command         *string               `json:"command"`
		Done            *bool                 `json:"done"`
		Answer          *string               `json:"answer"`
		Ask             bool                  `json:"ask"`
		Question        string                `json:"question"`
		Tool            string                `json:"tool"`
		ToolTask        string                `json:"tool_task"`
		Patch           string                `json:"patch"`
		ObjectiveLedger []StructuredObjective `json:"objective_ledger"`
		ProofPlan       StructuredProofPlan   `json:"proof_plan"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return StructuredCommandPayload{}, fmt.Errorf("parse structured command payload: %w", err)
	}
	if decoded.Command == nil || decoded.Done == nil || decoded.Answer == nil {
		return StructuredCommandPayload{}, fmt.Errorf("structured command payload missing required fields")
	}
	return StructuredCommandPayload{
		Command:         *decoded.Command,
		Done:            *decoded.Done,
		Answer:          *decoded.Answer,
		Ask:             decoded.Ask,
		Question:        decoded.Question,
		Tool:            decoded.Tool,
		ToolTask:        decoded.ToolTask,
		Patch:           decoded.Patch,
		ObjectiveLedger: mergeStructuredObjectiveLedger(nil, decoded.ObjectiveLedger),
		ProofPlan:       decoded.ProofPlan,
	}, nil
}

func ParseShellCommandProposal(raw string) (ShellCommandProposal, error) {
	var decoded struct {
		Command   string `json:"command"`
		Rationale string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ShellCommandProposal{}, fmt.Errorf("parse shell specialist response: %w", err)
	}
	command := strings.TrimSpace(decoded.Command)
	if command == "" {
		return ShellCommandProposal{}, fmt.Errorf("shell specialist response missing command")
	}
	return ShellCommandProposal{
		Command:   command,
		Rationale: strings.TrimSpace(decoded.Rationale),
	}, nil
}

func ParseCodeContentProposal(raw string) (CodeContentProposal, error) {
	var decoded struct {
		Content   *string `json:"content"`
		Rationale string  `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return CodeContentProposal{}, fmt.Errorf("parse code content specialist response: %w", err)
	}
	if decoded.Content == nil || strings.TrimSpace(*decoded.Content) == "" {
		return CodeContentProposal{}, fmt.Errorf("code content specialist response missing substantive content")
	}
	return CodeContentProposal{
		Content:   *decoded.Content,
		Rationale: strings.TrimSpace(decoded.Rationale),
	}, nil
}

func ExecuteStructuredCommand(ctx context.Context, command string, stdout, stderr io.Writer) (int, error) {
	return ExecuteStructuredCommandInDir(ctx, command, "", stdout, stderr)
}

func ExecuteStructuredCommandInDir(ctx context.Context, command, workingDirectory string, stdout, stderr io.Writer) (int, error) {
	cmd := newStructuredShellCommand(command)
	if strings.TrimSpace(workingDirectory) != "" {
		cmd.Dir = strings.TrimSpace(workingDirectory)
	}
	configureStructuredCommandProcess(cmd)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return 1, err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killStructuredCommandProcess(cmd)
		<-done
		return 1, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}
