package main

import "testing"

func TestParsePactlInfoValue(t *testing.T) {
	info := `Server String: /run/user/1000/pulse/native
Default Sink: alsa_output.pci-0000_00_1f.3.analog-stereo
Default Source: alsa_input.usb-Blue_Microphones_Yeti-00.analog-stereo`

	if got := parsePactlInfoValue(info, "Default Sink"); got != "alsa_output.pci-0000_00_1f.3.analog-stereo" {
		t.Fatalf("parsePactlInfoValue sink got=%q", got)
	}
	if got := parsePactlInfoValue(info, "Default Source"); got != "alsa_input.usb-Blue_Microphones_Yeti-00.analog-stereo" {
		t.Fatalf("parsePactlInfoValue source got=%q", got)
	}
}

func TestSortAndSearchTranscriptSegments(t *testing.T) {
	segments := []transcriptSegment{
		{Source: "speaker", Start: "00:00:05,000", End: "00:00:06,000", Text: "Let us start with action items"},
		{Source: "mic", Start: "00:00:02,000", End: "00:00:03,000", Text: "I agree with that plan"},
		{Source: "speaker", Start: "00:00:08,000", End: "00:00:09,000", Text: "We should track every deadline"},
	}
	sortTranscriptSegments(segments)

	if segments[0].Source != "mic" {
		t.Fatalf("expected first segment to be mic after sort, got %s", segments[0].Source)
	}

	matches := searchTranscriptSegments(segments, "track deadline", 10)
	if len(matches) != 1 {
		t.Fatalf("expected one match, got %d", len(matches))
	}
	if segments[matches[0]].Text != "We should track every deadline" {
		t.Fatalf("unexpected search match text: %s", segments[matches[0]].Text)
	}
}
