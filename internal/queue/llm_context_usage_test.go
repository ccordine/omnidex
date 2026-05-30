package queue

import "testing"

func TestLLMContextUtilizationPct(t *testing.T) {
	if got := llmContextUtilizationPct(0, 49152); got != 0 {
		t.Fatalf("zero sent = %#v, want 0", got)
	}
	if got := llmContextUtilizationPct(46000, 49152); got != 93.59 {
		t.Fatalf("46000/49152 = %#v, want 93.59", got)
	}
}

func TestLLMContextOverloaded(t *testing.T) {
	limit := 49152
	if llmContextOverloaded(46000, limit) {
		t.Fatal("46000 chars should not be overloaded at 49152 limit")
	}
	if !llmContextOverloaded(47000, limit) {
		t.Fatal("47000 chars should count as overloaded at 49152 limit")
	}
}
