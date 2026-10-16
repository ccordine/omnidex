package queue

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestCapturedLiveReviewRequiresItsImmutableParserRoute(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/review-acceptance.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Configuration codingagent.Configuration `json:"configuration"`
		Task          codingagent.Task          `json:"task"`
		Entries       []codingagent.Entry       `json:"entries"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	if err := validateCodingTaskAcceptanceJournal(capture.Entries, 1, capture.Configuration, capture.Task); err != nil {
		t.Fatalf("exact real native execution/review/parser receipts were rejected: %v", err)
	}
	for _, route := range []codingagent.Route{{}, {Provider: "codex", Model: "other-parser", Effort: "low", ServiceTier: "default"}} {
		changed := capture.Configuration
		changed.LedgerParser = route
		if err := validateCodingTaskAcceptanceJournal(capture.Entries, 1, changed, capture.Task); err == nil {
			t.Fatal("missing or foreign parser authority accepted real review receipts")
		}
	}
}
