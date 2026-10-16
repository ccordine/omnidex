package codex

import (
	"os"
	"path/filepath"
)

const agentPermissionProfileName = ":danger-full-access"

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

func agentPermissionFlags() []string {
	return []string{
		"-c", "default_permissions=\"" + agentPermissionProfileName + "\"",
		"-c", "shell_environment_policy.inherit=\"all\"",
	}
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
