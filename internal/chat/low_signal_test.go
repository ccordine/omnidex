package chat

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestIsLowSignalHello(t *testing.T) {
	if !IsLowSignal("hello", model.PipelineChat) {
		t.Fatal("expected hello to be low signal")
	}
}

func TestIsLowSignalRejectsConcreteRequest(t *testing.T) {
	if IsLowSignal("write a migration for users", model.PipelineChat) {
		t.Fatal("expected concrete request not to be low signal")
	}
}

func TestLowSignalResponseGreets(t *testing.T) {
	got := LowSignalResponse("hello")
	if got == "" || strings.Contains(strings.ToLower(got), "memory retrieval") {
		t.Fatalf("unexpected response: %q", got)
	}
}
