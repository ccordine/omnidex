package omni

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func buildStructuredCommandResponseFormat(observations []StructuredCommandObservation) map[string]interface{} {
	properties := map[string]interface{}{
		"command":          map[string]interface{}{"type": "string"},
		"done":             map[string]interface{}{"type": "boolean"},
		"answer":           map[string]interface{}{"type": "string"},
		"ask":              map[string]interface{}{"type": "boolean"},
		"question":         map[string]interface{}{"type": "string"},
		"tool":             map[string]interface{}{"type": "string"},
		"patch":            map[string]interface{}{"type": "string"},
		"objective_ledger": structuredObjectiveLedgerSchema(),
		"proof_plan":       structuredProofPlanSchema(),
		"tool_task": map[string]interface{}{
			"type": "string",
		},
	}
	if !hasRealCommandObservation(observations) {
		properties["done"] = map[string]interface{}{"type": "boolean", "enum": []bool{false}}
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   []string{"command", "done", "answer"},
	}
}

func structuredProofPlanSchema() map[string]interface{} {
	proofTypes := []string{
		structuredProofTypeUnitTest,
		structuredProofTypeIntegrationTest,
		structuredProofTypeSmokeTest,
		structuredProofTypeGoldenOutput,
		structuredProofTypeCompilerCheck,
		structuredProofTypeLintCheck,
		structuredProofTypeSourceVerification,
		structuredProofTypeManualEvaluatorAcceptance,
	}
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"objective_id": map[string]interface{}{"type": "string"},
			"proof_type":   map[string]interface{}{"type": "string", "enum": proofTypes},
			"files_to_create": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"files_to_modify": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"commands": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"acceptance_checks": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"out_of_scope": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"allowed_objective_sources": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string", "enum": defaultStructuredProofPlanAllowedSources()},
			},
		},
	}
}

func structuredObjectiveLedgerSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":               map[string]interface{}{"type": "string"},
				"description":      map[string]interface{}{"type": "string"},
				"status":           map[string]interface{}{"type": "string", "enum": []string{"pending", "satisfied"}},
				"kind":             map[string]interface{}{"type": "string", "enum": []string{string(WorkItemKindRead), string(WorkItemKindCreate), string(WorkItemKindUpdate), string(WorkItemKindDelete), string(WorkItemKindVerify), string(WorkItemKindArchitect)}},
				"evidence":         map[string]interface{}{"type": "string"},
				"source":           map[string]interface{}{"type": "string", "enum": []string{structuredObjectiveSourceUserExplicit, structuredObjectiveSourceRecipeRequired, structuredObjectiveSourceDetectedProject, structuredObjectiveSourceEvidenceRequiredPrerequisite, structuredObjectiveSourceMemorySuggested, structuredObjectiveSourceModelInferred}},
				"parent_objective": map[string]interface{}{"type": "string"},
				"required":         map[string]interface{}{"type": "boolean"},
				"packages":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			},
			"required": []string{"id", "description", "status", "kind"},
		},
	}
}

func buildShellCommandSpecialistRequest(input ShellCommandSpecialistInput) OllamaChatRequest {
	sessionMemories := filterExecutionSessionMemories(input.SessionMemories, input.UserPrompt+" "+input.ToolTask, "", len(input.SessionMemories))
	payload := struct {
		Role              string                          `json:"role"`
		UserPrompt        string                          `json:"user_prompt"`
		ToolTask          string                          `json:"tool_task"`
		ArchitectContract ImplementationArchitectContract `json:"architect_contract,omitempty"`
		ProjectFileMap    ProjectFileMap                  `json:"project_file_map,omitempty"`
		ProjectMapPolicy  []string                        `json:"project_map_policy,omitempty"`
		RepairFeedback    string                          `json:"repair_feedback,omitempty"`
		RejectedCommand   string                          `json:"rejected_command_preview,omitempty"`
		Observations      []StructuredCommandObservation  `json:"observations"`
		CompletedActions  []CompletedAction               `json:"completed_actions,omitempty"`
		LoopState         StructuredLoopState             `json:"loop_state,omitempty"`
		SessionMemories   []SessionMemory                 `json:"session_memories,omitempty"`
		WorksiteSurvey    WorksiteSurvey                  `json:"worksite_survey"`
		ToolRules         []string                        `json:"tool_rules"`
		RepairAttempt     int                             `json:"repair_attempt,omitempty"`
	}{
		Role:              "shell_execution_specialist",
		UserPrompt:        input.UserPrompt,
		ToolTask:          input.ToolTask,
		ArchitectContract: input.ArchitectContract,
		ProjectFileMap:    input.ProjectFileMap,
		ProjectMapPolicy:  projectFileMapPolicyLines(),
		RepairFeedback:    input.RepairFeedback,
		RepairAttempt:     input.RepairAttempt,
		RejectedCommand:   truncateStructuredTimelineValue(input.RejectedCommand),
		Observations:      compactStructuredObservationsForContext(input.Observations, 8, 650),
		CompletedActions:  input.CompletedActions,
		LoopState:         input.LoopState,
		SessionMemories:   compactSessionMemoriesForStructuredContext(sessionMemories, 6, 600),
		WorksiteSurvey:    input.WorksiteSurvey,
		ToolRules: []string{
			"Return JSON only with schema {\"command\":\"...\",\"rationale\":\"...\"}.",
			"Only choose a shell command that directly satisfies tool_task from the planner authority.",
			"If architect_contract is present, treat it as the implementation architect's authority over target_root, edit_surface, proof_commands, and guardrails.",
			"If architect_contract.current_item is present, satisfy only that one queued operation; use its cwd, path, operation, description, and verify fields literally.",
			"If project_file_map is present, treat it as authoritative; only mutate project_file_map.active_file and never touch unmapped source paths.",
			"Follow project_map_policy: shell must not use touch for mapped source files; write substantive content or delegate to architect.apply/code specialist.",
			"The architect decides what source area is edited and how it is proven; the coder/shell specialist only chooses the next concrete command inside that contract.",
			"If repair_feedback is non-empty, treat it as direct validator feedback for this retry; the next command must visibly correct that exact issue.",
			"Treat completed_actions as authoritative progress; never choose a command that repeats or recreates an already completed action.",
			"Treat loop_state as authoritative loop-monitor context; if it is stuck or blocked, choose a command that changes the pattern or gathers missing evidence.",
			"Treat completed_actions as the only deterministic do-not-repeat list.",
			"Rejected_command observations and failed commands are evidence with reasons; use them to correct strategy, not to create forbidden commands or framework/tool bans.",
			"If tool_task says creation, modification, writing, patching, build, or test is required, do not choose read-only inspection commands such as ls, cat, find, npm ls, sed -n, rg, grep, pwd, or test -f.",
			"If tool_task or observations mention chunked file editing, choose commands that read, patch, or verify one stated line range at a time; prefer the provided sed -n range over cat for large files.",
			"If tool_task says read-only inventory commands are forbidden, choose a file mutation, build, test, or patch-related shell command.",
			"If tool_task contains 'Implementation architect target root:', treat that target root as authoritative; cd into it or prefix all source/build/test paths with it.",
			"If tool_task names app, component, CRUD, UI, state, storage, or substantive source objectives, choose a command that writes or patches source files; do not choose dependency installs, echo/printf status text, or placeholder-only touch/mkdir scaffolds.",
			"If tool_task or repair_feedback mentions focused tests, TDD, proof plans, smoke tests, verification probes, source/build/test files, or placeholder-only files, do not use touch or mkdir alone; write substantive file content with a here-doc, tee, node/python script, sed/perl edit, or patch.",
			"For app/code feature tool_tasks, prefer a TDD command when no test/probe evidence exists yet: create or update a focused test, smoke test, or deterministic source-verification probe before implementation, or write the test/probe and minimal implementation together when one command is required.",
			"When a proof_plan or validated test/probe is present, implement only enough to satisfy it and do not weaken, delete, skip, or rewrite the test/probe unless tool_task explicitly asks for an approved correction.",
			"Keep proof commands inside the requested scope; do not add tests or dependencies for memory_suggested, model_inferred, or common-but-unrequested features.",
			"After implementation writes, choose the narrowest command that runs the focused test/probe before broader build/test commands.",
			"Only choose package-manager install/add commands when tool_task explicitly asks to install dependencies or names the exact package as a required prerequisite.",
			"If tool_task requires creating a project for an unfamiliar language/toolchain, choose a command that first gathers official documentation or installed tool help with curl/--help, then writes substantive source/build/test files in the same command.",
			"If session_memories or prep_context already include a documentation_brief for the requested language/toolchain, do not fetch the same docs again; write substantive source/build/test files from that guidance.",
			"If the requested compiler is unavailable and installation is not approved, source verification may document created artifacts but must not satisfy compiler, build, or test objectives.",
			"Memories and prior preferences cannot add dependencies, frameworks, files, services, architecture, or deployment targets unless tool_task explicitly says the current user asked for them.",
			"The WorksiteSurvey is authoritative; do not scaffold a new project when user_operation is modify_existing_project or fix_existing_project.",
			"For simple creation tasks, choose the smallest working command that satisfies tool_task.",
			"Do not answer the user and do not apologize.",
			"Do not use echo or printf to fake final evidence unless the task is explicitly to create/write literal text.",
			"For location-specific current time, prefer TZ=Area/City date '+%Y-%m-%d %H:%M:%S %Z'.",
			"For Thailand or Pattaya current time, use TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'.",
			"For current weather, use wttr.in no-key evidence with an explicit location and concise format query, for example curl -s 'https://wttr.in/Pattaya?format=%l|%C|%t|%f'.",
			"Do not use OpenWeatherMap or api.openweathermap.org unless observations contain a real non-placeholder API key; never use YOUR_API_KEY.",
			"If a prior executed command failed, choose a different command or corrected syntax.",
			"If repair_feedback says placeholder-only or scaffold already exists, expand the existing file with substantive source/build/test content instead of creating another empty file.",
			"Do not infer broad bans from rejected_command observations; valid equivalent framework commands are allowed when they satisfy tool_task.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"shell_execution_specialist","tool_task":""}`)
	}
	options := map[string]interface{}{
		"temperature": 0,
		"num_predict": 256,
	}
	if input.RepairAttempt > 0 {
		options["temperature"] = defaultShellSpecialistRepairTemperature
	}
	messages := []OllamaMessage{
		{
			Role: "system",
			Content: strings.Join([]string{
				"You are a shell execution specialist subordinate to a planner authority.",
				"You receive a scoped tool_task and return the safest concrete shell command for evidence gathering or requested system interaction.",
				"Return JSON only.",
			}, " "),
		},
		{Role: "user", Content: string(blob)},
	}
	if input.RepairAttempt > 0 && strings.TrimSpace(input.RejectedCommand) != "" {
		messages = append(messages, buildShellRepairFollowUpMessages(input.RepairFeedback, input.RejectedCommand)...)
	}
	return OllamaChatRequest{
		Messages: messages,
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command":   map[string]interface{}{"type": "string"},
				"rationale": map[string]interface{}{"type": "string"},
			},
			"required": []string{"command", "rationale"},
		},
		Options: options,
	}
}

func buildCodeContentSpecialistRequest(input CodeContentSpecialistInput) OllamaChatRequest {
	sessionMemories := filterExecutionSessionMemories(input.SessionMemories, input.UserPrompt+" "+input.WorkItem.Description, "", len(input.SessionMemories))
	fileContract := codeContentFileContract(input.ArchitectContract, input.WorkItem)
	rules := []string{
		"Return JSON only with schema {\"content\":\"full file content\",\"rationale\":\"brief reason\"}.",
		"Generate only the complete content for work_item.path; do not include shell commands, markdown fences, explanations outside JSON, or alternate paths.",
		"The architect owns cwd/path/operation. Use work_item literally and do not invent a different file.",
		"file_contract is authoritative. content must match file_contract.language and file_contract.role exactly.",
		"file_contract.must_include entries are required signals for this one file. file_contract.must_avoid entries are forbidden content classes for this one file.",
		"If architect_contract.acceptance_criteria is non-empty, the generated file must directly support those criteria within the current file's responsibility.",
		"If test_first is true, write a focused acceptance test, smoke test, or deterministic source probe for user_explicit behavior only; it must assert the relevant architect_contract.acceptance_criteria and must not be a render-only placeholder.",
		"If test_first is false, implement enough source code to satisfy the validated test/probe, the current work item, and the architect_contract.acceptance_criteria.",
		"Do not add unrequested dependencies, routing, authentication, cloud sync, databases, or framework changes.",
		"Do not weaken, skip, delete, or rewrite an existing validated test unless the work item explicitly says it is a test correction.",
		"The project_file_map is authoritative; generate content only for work_item.path and integrate with integration_context.",
	}
	if strings.TrimSpace(input.IntegrationContext) != "" {
		rules = append(rules, "integration_context describes how this file connects to other mapped files; honor those links in imports, mounts, and acceptance checks.")
	}
	if strings.TrimSpace(input.RepairFeedback) != "" {
		rules = append(rules,
			"repair_feedback is direct validator feedback for this retry; it is authoritative over prior assumptions.",
			"Repair the rejected file content directly; do not return the same rejected content or another forbidden file kind.",
			"If repair_feedback says content kind rejected, rewrite the file as pure file_contract.language and remove every file_contract.must_avoid class.",
			"If rejected_content_preview is present, treat it as the exact invalid output to replace; do not repeat it.",
		)
	}
	payload := struct {
		Role                   string                          `json:"role"`
		UserPrompt             string                          `json:"user_prompt"`
		ArchitectContract      ImplementationArchitectContract `json:"architect_contract"`
		WorkItem               ArchitectWorkItem               `json:"work_item"`
		FileContract           CodeContentFileContract         `json:"file_contract"`
		ExistingContent        string                          `json:"existing_content,omitempty"`
		TestFirst              bool                            `json:"test_first"`
		RepairFeedback         string                          `json:"repair_feedback,omitempty"`
		RepairAttempt          int                             `json:"repair_attempt,omitempty"`
		RejectedContentPreview string                          `json:"rejected_content_preview,omitempty"`
		IntegrationContext     string                          `json:"integration_context,omitempty"`
		ProjectFileMap         ProjectFileMap                  `json:"project_file_map,omitempty"`
		Observations           []StructuredCommandObservation  `json:"observations,omitempty"`
		SessionMemories        []SessionMemory                 `json:"session_memories,omitempty"`
		WorksiteSurvey         WorksiteSurvey                  `json:"worksite_survey"`
		Rules                  []string                        `json:"rules"`
	}{
		Role:                   "code_content_specialist",
		UserPrompt:             input.UserPrompt,
		ArchitectContract:      input.ArchitectContract,
		WorkItem:               input.WorkItem,
		FileContract:           fileContract,
		ExistingContent:        truncateStructuredTimelineValue(input.ExistingContent),
		TestFirst:              input.TestFirst,
		RepairFeedback:         input.RepairFeedback,
		RepairAttempt:          input.RepairAttempt,
		RejectedContentPreview: truncateStructuredTimelineValue(input.RejectedContent),
		IntegrationContext:     input.IntegrationContext,
		ProjectFileMap:         input.ProjectFileMap,
		Observations:           compactStructuredObservationsForContext(input.Observations, 6, 500),
		SessionMemories:        compactSessionMemoriesForStructuredContext(sessionMemories, 6, 600),
		WorksiteSurvey:         input.WorksiteSurvey,
		Rules:                  rules,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"code_content_specialist"}`)
	}
	messages := []OllamaMessage{
		{
			Role: "system",
			Content: strings.Join([]string{
				"You are a narrowly scoped coding specialist.",
				"The implementation architect has already chosen the exact file and operation.",
				"Your only job is to produce complete file content for that one work item.",
				"Return JSON only.",
			}, " "),
		},
		{Role: "user", Content: string(blob)},
	}
	if input.RepairAttempt > 0 && strings.TrimSpace(input.RejectedContent) != "" {
		messages = append(messages, buildSpecialistRepairFollowUpMessages(input.RepairFeedback, input.RejectedContent, []string{
			"Return JSON only with schema {\"content\":\"full file content\",\"rationale\":\"brief reason\"}.",
			"The validator feedback is authoritative for this repair attempt.",
			"Repair the rejected file content directly; do not restate or argue with the feedback.",
			"content must match file_contract.language and file_contract.role exactly.",
			"Do not return the same rejected content or another forbidden file kind.",
		})...)
	}
	options := map[string]interface{}{
		"temperature": 0,
		"num_predict": 4096,
	}
	if input.RepairAttempt > 0 {
		options["temperature"] = defaultShellSpecialistRepairTemperature
	}
	return OllamaChatRequest{
		Messages: messages,
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content":   map[string]interface{}{"type": "string"},
				"rationale": map[string]interface{}{"type": "string"},
			},
			"required": []string{"content", "rationale"},
		},
		Options: options,
	}
}

func codeContentFileContract(contract ImplementationArchitectContract, item ArchitectWorkItem) CodeContentFileContract {
	path := filepath.ToSlash(strings.TrimSpace(item.Path))
	lower := strings.ToLower(path)
	out := CodeContentFileContract{
		Path:      path,
		Operation: strings.TrimSpace(item.Operation),
		Role:      "project_source_file",
		Language:  "text",
	}
	switch {
	case lower == "package.json":
		out.Role = "npm_package_manifest"
		out.Language = "json"
		out.MustInclude = []string{`"scripts"`, `"build"`, `"test"`}
		out.MustAvoid = []string{"comments", "markdown", "html", "javascript module source"}
	case lower == "vite.config.js":
		out.Role = "vite_react_config"
		out.Language = "javascript_module"
		out.MustInclude = []string{"defineConfig", "@vitejs/plugin-react"}
		out.MustAvoid = []string{"html document", "react component", "css stylesheet"}
	case lower == "index.html":
		out.Role = "vite_html_shell"
		out.Language = "html"
		out.MustInclude = []string{`id="root"`, "/src/main.jsx"}
		out.MustAvoid = []string{"react component implementation", "css stylesheet", "node script"}
	case lower == "src/main.jsx" || lower == "src/main.js" || lower == "src/index.js":
		out.Role = "react_dom_mount_entry"
		out.Language = "javascript_or_jsx_module"
		out.MustInclude = []string{"createRoot", "App", "render"}
		out.MustAvoid = []string{"html document", "css stylesheet", "application UI implementation"}
	case lower == "scripts/smoke-test.mjs":
		out.Role = "deterministic_acceptance_probe"
		out.Language = "node_javascript_module"
		out.MustInclude = []string{"readFileSync", "process.exit"}
		out.MustAvoid = []string{"html document", "react component", "css stylesheet"}
	case lower == "src/app.js" || lower == "src/app.jsx":
		out.Role = "react_application_component"
		out.Language = "javascript_or_jsx_module"
		out.MustInclude = acceptanceSignalsForFileContract(contract)
		out.MustAvoid = []string{"html document", "css stylesheet", "react dom mount entry"}
	case lower == "src/app.css" || strings.HasSuffix(lower, ".css"):
		out.Role = "css_stylesheet"
		out.Language = "css"
		if includes := cssMustIncludeForContract(contract); len(includes) > 0 {
			out.MustInclude = includes
		} else {
			out.MustInclude = []string{"body", ".app"}
		}
		out.MustAvoid = []string{"html document", "javascript", "react component"}
	case strings.HasSuffix(lower, ".json"):
		out.Role = "json_data_or_config"
		out.Language = "json"
		out.MustAvoid = []string{"comments", "markdown", "html", "javascript"}
	case strings.HasSuffix(lower, ".html"):
		out.Role = "html_document"
		out.Language = "html"
		out.MustAvoid = []string{"react component implementation", "css stylesheet"}
	case strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".jsx") || strings.HasSuffix(lower, ".mjs"):
		out.Role = "javascript_module"
		out.Language = "javascript_or_jsx_module"
		out.MustAvoid = []string{"html document", "css stylesheet"}
	}
	return out
}

func acceptanceSignalsForFileContract(contract ImplementationArchitectContract) []string {
	signals := []string{}
	for _, criterion := range contract.AcceptanceCriteria {
		signals = append(signals, acceptanceCriterionSignals(criterion)...)
	}
	if len(signals) == 0 {
		return nil
	}
	return uniqueNonEmptyStrings(signals)
}
