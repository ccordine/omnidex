package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestAppendScrumChatMessageSanitizesInvalidUTF8(t *testing.T) {
	if _, err := appendScrumChatMessage(nil, "assistant", "ok\x00bad\xff"); err == nil {
		t.Fatal("invalid UTF-8 or NUL channel content must fail loudly")
	}
}

func TestDBScrumCardToAPIRejectsCorruptTypedJSON(t *testing.T) {
	card := queue.DBScrumCard{
		ID: "card-1", Checklist: json.RawMessage(`{"not":"a list"}`),
		RefFiles: json.RawMessage(`[]`), Tags: json.RawMessage(`[]`),
		TestCriteria: json.RawMessage(`[]`), FlowMetrics: json.RawMessage(`{}`),
	}
	if _, err := dbScrumCardToAPI(card); err == nil {
		t.Fatal("corrupt durable Scrum card JSON must fail loudly")
	}
}

func TestCanonicalScrumRowsHaveNoSanitizingMapWriterFallback(t *testing.T) {
	for _, path := range []string{"scrum_card_conversion.go", "scrum_channel_chat.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"apiScrumCardToPatch", "sanitizeScrumChannelText", "sanitizeScrumChannelBytes"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains silent canonical row rewrite %q", path, forbidden)
			}
		}
	}
}
