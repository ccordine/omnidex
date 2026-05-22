package omni

import (
	"encoding/json"
	"fmt"
	"strings"
)

const validatedPlaybookKind = "validated_playbook"

type ValidatedPlaybook struct {
	SchemaVersion     string   `json:"schema_version"`
	Name              string   `json:"name"`
	TaskPattern       string   `json:"task_pattern"`
	RequiredContext   []string `json:"required_context,omitempty"`
	CommandSequence   []string `json:"command_sequence"`
	ValidationSignals []string `json:"validation_signals,omitempty"`
	KnownFailures     []string `json:"known_failures,omitempty"`
	RecoverySteps     []string `json:"recovery_steps,omitempty"`
	SuccessEvidence   []string `json:"success_evidence,omitempty"`
	ObjectiveIDs      []string `json:"objective_ids,omitempty"`
	ModelProvider     string   `json:"model_provider,omitempty"`
	Duration          string   `json:"duration,omitempty"`
	Confidence        int      `json:"confidence"`
	Supersedes        string   `json:"supersedes,omitempty"`
	SupersededBy      string   `json:"superseded_by,omitempty"`
	LastSuccessfulUse string   `json:"last_successful_use,omitempty"`
	ScopePolicy       string   `json:"scope_policy"`
}

func extractValidatedPlaybook(prompt string, result CommandDecisionResult, provider string) (SessionMemory, bool) {
	if !structuredRunAccepted(result) {
		return SessionMemory{}, false
	}
	commands := successfulMutatingOrVerifyingCommands(result.Observations)
	if len(commands) < 2 {
		return SessionMemory{}, false
	}
	playbook := ValidatedPlaybook{
		SchemaVersion:     "1",
		Name:              validatedPlaybookName(prompt, result.ObjectiveLedger),
		TaskPattern:       compactTaskPattern(prompt, result.ObjectiveLedger),
		RequiredContext:   validatedPlaybookRequiredContext(result),
		CommandSequence:   commands,
		ValidationSignals: validationSignalsFromObservations(result.Observations),
		KnownFailures:     knownFailuresFromObservations(result.Observations),
		RecoverySteps:     recoveryStepsFromObservations(result.Observations),
		SuccessEvidence:   successEvidenceFromLedger(result.ObjectiveLedger),
		ObjectiveIDs:      structuredObjectiveIDs(result.ObjectiveLedger),
		ModelProvider:     strings.TrimSpace(provider),
		Duration:          result.Elapsed.String(),
		Confidence:        validatedPlaybookConfidence(result),
		LastSuccessfulUse: nowUTC(),
		ScopePolicy:       "advisory_only_reuse_accelerates_execution_but_current_objective_ledger_scope_and_validators_still_decide_every_step",
	}
	blob, err := json.MarshalIndent(playbook, "", "  ")
	if err != nil {
		return SessionMemory{}, false
	}
	tags := append([]string{
		"validated-playbook",
		"procedure-memory",
		"successful-workflow",
		"validator-approved",
	}, validatedPlaybookTags(prompt, result.ObjectiveLedger)...)
	return SessionMemory{
		Kind:      validatedPlaybookKind,
		Content:   string(blob),
		Tags:      cleanMemoryTags(tags),
		CreatedAt: nowUTC(),
	}, true
}

func structuredRunAccepted(result CommandDecisionResult) bool {
	if result.ExitCode != 0 || result.PartialProgress {
		return false
	}
	if len(result.ObjectiveLedger) == 0 {
		return hasSuccessfulCommandObservation(result.Observations)
	}
	return len(pendingStructuredObjectives(result.ObjectiveLedger)) == 0
}

func successfulMutatingOrVerifyingCommands(observations []StructuredCommandObservation) []string {
	out := []string{}
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if command == "" || obs.ExitCode != 0 {
			continue
		}
		if !structuredCommandLooksMutating(command) && !commandLooksLikeValidation(command) {
			continue
		}
		out = append(out, command)
	}
	return limitStrings(out, 12)
}

func commandLooksLikeValidation(command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	for _, marker := range []string{" test", "npm run build", "go test", "cargo test", "zig build", "make test", "pytest", "vitest", "grep -q", "test -", "curl "} {
		if strings.Contains(command, marker) || strings.HasPrefix(command, strings.TrimSpace(marker)) {
			return true
		}
	}
	return false
}

func validationSignalsFromObservations(observations []StructuredCommandObservation) []string {
	signals := []string{}
	for _, obs := range observations {
		if obs.ExitCode != 0 {
			continue
		}
		command := strings.TrimSpace(obs.Command)
		if command == "" || !commandLooksLikeValidation(command) {
			continue
		}
		signals = append(signals, compactPlaybookLine(command))
	}
	return limitStrings(signals, 8)
}

func knownFailuresFromObservations(observations []StructuredCommandObservation) []string {
	failures := []string{}
	for _, obs := range observations {
		if obs.ExitCode == 0 {
			continue
		}
		text := strings.TrimSpace(firstNonEmpty(obs.Stderr, obs.EvaluationFeedback, obs.RejectedCommand, obs.RejectedResponse))
		if text == "" {
			continue
		}
		failures = append(failures, compactPlaybookLine(text))
	}
	return limitStrings(failures, 6)
}

func recoveryStepsFromObservations(observations []StructuredCommandObservation) []string {
	steps := []string{}
	for _, obs := range observations {
		if obs.ExitCode != 0 {
			continue
		}
		command := strings.TrimSpace(obs.Command)
		if command == "" {
			continue
		}
		if len(steps) == 0 || steps[len(steps)-1] != command {
			steps = append(steps, command)
		}
	}
	return limitStrings(steps, 8)
}

func successEvidenceFromLedger(ledger []StructuredObjective) []string {
	evidence := []string{}
	for _, objective := range ledger {
		if !structuredObjectiveSatisfied(objective) {
			continue
		}
		line := strings.TrimSpace(objective.ID)
		if strings.TrimSpace(objective.Evidence) != "" {
			line += ": " + strings.TrimSpace(objective.Evidence)
		}
		evidence = append(evidence, compactPlaybookLine(line))
	}
	return limitStrings(evidence, 10)
}

func validatedPlaybookRequiredContext(result CommandDecisionResult) []string {
	context := []string{}
	if strings.TrimSpace(result.MinimalContext.Summary) != "" {
		context = append(context, compactPlaybookLine(result.MinimalContext.Summary))
	}
	for _, fact := range result.MinimalContext.Facts {
		if strings.TrimSpace(fact) != "" {
			context = append(context, compactPlaybookLine(fact))
		}
	}
	return limitStrings(context, 6)
}

func validatedPlaybookName(prompt string, ledger []StructuredObjective) string {
	tags := validatedPlaybookTags(prompt, ledger)
	if len(tags) == 0 {
		return "validated_playbook"
	}
	return strings.Join(limitStrings(tags, 5), "_")
}

func compactTaskPattern(prompt string, ledger []StructuredObjective) string {
	parts := []string{}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, compactPlaybookLine(prompt))
	}
	for _, objective := range ledger {
		if strings.TrimSpace(objective.Description) != "" {
			parts = append(parts, compactPlaybookLine(objective.Description))
		}
	}
	return strings.Join(limitStrings(parts, 6), " | ")
}

func validatedPlaybookTags(prompt string, ledger []StructuredObjective) []string {
	text := strings.ToLower(prompt + " " + strings.Join(structuredObjectiveIDs(ledger), " "))
	tags := []string{}
	for _, candidate := range []string{"react", "vite", "node", "npm", "go", "rust", "zig", "docker", "postgres", "pgsql", "cli", "crud", "notes", "calculator", "chess", "frontend", "backend", "test"} {
		if strings.Contains(text, candidate) {
			tags = append(tags, candidate)
		}
	}
	return cleanMemoryTags(tags)
}

func validatedPlaybookConfidence(result CommandDecisionResult) int {
	confidence := 70
	if len(pendingStructuredObjectives(result.ObjectiveLedger)) == 0 {
		confidence += 10
	}
	if len(validationSignalsFromObservations(result.Observations)) > 0 {
		confidence += 10
	}
	if len(knownFailuresFromObservations(result.Observations)) == 0 {
		confidence += 5
	}
	if confidence > 95 {
		confidence = 95
	}
	return confidence
}

func compactPlaybookLine(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > 260 {
		value = value[:260] + "..."
	}
	return value
}

func rememberValidatedPlaybookFromResult(session *Session, prompt string, result CommandDecisionResult, provider string) (SessionMemory, bool) {
	if session == nil {
		return SessionMemory{}, false
	}
	memory, ok := extractValidatedPlaybook(prompt, result, provider)
	if !ok {
		return SessionMemory{}, false
	}
	if sessionHasMemoryContent(session, memory.Content) {
		return SessionMemory{}, false
	}
	session.Memories = append(session.Memories, memory)
	return memory, true
}

func validatedPlaybookMemorySummary(memory SessionMemory) string {
	content := strings.TrimSpace(memory.Content)
	if content == "" {
		return ""
	}
	var playbook ValidatedPlaybook
	if err := json.Unmarshal([]byte(content), &playbook); err != nil {
		return compactPlaybookLine(content)
	}
	parts := []string{
		fmt.Sprintf("name=%s", playbook.Name),
		fmt.Sprintf("confidence=%d", playbook.Confidence),
	}
	if playbook.TaskPattern != "" {
		parts = append(parts, "task_pattern="+playbook.TaskPattern)
	}
	if len(playbook.CommandSequence) > 0 {
		parts = append(parts, "commands="+strings.Join(limitStrings(playbook.CommandSequence, 5), " -> "))
	}
	if len(playbook.ValidationSignals) > 0 {
		parts = append(parts, "validation="+strings.Join(limitStrings(playbook.ValidationSignals, 3), " | "))
	}
	parts = append(parts, "scope_policy="+playbook.ScopePolicy)
	return strings.Join(parts, "\n")
}
