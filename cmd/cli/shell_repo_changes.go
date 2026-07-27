package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func captureRepoWorkingTreeSnapshot() repoWorkingTreeSnapshot {
	snapshot := repoWorkingTreeSnapshot{
		Available: false,
		Files:     map[string]repoWorkingFileState{},
	}
	if !commandExists("git") {
		return snapshot
	}
	if _, err := runLocalCommand([]string{"git", "rev-parse", "--is-inside-work-tree"}, 4*time.Second); err != nil {
		return snapshot
	}

	raw, err := runLocalCommand([]string{"git", "status", "--porcelain=1", "--untracked-files=all"}, localShellCommandTimeout)
	if err != nil {
		return snapshot
	}
	snapshot.Available = true
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := parseGitPorcelainPath(line[3:])
		if path == "" {
			continue
		}
		snapshot.Files[path] = repoWorkingFileState{
			Status: status,
			Hash:   hashLocalFile(path),
		}
	}
	return snapshot
}

func parseGitPorcelainPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, " -> "); idx >= 0 {
		path = strings.TrimSpace(path[idx+4:])
	}
	path = strings.Trim(path, "\"")
	return strings.TrimSpace(path)
}

func hashLocalFile(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func buildRepoChangeSummary(before repoWorkingTreeSnapshot) string {
	if !before.Available {
		return ""
	}
	after := captureRepoWorkingTreeSnapshot()
	if !after.Available {
		return ""
	}

	changed := collectRepoChangedPaths(before, after)
	if len(changed) == 0 {
		return ""
	}

	stats := collectRepoDiffStats()
	lines := []string{"Code changes:"}
	const maxFiles = 6
	for i, path := range changed {
		if i >= maxFiles {
			lines = append(lines, fmt.Sprintf("• ... %d more file(s) changed", len(changed)-maxFiles))
			break
		}
		_, hadBefore := before.Files[path]
		afterState, hasAfter := after.Files[path]

		kind := describeRepoChangeKind(hadBefore, hasAfter, afterState)
		statText := ""
		if stat, ok := stats[path]; ok && stat.Known {
			statText = fmt.Sprintf(" (+%d -%d)", stat.Added, stat.Removed)
		} else if hasAfter && strings.TrimSpace(afterState.Status) == "??" {
			statText = " (untracked)"
		}
		lines = append(lines, fmt.Sprintf("• %s %s%s", kind, path, statText))

		if hasAfter {
			if snippet := readRepoDiffSnippet(path, afterState.Status); snippet != "" {
				lines = append(lines, "```diff")
				lines = append(lines, snippet)
				lines = append(lines, "```")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func collectRepoChangedPaths(before, after repoWorkingTreeSnapshot) []string {
	all := map[string]struct{}{}
	for path := range before.Files {
		all[path] = struct{}{}
	}
	for path := range after.Files {
		all[path] = struct{}{}
	}

	changed := make([]string, 0, len(all))
	for path := range all {
		b, bok := before.Files[path]
		a, aok := after.Files[path]
		if !bok || !aok {
			changed = append(changed, path)
			continue
		}
		if strings.TrimSpace(b.Status) != strings.TrimSpace(a.Status) || strings.TrimSpace(b.Hash) != strings.TrimSpace(a.Hash) {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func describeRepoChangeKind(hadBefore bool, hasAfter bool, after repoWorkingFileState) string {
	if !hadBefore && hasAfter {
		if statusContainsRune(after.Status, 'A') || strings.TrimSpace(after.Status) == "??" {
			return "Added"
		}
		if statusContainsRune(after.Status, 'D') {
			return "Deleted"
		}
		if statusContainsRune(after.Status, 'R') {
			return "Renamed"
		}
		return "Edited"
	}
	if hadBefore && !hasAfter {
		return "Reverted"
	}
	if hasAfter {
		if statusContainsRune(after.Status, 'D') {
			return "Deleted"
		}
		if statusContainsRune(after.Status, 'R') {
			return "Renamed"
		}
		if statusContainsRune(after.Status, 'A') || strings.TrimSpace(after.Status) == "??" {
			return "Added"
		}
	}
	return "Edited"
}

func statusContainsRune(status string, target rune) bool {
	for _, ch := range strings.TrimSpace(status) {
		if ch == target {
			return true
		}
	}
	return false
}

func collectRepoDiffStats() map[string]repoDiffStat {
	stats := map[string]repoDiffStat{}
	for _, args := range [][]string{
		{"git", "diff", "--numstat", "--"},
		{"git", "diff", "--cached", "--numstat", "--"},
	} {
		raw, err := runLocalCommand(args, localShellCommandTimeout)
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		for _, line := range strings.Split(raw, "\n") {
			path, stat, ok := parseRepoNumstatLine(line)
			if !ok {
				continue
			}
			prev, exists := stats[path]
			if !exists {
				stats[path] = stat
				continue
			}
			if prev.Known && stat.Known {
				prev.Added += stat.Added
				prev.Removed += stat.Removed
				prev.Known = true
			} else {
				prev.Known = false
			}
			stats[path] = prev
		}
	}
	return stats
}

func parseRepoNumstatLine(line string) (string, repoDiffStat, bool) {
	parts := strings.SplitN(strings.TrimSpace(line), "\t", 3)
	if len(parts) < 3 {
		return "", repoDiffStat{}, false
	}
	path := strings.TrimSpace(parts[2])
	if path == "" {
		return "", repoDiffStat{}, false
	}
	added, errAdded := strconv.Atoi(strings.TrimSpace(parts[0]))
	removed, errRemoved := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errAdded != nil || errRemoved != nil {
		return path, repoDiffStat{Known: false}, true
	}
	return path, repoDiffStat{
		Added:   added,
		Removed: removed,
		Known:   true,
	}, true
}

func readRepoDiffSnippet(path, status string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.TrimSpace(status) == "??" {
		return ""
	}
	for _, args := range [][]string{
		{"git", "--no-pager", "diff", "--", path},
		{"git", "--no-pager", "diff", "--cached", "--", path},
	} {
		raw, err := runLocalCommand(args, localShellCommandTimeout)
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		return trimDiffSnippet(raw, 32, 900)
	}
	return ""
}

func trimDiffSnippet(value string, maxLines, maxChars int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = append(lines[:maxLines], "...(truncated)")
	}
	out := strings.Join(lines, "\n")
	if maxChars > 0 && len(out) > maxChars {
		out = out[:maxChars] + "...(truncated)"
	}
	return strings.TrimSpace(out)
}
