package queue

import "testing"

func TestContextShrinkSavedPct(t *testing.T) {
	if got := contextShrinkSavedPct(1000, 100); got != 90 {
		t.Fatalf("saved pct=%v", got)
	}
	if got := contextShrinkSavedPct(0, 100); got != 0 {
		t.Fatalf("zero raw saved pct=%v", got)
	}
	if got := contextShrinkSavedPct(500, 600); got != 0 {
		t.Fatalf("negative saved pct=%v", got)
	}
}
