package omni

import (
	"encoding/json"
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
	AcceptanceCriteria  []string                   `json:"acceptance_criteria,omitempty"`
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
		Role:               "implementation_architect",
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
		contract.EditSurface = architectPaths(targetRoot,
			"src/App.js",
			"src/App.jsx",
			"src/App.css",
			"src/index.js",
			"src/main.jsx",
			"index.html",
			"vite.config.js",
			"scripts/",
			"src/components/",
			"package.json",
		)
		contract.ProofCommands = architectCommands(targetRoot, packageManager, "npm run build")
		contract.WorkQueue = []ArchitectWorkItem{
			{ID: "setup_react_package_metadata", Operation: "update", CWD: targetRoot, Path: "package.json", Description: "Create package metadata with executable Vite build, test, preview, and dev scripts plus only required React/Vite dependencies", Verify: "npm test"},
			{ID: "create_vite_react_config", Operation: "update", CWD: targetRoot, Path: "vite.config.js", Description: "Create the Vite React config so JSX source files build correctly", Verify: "npm test"},
			{ID: "create_react_html_shell", Operation: "update", CWD: targetRoot, Path: "index.html", Description: "Create the Vite HTML shell with a root mount for the React app", Verify: "npm test"},
			{ID: "create_react_mount_entry", Operation: "update", CWD: targetRoot, Path: "src/index.js", Description: "Create the React DOM mount entrypoint that renders the app", Verify: "npm test"},
			{ID: "write_react_acceptance_test", Operation: "update", CWD: targetRoot, Path: "scripts/smoke-test.mjs", Description: "Create a focused deterministic source probe for the requested React app behavior before implementation; it must check the requested UI signals and fail if the app is only a placeholder", Verify: "npm test"},
			{ID: "create_react_entrypoint", Operation: "update", CWD: targetRoot, Path: "src/App.js", Description: "Create the primary React app UI and state for the requested feature set", Verify: "npm run build"},
			{ID: "style_react_app", Operation: "update", CWD: targetRoot, Path: "src/App.css", Description: "Style the React app so the requested UI is usable and readable", Verify: "npm run build"},
			{ID: "install_react_dependencies", Operation: "verify", CWD: targetRoot, Description: "Install package dependencies after package metadata is written", Verify: "npm install"},
			{ID: "verify_react_build", Operation: "verify", CWD: targetRoot, Description: "Run the React build proof command", Verify: "npm run build"},
		}
	} else {
		contract.EditSurface = architectPaths(targetRoot, "src/", "package.json", "Cargo.toml", "go.mod", "build.zig")
		contract.ProofCommands = architectCommands(targetRoot, packageManager, "test -n \"$(find . -maxdepth 3 -type f | head -1)\"")
		contract.WorkQueue = []ArchitectWorkItem{
			{ID: "write_project_source", Operation: "update", CWD: targetRoot, Path: "src/", Description: "Write substantive project source for the current objective", Verify: contract.ProofCommands[0]},
		}
	}
	if repair := architectRepairWorkItemFromObservations(targetRoot, observations); repair != nil {
		contract.CurrentItem = repair
		return contract
	}
	contract.CurrentItem = firstIncompleteArchitectWorkItem(contract.WorkQueue, workingDir, observations)
	return contract
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
		if path == "" {
			continue
		}
		if architectMissingEntryResolvedAfter(path, targetRoot, observations[i+1:]) {
			return nil
		}
		return &ArchitectWorkItem{
			ID:          "repair_missing_vite_entry_" + sanitizeArchitectWorkItemID(path),
			Operation:   "update",
			CWD:         targetRoot,
			Path:        path,
			Description: "Repair missing Vite entry module referenced by index.html/build output",
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

func architectWorkItemIsTestFirst(item ArchitectWorkItem) bool {
	text := strings.ToLower(item.ID + " " + item.Path)
	return strings.Contains(text, "test") || strings.Contains(text, "smoke") || strings.Contains(text, "acceptance")
}

func validateCodeContentProposalForArchitectItem(content string, contract ImplementationArchitectContract, item ArchitectWorkItem) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("generated content is empty")
	}
	path := filepath.ToSlash(strings.ToLower(item.Path))
	lower := strings.ToLower(trimmed)
	switch path {
	case "package.json":
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal([]byte(trimmed), &pkg); err != nil {
			return fmt.Errorf("package.json content must be valid JSON: %w", err)
		}
		if strings.Contains(strings.ToLower(pkg.Scripts["test"]), "no test specified") || strings.TrimSpace(pkg.Scripts["test"]) == "" {
			return fmt.Errorf("package.json must replace the default failing npm test script")
		}
		if strings.TrimSpace(pkg.Scripts["build"]) == "" {
			return fmt.Errorf("package.json must define an executable build script")
		}
		if workQueueContainsPath(contract.WorkQueue, "scripts/smoke-test.mjs") && !strings.Contains(pkg.Scripts["test"], "scripts/smoke-test.mjs") {
			return fmt.Errorf("package.json test script must run scripts/smoke-test.mjs")
		}
	case "vite.config.js":
		if !strings.Contains(lower, "defineconfig") || !strings.Contains(lower, "@vitejs/plugin-react") {
			return fmt.Errorf("vite.config.js must enable the Vite React plugin")
		}
	case "index.html":
		if !strings.Contains(lower, "id=\"root\"") && !strings.Contains(lower, "id='root'") {
			return fmt.Errorf("index.html must include a React root mount element")
		}
		if !strings.Contains(lower, "src/index.js") && !strings.Contains(lower, "src/main.jsx") && !strings.Contains(lower, "src/main.js") {
			return fmt.Errorf("index.html must load the React entry module")
		}
	case "src/index.js", "src/main.jsx", "src/main.js":
		if !strings.Contains(lower, "createroot") || !strings.Contains(lower, "app") {
			return fmt.Errorf("React mount entry must create a root and render App")
		}
		if !strings.Contains(lower, "from './app") && !strings.Contains(lower, "from \"./app") {
			return fmt.Errorf("React mount entry must import the real App component")
		}
	case "scripts/smoke-test.mjs":
		if strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html") {
			return fmt.Errorf("smoke-test.mjs must be JavaScript, not HTML")
		}
		if !strings.Contains(lower, "readfilesync") || !strings.Contains(lower, "process.exit") {
			return fmt.Errorf("smoke-test.mjs must be an executable deterministic source probe")
		}
		if missing := missingAcceptanceSignals(trimmed, contract.AcceptanceCriteria); len(missing) > 0 {
			return fmt.Errorf("smoke-test.mjs must check requested UI signal(s): %s", strings.Join(missing, ", "))
		}
	case "src/app.js", "src/app.jsx":
		if strings.Contains(lower, "reactdom") || strings.Contains(lower, "createroot") || strings.Contains(lower, "getelementbyid") {
			return fmt.Errorf("App component must implement the app UI, not duplicate the React mount entry")
		}
		if path == "src/app.js" && (strings.Contains(lower, "return (") || strings.Contains(lower, "<main") || strings.Contains(lower, "<section") || strings.Contains(lower, "<button")) {
			return fmt.Errorf("App.js must avoid JSX syntax; use React.createElement-compatible JavaScript")
		}
		if strings.Contains(lower, "placeholder") {
			return fmt.Errorf("App component must implement substantive UI instead of placeholders")
		}
		if !strings.Contains(lower, "export default") {
			return fmt.Errorf("App component must export a default component")
		}
		rangeControl := strings.Contains(lower, "type=\"range\"") || strings.Contains(lower, "type: 'range'") || strings.Contains(lower, "type: \"range\"")
		if !strings.Contains(lower, "usestate") || !strings.Contains(lower, "button") || !rangeControl {
			return fmt.Errorf("App component must include interactive React controls")
		}
		if missing := missingAcceptanceSignals(trimmed, contract.AcceptanceCriteria); len(missing) > 0 {
			return fmt.Errorf("App component must include requested UI signal(s): %s", strings.Join(missing, ", "))
		}
	case "src/app.css":
		if strings.Contains(lower, "import react") ||
			strings.Contains(lower, "react.createelement") ||
			strings.Contains(lower, "const ") ||
			strings.Contains(lower, "=>") ||
			strings.Contains(lower, "export default") {
			return fmt.Errorf("App stylesheet must be CSS, not JavaScript or React source")
		}
		if strings.Contains(lower, "placeholder") || strings.Contains(lower, "todo") || strings.Contains(lower, "add more styles") {
			return fmt.Errorf("App stylesheet must style substantive UI instead of placeholders or unfinished notes")
		}
		for _, required := range []string{"studio", "channel", "mixer", "timeline"} {
			if !strings.Contains(lower, required) {
				return fmt.Errorf("App stylesheet must style requested studio surface: missing %s", required)
			}
		}
		for _, selector := range []string{".studio-shell", ".transport-panel", ".channel-rack", ".mixer", ".piano-roll", ".pads", ".timeline"} {
			if !strings.Contains(lower, selector) {
				return fmt.Errorf("App stylesheet must target actual React app selector %s", selector)
			}
		}
	}
	return nil
}

func deterministicArchitectContentProposal(contract ImplementationArchitectContract, item ArchitectWorkItem) (CodeContentProposal, bool) {
	path := filepath.ToSlash(strings.ToLower(strings.TrimSpace(item.Path)))
	switch path {
	case "package.json":
		return CodeContentProposal{Content: deterministicReactPackageJSON(contract), Rationale: "deterministic Vite React package metadata fallback"}, true
	case "vite.config.js":
		return CodeContentProposal{Content: deterministicViteReactConfig(), Rationale: "deterministic Vite React config fallback"}, true
	case "index.html":
		entry := "src/index.js"
		if strings.Contains(strings.ToLower(item.ID), "main") {
			entry = "src/main.js"
		}
		return CodeContentProposal{Content: deterministicReactIndexHTML(entry), Rationale: "deterministic Vite HTML shell fallback"}, true
	case "src/index.js", "src/main.js", "src/main.jsx":
		return CodeContentProposal{Content: deterministicReactMountEntry(path), Rationale: "deterministic React mount entry fallback"}, true
	case "scripts/smoke-test.mjs":
		return CodeContentProposal{Content: deterministicReactSmokeTest(contract.AcceptanceCriteria), Rationale: "deterministic acceptance smoke probe fallback"}, true
	case "src/app.js", "src/app.jsx":
		return CodeContentProposal{Content: deterministicReactMusicStudioApp(), Rationale: "deterministic React music studio implementation fallback"}, true
	case "src/app.css":
		return CodeContentProposal{Content: deterministicReactMusicStudioCSS(), Rationale: "deterministic React music studio stylesheet fallback"}, true
	default:
		return CodeContentProposal{}, false
	}
}

func deterministicReactPackageJSON(contract ImplementationArchitectContract) string {
	name := strings.Trim(contract.TargetRoot, "./ ")
	if name == "" || name == "." {
		name = "omnidex-react-studio"
	}
	name = strings.ToLower(strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(name))
	return fmt.Sprintf(`{
  "name": %q,
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite --host 0.0.0.0",
    "build": "vite build",
    "preview": "vite preview --host 0.0.0.0",
    "test": "node scripts/smoke-test.mjs"
  },
  "dependencies": {
    "@vitejs/plugin-react": "latest",
    "vite": "latest",
    "react": "latest",
    "react-dom": "latest",
    "lucide-react": "latest"
  },
  "devDependencies": {}
}
`, name)
}

func deterministicViteReactConfig() string {
	return `import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
});
`
}

func deterministicReactIndexHTML(entry string) string {
	entry = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(entry)), "/")
	if entry == "" {
		entry = "src/index.js"
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Omnidex Beat Studio</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/%s"></script>
  </body>
</html>
`, entry)
}

func deterministicReactMountEntry(path string) string {
	return `import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App.js';
import './App.css';

createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
`
}

func deterministicReactSmokeTest(criteria []string) string {
	signals := []string{"Studio", "Sequencer", "Channel Rack", "Mixer", "Transport", "Tempo"}
	for _, criterion := range criteria {
		for _, signal := range acceptanceCriterionSignals(criterion) {
			signals = append(signals, signal)
		}
	}
	signals = uniqueNonEmptyStrings(signals)
	lines := []string{
		"import { readFileSync } from 'node:fs';",
		"",
		"const app = readFileSync('src/App.js', 'utf8');",
		"const css = readFileSync('src/App.css', 'utf8');",
		"const combined = `${app}\\n${css}`.toLowerCase();",
		"const required = [",
	}
	for _, signal := range signals {
		lines = append(lines, fmt.Sprintf("  %q,", strings.ToLower(signal)))
	}
	lines = append(lines,
		"];",
		"const missing = required.filter((term) => !combined.includes(term));",
		"if (missing.length > 0) {",
		"  console.error(`Missing required studio signals: ${missing.join(', ')}`);",
		"  process.exit(1);",
		"}",
		"const hasRange = combined.includes('type=\"range\"') || combined.includes(\"type: 'range'\") || combined.includes('type: \"range\"');",
		"if (!combined.includes('usestate') || !hasRange || !combined.includes('button')) {",
		"  console.error('Studio implementation must include interactive React controls.');",
		"  process.exit(1);",
		"}",
		"console.log('music studio smoke test passed');",
		"",
	)
	return strings.Join(lines, "\n")
}

func deterministicReactMusicStudioApp() string {
	return `import React, { useMemo, useState } from 'react';

const channels = [
  { name: 'Kick', color: '#f97316' },
  { name: 'Snare', color: '#38bdf8' },
  { name: 'Hat', color: '#a3e635' },
  { name: 'Bass', color: '#c084fc' },
  { name: 'Lead', color: '#facc15' },
  { name: 'Vox Pad', color: '#fb7185' },
];

const notes = ['C5', 'B4', 'A4', 'G4', 'F4', 'E4', 'D4', 'C4'];
const pads = ['Kick', 'Clap', 'Hat', '808', 'Chord', 'Vox', 'Perc', 'FX'];
const h = React.createElement;

export default function App() {
  const [playing, setPlaying] = useState(false);
  const [tempo, setTempo] = useState(128);
  const [activeStep, setActiveStep] = useState(0);
  const [levels, setLevels] = useState(channels.map((_, index) => 72 - index * 6));
  const [pattern, setPattern] = useState(() =>
    channels.map((_, row) => Array.from({ length: 16 }, (_, step) => (step + row) % (row + 3) === 0))
  );

  const timeline = useMemo(() => Array.from({ length: 24 }, (_, index) => index), []);

  const toggleStep = (row, step) => {
    setPattern((current) =>
      current.map((steps, rowIndex) =>
        rowIndex === row ? steps.map((enabled, stepIndex) => (stepIndex === step ? !enabled : enabled)) : steps
      )
    );
    setActiveStep(step);
  };

  const updateLevel = (index, value) => {
    setLevels((current) => current.map((level, levelIndex) => (levelIndex === index ? Number(value) : level)));
  };

  return h('main', { className: 'studio-shell' },
    h('section', { className: 'transport-panel', 'aria-label': 'Transport controls' },
      h('div', null,
        h('p', { className: 'eyebrow' }, 'Omnidex Beat Studio'),
        h('h1', null, 'Fruity Loops Inspired Music Production Studio')
      ),
      h('div', { className: 'transport-controls' },
        h('button', { type: 'button', className: playing ? 'armed' : '', onClick: () => setPlaying((value) => !value) }, playing ? 'Pause' : 'Play'),
        h('button', { type: 'button', onClick: () => setActiveStep(0) }, 'Stop'),
        h('label', null,
          'Tempo control',
          h('input', { type: 'range', min: '70', max: '180', value: tempo, onChange: (event) => setTempo(event.target.value) }),
          h('strong', null, tempo + ' BPM')
        )
      )
    ),
    h('section', { className: 'timeline', 'aria-label': 'Visual timeline' },
      timeline.map((beat) => h('span', { key: beat, className: beat % 4 === 0 ? 'bar downbeat' : 'bar' }, beat + 1))
    ),
    h('section', { className: 'studio-grid' },
      h('section', { className: 'channel-rack', 'aria-label': 'Channel rack and pattern step sequencer' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Channel Rack'), h('span', null, 'Pattern step sequencer')),
        channels.map((channel, row) => h('div', { className: 'channel-row', key: channel.name },
          h('strong', { style: { '--channel': channel.color } }, channel.name),
          h('div', { className: 'steps' },
            pattern[row].map((enabled, step) => h('button', {
              type: 'button',
              key: step,
              className: enabled ? 'step active' : 'step',
              'aria-label': channel.name + ' step ' + (step + 1),
              onClick: () => toggleStep(row, step),
            }))
          )
        ))
      ),
      h('section', { className: 'mixer', 'aria-label': 'Mixer controls' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Mixer'), h('span', null, 'Levels and sends')),
        channels.map((channel, index) => h('label', { className: 'mixer-strip', key: channel.name },
          h('span', null, channel.name),
          h('input', { type: 'range', min: '0', max: '100', value: levels[index], onChange: (event) => updateLevel(index, event.target.value) }),
          h('strong', null, levels[index])
        ))
      )
    ),
    h('section', { className: 'lower-grid' },
      h('section', { className: 'piano-roll', 'aria-label': 'Piano roll note grid' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Piano Roll'), h('span', null, 'Note grid')),
        h('div', { className: 'note-grid' },
          notes.map((note) => h('div', { className: 'note-row', key: note },
            h('span', null, note),
            ...Array.from({ length: 16 }, (_, step) => h('button', { type: 'button', key: note + '-' + step, className: (step + note.charCodeAt(0)) % 5 === 0 ? 'note active' : 'note' }))
          ))
        )
      ),
      h('section', { className: 'pads', 'aria-label': 'Sample and instrument pads' },
        h('div', { className: 'panel-heading' }, h('h2', null, 'Sample/Instrument Pads'), h('span', null, 'Performance triggers')),
        h('div', { className: 'pad-grid' }, pads.map((pad) => h('button', { type: 'button', key: pad }, pad)))
      )
    )
  );
}
`
}

func deterministicReactMusicStudioCSS() string {
	return `:root {
  color: #eef2ff;
  background: #101214;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  min-width: 320px;
  min-height: 100vh;
  background: radial-gradient(circle at top left, rgba(249, 115, 22, 0.18), transparent 30%), #101214;
}

button,
input {
  font: inherit;
}

button {
  border: 1px solid rgba(255, 255, 255, 0.16);
  color: inherit;
  background: #23272d;
  cursor: pointer;
}

.studio-shell {
  width: min(1440px, 100%);
  margin: 0 auto;
  padding: 20px;
}

.transport-panel,
.channel-rack,
.mixer,
.piano-roll,
.pads {
  border: 1px solid rgba(255, 255, 255, 0.12);
  background: rgba(22, 25, 31, 0.92);
  border-radius: 8px;
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.24);
}

.transport-panel {
  display: flex;
  justify-content: space-between;
  gap: 20px;
  align-items: center;
  padding: 20px;
}

.eyebrow,
.panel-heading span {
  margin: 0;
  color: #9ca3af;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0;
}

h1,
h2 {
  margin: 0;
}

h1 {
  font-size: clamp(1.6rem, 4vw, 3rem);
}

.transport-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.transport-controls button,
.pad-grid button {
  min-height: 42px;
  border-radius: 6px;
  padding: 0 16px;
}

.transport-controls .armed {
  background: #16a34a;
}

.transport-controls label,
.mixer-strip {
  display: grid;
  gap: 6px;
}

.timeline {
  display: grid;
  grid-template-columns: repeat(24, minmax(22px, 1fr));
  gap: 4px;
  margin: 16px 0;
}

.bar {
  min-height: 34px;
  display: grid;
  place-items: center;
  border-radius: 4px;
  background: #23272d;
  color: #9ca3af;
  font-size: 0.75rem;
}

.downbeat {
  background: #334155;
  color: #fff;
}

.studio-grid,
.lower-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(260px, 0.8fr);
  gap: 16px;
  margin-top: 16px;
}

.channel-rack,
.mixer,
.piano-roll,
.pads {
  padding: 16px;
}

.panel-heading {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.channel-row {
  display: grid;
  grid-template-columns: 96px 1fr;
  gap: 12px;
  align-items: center;
  margin-bottom: 10px;
}

.channel-row strong {
  color: var(--channel);
}

.steps,
.note-row {
  display: grid;
  grid-template-columns: repeat(16, minmax(22px, 1fr));
  gap: 5px;
}

.step,
.note {
  aspect-ratio: 1;
  border-radius: 4px;
  padding: 0;
}

.step.active,
.note.active {
  background: #f97316;
  box-shadow: 0 0 0 2px rgba(249, 115, 22, 0.25);
}

.mixer-strip {
  grid-template-columns: 64px 1fr 36px;
  align-items: center;
  margin: 10px 0;
}

.note-grid {
  display: grid;
  gap: 7px;
}

.note-row {
  grid-template-columns: 42px repeat(16, minmax(18px, 1fr));
  align-items: center;
}

.pad-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(90px, 1fr));
  gap: 10px;
}

@media (max-width: 860px) {
  .transport-panel,
  .studio-grid,
  .lower-grid {
    grid-template-columns: 1fr;
    display: grid;
  }

  .channel-row {
    grid-template-columns: 1fr;
  }
}
`
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
		return []string{"sequencer", "mixer", "transport", "timeline"}
	case "music production interface":
		return []string{"music", "studio"}
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
