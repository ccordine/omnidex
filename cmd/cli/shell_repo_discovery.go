package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type repoLookupResult struct {
	Root         string
	Reason       string
	Alternatives []string
}

type repoTimelineEntry struct {
	Path          string
	Status        string
	Kind          string
	Stats         repoDiffStat
	WorkingMTime  time.Time
	CommitTime    time.Time
	CommitHash    string
	CommitSubject string
	EffectiveTime time.Time
	TimeSource    string
}

type repoCommitEntry struct {
	Time    time.Time
	Hash    string
	Subject string
}

func showRepositoryWalkthrough() (string, error) {
	repo, err := locateNearbyRepository()
	if err != nil {
		return "", err
	}

	branch := "(unknown)"
	if value, err := gitCurrentBranch(repo.Root); err == nil && strings.TrimSpace(value) != "" {
		branch = strings.TrimSpace(value)
	}

	entries, err := collectRepoTimeline(repo.Root)
	if err != nil {
		return "", err
	}
	recentCommits := collectRecentRepoCommits(repo.Root, 6)

	lines := []string{
		"Repository walkthrough:",
		"selected_repo=" + repo.Root,
		"selection_reason=" + safeValue(repo.Reason, "unknown"),
		"branch=" + branch,
		fmt.Sprintf("changed_files=%d", len(entries)),
	}
	if len(repo.Alternatives) > 0 {
		lines = append(lines, "other_nearby_repos="+strings.Join(repo.Alternatives, ","))
	}

	if len(entries) == 0 {
		lines = append(lines, "working_tree=clean (no uncommitted file changes detected)")
	} else {
		lines = append(lines, "chronological_changes (most recent first):")
		const maxItems = 20
		for i, entry := range entries {
			if i >= maxItems {
				lines = append(lines, fmt.Sprintf("- ... %d more changed file(s)", len(entries)-maxItems))
				break
			}
			statText := ""
			if entry.Stats.Known {
				statText = fmt.Sprintf(" (+%d -%d)", entry.Stats.Added, entry.Stats.Removed)
			} else if strings.TrimSpace(entry.Status) == "??" {
				statText = " (untracked)"
			}
			lines = append(lines, fmt.Sprintf("- %s %s%s", safeValue(entry.Kind, "Edited"), entry.Path, statText))
			if !entry.EffectiveTime.IsZero() {
				lines = append(lines, "  last_changed="+entry.EffectiveTime.Format(time.RFC3339)+" source="+safeValue(entry.TimeSource, "unknown"))
			}
			if !entry.CommitTime.IsZero() && strings.TrimSpace(entry.CommitHash) != "" {
				lines = append(lines, fmt.Sprintf("  last_commit=%s %s %s", entry.CommitTime.Format(time.RFC3339), entry.CommitHash, strings.TrimSpace(entry.CommitSubject)))
			}
		}
	}

	if len(recentCommits) > 0 {
		lines = append(lines, "recent_commits:")
		for _, commit := range recentCommits {
			lines = append(lines, fmt.Sprintf("- %s %s %s", commit.Time.Format(time.RFC3339), commit.Hash, commit.Subject))
		}
	}

	return strings.Join(lines, "\n"), nil
}

func locateNearbyRepository() (repoLookupResult, error) {
	if !commandExists("git") {
		return repoLookupResult{}, errors.New("`git` is not available on this system")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return repoLookupResult{}, fmt.Errorf("unable to determine current directory: %w", err)
	}
	cwd, _ = filepath.Abs(cwd)

	if root, err := discoverGitRootFromDir(cwd); err == nil && strings.TrimSpace(root) != "" {
		return repoLookupResult{
			Root:   root,
			Reason: "inside current git work tree",
		}, nil
	}

	bases := []string{cwd}
	if parent := filepath.Dir(cwd); parent != "" && parent != cwd {
		bases = append(bases, parent)
	}
	if grandParent := filepath.Dir(filepath.Dir(cwd)); grandParent != "" && grandParent != cwd {
		bases = append(bases, grandParent)
	}

	candidates := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for _, base := range bases {
		repos := discoverChildGitRepos(base, 2, 80)
		for _, repo := range repos {
			validRoot, err := discoverGitRootFromDir(repo)
			if err != nil {
				continue
			}
			absRepo, _ := filepath.Abs(validRoot)
			absRepo = strings.TrimSpace(absRepo)
			if absRepo == "" {
				continue
			}
			if _, ok := seen[absRepo]; ok {
				continue
			}
			seen[absRepo] = struct{}{}
			candidates = append(candidates, absRepo)
		}
	}
	if len(candidates) == 0 {
		return repoLookupResult{}, errors.New("no git repository found in current or nearby directories")
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		di := pathDistance(cwd, candidates[i])
		dj := pathDistance(cwd, candidates[j])
		if di != dj {
			return di < dj
		}
		return candidates[i] < candidates[j]
	})

	selected := candidates[0]
	alternatives := []string{}
	for _, candidate := range candidates[1:] {
		alternatives = append(alternatives, candidate)
		if len(alternatives) >= 4 {
			break
		}
	}
	return repoLookupResult{
		Root:         selected,
		Reason:       "nearby repository discovered",
		Alternatives: alternatives,
	}, nil
}

func discoverGitRootFromDir(dir string) (string, error) {
	out, err := runLocalCommandMax([]string{"git", "-C", dir, "rev-parse", "--show-toplevel"}, 4*time.Second, 2000)
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", errors.New("empty git root")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return root, nil
	}
	return absRoot, nil
}

func discoverChildGitRepos(base string, maxDepth, maxVisited int) []string {
	base = strings.TrimSpace(base)
	if base == "" {
		return nil
	}
	info, err := os.Stat(base)
	if err != nil || !info.IsDir() {
		return nil
	}

	type queueItem struct {
		Path  string
		Depth int
	}

	repos := make([]string, 0, 8)
	seenDirs := map[string]struct{}{}
	queue := []queueItem{{Path: base, Depth: 0}}
	visited := 0

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if _, ok := seenDirs[item.Path]; ok {
			continue
		}
		seenDirs[item.Path] = struct{}{}
		visited++
		if maxVisited > 0 && visited > maxVisited {
			break
		}

		if hasGitMetadata(item.Path) {
			repos = append(repos, item.Path)
			continue
		}
		if item.Depth >= maxDepth {
			continue
		}

		entries, err := os.ReadDir(item.Path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := strings.TrimSpace(entry.Name())
			if name == "" || name == ".git" {
				continue
			}
			if strings.HasPrefix(name, ".") {
				continue
			}
			nextPath := filepath.Join(item.Path, name)
			queue = append(queue, queueItem{Path: nextPath, Depth: item.Depth + 1})
		}
	}

	sort.Strings(repos)
	return repos
}

func hasGitMetadata(path string) bool {
	gitPath := filepath.Join(strings.TrimSpace(path), ".git")
	if gitPath == "" {
		return false
	}
	_, err := os.Stat(gitPath)
	return err == nil
}

func pathDistance(fromPath, toPath string) int {
	from := splitPathSegments(fromPath)
	to := splitPathSegments(toPath)
	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}
	return (len(from) - common) + (len(to) - common)
}

func splitPathSegments(path string) []string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return nil
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
