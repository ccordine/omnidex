package omni

import (
	"encoding/json"
	"strings"
)

func buildStructuredCommandMessages(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe, surveys ...WorksiteSurvey) []OllamaMessage {
	survey := WorksiteSurvey{}
	if len(surveys) > 0 {
		survey = surveys[0]
	}
	return buildStructuredCommandMessagesWithPrep(prompt, history, memories, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, survey, PrepContextBundle{})
}

func buildStructuredCommandMessagesWithPrep(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation, currentWorkingDirectory string, objectiveLedger []StructuredObjective, minimalContext MinimalContext, recipes []Recipe, survey WorksiteSurvey, prep PrepContextBundle) []OllamaMessage {
	memories = filterExecutionSessionMemories(memories, prompt, currentWorkingDirectory, len(memories))
	messages := []OllamaMessage{}
	if memoryMessage := buildStructuredCommandCapabilityMemoryMessage(memories); memoryMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: memoryMessage},
			OllamaMessage{Role: "assistant", Content: "Capability memory received. I will use it only to avoid repeating false capability limitations."},
		)
	}
	if contextMessage := buildStructuredMinimalContextMessage(minimalContext); contextMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: contextMessage},
			OllamaMessage{Role: "assistant", Content: "Minimal context inventory received. I will use only these relevant facts unless tool evidence adds more."},
		)
	} else if historyMessage := buildStructuredCommandHistoryMessage(history); historyMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: historyMessage},
			OllamaMessage{Role: "assistant", Content: "Reference history received. I will use it only when the active task needs omitted context."},
		)
	}
	if prepMessage := buildStructuredPrepContextMessage(memories); prepMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: prepMessage},
			OllamaMessage{Role: "assistant", Content: "Prep context received. I will use it as compact advisory routing and documentation context only where it directly helps the active task."},
		)
	}
	if prepMessage := buildStructuredPrepContextBundleMessage(prep); prepMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: prepMessage},
			OllamaMessage{Role: "assistant", Content: "Prep bundle received. I will use only routed, provenance-backed briefs for the role and objective that need them."},
		)
	}
	messages = append(messages, OllamaMessage{Role: "user", Content: buildStructuredCommandUserMessage(prompt, observations, currentWorkingDirectory, objectiveLedger, minimalContext, recipes, survey)})
	if len(observations) > 0 {
		if repair, ok := structuredRepairContextFromObservation(observations[len(observations)-1]); ok {
			messages = append(messages, buildStructuredPlannerRepairFollowUpMessages(repair)...)
		}
	}
	return messages
}

func buildStructuredMinimalContextMessage(minimalContext MinimalContext) string {
	context := normalizeMinimalContext(minimalContext)
	if context.Summary == "" && len(context.Facts) == 0 && len(context.Constraints) == 0 && len(context.OpenItems) == 0 {
		return ""
	}
	payload := struct {
		MinimalContext MinimalContext `json:"minimal_context"`
	}{
		MinimalContext: context,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredPrepContextMessage(memories []SessionMemory) string {
	prep := compactStructuredPrepMemories(memories, 8)
	if len(prep) == 0 {
		return ""
	}
	payload := struct {
		PrepContext []SessionMemory `json:"prep_context"`
		Rules       []string        `json:"rules"`
	}{
		PrepContext: prep,
		Rules: []string{
			"Prep context is advisory and scoped to the current task.",
			"Use codebase_route_brief for likely files/tests and documentation_brief for API/convention guidance.",
			"Use web_research_brief only for freshness or external facts required by the task.",
			"Do not let prep context add unrequested dependencies, frameworks, services, or architecture.",
			"Prefer the smallest subset of prep context needed for the next action.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredPrepContextBundleMessage(bundle PrepContextBundle) string {
	if len(allPrepBriefs(bundle)) == 0 && len(bundle.Evidence) == 0 {
		return ""
	}
	compact := CompactPrepContextBundle(bundle, defaultPrepContextBudgetLimit)
	payload := struct {
		PrepBundle PrepContextBundle `json:"prep_context_bundle"`
		Rules      []string          `json:"rules"`
	}{
		PrepBundle: compact,
		Rules: []string{
			"Prep bundle is evidence-led, budgeted, routed, and validated.",
			"Use only briefs whose used_by includes your role or directly supports the active objective.",
			"Do not treat memory, documentation, or web research as execution permission.",
			"When documentation_brief already covers the requested language/toolchain, use it for project structure and examples instead of fetching the same docs again.",
			"If the active objective is to build/create an app in an empty workspace, the next planner action should write source/build/test files, not merely describe that the workspace is empty.",
			"Do not claim completion from prep context; completion requires validator evidence.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredCommandCapabilityMemoryMessage(memories []SessionMemory) string {
	recent := recentStructuredCapabilityMemories(memories)
	if len(recent) == 0 {
		return ""
	}
	payload := struct {
		CapabilityMemory []SessionMemory `json:"capability_memory"`
	}{
		CapabilityMemory: recent,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func compactStructuredPrepMemories(memories []SessionMemory, limit int) []SessionMemory {
	if limit <= 0 {
		limit = 8
	}
	allowed := map[string]bool{
		"codebase_route_brief":   true,
		"documentation_brief":    true,
		"web_research_brief":     true,
		"expertise_research":     true,
		"documentation_research": true,
		validatedPlaybookKind:    true,
	}
	out := []SessionMemory{}
	for i := len(memories) - 1; i >= 0; i-- {
		memory := memories[i]
		if !allowed[strings.TrimSpace(memory.Kind)] {
			continue
		}
		content := strings.TrimSpace(memory.Content)
		if content == "" {
			continue
		}
		if strings.TrimSpace(memory.Kind) == validatedPlaybookKind {
			content = validatedPlaybookMemorySummary(memory)
		}
		if len(content) > 1800 {
			content = content[:1800] + "\n...[truncated]"
		}
		memory.Content = content
		memory.Tags = limitStrings(cleanMemoryTags(memory.Tags), 10)
		out = append(out, memory)
		if len(out) >= limit {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func buildStructuredCommandHistoryMessage(history []Message) string {
	recent := recentStructuredMemoryRecords(history)
	if len(recent) == 0 {
		return ""
	}
	payload := struct {
		ReferenceHistory []StructuredMemoryRecord `json:"reference_history"`
	}{
		ReferenceHistory: recent,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredCommandUserMessage(prompt string, observations []StructuredCommandObservation, args ...interface{}) string {
	workingDirectory := ""
	objectiveLedger := []StructuredObjective(nil)
	if len(args) > 0 {
		if value, ok := args[0].(string); ok {
			workingDirectory = value
		}
	}
	if len(args) > 1 {
		if value, ok := args[1].([]StructuredObjective); ok {
			objectiveLedger = value
		}
	}
	minimalContext := MinimalContext{}
	if len(args) > 2 {
		if value, ok := args[2].(MinimalContext); ok {
			minimalContext = normalizeMinimalContext(value)
		}
	}
	recipes := []Recipe(nil)
	if len(args) > 3 {
		if value, ok := args[3].([]Recipe); ok {
			recipes = value
		}
	}
	worksiteSurvey := WorksiteSurvey{}
	if len(args) > 4 {
		if value, ok := args[4].(WorksiteSurvey); ok {
			worksiteSurvey = value
		}
	}
	if worksiteSurvey.TaskMode == "" {
		worksiteSurvey.TaskMode = inferTaskMode(prompt, worksiteSurvey)
	}
	payload := struct {
		ActivePromptOpen string                  `json:"active_prompt_open"`
		ToolInventory    StructuredToolInventory `json:"tool_inventory"`
		ActiveTask       struct {
			CurrentPrompt               string                         `json:"current_prompt"`
			Prompt                      string                         `json:"prompt"`
			TaskMode                    TaskMode                       `json:"task_mode,omitempty"`
			CurrentWorkingDirectory     string                         `json:"current_working_directory"`
			WorksiteSurvey              WorksiteSurvey                 `json:"worksite_survey"`
			RuntimeStateLifetime        StructuredRuntimeStateLifetime `json:"runtime_state_lifetime"`
			MinimalContext              MinimalContext                 `json:"minimal_context,omitempty"`
			Recipes                     []RecipeRuntimeConstraint      `json:"recipes,omitempty"`
			ObjectiveLedger             []StructuredObjective          `json:"objective_ledger,omitempty"`
			WorkItems                   []ObjectiveWorkItem            `json:"work_items,omitempty"`
			CurrentWorkItem             *ObjectiveWorkItem             `json:"current_work_item,omitempty"`
			ProjectFileMap              ProjectFileMap                 `json:"project_file_map,omitempty"`
			ProjectMapPolicy            []string                       `json:"project_map_policy,omitempty"`
			ChildJobs                   []ChildJob                     `json:"child_jobs,omitempty"`
			CurrentChildJob             *ChildJob                      `json:"current_child_job,omitempty"`
			ChildJobNextAction          *ChildJobAction                `json:"child_job_next_action,omitempty"`
			CompletedActions            []CompletedAction              `json:"completed_actions,omitempty"`
			LoopState                   StructuredLoopState            `json:"loop_state,omitempty"`
			ForbiddenCommands           []string                       `json:"forbidden_commands,omitempty"`
			RecoveryInstruction         string                         `json:"recovery_instruction,omitempty"`
			LatestRejectionFeedback     string                         `json:"latest_rejection_feedback,omitempty"`
			RejectedCommandPreview      string                         `json:"rejected_command_preview,omitempty"`
			RejectedResponsePreview     string                         `json:"rejected_response_preview,omitempty"`
			RejectionRepairGuidance     string                         `json:"rejection_repair_guidance,omitempty"`
			DevelopmentLoop             []string                       `json:"development_loop,omitempty"`
			ProofPolicy                 []string                       `json:"proof_policy,omitempty"`
			ProofPlanAllowedSources     []string                       `json:"proof_plan_allowed_sources,omitempty"`
			ProofLifecycle              []string                       `json:"proof_lifecycle,omitempty"`
			TaskRoute                   TaskRoute                      `json:"task_route,omitempty"`
			PendingObjectiveIDs         []string                       `json:"pending_objective_ids,omitempty"`
			MustReturnCommand           bool                           `json:"must_return_command"`
			RealCommandObservationCount int                            `json:"real_command_observation_count"`
			SuccessfulCommandCount      int                            `json:"successful_command_count"`
			FailedCommandCount          int                            `json:"failed_command_count"`
			AttemptBudgetRemaining      int                            `json:"attempt_budget_remaining"`
			Observations                []StructuredCommandObservation `json:"observations"`
		} `json:"active_task"`
		ActivePromptClose string `json:"active_prompt_close"`
	}{}
	payload.ActivePromptOpen = prompt
	payload.ToolInventory = buildStructuredToolInventory()
	payload.ActiveTask.CurrentPrompt = prompt
	payload.ActiveTask.Prompt = prompt
	payload.ActiveTask.TaskMode = worksiteSurvey.TaskMode
	payload.ActiveTask.CurrentWorkingDirectory = structuredPromptWorkingDirectory(workingDirectory)
	payload.ActiveTask.WorksiteSurvey = worksiteSurvey
	payload.ActiveTask.RuntimeStateLifetime = structuredRuntimeStateLifetime()
	payload.ActiveTask.MinimalContext = minimalContext
	payload.ActiveTask.Recipes = recipeRuntimeConstraints(recipes)
	payload.ActiveTask.ObjectiveLedger = mergeStructuredObjectiveLedger(nil, objectiveLedger)
	payload.ActiveTask.WorkItems = ReconcileObjectiveWorkItemsFromObservations(BuildObjectiveWorkItemsFromLedger(prompt, payload.ActiveTask.ObjectiveLedger, workingDirectory, worksiteSurvey), observations)
	payload.ActiveTask.CurrentWorkItem = firstPendingObjectiveWorkItem(payload.ActiveTask.WorkItems)
	var latest *StructuredCommandObservation
	if len(observations) > 0 {
		latest = &observations[len(observations)-1]
	}
	successReconciliation := RunSuccessReconciliation(SuccessReconciliationInput{
		LatestObservation: latest,
		ObjectiveLedger:   payload.ActiveTask.ObjectiveLedger,
		WorkQueue:         payload.ActiveTask.WorkItems,
		ChildJobs:         BuildChildJobsFromObjectiveWorkItems(payload.ActiveTask.WorkItems),
		WorkingDirectory:  workingDirectory,
		Observations:      observations,
	})
	payload.ActiveTask.ObjectiveLedger = successReconciliation.ObjectiveLedger
	payload.ActiveTask.WorkItems = successReconciliation.WorkQueue
	payload.ActiveTask.ProjectFileMap = activeProjectFileMapFromResult(prompt, mapDrivenArchitectToolTask(workingDirectory, worksiteSurvey), workingDirectory, worksiteSurvey, observations)
	payload.ActiveTask.ProjectMapPolicy = projectFileMapPolicyLines()
	payload.ActiveTask.ChildJobs = successReconciliation.ChildJobs
	payload.ActiveTask.CurrentChildJob = successReconciliation.NextRequiredChildJob
	payload.ActiveTask.ChildJobNextAction = successReconciliation.NextAction
	payload.ActiveTask.CompletedActions = completedActionsFromState(payload.ActiveTask.ObjectiveLedger, observations)
	payload.ActiveTask.LoopState = structuredLoopStateFromState(payload.ActiveTask.ObjectiveLedger, observations)
	payload.ActiveTask.ForbiddenCommands = payload.ActiveTask.LoopState.ForbiddenCommands
	payload.ActiveTask.RecoveryInstruction = payload.ActiveTask.LoopState.Instruction
	if len(observations) > 0 {
		if repair, ok := structuredRepairContextFromObservation(observations[len(observations)-1]); ok {
			payload.ActiveTask.LatestRejectionFeedback = repair.Feedback
			payload.ActiveTask.RejectedCommandPreview = repair.RejectedCommand
			payload.ActiveTask.RejectedResponsePreview = repair.RejectedResponse
			payload.ActiveTask.RejectionRepairGuidance = repair.Guidance
		}
	}
	payload.ActiveTask.DevelopmentLoop = structuredDevelopmentLoopPolicy()
	payload.ActiveTask.ProofPolicy = structuredProofPolicy()
	payload.ActiveTask.ProofPlanAllowedSources = defaultStructuredProofPlanAllowedSources()
	payload.ActiveTask.ProofLifecycle = structuredProofPlanLifecycle()
	if route, ok := LoadCodebaseTaskRoute(payload.ActiveTask.CurrentWorkingDirectory, prompt); ok {
		payload.ActiveTask.TaskRoute = route
	}
	payload.ActiveTask.PendingObjectiveIDs = structuredObjectiveIDs(pendingStructuredObjectives(payload.ActiveTask.ObjectiveLedger))
	payload.ActiveTask.MustReturnCommand = !hasRealCommandObservation(observations)
	payload.ActiveTask.RealCommandObservationCount = realCommandObservationCount(observations)
	payload.ActiveTask.SuccessfulCommandCount = successfulCommandObservationCount(observations)
	payload.ActiveTask.FailedCommandCount = failedCommandObservationCount(observations)
	payload.ActiveTask.AttemptBudgetRemaining = maxInt(0, defaultCommandDecisionMaxSteps-len(observations))
	payload.ActiveTask.Observations = observations
	payload.ActivePromptClose = prompt
	blob, err := json.Marshal(payload)
	if err != nil {
		return prompt
	}
	return string(blob)
}
