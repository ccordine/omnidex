package omni

import (
	"os"
	"path/filepath"
	"strings"
)

type ImplementationArchitectContract struct {
	Role                string                     `json:"role"`
	SourcePrompt        string                     `json:"source_prompt,omitempty"`
	SourceToolTask      string                     `json:"source_tool_task,omitempty"`
	TargetRoot          string                     `json:"target_root"`
	Framework           string                     `json:"framework,omitempty"`
	PackageManager      string                     `json:"package_manager,omitempty"`
	EditSurface         []string                   `json:"edit_surface,omitempty"`
	ProofCommands       []string                   `json:"proof_commands,omitempty"`
	ResearchRequests    []ArchitectResearchRequest `json:"research_requests,omitempty"`
	MemoryBriefs        []PrepBrief                `json:"memory_briefs,omitempty"`
	DocumentationBriefs []PrepBrief                `json:"documentation_briefs,omitempty"`
	WebResearchBriefs   []PrepBrief                `json:"web_research_briefs,omitempty"`
	AcceptanceCriteria  []string                   `json:"acceptance_criteria,omitempty"`
	Guardrails          []string                   `json:"guardrails,omitempty"`
	ValidatorScopes     []string                   `json:"validator_scopes,omitempty"`
	WorkQueue           []ArchitectWorkItem        `json:"work_queue,omitempty"`
	CurrentItem         *ArchitectWorkItem         `json:"current_item,omitempty"`
	ProjectFileMap      ProjectFileMap             `json:"project_file_map,omitempty"`
}

type ArchitectResearchRequest struct {
	ID         string   `json:"id"`
	Specialist string   `json:"specialist"`
	Tools      []string `json:"tools"`
	Query      string   `json:"query"`
	Reason     string   `json:"reason"`
	Required   bool     `json:"required"`
}

type ArchitectWorkItem struct {
	ID          string `json:"id"`
	Operation   string `json:"operation"`
	CWD         string `json:"cwd"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description"`
	Verify      string `json:"verify,omitempty"`
}

func buildImplementationArchitectContract(prompt, toolTask, workingDir string, survey WorksiteSurvey, observations []StructuredCommandObservation) ImplementationArchitectContract {
	if !toolTaskNeedsImplementationArchitect(prompt, toolTask) {
		return ImplementationArchitectContract{}
	}
	targetRoot := implementationArchitectTargetRootFromToolTask(toolTask)
	if targetRoot == "" {
		targetRoot = firstNestedAppRootWithFiles(workingDir)
	}
	if targetRoot == "" {
		targetRoot = "."
	}
	text := strings.ToLower(prompt + "\n" + toolTask)
	framework := ""
	if strings.Contains(text, "react") {
		framework = "react"
	}
	packageManager := survey.PackageManager
	if packageManager == "" || packageManager == packageManagerNone {
		packageManager = detectPackageManagerForArchitect(workingDir, targetRoot)
	}
	contract := ImplementationArchitectContract{
		Role:               "implementation_architect",
		SourcePrompt:       strings.TrimSpace(prompt),
		SourceToolTask:     strings.TrimSpace(toolTask),
		TargetRoot:         targetRoot,
		Framework:          framework,
		PackageManager:     packageManager,
		AcceptanceCriteria: explicitReactAppAcceptanceCriteria(prompt, toolTask),
		Guardrails: []string{
			"Planner decides that implementation is needed; architect decides what source area is edited and how it is proven.",
			"Coder/shell specialist must execute inside target_root or use paths under target_root.",
			"Do not scaffold a new sibling project when target_root already exists.",
			"Do not create placeholder-only files; write substantive source/build/test content.",
			"Use existing project dependencies unless the current objective explicitly requires an install.",
		},
		ValidatorScopes: []string{
			"mechanical_command_validator: command must target architect target_root/edit_surface and obey dependency guardrails.",
			"proof_validator: proof commands must be executable in target_root and tied to current objectives.",
			"alignment_validator: after implementation evidence exists, check the completed work against user objectives without adding unrequested expectations.",
		},
	}
	if framework == "react" {
		reactFileItem := func(id, path, description, verify string) ArchitectWorkItem {
			return ArchitectWorkItem{ID: id, Operation: architectWriteOperationForPath(workingDir, targetRoot, path, observations), CWD: targetRoot, Path: path, Description: description, Verify: verify}
		}
		contract.EditSurface = architectPaths(targetRoot,
			"src/App.js",
			"src/App.jsx",
			"src/App.css",
			"src/main.jsx",
			"index.html",
			"vite.config.js",
			"scripts/",
			"src/components/",
			"package.json",
		)
		contract.ProofCommands = architectCommands(targetRoot, packageManager, "npm run build")
		contract.WorkQueue = []ArchitectWorkItem{
			reactFileItem("setup_react_package_metadata", "package.json", "Create or update package metadata with executable Vite build, test, preview, and dev scripts plus only required React/Vite dependencies", "npm test"),
			reactFileItem("create_vite_react_config", "vite.config.js", "Create or update the Vite React config so JSX source files build correctly", "npm test"),
			reactFileItem("create_react_html_shell", "index.html", "Create or update the Vite HTML shell with a root mount for the React app", "npm test"),
			reactFileItem("create_react_mount_entry", "src/main.jsx", "Create or update the React DOM mount entrypoint that renders the app", "npm test"),
			reactFileItem("write_react_acceptance_test", "scripts/smoke-test.mjs", "Create or update a focused deterministic source probe for the requested React app behavior before implementation; it must check the requested UI signals and fail if the app is only a placeholder", "npm test"),
			reactFileItem("create_react_entrypoint", "src/App.js", "Create or update the primary React app UI and state for the requested feature set", "npm run build"),
			reactFileItem("style_react_app", "src/App.css", "Create or update the React app stylesheet so the requested UI is usable and readable", "npm run build"),
			{ID: "install_react_dependencies", Operation: "verify", CWD: targetRoot, Description: "Install package dependencies after package metadata is written", Verify: "npm install"},
			{ID: "run_react_acceptance_test", Operation: "verify", CWD: targetRoot, Description: "Run the focused acceptance smoke test after implementation", Verify: "npm test"},
			{ID: "verify_react_build", Operation: "verify", CWD: targetRoot, Description: "Run the React build proof command", Verify: "npm run build"},
		}
		contract.WorkQueue = insertReadItemsBeforeUpdates(contract.WorkQueue)
	} else {
		contract.EditSurface = architectPaths(targetRoot, "src/", "package.json", "Cargo.toml", "go.mod", "build.zig")
		contract.ProofCommands = architectCommands(targetRoot, packageManager, "test -n \"$(find . -maxdepth 3 -type f | head -1)\"")
		contract.WorkQueue = []ArchitectWorkItem{
			{ID: "write_project_source", Operation: "update", CWD: targetRoot, Path: "src/", Description: "Write substantive project source for the current objective", Verify: contract.ProofCommands[0]},
		}
		contract.WorkQueue = insertReadItemsBeforeUpdates(contract.WorkQueue)
	}
	if repair := architectRepairWorkItemFromObservations(targetRoot, observations); repair != nil {
		contract.CurrentItem = repair
		return contract
	}
	contract.CurrentItem = firstIncompleteArchitectWorkItem(contract.WorkQueue, workingDir, contract, observations)
	return contract
}

func architectContractPrompt(contract ImplementationArchitectContract) string {
	return strings.TrimSpace(contract.SourcePrompt)
}

func architectWriteOperationForPath(workingDir, cwd, path string, observations []StructuredCommandObservation) string {
	if operation := observedArchitectWriteOperationForPath(cwd, path, observations); operation != "" {
		return operation
	}
	targetPath := filepath.Join(workingDir, cwd, path)
	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		return "update"
	}
	return "create"
}

func observedArchitectWriteOperationForPath(cwd, path string, observations []StructuredCommandObservation) string {
	wantPath := filepath.ToSlash(strings.ToLower(filepath.Join(cwd, path)))
	if wantPath == "" || strings.TrimSpace(path) == "" {
		return ""
	}
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode != 0 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(obs.Command))
		if len(fields) < 3 || strings.ToLower(fields[0]) != "architect.apply" {
			continue
		}
		operation := strings.ToLower(fields[1])
		if operation != "create" && operation != "update" {
			continue
		}
		gotPath := filepath.ToSlash(strings.ToLower(fields[len(fields)-1]))
		if gotPath == wantPath || strings.HasSuffix(gotPath, "/"+strings.TrimPrefix(wantPath, "./")) {
			return operation
		}
	}
	return ""
}

func insertReadItemsBeforeUpdates(queue []ArchitectWorkItem) []ArchitectWorkItem {
	out := make([]ArchitectWorkItem, 0, len(queue)*2)
	for _, item := range queue {
		if item.Operation == "update" && strings.TrimSpace(item.Path) != "" && !strings.HasSuffix(strings.TrimSpace(item.Path), "/") {
			read := ArchitectWorkItem{
				ID:          "read_before_" + item.ID,
				Operation:   "read",
				CWD:         item.CWD,
				Path:        item.Path,
				Description: "Read current file content before updating " + item.Path,
			}
			out = append(out, read)
		}
		out = append(out, item)
	}
	return out
}

func architectRepairWorkItemFromObservations(targetRoot string, observations []StructuredCommandObservation) *ArchitectWorkItem {
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 {
			continue
		}
		path := viteMissingEntryPath(obs.Stderr)
		if path == "" {
			path = viteMissingEntryPath(obs.Stdout)
		}
		if path == "" {
			path = viteSyntaxErrorPath(obs.Stderr)
		}
		if path == "" {
			path = viteSyntaxErrorPath(obs.Stdout)
		}
		if path == "" && viteCSSPostError(obs.Stderr+"\n"+obs.Stdout) {
			path = "src/App.css"
		}
		if path == "" && strings.Contains(obs.Stderr+"\n"+obs.Stdout, "Cannot find module '@vitejs/plugin-react'") {
			path = "package.json"
		}
		if path == "" && strings.Contains(obs.Stderr+"\n"+obs.Stdout, "require is not defined in ES module scope") {
			path = "scripts/smoke-test.mjs"
		}
		if path == "" {
			continue
		}
		if architectMissingEntryResolvedAfter(path, targetRoot, observations[i+1:]) {
			return nil
		}
		description := "Repair missing Vite entry module referenced by index.html/build output"
		if path == "package.json" {
			description = "Repair package metadata for dependency imported by Vite config/build output"
		}
		return &ArchitectWorkItem{
			ID:          "repair_missing_vite_entry_" + sanitizeArchitectWorkItemID(path),
			Operation:   "update",
			CWD:         targetRoot,
			Path:        path,
			Description: description,
			Verify:      "npm run build",
		}
	}
	return nil
}

func viteMissingEntryPath(output string) string {
	for _, marker := range []string{"Failed to resolve /", "failed to resolve /", "Rollup failed to resolve import \"/"} {
		idx := strings.Index(output, marker)
		if idx < 0 {
			continue
		}
		rest := output[idx+len(marker):]
		if strings.HasSuffix(marker, "\"/") {
			if end := strings.Index(rest, "\""); end >= 0 {
				rest = rest[:end]
			}
		} else if end := strings.IndexAny(rest, " \n\r\t\"'"); end >= 0 {
			rest = rest[:end]
		}
		rest = strings.TrimPrefix(strings.TrimSpace(rest), "/")
		if strings.HasPrefix(rest, "src/") && (strings.HasSuffix(rest, ".js") || strings.HasSuffix(rest, ".jsx")) {
			return filepath.ToSlash(rest)
		}
	}
	return ""
}

func viteSyntaxErrorPath(output string) string {
	for _, marker := range []string{"[ src/", "[src/"} {
		idx := strings.Index(output, marker)
		if idx < 0 {
			continue
		}
		rest := output[idx+1:]
		if end := strings.Index(rest, ":"); end >= 0 {
			path := filepath.ToSlash(strings.TrimSpace(rest[:end]))
			if strings.HasPrefix(path, "src/") && (strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx")) {
				return path
			}
		}
	}
	return ""
}

func viteCSSPostError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "[plugin vite:css-post]") || strings.Contains(lower, "lightningcss")
}

func architectMissingEntryResolvedAfter(path, targetRoot string, observations []StructuredCommandObservation) bool {
	item := ArchitectWorkItem{Operation: "update", CWD: targetRoot, Path: path}
	for _, obs := range observations {
		if obs.ExitCode == 0 && architectApplyObservationMatches(item, obs) {
			return true
		}
		if obs.ExitCode == 0 && architectObservationIsBuildProof(targetRoot, obs) {
			return true
		}
	}
	return false
}

func architectObservationIsBuildProof(targetRoot string, obs StructuredCommandObservation) bool {
	command := normalizeStructuredCommandForComparison(obs.Command)
	if command == "" {
		return false
	}
	build := normalizeStructuredCommandForComparison(commandInArchitectCWD(targetRoot, "npm run build"))
	return command == build || strings.Contains(command, "npm run build")
}

func sanitizeArchitectWorkItemID(path string) string {
	clean := strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(strings.TrimSpace(path))
	clean = strings.Trim(clean, "_")
	if clean == "" {
		return "file"
	}
	return clean
}

func explicitReactAppAcceptanceCriteria(prompt, toolTask string) []string {
	text := strings.ToLower(prompt + "\n" + toolTask)
	candidates := []string{
		"pattern step sequencer",
		"channel rack",
		"mixer controls",
		"transport controls",
		"tempo control",
		"piano roll",
		"note grid",
		"sample/instrument pads",
		"sample pads",
		"instrument pads",
		"visual timeline",
		"production-studio ui",
		"usable app, not a landing page",
		"note capture",
		"note list",
		"in-memory notes",
		"create notes",
		"update notes",
		"delete notes",
		"todo list",
		"graphing calculator",
		"function plot",
		"equation input",
		"graph canvas",
		"coordinate plane",
		"plot graph",
	}
	out := []string{}
	for _, candidate := range candidates {
		if strings.Contains(text, candidate) {
			out = append(out, candidate)
		}
	}
	if len(out) == 0 && strings.Contains(text, "music production") {
		out = append(out, "music production interface")
	}
	if len(out) == 0 && promptRequestsNoteApp(prompt, toolTask) {
		out = append(out, "note capture", "note list", "in-memory notes")
	}
	if len(out) == 0 && promptRequestsGraphingCalculator(prompt, toolTask) {
		out = append(out, "graphing calculator", "function plot", "equation input", "graph canvas")
	}
	return out
}

func enrichImplementationArchitectContract(contract ImplementationArchitectContract, prompt, toolTask string, prep PrepContextBundle, memories []SessionMemory) ImplementationArchitectContract {
	if !hasImplementationArchitectContract(contract) {
		return contract
	}
	compactPrep := CompactPrepContextBundle(prep, defaultPrepContextBudgetLimit/2)
	contract.MemoryBriefs = limitPrepBriefsForArchitect(compactPrep.MemoryBriefs, 4)
	contract.DocumentationBriefs = limitPrepBriefsForArchitect(compactPrep.DocumentationBriefs, 5)
	contract.WebResearchBriefs = limitPrepBriefsForArchitect(compactPrep.WebResearchBriefs, 4)
	for _, memory := range compactSessionMemoriesForStructuredContext(memories, 4, 500) {
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		if memoryBriefLooksForeignToPrompt(memory, prompt, toolTask) {
			continue
		}
		contract.MemoryBriefs = append(contract.MemoryBriefs, PrepBrief{
			ID:      "session-memory-" + strings.ReplaceAll(strings.TrimSpace(memory.Kind), " ", "-"),
			Kind:    firstNonEmpty(strings.TrimSpace(memory.Kind), "memory"),
			Content: strings.TrimSpace(memory.Content),
			Tags:    cleanMemoryTags(memory.Tags),
			UsedBy:  []string{"implementation_architect", "documentation_specialist", "code_content_specialist"},
		})
	}
	contract.MemoryBriefs = limitPrepBriefsForArchitect(contract.MemoryBriefs, 6)
	contract.ResearchRequests = architectResearchRequests(prompt, toolTask, contract, compactPrep)
	contract.Guardrails = append(contract.Guardrails,
		"Before guessing unfamiliar APIs or project conventions, the architect may request memory.search, pgsql.query, documentation, or web research through research_requests.",
		"Documentation specialist and implementation architect should share authoritative docs; code specialists receive only compact briefs and the current work item.",
		"Research briefs are advisory context and cannot expand scope, dependencies, or completion criteria without user_explicit or recipe_required provenance.",
	)
	return contract
}

func limitPrepBriefsForArchitect(briefs []PrepBrief, limit int) []PrepBrief {
	out := []PrepBrief{}
	for _, brief := range briefs {
		if strings.TrimSpace(brief.Content) == "" {
			continue
		}
		brief.Content = truncateStructuredTimelineValue(brief.Content)
		brief.UsedBy = appendUniqueStrings(brief.UsedBy, "implementation_architect", "documentation_specialist", "code_content_specialist")
		out = append(out, brief)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func architectResearchRequests(prompt, toolTask string, contract ImplementationArchitectContract, prep PrepContextBundle) []ArchitectResearchRequest {
	query := strings.TrimSpace(prompt)
	if strings.TrimSpace(toolTask) != "" {
		query = strings.TrimSpace(prompt + " " + toolTask)
	}
	requests := []ArchitectResearchRequest{
		{ID: "architect-memory-search", Specialist: "memory_retrieval_specialist", Tools: []string{"memory.search", "pgsql.query"}, Query: query, Reason: "Retrieve relevant validated playbooks, project memories, schema knowledge, and prior successful procedures before coding.", Required: false},
		{ID: "architect-documentation-brief", Specialist: "documentation_specialist", Tools: []string{"memory.search", "pgsql.query", "web.search", "web.fetch", "curl"}, Query: architectDocumentationQuery(query, contract), Reason: "Provide authoritative setup, API, file layout, proof command, and example guidance for the architect work queue.", Required: len(contract.DocumentationBriefs) == 0},
	}
	if prep.WebResearchChecked || strings.Contains(strings.ToLower(query), "latest") || strings.Contains(strings.ToLower(query), "current") {
		requests = append(requests, ArchitectResearchRequest{ID: "architect-web-research", Specialist: "web_research_specialist", Tools: []string{"web.search", "web.fetch", "curl", "memory.create"}, Query: query, Reason: "Gather fresh external facts or current documentation when memory/docs are missing or stale.", Required: len(contract.WebResearchBriefs) == 0})
	}
	return requests
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values)+len(additions))
	for _, value := range append(values, additions...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func architectDocumentationQuery(query string, contract ImplementationArchitectContract) string {
	parts := []string{strings.TrimSpace(query)}
	if contract.Framework != "" {
		parts = append(parts, contract.Framework+" official documentation")
	}
	if contract.PackageManager != "" && contract.PackageManager != packageManagerNone {
		parts = append(parts, contract.PackageManager+" build test scripts")
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func toolTaskNeedsImplementationArchitect(prompt, toolTask string) bool {
	text := strings.ToLower(prompt + "\n" + toolTask)
	if strings.Contains(text, "current weather") || strings.Contains(text, "current time") || strings.Contains(text, "openweathermap") {
		return false
	}
	if strings.Contains(text, "weather") && strings.Contains(text, "public evidence") {
		return false
	}
	if strings.Contains(strings.ToLower(toolTask), "implementation architect target root:") {
		return true
	}
	if strings.Contains(text, "target path does not exist") ||
		strings.Contains(text, "proposed command already completed earlier") ||
		strings.Contains(text, "choose the next unread relevant file") {
		return false
	}
	promptLower := strings.ToLower(prompt)
	if !promptRequestsImplementationArchitecture(promptLower) {
		return false
	}
	if strings.Contains(text, "existing react") || strings.Contains(text, "existing project") {
		return strings.Contains(text, "implementation architect target root:")
	}
	for _, needle := range []string{
		"implementation architect target root:",
		"app-building task",
		"required app files",
		"create or modify the actual project files",
		"substantive source/build/test",
		"component",
		"crud",
		"ui",
		"step sequencer",
		"music production app",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func promptRequestsImplementationArchitecture(prompt string) bool {
	for _, needle := range []string{"build", "create", "implement", "app", "react", "component", "crud", "ui", "frontend", "cli", "project"} {
		if strings.Contains(prompt, needle) {
			return true
		}
	}
	return false
}

func firstIncompleteArchitectWorkItem(queue []ArchitectWorkItem, workingDir string, contract ImplementationArchitectContract, observations []StructuredCommandObservation) *ArchitectWorkItem {
	for i := range queue {
		item := queue[i]
		if architectWorkItemSatisfied(item, workingDir, contract, observations) {
			continue
		}
		if err := architectImplementationBlockedByMissingTestProbe(queue, item, workingDir, contract, architectContractPrompt(contract), observations); err != nil {
			continue
		}
		return &queue[i]
	}
	return nil
}

func architectWorkItemSatisfied(item ArchitectWorkItem, workingDir string, contract ImplementationArchitectContract, observations []StructuredCommandObservation) bool {
	switch item.Operation {
	case "read":
		command := normalizeStructuredCommandForComparison("architect.read " + filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
		for _, obs := range observations {
			if obs.ExitCode == 0 && normalizeStructuredCommandForComparison(obs.Command) == command {
				return true
			}
		}
		return false
	case "update", "create":
		if item.Path == "" || strings.HasSuffix(item.Path, "/") {
			return false
		}
		for _, obs := range observations {
			if obs.ExitCode == 0 && architectApplyObservationMatches(item, obs) {
				if hasImplementationArchitectContract(contract) {
					if _, err := architectWorkItemFileEvidenceValid(item, workingDir, contract, architectContractPrompt(contract)); err != nil {
						return false
					}
				}
				return true
			}
		}
		if hasImplementationArchitectContract(contract) {
			if _, err := architectWorkItemFileEvidenceValid(item, workingDir, contract, architectContractPrompt(contract)); err == nil {
				return true
			}
		}
		return false
	case "delete":
		command := normalizeStructuredCommandForComparison("architect.delete " + filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
		for _, obs := range observations {
			if obs.ExitCode == 0 && normalizeStructuredCommandForComparison(obs.Command) == command {
				return true
			}
		}
		return false
	case "verify":
		verify := normalizeStructuredCommandForComparison(commandInArchitectCWD(item.CWD, item.Verify))
		latestPackageMutation := latestArchitectApplyObservationIndex(item.CWD, "package.json", observations)
		latestSmokeRelevantMutation := latestArchitectApplyObservationIndexForPaths(item.CWD, []string{"package.json", "scripts/smoke-test.mjs", "src/App.js", "src/App.css"}, observations)
		for i, obs := range observations {
			if obs.ExitCode == 0 && normalizeStructuredCommandForComparison(obs.Command) == verify {
				if normalizeStructuredCommandForComparison(item.Verify) == "npm install" && latestPackageMutation >= 0 && i < latestPackageMutation {
					continue
				}
				if normalizeStructuredCommandForComparison(item.Verify) == "npm test" && latestSmokeRelevantMutation >= 0 && i < latestSmokeRelevantMutation {
					continue
				}
				return true
			}
		}
	}
	return false
}

func latestArchitectApplyObservationIndex(cwd, path string, observations []StructuredCommandObservation) int {
	item := ArchitectWorkItem{Operation: "update", CWD: cwd, Path: path}
	for i := len(observations) - 1; i >= 0; i-- {
		if observations[i].ExitCode == 0 && architectApplyObservationMatches(item, observations[i]) {
			return i
		}
	}
	return -1
}

func latestArchitectApplyObservationIndexForPaths(cwd string, paths []string, observations []StructuredCommandObservation) int {
	latest := -1
	for _, path := range paths {
		if idx := latestArchitectApplyObservationIndex(cwd, path, observations); idx > latest {
			latest = idx
		}
	}
	return latest
}

func architectWorkItemIsTestFirst(item ArchitectWorkItem) bool {
	text := strings.ToLower(item.ID + " " + item.Path)
	return strings.Contains(text, "test") || strings.Contains(text, "smoke") || strings.Contains(text, "acceptance")
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func workQueueContainsPath(queue []ArchitectWorkItem, path string) bool {
	path = filepath.ToSlash(strings.ToLower(strings.TrimSpace(path)))
	for _, item := range queue {
		if filepath.ToSlash(strings.ToLower(strings.TrimSpace(item.Path))) == path {
			return true
		}
	}
	return false
}

func missingAcceptanceSignals(content string, criteria []string) []string {
	lower := strings.ToLower(content)
	missing := []string{}
	for _, criterion := range criteria {
		for _, signal := range acceptanceCriterionSignals(criterion) {
			if strings.Contains(lower, signal) {
				goto found
			}
		}
		missing = append(missing, criterion)
	found:
	}
	return missing
}

func missingCSSAcceptanceSignals(content string, criteria []string) []string {
	lower := strings.ToLower(content)
	missing := []string{}
	for _, criterion := range criteria {
		found := false
		for _, signal := range acceptanceCriterionSignals(criterion) {
			if strings.Contains(lower, signal) {
				found = true
				break
			}
			className := "." + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(signal)), " ", "-")
			if strings.Contains(lower, className) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, criterion)
		}
	}
	return missing
}

func acceptanceCriterionSignals(criterion string) []string {
	switch strings.ToLower(strings.TrimSpace(criterion)) {
	case "pattern step sequencer":
		return []string{"sequencer", "step"}
	case "channel rack":
		return []string{"channel rack", "channel"}
	case "mixer controls":
		return []string{"mixer"}
	case "transport controls":
		return []string{"transport", "play", "stop"}
	case "tempo control":
		return []string{"tempo", "bpm"}
	case "piano roll":
		return []string{"piano roll", "piano"}
	case "note grid":
		return []string{"note grid", "note"}
	case "sample/instrument pads", "sample pads", "instrument pads":
		return []string{"pad", "sample", "instrument"}
	case "visual timeline":
		return []string{"timeline"}
	case "production-studio ui":
		return []string{"studio"}
	case "usable app, not a landing page":
		return []string{"sequencer", "mixer", "transport", "timeline", "note", "todo", "button"}
	case "music production interface":
		return []string{"music", "studio"}
	case "note capture", "note list", "in-memory notes", "create notes", "update notes", "delete notes", "todo list":
		return []string{"note", "todo", "title", "body", "list"}
	case "graphing calculator", "graph canvas", "function plot", "equation input", "coordinate plane", "plot graph":
		return []string{"graph", "calculator", "equation", "plot", "function", "coordinate"}
	default:
		return []string{strings.ToLower(strings.TrimSpace(criterion))}
	}
}

func architectApplyObservationMatches(item ArchitectWorkItem, obs StructuredCommandObservation) bool {
	command := strings.TrimSpace(obs.Command)
	if !strings.HasPrefix(strings.ToLower(command), "architect.apply ") {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) < 3 {
		return false
	}
	gotOperation := strings.ToLower(fields[1])
	wantOperation := strings.ToLower(item.Operation)
	if gotOperation != wantOperation && !architectWriteOperationsEquivalent(gotOperation, wantOperation) {
		return false
	}
	gotPath := filepath.ToSlash(strings.ToLower(fields[len(fields)-1]))
	wantPath := filepath.ToSlash(strings.ToLower(filepath.Join(item.CWD, item.Path)))
	if wantPath == "" {
		wantPath = filepath.ToSlash(strings.ToLower(item.Path))
	}
	return gotPath == wantPath || strings.HasSuffix(gotPath, "/"+strings.TrimPrefix(wantPath, "./"))
}

func architectWriteOperationsEquivalent(a, b string) bool {
	return (a == "create" || a == "update") && (b == "create" || b == "update")
}

func commandInArchitectCWD(cwd, command string) string {
	command = strings.TrimSpace(command)
	cwd = strings.TrimSpace(cwd)
	if command == "" || cwd == "" || cwd == "." || strings.HasPrefix(command, "cd ") {
		return command
	}
	return "cd " + cwd + " && " + command
}

func detectPackageManagerForArchitect(workingDir, targetRoot string) string {
	root := workingDir
	if targetRoot != "" && targetRoot != "." {
		root = filepath.Join(workingDir, targetRoot)
	}
	switch {
	case fileHasContent(filepath.Join(root, "package-lock.json")):
		return packageManagerNPM
	case fileHasContent(filepath.Join(root, "pnpm-lock.yaml")):
		return packageManagerPNPM
	case fileHasContent(filepath.Join(root, "yarn.lock")):
		return packageManagerYarn
	case fileHasContent(filepath.Join(root, "package.json")):
		return packageManagerNPM
	default:
		return packageManagerNone
	}
}

func architectPaths(targetRoot string, paths ...string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if targetRoot != "" && targetRoot != "." {
			path = filepath.ToSlash(filepath.Join(targetRoot, path))
		}
		out = append(out, path)
	}
	return out
}

func architectCommands(targetRoot, packageManager, fallback string) []string {
	cmd := fallback
	if packageManager == packageManagerNPM || packageManager == "" || packageManager == packageManagerNone {
		cmd = "npm run build"
	}
	if targetRoot != "" && targetRoot != "." {
		cmd = "cd " + targetRoot + " && " + cmd
	}
	return []string{cmd}
}
