package projectgit

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

func collectChangedFiles(ctx context.Context, runner commandRunner, location string, status *Status) error {
	porcelain, err := requiredOutputAllowEmpty(ctx, runner, location, "status", "status", "--porcelain=v1", "-u")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(porcelain, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if len(line) < 4 || line[2] != ' ' {
			return fmt.Errorf("git status returned malformed porcelain row %q", line)
		}
		indexStatus, worktreeStatus, path := line[0:1], line[1:2], line[3:]
		if arrow := strings.Index(path, " -> "); arrow >= 0 {
			path = path[arrow+4:]
		}
		if path == "" {
			return fmt.Errorf("git status returned an empty changed path")
		}
		switch {
		case indexStatus == "?" && worktreeStatus == "?":
			status.UntrackedCount++
		case indexStatus == "U" || worktreeStatus == "U" || (indexStatus == "A" && worktreeStatus == "A") || (indexStatus == "D" && worktreeStatus == "D"):
			status.ConflictedCount++
		case indexStatus == "D" || worktreeStatus == "D":
			status.DeletedCount++
		default:
			if indexStatus != " " && indexStatus != "?" {
				status.StagedCount++
			}
			if worktreeStatus != " " && worktreeStatus != "?" {
				status.ModifiedCount++
			}
		}
		if len(status.ChangedFiles) < ChangedFileLimit {
			status.ChangedFiles = append(status.ChangedFiles, ChangedFile{
				Path: path, IndexStatus: indexStatus, WorktreeStatus: worktreeStatus, Status: indexStatus + worktreeStatus,
			})
		}
	}
	status.Clean = status.StagedCount+status.ModifiedCount+status.UntrackedCount+status.DeletedCount+status.ConflictedCount == 0
	return nil
}

func collectCommits(ctx context.Context, runner commandRunner, location string, status *Status) error {
	logOutput, err := runner.Output(ctx, location, "log", fmt.Sprintf("-%d", CommitLimit), "--format=%H%x00%s%x00%an%x00%ar%x1e")
	if err != nil {
		return fmt.Errorf("git log command failed: %w", err)
	}
	if !utf8.ValidString(logOutput) {
		return fmt.Errorf("git log command returned invalid UTF-8")
	}
	logOutput = strings.Trim(logOutput, "\r\n")
	if logOutput == "" {
		return fmt.Errorf("git log command returned an empty value")
	}
	for _, rawRecord := range strings.Split(logOutput, "\x1e") {
		record := strings.Trim(rawRecord, "\r\n")
		if record == "" {
			continue
		}
		parts := strings.Split(record, "\x00")
		if len(parts) != 4 || len(parts[0]) < 12 {
			return fmt.Errorf("git log returned a malformed commit record")
		}
		status.RecentCommits = append(status.RecentCommits, Commit{
			Hash: parts[0][:12], Subject: parts[1], Author: parts[2], RelativeDate: parts[3],
		})
	}
	if len(status.RecentCommits) == 0 {
		return fmt.Errorf("git log returned no commits for a repository with HEAD")
	}
	last := status.RecentCommits[0]
	status.LastCommit = &last
	return nil
}
