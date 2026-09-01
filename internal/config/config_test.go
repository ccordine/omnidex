package config

import (
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestLoadCodingScopeMode(t *testing.T) {
	original, configured := os.LookupEnv("OMNI_CODING_SCOPE_MODE")
	if err := os.Unsetenv("OMNI_CODING_SCOPE_MODE"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if configured {
			_ = os.Setenv("OMNI_CODING_SCOPE_MODE", original)
			return
		}
		_ = os.Unsetenv("OMNI_CODING_SCOPE_MODE")
	})

	if got := Load().CodingScopeMode; got != model.CodingScopeModeNormal {
		t.Fatalf("default coding scope mode=%q, want %q", got, model.CodingScopeModeNormal)
	}

	if err := os.Setenv("OMNI_CODING_SCOPE_MODE", string(model.CodingScopeModeExpansive)); err != nil {
		t.Fatal(err)
	}
	if got := Load().CodingScopeMode; got != model.CodingScopeModeExpansive {
		t.Fatalf("configured coding scope mode=%q, want %q", got, model.CodingScopeModeExpansive)
	}
}
