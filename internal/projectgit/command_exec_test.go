package projectgit

import (
	"strings"
	"testing"
)

func TestBoundedCommandBufferRejectsOneByteOverLimit(t *testing.T) {
	buffer := &boundedCommandBuffer{limit: 4}
	if written, err := buffer.Write([]byte("1234")); err != nil || written != 4 || buffer.String() != "1234" {
		t.Fatalf("exact-bound write=(%d,%v) value=%q", written, err, buffer.String())
	}
	if written, err := buffer.Write([]byte("5")); err == nil || written != 0 || !buffer.Exceeded() {
		t.Fatalf("over-bound write=(%d,%v) exceeded=%v", written, err, buffer.Exceeded())
	}
}

func TestCommandErrorPresentationIsBounded(t *testing.T) {
	message := strings.Repeat("x", maxGitCommandErrorBytes+1)
	if len(message) <= maxGitCommandErrorBytes {
		t.Fatal("invalid test fixture")
	}
	output := message[:maxGitCommandErrorBytes] + "…"
	err := &commandError{ExitCode: 1, Output: output, Cause: assertCommandFailure{}}
	if got := err.Error(); got != output {
		t.Fatalf("error=%q", got)
	}
}

type assertCommandFailure struct{}

func (assertCommandFailure) Error() string { return "command failed" }
