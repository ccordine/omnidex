package omni

import (
	"fmt"
	"path/filepath"
	"strings"
)

type ImplementationArchitectContract struct {
	Role                string                     `json:"role"`
	TargetRoot          string                     `json:"target_root"`
	Framework           string                     `json:"framework,omitempty"`
	PackageManager      string                     `json:"package_manager,omitempty"`
	EditSurface         []string                   `json:"edit_surface,omitempty"`
	ProofCommands       []string                   `json:"proof_commands,omitempty"`
	ResearchRequests    []ArchitectResearchRequest `json:"research_requests,omitempty"`
	MemoryBriefs        []PrepBrief                `json:"memory_briefs,omitempty"`
	DocumentationBriefs []PrepBrief                `json:"documentation_briefs,omitempty"`
	WebResearchBriefs   []PrepBrief                `json:"web_research_briefs,omitempty"`
	Guardrails          []string                   `json:"guardrails,omitempty"`
	ValidatorScopes     []string                   `json:"validator_scopes,omitempty"`
	WorkQueue           []ArchitectWorkItem        `json:"work_queue,omitempty"`
	CurrentItem         *ArchitectWorkItem         `json:"current_item,omitempty"`
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
		Role:           "implementation_architect",
		TargetRoot:     targetRoot,
		Framework:      framework,
		PackageManager: packageManager,
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
		contract.EditSurface = architectPaths(targetRoot,
			"src/App.js",
			"src/App.jsx",
			"src/App.css",
			"src/index.js",
			"src/main.jsx",
			"src/components/",
			"package.json",
		)
		contract.ProofCommands = architectCommands(targetRoot, packageManager, "npm run build")
		contract.WorkQueue = []ArchitectWorkItem{
			{ID: "write_react_acceptance_test", Operation: "update", CWD: targetRoot, Path: "src/App.test.js", Description: "Create a focused acceptance test or deterministic source probe for the requested React app behavior before implementation", Verify: "npm test -- --watchAll=false"},
			{ID: "create_react_entrypoint", Operation: "update", CWD: targetRoot, Path: "src/App.js", Description: "Create the primary React app UI and state for the requested feature set", Verify: "npm run build"},
			{ID: "style_react_app", Operation: "update", CWD: targetRoot, Path: "src/App.css", Description: "Style the React app so the requested UI is usable and readable", Verify: "npm run build"},
			{ID: "verify_react_build", Operation: "verify", CWD: targetRoot, Description: "Run the React build proof command", Verify: "npm run build"},
		}
	} else {
		contract.EditSurface = architectPaths(targetRoot, "src/", "package.json", "Cargo.toml", "go.mod", "build.zig")
		contract.ProofCommands = architectCommands(targetRoot, packageManager, "test -n \"$(find . -maxdepth 3 -type f | head -1)\"")
		contract.WorkQueue = []ArchitectWorkItem{
			{ID: "write_project_source", Operation: "update", CWD: targetRoot, Path: "src/", Description: "Write substantive project source for the current objective", Verify: contract.ProofCommands[0]},
		}
	}
	contract.CurrentItem = firstIncompleteArchitectWorkItem(contract.WorkQueue, workingDir, observations)
	return contract
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

func firstIncompleteArchitectWorkItem(queue []ArchitectWorkItem, workingDir string, observations []StructuredCommandObservation) *ArchitectWorkItem {
	for i := range queue {
		item := queue[i]
		if architectWorkItemSatisfied(item, workingDir, observations) {
			continue
		}
		return &queue[i]
	}
	return nil
}

func architectWorkItemSatisfied(item ArchitectWorkItem, workingDir string, observations []StructuredCommandObservation) bool {
	switch item.Operation {
	case "update", "create":
		if item.Path == "" || strings.HasSuffix(item.Path, "/") {
			return false
		}
		for _, obs := range observations {
			if obs.ExitCode == 0 && architectApplyObservationMatches(item, obs) {
				return true
			}
		}
		return false
	case "verify":
		verify := normalizeStructuredCommandForComparison(commandInArchitectCWD(item.CWD, item.Verify))
		for _, obs := range observations {
			if obs.ExitCode == 0 && normalizeStructuredCommandForComparison(obs.Command) == verify {
				return true
			}
		}
	}
	return false
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
	if strings.ToLower(fields[1]) != strings.ToLower(item.Operation) {
		return false
	}
	gotPath := filepath.ToSlash(strings.ToLower(fields[len(fields)-1]))
	wantPath := filepath.ToSlash(strings.ToLower(filepath.Join(item.CWD, item.Path)))
	if wantPath == "" {
		wantPath = filepath.ToSlash(strings.ToLower(item.Path))
	}
	return gotPath == wantPath || strings.HasSuffix(gotPath, "/"+strings.TrimPrefix(wantPath, "./"))
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

func hasImplementationArchitectContract(contract ImplementationArchitectContract) bool {
	return strings.TrimSpace(contract.TargetRoot) != "" || len(contract.EditSurface) > 0 || len(contract.ProofCommands) > 0
}

func validateCommandAgainstImplementationArchitectContract(command string, contract ImplementationArchitectContract) error {
	if !hasImplementationArchitectContract(contract) || strings.TrimSpace(command) == "" {
		return nil
	}
	lower := strings.ToLower(command)
	if strings.Contains(lower, "/path/to/your/project") || strings.Contains(lower, "<project") || strings.Contains(lower, "your-project") {
		return errArchitectContract("command contains placeholder project path; use architect target_root %q", contract.TargetRoot)
	}
	target := strings.TrimSpace(contract.TargetRoot)
	if target == "" || target == "." {
		return validateCommandAgainstArchitectCurrentItem(command, contract)
	}
	if structuredCommandLooksDependencyInstall(command) || structuredCommandLooksReadOnlyEvidence(command) {
		return nil
	}
	cmd := filepath.ToSlash(strings.ToLower(command))
	target = filepath.ToSlash(strings.ToLower(strings.Trim(target, "/")))
	if commandChangesIntoProjectRoot(cmd, target) || strings.Contains(cmd, target+"/") {
		return validateCommandAgainstArchitectCurrentItem(command, contract)
	}
	return errArchitectContract("command must target architect root %q by cd-ing into it or using paths under it", contract.TargetRoot)
}

func validateCommandAgainstArchitectCurrentItem(command string, contract ImplementationArchitectContract) error {
	if contract.CurrentItem == nil {
		return nil
	}
	item := *contract.CurrentItem
	if item.Operation == "verify" {
		expected := normalizeStructuredCommandForComparison(commandInArchitectCWD(item.CWD, item.Verify))
		if normalizeStructuredCommandForComparison(command) == expected {
			return nil
		}
		return errArchitectContract("current work item %q requires verification command %q", item.ID, commandInArchitectCWD(item.CWD, item.Verify))
	}
	if item.Path == "" {
		return nil
	}
	cmd := filepath.ToSlash(strings.ToLower(command))
	path := filepath.ToSlash(strings.ToLower(item.Path))
	if item.CWD != "" && item.CWD != "." {
		if commandChangesIntoProjectRoot(cmd, strings.ToLower(item.CWD)) {
			if strings.Contains(cmd, path) {
				return nil
			}
		}
		full := filepath.ToSlash(strings.ToLower(filepath.Join(item.CWD, item.Path)))
		if strings.Contains(cmd, full) {
			return nil
		}
		return errArchitectContract("current work item %q requires path %q under cwd %q", item.ID, item.Path, item.CWD)
	}
	if strings.Contains(cmd, path) {
		return nil
	}
	return errArchitectContract("current work item %q requires path %q", item.ID, item.Path)
}

func errArchitectContract(format string, args ...interface{}) error {
	return &implementationArchitectValidationError{message: formatArchitectError(format, args...)}
}

type implementationArchitectValidationError struct {
	message string
}

func (e *implementationArchitectValidationError) Error() string {
	return e.message
}

func formatArchitectError(format string, args ...interface{}) string {
	return "architect contract violation: " + fmt.Sprintf(format, args...)
}
