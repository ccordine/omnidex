package worker

import (
	"strings"
	"testing"
)

func TestRepositoryVerificationOutputIsCompleteWithinHardBound(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("json-event\n", 40_000)
	output := newExactRepositoryCommandOutput(len(content))
	if written, err := output.Write([]byte(content)); err != nil || written != len(content) {
		t.Fatalf("write exact output bytes=%d error=%v", written, err)
	}
	if err := output.Validate("stdout"); err != nil {
		t.Fatal(err)
	}
	if output.String() != content {
		t.Fatal("bounded repository output was silently truncated")
	}
}

func TestRepositoryVerificationOutputFailsLoudlyInsteadOfTruncating(t *testing.T) {
	t.Parallel()
	output := newExactRepositoryCommandOutput(8)
	if written, err := output.Write([]byte("123456789")); err != nil || written != 9 {
		t.Fatalf("write overflow bytes=%d error=%v", written, err)
	}
	if err := output.Validate("stdout"); err == nil || !strings.Contains(err.Error(), "8-byte") {
		t.Fatalf("overflow error=%v", err)
	}
	if output.String() != "12345678" {
		t.Fatalf("overflow retained bytes beyond its hard bound: %q", output.String())
	}
}

func TestRepositoryVerificationEvidenceRetainsStructuredOutputBeyondOrdinaryLimit(t *testing.T) {
	t.Parallel()
	stdout := strings.Repeat("{\"Action\":\"pass\"}\n", 1_000)
	result := repositoryGoVerificationResult(
		[]string{"test", "-json", "-count=1", "./..."}, stdout, "", 0, nil,
	)
	if result.Output["stdout"] != stdout || len(result.Evidence) != 1 ||
		!strings.Contains(result.Evidence[0].Excerpt, stdout) {
		t.Fatal("structured Go test output was not retained as exact evidence")
	}
}
