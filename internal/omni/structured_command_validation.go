package omni

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateStructuredCommandString(command string) error {
	command = normalizeStructuredCommand(command)
	if structuredCommandLooksLikeMultilinePackageManagerScript(command) {
		return fmt.Errorf("multiline package-manager scripts are blocked; choose one concrete package/build command for the next objective")
	}
	if startsWithShellRedirectionToken(command) {
		return fmt.Errorf("command starts with shell redirection token")
	}
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(command)
	switch lower {
	case "none", "null", "n/a", "no command":
		return fmt.Errorf("command is not executable shell evidence")
	}
	for _, pseudoTool := range []string{"web.search", "browser.search", "search_web", "internet.search"} {
		if strings.HasPrefix(lower, pseudoTool) {
			return fmt.Errorf("%s is not a shell command; use curl with a public source such as news.google.com/rss/search or duckduckgo.com/html", strings.Fields(trimmed)[0])
		}
	}
	if isNonEvidenceShellCommand(command) {
		return fmt.Errorf("command is a shell/no-op launcher without task-specific side effects or output")
	}
	if structuredCommandUsesRecursiveForceRemove(command) {
		return fmt.Errorf("recursive force removal is blocked; use non-destructive creation/update commands or ask for explicit deletion approval")
	}
	if strings.Contains(lower, "openweathermap") || strings.Contains(lower, "api.openweathermap.org") {
		return fmt.Errorf("OpenWeatherMap requires an API key; use no-key wttr.in with an explicit location path and concise format query instead")
	}
	if err := validateGoogleNewsRSSCommand(command); err != nil {
		return err
	}
	if structuredCommandLooksLikeOSIdentification(command) && !structuredCommandDiscoversPackageManager(command) {
		return fmt.Errorf("OS identification command must include package-manager discovery with command -v pacman apt dnf yum zypper apk")
	}
	for _, placeholder := range []string{
		"<location>", "<query>", "<file>", "<filename>", "<path>", "<url>", "<number>", "<name>", "<project>",
		"<city>", "<country>", "<timezone>", "<api_key>", "<token>", "<placeholder>",
	} {
		if strings.Contains(lower, placeholder) {
			return fmt.Errorf("placeholder angle-bracket value in command")
		}
	}
	if containsLiteralPlaceholderProjectPath(command) {
		return fmt.Errorf("literal placeholder project path in command")
	}
	if strings.Contains(lower, "your_api_key") || strings.Contains(lower, "api_key_here") {
		return fmt.Errorf("placeholder angle-bracket value in command")
	}
	if isPureEchoCommand(command) {
		return fmt.Errorf("pure echo command is not command evidence")
	}
	if isPurePrintCommand(command) && commandPrintsFalseCapabilityLimitation(command) {
		return fmt.Errorf("print-only false capability limitation is not command evidence; gather real tool or public-source evidence instead")
	}
	if err := validateWTTRCommand(command); err != nil {
		return err
	}
	if err := validateDateCommand(command); err != nil {
		return err
	}
	return nil
}

func containsLiteralPlaceholderProjectPath(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"/path/to/project",
		"/your/project",
		"/path/to/",
		"your_project",
		"todo_path",
		"replace_me",
		"{project}",
		"${project}",
		"$project",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, marker := range []string{"YOUR_PROJECT", "TODO_PATH", "REPLACE_ME"} {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func isPlaceholderPathValidationError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "placeholder project path")
}

func normalizeStructuredCommand(command string) string {
	command = normalizeStructuredCommandLineBreaks(command)
	command = normalizeStructuredMkdirParents(command)
	command = normalizePersistentCDScaffoldCommand(command)
	return command
}

func normalizePersistentCDScaffoldCommand(command string) string {
	spec, ok := scaffoldCommandSpec(command, "")
	if !ok || strings.TrimSpace(spec.TrailingCD) == "" {
		return command
	}
	return spec.Command
}

func normalizeStructuredCommandLineBreaks(command string) string {
	command = strings.TrimSpace(command)
	if !strings.ContainsAny(command, "\n\r") || strings.Contains(command, "<<") {
		return command
	}
	parts := []string{}
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	for _, r := range command {
		if r == '\r' {
			continue
		}
		if r == '\n' && !inSingleQuote && !inDoubleQuote {
			part := strings.TrimSpace(current.String())
			if part != "" {
				parts = append(parts, part)
			}
			current.Reset()
			escaped = false
			continue
		}
		current.WriteRune(r)
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
	}
	if part := strings.TrimSpace(current.String()); part != "" {
		parts = append(parts, part)
	}
	if len(parts) <= 1 {
		return command
	}
	return strings.Join(parts, " && ")
}

func normalizeStructuredMkdirParents(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return command
	}
	parts := strings.Split(command, "&&")
	changed := false
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if !strings.HasPrefix(trimmed, "mkdir ") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || fields[0] != "mkdir" || strings.HasPrefix(fields[1], "-") {
			continue
		}
		parts[i] = " mkdir -p " + strings.TrimSpace(strings.TrimPrefix(trimmed, "mkdir "))
		changed = true
	}
	if !changed {
		return command
	}
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return strings.Join(parts, " && ")
}

func structuredCommandUsesRecursiveForceRemove(command string) bool {
	parts := strings.Fields(command)
	for i, part := range parts {
		if cleanCommandPathToken(part) != "rm" {
			continue
		}
		end := len(parts)
		for j := i + 1; j < len(parts); j++ {
			token := cleanCommandPathToken(parts[j])
			switch token {
			case "&&", "||", ";", "|":
				end = j
			}
			if end == j {
				break
			}
		}
		if rmUsesRecursiveForce(parts[i:end]) {
			return true
		}
	}
	return false
}

func structuredCommandLooksLikeMultilinePackageManagerScript(command string) bool {
	if !strings.ContainsAny(command, "\n\r") {
		return false
	}
	if strings.Contains(command, "<<") {
		return false
	}
	packageManagerLines := 0
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	atLineStart := true
	lineHasContent := false
	var lineBuilder strings.Builder
	for _, r := range command {
		if r == '\r' {
			continue
		}
		if r == '\n' {
			if lineHasContent {
				if structuredLineStartsWithPackageManager(lineBuilder.String()) {
					packageManagerLines++
					if packageManagerLines > 1 {
						return true
					}
				}
				lineBuilder.Reset()
				lineHasContent = false
			}
			atLineStart = true
			escaped = false
			continue
		}
		if atLineStart {
			if r == ' ' || r == '\t' {
				continue
			}
			if !inSingleQuote && !inDoubleQuote {
				lineBuilder.WriteRune(r)
			}
			atLineStart = false
		} else if !inSingleQuote && !inDoubleQuote {
			lineBuilder.WriteRune(r)
		}
		lineHasContent = true
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
	}
	if lineHasContent && structuredLineStartsWithPackageManager(lineBuilder.String()) {
		packageManagerLines++
	}
	if packageManagerLines > 1 {
		return true
	}
	return false
}

func structuredLineStartsWithPackageManager(line string) bool {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return false
	}
	switch cleanCommandPathToken(fields[0]) {
	case "npm", "npx", "pnpm", "yarn":
		return true
	default:
		return false
	}
}

func validateStructuredCommandForObservations(command string, observations []StructuredCommandObservation) error {
	if err := validateStructuredCommandString(command); err != nil {
		return err
	}
	return nil
}

var errRepeatedFailedStructuredCommand = errors.New("command repeats a previous failed command; check completed_actions, choose a different command, source, or local tool")
var errRepeatedSuccessfulStructuredCommand = errors.New("command already completed earlier; inspect completed_actions, update the objective ledger, or choose the next required action")

func validateStructuredCommandForRun(command string, observations []StructuredCommandObservation, workingDirectory string, objectiveLedger []StructuredObjective) error {
	return validateStructuredCommandForRunWithSurvey(command, observations, workingDirectory, objectiveLedger, WorksiteSurvey{})
}

func projectFileMapForCommandValidation(workingDir string, observations []StructuredCommandObservation) ProjectFileMap {
	return activeProjectFileMapFromResult(
		"Build the application.",
		mapDrivenArchitectToolTask(workingDir, WorksiteSurvey{}),
		workingDir,
		WorksiteSurvey{},
		observations,
	)
}

func validateStructuredCommandForRunWithSurvey(command string, observations []StructuredCommandObservation, workingDirectory string, objectiveLedger []StructuredObjective, survey WorksiteSurvey) error {
	if err := validateStructuredCommandForObservations(command, observations); err != nil {
		return err
	}
	if err := validateStructuredCommandWorkspaceProtection(command, workingDirectory); err != nil {
		return err
	}
	if err := validateStructuredCommandForTaskMode(command, "", survey.TaskMode); err != nil {
		return err
	}
	if err := validateUnsafeMutationCommandShape(command); err != nil {
		return err
	}
	if err := validateCargoScaffoldUsesActiveWorkspace(command, workingDirectory); err != nil {
		return err
	}
	if err := validateNestedGoModuleCommandScope(command, workingDirectory); err != nil {
		return err
	}
	if err := validateStructuredScaffoldScope(command, survey); err != nil {
		return err
	}
	if err := validateRepeatedPlaceholderOnlyAppMutation(command, observations, objectiveLedger); err != nil {
		return err
	}
	if err := validatePrintOnlyCommandForSubstantiveObjectives(command, objectiveLedger); err != nil {
		return err
	}
	if err := validatePlaceholderOnlySourceMutation(command, objectiveLedger, observations); err != nil {
		return err
	}
	if err := validateConflictingEntrypointMutation(command, workingDirectory); err != nil {
		return err
	}
	if err := validateCommandAgainstProjectFileMap(command, workingDirectory, projectFileMapForCommandValidation(workingDirectory, observations)); err != nil {
		return err
	}
	if err := validateStructuredDependencyScope(command, objectiveLedger, workingDirectory); err != nil {
		return err
	}
	return nil
}

func validateStructuredCommandForTaskMode(command, patch string, mode TaskMode) error {
	if normalizeTaskMode(mode) != TaskModeResearchOnly {
		return nil
	}
	if strings.TrimSpace(patch) != "" {
		return fmt.Errorf("research_only mode forbids code patches and source writes; record project issues as incidental findings unless the user explicitly asks for repair")
	}
	if researchOnlyCommandForbidden(command) {
		return fmt.Errorf("research_only mode forbids mutation, dependency changes, package installs, build repair, and verification commands that may change project state; record project issues as incidental findings unless the user explicitly asks for repair")
	}
	return nil
}

func researchOnlyCommandForbidden(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	lower := strings.ToLower(command)
	if structuredCommandLooksMutating(command) || structuredCommandLooksVerifier(command) {
		return true
	}
	for _, needle := range []string{
		"npm pkg set", "npm install", "pnpm add", "yarn add", "bun add",
		"cat >", "tee ", "apply_patch", "architect.apply", "empty_file.apply",
		"writefile", "writefilesync", "appendfile", "appendfilesync", ".write(",
		"pip install", "go get", "cargo add",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	if scriptOpenLooksWritable(lower) {
		return true
	}
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) == 0 {
			continue
		}
		switch cleanCommandPathToken(segment[0]) {
		case "mv", "cp", "rm":
			return true
		case "mkdir":
			return true
		}
	}
	return false
}

func scriptOpenLooksWritable(lower string) bool {
	if !strings.Contains(lower, "open(") {
		return false
	}
	for _, marker := range []string{",\"w\"", ", \"w\"", ",'w'", ", 'w'", ",\"a\"", ", \"a\"", ",'a'", ", 'a'", ",\"wb\"", ", \"wb\"", ",'wb'", ", 'wb'"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func validateUnsafeMutationCommandShape(command string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	if semicolonMutationChain(command) {
		return fmt.Errorf("unsafe mutation command shape: semicolon-chained mutation commands are not allowed; split mutation, install, build, and verification into separate evidence-producing steps")
	}
	if wildcardMutationLoop(command) {
		return fmt.Errorf("unsafe mutation command shape: wildcard mutation loops are not allowed; enumerate concrete files and validate each mutation")
	}
	return nil
}

func semicolonMutationChain(command string) bool {
	if !hasTopLevelShellSemicolon(command) {
		return false
	}
	return structuredCommandLooksMutating(command) || structuredCommandLooksVerifier(command)
}

func wildcardMutationLoop(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if strings.Contains(lower, "for ") && strings.Contains(lower, " in ") && strings.Contains(lower, "*") {
		for _, needle := range []string{" mv ", " cp ", " rm ", " mkdir ", " npm install", " npm pkg set", " cat >", " tee "} {
			if strings.Contains(" "+lower+" ", needle) {
				return true
			}
		}
	}
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) == 0 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		switch root {
		case "mv", "cp", "rm":
			for _, arg := range segment[1:] {
				clean := cleanCommandPathToken(arg)
				if strings.Contains(clean, "*") || strings.Contains(clean, "?") || strings.Contains(clean, "[") {
					return true
				}
			}
		}
	}
	return false
}

func structuredCommandLooksVerifier(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	return strings.Contains(lower, "npm run build") ||
		strings.Contains(lower, "npm test") ||
		strings.Contains(lower, "pnpm test") ||
		strings.Contains(lower, "yarn test") ||
		strings.Contains(lower, "go test") ||
		strings.Contains(lower, "cargo test") ||
		strings.Contains(lower, "cargo build") ||
		strings.Contains(lower, "zig build") ||
		strings.Contains(lower, "pytest")
}

func structuredCommandHasPartialFailure(command, stdoutText, stderrText string) bool {
	if !structuredCommandLooksMutating(command) && !strings.Contains(command, ";") && !strings.Contains(command, "&&") {
		return false
	}
	output := strings.ToLower(stdoutText + "\n" + stderrText)
	for _, needle := range []string{
		"cannot stat",
		"no such file or directory",
		"command not found",
		"permission denied",
		"failed to",
	} {
		if strings.Contains(output, needle) {
			return true
		}
	}
	return false
}

func validatePrintOnlyCommandForSubstantiveObjectives(command string, objectiveLedger []StructuredObjective) error {
	if !objectiveLedgerNeedsSubstantiveAppFiles(objectiveLedger) || !isPurePrintCommand(command) {
		return nil
	}
	return fmt.Errorf("print-only command does not satisfy substantive app objectives; write, patch, build, test, or inspect concrete project evidence instead")
}

func validateRepeatedPlaceholderOnlyAppMutation(command string, observations []StructuredCommandObservation, objectiveLedger []StructuredObjective) error {
	if !shellProposalIsPlaceholderOnlyMutation(command) || !objectiveLedgerNeedsSubstantiveAppFiles(objectiveLedger) {
		return nil
	}
	if latestPlaceholderOnlySuccess(observations) == nil {
		return nil
	}
	return fmt.Errorf("placeholder-only scaffold already exists; expand it with substantive source/build/test file content instead of another mkdir/touch")
}

func objectiveLedgerNeedsSubstantiveAppFiles(objectiveLedger []StructuredObjective) bool {
	for _, objective := range pendingStructuredObjectives(objectiveLedger) {
		if objectiveTextNeedsSubstantiveAppFiles(objective.ID + " " + objective.Description) {
			return true
		}
	}
	return false
}

func objectiveTextNeedsSubstantiveAppFiles(text string) bool {
	for _, token := range strings.Fields(normalizedDependencyText(text)) {
		switch token {
		case "app", "apps", "application", "applications", "component", "components", "crud", "entry", "entrypoint", "entrypoints", "source", "sources", "ui":
			return true
		default:
			if strings.HasPrefix(token, "implement") {
				return true
			}
		}
	}
	return false
}

func validateNestedGoModuleCommandScope(command, workingDirectory string) error {
	if strings.TrimSpace(command) == "" || strings.TrimSpace(workingDirectory) == "" {
		return nil
	}
	if !rootCommandRunsGoModInit(command) {
		return nil
	}
	nested := firstNestedGoMod(workingDirectory)
	if nested == "" {
		return nil
	}
	return fmt.Errorf("go mod init at workspace root is unsafe because nested module %s already exists; run Go commands from the existing module directory instead", nested)
}

func rootCommandRunsGoModInit(command string) bool {
	segments := structuredCommandSegments(command)
	for _, segment := range segments {
		if len(segment) < 3 {
			continue
		}
		if cleanCommandPathToken(segment[0]) == "go" && segment[1] == "mod" && segment[2] == "init" {
			return true
		}
	}
	return false
}

func firstNestedGoMod(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	rootGoMod := filepath.Join(root, "go.mod")
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "build" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" || path == rootGoMod {
			return nil
		}
		if rel, relErr := filepath.Rel(root, path); relErr == nil {
			found = rel
		} else {
			found = path
		}
		return nil
	})
	return found
}

func validateCargoScaffoldUsesActiveWorkspace(command, workingDirectory string) error {
	workingDirectory = strings.TrimSpace(workingDirectory)
	if command == "" || workingDirectory == "" {
		return nil
	}
	base := strings.ToLower(filepath.Base(workingDirectory))
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 || cleanCommandPathToken(segment[0]) != "cargo" || segment[1] != "new" {
			continue
		}
		for _, raw := range segment[2:] {
			arg := strings.Trim(raw, `"'`)
			if arg == "" || strings.HasPrefix(arg, "-") {
				continue
			}
			clean := strings.ToLower(filepath.Base(filepath.Clean(arg)))
			if clean == "." || clean == base {
				return fmt.Errorf("scope_drift: cargo new would create a nested project inside the active workspace; use cargo init or write Cargo.toml/src files in place")
			}
			break
		}
	}
	return nil
}
