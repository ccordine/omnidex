package main

import (
	"fmt"
	"strings"
)

func splitTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.ToLower(strings.TrimSpace(part))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func applyExecutionProfile(
	rawArgs []string,
	profile string,
	webMode *string,
	workspaceMode *string,
	allowMissingTools *bool,
	reasoningLevel *string,
	autonomyMode *string,
	approvalMode *string,
	verificationMode *string,
	verificationIterations *int,
	verbose *bool,
	maxChars *int,
	localShell *bool,
) (bool, error) {
	selected := strings.ToLower(strings.TrimSpace(profile))
	if selected == "" || selected == "default" {
		return false, nil
	}
	if selected != "architect" {
		return false, fmt.Errorf("invalid --profile value %q (use default|architect)", profile)
	}

	if webMode != nil && !flagProvided(rawArgs, "web") {
		*webMode = "auto"
	}
	if workspaceMode != nil && !flagProvided(rawArgs, "workspace") {
		*workspaceMode = "on"
	}
	if allowMissingTools != nil && !flagProvided(rawArgs, "allow-missing-tools") {
		*allowMissingTools = true
	}
	if reasoningLevel != nil && !flagProvided(rawArgs, "reasoning") {
		*reasoningLevel = "deep"
	}
	if autonomyMode != nil && !flagProvided(rawArgs, "autonomy") {
		*autonomyMode = "on"
	}
	if approvalMode != nil && !flagProvided(rawArgs, "approval") {
		*approvalMode = "on"
	}
	if verificationMode != nil && !flagProvided(rawArgs, "verify") {
		*verificationMode = "on"
	}
	if verificationIterations != nil && !flagProvided(rawArgs, "verify-iterations") {
		*verificationIterations = 3
	}
	if verbose != nil && !flagProvided(rawArgs, "verbose") {
		*verbose = true
	}
	if maxChars != nil && !flagProvided(rawArgs, "max-chars") {
		*maxChars = 2200
	}
	if localShell != nil && !flagProvided(rawArgs, "local-shell") {
		*localShell = true
	}
	return true, nil
}

func flagProvided(args []string, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	long := "--" + name
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == long || strings.HasPrefix(trimmed, long+"=") {
			return true
		}
	}
	return false
}
