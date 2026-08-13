package worker

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundedCommandOutputCapsBeforeProjectionAndCountsObservedBytes(t *testing.T) {
	output, err := newBoundedCommandOutput(7)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("x"), 1<<20)
	if written, err := output.Write(payload); err != nil || written != len(payload) {
		t.Fatalf("written=%d err=%v", written, err)
	}
	text, observed, truncated := output.Result()
	if text != strings.Repeat("x", 7) || observed != int64(len(payload)) || !truncated {
		t.Fatalf("text=%q observed=%d truncated=%t", text, observed, truncated)
	}
	if len(output.prefix) != 7 || cap(output.prefix) != 7 {
		t.Fatalf("retained len/cap=%d/%d", len(output.prefix), cap(output.prefix))
	}
}

func TestBoundedCommandOutputNeverReturnsPartialUTF8(t *testing.T) {
	output, err := newBoundedCommandOutput(4)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = output.Write([]byte("abc界"))
	text, observed, truncated := output.Result()
	if text != "abc" || !utf8.ValidString(text) || observed != 6 || !truncated {
		t.Fatalf("text=%q observed=%d truncated=%t", text, observed, truncated)
	}
}
