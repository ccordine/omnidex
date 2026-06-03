package codexrunner

import (
	"fmt"
	"strings"
)

// PreflightIssue describes a missing host dependency for Codex SDK runs.
type PreflightIssue struct {
	Tool string
	Hint string
}

// Preflight checks that node, npm, and the Codex CLI are reachable on the augmented PATH.
func Preflight() []PreflightIssue {
	return PreflightFor(NodeBin(), NPMBin(), CodexBin())
}

func PreflightFor(nodeBin, npmBin, codexBin string) []PreflightIssue {
	checks := []struct {
		tool string
		hint string
	}{
		{firstNonEmpty(nodeBin, NodeBin()), "install Node.js or set OMNI_CODEX_NODE_BIN to the full node path"},
		{firstNonEmpty(npmBin, NPMBin()), "install npm or set OMNI_CODEX_NPM_BIN to the full npm path"},
		{firstNonEmpty(codexBin, CodexBin()), "install Codex CLI or set OMNI_CODEX_BIN to the full codex path"},
	}
	env := CommandEnv()
	issues := make([]PreflightIssue, 0, len(checks))
	for _, check := range checks {
		if _, err := lookPathInEnv(check.tool, env); err != nil {
			issues = append(issues, PreflightIssue{Tool: check.tool, Hint: check.hint})
		}
	}
	return issues
}

// ResolveCodexPath returns the exact Codex executable path used by the Node SDK runner.
func ResolveCodexPath(codexBin string) (string, error) {
	codexBin = firstNonEmpty(codexBin, CodexBin())
	path, err := lookPathInEnv(codexBin, CommandEnv())
	if err != nil {
		return "", fmt.Errorf("codex binary is not available in PATH (%s); install Codex CLI or set OMNI_CODEX_BIN", codexBin)
	}
	return path, nil
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
	return fmt.Errorf("codex host preflight failed: %s", strings.Join(parts, "; "))
}
