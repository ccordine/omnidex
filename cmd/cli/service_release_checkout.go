package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var serviceReleaseCommitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type serviceReleaseCheckout struct {
	Root   string
	Commit string
}

func resolveServiceReleaseCheckout(
	root string,
	embeddedCommit string,
	environment []string,
	runner serviceProcessRunner,
) (serviceReleaseCheckout, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return serviceReleaseCheckout{}, fmt.Errorf("service release checkout root is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return serviceReleaseCheckout{}, fmt.Errorf("inspect service release checkout %s: %w", root, err)
	}
	if !info.IsDir() {
		return serviceReleaseCheckout{}, fmt.Errorf("service release checkout is not a directory: %s", root)
	}
	if runner == nil {
		return serviceReleaseCheckout{}, fmt.Errorf("service process runner is required")
	}
	if !serviceReleaseCommitPattern.MatchString(embeddedCommit) {
		return serviceReleaseCheckout{}, fmt.Errorf("embedded release commit must be an exact lowercase Git object identity")
	}

	checkoutRoot, err := serviceGitOutputLine(runner, environment, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return serviceReleaseCheckout{}, fmt.Errorf("target %s is not an available Git checkout: %w", root, err)
	}
	checkoutRoot, err = filepath.Abs(checkoutRoot)
	if err != nil {
		return serviceReleaseCheckout{}, fmt.Errorf("resolve target Git checkout root: %w", err)
	}
	checkoutRoot = filepath.Clean(checkoutRoot)
	gitDirectory, err := os.Lstat(filepath.Join(checkoutRoot, ".git"))
	if err != nil || !gitDirectory.IsDir() || gitDirectory.Mode()&os.ModeSymlink != 0 {
		return serviceReleaseCheckout{}, fmt.Errorf(
			"target Git checkout must have a real .git directory: %s",
			checkoutRoot,
		)
	}

	head, err := serviceGitOutputLine(runner, environment, checkoutRoot, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return serviceReleaseCheckout{}, fmt.Errorf("resolve target Git checkout HEAD: %w", err)
	}
	if !serviceReleaseCommitPattern.MatchString(head) {
		return serviceReleaseCheckout{}, fmt.Errorf("target Git checkout HEAD is not an exact lowercase object identity")
	}
	status, err := runner.Output(serviceProcessRequest{
		Invocation: []string{
			"git", "-C", checkoutRoot, "status", "--porcelain=v1",
			"--untracked-files=normal", "--ignore-submodules=none",
		},
		Workdir:     checkoutRoot,
		Environment: environment,
	})
	if err != nil {
		return serviceReleaseCheckout{}, fmt.Errorf("inspect target Git checkout state: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return serviceReleaseCheckout{}, fmt.Errorf("target Git checkout is dirty; commit or remove tracked and untracked changes before service release operations")
	}
	if head != embeddedCommit {
		return serviceReleaseCheckout{}, fmt.Errorf(
			"target Git checkout HEAD %s does not equal embedded release commit %s",
			head,
			embeddedCommit,
		)
	}
	return serviceReleaseCheckout{Root: checkoutRoot, Commit: head}, nil
}

func serviceGitOutputLine(
	runner serviceProcessRunner,
	environment []string,
	workdir string,
	arguments ...string,
) (string, error) {
	output, err := runner.Output(serviceProcessRequest{
		Invocation:  append([]string{"git", "-C", workdir}, arguments...),
		Workdir:     workdir,
		Environment: environment,
	})
	if err != nil {
		return "", err
	}
	return exactServiceOutputLine(output)
}

func exactServiceOutputLine(output string) (string, error) {
	line := strings.TrimSuffix(output, "\n")
	line = strings.TrimSuffix(line, "\r")
	if line == "" || strings.ContainsAny(line, "\r\n") || strings.TrimSpace(line) != line {
		return "", fmt.Errorf("command did not return one exact non-empty line")
	}
	return line, nil
}
