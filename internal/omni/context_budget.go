package omni

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	structuredBudgetRolePlanner    = "planner"
	structuredBudgetRoleShell      = "shell"
	structuredBudgetRoleEvaluator  = "evaluator"
	structuredBudgetRoleCompletion = "completion"
)

type structuredContextBudgetReport struct {
	Applied            bool
	OriginalChars      int
	FinalChars         int
	ObservationsBefore int
	ObservationsAfter  int
	MemoriesBefore     int
	MemoriesAfter      int
	PrepBudgetBefore   int
	PrepBudgetAfter    int
}

func budgetStructuredPlannerContext(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe, survey WorksiteSurvey, prep PrepContextBundle) ([]Message, []SessionMemory, []StructuredCommandObservation, MinimalContext, PrepContextBundle, structuredContextBudgetReport) {
	memories = filterExecutionSessionMemories(memories, prompt, currentWorkingDirectory, len(memories))
	report := structuredContextBudgetReport{
		ObservationsBefore: len(observations),
		ObservationsAfter:  len(observations),
		MemoriesBefore:     len(memories),
		MemoriesAfter:      len(memories),
		PrepBudgetBefore:   prepContextBudget(prep),
		PrepBudgetAfter:    prepContextBudget(prep),
	}
	initial := buildStructuredCommandRequestWithContextRecipesSurveyAndPrepRaw(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, survey, prep)
	report.OriginalChars = approxOllamaRequestChars(initial)
	report.FinalChars = report.OriginalChars
	budgetChars := structuredRolePromptBudgetChars(structuredBudgetRolePlanner)
	if report.OriginalChars <= budgetChars {
		return history, memories, observations, minimalContext, prep, report
	}

	report.Applied = true
	candidates := []struct {
		observationCount int
		observationChars int
		memoryCount      int
		memoryChars      int
		historyCount     int
		historyChars     int
		prepLimit        int
		contextChars     int
	}{
		{8, 700, 10, 900, 4, 900, 6000, 1200},
		{5, 450, 6, 600, 2, 600, 3000, 800},
		{3, 280, 3, 400, 0, 0, 1500, 500},
	}
	bestHistory := history
	bestMemories := memories
	bestObservations := observations
	bestMinimalContext := minimalContext
	bestPrep := prep
	for _, candidate := range candidates {
		nextHistory := compactMessagesForStructuredContext(history, minimalContext, candidate.historyCount, candidate.historyChars)
		nextMemories := compactSessionMemoriesForStructuredContext(memories, candidate.memoryCount, candidate.memoryChars)
		nextObservations := compactStructuredObservationsForContext(observations, candidate.observationCount, candidate.observationChars)
		nextMinimalContext := compactMinimalContextForStructuredBudget(minimalContext, candidate.contextChars)
		nextPrep := CompactPrepContextBundle(prep, candidate.prepLimit)
		req := buildStructuredCommandRequestWithContextRecipesSurveyAndPrepRaw(prompt, nextHistory, nextMemories, nextObservations, currentWorkingDirectory, objectiveLedger, nextMinimalContext, recipes, survey, nextPrep)
		size := approxOllamaRequestChars(req)
		bestHistory = nextHistory
		bestMemories = nextMemories
		bestObservations = nextObservations
		bestMinimalContext = nextMinimalContext
		bestPrep = nextPrep
		report.FinalChars = size
		if size <= budgetChars {
			break
		}
	}
	report.ObservationsAfter = len(bestObservations)
	report.MemoriesAfter = len(bestMemories)
	report.PrepBudgetAfter = prepContextBudget(bestPrep)
	return bestHistory, bestMemories, bestObservations, bestMinimalContext, bestPrep, report
}

func structuredRolePromptBudgetChars(role string) int {
	switch strings.TrimSpace(role) {
	case structuredBudgetRolePlanner:
		return defaultStructuredPlannerPromptBudgetChars
	case structuredBudgetRoleShell:
		return defaultStructuredShellPromptBudgetChars
	case structuredBudgetRoleEvaluator:
		return defaultStructuredEvaluatorPromptBudgetChars
	case structuredBudgetRoleCompletion:
		return defaultStructuredCompletionPromptBudgetChars
	default:
		return defaultStructuredPlannerPromptBudgetChars
	}
}

func approxOllamaRequestChars(req OllamaChatRequest) int {
	total := len(req.ContextSystem)
	for _, message := range req.Messages {
		total += len(message.Role) + len(message.Content) + 16
	}
	if req.Format != nil {
		if blob, err := json.Marshal(req.Format); err == nil {
			total += len(blob)
		}
	}
	if req.Options != nil {
		if blob, err := json.Marshal(req.Options); err == nil {
			total += len(blob)
		}
	}
	return total
}

func compactMessagesForStructuredContext(history []Message, minimalContext MinimalContext, maxCount, maxChars int) []Message {
	if maxCount <= 0 || len(history) == 0 || minimalContextHasContent(minimalContext) {
		return nil
	}
	start := 0
	if len(history) > maxCount {
		start = len(history) - maxCount
	}
	out := make([]Message, 0, len(history[start:]))
	for _, msg := range history[start:] {
		msg.Content = truncateForStructuredContext(msg.Content, maxChars)
		out = append(out, msg)
	}
	return out
}

func compactSessionMemoriesForStructuredContext(memories []SessionMemory, maxCount, maxChars int) []SessionMemory {
	if maxCount <= 0 || len(memories) == 0 {
		return nil
	}
	start := 0
	if len(memories) > maxCount {
		start = len(memories) - maxCount
	}
	out := make([]SessionMemory, 0, minInt(maxCount, len(memories[start:])))
	for _, memory := range memories[start:] {
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		memory.Content = truncateForStructuredContext(memory.Content, maxChars)
		memory.Tags = limitStrings(cleanMemoryTags(memory.Tags), 8)
		out = append(out, memory)
	}
	return out
}

func compactStructuredObservationsForContext(observations []StructuredCommandObservation, maxCount, textChars int) []StructuredCommandObservation {
	if len(observations) == 0 {
		return nil
	}
	if maxCount <= 0 {
		maxCount = 1
	}
	pinned := StructuredCommandObservation{}
	hasPinned := false
	for i := len(observations) - 1; i >= 0; i-- {
		if _, ok := structuredRepairContextFromObservation(observations[i]); ok {
			pinned = observations[i]
			hasPinned = true
			break
		}
	}
	start := 0
	dropped := 0
	if len(observations) > maxCount {
		start = len(observations) - maxCount
		dropped = start
	}
	out := make([]StructuredCommandObservation, 0, len(observations[start:])+2)
	if dropped > 0 {
		out = append(out, StructuredCommandObservation{
			Step:   observations[0].Step,
			Stdout: fmt.Sprintf("context_compacted: omitted %d older observations; completed_actions and loop_state summarize durable progress", dropped),
		})
	}
	for _, observation := range observations[start:] {
		out = append(out, compactStructuredObservationForContext(observation, textChars))
	}
	if hasPinned && start > 0 {
		pinnedCompact := compactStructuredObservationForContext(pinned, textChars)
		alreadyPresent := false
		for _, observation := range observations[start:] {
			if observation.Step == pinned.Step &&
				observation.RejectedCommand == pinned.RejectedCommand &&
				observation.RejectedResponse == pinned.RejectedResponse &&
				observation.Stderr == pinned.Stderr {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			out = append(out, pinnedCompact)
		}
	}
	return out
}

func compactStructuredObservationForContext(observation StructuredCommandObservation, textChars int) StructuredCommandObservation {
	if textChars <= 0 {
		textChars = 400
	}
	observation.Command = truncateForStructuredContext(observation.Command, textChars)
	observation.RejectedCommand = truncateForStructuredContext(observation.RejectedCommand, textChars)
	observation.RejectedResponse = truncateForStructuredContext(observation.RejectedResponse, textChars)
	observation.EvaluationFeedback = truncateForStructuredContext(observation.EvaluationFeedback, textChars)
	observation.CapabilityMemory = truncateForStructuredContext(observation.CapabilityMemory, textChars)
	observation.Stdout = truncateForStructuredContext(observation.Stdout, textChars)
	observation.Stderr = truncateForStructuredContext(observation.Stderr, textChars)
	observation.Question = truncateForStructuredContext(observation.Question, textChars)
	observation.UserResponse = truncateForStructuredContext(observation.UserResponse, textChars)
	return observation
}

func compactMinimalContextForStructuredBudget(input MinimalContext, maxChars int) MinimalContext {
	context := normalizeMinimalContext(input)
	if maxChars <= 0 {
		return MinimalContext{}
	}
	itemLimit := maxInt(120, maxChars/6)
	context.Summary = truncateForStructuredContext(context.Summary, maxChars/3)
	context.Facts = compactStringListForStructuredContext(context.Facts, 6, itemLimit)
	context.Constraints = compactStringListForStructuredContext(context.Constraints, 6, itemLimit)
	context.OpenItems = compactStringListForStructuredContext(context.OpenItems, 6, itemLimit)
	return context
}

func compactStringListForStructuredContext(values []string, maxCount, maxChars int) []string {
	if maxCount <= 0 || len(values) == 0 {
		return nil
	}
	out := make([]string, 0, minInt(len(values), maxCount))
	for _, value := range values {
		value = truncateForStructuredContext(value, maxChars)
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, value)
		if len(out) >= maxCount {
			break
		}
	}
	return out
}

func truncateForStructuredContext(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if limit <= 0 || trimmed == "" {
		return ""
	}
	if len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit] + "\n[context truncated]"
}
