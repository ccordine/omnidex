package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/media"
)

func TestParseNextEpisodeIntent(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expectOK   bool
		expectHint string
	}{
		{
			name:       "matches with show name",
			input:      "play the next episode of Star Trek",
			expectOK:   true,
			expectHint: "Star Trek",
		},
		{
			name:       "matches without show name",
			input:      "play next episode",
			expectOK:   true,
			expectHint: "",
		},
		{
			name:       "does not match unrelated prompt",
			input:      "hello there",
			expectOK:   false,
			expectHint: "",
		},
		{
			name:       "requires play verb",
			input:      "what is the next episode of star trek",
			expectOK:   false,
			expectHint: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, ok := parseNextEpisodeIntent(tt.input)
			if ok != tt.expectOK {
				t.Fatalf("parseNextEpisodeIntent(%q) ok=%v want %v", tt.input, ok, tt.expectOK)
			}
			if intent.ShowHint != tt.expectHint {
				t.Fatalf("parseNextEpisodeIntent(%q) hint=%q want %q", tt.input, intent.ShowHint, tt.expectHint)
			}
		})
	}
}

func TestNaturalLess(t *testing.T) {
	values := []string{
		"/shows/star-trek-s1e10.mkv",
		"/shows/star-trek-s1e2.mkv",
		"/shows/star-trek-s1e1.mkv",
	}
	want := []string{
		"/shows/star-trek-s1e1.mkv",
		"/shows/star-trek-s1e2.mkv",
		"/shows/star-trek-s1e10.mkv",
	}

	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if naturalLess(values[j], values[i]) {
				values[i], values[j] = values[j], values[i]
			}
		}
	}

	if !reflect.DeepEqual(values, want) {
		t.Fatalf("natural sort mismatch\ngot:  %v\nwant: %v", values, want)
	}
}

func TestPickNextPath(t *testing.T) {
	base := filepath.Clean("/media/star-trek")
	paths := []string{
		filepath.Join(base, "S01E01.mkv"),
		filepath.Join(base, "S01E02.mkv"),
		filepath.Join(base, "S01E03.mkv"),
	}

	next, err := pickNextPath(paths, filepath.Join(base, "S01E02.mkv"))
	if err != nil {
		t.Fatalf("pickNextPath returned error: %v", err)
	}
	want := filepath.Join(base, "S01E03.mkv")
	if next != want {
		t.Fatalf("pickNextPath returned %q want %q", next, want)
	}
}

func TestFilterCandidatesByShowHint(t *testing.T) {
	paths := []string{
		"/media/Star Trek/S01E01.mkv",
		"/media/Star Trek/S01E02.mkv",
		"/media/Another Show/S01E01.mkv",
	}
	filtered := filterCandidatesByShowHint(paths, "star trek")
	want := []string{
		"/media/Star Trek/S01E01.mkv",
		"/media/Star Trek/S01E02.mkv",
	}
	if !reflect.DeepEqual(filtered, want) {
		t.Fatalf("filterCandidatesByShowHint mismatch\ngot:  %v\nwant: %v", filtered, want)
	}
}

func TestParsePlaybackContextIntent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expectOK bool
		expectQ  string
	}{
		{
			name:     "just happened in show",
			input:    "what just happened in the show?",
			expectOK: true,
			expectQ:  "",
		},
		{
			name:     "query extraction",
			input:    "what did they just say about warp core breach?",
			expectOK: true,
			expectQ:  "warp core breach",
		},
		{
			name:     "non media question",
			input:    "what just happened in the news?",
			expectOK: false,
			expectQ:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, ok := parsePlaybackContextIntent(tt.input)
			if ok != tt.expectOK {
				t.Fatalf("parsePlaybackContextIntent(%q) ok=%v want %v", tt.input, ok, tt.expectOK)
			}
			if intent.Query != tt.expectQ {
				t.Fatalf("parsePlaybackContextIntent(%q) query=%q want %q", tt.input, intent.Query, tt.expectQ)
			}
		})
	}
}

func TestParsePlaybackControlIntent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expectOK bool
		action   string
	}{
		{
			name:     "play vlc",
			input:    "Could you play my VLC?",
			expectOK: true,
			action:   "play",
		},
		{
			name:     "resume paused playback",
			input:    "Can you press play on it, I have it paused",
			expectOK: true,
			action:   "play",
		},
		{
			name:     "pause vlc",
			input:    "pause vlc",
			expectOK: true,
			action:   "pause",
		},
		{
			name:     "status query",
			input:    "is vlc playing right now?",
			expectOK: true,
			action:   "status",
		},
		{
			name:     "whats currently playing query",
			input:    "Can you tell me what's currently playing on VLC right now?",
			expectOK: true,
			action:   "status",
		},
		{
			name:     "what is playing query",
			input:    "what is playing on vlc",
			expectOK: true,
			action:   "status",
		},
		{
			name:     "next episode should not match control",
			input:    "play the next episode of star trek",
			expectOK: false,
			action:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, ok := parsePlaybackControlIntent(tt.input)
			if ok != tt.expectOK {
				t.Fatalf("parsePlaybackControlIntent(%q) ok=%v want %v", tt.input, ok, tt.expectOK)
			}
			if intent.Action != tt.action {
				t.Fatalf("parsePlaybackControlIntent(%q) action=%q want %q", tt.input, intent.Action, tt.action)
			}
		})
	}
}

func TestSubtitleIndexSelection(t *testing.T) {
	lines := []media.SubtitleLine{
		{LineNumber: 1, Start: "00:00:01,000", End: "00:00:02,000", Text: "Captain on bridge"},
		{LineNumber: 2, Start: "00:00:03,000", End: "00:00:04,000", Text: "Set course for Vulcan"},
		{LineNumber: 3, Start: "00:00:05,000", End: "00:00:06,000", Text: "Engage warp drive"},
	}

	idx, mode, _ := selectSubtitleAnchorLine(lines, 5200, "")
	if idx != 2 || mode != "exact_timestamp" {
		t.Fatalf("selectSubtitleAnchorLine exact mismatch idx=%d mode=%s", idx, mode)
	}

	idx, mode, _ = selectSubtitleAnchorLine(lines, 5500, "warp drive")
	if idx != 2 || mode != "query_near_timestamp" {
		t.Fatalf("selectSubtitleAnchorLine query mismatch idx=%d mode=%s", idx, mode)
	}
}
