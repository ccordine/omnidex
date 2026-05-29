package omni

import (
	"bytes"
	"runtime"
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

func TestOmniHostServiceUnknownAction(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd actions require Linux")
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)

	err := app.Run([]string{"host", "service", "deploy"})
	if err == nil {
		t.Fatal("expected error for unknown service action")
	}
	if !strings.Contains(err.Error(), "unknown service action") {
		t.Fatalf("unexpected error: %v", err)
	}
}
