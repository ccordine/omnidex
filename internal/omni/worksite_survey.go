package omni

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	projectStateEmptyDirectory    = "empty_directory"
	projectStateExistingProject   = "existing_project"
	projectStateExistingReactApp  = "existing_react_app"
	projectStateExistingNodeApp   = "existing_node_app"
	projectStateExistingGoProject = "existing_go_project"
	projectStateMixedWorkspace    = "mixed_workspace"
)

type WorksiteSurvey struct {
	WorkspacePath  string   `json:"workspace_path"`
	ProjectState   string   `json:"project_state"`
	PackageManager string   `json:"package_manager"`
	Manifests      []string `json:"manifests,omitempty"`
	Frameworks     []string `json:"frameworks,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
}

func BuildWorksiteSurvey(workspace string) (WorksiteSurvey, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return WorksiteSurvey{}, fmt.Errorf("workspace is required")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return WorksiteSurvey{}, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return WorksiteSurvey{}, fmt.Errorf("read workspace %s: %w", absolute, err)
	}

	survey := WorksiteSurvey{WorkspacePath: absolute, PackageManager: "none"}
	meaningful := 0
	for _, entry := range entries {
		name := entry.Name()
		if (strings.HasPrefix(name, ".") && name != ".env") || name == "node_modules" {
			continue
		}
		meaningful++
	}
	if meaningful == 0 {
		survey.ProjectState = projectStateEmptyDirectory
		survey.Evidence = []string{"workspace has no meaningful project files"}
		return survey, nil
	}

	hasPackage, err := fileExists(filepath.Join(absolute, "package.json"))
	if err != nil {
		return WorksiteSurvey{}, err
	}
	hasGoMod, err := fileExists(filepath.Join(absolute, "go.mod"))
	if err != nil {
		return WorksiteSurvey{}, err
	}
	if hasPackage {
		survey.ProjectState = projectStateExistingNodeApp
		survey.PackageManager, err = detectNodePackageManager(absolute)
		if err != nil {
			return WorksiteSurvey{}, err
		}
		survey.Manifests = append(survey.Manifests, "package.json")
		survey.Evidence = append(survey.Evidence, "package.json exists")
		hasReact, err := packageJSONHasDependencies(absolute, "react", "react-dom")
		if err != nil {
			return WorksiteSurvey{}, err
		}
		if hasReact {
			survey.ProjectState = projectStateExistingReactApp
			survey.Frameworks = []string{"react"}
		}
	}
	if hasGoMod {
		survey.Manifests = append(survey.Manifests, "go.mod")
		survey.Evidence = append(survey.Evidence, "go.mod exists")
		if hasPackage {
			survey.ProjectState = projectStateMixedWorkspace
		} else {
			survey.ProjectState = projectStateExistingGoProject
		}
	}
	if survey.ProjectState == "" {
		survey.ProjectState = projectStateExistingProject
		survey.Evidence = append(survey.Evidence, "workspace contains existing files")
	}
	return survey, nil
}

func detectNodePackageManager(workspace string) (string, error) {
	for _, candidate := range []struct {
		file    string
		manager string
	}{
		{file: "pnpm-lock.yaml", manager: "pnpm"},
		{file: "yarn.lock", manager: "yarn"},
		{file: "bun.lockb", manager: "bun"},
	} {
		exists, err := fileExists(filepath.Join(workspace, candidate.file))
		if err != nil {
			return "", err
		}
		if exists {
			return candidate.manager, nil
		}
	}
	return "npm", nil
}

func packageJSONHasDependencies(workspace string, dependencies ...string) (bool, error) {
	path := filepath.Join(workspace, "package.json")
	blob, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	var manifest struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if err := json.Unmarshal(blob, &manifest); err != nil {
		return false, fmt.Errorf("decode %s: %w", path, err)
	}
	for _, dependency := range dependencies {
		if _, exists := manifest.Dependencies[dependency]; exists {
			continue
		}
		if _, exists := manifest.DevDependencies[dependency]; !exists {
			return false, nil
		}
	}
	return true, nil
}
