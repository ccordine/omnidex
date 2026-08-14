package queue

import (
	"strings"
	"testing"
)

func TestParseScrumCardColumnRequiresOneCanonicalRegisteredValue(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"backlog", "ready", "assigned", "in_progress", "review", "blocked", "error", "done",
	} {
		column, err := ParseScrumCardColumn(value)
		if err != nil {
			t.Fatalf("ParseScrumCardColumn(%q): %v", value, err)
		}
		if string(column) != value {
			t.Fatalf("ParseScrumCardColumn(%q)=%q", value, column)
		}
	}
	for _, value := range []string{"", " done", "done ", "DONE", "in-progress", "in progress", "triage"} {
		if _, err := ParseScrumCardColumn(value); err == nil {
			t.Errorf("ParseScrumCardColumn(%q) unexpectedly succeeded", value)
		}
	}
}

func TestScrumCardMoveRejectsNoncanonicalAuthorityBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	for _, move := range []ScrumCardMove{
		{ProjectID: 1, CardID: "card", Column: ScrumCardColumn("DONE")},
		{ProjectID: 1, CardID: " card", Column: ScrumCardDone},
		{ProjectID: 1, CardID: "bad\x00card", Column: ScrumCardDone},
		{ProjectID: 1, CardID: "card", Column: ScrumCardDone, BeforeCardID: "before "},
		{ProjectID: 1, CardID: "card", Column: ScrumCardDone, BeforeCardID: "bad\x00before"},
		{ProjectID: 1, CardID: "card", Column: ScrumCardDone, BeforeCardID: strings.Repeat("x", MaxScrumCardIDBytes+1)},
	} {
		if _, _, err := (*Repository)(nil).MoveScrumCard(t.Context(), move); err == nil {
			t.Fatalf("MoveScrumCard(%+v) unexpectedly accepted noncanonical authority", move)
		}
	}
}

func TestScrumCardMoveRequiresObservedServerRevisionBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	move := ScrumCardMove{ProjectID: 1, CardID: "card", Column: ScrumCardDone}
	if _, _, err := (*Repository)(nil).MoveScrumCard(t.Context(), move); err == nil ||
		err.Error() != "Scrum card move requires an expected card revision" {
		t.Fatalf("MoveScrumCard missing revision error=%v", err)
	}
}
