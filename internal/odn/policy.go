package odn

import (
	"fmt"
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
	"go":      {},
	"gofmt":   {},
	"git":     {},
	"npm":     {},
	"pnpm":    {},
	"yarn":    {},
	"docker":  {},
	"make":    {},
	"sh":      {},
	"bash":    {},
	"node":    {},
	"python3": {},
}

var denyFragments = []string{
	"rm -rf /",
	"mkfs",
	"dd if=",
	":(){",
	"shutdown",
	"reboot",
	"halt",
	"poweroff",
	"userdel",
	"groupdel",
	"chmod 777 /",
	"chown -r /",
	"curl ",
	"wget ",
	"scp ",
	"nc ",
	"ncat ",
	"telnet ",
	"ssh ",
	"gpg --export-secret",
	"git reset --hard",
}

func EvaluateCommandPolicy(command, workspacePath string) PolicyDecision {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		return PolicyDecision{Allowed: false, ReasonCode: "empty_command", Detail: "command was empty"}
	}
	if strings.ContainsAny(normalized, "\n\r") {
		return PolicyDecision{Allowed: false, ReasonCode: "multiline_command", Detail: "multiline commands are blocked"}
	}

	lower := strings.ToLower(normalized)
	for _, fragment := range denyFragments {
		if strings.Contains(lower, fragment) {
			return PolicyDecision{Allowed: false, ReasonCode: "deny_fragment", Detail: fmt.Sprintf("blocked fragment: %s", fragment)}
		}
	}

	if strings.ContainsAny(normalized, "`$><") {
		return PolicyDecision{Allowed: false, ReasonCode: "shell_metachar_blocked", Detail: "metacharacters are blocked (`,$,>,<)"}
	}
	if strings.Contains(normalized, "&&") || strings.Contains(normalized, "||") || strings.Contains(normalized, ";") {
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

	if !commandWithinWorkspace(normalized, workspacePath) {
		return PolicyDecision{Allowed: false, ReasonCode: "workspace_escape", Detail: "command appears to target paths outside workspace"}
	}

	return PolicyDecision{Allowed: true, ReasonCode: "allow", Detail: "command passed deterministic policy checks"}
}

func commandWithinWorkspace(command, workspacePath string) bool {
	workspaceAbs, err := filepath.Abs(workspacePath)
	if err != nil {
		return false
	}

	parts := strings.Fields(command)
	for _, part := range parts[1:] {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(candidate, "-") {
			continue
		}
		if strings.Contains(candidate, "=") {
			continue
		}
		if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
			continue
		}

		if strings.HasPrefix(candidate, "/") {
			if !isWithinWorkspace(workspaceAbs, candidate) {
				return false
			}
			continue
		}

		if strings.HasPrefix(candidate, "../") || candidate == ".." {
			return false
		}
	}

	return true
}
