package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/envfile"
)

func TestResolveCoreURLUsesExplicitProcessAuthority(t *testing.T) {
	got, err := resolveCoreURL("https://explicit.example", true, "/missing/bin/agent-cli", func(string) (map[string]string, error) {
		t.Fatal("managed config must not be read when CORE_URL is explicit")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://explicit.example" {
		t.Fatalf("core URL = %q", got)
	}
}

func TestResolveCoreURLRejectsExplicitBlankAuthorityWithoutManagedFallback(t *testing.T) {
	_, err := resolveCoreURL("", true, "/missing/bin/agent-cli", func(string) (map[string]string, error) {
		t.Fatal("managed config must not replace an explicitly present CORE_URL")
		return nil, nil
	})
	if !errors.Is(err, errCoreURLRequired) {
		t.Fatalf("error = %v, want CORE_URL required", err)
	}
}

func TestResolveCoreURLReadsManagedInstallEnvironment(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "agent-cli")
	writeCoreURLTestExecutable(t, executable)
	got, err := resolveCoreURL("", false, executable, func(path string) (map[string]string, error) {
		if path != filepath.Join(root, ".env") {
			t.Fatalf("managed config path = %q", path)
		}
		return map[string]string{"OTHER": "value", "CORE_URL": "https://omni.example"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://omni.example" {
		t.Fatalf("core URL = %q", got)
	}
}

func TestResolveCoreURLFailsWithoutOneManagedAuthority(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		err  error
		want string
	}{
		{name: "missing file", err: os.ErrNotExist, want: "managed CORE_URL is unavailable"},
		{name: "missing key", raw: "OTHER=value\n", want: "does not define CORE_URL"},
		{name: "duplicate key", raw: "CORE_URL=https://one.example\nCORE_URL=https://two.example\n", want: "defined more than once"},
		{name: "invalid URL", raw: "CORE_URL=omni.example\n", want: "absolute http or https"},
		{name: "credentials", raw: "CORE_URL=https://user:pass@omni.example\n", want: "must not contain credentials"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "bin", "agent-cli")
			writeCoreURLTestExecutable(t, executable)
			_, err := resolveCoreURL("", false, executable, func(string) (map[string]string, error) {
				if test.err != nil {
					return nil, test.err
				}
				values, parseErr := envfile.Parse([]byte(test.raw))
				if parseErr != nil {
					return nil, fmt.Errorf("parse managed environment: %w", parseErr)
				}
				return values, nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want fragment %q", err, test.want)
			}
		})
	}
}

func TestResolveCoreURLUsesTheExactSharedEnvironmentParser(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	executable := filepath.Join(root, "bin", "agent-cli")
	writeCoreURLTestExecutable(t, executable)
	if err := os.WriteFile(path, []byte(" export CORE_URL=https://omni.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveCoreURL("", false, executable, readManagedEnvironment)
	if err == nil || !strings.Contains(err.Error(), "uppercase key") {
		t.Fatalf("error = %v", err)
	}
	if !errors.Is(validateCoreURL(""), errCoreURLRequired) {
		t.Fatal("blank core URL must retain its typed validation error")
	}
}

func TestManagedEnvironmentPathFollowsExecutableSymlinkToInstallAuthority(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "install", "bin", "agent-cli")
	writeCoreURLTestExecutable(t, executable)
	link := filepath.Join(root, "system-bin", "agent-cli")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	got, err := managedEnvironmentPath(link)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "install", ".env"); got != want {
		t.Fatalf("managed environment path = %q, want %q", got, want)
	}
}

func writeCoreURLTestExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test executable\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}
