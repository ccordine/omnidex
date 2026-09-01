package main

import (
	"reflect"
	"testing"
)

func TestParseDebugPortFromCmdline(t *testing.T) {
	input := "/usr/bin/chromium --remote-debugging-port=9333 --user-data-dir=/tmp/profile"
	if port := parseDebugPortFromCmdline(input); port != 9333 {
		t.Fatalf("parseDebugPortFromCmdline(%q)=%d want 9333", input, port)
	}
}

func TestMergePorts(t *testing.T) {
	got := mergePorts([]int{9222, 9222, 9333}, []int{9229, 9333, 9223})
	want := []int{9222, 9223, 9229, 9333}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergePorts mismatch got=%v want=%v", got, want)
	}
}

func TestBrowserEmailStateKey(t *testing.T) {
	key := browserEmailStateKey("https://mail.google.com/mail/u/0/#inbox", browserEmailItem{
		Sender:   "Alice",
		Subject:  "Build update",
		TimeText: "10:42 AM",
		Key:      "alice|build update|10:42 am",
	})
	if key == "" {
		t.Fatalf("expected non-empty state key")
	}
}

func TestPruneBrowserEmailState(t *testing.T) {
	state := browserEmailState{
		Version: browserEmailStateVersion,
		Seen: map[string]string{
			"a": "2026-01-01T00:00:00Z",
			"b": "2026-01-02T00:00:00Z",
			"c": "2026-01-03T00:00:00Z",
		},
	}
	pruneBrowserEmailState(&state, 2)
	if len(state.Seen) != 2 {
		t.Fatalf("expected pruned state size 2, got %d", len(state.Seen))
	}
	if _, ok := state.Seen["a"]; ok {
		t.Fatalf("expected oldest key to be pruned")
	}
}
