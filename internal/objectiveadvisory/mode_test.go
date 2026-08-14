package objectiveadvisory

import "testing"

func TestParseModeAcceptsOnlyExplicitRegisteredModes(t *testing.T) {
	for raw, want := range map[string]Mode{
		"":       ModeOff,
		"off":    ModeOff,
		"shadow": ModeShadow,
		"active": ModeActive,
	} {
		got, err := ParseMode(raw)
		if err != nil || got != want {
			t.Fatalf("ParseMode(%q)=%q error=%v want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"on", "enabled", "reasoning", "ACTIVE", " active "} {
		if _, err := ParseMode(raw); err == nil {
			t.Fatalf("ParseMode(%q) accepted an implicit or non-exact mode", raw)
		}
	}
}
