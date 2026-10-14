package codex

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Authentication belongs to the host Codex process. Runtime service secrets
// are not inherited by the agent or its commands.
func agentProcessEnvironment() []string {
	result := []string{}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "PATH", "LANG", "LC_ALL", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "CODEX_HOME"} {
		if value, present := os.LookupEnv(key); present {
			result = append(result, key+"="+value)
		}
	}
	return result
}

func agentPermissionFlags(call codingagent.Assignment, scratch ...string) []string {
	return agentProfileFlags(agentPermissionProfile(call, scratch...))
}

func agentProfileFlags(profile map[string]any) []string {
	flags := []string{"-c", "default_permissions=\"omnidex-task\"", "-c", "permissions.omnidex-task.network.enabled=false", "-c", "shell_environment_policy.inherit=\"none\""}
	files := profile["filesystem"].(map[string]any)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	entries := make([]string, 0, len(paths))
	for _, path := range paths {
		access := files[path]
		key, _ := json.Marshal(path)
		value, _ := json.Marshal(access)
		entries = append(entries, string(key)+"="+string(value))
	}
	flags = append(flags, "-c", "permissions.omnidex-task.filesystem={"+strings.Join(entries, ",")+"}")
	return flags
}

func (client *Client) executionPermissionProfile(call codingagent.Assignment, scratch string) (map[string]any, error) {
	executable, err := exec.LookPath(client.command)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	profile := agentPermissionProfile(call, scratch)
	files := profile["filesystem"].(map[string]any)
	files[resolved] = "read"
	return profile, nil
}

func agentPermissionProfile(call codingagent.Assignment, scratch ...string) map[string]any {
	files := map[string]any{":root": "deny", ":minimal": "read"}
	for _, directory := range agentRuntimeReadPaths(call.Root) {
		files[directory] = "read"
	}
	moduleCache := agentGoModuleCache()
	if resolved, err := filepath.EvalSymlinks(moduleCache); err == nil {
		moduleCache = resolved
	}
	files[filepath.Join(moduleCache, "cache", "vcs")] = "deny"
	for _, file := range call.ReadFiles {
		files[filepath.Join(call.Root, filepath.FromSlash(file))] = "read"
	}
	for _, file := range call.WriteFiles {
		files[filepath.Join(call.Root, filepath.FromSlash(file))] = "read"
	}
	for _, file := range call.DeleteFiles {
		files[filepath.Join(call.Root, filepath.FromSlash(file))] = "read"
	}
	for _, directory := range scratch {
		files[directory] = "write"
	}
	return map[string]any{"filesystem": files, "network": map[string]bool{"enabled": false}}
}

// Installed toolchains and dependencies are read-only runtime inputs, distinct
// from source authority. Authentication directories are never granted.
func agentRuntimeReadPaths(root string) []string {
	paths := []string{runtime.GOROOT(), agentGoModuleCache(), filepath.Join(root, "node_modules"), filepath.Join(root, "vendor")}
	for _, name := range []string{"go", "python3", "sh"} {
		if executable, err := exec.LookPath(name); err == nil {
			paths = append(paths, executable)
		}
	}
	result := []string{}
	for _, path := range paths {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && filepath.IsAbs(resolved) && resolved != string(filepath.Separator) {
			result = append(result, resolved)
		}
	}
	return result
}

func agentGoModuleCache() string {
	if cache := os.Getenv("GOMODCACHE"); filepath.IsAbs(cache) {
		return cache
	}
	if paths := filepath.SplitList(os.Getenv("GOPATH")); len(paths) > 0 && filepath.IsAbs(paths[0]) {
		return filepath.Join(paths[0], "pkg", "mod")
	}
	return filepath.Join(os.Getenv("HOME"), "go", "pkg", "mod")
}
