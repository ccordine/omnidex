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
