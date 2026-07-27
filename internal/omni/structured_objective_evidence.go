package omni

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func reconcileStructuredObjectiveLedgerFromObservation(step int, ledger []StructuredObjective, obs StructuredCommandObservation, onEvent func(StructuredCommandEvent)) []StructuredObjective {
	if strings.TrimSpace(obs.Command) == "" || obs.ExitCode != 0 {
		return ledger
	}
	pending := pendingStructuredObjectives(ledger)
	if len(pending) == 0 {
		return ledger
	}
	satisfied := []StructuredObjective{}
	for _, objective := range pending {
		if structuredObservationSatisfiesObjective(obs, objective) {
			objective.Status = "satisfied"
			objective.Evidence = structuredObjectiveEvidenceFromObservation(obs)
			satisfied = append(satisfied, objective)
		}
	}
	if len(satisfied) == 0 {
		return ledger
	}
	ids := structuredObjectiveIDs(satisfied)
	emitStructuredCommandEvent(onEvent, "objective_ledger_reconciled", "Pending objective(s) satisfied from prior successful command evidence", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"objectives": strings.Join(ids, ","),
	})
	return mergeStructuredObjectiveLedger(ledger, satisfied)
}

func acceptPartialCompletionForContinuation(step int, before, after []StructuredObjective, obs StructuredCommandObservation, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) {
	if result == nil || obs.ExitCode != 0 {
		return
	}
	pendingBefore := pendingStructuredObjectives(before)
	pendingAfter := pendingStructuredObjectives(after)
	if len(pendingBefore) == 0 || len(pendingAfter) == 0 || len(pendingAfter) >= len(pendingBefore) {
		return
	}
	completed := newlySatisfiedStructuredObjectiveIDs(before, after)
	if len(completed) == 0 {
		return
	}
	result.PartialProgress = true
	emitStructuredCommandEvent(onEvent, "partial_completion_accepted", "Partial completion accepted; continuing remaining objectives", map[string]string{
		"step":                 fmt.Sprintf("%d", step),
		"completed_objectives": strings.Join(completed, ","),
		"pending_objectives":   strings.Join(structuredObjectiveIDs(pendingAfter), ","),
		"command":              truncateStructuredTimelineValue(obs.Command),
		"continuation":         "same job must continue before unrelated work or done=true",
	})
}

func newlySatisfiedStructuredObjectiveIDs(before, after []StructuredObjective) []string {
	beforeSatisfied := map[string]bool{}
	for _, objective := range before {
		if structuredObjectiveSatisfied(objective) {
			beforeSatisfied[objective.ID] = true
		}
	}
	ids := []string{}
	for _, objective := range after {
		if objective.ID == "" || beforeSatisfied[objective.ID] || !structuredObjectiveSatisfied(objective) {
			continue
		}
		ids = append(ids, objective.ID)
	}
	sort.Strings(ids)
	return ids
}

func structuredObservationSatisfiesObjective(obs StructuredCommandObservation, objective StructuredObjective) bool {
	if len(objective.RequiredEvidence) > 0 {
		return structuredObjectiveRequiredEvidenceSatisfied(objective, []StructuredCommandObservation{obs}, obs.CWD)
	}
	command := strings.ToLower(strings.TrimSpace(obs.Command))
	output := strings.ToLower(obs.Stdout + "\n" + obs.Stderr)
	target := normalizedDependencyText(objective.ID + " " + objective.Description)
	if command == "" || target == "" {
		return false
	}
	if strings.Contains(target, emptyProjectFileObjectiveID) || strings.Contains(target, "empty project file") || strings.Contains(target, "empty placeholder") {
		if strings.HasPrefix(command, "empty_file.apply ") {
			return true
		}
		if strings.HasPrefix(command, "empty_file.verify") && strings.Contains(output, "remaining_empty_files=0") {
			return true
		}
	}
	if strings.Contains(command, "mkdir") && (strings.Contains(target, " setup ") || strings.Contains(target, " structure ")) {
		return true
	}
	if scaffoldObservationSatisfiesObjective(command, output, target, obs.CWD) {
		return true
	}
	if structuredObservationSatisfiesSourceWriteObjective(command, target) {
		return true
	}
	if (strings.Contains(command, "rm ") || strings.Contains(command, "rm -f ")) &&
		(strings.Contains(target, " remove ") || strings.Contains(target, " delete ") || strings.Contains(target, " cleanup ") || strings.Contains(target, " clean up ")) {
		return true
	}
	if strings.Contains(command, "npm install") || strings.Contains(command, "npm add") || strings.Contains(command, "pnpm add") || strings.Contains(command, "yarn add") {
		for _, pkg := range objective.Packages {
			if strings.Contains(command, strings.ToLower(pkg)) {
				return true
			}
		}
	}
	if objectiveRequiresDockerLifecycle(target) {
		return dockerLifecycleEvidenceSatisfiesObjective(command, output, target)
	}
	if objectiveRequiresBackendTest(target) {
		return (strings.Contains(command, "go test") || strings.Contains(command, "make test") || strings.Contains(output, "go test")) &&
			(strings.Contains(command, "backend") || strings.Contains(command, "calculus-api") || strings.Contains(output, "calculus-api"))
	}
	if objectiveRequiresFrontendTest(target) {
		return (strings.Contains(command, "npm test") || strings.Contains(command, "make test") || strings.Contains(output, "react-scripts test")) &&
			(strings.Contains(command, "frontend") || strings.Contains(command, "make test") || strings.Contains(output, "react-scripts test"))
	}
	if objectiveRequiresSmokeTest(target) {
		return strings.Contains(command, "smoke") || strings.Contains(command, "make test") && strings.Contains(output, "smoke test passed") || strings.Contains(output, "smoke test passed")
	}
	if objectiveRequiresFrontendBuild(target) {
		return (strings.Contains(command, "npm run build") || strings.Contains(command, "make build") || strings.Contains(output, "compiled successfully")) &&
			!strings.Contains(command, "go test")
	}
	if objectiveRequiresReactUIBuildEvidence(target) {
		return strings.Contains(command, "npm run build") &&
			(strings.Contains(output, "built in") || strings.Contains(output, "✓ built") || strings.Contains(output, "dist/"))
	}
	if strings.Contains(target, "entrypoint") || strings.Contains(target, "entry point") {
		return strings.Contains(command, "npm run build") &&
			(strings.Contains(output, "built in") || strings.Contains(output, "✓ built") || strings.Contains(output, "dist/"))
	}
	if strings.Contains(command, "npm run build") && (strings.Contains(target, " verify ") || strings.Contains(target, " build ")) {
		return true
	}
	if strings.Contains(command, "npm test") && (strings.Contains(target, " verify ") || strings.Contains(target, " test ")) {
		return true
	}
	if strings.Contains(command, "go test") && (strings.Contains(target, " verify ") || strings.Contains(target, " test ")) && !strings.Contains(target, "frontend") {
		return true
	}
	return false
}

func structuredObjectiveRequiredEvidenceSatisfied(objective StructuredObjective, observations []StructuredCommandObservation, workingDir string) bool {
	if len(objective.RequiredEvidence) == 0 {
		return false
	}
	for _, predicate := range objective.RequiredEvidence {
		if !structuredEvidencePredicateSatisfied(predicate, observations, workingDir) {
			return false
		}
	}
	return true
}

func structuredEvidencePredicateSatisfied(predicate string, observations []StructuredCommandObservation, workingDir string) bool {
	predicate = strings.TrimSpace(predicate)
	if predicate == "" {
		return true
	}
	kind, rest, ok := strings.Cut(predicate, ":")
	if !ok {
		kind = predicate
		rest = ""
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	rest = strings.TrimSpace(rest)
	switch kind {
	case "command_passed":
		want := normalizeStructuredCommandForComparison(rest)
		for _, obs := range observations {
			if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
				continue
			}
			got := normalizeStructuredCommandForComparison(obs.Command)
			if got == want || (want != "" && strings.Contains(got, want)) || (want != "" && strings.Contains(strings.ToLower(obs.Stdout+"\n"+obs.Stderr), strings.ToLower(rest))) {
				return true
			}
		}
	case "file_exists":
		return workingDir != "" && fileExists(filepath.Join(workingDir, filepath.Clean(rest)))
	case "project_directory_exists":
		if rest == "" {
			return workingDir != "" && dirExists(workingDir)
		}
		return workingDir != "" && dirExists(filepath.Join(workingDir, filepath.Clean(rest)))
	case "package_json_exists":
		root := workingDir
		if rest != "" {
			root = filepath.Join(workingDir, filepath.Clean(rest))
		}
		return root != "" && fileExists(filepath.Join(root, "package.json"))
	case "src_directory_exists":
		root := workingDir
		if rest != "" {
			root = filepath.Join(workingDir, filepath.Clean(rest))
		}
		return root != "" && dirExists(filepath.Join(root, "src"))
	case "entrypoint_exists":
		root := workingDir
		if rest != "" {
			root = filepath.Join(workingDir, filepath.Clean(rest))
		}
		for _, rel := range []string{"src/index.js", "src/index.jsx", "src/main.jsx", "src/main.tsx"} {
			if root != "" && fileExists(filepath.Join(root, rel)) {
				return true
			}
		}
		return false
	case "app_component_exists":
		root := workingDir
		if rest != "" {
			root = filepath.Join(workingDir, filepath.Clean(rest))
		}
		for _, rel := range []string{"src/App.js", "src/App.jsx", "src/App.tsx"} {
			if root != "" && fileExists(filepath.Join(root, rel)) {
				return true
			}
		}
		return false
	case "dependencies_declared_or_installed":
		root := workingDir
		if rest != "" {
			root = filepath.Join(workingDir, filepath.Clean(rest))
		}
		return root != "" && (fileExists(filepath.Join(root, "package.json")) || dirExists(filepath.Join(root, "node_modules")))
	case "file_absent":
		return workingDir != "" && !fileExists(filepath.Join(workingDir, filepath.Clean(rest)))
	case "file_nonempty":
		return workingDir != "" && fileHasContent(filepath.Join(workingDir, filepath.Clean(rest)))
	case "file_contains":
		path, needle, ok := strings.Cut(rest, ":")
		if !ok || workingDir == "" {
			return false
		}
		content, err := os.ReadFile(filepath.Join(workingDir, filepath.Clean(strings.TrimSpace(path))))
		return err == nil && strings.Contains(string(content), needle)
	case "no_js_files_with_jsx":
		return workingDir != "" && noJSFilesContainJSX(filepath.Join(workingDir, filepath.Clean(firstNonEmpty(rest, "."))))
	case "package_script_exists":
		if workingDir == "" {
			return false
		}
		content, err := os.ReadFile(filepath.Join(workingDir, "package.json"))
		if err != nil {
			return false
		}
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		return json.Unmarshal(content, &pkg) == nil && strings.TrimSpace(pkg.Scripts[rest]) != ""
	case "source_verification", "source_verification_passed":
		for _, obs := range observations {
			if runtimeSourceVerificationObservation(obs) {
				return true
			}
		}
	case "artifact_validation_passed":
		for _, obs := range observations {
			if obs.ExitCode == 0 && strings.EqualFold(strings.TrimSpace(obs.EvidenceKind), "artifact_validation") && strings.EqualFold(strings.TrimSpace(obs.GeneratedBy), "runtime") {
				return true
			}
		}
	case "tdd_contract_passed", "smoke_check_passed":
		for _, obs := range observations {
			if obs.ExitCode == 0 && strings.EqualFold(strings.TrimSpace(obs.EvidenceKind), kind) {
				if rest == "" || strings.Contains(strings.ToLower(obs.Command+" "+obs.Stdout+" "+obs.Stderr), strings.ToLower(rest)) {
					return true
				}
			}
		}
	}
	return false
}

func viteJSXSyntaxDisabledPath(output string) string {
	lower := strings.ToLower(output)
	if !(strings.Contains(lower, "jsx syntax") && (strings.Contains(lower, "not currently enabled") || strings.Contains(lower, "disabled")) ||
		strings.Contains(lower, "unexpected jsx expression")) {
		return ""
	}
	for _, token := range strings.Fields(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ", "(", " ", ")", " ", "[", " ", "]", " ", "\"", " ", "'", " ").Replace(output)) {
		clean := strings.Trim(token, ":,;")
		slash := filepath.ToSlash(clean)
		idx := strings.Index(slash, "src/")
		if idx >= 0 {
			slash = slash[idx:]
		}
		if jsIdx := strings.Index(strings.ToLower(slash), ".js"); jsIdx >= 0 {
			slash = slash[:jsIdx+len(".js")]
		}
		if strings.HasPrefix(slash, "src/") && strings.HasSuffix(strings.ToLower(slash), ".js") {
			return slash
		}
		if strings.HasPrefix(slash, "/") && strings.HasSuffix(strings.ToLower(slash), ".js") {
			if srcIdx := strings.Index(slash, "/src/"); srcIdx >= 0 {
				return strings.TrimPrefix(slash[srcIdx:], "/")
			}
		}
	}
	return ""
}

func noJSFilesContainJSX(root string) bool {
	return len(jsFilesWithJSX(root, 1)) == 0
}

func jsFilesWithJSX(root string, limit int) []string {
	root = filepath.Clean(root)
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "node_modules", ".git", "dist", "build", "coverage":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.EqualFold(filepath.Ext(path), ".js") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if jsSourceLooksLikeJSX(string(content)) {
			matches = append(matches, path)
			if limit > 0 && len(matches) >= limit {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return matches
}

func jsSourceLooksLikeJSX(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if strings.Contains(trimmed, "<") && strings.Contains(trimmed, ">") {
			if strings.Contains(trimmed, "=>") || strings.Contains(trimmed, "<=") || strings.Contains(trimmed, ">=") || strings.Contains(trimmed, "!==") || strings.Contains(trimmed, "===") {
				continue
			}
			return true
		}
	}
	return false
}

func structuredObservationSatisfiesSourceWriteObjective(command, target string) bool {
	if !structuredCommandLooksAppFileMutation(command) {
		return false
	}
	lower := strings.ToLower(command)
	identifierText := normalizedIdentifierText(lower)
	if strings.Contains(target, " context ") {
		return strings.Contains(identifierText, "createcontext") ||
			strings.Contains(identifierText, "notescontext") ||
			strings.Contains(identifierText, "notesprovider") ||
			strings.Contains(identifierText, "usenotes")
	}
	if strings.Contains(target, " appjs ") || strings.Contains(target, " app js ") || strings.Contains(target, " app ") {
		return (strings.Contains(lower, "app.js") || strings.Contains(lower, "app.jsx") || strings.Contains(lower, "src/app")) &&
			(strings.Contains(identifierText, "notesprovider") || strings.Contains(identifierText, "notescontext") || strings.Contains(identifierText, "usenotes"))
	}
	if strings.Contains(target, " noteslist ") ||
		strings.Contains(target, " notelist ") ||
		(strings.Contains(target, " note ") && strings.Contains(target, " list ") && strings.Contains(target, " component ")) {
		return (strings.Contains(lower, "noteslist.js") || strings.Contains(lower, "noteslist.jsx") || strings.Contains(lower, "notelist.js") || strings.Contains(lower, "notelist.jsx")) &&
			(strings.Contains(identifierText, "noteslist") || strings.Contains(identifierText, "notelist"))
	}
	if (strings.Contains(target, " add ") && strings.Contains(target, " delete ")) ||
		(strings.Contains(target, " create ") && strings.Contains(target, " delete ")) {
		return strings.Contains(identifierText, "addnote") && strings.Contains(identifierText, "deletenote")
	}
	if strings.Contains(target, " crud ") {
		matches := 0
		for _, marker := range []string{"addnote", "createnote", "deletenote", "editnote", "updatenote"} {
			if strings.Contains(identifierText, marker) {
				matches++
			}
		}
		return matches >= 2
	}
	if strings.Contains(target, " memory ") || strings.Contains(target, " in memory ") || strings.Contains(target, " state ") {
		return strings.Contains(identifierText, "usestate") && strings.Contains(identifierText, "setnotes")
	}
	return false
}

func normalizedIdentifierText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func objectiveRequiresBackendTest(target string) bool {
	return strings.Contains(target, "backend") && (strings.Contains(target, "test") || strings.Contains(target, "verify"))
}

func objectiveRequiresFrontendTest(target string) bool {
	return strings.Contains(target, "frontend") && strings.Contains(target, "test")
}

func objectiveRequiresSmokeTest(target string) bool {
	return strings.Contains(target, "smoke")
}

func objectiveRequiresFrontendBuild(target string) bool {
	return strings.Contains(target, "frontend") && strings.Contains(target, "build")
}

func objectiveRequiresReactUIBuildEvidence(target string) bool {
	for _, marker := range []string{
		"step sequencer",
		"sequencer",
		"channel rack",
		"mixer",
		"transport",
		"tempo",
		"piano roll",
		"note grid",
		"sample",
		"instrument pad",
		"timeline",
		"studio ui",
		"production studio",
	} {
		if strings.Contains(target, marker) {
			return true
		}
	}
	return false
}

func objectiveRequiresDockerLifecycle(target string) bool {
	return strings.Contains(target, "docker") || strings.Contains(target, "container")
}

func dockerLifecycleEvidenceSatisfiesObjective(command, output, target string) bool {
	if strings.Contains(target, "dockerfile") || strings.Contains(target, "dependencies") || strings.Contains(target, "compatibility") {
		return strings.Contains(command, "docker build") || strings.Contains(output, "successfully built") || strings.Contains(output, "writing image") || strings.Contains(output, "naming to")
	}
	if strings.Contains(target, "build") || strings.Contains(target, "image") {
		return strings.Contains(command, "docker build") || strings.Contains(output, "successfully built") || strings.Contains(output, "writing image") || strings.Contains(output, "naming to")
	}
	if strings.Contains(target, "run") || strings.Contains(target, "container") {
		hasRun := strings.Contains(command, "docker run") || strings.Contains(output, "docker run") || strings.Contains(output, "running=true") || strings.Contains(output, "restart_count=0")
		hasInspect := strings.Contains(command, "docker inspect") || strings.Contains(output, "running=true") || strings.Contains(output, "restart_count=0")
		hasLogs := strings.Contains(command, "docker logs") || strings.Contains(output, "docker_logs_clear") || strings.Contains(output, "logs clear")
		hasHealth := strings.Contains(command, "curl") || strings.Contains(output, "health=") || strings.Contains(output, "http")
		return hasRun && hasInspect && hasLogs && hasHealth
	}
	return false
}
