package omni

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	userOperationCreateNewProject = "create_new_project"
	userOperationModifyExisting   = "modify_existing_project"
	userOperationFixExisting      = "fix_existing_project"
	userOperationInspectExisting  = "inspect_existing_project"
	userOperationRunTests         = "run_tests"
	userOperationInstallDeps      = "install_deps"
	userOperationUnknown          = "unknown"
	projectStateEmptyDirectory    = "empty_directory"
	projectStateExistingProject   = "existing_project"
	projectStateExistingReactApp  = "existing_react_app"
	projectStateExistingNodeApp   = "existing_node_app"
	projectStateExistingGoProject = "existing_go_project"
	projectStateMixedWorkspace    = "mixed_workspace"
	projectStateUnknown           = "unknown"
	packageManagerNPM             = "npm"
	packageManagerPNPM            = "pnpm"
	packageManagerYarn            = "yarn"
	packageManagerBun             = "bun"
	packageManagerNone            = "none"
	packageManagerUnknown         = "unknown"
)

type WorksiteSurvey struct {
	WorkspacePath         string   `json:"workspace_path"`
	UserOperation         string   `json:"user_operation"`
	TaskMode              TaskMode `json:"task_mode,omitempty"`
	ProjectState          string   `json:"project_state"`
	PackageManager        string   `json:"package_manager,omitempty"`
	Manifests             []string `json:"manifests,omitempty"`
	Frameworks            []string `json:"frameworks,omitempty"`
	Evidence              []string `json:"evidence,omitempty"`
	AllowedOperation      bool     `json:"allowed_operation"`
	BlockReason           string   `json:"block_reason,omitempty"`
	RecommendedRecipeIDs  []string `json:"recommended_recipe_ids,omitempty"`
	ForbiddenRecipeIDs    []string `json:"forbidden_recipe_ids,omitempty"`
	AllowedScaffoldRoot   string   `json:"allowed_scaffold_root,omitempty"`
	ExplicitNewTargetPath string   `json:"explicit_new_target_path,omitempty"`
}

func BuildWorksiteSurvey(workspace string) WorksiteSurvey {
	workspace = structuredPromptWorkingDirectory(workspace)
	survey := WorksiteSurvey{
		WorkspacePath:    workspace,
		UserOperation:    userOperationUnknown,
		ProjectState:     projectStateUnknown,
		PackageManager:   packageManagerUnknown,
		AllowedOperation: true,
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		survey.Evidence = append(survey.Evidence, "workspace unreadable: "+err.Error())
		return survey
	}
	meaningful := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".env" {
			continue
		}
		if name == "node_modules" {
			continue
		}
		meaningful++
	}
	if meaningful == 0 {
		survey.ProjectState = projectStateEmptyDirectory
		survey.PackageManager = packageManagerNone
		survey.Evidence = append(survey.Evidence, "workspace has no meaningful project files")
	}
	hasPackage := surveyFileExists(filepath.Join(workspace, "package.json"))
	hasGoMod := surveyFileExists(filepath.Join(workspace, "go.mod"))
	if hasPackage {
		survey.Manifests = append(survey.Manifests, "package.json")
		survey.Evidence = append(survey.Evidence, "package.json exists")
		survey.ProjectState = projectStateExistingNodeApp
		survey.PackageManager = detectNodePackageManager(workspace)
		if packageJSONHasDeps(workspace, "react", "react-dom") {
			survey.ProjectState = projectStateExistingReactApp
			survey.Frameworks = append(survey.Frameworks, "react")
			survey.Evidence = append(survey.Evidence, "package.json dependencies include react and react-dom")
		}
		if dirExists(filepath.Join(workspace, "src")) {
			survey.Evidence = append(survey.Evidence, "src/ exists")
		}
		for _, pattern := range []string{"vite.config.*", "webpack.config.*", "next.config.*"} {
			if matches, _ := filepath.Glob(filepath.Join(workspace, pattern)); len(matches) > 0 {
				survey.Evidence = append(survey.Evidence, filepath.Base(matches[0])+" exists")
			}
		}
	}
	if hasGoMod {
		survey.Manifests = append(survey.Manifests, "go.mod")
		survey.Evidence = append(survey.Evidence, "go.mod exists")
		if hasPackage {
			survey.ProjectState = projectStateMixedWorkspace
		} else {
			survey.ProjectState = projectStateExistingGoProject
			survey.PackageManager = packageManagerNone
		}
	}
	if survey.ProjectState == projectStateUnknown && meaningful > 0 {
		survey.ProjectState = projectStateExistingProject
		survey.PackageManager = packageManagerNone
		survey.Evidence = append(survey.Evidence, "workspace contains existing files")
	}
	return survey
}

func (s WorksiteSurvey) WithOperation(operation string) WorksiteSurvey {
	s.UserOperation = normalizeUserOperation(operation)
	s.AllowedOperation = true
	s.BlockReason = ""
	if s.UserOperation == userOperationModifyExisting || s.UserOperation == userOperationFixExisting || s.UserOperation == userOperationInspectExisting {
		if s.ProjectState == projectStateEmptyDirectory {
			s.AllowedOperation = false
			s.BlockReason = "requested existing-project operation but workspace appears empty"
		}
	}
	if s.UserOperation == userOperationCreateNewProject && s.ProjectState != projectStateEmptyDirectory && s.ExplicitNewTargetPath == "" {
		s.ForbiddenRecipeIDs = cleanStringList(append(s.ForbiddenRecipeIDs, "react.create_new", "npm.frontend.create_new"))
	}
	return s
}

func normalizeUserOperation(operation string) string {
	switch strings.TrimSpace(operation) {
	case userOperationCreateNewProject, userOperationModifyExisting, userOperationFixExisting, userOperationInspectExisting, userOperationRunTests, userOperationInstallDeps:
		return strings.TrimSpace(operation)
	default:
		return userOperationUnknown
	}
}

func detectNodePackageManager(workspace string) string {
	switch {
	case surveyFileExists(filepath.Join(workspace, "pnpm-lock.yaml")):
		return packageManagerPNPM
	case surveyFileExists(filepath.Join(workspace, "yarn.lock")):
		return packageManagerYarn
	case surveyFileExists(filepath.Join(workspace, "bun.lockb")):
		return packageManagerBun
	case surveyFileExists(filepath.Join(workspace, "package-lock.json")):
		return packageManagerNPM
	case surveyFileExists(filepath.Join(workspace, "package.json")):
		return packageManagerNPM
	default:
		return packageManagerUnknown
	}
}

func packageJSONHasDeps(workspace string, deps ...string) bool {
	blob, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies    map[string]interface{} `json:"dependencies"`
		DevDependencies map[string]interface{} `json:"devDependencies"`
	}
	if err := json.Unmarshal(blob, &pkg); err != nil {
		return false
	}
	for _, dep := range deps {
		if _, ok := pkg.Dependencies[dep]; ok {
			continue
		}
		if _, ok := pkg.DevDependencies[dep]; ok {
			continue
		}
		return false
	}
	return true
}

func surveyFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func worksiteSurveyAllowsCreateNew(survey WorksiteSurvey) bool {
	return survey.UserOperation == userOperationCreateNewProject
}
