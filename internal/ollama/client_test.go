package ollama

import (
	"errors"
	"strings"
	"testing"
)

func TestWrapConnectivityErrorAddsDockerHint(t *testing.T) {
	client := &Client{baseURL: "http://host.docker.internal:11434"}
	baseErr := errors.New(`Post "http://host.docker.internal:11434/api/generate": dial tcp 172.17.0.1:11434: connect: connection refused`)

	got := client.wrapConnectivityError(baseErr, "/api/generate")
	if got == nil {
		t.Fatalf("expected wrapped error")
	}
	if !strings.Contains(got.Error(), "OLLAMA_HOST=0.0.0.0:11434") {
		t.Fatalf("expected docker/ollama hint in error, got: %v", got)
	}
}

func TestWrapConnectivityErrorNoHintForOtherHosts(t *testing.T) {
	client := &Client{baseURL: "http://localhost:11434"}
	baseErr := errors.New(`dial tcp 127.0.0.1:11434: connect: connection refused`)

	got := client.wrapConnectivityError(baseErr, "/api/generate")
	if got == nil {
		t.Fatalf("expected error")
	}
	if got.Error() != baseErr.Error() {
		t.Fatalf("expected unchanged error, got: %v", got)
	}
}
