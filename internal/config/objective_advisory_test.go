package config

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

func TestLoadObjectiveAdvisoryModeIsExplicitAndOffByDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")

	for raw, want := range map[string]objectiveadvisory.Mode{
		"":       objectiveadvisory.ModeOff,
		"off":    objectiveadvisory.ModeOff,
		"shadow": objectiveadvisory.ModeShadow,
		"active": objectiveadvisory.ModeActive,
	} {
		t.Run(string(want)+"_from_"+raw, func(t *testing.T) {
			t.Setenv("OMNI_OBJECTIVE_ADVISORY_MODE", raw)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ObjectiveAdvisoryMode != want {
				t.Fatalf("mode=%q want %q", cfg.ObjectiveAdvisoryMode, want)
			}
		})
	}
}

func TestLoadRejectsUnknownObjectiveAdvisoryMode(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("OMNI_OBJECTIVE_ADVISORY_MODE", "enabled")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "objective advisory mode") {
		t.Fatalf("Load() error=%v, want explicit advisory mode rejection", err)
	}
}
