package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func discoverNeighborVideos(currentPath string) ([]string, error) {
	currentDir := filepath.Dir(currentPath)
	parentDir := filepath.Dir(currentDir)

	roots := []string{currentDir}
	if parentDir != "" && parentDir != currentDir {
		entries, err := os.ReadDir(parentDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				roots = append(roots, filepath.Join(parentDir, entry.Name()))
			}
		}
	}

	seenFiles := map[string]struct{}{}
	out := make([]string, 0, 256)
	for _, root := range dedupePaths(roots) {
		if len(out) >= mediaScanFileLimit {
			break
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			depth := strings.Count(filepath.ToSlash(rel), "/")
			if d.IsDir() {
				if depth > mediaScanMaxDepth {
					return filepath.SkipDir
				}
				return nil
			}
			if depth > mediaScanMaxDepth {
				return nil
			}
			if !isVideoFile(path) {
				return nil
			}

			clean := filepath.Clean(path)
			if _, exists := seenFiles[clean]; exists {
				return nil
			}
			seenFiles[clean] = struct{}{}
			out = append(out, clean)
			if len(out) >= mediaScanFileLimit {
				return errors.New("scan limit reached")
			}
			return nil
		})
	}

	sort.Slice(out, func(i, j int) bool { return naturalLess(out[i], out[j]) })
	return out, nil
}

func filterCandidatesByShowHint(paths []string, showHint string) []string {
	cleanHint := normalizeForMatch(showHint)
	if cleanHint == "" {
		return paths
	}
	tokens := strings.Fields(cleanHint)
	if len(tokens) == 0 {
		return paths
	}

	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized := normalizeForMatch(path)
		match := true
		for _, token := range tokens {
			if !strings.Contains(normalized, token) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func pickNextPath(paths []string, currentPath string) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("no candidate files to choose from")
	}

	cleanCurrent := filepath.Clean(currentPath)
	index := -1
	for i, path := range paths {
		if filepath.Clean(path) == cleanCurrent {
			index = i
			break
		}
	}

	if index >= 0 {
		if index+1 < len(paths) {
			return paths[index+1], nil
		}
		return "", errors.New("already at the last discovered episode")
	}

	for _, path := range paths {
		if naturalLess(cleanCurrent, path) {
			return path, nil
		}
	}

	return "", errors.New("no later episode was found relative to the current file")
}

func openMediaWithVLC(player, targetPath string) (string, error) {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		absTarget = targetPath
	}
	uri := fileURI(absTarget)

	if _, err := exec.LookPath("playerctl"); err == nil && strings.TrimSpace(player) != "" {
		if _, err := playerctlValue(player, "open", uri); err == nil {
			_, _ = playerctlValue(player, "play")
			return "playerctl-open", nil
		}
	}

	if _, err := exec.LookPath("vlc"); err != nil {
		return "", errors.New("`vlc` command not found and playerctl open failed")
	}
	cmd := tracedExecCommand("vlc", "--one-instance", absTarget)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to launch vlc: %w", err)
	}
	_ = cmd.Process.Release()
	return "vlc-launch", nil
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func isProcessRunning(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, "pgrep", "-x", name)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func dedupePaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}
