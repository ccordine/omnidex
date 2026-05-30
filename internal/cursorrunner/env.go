package cursorrunner

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultModel = "composer-2.5"

// DefaultModel returns the Cursor model id used when none is configured.
func DefaultModel() string {
	return firstNonEmpty(os.Getenv("OMNI_CURSOR_MODEL"), defaultModel)
}

// CommandEnv returns an environment for Cursor SDK child processes with a PATH
// that includes common node/npm locations and core Unix utilities (base64, etc.).
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
	ordered := make([]string, 0, 16)
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

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		add(filepath.Join(home, ".local/share/mise/shims"))
		if matches, _ := filepath.Glob(filepath.Join(home, ".local/share/mise/installs/node/*/bin")); len(matches) > 0 {
			add(matches[len(matches)-1])
		}
	}
	add(filepath.Dir(NodeBin()))
	add(filepath.Dir(NPMBin()))
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
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}
