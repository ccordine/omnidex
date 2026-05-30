package projectgit

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	ChangedFileLimit = 64
	CommitLimit      = 12
)

func CollectStatus(ctx context.Context, location, source string) (map[string]any, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, fmt.Errorf("project location is required")
	}
	if strings.TrimSpace(source) == "" {
		source = "local"
	}

	payload := map[string]any{
		"location": location,
		"source":   source,
		"is_repo":  false,
	}

	inside, err := gitOutput(ctx, location, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		payload["message"] = "Not a git repository"
		if err != nil {
			payload["error"] = strings.TrimSpace(err.Error())
		}
		return payload, nil
	}

	payload["is_repo"] = true

	root, _ := gitOutput(ctx, location, "rev-parse", "--show-toplevel")
	payload["root"] = strings.TrimSpace(root)

	branch, _ := gitOutput(ctx, location, "branch", "--show-current")
	branch = strings.TrimSpace(branch)
	detached := branch == ""
	payload["branch"] = branch
	payload["detached"] = detached

	headShort, _ := gitOutput(ctx, location, "rev-parse", "--short", "HEAD")
	payload["head_short"] = strings.TrimSpace(headShort)

	upstream, upstreamErr := gitOutput(ctx, location, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	upstream = strings.TrimSpace(upstream)
	hasUpstream := upstreamErr == nil && upstream != "" && upstream != "@{upstream}"
	payload["has_upstream"] = hasUpstream
	payload["upstream_branch"] = upstream

	ahead, behind := 0, 0
	if hasUpstream {
		counts, err := gitOutput(ctx, location, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
		if err == nil {
			parts := strings.Fields(strings.TrimSpace(counts))
			if len(parts) == 2 {
				behind, _ = strconv.Atoi(parts[0])
				ahead, _ = strconv.Atoi(parts[1])
			}
		}
	}
	payload["ahead"] = ahead
	payload["behind"] = behind

	remoteURL, _ := gitOutput(ctx, location, "remote", "get-url", "origin")
	payload["remote_url"] = strings.TrimSpace(remoteURL)

	staged, modified, untracked, deleted, conflicted := 0, 0, 0, 0, 0
	changedFiles := make([]map[string]any, 0, ChangedFileLimit)
	if porcelain, err := gitOutput(ctx, location, "status", "--porcelain=v1", "-u"); err == nil {
		for _, line := range strings.Split(porcelain, "\n") {
			line = strings.TrimRight(line, "\r")
			if line == "" {
				continue
			}
			if len(line) < 3 {
				continue
			}
			indexStatus := line[0:1]
			worktreeStatus := line[1:2]
			path := strings.TrimSpace(line[3:])
			if arrow := strings.Index(path, " -> "); arrow >= 0 {
				path = path[arrow+4:]
			}

			switch {
			case indexStatus == "?" && worktreeStatus == "?":
				untracked++
			case indexStatus == "U" || worktreeStatus == "U" || (indexStatus == "A" && worktreeStatus == "A") || (indexStatus == "D" && worktreeStatus == "D"):
				conflicted++
			case indexStatus == "D" || worktreeStatus == "D":
				deleted++
			default:
				if indexStatus != " " && indexStatus != "?" {
					staged++
				}
				if worktreeStatus != " " && worktreeStatus != "?" {
					modified++
				}
			}

			if len(changedFiles) < ChangedFileLimit {
				changedFiles = append(changedFiles, map[string]any{
					"path":            path,
					"index_status":    indexStatus,
					"worktree_status": worktreeStatus,
					"status":          indexStatus + worktreeStatus,
				})
			}
		}
	}
	payload["staged_count"] = staged
	payload["modified_count"] = modified
	payload["untracked_count"] = untracked
	payload["deleted_count"] = deleted
	payload["conflicted_count"] = conflicted
	payload["changed_files"] = changedFiles
	payload["clean"] = staged+modified+untracked+deleted+conflicted == 0

	stashList, _ := gitOutput(ctx, location, "stash", "list")
	stashCount := 0
	if strings.TrimSpace(stashList) != "" {
		stashCount = len(strings.Split(strings.TrimSpace(stashList), "\n"))
	}
	payload["stash_count"] = stashCount

	commits := make([]map[string]any, 0, CommitLimit)
	if logOut, err := gitOutput(ctx, location, "log", fmt.Sprintf("-%d", CommitLimit), "--format=%H|%s|%an|%ar"); err == nil {
		for _, line := range strings.Split(logOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 4)
			if len(parts) < 4 {
				continue
			}
			commits = append(commits, map[string]any{
				"hash":          parts[0][:minInt(12, len(parts[0]))],
				"subject":       parts[1],
				"author":        parts[2],
				"relative_date": parts[3],
			})
		}
	}
	payload["recent_commits"] = commits
	if len(commits) > 0 {
		payload["last_commit"] = commits[0]
	}

	return payload, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}
	return string(out), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
