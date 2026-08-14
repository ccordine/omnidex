package queue

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestScrumCardItemMutationRejectsNoncanonicalInternalAuthority(t *testing.T) {
	t.Parallel()
	for _, mutation := range []ScrumCardItemMutation{
		{ProjectID: 1, CardID: " card", ExpectedUpdatedAt: time.Now(), Collection: ScrumCardChecklist, Action: ScrumCardItemRemove, ItemID: "item"},
		{ProjectID: 1, CardID: "card", ExpectedUpdatedAt: time.Now(), Collection: ScrumCardChecklist, Action: ScrumCardItemRemove, ItemID: " item"},
		{ProjectID: 1, CardID: "card", ExpectedUpdatedAt: time.Now(), Collection: ScrumCardChecklist, Action: ScrumCardItemAdd, Text: " text "},
	} {
		if err := mutation.ValidateForTransport(); err == nil || !strings.Contains(err.Error(), "canonical") {
			t.Fatalf("mutation=%+v error=%v", mutation, err)
		}
	}
	if _, err := decodeCanonicalScrumCardItems([]byte(`[{"id":" item","text":"text","done":false}]`)); err == nil || !strings.Contains(err.Error(), "noncanonical") {
		t.Fatalf("durable item error=%v", err)
	}
}

func TestScrumCardItemNotFoundIsTyped(t *testing.T) {
	t.Parallel()
	items := []ScrumCardItem{{ID: "present", Text: "Present"}}
	_, err := applyScrumCardItemMutation(items, ScrumCardItemMutation{
		Action: ScrumCardItemRemove, ItemID: "missing",
	})
	if !errors.Is(err, ErrScrumCardItemNotFound) {
		t.Fatalf("error=%v", err)
	}
}
