package omni

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type DeterministicProjectProbe struct {
	PackageManager string   `json:"package_manager"`
	TestCommands   []string `json:"test_commands,omitempty"`
	BuildCommands  []string `json:"build_commands,omitempty"`
	RunCommands    []string `json:"run_commands,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
}

func deterministicProjectProbe(workspace string) (DeterministicProjectProbe, error) {
	packageManager, err := detectPackageManager(workspace)
	if err != nil {
		return DeterministicProjectProbe{}, err
	}
	probe := DeterministicProjectProbe{PackageManager: packageManager}
	goModule, err := fileExists(filepath.Join(workspace, "go.mod"))
	if err != nil {
		return DeterministicProjectProbe{}, err
	}
	if goModule {
		probe.TestCommands = []string{"go test ./..."}
		probe.BuildCommands = []string{"go build ./..."}
		probe.Evidence = append(probe.Evidence, "go.mod exists")
	}
	packageManifest, err := fileExists(filepath.Join(workspace, "package.json"))
	if err != nil {
		return DeterministicProjectProbe{}, err
	}
	if packageManifest {
		probe.Evidence = append(probe.Evidence, "package.json exists")
		scripts, err := packageJSONScripts(workspace)
		if err != nil {
			return DeterministicProjectProbe{}, err
		}
		if _, exists := scripts["test"]; exists {
			probe.TestCommands = append(probe.TestCommands, "npm test")
		}
		if _, exists := scripts["build"]; exists {
			probe.BuildCommands = append(probe.BuildCommands, "npm run build")
		}
		if _, exists := scripts["dev"]; exists {
			probe.RunCommands = append(probe.RunCommands, "npm run dev")
		}
		if _, exists := scripts["start"]; exists {
			probe.RunCommands = append(probe.RunCommands, "npm start")
		}
	}
	return probe, nil
}

func detectPackageManager(workspace string) (string, error) {
	candidates := []struct {
		file    string
		manager string
	}{
		{file: "pnpm-lock.yaml", manager: "pnpm"},
		{file: "yarn.lock", manager: "yarn"},
		{file: "bun.lockb", manager: "bun"},
		{file: "package.json", manager: "npm"},
		{file: "go.mod", manager: "go"},
		{file: "Cargo.toml", manager: "cargo"},
		{file: "pyproject.toml", manager: "python"},
	}
	for _, candidate := range candidates {
		exists, err := fileExists(filepath.Join(workspace, candidate.file))
		if err != nil {
			return "", err
		}
		if exists {
			return candidate.manager, nil
		}
	}
	return "unknown", nil
}

func packageJSONScripts(workspace string) (map[string]string, error) {
	path := filepath.Join(workspace, "package.json")
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read package.json %q: %w", path, err)
	}
	var payload struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(blob, &payload); err != nil {
		return nil, fmt.Errorf("decode package.json %q: %w", path, err)
	}
	return payload.Scripts, nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect project file %q: %w", path, err)
	}
	return !info.IsDir(), nil
}

func shouldSkipSnapshotDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "target", "build", "dist", ".next", ".cache":
		return true
	default:
		return false
	}
}
