package queue

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeUTF8TextPreservesValid(t *testing.T) {
	in := "play job — café"
	if got := SanitizeUTF8Text(in); got != in {
		t.Fatalf("got %q want %q", got, in)
	}
}

func TestSanitizeUTF8TextReplacesInvalid(t *testing.T) {
	in := "before\x00after\xff\xfe"
	got := SanitizeUTF8Text(in)
	if !utf8.ValidString(got) {
		t.Fatalf("not valid utf8: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("unexpected sanitize result: %q", got)
	}
	if strings.Contains(got, "\x00") {
		t.Fatalf("expected null bytes stripped: %q", got)
	}
}

func TestTruncateUTF8TextDoesNotCutArrowRune(t *testing.T) {
	input := strings.Repeat("a", 12) + "→ done"
	got := TruncateUTF8Text(input, 14, "...")
	if !utf8.ValidString(got) {
		t.Fatalf("not valid utf8: %q", got)
	}
	if strings.Contains(got, string([]byte{0xe2, 0x86, 0x2e})) {
		t.Fatalf("found truncated arrow before suffix: %q", got)
	}
	if got != strings.Repeat("a", 12)+"..." {
		t.Fatalf("truncate=%q", got)
	}
}

func TestTelemetryPromptSummaryKeepsValidUTF8(t *testing.T) {
	instruction := strings.Repeat("a", 240) + "→ route"
	got := telemetryPromptSummary(instruction, 242)
	if !utf8.ValidString(got) {
		t.Fatalf("not valid utf8: %q", got)
	}
	if strings.Contains(got, string([]byte{0xe2, 0x86, 0x2e})) {
		t.Fatalf("found truncated arrow before redaction suffix: %q", got)
	}
}
