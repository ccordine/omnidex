package queue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateStoredScrumCardRejectsSilentCanonicalization(t *testing.T) {
	revision := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	valid := DBScrumCard{
		ID: "stored-card", ProjectID: 7, Title: "Stored", Column: "assigned",
		Checklist: json.RawMessage(`[]`), RefFiles: json.RawMessage(`[]`),
		Tags: json.RawMessage(`[]`), TestCriteria: json.RawMessage(`[]`),
		FlowMetrics: json.RawMessage(`{}`), CreatedAt: revision, UpdatedAt: revision,
	}
	if err := validateStoredScrumCard(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*DBScrumCard){
		"padded column": func(card *DBScrumCard) { card.Column = " assigned" },
		"padded job":    func(card *DBScrumCard) { card.JobID = " 17" },
		"aliased job":   func(card *DBScrumCard) { card.JobID = "017" },
		"invalid text":  func(card *DBScrumCard) { card.Description = string([]byte{0xff}) },
		"NUL text":      func(card *DBScrumCard) { card.CardTicket = "bad\x00ticket" },
		"object tags":   func(card *DBScrumCard) { card.Tags = json.RawMessage(`{}`) },
		"negative order": func(card *DBScrumCard) {
			card.BoardOrder = -1
		},
	} {
		t.Run(name, func(t *testing.T) {
			card := valid
			mutate(&card)
			if err := validateStoredScrumCard(card); err == nil {
				t.Fatal("invalid stored card was accepted")
			}
		})
	}
}

func TestScrumCardScannersContainNoTextSanitizerFallback(t *testing.T) {
	for _, source := range []string{
		mustReadScrumSource(t, "scrum_card_scan.go"),
		mustReadScrumSource(t, "scrum_card_pagination.go"),
	} {
		for _, forbidden := range []string{"SanitizeUTF8", "sanitizeScrumCardFields"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("canonical Scrum card read retains sanitizer fallback %q", forbidden)
			}
		}
	}
}

func mustReadScrumSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
