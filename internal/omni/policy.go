package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PolicyDecision struct {
	Allowed    bool
	ReasonCode string
	Detail     string
}

var allowedCommandRoots = map[string]struct{}{
	"ls":      {},
	"pwd":     {},
	"cat":     {},
	"echo":    {},
	"printf":  {},
	"mkdir":   {},
	"touch":   {},
	"cp":      {},
	"mv":      {},
	"rm":      {},
	"find":    {},
	"rg":      {},
	"grep":    {},
	"head":    {},
	"tail":    {},
	"sed":     {},
	"awk":     {},
	"date":    {},
	"whoami":  {},
	"uname":   {},
	"go":      {},
	"gofmt":   {},
	"git":     {},
	"npm":     {},
	"pnpm":    {},
	"yarn":    {},
	"docker":  {},
	"make":    {},
	"curl":    {},
	"wget":    {},
	"jq":      {},
	"env":     {},
	"sh":      {},
	"bash":    {},
	"node":    {},
	"python3": {},
}

func EvaluateCommandPolicy(command, workspacePath string) PolicyDecision {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		return PolicyDecision{Allowed: false, ReasonCode: "empty_command", Detail: "command was empty"}
	}
	if strings.ContainsAny(normalized, "\n\r") {
		return PolicyDecision{Allowed: false, ReasonCode: "multiline_command", Detail: "multiline commands are blocked"}
	}

	if hasShellSubstitutionSyntax(normalized) {
		return PolicyDecision{Allowed: false, ReasonCode: "command_substitution_blocked", Detail: "command substitution is blocked"}
	}
	if hasShellChainSyntax(normalized) {
		return PolicyDecision{Allowed: false, ReasonCode: "chained_command_blocked", Detail: "command chaining is blocked"}
	}

	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return PolicyDecision{Allowed: false, ReasonCode: "empty_command", Detail: "command was empty after parsing"}
	}
	root := parts[0]
	if _, ok := allowedCommandRoots[root]; !ok {
		return PolicyDecision{Allowed: false, ReasonCode: "root_command_not_allowlisted", Detail: fmt.Sprintf("command %q is not allowlisted", root)}
	}
	if blocksByCommandStructure(parts) {
		return PolicyDecision{Allowed: false, ReasonCode: "unsafe_command_structure", Detail: "command structure is blocked"}
	}

	if !commandWithinWorkspace(normalized, workspacePath) {
		return PolicyDecision{Allowed: false, ReasonCode: "workspace_escape", Detail: "command appears to target paths outside workspace"}
	}

	return PolicyDecision{Allowed: true, ReasonCode: "allow", Detail: "command passed deterministic policy checks"}
}

func hasShellSubstitutionSyntax(command string) bool {
	previous := rune(0)
	for _, r := range command {
		if r == '`' {
			return true
		}
		if previous == '$' && r == '(' {
			return true
		}
		previous = r
	}
	return false
}

func hasShellChainSyntax(command string) bool {
	previous := rune(0)
	for _, r := range command {
		if r == ';' {
			return true
		}
		if previous == '&' && r == '&' {
			return true
		}
		if previous == '|' && r == '|' {
			return true
		}
		previous = r
	}
	return false
}

func blocksByCommandStructure(parts []string) bool {
	if len(parts) == 0 {
		return true
	}
	switch parts[0] {
	case "rm":
		return rmTargetsRoot(parts)
	case "git":
		return len(parts) >= 3 && parts[1] == "reset" && parts[2] == "--hard"
	}
	return false
}

func rmTargetsRoot(parts []string) bool {
	for _, part := range parts[1:] {
		clean := cleanCommandPathToken(part)
		if clean == "/" {
			return true
		}
	}
	return false
}

func commandWithinWorkspace(command, workspacePath string) bool {
	workspaceAbs, err := filepath.Abs(workspacePath)
	if err != nil {
		return false
	}
	allowedRoots := []string{workspaceAbs}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		allowedRoots = append(allowedRoots, filepath.Join(home, "Projects"))
	}

	parts := strings.Fields(command)
	for _, part := range parts[1:] {
		candidate := cleanCommandPathToken(part)
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(candidate, "-") {
			continue
		}
		if strings.Contains(candidate, "=") {
			continue
		}
		if candidate == "|" || candidate == "\\" {
			continue
		}
		if isShellRedirectToken(candidate) {
			continue
		}
		if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
			continue
		}

		resolved, pathLike := resolveCommandPathToken(candidate, workspaceAbs)
		if !pathLike {
			continue
		}
		if !isWithinAnyRoot(allowedRoots, resolved) {
			return false
		}
	}

	return true
}

func cleanCommandPathToken(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), `"'`)
}

func isShellRedirectToken(candidate string) bool {
	switch candidate {
	case ">", ">>", "<", "<<", "2>", "2>>", "1>", "1>>":
		return true
	}
	return strings.HasPrefix(candidate, "2>") || strings.HasPrefix(candidate, "1>")
}

func resolveCommandPathToken(candidate, workspaceAbs string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	if strings.HasPrefix(candidate, "~") {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return "", false
		}
		if candidate == "~" {
			return home, true
		}
		if strings.HasPrefix(candidate, "~/") {
			return filepath.Join(home, strings.TrimPrefix(candidate, "~/")), true
		}
		return "", false
	}
	if filepath.IsAbs(candidate) {
		return candidate, true
	}
	if candidate == "." || candidate == ".." || strings.HasPrefix(candidate, "./") || strings.HasPrefix(candidate, "../") {
		return filepath.Join(workspaceAbs, candidate), true
	}
	if strings.Contains(candidate, "/") {
		return filepath.Join(workspaceAbs, candidate), true
	}
	return "", false
}

func isWithinAnyRoot(roots []string, targetPath string) bool {
	for _, root := range roots {
		if isWithinWorkspace(root, targetPath) {
			return true
		}
	}
	return false
}
