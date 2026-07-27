package api

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/queue"
)

func TestAppendScrumChatMessageSanitizesInvalidUTF8(t *testing.T) {
	chat := appendScrumChatMessage(nil, "assistant", "ok\x00bad\xff")
	if len(chat) != 1 {
		t.Fatalf("len=%d want 1", len(chat))
	}
	if !utf8.ValidString(chat[0].Content) {
		t.Fatalf("content not valid utf8: %q", chat[0].Content)
	}
	if !strings.Contains(chat[0].Content, "ok") {
		t.Fatalf("content=%q", chat[0].Content)
	}
}

func TestDBScrumCardToAPIRejectsCorruptTypedJSON(t *testing.T) {
	card := queue.DBScrumCard{
		ID:           "card-1",
		Checklist:    json.RawMessage(`{"not":"a list"}`),
		RefFiles:     json.RawMessage(`[]`),
		Chat:         json.RawMessage(`[]`),
		PlanningChat: json.RawMessage(`[]`),
		Tags:         json.RawMessage(`[]`),
		TestCriteria: json.RawMessage(`[]`),
	}
	if _, err := dbScrumCardToAPI(card); err == nil {
		t.Fatal("corrupt durable Scrum card JSON must fail loudly")
	}
}

func TestAPIScrumCardToPatchRejectsCorruptConfigJSON(t *testing.T) {
	if _, err := apiScrumCardToPatch(ScrumCard{Title: "card", ModelConfig: json.RawMessage(`{`)}); err == nil {
		t.Fatal("corrupt Scrum card config JSON must fail loudly")
	}
}

func TestApiScrumCardToPatchSanitizesConsoleLog(t *testing.T) {
	patch, err := apiScrumCardToPatch(ScrumCard{
		Title:      "t",
		ConsoleLog: "log\x00tail",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := patch["console_log"].(string)
	if !ok {
		t.Fatalf("console_log type=%T", patch["console_log"])
	}
	if !utf8.ValidString(raw) {
		t.Fatalf("console_log not valid utf8: %q", raw)
	}
}
