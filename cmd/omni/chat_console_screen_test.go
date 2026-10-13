package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/term"
)

func TestActivityUnicodePreservesPhysicalComposerRowsAndCursor(t *testing.T) {
	for _, detail := range []string{"界", "🙂", "e\u0301"} {
		t.Run(detail, func(t *testing.T) {
			console, screen, input, reads, done := screenComposer(t, 32)
			if err := console.SetActivity(chatActivity{JobID: 6, StepID: 20, Label: "Planning", Working: true}); err != nil {
				t.Fatal(err)
			}
			if err := console.SetActivityDetail(6, 20, detail); err != nil {
				t.Fatal(err)
			}
			rows, row, column, _ := screen.snapshot()
			if rows[0] != "transcript> keep" || rows[2] != "you>" || row != 2 || column != 5 {
				t.Fatalf("Unicode status moved input: rows=%q cursor=(%d,%d)", rows, row, column)
			}
			writeComposerKeys(t, input, reads, "ab\x1b[DX")
			rows, row, column, _ = screen.snapshot()
			if rows[2] != "you> aXb" || row != 2 || column != 7 {
				t.Fatalf("Unicode status broke physical editing: rows=%q cursor=(%d,%d)", rows, row, column)
			}
			finishScreenComposer(t, input, done, "aXb")
		})
	}
}

func TestActivityRemovalClearsAllWrappedComposerRows(t *testing.T) {
	console, screen, input, reads, done := screenComposer(t, 20)
	if err := console.SetActivity(chatActivity{JobID: 6, StepID: 20, Label: "Planning", Working: true}); err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("x", 34) + "TAIL"
	writeComposerKeys(t, input, reads, text+"\x01")
	if err := console.SetActivity(chatActivity{}); err != nil {
		t.Fatal(err)
	}
	rows, row, column, _ := screen.snapshot()
	want := []string{"transcript> keep", "you> " + strings.Repeat("x", 15), strings.Repeat("x", 19) + "T", "AIL"}
	for index, expected := range want {
		if rows[index] != expected {
			t.Fatalf("activity removal changed transcript or retained stale input: rows=%q", rows)
		}
	}
	for _, value := range rows[len(want):] {
		if value != "" {
			t.Fatalf("activity removal left wrapped input below editor: rows=%q", rows)
		}
	}
	if row != 1 || column != 5 {
		t.Fatalf("activity removal lost the editing cursor: (%d,%d)", row, column)
	}
	writeComposerKeys(t, input, reads, "Z")
	finishScreenComposer(t, input, done, "Z"+text)
}

func TestActivityUnicodeTranscriptPreservesOriginalText(t *testing.T) {
	console, screen, input, _, done := screenComposer(t, 32)
	if err := console.SetActivity(chatActivity{JobID: 6, StepID: 20, Label: "Planning", Working: true}); err != nil {
		t.Fatal(err)
	}
	const detail = "界🙂e\u0301"
	if err := console.SetActivityDetail(6, 20, detail); err != nil {
		t.Fatal(err)
	}
	if err := console.WriteOutput("omni> " + detail + "\n"); err != nil {
		t.Fatal(err)
	}
	_, _, _, transcript := screen.snapshot()
	if !strings.Contains(transcript, "omni> "+detail+"\r\n") {
		t.Fatalf("activity projection changed original transcript text: %q", transcript)
	}
	finishScreenComposer(t, input, done, "")
}

func screenComposer(t *testing.T, width int) (*chatConsole, *consoleTestScreen, *io.PipeWriter, chan struct{}, chan string) {
	t.Helper()
	screen := newConsoleTestScreen(width, 12)
	if _, err := screen.Write([]byte("transcript> keep\r\n")); err != nil {
		t.Fatal(err)
	}
	source, input := io.Pipe()
	t.Cleanup(func() { _ = input.Close(); _ = source.Close() })
	reads := make(chan struct{}, 1)
	terminal := term.NewTerminal(terminalReadWriter{reader: composerTestReader{source, reads}, writer: screen}, "you> ")
	if err := terminal.SetSize(width, 12); err != nil {
		t.Fatal(err)
	}
	console := &chatConsole{
		terminal: terminal, output: screen, terminalFD: -1,
		prompt: "you> ", terminalWidth: width, terminalHeight: 12,
	}
	done := make(chan string, 1)
	go func() {
		line, err := terminal.ReadLine()
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		done <- line
	}()
	select {
	case <-reads:
	case <-time.After(time.Second):
		t.Fatal("composer did not begin its input read")
	}
	return console, screen, input, reads, done
}

func writeComposerKeys(t *testing.T, input *io.PipeWriter, reads chan struct{}, keys string) {
	t.Helper()
	if _, err := input.Write([]byte(keys)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reads:
	case <-time.After(time.Second):
		t.Fatal("composer did not finish editing the input chunk")
	}
}

func finishScreenComposer(t *testing.T, input *io.PipeWriter, done chan string, want string) {
	t.Helper()
	if _, err := input.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-done:
		if line != want {
			t.Fatalf("terminal changed input source: got %q, want %q", line, want)
		}
	case <-time.After(time.Second):
		t.Fatal("composer did not submit its edited input")
	}
}
