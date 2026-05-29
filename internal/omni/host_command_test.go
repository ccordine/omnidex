package omni

import (
	"bytes"
	"strings"
	"testing"
)

func TestOmniHostServeRejectsUnknownArgs(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)

	err := app.Run([]string{"host", "serve", "--listen", "127.0.0.1:0", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected host serve argument(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOmniHostUnknownSubcommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)

	err := app.Run([]string{"host", "nope"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown host subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
