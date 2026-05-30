package cursorrunner

import (
	"fmt"
	"strings"
)

// PreflightIssue describes a missing host dependency for Cursor SDK runs.
type PreflightIssue struct {
	Tool string
	Hint string
}

// Preflight checks that node, npm, and base64 are reachable on the augmented PATH.
func Preflight() []PreflightIssue {
	env := CommandEnv()
	checks := []struct {
		tool string
		hint string
	}{
		{NodeBin(), "install Node.js or set OMNI_CURSOR_NODE_BIN to the full node path"},
		{NPMBin(), "install npm or set OMNI_CURSOR_NPM_BIN to the full npm path"},
		{"base64", "ensure /usr/bin is on PATH (Cursor SDK shell helpers require base64)"},
	}
	issues := make([]PreflightIssue, 0, len(checks))
	for _, check := range checks {
		if _, err := lookPathInEnv(check.tool, env); err != nil {
			issues = append(issues, PreflightIssue{Tool: check.tool, Hint: check.hint})
		}
	}
	return issues
}

// PreflightError formats preflight issues as a single actionable error.
func PreflightError(issues []PreflightIssue) error {
	if len(issues) == 0 {
		return nil
	}
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, fmt.Sprintf("%s not found (%s)", issue.Tool, issue.Hint))
	}
	return fmt.Errorf("cursor host preflight failed: %s", strings.Join(parts, "; "))
}
