package omni

import "testing"

func TestClassifyToolchainFeedbackGoUndefined(t *testing.T) {
	feedback := ClassifyToolchainFeedback("go test ./...", "", "# app\n./main.go:10:7: undefined: solve\nFAIL")
	if feedback.Toolchain != "go" || feedback.Kind != ToolchainFeedbackCompileError {
		t.Fatalf("feedback = %#v", feedback)
	}
}

func TestClassifyToolchainFeedbackRustTestFailure(t *testing.T) {
	feedback := ClassifyToolchainFeedback("cargo test", "thread 'tests::moves' panicked at src/main.rs:10:5\ntest result: FAILED. 0 passed; 1 failed", "")
	if feedback.Toolchain != "rust" || feedback.Kind != ToolchainFeedbackTestFailure {
		t.Fatalf("feedback = %#v", feedback)
	}
}

func TestClassifyToolchainFeedbackZigCompileError(t *testing.T) {
	feedback := ClassifyToolchainFeedback("zig build test", "", "src/main.zig:3:1: error: expected ';' after statement")
	if feedback.Toolchain != "zig" || feedback.Kind != ToolchainFeedbackCompileError {
		t.Fatalf("feedback = %#v", feedback)
	}
}

func TestClassifyToolchainFeedbackNPMViteImportFailure(t *testing.T) {
	feedback := ClassifyToolchainFeedback("npm run build", "", "[vite]: Rollup failed to resolve import \"react\" from src/App.jsx")
	if feedback.Toolchain != "vite" || feedback.Kind != ToolchainFeedbackDependencyMissing {
		t.Fatalf("feedback = %#v", feedback)
	}
}
