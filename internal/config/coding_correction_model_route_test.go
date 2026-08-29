package config

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestCorrectionOnlyEnvironmentRouteDoesNotChangeInitialCodingFragmentModel(t *testing.T) {
	t.Setenv("OMNI_CODING_FRAGMENT_MODEL", "qwen3.5:9b-q4_K_M")
	t.Setenv("OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL", "deepseek-r1:8b")
	t.Setenv("OMNI_CODING_FRAGMENT_CORRECTION_MODEL", "deepseek-r1:8b")

	routing := loadStationModels(Config{})
	if got := routing[station.CodingFragment]; got != "qwen3.5:9b-q4_K_M" {
		t.Fatalf("initial coding fragment route=%q", got)
	}
	if got := routing[station.CodingFragmentRepairGuidance]; got != "deepseek-r1:8b" {
		t.Fatalf("coding fragment repair guidance route=%q", got)
	}
	if got := routing[station.CodingFragmentCorrection]; got != "deepseek-r1:8b" {
		t.Fatalf("coding fragment correction route=%q", got)
	}
}
