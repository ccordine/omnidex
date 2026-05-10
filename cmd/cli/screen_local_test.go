package main

import "testing"

func TestParseScreenReadIntent(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectOK  bool
		expectOCR bool
		expectVis bool
	}{
		{
			name:      "read screen text",
			input:     "read my screen text",
			expectOK:  true,
			expectOCR: true,
			expectVis: false,
		},
		{
			name:      "describe screen",
			input:     "what's on my screen right now?",
			expectOK:  true,
			expectOCR: true,
			expectVis: true,
		},
		{
			name:      "vision only",
			input:     "describe my screen vision only",
			expectOK:  true,
			expectOCR: false,
			expectVis: true,
		},
		{
			name:      "unrelated",
			input:     "recommend a monitor for gaming",
			expectOK:  false,
			expectOCR: false,
			expectVis: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, ok := parseScreenReadIntent(tt.input)
			if ok != tt.expectOK {
				t.Fatalf("parseScreenReadIntent(%q) ok=%v want %v", tt.input, ok, tt.expectOK)
			}
			if !ok {
				return
			}
			if intent.WithOCR != tt.expectOCR {
				t.Fatalf("parseScreenReadIntent(%q) WithOCR=%v want %v", tt.input, intent.WithOCR, tt.expectOCR)
			}
			if intent.WithVision != tt.expectVis {
				t.Fatalf("parseScreenReadIntent(%q) WithVision=%v want %v", tt.input, intent.WithVision, tt.expectVis)
			}
		})
	}
}

func TestScreenGenerateEndpoint(t *testing.T) {
	if got := screenGenerateEndpoint("http://localhost:11434"); got != "http://localhost:11434/api/generate" {
		t.Fatalf("screenGenerateEndpoint mismatch got=%q", got)
	}
	if got := screenGenerateEndpoint("http://localhost:11434/api/generate"); got != "http://localhost:11434/api/generate" {
		t.Fatalf("screenGenerateEndpoint passthrough mismatch got=%q", got)
	}
}
