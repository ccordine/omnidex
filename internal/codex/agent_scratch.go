package codex

import (
	"fmt"
	"os"
	"path/filepath"
)

// Disposable tool output has its own grant; verification never writes source.
func (client *Client) agentScratch() (string, func(), error) {
	root, err := filepath.Abs(client.directory)
	if err != nil {
		return "", nil, err
	}
	base := filepath.Join(root, "bin", "agent-runtime")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("agent runtime cache requires a real directory")
	}
	directory, err := os.MkdirTemp(base, "call-")
	if err != nil {
		return "", nil, err
	}
	return directory, func() { _ = os.RemoveAll(directory) }, nil
}

func agentToolEnvironment(scratch string) map[string]string {
	return map[string]string{"PATH": os.Getenv("PATH"), "HOME": scratch, "TMPDIR": scratch,
		"XDG_CACHE_HOME": scratch, "GOCACHE": filepath.Join(scratch, "go-build"), "GOPATH": filepath.Join(scratch, "go"),
		"GOMODCACHE": agentGoModuleCache(), "GOPROXY": "off", "GOTOOLCHAIN": "local", "GOENV": "off",
		"PYTHONDONTWRITEBYTECODE": "1", "GOMAXPROCS": "2", "CARGO_HOME": filepath.Join(scratch, "cargo"),
		"npm_config_cache": filepath.Join(scratch, "npm")}
}
