package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Pure parser leaves keep normal sign-in/configuration, but global agent policy
// is not their data or authority. The private mount never edits the host profile.
func startParserConnection(ctx context.Context, command, directory, scratch string) (*connection, error) {
	profile := os.Getenv("CODEX_HOME")
	if profile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		profile = filepath.Join(home, ".codex")
	}
	files := []string{}
	for _, name := range []string{"AGENTS.md", "AGENTS.override.md"} {
		path := filepath.Join(profile, name)
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	arguments := configuredConnectionArguments(nil)
	if len(files) == 0 {
		return launchConnection(ctx, command, directory, true, arguments)
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("global parser instruction isolation requires private Linux mounts on this host")
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		return nil, fmt.Errorf("parser instruction isolation requires unshare: %w", err)
	}
	if _, err := exec.LookPath("mount"); err != nil {
		return nil, fmt.Errorf("parser instruction isolation requires mount: %w", err)
	}
	empty := filepath.Join(scratch, "empty-parser-instructions")
	if err := os.WriteFile(empty, nil, 0400); err != nil {
		return nil, err
	}
	resolved, err := exec.LookPath(command)
	if err != nil {
		return nil, err
	}
	// All paths and the executable are positional arguments, never shell text.
	script := `empty=$1; count=$2; shift 2; while [ "$count" -gt 0 ]; do mount --bind "$empty" "$1" && mount -o remount,bind,ro "$1" || exit 1; shift; count=$((count-1)); done; exec "$@"`
	wrapped := []string{"--user", "--map-root-user", "--mount", "--propagation", "private", "/bin/sh", "-c", script, "parser-isolation", empty, fmt.Sprint(len(files))}
	wrapped = append(wrapped, files...)
	wrapped = append(wrapped, resolved)
	wrapped = append(wrapped, arguments...)
	return launchConnection(ctx, unshare, directory, true, wrapped)
}
