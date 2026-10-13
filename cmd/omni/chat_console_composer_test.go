package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/term"
)

func TestLiveActivityKeepsInputAtStartOfSeparateTerminalRow(t *testing.T) {
	console, _ := planReviewTestConsole(t)
	if err := console.SetPrompt("you #6> "); err != nil {
		t.Fatal(err)
	}
	if err := console.SetActivity(chatActivity{JobID: 6, StepID: 20, Label: "Planning", Working: true, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := console.SetActivityDetail(6, 20, strings.Repeat("actual candidate ", 20)); err != nil {
		t.Fatal(err)
	}
	prompt := []rune(console.renderedPrompt)
	index := strings.Index(string(prompt), "you #6> ")
	if index < 0 || len([]rune(string(prompt)[:index])) != 80 {
		t.Fatalf("live activity shares the input row: %q", console.renderedPrompt)
	}
}

type composerTestReader struct {
	source io.Reader
	reads  chan struct{}
}

func (reader composerTestReader) Read(buffer []byte) (int, error) {
	reader.reads <- struct{}{}
	return reader.source.Read(buffer)
}

func TestTypingAndCursorEditingSurviveActivityAndStreamingOutput(t *testing.T) {
	console, output := planReviewTestConsole(t)
	source, input := io.Pipe()
	defer input.Close()
	defer source.Close()
	reads := make(chan struct{}, 1)
	console.terminal = term.NewTerminal(terminalReadWriter{reader: composerTestReader{source, reads}, writer: output}, "you> ")
	done := make(chan string, 1)
	go func() {
		line, err := console.terminal.ReadLine()
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		done <- line
	}()
	<-reads
	if _, err := input.Write([]byte("mid")); err != nil {
		t.Fatal(err)
	}
	<-reads
	if err := console.SetPrompt("you #6> "); err != nil {
		t.Fatal(err)
	}
	if err := console.SetActivity(chatActivity{JobID: 6, Label: "Planning", Working: true}); err != nil {
		t.Fatal(err)
	}
	if err := console.WriteOutput("check> actual model options\n"); err != nil {
		t.Fatal(err)
	}
	if err := console.PulseActivity(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Write([]byte("\x1b[DX\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case actual := <-done:
		if actual != "miXd" {
			t.Fatalf("streaming moved input cursor or changed text: %q", actual)
		}
	case <-time.After(time.Second):
		t.Fatal("live composer stopped accepting input")
	}
}
