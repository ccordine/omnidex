package omni

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func buildPromptInterpreterRequest(input PromptInterpretationInput) OllamaChatRequest {
	payload := struct {
		Role                    string                   `json:"role"`
		UserPrompt              string                   `json:"user_prompt"`
		ReferenceHistory        []StructuredMemoryRecord `json:"reference_history,omitempty"`
		CurrentWorkingDirectory string                   `json:"current_working_directory"`
		WorksiteSurvey          WorksiteSurvey           `json:"worksite_survey"`
		AvailableRecipes        []RecipePromptCandidate  `json:"available_recipes,omitempty"`
		Instructions            []string                 `json:"instructions"`
	}{
		Role:                    "prompt_interpreter",
		UserPrompt:              input.UserPrompt,
		ReferenceHistory:        compactPromptInterpreterHistory(input.History, 3),
		CurrentWorkingDirectory: input.CurrentWorkingDirectory,
		WorksiteSurvey:          compactPromptInterpreterSurvey(input.WorksiteSurvey),
		AvailableRecipes:        compactRecipePromptCandidates(input.Recipes, 4),
		Instructions: []string{
			"Interpret the user's words into durable task objectives for downstream planners.",
			"Classify user_operation as create_new_project, modify_existing_project, fix_existing_project, inspect_existing_project, run_tests, install_deps, or unknown.",
			"The WorksiteSurvey is authoritative filesystem grounding; do not contradict its project_state or evidence.",
			"If WorksiteSurvey project_state is an existing app and the current prompt refers to this/current/existing project, prefer modify_existing_project over create_new_project.",
			"Do not create create-new objectives when user_operation is modify_existing_project or fix_existing_project.",
			"If an available recipe directly matches the task, return its id in selected_recipe_ids.",
			"Return objectives only when the request has concrete criteria, outputs, constraints, or verification needs.",
			"Use stable snake_case ids.",
			"Return the objectives in the objective_ledger JSON field.",
			"Every objective_ledger item must include kind=read|create|update|delete|verify|architect; use architect for code/app/project implementation that needs a nested per-file work queue.",
			"Set objective source to user_explicit only for requirements directly stated in the current user prompt.",
			"Set objective source to evidence_required_prerequisite only when command/workspace evidence proves the user-explicit objective cannot be completed without that prerequisite; include parent_objective and evidence.",
			"Set objective source to memory_suggested for preferences or prior-history items that are not explicitly requested now.",
			"Set objective source to model_inferred for any plausible but unsupported expansion.",
			"Use packages only for dependency package names directly justified by that objective.",
			"Set requires_reference_history=true only when the current user prompt is an unresolved follow-up that needs prior omitted entities, paths, locations, preferences, or evidence.",
			"Set requires_reference_history=false when the current prompt is standalone or provides its own concrete task, even if reference history contains similar prior work.",
			"All initial objectives should normally be pending.",
			"Do not choose shell commands.",
			"Do not answer the user.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"prompt_interpreter"}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are the prompt interpreter specialist for Omni.",
					"Your only job is translating the user's natural-language request into structured objectives.",
					"Downstream command planners must use your objective ledger instead of interpreting user wording themselves.",
					"Return one compact JSON object only. No markdown. No prose.",
					"Keep objective descriptions short and concrete.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"objective_ledger": structuredObjectiveLedgerSchema(),
				"requires_reference_history": map[string]interface{}{
					"type": "boolean",
				},
				"selected_recipe_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"user_operation": map[string]interface{}{
					"type": "string",
					"enum": []string{userOperationCreateNewProject, userOperationModifyExisting, userOperationFixExisting, userOperationInspectExisting, userOperationRunTests, userOperationInstallDeps, userOperationUnknown},
				},
				"recommended_recipe_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"forbidden_recipe_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
			},
			"required": []string{"objective_ledger", "requires_reference_history"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 384,
		},
	}
}

func buildContextSummarizerRequest(input MinimalContextInput) OllamaChatRequest {
	payload := struct {
		Role                    string                   `json:"role"`
		UserPrompt              string                   `json:"user_prompt"`
		CurrentWorkingDirectory string                   `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective    `json:"objective_ledger,omitempty"`
		CompletedActions        []CompletedAction        `json:"completed_actions,omitempty"`
		ReferenceHistory        []StructuredMemoryRecord `json:"reference_history,omitempty"`
		SessionMemories         []SessionMemory          `json:"session_memories,omitempty"`
		ExistingContext         MinimalContext           `json:"existing_context,omitempty"`
		WorksiteSurvey          WorksiteSurvey           `json:"worksite_survey"`
		Instructions            []string                 `json:"instructions"`
	}{
		Role:                    "summary_specialist",
		UserPrompt:              input.UserPrompt,
		CurrentWorkingDirectory: input.CurrentWorkingDirectory,
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, input.ObjectiveLedger),
		CompletedActions:        input.CompletedActions,
		ReferenceHistory:        recentStructuredMemoryRecords(input.History),
		SessionMemories:         recentStructuredSessionMemories(input.SessionMemories),
		ExistingContext:         normalizeMinimalContext(input.ExistingContext),
		WorksiteSurvey:          input.WorksiteSurvey,
		Instructions: []string{
			"Load the smallest context inventory needed for this active task.",
			"The WorksiteSurvey is authoritative workspace grounding.",
			"Keep only facts, constraints, and open items relevant to the objective ledger and current prompt.",
			"Treat completed_actions as authoritative progress already accomplished in this turn; do not move completed work back into open_items.",
			"Never carry prior project dependencies, frameworks, package names, or build requirements into a new standalone task.",
			"Memories may not create requirements, dependencies, frameworks, files, services, architecture, or deployment targets unless the current prompt explicitly asks to apply them.",
			"Discard unrelated transcript detail.",
			"Return empty arrays when no context is needed.",
			"Do not choose shell commands.",
			"Do not answer the user.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"summary_specialist"}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are the summary specialist for Omni.",
					"You maintain a mutable minimal context inventory for downstream models.",
					"Your output replaces raw history unless downstream code explicitly needs the raw record.",
					"Return JSON only.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: minimalContextSchema(),
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 512,
		},
	}
}

func buildCompletionCheckerRequest(input CompletionCheckInput) OllamaChatRequest {
	payload := struct {
		Role                    string                         `json:"role"`
		UserPrompt              string                         `json:"user_prompt"`
		CurrentWorkingDirectory string                         `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective          `json:"objective_ledger,omitempty"`
		CompletedActions        []CompletedAction              `json:"completed_actions,omitempty"`
		LoopState               StructuredLoopState            `json:"loop_state,omitempty"`
		MinimalContext          MinimalContext                 `json:"minimal_context,omitempty"`
		Observations            []StructuredCommandObservation `json:"observations"`
		CandidateAnswer         string                         `json:"candidate_answer"`
		Instructions            []string                       `json:"instructions"`
	}{
		Role:                    "done_check_specialist",
		UserPrompt:              input.UserPrompt,
		CurrentWorkingDirectory: input.CurrentWorkingDirectory,
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, input.ObjectiveLedger),
		CompletedActions:        input.CompletedActions,
		LoopState:               input.LoopState,
		MinimalContext:          normalizeMinimalContext(input.MinimalContext),
		Observations:            compactStructuredObservationsForContext(input.Observations, 10, 750),
		CandidateAnswer:         input.CandidateAnswer,
		Instructions: []string{
			"Decide whether the task is already complete from objective ledger, minimal context, observations, and candidate answer.",
			"Treat completed_actions as authoritative evidence of work already completed; never require the same completed action again.",
			"Treat loop_state as authoritative loop-monitor context; if it shows blocked or stuck progress, explain which pending objective still lacks evidence.",
			"Mark objectives satisfied only when observations or explicit evidence prove them.",
			"Do not require memory_suggested or model_inferred extras for completion.",
			"Memories are advisory context only and cannot create completion requirements unless represented by user_explicit, recipe_required, or detected_project objectives.",
			"Do not choose shell commands.",
			"Do not answer the user.",
			"Return updated objective_ledger and a concise reason.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"done_check_specialist"}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are the done-check specialist for Omni.",
					"Your only job is deciding whether the current task is already complete.",
					"You update objective ledger statuses from observed evidence.",
					"Return JSON only.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"done":             map[string]interface{}{"type": "boolean"},
				"reason":           map[string]interface{}{"type": "string"},
				"objective_ledger": structuredObjectiveLedgerSchema(),
			},
			"required": []string{"done", "reason", "objective_ledger"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 512,
		},
	}
}

func ParseCompletionCheck(raw string) (CompletionCheck, error) {
	var decoded struct {
		Done            bool                  `json:"done"`
		Reason          string                `json:"reason"`
		ObjectiveLedger []StructuredObjective `json:"objective_ledger"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return CompletionCheck{}, fmt.Errorf("parse completion check: %w", err)
	}
	return CompletionCheck{
		Done:            decoded.Done,
		Reason:          strings.TrimSpace(decoded.Reason),
		ObjectiveLedger: mergeStructuredObjectiveLedger(nil, decoded.ObjectiveLedger),
	}, nil
}

func minimalContextSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary":     map[string]interface{}{"type": "string"},
			"facts":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"constraints": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"open_items":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		},
		"required": []string{"summary", "facts", "constraints", "open_items"},
	}
}

func ParseMinimalContext(raw string) (MinimalContext, error) {
	var decoded MinimalContext
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return MinimalContext{}, fmt.Errorf("parse minimal context: %w", err)
	}
	return normalizeMinimalContext(decoded), nil
}

func normalizeMinimalContext(input MinimalContext) MinimalContext {
	return MinimalContext{
		Summary:     truncateMinimalContextValue(input.Summary),
		Facts:       cleanMinimalContextList(input.Facts),
		Constraints: cleanMinimalContextList(input.Constraints),
		OpenItems:   cleanMinimalContextList(input.OpenItems),
	}
}

func cleanMinimalContextList(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		clean := truncateMinimalContextValue(value)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func truncateMinimalContextValue(value string) string {
	clean := strings.Join(strings.Fields(value), " ")
	if len(clean) <= 500 {
		return clean
	}
	return clean[:500] + " [truncated]"
}

func ParsePromptInterpretation(raw string) (PromptInterpretation, error) {
	var decoded struct {
		ObjectiveLedger          []StructuredObjective `json:"objective_ledger"`
		RecipeIDs                []string              `json:"selected_recipe_ids"`
		RequiresReferenceHistory bool                  `json:"requires_reference_history"`
		UserOperation            string                `json:"user_operation"`
		RecommendedRecipeIDs     []string              `json:"recommended_recipe_ids"`
		ForbiddenRecipeIDs       []string              `json:"forbidden_recipe_ids"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		repaired, repairErr := repairPromptInterpretationJSON(raw)
		if repairErr != nil {
			return PromptInterpretation{}, fmt.Errorf("parse prompt interpretation: %w", err)
		}
		if err := json.Unmarshal([]byte(repaired), &decoded); err != nil {
			return PromptInterpretation{}, fmt.Errorf("parse prompt interpretation: %w", err)
		}
	}
	for i := range decoded.ObjectiveLedger {
		if strings.TrimSpace(decoded.ObjectiveLedger[i].Source) == "" {
			decoded.ObjectiveLedger[i].Source = structuredObjectiveSourceUserExplicit
		}
		if !decoded.ObjectiveLedger[i].Required {
			decoded.ObjectiveLedger[i].Required = true
		}
	}
	return PromptInterpretation{
		ObjectiveLedger:          mergeStructuredObjectiveLedger(nil, decoded.ObjectiveLedger),
		RecipeIDs:                cleanStringList(decoded.RecipeIDs),
		RequiresReferenceHistory: decoded.RequiresReferenceHistory,
		UserOperation:            normalizeUserOperation(decoded.UserOperation),
		RecommendedRecipeIDs:     cleanStringList(decoded.RecommendedRecipeIDs),
		ForbiddenRecipeIDs:       cleanStringList(decoded.ForbiddenRecipeIDs),
	}, nil
}

func repairPromptInterpretationJSON(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", fmt.Errorf("empty prompt interpretation")
	}
	start := strings.Index(text, "{")
	if start < 0 {
		return "", fmt.Errorf("missing object start")
	}
	text = text[start:]
	if end := strings.LastIndex(text, "}"); end >= 0 {
		candidate := text[:end+1]
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
		text = candidate
	}
	if !strings.Contains(text, `"objective_ledger"`) {
		return "", fmt.Errorf("missing objective ledger")
	}
	text = trimPromptInterpretationToLastCompleteObjective(text)
	openObjects := strings.Count(text, "{") - strings.Count(text, "}")
	openArrays := strings.Count(text, "[") - strings.Count(text, "]")
	var b strings.Builder
	b.WriteString(text)
	for i := 0; i < openArrays; i++ {
		b.WriteString("]")
	}
	for i := 0; i < openObjects; i++ {
		b.WriteString("}")
	}
	return b.String(), nil
}

func trimPromptInterpretationToLastCompleteObjective(text string) string {
	ledgerIdx := strings.Index(text, `"objective_ledger"`)
	if ledgerIdx < 0 {
		return text
	}
	ledgerStart := strings.Index(text[ledgerIdx:], "[")
	if ledgerStart < 0 {
		return text
	}
	ledgerStart += ledgerIdx
	lastComplete := strings.LastIndex(text, "}")
	if lastComplete <= ledgerStart {
		return text
	}
	if strings.Count(text[:lastComplete+1], "{") > strings.Count(text[:lastComplete+1], "}") {
		return text[:lastComplete+1]
	}
	return text
}

func compactPromptInterpreterHistory(history []Message, limit int) []StructuredMemoryRecord {
	records := recentStructuredMemoryRecords(history)
	if limit <= 0 || len(records) <= limit {
		return records
	}
	return records[len(records)-limit:]
}

func compactPromptInterpreterSurvey(survey WorksiteSurvey) WorksiteSurvey {
	if len(survey.Evidence) > 4 {
		survey.Evidence = survey.Evidence[:4]
	}
	if len(survey.Frameworks) > 4 {
		survey.Frameworks = survey.Frameworks[:4]
	}
	if len(survey.Manifests) > 4 {
		survey.Manifests = survey.Manifests[:4]
	}
	return survey
}

func compactRecipePromptCandidates(recipes []Recipe, limit int) []RecipePromptCandidate {
	candidates := recipePromptCandidates(recipes)
	if limit <= 0 || len(candidates) <= limit {
		return candidates
	}
	return candidates[:limit]
}

func cleanStringList(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func ParseStructuredLLMEvaluation(raw string) (StructuredLLMEvaluation, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var decoded map[string]interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return StructuredLLMEvaluation{}, fmt.Errorf("parse structured response evaluation: %w", err)
	}
	confidence, err := parseStructuredEvaluationConfidence(decoded["confidence"])
	if err != nil {
		return StructuredLLMEvaluation{}, err
	}
	feedback, _ := decoded["feedback"].(string)
	verdict, _ := decoded["verdict"].(string)
	blockingReason, _ := decoded["blocking_reason"].(string)
	verdict = normalizeStructuredEvaluationVerdict(verdict)
	if verdict == "accept" && structuredEvaluationFeedbackSuggestsHardReject(feedback+" "+blockingReason) {
		verdict = "reject"
	}
	return StructuredLLMEvaluation{
		Verdict:        verdict,
		Confidence:     confidence,
		BlockingReason: strings.TrimSpace(blockingReason),
		Feedback:       strings.TrimSpace(feedback),
	}, nil
}

func normalizeStructuredEvaluationVerdict(verdict string) string {
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case "reject", "revise":
		return strings.ToLower(strings.TrimSpace(verdict))
	default:
		return "accept"
	}
}

func structuredEvaluationFeedbackSuggestsHardReject(feedback string) bool {
	lower := strings.ToLower(feedback)
	for _, marker := range []string{
		"does not align",
		"not align",
		"scope drift",
		"scope_drift",
		"semantic mismatch",
		"contradicts worksite",
		"wrong project",
		"create a new project",
		"create a new react project",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func parseStructuredEvaluationConfidence(raw interface{}) (int, error) {
	switch value := raw.(type) {
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return validateStructuredEvaluationConfidence(int(parsed))
		}
		floatValue, err := strconv.ParseFloat(value.String(), 64)
		if err != nil {
			return 0, fmt.Errorf("structured response evaluation confidence is not numeric")
		}
		return validateStructuredEvaluationConfidence(int(floatValue))
	case float64:
		return validateStructuredEvaluationConfidence(int(value))
	case int:
		return validateStructuredEvaluationConfidence(value)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("structured response evaluation confidence is not numeric")
		}
		return validateStructuredEvaluationConfidence(parsed)
	default:
		return 0, fmt.Errorf("structured response evaluation missing confidence")
	}
}

func validateStructuredEvaluationConfidence(value int) (int, error) {
	if value < 0 || value > 100 {
		return 0, fmt.Errorf("structured response evaluation confidence out of range")
	}
	return value, nil
}

func validateStructuredEvaluationConsistency(evaluation StructuredLLMEvaluation) error {
	if evaluation.Confidence >= defaultEvaluatorThreshold {
		return nil
	}
	if structuredEvaluationFeedbackClaimsSuccess(evaluation.Feedback) {
		return fmt.Errorf("low confidence contradicts positive feedback")
	}
	return nil
}

func structuredEvaluationFeedbackClaimsSuccess(feedback string) bool {
	lower := strings.ToLower(feedback)
	if strings.Contains(lower, "not on track") || strings.Contains(lower, "off track") {
		return false
	}
	for _, phrase := range []string{
		"on track",
		"successfully completed",
		"correctly answered",
		"answered correctly",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func repeatedStructuredEvaluationFeedback(evaluation StructuredLLMEvaluation, observations []StructuredCommandObservation) bool {
	feedback := strings.TrimSpace(evaluation.Feedback)
	if feedback == "" {
		return false
	}
	for _, obs := range observations {
		if strings.TrimSpace(obs.EvaluationFeedback) == feedback {
			return true
		}
	}
	return false
}

const structuredRealtimeCapabilityMemory = "Omni can use shell commands and public unauthenticated sources to gather current facts. For location-specific time, use TZ=Area/City date or another evidence command; do not claim no real-time access when command evidence can be gathered."
const structuredWeatherCapabilityMemory = "Omni can gather current weather with public no-key wttr.in using an explicit location path and concise format query; do not use OpenWeatherMap, api.openweathermap.org, YOUR_API_KEY, or other API-key services without real observed credentials."

func structuredCapabilityMemoryForRejectedResponse(response, feedback string) string {
	if structuredTextSuggestsScopeDrift(response) || structuredTextSuggestsScopeDrift(feedback) {
		return structuredScopeCapabilityMemory
	}
	if structuredTextSuggestsKeyedWeatherSource(response) || structuredTextSuggestsKeyedWeatherSource(feedback) {
		return structuredWeatherCapabilityMemory
	}
	if structuredTextSuggestsFalseCapabilityLimit(response) || structuredTextSuggestsFalseCapabilityLimit(feedback) {
		return structuredRealtimeCapabilityMemory
	}
	return ""
}

func structuredTextSuggestsScopeDrift(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "dependency scope drift") || strings.Contains(lower, "unrequested package")
}

func structuredTextSuggestsKeyedWeatherSource(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "openweathermap") ||
		strings.Contains(lower, "api.openweathermap.org") ||
		strings.Contains(lower, "your_api_key") ||
		strings.Contains(lower, "api_key_here")
}

func structuredTextSuggestsFalseCapabilityLimit(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"as an ai",
		"i am unable",
		"i'm unable",
		"i cannot",
		"i can't",
		"i do not have access",
		"i don't have access",
		"do not have access to real-time",
		"don't have access to real-time",
		"cannot access real-time",
		"can't access real-time",
		"no access to real-time",
		"do not have internet access",
		"don't have internet access",
		"no internet access",
		"cannot browse",
		"can't browse",
		"unable to browse",
		"not able to browse",
		"check a weather website",
		"check the current time",
		"time zone app",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func structuredTextDefersEvidenceToFutureCommand(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"can be identified by running",
		"can be determined by running",
		"can be found by running",
		"can be checked by running",
		"run the command",
		"using the uname",
		"using uname",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func previousUserResponseForQuestion(observations []StructuredCommandObservation, question string) (string, bool) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", false
	}
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Question) == question && strings.TrimSpace(observations[i].UserResponse) != "" {
			return observations[i].UserResponse, true
		}
	}
	return "", false
}

func isTransientStructuredLLMError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "model runner has unexpectedly stopped") ||
		strings.Contains(text, "ollama returned status 500") ||
		strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "connection reset by peer")
}

func classifyStructuredLLMFailure(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "model runner has unexpectedly stopped") ||
		strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "connection reset by peer") {
		return "ollama_model_runner_crash_or_restart"
	}
	if strings.Contains(text, "ollama returned status 500") {
		return "ollama_internal_error"
	}
	if strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "client.timeout") {
		return "ollama_request_timeout"
	}
	return "ollama_request_failure"
}
