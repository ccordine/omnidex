package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSubtitleLinesSRT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.srt")
	content := "1\n00:00:01,000 --> 00:00:02,000\nHello there.\n\n2\n00:00:03,000 --> 00:00:04,000\nGeneral Kenobi.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	lines, err := ParseSubtitleLines(path)
	if err != nil {
		t.Fatalf("ParseSubtitleLines failed: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].Start != "00:00:01,000" || lines[0].End != "00:00:02,000" {
		t.Fatalf("unexpected timing: %+v", lines[0])
	}
	if lines[1].Text != "General Kenobi." {
		t.Fatalf("unexpected line text: %+v", lines[1])
	}
}

func TestParseSubtitleLinesVTT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.vtt")
	content := "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nA line\n\n00:00:03.000 --> 00:00:04.000 align:start\nAnother line\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	lines, err := ParseSubtitleLines(path)
	if err != nil {
		t.Fatalf("ParseSubtitleLines failed: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[1].End != "00:00:04.000" {
		t.Fatalf("unexpected end timing: %+v", lines[1])
	}
}

func TestDiscoverEpisodesAndSearch(t *testing.T) {
	dir := t.TempDir()
	showDir := filepath.Join(dir, "Star Trek")
	if err := os.MkdirAll(showDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	video := filepath.Join(showDir, "Star.Trek.S01E01.mkv")
	sub := filepath.Join(showDir, "Star.Trek.S01E01.en.srt")
	if err := os.WriteFile(video, []byte{}, 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	srt := "1\n00:00:01,000 --> 00:00:02,000\nCaptain on bridge.\n\n2\n00:00:03,000 --> 00:00:04,000\nWarp drive engaged.\n"
	if err := os.WriteFile(sub, []byte(srt), 0o644); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}

	episodes, err := DiscoverEpisodes(dir, 0)
	if err != nil {
		t.Fatalf("DiscoverEpisodes failed: %v", err)
	}
	if len(episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(episodes))
	}
	if episodes[0].SubtitlePath == "" {
		t.Fatalf("expected subtitle path to be matched")
	}
	if episodes[0].Season != 1 || episodes[0].Episode != 1 {
		t.Fatalf("unexpected season/episode: %+v", episodes[0])
	}

	matches, err := SearchSubtitleLines(dir, "warp drive", 1, 10)
	if err != nil {
		t.Fatalf("SearchSubtitleLines failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Line.Text != "Warp drive engaged." {
		t.Fatalf("unexpected match line: %+v", matches[0].Line)
	}
	if len(matches[0].Before) != 1 {
		t.Fatalf("expected one context line before match")
	}
}

func TestChunkSubtitleLines(t *testing.T) {
	lines := []SubtitleLine{
		{LineNumber: 1, Start: "00:00:01,000", End: "00:00:02,000", Text: "one"},
		{LineNumber: 2, Start: "00:00:03,000", End: "00:00:04,000", Text: "two"},
		{LineNumber: 3, Start: "00:00:05,000", End: "00:00:06,000", Text: "three"},
	}
	chunks := ChunkSubtitleLines(lines, 2)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].FromLine != 1 || chunks[0].ToLine != 2 {
		t.Fatalf("unexpected first chunk bounds: %+v", chunks[0])
	}
	if chunks[1].FromLine != 3 || chunks[1].ToLine != 3 {
		t.Fatalf("unexpected second chunk bounds: %+v", chunks[1])
	}
}

func TestTimestampToMillis(t *testing.T) {
	tests := []struct {
		input  string
		wantMS int64
		ok     bool
	}{
		{input: "00:00:01,000", wantMS: 1000, ok: true},
		{input: "00:01:02.500", wantMS: 62500, ok: true},
		{input: "1:02:03.045", wantMS: 3723045, ok: true},
		{input: "bad", wantMS: 0, ok: false},
	}

	for _, tt := range tests {
		got, ok := TimestampToMillis(tt.input)
		if ok != tt.ok {
			t.Fatalf("TimestampToMillis(%q) ok=%v want %v", tt.input, ok, tt.ok)
		}
		if got != tt.wantMS {
			t.Fatalf("TimestampToMillis(%q)=%d want %d", tt.input, got, tt.wantMS)
		}
	}
}
