package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type codexUpdateBinding struct{ packageRoot, prefix string }

func bindCodexUpdate(command string) (codexUpdateBinding, error) {
	path, err := exec.LookPath(command)
	if err != nil {
		return codexUpdateBinding{}, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return codexUpdateBinding{}, err
	}
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		if filepath.Base(directory) == "codex" && filepath.Base(filepath.Dir(directory)) == "@openai" {
			data, err := os.ReadFile(filepath.Join(directory, "package.json"))
			var manifest struct {
				Name string `json:"name"`
			}
			if err == nil && len(data) <= 64<<10 && json.Unmarshal(data, &manifest) == nil && manifest.Name == "@openai/codex" {
				prefix, err := existingNpmPrefix(directory)
				if err != nil {
					return codexUpdateBinding{}, err
				}
				launcher := filepath.Join(prefix, "bin", "codex")
				native, err := filepath.EvalSymlinks(resolveCodexExecutable(launcher))
				if err != nil || native != path {
					return codexUpdateBinding{}, fmt.Errorf("Codex updater could not verify this installation's launcher")
				}
				return codexUpdateBinding{packageRoot: directory, prefix: prefix}, nil
			}
		}
		if directory == filepath.Dir(directory) {
			break
		}
	}
	return codexUpdateBinding{}, fmt.Errorf("This Codex installation has no verified package update binding.")
}

func existingNpmPrefix(packageRoot string) (string, error) {
	modules := filepath.Dir(filepath.Dir(packageRoot))
	library := filepath.Dir(modules)
	if filepath.Base(modules) != "node_modules" || filepath.Base(library) != "lib" {
		return "", fmt.Errorf("Codex is not installed in a supported existing npm prefix")
	}
	return filepath.Dir(library), nil
}

func (binding codexUpdateBinding) environment() []string {
	environment := agentProcessEnvironment()
	filtered := environment[:0]
	for _, variable := range environment {
		name, _, _ := strings.Cut(variable, "=")
		if name != "CODEX_MANAGED_PACKAGE_ROOT" && !strings.HasPrefix(name, "CODEX_MANAGED_BY_") && !strings.EqualFold(name, "npm_config_prefix") {
			filtered = append(filtered, variable)
		}
	}
	return append(filtered, "CODEX_MANAGED_PACKAGE_ROOT="+binding.packageRoot, "CODEX_MANAGED_BY_NPM=1", "npm_config_prefix="+binding.prefix)
}
