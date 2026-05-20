package omni

import "strings"

type FailureFingerprint struct {
	Kind        string `json:"kind"`
	Summary     string `json:"summary"`
	Remediation string `json:"remediation,omitempty"`
}

func ClassifyFailure(output string) FailureFingerprint {
	lower := strings.ToLower(strings.TrimSpace(output))
	switch {
	case lower == "":
		return FailureFingerprint{Kind: "unknown", Summary: "No failure output was captured."}
	case strings.Contains(lower, "command not found") || strings.Contains(lower, "executable file not found"):
		return FailureFingerprint{Kind: "missing_command", Summary: "A required command is unavailable.", Remediation: "Probe installed tools or install the missing host dependency before retrying."}
	case strings.Contains(lower, "permission denied") || strings.Contains(lower, "operation not permitted"):
		return FailureFingerprint{Kind: "permission_denied", Summary: "The operation was blocked by filesystem, process, or sandbox permissions.", Remediation: "Request permission or choose a non-privileged evidence path."}
	case strings.Contains(lower, "eaddrinuse") || strings.Contains(lower, "address already in use") || strings.Contains(lower, "port is already allocated"):
		return FailureFingerprint{Kind: "port_in_use", Summary: "The requested port is already in use.", Remediation: "Detect the listener or choose another port before retrying."}
	case strings.Contains(lower, "could not resolve host") || strings.Contains(lower, "temporary failure in name resolution") || strings.Contains(lower, "network is unreachable"):
		return FailureFingerprint{Kind: "network_failure", Summary: "Network access failed.", Remediation: "Retry with network permission or use local evidence if acceptable."}
	case strings.Contains(lower, "no such file or directory") || strings.Contains(lower, "cannot find module") || strings.Contains(lower, "can't resolve"):
		return FailureFingerprint{Kind: "missing_file", Summary: "A required file, module, or path is missing.", Remediation: "Inspect the workspace and create or correct the missing path."}
	case strings.Contains(lower, "syntax error") || strings.Contains(lower, "unexpected token") || strings.Contains(lower, "parse error"):
		return FailureFingerprint{Kind: "syntax_error", Summary: "A parser or shell reported invalid syntax.", Remediation: "Inspect the referenced file or command and apply a minimal syntax fix."}
	case strings.Contains(lower, "test failed") || strings.Contains(lower, "failed tests") || strings.Contains(lower, "--- fail:") || strings.Contains(lower, "npm err! test"):
		return FailureFingerprint{Kind: "test_failure", Summary: "The verification command reported failing tests.", Remediation: "Use the failing test names and assertions as the next targeted repair context."}
	case strings.Contains(lower, "404 not found") || strings.Contains(lower, "not found - get https://registry.npmjs.org"):
		return FailureFingerprint{Kind: "dependency_unavailable", Summary: "A dependency or remote artifact was not found.", Remediation: "Verify the package name or switch to the correct dependency source."}
	default:
		return FailureFingerprint{Kind: "unknown", Summary: "The failure did not match a known deterministic fingerprint."}
	}
}

func ClassifyObservationFailure(obs StructuredCommandObservation) FailureFingerprint {
	return ClassifyFailure(strings.TrimSpace(obs.Stderr + "\n" + obs.Stdout))
}
