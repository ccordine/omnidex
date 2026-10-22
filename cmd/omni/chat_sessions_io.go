package main

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/term"
)

// A session retains its editor, router and presenter. Only physical terminal
// access changes selection; retained buffers and operations never change owner.
type sessionTerminal struct {
	inputMu  sync.Mutex
	outputMu sync.Mutex
	selected model.ChannelID
	source   planReviewInputSource
	output   io.Writer
}

type sessionTerminalInput struct {
	terminal *sessionTerminal
	id       model.ChannelID
	ctx      context.Context
	mu       sync.Mutex
}

func (input *sessionTerminalInput) LockInput()   { input.mu.Lock() }
func (input *sessionTerminalInput) UnlockInput() { input.mu.Unlock() }
func (input *sessionTerminalInput) Read(buffer []byte) (int, error) {
	input.LockInput()
	defer input.UnlockInput()
	return input.ReadInputLocked(buffer)
}
func (input *sessionTerminalInput) ReadInputLocked(buffer []byte) (int, error) {
	if err := input.ctx.Err(); err != nil {
		return 0, err
	}
	terminal := input.terminal
	terminal.inputMu.Lock()
	terminal.outputMu.Lock()
	selected := terminal.selected == input.id
	terminal.outputMu.Unlock()
	if !selected {
		terminal.inputMu.Unlock()
		select {
		case <-input.ctx.Done():
			return 0, input.ctx.Err()
		case <-time.After(20 * time.Millisecond):
			return 0, errPlanReviewInputSourceIdle
		}
	}
	count, err := terminal.source.Read(buffer)
	terminal.inputMu.Unlock()
	return count, err
}

type sessionTerminalOutput struct {
	terminal *sessionTerminal
	id       model.ChannelID
}

func (output sessionTerminalOutput) Write(buffer []byte) (int, error) {
	output.terminal.outputMu.Lock()
	defer output.terminal.outputMu.Unlock()
	if output.terminal.selected != output.id {
		return len(buffer), nil
	}
	return output.terminal.output.Write(buffer)
}

func (terminal *sessionTerminal) selectConsole(id model.ChannelID, console *chatConsole) error {
	terminal.inputMu.Lock()
	terminal.outputMu.Lock()
	terminal.selected = id
	terminal.outputMu.Unlock()
	terminal.inputMu.Unlock()
	view := console.workspace
	view.mu.Lock()
	defer view.mu.Unlock()
	view.lastFrame = nil
	return view.drawLocked()
}

func newRetainedSessionConsole(ctx context.Context, terminal *sessionTerminal, id model.ChannelID, width, height int) (*chatConsole, error) {
	view, err := newChatWorkspace(sessionTerminalOutput{terminal, id}, width, height)
	if err != nil {
		return nil, err
	}
	source := &workspaceInputSource{source: &sessionTerminalInput{terminal: terminal, id: id, ctx: ctx}, view: view}
	router, err := newPlanReviewInputRouter(source)
	if err != nil {
		return nil, err
	}
	source.review = router.ReviewEnabled
	paste := newBracketedPasteReader(router)
	editor := term.NewTerminal(terminalReadWriter{reader: paste, writer: view}, "you> ")
	console := &chatConsole{workspace: view, terminal: editor, pasteReader: paste, reviewInput: router, output: view, terminalFD: -1, editorMode: "infer", prompt: "you> "}
	columns, rows, _ := view.Resize(width, height)
	console.terminalWidth, console.terminalHeight = columns, rows
	if err := editor.SetSize(columns, rows); err != nil {
		return nil, err
	}
	view.editorSwitching = true
	return console, nil
}
