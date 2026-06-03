package codexrunner

import (
	"os"
	"path/filepath"
	"strings"
)

// CommandEnv returns an environment for Codex SDK child processes with a PATH
// that includes common CLI, node/npm, and shell utility locations.
func CommandEnv() []string {
	mergedPath := augmentPath(os.Getenv("PATH"))
	out := make([]string, 0, len(os.Environ())+1)
	replaced := false
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "PATH=") {
			out = append(out, "PATH="+mergedPath)
			replaced = true
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, "PATH="+mergedPath)
	}
	return out
}

func augmentPath(existing string) string {
	seen := map[string]struct{}{}
	ordered := make([]string, 0, 20)
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			return
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return
		}
		seen[dir] = struct{}{}
		ordered = append(ordered, dir)
	}
	addBinDir := func(tool string) {
		tool = strings.TrimSpace(tool)
		if filepath.IsAbs(tool) || strings.ContainsRune(tool, os.PathSeparator) {
			add(filepath.Dir(tool))
		}
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		add(filepath.Join(home, ".local/share/mise/shims"))
		if matches, _ := filepath.Glob(filepath.Join(home, ".local/share/mise/installs/node/*/bin")); len(matches) > 0 {
			add(matches[len(matches)-1])
		}
		add(filepath.Join(home, ".local/bin"))
		add(filepath.Join(home, ".npm-global/bin"))
		add(filepath.Join(home, ".bun/bin"))
		add(filepath.Join(home, ".cargo/bin"))
	}
	addBinDir(NodeBin())
	addBinDir(NPMBin())
	addBinDir(CodexBin())
	for _, dir := range filepath.SplitList(existing) {
		add(dir)
	}
	add("/usr/local/bin")
	add("/usr/bin")
	add("/bin")
	return strings.Join(ordered, string(os.PathListSeparator))
}

func lookPathInEnv(name string, env []string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", os.ErrNotExist
	}
	if filepath.IsAbs(name) || strings.ContainsRune(name, os.PathSeparator) {
		return usableCommandPath(name)
	}
	pathValue := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			pathValue = entry[len("PATH="):]
			break
		}
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if path, err := usableCommandPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", os.ErrNotExist
}

func usableCommandPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", os.ErrNotExist
	}
	if info.Mode()&0o111 == 0 {
		return "", os.ErrPermission
	}
	return path, nil
}
