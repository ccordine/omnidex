package omni

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateShellProposalAgainstToolTask(command, toolTask string) error {
	return validateShellProposalAgainstToolTaskWithRationale(command, toolTask, "")
}

func validateShellProposalAgainstToolTaskWithRationale(command, toolTask, rationale string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	if touchTargetsProjectSourceArtifact(command) {
		return fmt.Errorf("placeholder-only touch creates empty source files; write substantive content with a here-doc, tee, patch.apply, or code specialist instead of touch")
	}
	if err := validateShellDependencyInstallRationale(command, toolTask, rationale); err != nil {
		return err
	}
	if err := validateShellProposalDoesNotRepeatInvalidMissingFileRead(command, toolTask); err != nil {
		return err
	}
	if !toolTaskRequiresMutation(toolTask) {
		return nil
	}
	if toolTaskAllowsInspectionEvidence(toolTask) && structuredCommandLooksReadOnlyEvidence(command) {
		return nil
	}
	if toolTaskAllowsPackageMetadataOperations(toolTask) && structuredCommandLooksPackageMetadataOperation(command) {
		return nil
	}
	if toolTaskRequiresSourceImplementation(toolTask) && structuredCommandLooksDependencyInstall(command) && !toolTaskAllowsDependencyInstall(toolTask) {
		return fmt.Errorf("tool_task requires source file implementation; dependency install command %q does not satisfy it", strings.TrimSpace(command))
	}
	if err := validateShellProposalMatchesToolTaskTarget(command, toolTask); err != nil {
		return err
	}
	if shellProposalIsPlaceholderOnlyMutation(command) {
		if toolTaskRequiresSubstantiveProofContent(toolTask) {
			return fmt.Errorf("tool_task requires substantive source/build/test content; placeholder-only command %q does not satisfy focused TDD/proof work", strings.TrimSpace(command))
		}
		if toolTaskAllowsScaffoldSetupStep(toolTask) {
			return nil
		}
		return fmt.Errorf("tool_task requires substantive file content or verification; placeholder-only command %q does not satisfy it", strings.TrimSpace(command))
	}
	if shellProposalWritesOnlyResearchArtifact(command, toolTask) {
		return fmt.Errorf("tool_task requires substantive source/build/test files; documentation download command %q does not satisfy it", strings.TrimSpace(command))
	}
	if structuredCommandLooksMutating(command) {
		return nil
	}
	return fmt.Errorf("tool_task requires file creation, modification, build, or test work; read-only command %q does not satisfy it", strings.TrimSpace(command))
}

func validateShellProposalDoesNotRepeatLatestFailedCommand(command string, observations []StructuredCommandObservation) error {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return nil
	}
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) != normalized {
			return nil
		}
		if obs.ExitCode == 0 {
			return nil
		}
		return fmt.Errorf("shell command repeats the latest failed execution %q; previous stderr: %s", strings.TrimSpace(command), strings.TrimSpace(obs.Stderr))
	}
	return nil
}

func validateShellProposalDoesNotRepeatInvalidMissingFileRead(command, toolTask string) error {
	invalid := invalidMissingFileReadCommandFromToolTask(toolTask)
	if invalid == "" {
		return nil
	}
	if normalizeStructuredCommandForComparison(command) != normalizeStructuredCommandForComparison(invalid) {
		return nil
	}
	return fmt.Errorf("missing-file recovery must not retry invalid read command %q; inspect the parent directory or run bounded file discovery instead", strings.TrimSpace(command))
}

func invalidMissingFileReadCommandFromToolTask(toolTask string) string {
	task := strings.TrimSpace(toolTask)
	if !strings.Contains(strings.ToLower(task), "target path does not exist") {
		return ""
	}
	marker := "Invalid command:"
	start := strings.Index(task, marker)
	if start < 0 {
		return ""
	}
	rest := strings.TrimSpace(task[start+len(marker):])
	for _, endMarker := range []string{". Failure:", ". Required next behavior:", "\n"} {
		if idx := strings.Index(rest, endMarker); idx >= 0 {
			rest = strings.TrimSpace(rest[:idx])
			break
		}
	}
	return strings.Trim(strings.TrimSpace(rest), `"'`)
}

func toolTaskRequiresSubstantiveProofContent(toolTask string) bool {
	task := strings.ToLower(strings.TrimSpace(toolTask))
	for _, needle := range []string{
		"focused test",
		"failing test",
		"smoke test",
		"tdd",
		"proof",
		"verification probe",
		"source-verification",
		"source/build/test",
		"substantive source",
		"substantive file",
		"placeholder-only",
		"scaffold already exists",
		"empty project file",
	} {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func validateShellProposalMatchesToolTaskTarget(command, toolTask string) error {
	if err := validateShellProposalMatchesArchitectTarget(command, toolTask); err != nil {
		return err
	}
	emptyFiles := emptyProjectFilesFromToolTask(toolTask)
	if len(emptyFiles) == 0 {
		return validateShellProposalLanguageScope(command, toolTask)
	}
	if err := validateShellProposalLanguageScope(command, toolTask); err != nil {
		return err
	}
	if structuredCommandLooksReadOnlyEvidence(command) || structuredCommandLooksDependencyInstall(command) {
		return nil
	}
	if commandTargetsAnyEmptyProjectFile(command, emptyFiles) {
		return nil
	}
	root := commonProjectRootForListedFiles(emptyFiles)
	if root != "" {
		return fmt.Errorf("empty-file recovery targets nested project %q; command %q must cd into that project or write one of the listed files with its full path", root, strings.TrimSpace(command))
	}
	return fmt.Errorf("empty-file recovery must fill or remove one of the listed empty file(s): %s", strings.Join(emptyFiles, ","))
}

func validateShellProposalMatchesArchitectTarget(command, toolTask string) error {
	target := implementationArchitectTargetRootFromToolTask(toolTask)
	if target == "" {
		return nil
	}
	if structuredCommandLooksReadOnlyEvidence(command) || structuredCommandLooksDependencyInstall(command) {
		return nil
	}
	cmd := filepath.ToSlash(strings.ToLower(command))
	target = filepath.ToSlash(strings.ToLower(strings.Trim(target, "/")))
	if commandChangesIntoProjectRoot(cmd, target) || strings.Contains(cmd, target+"/") {
		return nil
	}
	return fmt.Errorf("tool_task target root is %q; command %q must cd into that root or use paths under it", target, strings.TrimSpace(command))
}

func implementationArchitectTargetRootFromToolTask(toolTask string) string {
	const marker = "Implementation architect target root:"
	idx := strings.Index(toolTask, marker)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(toolTask[idx+len(marker):])
	if end := strings.Index(rest, "."); end >= 0 {
		rest = rest[:end]
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], `"'`)
}

func validateShellProposalLanguageScope(command, toolTask string) error {
	task := strings.ToLower(toolTask)
	cmd := strings.ToLower(command)
	if (strings.Contains(task, "react") || strings.Contains(task, "javascript") || strings.Contains(task, "frontend")) &&
		(strings.Contains(cmd, ".py") || strings.Contains(cmd, "python -") || strings.Contains(cmd, "python3 -")) {
		return fmt.Errorf("tool_task is for a React/JavaScript frontend; command %q drifts into Python instead of project source/build/test files", strings.TrimSpace(command))
	}
	return nil
}

func emptyProjectFilesFromToolTask(toolTask string) []string {
	const marker = "Empty file(s):"
	idx := strings.Index(toolTask, marker)
	if idx < 0 {
		return nil
	}
	rest := toolTask[idx+len(marker):]
	for _, boundary := range []string{". Active task:", ". Pending objective", ". Required next", ". Do not", ". After fixing", "\n"} {
		if end := strings.Index(rest, boundary); end >= 0 {
			rest = rest[:end]
			break
		}
	}
	files := []string{}
	for _, part := range strings.Split(rest, ",") {
		file := strings.TrimSpace(part)
		file = strings.Trim(file, `"'`)
		if file != "" {
			files = append(files, filepath.ToSlash(file))
		}
	}
	return files
}

func commandTargetsAnyEmptyProjectFile(command string, files []string) bool {
	cmd := filepath.ToSlash(strings.ToLower(command))
	for _, file := range files {
		file = filepath.ToSlash(strings.TrimSpace(file))
		if file == "" {
			continue
		}
		lowerFile := strings.ToLower(file)
		if strings.Contains(cmd, lowerFile) {
			return true
		}
		root := commonProjectRootForListedFiles([]string{file})
		if root == "" {
			continue
		}
		rel := strings.TrimPrefix(file, root+"/")
		if rel != file && commandChangesIntoProjectRoot(cmd, root) && strings.Contains(cmd, strings.ToLower(rel)) {
			return true
		}
	}
	return false
}

func commonProjectRootForListedFiles(files []string) string {
	root := ""
	for _, file := range files {
		file = strings.Trim(filepath.ToSlash(strings.TrimSpace(file)), "/")
		parts := strings.Split(file, "/")
		if len(parts) < 2 {
			return ""
		}
		candidate := parts[0]
		if candidate == "src" || candidate == "tests" || candidate == "test" {
			return ""
		}
		if root == "" {
			root = candidate
			continue
		}
		if root != candidate {
			return ""
		}
	}
	return root
}

func commandChangesIntoProjectRoot(command, root string) bool {
	root = strings.ToLower(strings.Trim(filepath.ToSlash(root), "/"))
	if root == "" {
		return false
	}
	for _, needle := range []string{
		"cd " + root,
		"cd ./" + root,
		"cd '" + root + "'",
		"cd \"" + root + "\"",
		"--prefix " + root,
		"--prefix=./" + root,
		"--prefix=" + root,
	} {
		if strings.Contains(command, needle) {
			return true
		}
	}
	return false
}

func toolTaskAllowsScaffoldSetupStep(toolTask string) bool {
	task := strings.ToLower(toolTask)
	if strings.Contains(task, "read-only inventory commands are forbidden") {
		return false
	}
	if strings.Contains(task, "do not create placeholder") ||
		strings.Contains(task, "placeholder-only") ||
		strings.Contains(task, "substantive source") ||
		strings.Contains(task, "substantive file") {
		return false
	}
	for _, needle := range []string{
		"setup", "set up", "structure", "scaffold", "create_note_app_structure", "create project structure",
		"create or modify", "file creation", "directory", "directories", "component structure",
	} {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func validateShellDependencyInstallRationale(command, toolTask, rationale string) error {
	requests := structuredDependencyInstallRequests(command)
	if len(requests) == 0 {
		return nil
	}
	if toolTaskAllowsPackageMetadataOperations(toolTask) && packageMetadataDependencyRequestsAllowed(requests) {
		return nil
	}
	if strings.TrimSpace(rationale) == "" {
		return nil
	}
	namedPackages := []string{}
	for _, request := range requests {
		namedPackages = append(namedPackages, request.Packages...)
	}
	namedPackages = cleanStringList(namedPackages)
	if len(namedPackages) == 0 {
		return nil
	}
	taskText := normalizedDependencyText(toolTask)
	rationaleText := normalizedDependencyText(rationale)
	for _, pkg := range namedPackages {
		normalized := normalizeDependencyPackageName(pkg)
		if normalized == "" {
			continue
		}
		if dependencyPackageMentioned(normalized, taskText) {
			continue
		}
		if dependencyRationaleIsEvidenceAnchored(normalized, rationaleText) {
			continue
		}
		return fmt.Errorf("dependency scope drift: dependency install command %q adds package %q without tool_task or evidence-backed rationale; do not add packages because they are merely common requirements", strings.TrimSpace(command), pkg)
	}
	return nil
}

func packageMetadataDependencyRequestsAllowed(requests []structuredDependencyInstallRequest) bool {
	allowed := map[string]bool{}
	for _, dep := range reactVitePackageMetadataDependencies() {
		allowed[normalizeDependencyPackageName(dep)] = true
	}
	for _, request := range requests {
		for _, pkg := range request.Packages {
			normalized := normalizeDependencyPackageName(pkg)
			if normalized == "" {
				continue
			}
			if !allowed[normalized] {
				return false
			}
		}
	}
	return true
}

func dependencyRationaleIsEvidenceAnchored(pkg, rationaleText string) bool {
	if pkg == "" || !dependencyPackageMentioned(pkg, rationaleText) {
		return false
	}
	evidenceNeedles := []string{
		"package json",
		"package lock",
		"lockfile",
		"cannot find module",
		"module not found",
		"missing module",
		"build error",
		"test error",
		"compiler error",
		"user asked",
		"user requested",
		"recipe required",
		"required prerequisite",
		"observed",
		"evidence",
	}
	for _, needle := range evidenceNeedles {
		if strings.Contains(rationaleText, needle) {
			return true
		}
	}
	return false
}

func dependencyPackageMentioned(pkg, normalizedText string) bool {
	pkg = normalizeDependencyPackageName(pkg)
	if pkg == "" || normalizedText == "" {
		return false
	}
	if strings.Contains(normalizedText, " "+pkg+" ") {
		return true
	}
	tokenized := normalizedDependencyText(pkg)
	return strings.TrimSpace(tokenized) != "" && strings.Contains(normalizedText, tokenized)
}

func toolTaskRequiresSourceImplementation(toolTask string) bool {
	task := strings.ToLower(toolTask)
	needles := []string{
		"actual project files",
		"component",
		"crud",
		"implement",
		"in-memory",
		"source/build/test",
		"source file",
		"store_notes",
		"substantive source",
		"ui",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func toolTaskAllowsDependencyInstall(toolTask string) bool {
	task := strings.ToLower(toolTask)
	needles := []string{
		"install dependencies",
		"install package",
		"install required",
		"dependency install",
		"package install",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func toolTaskAllowsPackageMetadataOperations(toolTask string) bool {
	task := strings.ToLower(toolTask)
	for _, needle := range []string{
		"package_metadata_update",
		"package metadata",
		"package.json",
		"setup_react_package_metadata",
		"configure_package_scripts",
		"configure_vite_react_package",
		"install_dependencies",
	} {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func structuredCommandLooksPackageMetadataOperation(command string) bool {
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		if root != "npm" {
			continue
		}
		action := segment[1]
		if action == "install" || action == "add" || action == "i" || action == "set-script" {
			return true
		}
		if action == "pkg" && len(segment) >= 3 {
			switch segment[2] {
			case "set", "delete", "del", "get":
				return true
			}
		}
	}
	return false
}

func structuredCommandLooksDependencyInstall(command string) bool {
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		action := segment[1]
		switch root {
		case "npm":
			if action == "install" || action == "add" || action == "i" {
				return true
			}
		case "pnpm", "yarn", "bun":
			if action == "add" || action == "install" {
				return true
			}
		case "cargo":
			if action == "add" {
				return true
			}
		case "go":
			if action == "get" {
				return true
			}
		case "pip", "pip3":
			if action == "install" {
				return true
			}
		case "composer":
			if action == "require" || action == "install" {
				return true
			}
		}
	}
	return false
}

func shellProposalWritesOnlyResearchArtifact(command, toolTask string) bool {
	task := strings.ToLower(toolTask)
	if !strings.Contains(task, "substantive source") && !strings.Contains(task, "actual project files") {
		return false
	}
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "curl") {
		return false
	}
	if !strings.Contains(lower, ">") && !strings.Contains(lower, " -o ") && !strings.Contains(lower, " --output ") {
		return false
	}
	return !structuredCommandLooksAppFileMutation(command)
}

func shellProposalIsPlaceholderOnlyMutation(command string) bool {
	segments := structuredCommandSegments(command)
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		if root == "touch" || root == "mkdir" {
			continue
		}
		return false
	}
	return true
}

func toolTaskAllowsInspectionEvidence(toolTask string) bool {
	task := strings.ToLower(toolTask)
	needles := []string{
		"inspect_empty_placeholder",
		"inspect for empty placeholder",
		"inspect existing files",
		"target path does not exist",
		"run a bounded file discovery command",
		"inspect the parent directory",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func structuredCommandLooksReadOnlyEvidence(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	root := cleanCommandPathToken(fields[0])
	if root == "cd" {
		for i, field := range fields {
			if field == "&&" && i+1 < len(fields) {
				root = cleanCommandPathToken(fields[i+1])
				break
			}
			if field == ";" && i+1 < len(fields) {
				root = cleanCommandPathToken(fields[i+1])
				break
			}
		}
	}
	switch root {
	case "find", "ls", "cat", "grep", "rg", "sed", "head", "tail", "test", "stat", "pwd", "jq", "npm":
	default:
		return false
	}
	return !structuredCommandLooksMutating(command)
}

func toolTaskRequiresMutation(toolTask string) bool {
	task := strings.ToLower(toolTask)
	if strings.Contains(task, "do not continue with read-only") || strings.Contains(task, "read-only inventory commands") {
		return true
	}
	needles := []string{
		"required next behavior: create or modify the actual project files now",
		"patch existing project files",
		"substantive source/build/test files",
		"substantive source, build metadata, tests, and verification files",
		"focused test",
		"failing test",
		"smoke test",
		"tdd",
		"proof",
		"verification probe",
		"implementation architect target root:",
		"completion is blocked because empty project files remain",
		"empty file(s):",
		"writes index.html, src/index.js, styles, package scripts, and verification files",
		"npm run build",
		"npm test",
	}
	for _, needle := range needles {
		if strings.Contains(task, needle) {
			return true
		}
	}
	return false
}

func validateStructuredScaffoldScope(command string, survey WorksiteSurvey) error {
	if strings.TrimSpace(command) == "" || survey.UserOperation == "" || survey.UserOperation == userOperationUnknown {
		return nil
	}
	if !structuredCommandHasScaffold(command) {
		return nil
	}
	if worksiteSurveyAllowsCreateNew(survey) {
		return nil
	}
	return fmt.Errorf("scope_drift: scaffold command forbidden for %s in %s; full access does not permit changing task scope", survey.UserOperation, survey.ProjectState)
}

func structuredCommandHasScaffold(command string) bool {
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) < 2 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		if root == "npx" && len(segment) >= 2 {
			tool := cleanCommandPathToken(segment[1])
			if tool == "create-react-app" || tool == "degit" {
				return true
			}
		}
		if root == "npm" && len(segment) >= 3 {
			action := segment[1]
			tool := cleanCommandPathToken(segment[2])
			if action == "create" && strings.HasPrefix(tool, "vite") {
				return true
			}
			if action == "init" && (strings.HasPrefix(tool, "vite") || tool == "react-app") {
				return true
			}
		}
		if (root == "pnpm" || root == "yarn" || root == "bun") && len(segment) >= 3 && segment[1] == "create" {
			if strings.HasPrefix(cleanCommandPathToken(segment[2]), "vite") {
				return true
			}
		}
		if root == "cargo" && len(segment) >= 2 && (segment[1] == "new" || segment[1] == "init") {
			return true
		}
		if root == "git" && len(segment) >= 2 && segment[1] == "clone" {
			return true
		}
	}
	return false
}
