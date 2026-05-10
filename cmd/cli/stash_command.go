package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type stashAction string

const (
	stashActionPush  stashAction = "push"
	stashActionList  stashAction = "list"
	stashActionPop   stashAction = "pop"
	stashActionApply stashAction = "apply"
)

type stashOptions struct {
	Prefix      string
	Message     string
	TrackedOnly bool
	IncludeAll  bool
	Action      stashAction
	Ref         string
}

var stashNow = time.Now

func runStash(args []string) {
	opts, help, err := parseStashArgs(args)
	if err != nil {
		die(err.Error())
	}
	if help {
		printStashUsage()
		return
	}

	target, err := resolveStashTarget(opts.Prefix)
	if err != nil {
		die(err.Error())
	}
	repoRoot, err := resolveGitRepoRoot(target)
	if err != nil {
		die(err.Error())
	}

	invocation := buildGitStashInvocation(repoRoot, opts, stashNow())
	runCommandOrExit("git", invocation, repoRoot, "stash command failed")
}

func printStashUsage() {
	fmt.Println("usage: omni stash [--prefix path] [--message text] [--tracked-only] [--all] [--list|--pop [ref]|--apply [ref]]")
}

func parseStashArgs(args []string) (stashOptions, bool, error) {
	opts := stashOptions{
		Action: stashActionPush,
	}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}

		if arg == "-h" || arg == "--help" {
			return opts, true, nil
		}

		switch {
		case arg == "--prefix":
			value, nextIndex, err := requiredNextValue(args, i, "--prefix")
			if err != nil {
				return opts, false, err
			}
			i = nextIndex
			opts.Prefix = value
		case strings.HasPrefix(arg, "--prefix="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--prefix="))
			if value == "" {
				return opts, false, errors.New("--prefix requires a non-empty value")
			}
			opts.Prefix = value
		case arg == "--message" || arg == "-m":
			value, nextIndex, err := requiredNextValue(args, i, arg)
			if err != nil {
				return opts, false, err
			}
			i = nextIndex
			opts.Message = value
		case strings.HasPrefix(arg, "--message="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--message="))
			if value == "" {
				return opts, false, errors.New("--message requires a non-empty value")
			}
			opts.Message = value
		case strings.HasPrefix(arg, "-m="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "-m="))
			if value == "" {
				return opts, false, errors.New("-m requires a non-empty value")
			}
			opts.Message = value
		case arg == "--tracked-only":
			opts.TrackedOnly = true
		case arg == "--all":
			opts.IncludeAll = true
		case arg == "--list":
			if err := setStashAction(&opts, stashActionList); err != nil {
				return opts, false, err
			}
		case arg == "--pop":
			if err := setStashAction(&opts, stashActionPop); err != nil {
				return opts, false, err
			}
			if ref, consumed := optionalActionRef(args, i+1); consumed {
				opts.Ref = ref
				i++
			}
		case strings.HasPrefix(arg, "--pop="):
			if err := setStashAction(&opts, stashActionPop); err != nil {
				return opts, false, err
			}
			ref := strings.TrimSpace(strings.TrimPrefix(arg, "--pop="))
			if ref == "" {
				return opts, false, errors.New("--pop requires a non-empty ref")
			}
			opts.Ref = ref
		case arg == "--apply":
			if err := setStashAction(&opts, stashActionApply); err != nil {
				return opts, false, err
			}
			if ref, consumed := optionalActionRef(args, i+1); consumed {
				opts.Ref = ref
				i++
			}
		case strings.HasPrefix(arg, "--apply="):
			if err := setStashAction(&opts, stashActionApply); err != nil {
				return opts, false, err
			}
			ref := strings.TrimSpace(strings.TrimPrefix(arg, "--apply="))
			if ref == "" {
				return opts, false, errors.New("--apply requires a non-empty ref")
			}
			opts.Ref = ref
		default:
			return opts, false, fmt.Errorf("unknown option: %s", arg)
		}
	}

	if opts.TrackedOnly && opts.IncludeAll {
		return opts, false, errors.New("--tracked-only cannot be combined with --all")
	}
	return opts, false, nil
}

func setStashAction(opts *stashOptions, action stashAction) error {
	if opts == nil {
		return errors.New("stash options are required")
	}
	if opts.Action == stashActionPush || opts.Action == "" || opts.Action == action {
		opts.Action = action
		return nil
	}
	return errors.New("choose only one action: --list, --pop, or --apply")
}

func optionalActionRef(args []string, index int) (string, bool) {
	if index >= len(args) {
		return "", false
	}
	value := strings.TrimSpace(args[index])
	if value == "" || strings.HasPrefix(value, "-") {
		return "", false
	}
	return value, true
}

func requiredNextValue(args []string, index int, flagName string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", flagName)
	}
	value := strings.TrimSpace(args[index+1])
	if value == "" {
		return "", index, fmt.Errorf("%s requires a non-empty value", flagName)
	}
	return value, index + 1, nil
}

func resolveStashTarget(explicitPrefix string) (string, error) {
	target := strings.TrimSpace(expandHomePath(explicitPrefix))
	cwd := strings.TrimSpace(currentWorkingDirectory())

	if target == "" && cwd != "" && looksLikeOmnidexRepoRoot(cwd) {
		target = cwd
	}
	if target == "" {
		target = strings.TrimSpace(expandHomePath(os.Getenv(omniRuntimeDirEnv)))
	}
	if target == "" {
		target = cwd
	}
	if target == "" {
		return "", errors.New("unable to locate repository; pass --prefix <path>")
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve stash target %q: %w", target, err)
	}
	return filepath.Clean(abs), nil
}

func resolveGitRepoRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("repository path is required")
	}

	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(output))
		if details == "" {
			details = err.Error()
		}
		return "", fmt.Errorf("unable to resolve git repository at %s: %s", path, details)
	}

	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("unable to resolve git repository at %s", path)
	}

	if abs, absErr := filepath.Abs(root); absErr == nil {
		root = abs
	}
	return filepath.Clean(root), nil
}

func buildGitStashInvocation(repoRoot string, opts stashOptions, now time.Time) []string {
	args := []string{"-C", repoRoot, "stash"}

	switch opts.Action {
	case stashActionList:
		return append(args, "list")
	case stashActionPop:
		args = append(args, "pop")
		if ref := strings.TrimSpace(opts.Ref); ref != "" {
			args = append(args, ref)
		}
		return args
	case stashActionApply:
		args = append(args, "apply")
		if ref := strings.TrimSpace(opts.Ref); ref != "" {
			args = append(args, ref)
		}
		return args
	default:
		args = append(args, "push")
		if opts.IncludeAll {
			args = append(args, "--all")
		} else if !opts.TrackedOnly {
			args = append(args, "--include-untracked")
		}

		message := strings.TrimSpace(opts.Message)
		if message == "" {
			message = fmt.Sprintf("omni stash %s", now.Format(time.RFC3339))
		}
		args = append(args, "--message", message)
		return args
	}
}

func runCommandOrExit(name string, args []string, workdir string, context string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		die(fmt.Sprintf("%s: %v", context, err))
	}
}
