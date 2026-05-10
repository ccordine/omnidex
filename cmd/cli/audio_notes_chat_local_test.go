package main

import "testing"

func TestParseLocalAudioNotesIntent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		action string
		query  string
		ok     bool
	}{
		{
			name:   "start notes",
			input:  "take notes during this call until I say stop",
			action: "start",
			ok:     true,
		},
		{
			name:   "stop notes",
			input:  "please stop taking notes now",
			action: "stop",
			ok:     true,
		},
		{
			name:   "status notes",
			input:  "are you taking notes",
			action: "status",
			ok:     true,
		},
		{
			name:   "search notes",
			input:  "search notes for action items and owners",
			action: "search",
			query:  "action items and owners",
			ok:     true,
		},
		{
			name:  "not matched",
			input: "tell me a joke",
			ok:    false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			intent, ok := parseLocalAudioNotesIntent(tc.input)
			if ok != tc.ok {
				t.Fatalf("parseLocalAudioNotesIntent ok=%t want=%t", ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if intent.Action != tc.action {
				t.Fatalf("action=%q want=%q", intent.Action, tc.action)
			}
			if intent.Query != tc.query {
				t.Fatalf("query=%q want=%q", intent.Query, tc.query)
			}
		})
	}
}
