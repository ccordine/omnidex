package main

import "testing"

func TestMatchChatCapabilityKindBySemantics(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		localMedia bool
		localBrows bool
		localScrn  bool
		localShell bool
		localAudio bool
		want       string
	}{
		{
			name:       "screen request without exact trigger phrase",
			input:      "Could you capture the display and summarize what UI elements are visible?",
			localMedia: true, localBrows: true, localScrn: true, localShell: true, localAudio: true,
			want: "local_screen",
		},
		{
			name:       "browser request without exact trigger phrase",
			input:      "Inspect my browser tabs and watch JavaScript errors for a few seconds.",
			localMedia: true, localBrows: true, localScrn: true, localShell: true, localAudio: true,
			want: "local_browser",
		},
		{
			name:       "media request without exact trigger phrase",
			input:      "Resume playback in VLC and move to whatever episode follows this one.",
			localMedia: true, localBrows: true, localScrn: true, localShell: true, localAudio: true,
			want: "local_media",
		},
		{
			name:       "shell request without exact trigger phrase",
			input:      "Tell me the current directory path and my local IP details.",
			localMedia: true, localBrows: true, localScrn: true, localShell: true, localAudio: true,
			want: "local_shell",
		},
		{
			name:       "audio request without exact trigger phrase",
			input:      "Begin transcribing my microphone for meeting notes.",
			localMedia: true, localBrows: true, localScrn: true, localShell: true, localAudio: true,
			want: "local_audio",
		},
		{
			name:       "general chat falls back",
			input:      "Hello, how is your day going?",
			localMedia: true, localBrows: true, localScrn: true, localShell: true, localAudio: true,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchChatCapabilityKind(tc.input, tc.localMedia, tc.localBrows, tc.localScrn, tc.localShell, tc.localAudio)
			if got != tc.want {
				t.Fatalf("matchChatCapabilityKind(%q)=%q want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBuildChatActionCandidateUsesCapabilityFallbackSummary(t *testing.T) {
	candidate := buildChatActionCandidate(
		"Inspect my browser tabs for active pages",
		true,
		true,
		true,
		true,
		true,
		&localShellState{},
	)
	if candidate == nil {
		t.Fatal("expected candidate")
	}
	if candidate.Kind != "local_browser" {
		t.Fatalf("kind=%q want local_browser", candidate.Kind)
	}
	if candidate.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
}
