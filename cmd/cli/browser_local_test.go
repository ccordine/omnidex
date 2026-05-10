package main

import (
	"reflect"
	"testing"
)

func TestParseBrowserScanIntent(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectOK    bool
		expectCon   bool
		expectEmail bool
		expectSec   int
		expectLimit int
	}{
		{
			name:        "tabs request",
			input:       "show my open tabs",
			expectOK:    true,
			expectCon:   false,
			expectEmail: false,
			expectSec:   2,
			expectLimit: 50,
		},
		{
			name:        "console request",
			input:       "read the javascript console from my browser",
			expectOK:    true,
			expectCon:   true,
			expectEmail: false,
			expectSec:   3,
			expectLimit: 80,
		},
		{
			name:        "console request with duration",
			input:       "watch console logs for 7 seconds",
			expectOK:    true,
			expectCon:   true,
			expectEmail: false,
			expectSec:   7,
			expectLimit: 80,
		},
		{
			name:        "email watch request",
			input:       "check my browser tabs for my email and tell me what has just come in",
			expectOK:    true,
			expectCon:   false,
			expectEmail: true,
			expectSec:   0,
			expectLimit: 40,
		},
		{
			name:        "non local browser query",
			input:       "what browser should i use for privacy",
			expectOK:    false,
			expectCon:   false,
			expectEmail: false,
			expectSec:   0,
			expectLimit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, ok := parseBrowserScanIntent(tt.input)
			if ok != tt.expectOK {
				t.Fatalf("parseBrowserScanIntent(%q) ok=%v want %v", tt.input, ok, tt.expectOK)
			}
			if !ok {
				return
			}
			if intent.WithConsole != tt.expectCon {
				t.Fatalf("parseBrowserScanIntent(%q) WithConsole=%v want %v", tt.input, intent.WithConsole, tt.expectCon)
			}
			if intent.EmailWatch != tt.expectEmail {
				t.Fatalf("parseBrowserScanIntent(%q) EmailWatch=%v want %v", tt.input, intent.EmailWatch, tt.expectEmail)
			}
			if intent.Seconds != tt.expectSec {
				t.Fatalf("parseBrowserScanIntent(%q) Seconds=%d want %d", tt.input, intent.Seconds, tt.expectSec)
			}
			if intent.Limit != tt.expectLimit {
				t.Fatalf("parseBrowserScanIntent(%q) Limit=%d want %d", tt.input, intent.Limit, tt.expectLimit)
			}
		})
	}
}

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
