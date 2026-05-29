package hostbridge

import (
	"strings"
	"testing"
)

func TestShellInvocationArgs(t *testing.T) {
	tests := map[string][]string{
		"bash":   {"-il"},
		"zsh":    {"-il"},
		"fish":   {"-i"},
		"nu":     {"-il"},
		"custom": {"-i"},
	}
	for shell, want := range tests {
		got := shellInvocationArgs(shell)
		if len(got) != len(want) {
			t.Fatalf("shellInvocationArgs(%q)=%v want %v", shell, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("shellInvocationArgs(%q)=%v want %v", shell, got, want)
			}
		}
	}
}

func TestTerminalEnvSetsCoreVariables(t *testing.T) {
	env := terminalEnv("/tmp/project", "/bin/bash")
	lookup := map[string]string{}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		lookup[key] = value
	}
	if lookup["SHELL"] != "/bin/bash" {
		t.Fatalf("SHELL=%q", lookup["SHELL"])
	}
	if lookup["PWD"] != "/tmp/project" {
		t.Fatalf("PWD=%q", lookup["PWD"])
	}
	if lookup["TERM"] != "xterm-256color" {
		t.Fatalf("TERM=%q", lookup["TERM"])
	}
	if lookup["OMNI_TERMINAL"] != "1" {
		t.Fatalf("OMNI_TERMINAL=%q", lookup["OMNI_TERMINAL"])
	}
	if lookup["HOME"] == "" {
		t.Fatalf("HOME missing from env")
	}
}
