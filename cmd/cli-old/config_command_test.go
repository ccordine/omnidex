package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigEditorDefaultsToVim(t *testing.T) {
	t.Setenv("OMNI_CONFIG_EDITOR", "")
	t.Setenv("EDITOR", "")
	if got := resolveConfigEditor(""); got != "vim" {
		t.Fatalf("resolveConfigEditor()=%q, want %q", got, "vim")
	}
}

func TestResolveConfigEditorPrefersExplicitAndEnv(t *testing.T) {
	t.Setenv("OMNI_CONFIG_EDITOR", "nano")
	t.Setenv("EDITOR", "vi")
	if got := resolveConfigEditor("code --wait"); got != "code --wait" {
		t.Fatalf("resolveConfigEditor(explicit)=%q, want %q", got, "code --wait")
	}
	if got := resolveConfigEditor(""); got != "nano" {
		t.Fatalf("resolveConfigEditor(env)=%q, want %q", got, "nano")
	}
}

func TestResolveManagedConfigPathPrefersExistingEnv(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("OLLAMA_MODEL=test\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	other := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(other); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	t.Setenv(omniRuntimeDirEnv, root)
	got, err := resolveManagedConfigPath("")
	if err != nil {
		t.Fatalf("resolveManagedConfigPath() error: %v", err)
	}
	if got != path {
		t.Fatalf("resolveManagedConfigPath()=%q, want %q", got, path)
	}
}

func TestResolveManagedConfigPathCreatesFromDefaultTemplate(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "default.env")
	templateContent := "OLLAMA_MODEL=template\n"
	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("write default.env: %v", err)
	}

	other := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(other); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	t.Setenv(omniRuntimeDirEnv, root)
	got, err := resolveManagedConfigPath("")
	if err != nil {
		t.Fatalf("resolveManagedConfigPath() error: %v", err)
	}
	want := filepath.Join(root, ".env")
	if got != want {
		t.Fatalf("resolveManagedConfigPath()=%q, want %q", got, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read created .env: %v", err)
	}
	if string(data) != templateContent {
		t.Fatalf("created .env content mismatch: got=%q want=%q", string(data), templateContent)
	}
}

func TestEditorCommandArgs(t *testing.T) {
	args, err := editorCommandArgs("vim", "/tmp/.env")
	if err != nil {
		t.Fatalf("editorCommandArgs error: %v", err)
	}
	if strings.Join(args, " ") != "vim /tmp/.env" {
		t.Fatalf("unexpected args: %v", args)
	}

	args, err = editorCommandArgs("code --wait", "/tmp/.env")
	if err != nil {
		t.Fatalf("editorCommandArgs error: %v", err)
	}
	if strings.Join(args, " ") != "code --wait /tmp/.env" {
		t.Fatalf("unexpected args: %v", args)
	}
}
