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
		{name: "valid", raw: "CORE_URL=https://omni.example\n"},
		{name: "missing", raw: "OTHER=value\n", want: "does not define CORE_URL"},
		{name: "blank", raw: "CORE_URL=\n", want: "CORE_URL is required"},
		{name: "malformed", raw: "CORE_URL=omni.example\n", want: "absolute http or https"},
		{name: "duplicate", raw: "CORE_URL=https://one.example\nCORE_URL=https://two.example\n", want: "defined more than once"},
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
