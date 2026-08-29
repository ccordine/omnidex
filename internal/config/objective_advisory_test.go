package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsRetiredObjectiveAdvisoryEnvironment(t *testing.T) {
	for _, key := range []string{"OMNI_OBJECTIVE_ADVISORY_MODE", "OMNI_OBJECTIVE_ADVISORY_MODEL"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(key, "active")

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), key+" was removed") {
				t.Fatalf("Load() error=%v, want explicit retired-setting rejection", err)
			}
		})
	}
}
