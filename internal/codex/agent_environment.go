package codex

import (
	"os"
	"path/filepath"
)

// Restricted planning and parsing processes use this minimal environment.
// Execution, review, and checks inherit the launching host environment.
func agentProcessEnvironment() []string {
	result := []string{}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "PATH", "LANG", "LC_ALL", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "CODEX_HOME"} {
		if value, present := os.LookupEnv(key); present {
			result = append(result, key+"="+value)
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
