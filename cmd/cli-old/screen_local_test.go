package main

import (
	"strings"
	"testing"
)

func TestRetiredScreenReadInferenceFlagsFailExplicitly(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"vision":             {args: []string{"--vision"}, want: "--vision"},
		"single hyphen":      {args: []string{"-vision=true"}, want: "--vision"},
		"prompt":             {args: []string{"--prompt", "toolbar"}, want: "--prompt"},
		"model":              {args: []string{"--model=retired-model"}, want: "--model"},
		"base URL":           {args: []string{"--base-url", "http://127.0.0.1:11434"}, want: "--base-url"},
		"after current flag": {args: []string{"--keep", "--vision"}, want: "--vision"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := retiredScreenReadFlag(test.args); got != test.want {
				t.Fatalf("retiredScreenReadFlag(%q) = %q, want %q", test.args, got, test.want)
			}
		})
	}
}

func TestCurrentScreenReadArgumentsDoNotTriggerRetiredFlagFailure(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--ocr"},
		{"--keep", "--json"},
		{"vision"},
	} {
		if got := retiredScreenReadFlag(args); got != "" {
			t.Fatalf("retiredScreenReadFlag(%q) = %q", args, got)
		}
	}
}

func TestScreenReadRejectsDisabledOCRBeforeAccessingHost(t *testing.T) {
	if _, err := screenReadReport(false, false); err == nil || !strings.Contains(err.Error(), "requires OCR") {
		t.Fatalf("screenReadReport accepted disabled OCR: %v", err)
	}
}

func TestScreenReadTextContainsOnlyDeterministicCaptureAndOCR(t *testing.T) {
	text := screenReadToText(screenReadResult{
		GeneratedAt: "2026-08-28T12:00:00Z",
		CaptureTool: "grim",
		ImagePath:   "/tmp/screen.png",
		OCRText:     "Visible text",
	})
	for _, expected := range []string{"capture_tool=grim", "image_path=/tmp/screen.png", "ocr_text:", "Visible text"} {
		if !strings.Contains(text, expected) {
			t.Errorf("screen read text omitted %q: %s", expected, text)
		}
	}
	if strings.Contains(strings.ToLower(text), "vision") {
		t.Fatalf("screen read text retained vision output: %s", text)
	}
}
