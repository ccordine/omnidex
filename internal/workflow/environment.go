package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ScopeVariable = "OMNIDEX_WORKFLOW_SCOPE"

// Environment reuses one workspace resource directory across native roles,
// retries and generations. Source and application dependencies stay in place.
func (scope Scope) Environment(environment []string, helper string) ([]string, error) {
	directory := filepath.Join(scope.Directory, "jobs", fmt.Sprint(scope.Job), key(scope.Task))
	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0700); err != nil {
		return nil, err
	}
	scope.Docker, _ = exec.LookPath("docker")
	scope.Go, _ = exec.LookPath("go")
	// Preserve explicit host/TLS wiring; never redirect an unsupported endpoint.
	scope.NoDockerTracking = os.Getenv("DOCKER_HOST") != ""
	if scope.Docker != "" && !scope.NoDockerTracking {
		if value, err := exec.Command(scope.Docker, "context", "show").Output(); err == nil {
			scope.DockerContext = strings.TrimSpace(string(value))
		}
	}
	data, err := json.Marshal(scope)
	if err != nil {
		return nil, err
	}
	config := filepath.Join(directory, "scope.json")
	if err = os.WriteFile(config, data, 0600); err != nil {
		return nil, err
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	for name, binary := range map[string]string{"docker": scope.Docker, "go": scope.Go} {
		if binary == "" {
			continue
		}
		body := "#!/bin/sh\nexec " + quote(helper) + " workflow-exec " + quote(config) + " " + name + " \"$@\"\n"
		if err = os.WriteFile(filepath.Join(directory, "bin", name), []byte(body), 0700); err != nil {
			return nil, err
		}
	}
	set := func(name, value string) {
		prefix := name + "="
		for i, entry := range environment {
			if strings.HasPrefix(entry, prefix) {
				environment[i] = prefix + value
				return
			}
		}
		environment = append(environment, prefix+value)
	}
	environment = append([]string(nil), environment...)
	set("PATH", filepath.Join(directory, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Preserve both host temporary-directory settings. Go's testing.TempDir also
	// uses GOTMPDIR, so relocating it changes application fixture IPC paths.
	set(ScopeVariable, config)
	return environment, nil
}

func Load(file string) (Scope, error) {
	var scope Scope
	data, err := os.ReadFile(file)
	if err != nil {
		return scope, err
	}
	err = json.Unmarshal(data, &scope)
	return scope, err
}

func (scope Scope) CleanTemporary(task string) error {
	// This location is exclusively owned temporary storage. Sources, caches,
	// installed tools, receipts and retained build outputs are outside it.
	parent := filepath.Join(scope.Directory, "jobs", fmt.Sprint(scope.Job), key(task))
	actual, err := filepath.EvalSymlinks(parent)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if actual != parent {
		return fmt.Errorf("workflow temporary directory changed ownership")
	}
	return os.RemoveAll(parent)
}
