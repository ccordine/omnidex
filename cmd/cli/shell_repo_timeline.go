package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func collectRepoTimeline(repoRoot string) ([]repoTimelineEntry, error) {
	raw, err := runLocalCommandMax([]string{"git", "-C", repoRoot, "status", "--porcelain=1", "--untracked-files=all"}, localShellCommandTimeout, 12000)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []repoTimelineEntry{}, nil
	}

	stats := collectRepoDiffStatsForRepo(repoRoot)
	entries := make([]repoTimelineEntry, 0, 16)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := parseGitPorcelainPath(line[3:])
		if path == "" {
			continue
		}
		entry := repoTimelineEntry{
			Path:   path,
			Status: status,
			Kind:   describeRepoStatusKind(status),
		}
		if stat, ok := stats[path]; ok {
			entry.Stats = stat
		}

		absPath := filepath.Join(repoRoot, path)
		if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
			entry.WorkingMTime = info.ModTime()
		}
		commitTime, commitHash, commitSubject := queryRepoFileLastCommit(repoRoot, path)
		entry.CommitTime = commitTime
		entry.CommitHash = commitHash
		entry.CommitSubject = commitSubject

		entry.EffectiveTime = entry.CommitTime
		entry.TimeSource = "last commit"
		if !entry.WorkingMTime.IsZero() && (entry.EffectiveTime.IsZero() || entry.WorkingMTime.After(entry.EffectiveTime)) {
			entry.EffectiveTime = entry.WorkingMTime
			entry.TimeSource = "working tree mtime"
		}
		if entry.EffectiveTime.IsZero() {
			entry.TimeSource = "unknown"
		}

		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i].EffectiveTime
		right := entries[j].EffectiveTime
		if !left.Equal(right) {
			return left.After(right)
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func describeRepoStatusKind(status string) string {
	clean := strings.TrimSpace(status)
	if clean == "" {
		return "Edited"
	}
	if clean == "??" {
		return "Added"
	}
	if statusContainsRune(clean, 'R') {
		return "Renamed"
	}
	if statusContainsRune(clean, 'D') {
		return "Deleted"
	}
	if statusContainsRune(clean, 'A') {
		return "Added"
	}
	if statusContainsRune(clean, 'M') {
		return "Edited"
	}
	return "Edited"
}

func collectRepoDiffStatsForRepo(repoRoot string) map[string]repoDiffStat {
	stats := map[string]repoDiffStat{}
	for _, args := range [][]string{
		{"git", "-C", repoRoot, "diff", "--numstat", "--"},
		{"git", "-C", repoRoot, "diff", "--cached", "--numstat", "--"},
	} {
		raw, err := runLocalCommandMax(args, localShellCommandTimeout, 12000)
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

func queryRepoFileLastCommit(repoRoot, path string) (time.Time, string, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return time.Time{}, "", ""
	}
	raw, err := runLocalCommandMax([]string{"git", "-C", repoRoot, "log", "-1", "--format=%ct\t%h\t%s", "--", path}, 5*time.Second, 3000)
	if err != nil || strings.TrimSpace(raw) == "" {
		return time.Time{}, "", ""
	}
	parts := strings.SplitN(strings.TrimSpace(raw), "\t", 3)
	if len(parts) < 3 {
		return time.Time{}, "", ""
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return time.Time{}, "", ""
	}
	return time.Unix(epoch, 0).Local(), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
}

func collectRecentRepoCommits(repoRoot string, limit int) []repoCommitEntry {
	if limit <= 0 {
		limit = 6
	}
	raw, err := runLocalCommandMax(
		[]string{"git", "-C", repoRoot, "log", fmt.Sprintf("-%d", limit), "--format=%ct\t%h\t%s"},
		6*time.Second,
		6000,
	)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	out := make([]repoCommitEntry, 0, limit)
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(parts) < 3 {
			continue
		}
		epoch, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, repoCommitEntry{
			Time:    time.Unix(epoch, 0).Local(),
			Hash:    strings.TrimSpace(parts[1]),
			Subject: strings.TrimSpace(parts[2]),
		})
	}
	return out
}

func gitCurrentBranch(repoRoot string) (string, error) {
	return runLocalCommandMax([]string{"git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD"}, 4*time.Second, 800)
}
