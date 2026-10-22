package codex

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// The official npm launcher delegates to this same native executable. Selecting
// it directly gives the sandbox and cancellation one actual process authority.
// Other configured commands retain their original behavior.
func resolveCodexExecutable(command string) string {
	path, err := exec.LookPath(command)
	if err != nil {
		return command
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return command
	}
	if filepath.Base(resolved) != "codex.js" || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return command
	}
	packageRoot := filepath.Dir(filepath.Dir(resolved))
	data, err := os.ReadFile(filepath.Join(packageRoot, "package.json"))
	if err != nil {
		return command
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Name != "@openai/codex" {
		return command
	}
	platform := map[string]string{"linux": "linux", "darwin": "darwin", "windows": "win32"}[runtime.GOOS]
	architecture := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	triples := map[string]string{
		"linux/amd64": "x86_64-unknown-linux-musl", "linux/arm64": "aarch64-unknown-linux-musl",
		"darwin/amd64": "x86_64-apple-darwin", "darwin/arm64": "aarch64-apple-darwin",
		"windows/amd64": "x86_64-pc-windows-msvc", "windows/arm64": "aarch64-pc-windows-msvc",
	}
	triple := triples[runtime.GOOS+"/"+runtime.GOARCH]
	if platform == "" || architecture == "" || triple == "" {
		return command
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	packageName := "codex-" + platform + "-" + architecture
	candidates := []string{
		filepath.Join(packageRoot, "node_modules", "@openai", packageName, "vendor", triple, "bin", name),
		filepath.Join(filepath.Dir(packageRoot), packageName, "vendor", triple, "bin", name),
		filepath.Join(packageRoot, "vendor", triple, "bin", name),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return command
}
