package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateManagedCLIEnvironmentFileRequiresExactCoreURLAuthority(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "valid", raw: "CORE_URL=https://omni.example\nDOCKER_CONTEXT=default\n"},
		{name: "missing core URL", raw: "DOCKER_CONTEXT=default\n", want: "does not define CORE_URL"},
		{name: "blank core URL", raw: "CORE_URL=\nDOCKER_CONTEXT=default\n", want: "CORE_URL is required"},
		{name: "malformed core URL", raw: "CORE_URL=omni.example\nDOCKER_CONTEXT=default\n", want: "absolute http or https"},
		{name: "duplicate core URL", raw: "CORE_URL=https://one.example\nCORE_URL=https://two.example\nDOCKER_CONTEXT=default\n", want: "defined more than once"},
		{name: "missing Docker context", raw: "CORE_URL=https://omni.example\n", want: "does not define DOCKER_CONTEXT"},
		{name: "rootless Docker context", raw: "CORE_URL=https://omni.example\nDOCKER_CONTEXT=rootless\n", want: "rootless Docker is unsupported"},
		{name: "legacy socket selector", raw: "CORE_URL=https://omni.example\nDOCKER_CONTEXT=default\nDOCKER_SOCKET_PATH=/tmp/docker.sock\n", want: "DOCKER_SOCKET_PATH"},
		{name: "ambient Docker host", raw: "CORE_URL=https://omni.example\nDOCKER_CONTEXT=default\nDOCKER_HOST=unix:///run/user/1000/docker.sock\n", want: "DOCKER_HOST"},
		{name: "ambient Buildx selector", raw: "CORE_URL=https://omni.example\nDOCKER_CONTEXT=default\nBUILDX_BUILDER=alternate\n", want: "BUILDX_BUILDER"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(test.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			err := validateManagedCLIEnvironmentFile(path)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
