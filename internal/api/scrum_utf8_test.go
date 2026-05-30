package api

import (
	"strings"
	"testing"
	"unicode/utf8"
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

func TestApiScrumCardToPatchSanitizesConsoleLog(t *testing.T) {
	patch := apiScrumCardToPatch(ScrumCard{
		Title:      "t",
		ConsoleLog: "log\x00tail",
	})
	raw, ok := patch["console_log"].(string)
	if !ok {
		t.Fatalf("console_log type=%T", patch["console_log"])
	}
	if !utf8.ValidString(raw) {
		t.Fatalf("console_log not valid utf8: %q", raw)
	}
}
