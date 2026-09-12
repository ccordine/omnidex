package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadNeedsNoCodingScopeSetting(t *testing.T) {
	t.Setenv("OMNI_CODING_SCOPE_MODE", "")
	if err := os.Unsetenv("OMNI_CODING_SCOPE_MODE"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsRemovedScopeSetting(t *testing.T) {
	for _, value := range []string{"strict", "normal", "expansive", ""} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("OMNI_CODING_SCOPE_MODE", value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OMNI_CODING_SCOPE_MODE was removed") {
				t.Fatalf("removed environment setting %q: %v", value, err)
			}
		})
	}
}
