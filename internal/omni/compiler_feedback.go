package omni

import "strings"

type ToolchainFeedback struct {
	Toolchain string   `json:"toolchain"`
	Command   string   `json:"command"`
	Kind      string   `json:"kind"`
	Summary   string   `json:"summary"`
	Hints     []string `json:"hints,omitempty"`
}

const (
	ToolchainFeedbackNone              = ""
	ToolchainFeedbackCompileError      = "compile_error"
	ToolchainFeedbackTestFailure       = "test_failure"
	ToolchainFeedbackDependencyMissing = "dependency_missing"
	ToolchainFeedbackSyntaxError       = "syntax_error"
	ToolchainFeedbackModuleError       = "module_error"
)

func ClassifyToolchainFeedback(command, stdout, stderr string) ToolchainFeedback {
	toolchain := detectToolchainFromCommand(command)
	if toolchain == "" {
		return ToolchainFeedback{}
	}
	text := strings.ToLower(stdout + "\n" + stderr)
	if toolchain == "npm" && strings.Contains(text, "vite") {
		toolchain = "vite"
	}
	feedback := ToolchainFeedback{
		Toolchain: toolchain,
		Command:   strings.TrimSpace(command),
	}
	switch toolchain {
	case "go":
		feedback.Kind, feedback.Summary, feedback.Hints = classifyGoFeedback(text)
	case "rust":
		feedback.Kind, feedback.Summary, feedback.Hints = classifyRustFeedback(text)
	case "zig":
		feedback.Kind, feedback.Summary, feedback.Hints = classifyZigFeedback(text)
	case "npm", "vite":
		feedback.Kind, feedback.Summary, feedback.Hints = classifyNPMFeedback(text)
	}
	if feedback.Kind == "" {
		return ToolchainFeedback{Toolchain: toolchain, Command: strings.TrimSpace(command)}
	}
	return feedback
}

func detectToolchainFromCommand(command string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(command)))
	if len(fields) == 0 {
		return ""
	}
	switch cleanCommandPathToken(fields[0]) {
	case "go":
		return "go"
	case "cargo", "rustc":
		return "rust"
	case "zig":
		return "zig"
	case "npm", "pnpm", "yarn", "bun", "node", "npx":
		if strings.Contains(strings.ToLower(command), "vite") {
			return "vite"
		}
		return "npm"
	default:
		if strings.Contains(strings.ToLower(command), "npm run") && strings.Contains(strings.ToLower(command), "vite") {
			return "vite"
		}
		return ""
	}
}

func classifyGoFeedback(text string) (string, string, []string) {
	switch {
	case strings.Contains(text, "no required module provides package") || strings.Contains(text, "go.mod file not found"):
		return ToolchainFeedbackModuleError, "Go module metadata or import resolution failed", []string{"inspect go.mod", "run go mod tidy only when imports are intended"}
	case strings.Contains(text, "undefined:"):
		return ToolchainFeedbackCompileError, "Go compile failed due to undefined symbol", []string{"inspect the named symbol", "add implementation or correct import/name"}
	case strings.Contains(text, "syntax error:"):
		return ToolchainFeedbackSyntaxError, "Go parser reported a syntax error", []string{"open the reported file and line", "fix syntax before broader tests"}
	case strings.Contains(text, "--- fail:") || strings.Contains(text, "fail\t"):
		return ToolchainFeedbackTestFailure, "Go tests failed", []string{"use the failing test name and assertion output as the next target"}
	case strings.Contains(text, "build failed"):
		return ToolchainFeedbackCompileError, "Go build failed", []string{"read compiler diagnostics and patch the named file"}
	default:
		return "", "", nil
	}
}

func classifyRustFeedback(text string) (string, string, []string) {
	switch {
	case strings.Contains(text, "failed to resolve") || strings.Contains(text, "unresolved import") || strings.Contains(text, "cannot find"):
		return ToolchainFeedbackCompileError, "Rust compile failed due to unresolved item or import", []string{"inspect the named path", "add implementation or import the correct item"}
	case strings.Contains(text, "error[e") || strings.Contains(text, "could not compile"):
		return ToolchainFeedbackCompileError, "Rust compiler reported errors", []string{"use rustc error codes and spans as the next patch target"}
	case strings.Contains(text, "test result: failed") || strings.Contains(text, "panicked at"):
		return ToolchainFeedbackTestFailure, "Rust tests failed", []string{"open the failing test", "patch behavior until cargo test passes"}
	default:
		return "", "", nil
	}
}

func classifyZigFeedback(text string) (string, string, []string) {
	switch {
	case strings.Contains(text, "error: unable to load") || strings.Contains(text, "no such file or directory"):
		return ToolchainFeedbackModuleError, "Zig could not load a required file or module", []string{"inspect build.zig and source paths"}
	case strings.Contains(text, "error:"):
		return ToolchainFeedbackCompileError, "Zig compiler reported errors", []string{"use the reported file and line as the next patch target"}
	case strings.Contains(text, "0 passed") || strings.Contains(text, "failed"):
		return ToolchainFeedbackTestFailure, "Zig tests failed", []string{"run the focused zig test again after patching"}
	default:
		return "", "", nil
	}
}

func classifyNPMFeedback(text string) (string, string, []string) {
	switch {
	case strings.Contains(text, "cannot find module") || strings.Contains(text, "module not found") || strings.Contains(text, "failed to resolve import"):
		return ToolchainFeedbackDependencyMissing, "JavaScript dependency or import resolution failed", []string{"inspect package.json and import path", "install only requested/required packages"}
	case strings.Contains(text, "syntaxerror") || strings.Contains(text, "unexpected token"):
		return ToolchainFeedbackSyntaxError, "JavaScript parser reported a syntax error", []string{"open the reported file and line", "fix syntax before rebuilding"}
	case strings.Contains(text, "test failed") || strings.Contains(text, "failed tests") || strings.Contains(text, "expect("):
		return ToolchainFeedbackTestFailure, "npm test reported failing tests", []string{"use the failing assertion as the next target"}
	case strings.Contains(text, "vite") && (strings.Contains(text, "error") || strings.Contains(text, "failed")):
		return ToolchainFeedbackCompileError, "Vite build failed", []string{"inspect the Vite diagnostic", "patch the named import or component"}
	default:
		return "", "", nil
	}
}
