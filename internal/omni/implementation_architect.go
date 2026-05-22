package omni

import (
	"fmt"
	"path/filepath"
	"strings"
)

type ImplementationArchitectContract struct {
	Role            string   `json:"role"`
	TargetRoot      string   `json:"target_root"`
	Framework       string   `json:"framework,omitempty"`
	PackageManager  string   `json:"package_manager,omitempty"`
	EditSurface     []string `json:"edit_surface,omitempty"`
	ProofCommands   []string `json:"proof_commands,omitempty"`
	Guardrails      []string `json:"guardrails,omitempty"`
	ValidatorScopes []string `json:"validator_scopes,omitempty"`
}

func buildImplementationArchitectContract(prompt, toolTask, workingDir string, survey WorksiteSurvey, observations []StructuredCommandObservation) ImplementationArchitectContract {
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
	} else {
		contract.EditSurface = architectPaths(targetRoot, "src/", "package.json", "Cargo.toml", "go.mod", "build.zig")
		contract.ProofCommands = architectCommands(targetRoot, packageManager, "test -n \"$(find . -maxdepth 3 -type f | head -1)\"")
	}
	return contract
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
		return nil
	}
	if structuredCommandLooksDependencyInstall(command) || structuredCommandLooksReadOnlyEvidence(command) {
		return nil
	}
	cmd := filepath.ToSlash(strings.ToLower(command))
	target = filepath.ToSlash(strings.ToLower(strings.Trim(target, "/")))
	if commandChangesIntoProjectRoot(cmd, target) || strings.Contains(cmd, target+"/") {
		return nil
	}
	return errArchitectContract("command must target architect root %q by cd-ing into it or using paths under it", contract.TargetRoot)
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
