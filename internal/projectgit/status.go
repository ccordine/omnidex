package projectgit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type commandRunner interface {
	Output(context.Context, string, ...string) (string, error)
}

func CollectStatus(ctx context.Context, location, source string) (Status, error) {
	return collectStatus(ctx, location, source, execCommandRunner{})
}

func collectStatus(ctx context.Context, location, source string, runner commandRunner) (Status, error) {
	if location == "" || location != strings.TrimSpace(location) {
		return Status{}, fmt.Errorf("project location must be one exact nonblank path")
	}
	if source == "" || source != strings.TrimSpace(source) {
		return Status{}, fmt.Errorf("git status source must be one exact nonblank value")
	}
	isRepo, err := hasRepositoryMarker(location)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Location: location, Source: source,
		ChangedFiles: []ChangedFile{}, RecentCommits: []Commit{},
	}
	if !isRepo {
		status.Message = "Not a Git repository"
		return status, status.Validate()
	}
	inside, err := runner.Output(ctx, location, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return Status{}, fmt.Errorf("git repository classification failed: %w", err)
	}
	if strings.TrimSpace(inside) != "true" {
		return Status{}, fmt.Errorf("git repository classification returned %q", strings.TrimSpace(inside))
	}
	status.IsRepo = true
	if status.Root, err = requiredOutput(ctx, runner, location, "root", "rev-parse", "--show-toplevel"); err != nil {
		return Status{}, err
	}
	if status.Branch, err = requiredOutputAllowEmpty(ctx, runner, location, "branch", "branch", "--show-current"); err != nil {
		return Status{}, err
	}
	status.Detached = status.Branch == ""
	if status.HeadShort, err = requiredOutput(ctx, runner, location, "head", "rev-parse", "--short", "HEAD"); err != nil {
		return Status{}, err
	}
	if err := collectUpstream(ctx, runner, location, &status); err != nil {
		return Status{}, err
	}
	if status.RemoteURL, _, err = optionalConfig(ctx, runner, location, "remote origin", "remote.origin.url"); err != nil {
		return Status{}, err
	}
	if err := collectChangedFiles(ctx, runner, location, &status); err != nil {
		return Status{}, err
	}
	stash, err := requiredOutputAllowEmpty(ctx, runner, location, "stash", "stash", "list")
	if err != nil {
		return Status{}, err
	}
	if stash != "" {
		status.StashCount = len(strings.Split(stash, "\n"))
	}
	if err := collectCommits(ctx, runner, location, &status); err != nil {
		return Status{}, err
	}
	if err := status.Validate(); err != nil {
		return Status{}, fmt.Errorf("validate git status: %w", err)
	}
	return status, nil
}

func requiredOutput(ctx context.Context, runner commandRunner, location, label string, args ...string) (string, error) {
	value, err := requiredOutputAllowEmpty(ctx, runner, location, label, args...)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("git %s command returned an empty value", label)
	}
	return value, nil
}

func requiredOutputAllowEmpty(ctx context.Context, runner commandRunner, location, label string, args ...string) (string, error) {
	value, err := runner.Output(ctx, location, args...)
	if err != nil {
		return "", fmt.Errorf("git %s command failed: %w", label, err)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("git %s command returned invalid UTF-8 or NUL bytes", label)
	}
	return strings.TrimSpace(value), nil
}

func optionalConfig(ctx context.Context, runner commandRunner, location, label, key string) (string, bool, error) {
	value, err := runner.Output(ctx, location, "config", "--get", key)
	if err != nil {
		var commandErr *commandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("git %s command failed: %w", label, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, fmt.Errorf("git %s command returned an empty configured value", label)
	}
	return value, true, nil
}

func collectUpstream(ctx context.Context, runner commandRunner, location string, status *Status) error {
	if status.Detached {
		return nil
	}
	remote, hasRemote, err := optionalConfig(ctx, runner, location, "upstream remote", "branch."+status.Branch+".remote")
	if err != nil {
		return err
	}
	merge, hasMerge, err := optionalConfig(ctx, runner, location, "upstream merge", "branch."+status.Branch+".merge")
	if err != nil {
		return err
	}
	if hasRemote != hasMerge {
		return fmt.Errorf("git upstream configuration is incomplete")
	}
	if !hasRemote {
		return nil
	}
	_ = remote
	_ = merge
	status.UpstreamBranch, err = requiredOutput(ctx, runner, location, "upstream", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return err
	}
	counts, err := requiredOutput(ctx, runner, location, "upstream counts", "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if err != nil {
		return err
	}
	parts := strings.Fields(counts)
	if len(parts) != 2 {
		return fmt.Errorf("git upstream counts must contain exactly two integers")
	}
	status.Behind, err = parseNonnegativeInt(parts[0], "behind")
	if err != nil {
		return err
	}
	status.Ahead, err = parseNonnegativeInt(parts[1], "ahead")
	if err != nil {
		return err
	}
	status.HasUpstream = true
	return nil
}

func parseNonnegativeInt(raw, label string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || strconv.Itoa(value) != raw {
		return 0, fmt.Errorf("git %s count %q is not a canonical non-negative integer", label, raw)
	}
	return value, nil
}
