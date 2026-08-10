package main

import (
	"os"
	"strings"
	"testing"
)

func TestSeriousRailSummaryDistinguishesLocalGateFromProductPromotion(t *testing.T) {
	summary := gateEvidenceSummary("sealed matrix runs", 72, true, false)
	if !strings.HasPrefix(summary, "gate evidence qualified true; ") ||
		!strings.Contains(summary, "product promotion eligible false; ") ||
		!strings.HasSuffix(summary, "sealed matrix runs 72\n") {
		t.Fatalf("ambiguous serious rail summary %q", summary)
	}
}

func TestEverySeriousRailCommandUsesGateEvidenceSummary(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(raw), "gateEvidenceSummary("); count != 8 {
		t.Fatalf("serious command gate summaries=%d want=8", count)
	}
	for _, forbidden := range []string{
		"sealed matrix runs %d; promotion eligible",
		"sealed Resume schedules %d; promotion eligible",
		"sealed Transfer surfaces %d; promotion eligible",
		"sealed Scale runs %d; promotion eligible",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("serious command retains ambiguous output %q", forbidden)
		}
	}
}
