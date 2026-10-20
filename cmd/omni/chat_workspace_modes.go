package main

import (
	"errors"

	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/term"
)

// This is a presentation boundary, never a submitted line or authority change.
type workspaceModeSwitch struct{ mode string }

func (event workspaceModeSwitch) Error() string { return "switch editor to " + event.mode }

type workspaceEditorState struct {
	terminal *term.Terminal
	paste    *bracketedPasteReader
	overflow bool
	draft    workspaceScreen
	prompt   string
}

// ReadLine retains x/term's private draft, cursor and history on a reader error.
// Keep those existing editors rather than reconstructing their state from text.
func (console *chatConsole) readEditorLine() (string, error) {
	for {
		console.presentationMu.Lock()
		editor := console.terminal
		console.presentationMu.Unlock()
		line, err := editor.ReadLine()
		var event workspaceModeSwitch
		if !errors.As(err, &event) {
			console.presentationMu.Lock()
			console.inputMode = console.editorMode
			console.presentationMu.Unlock()
			return line, err
		}
		err = console.selectEditor(event.mode)
		console.reviewInput.finishEditorSwitch(event.mode)
		if err != nil {
			return "", err
		}
	}
}

func (console *chatConsole) selectEditor(mode string) error {
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	if console.editorMode == mode {
		return nil
	}
	if console.editors == nil {
		console.editors = make(map[string]workspaceEditorState)
	}
	previous := console.editorMode
	if previous == "" {
		previous = "infer"
	}
	view := console.workspace
	view.mu.Lock()
	draft := view.detachDraftLocked()
	view.mu.Unlock()
	console.promptMu.Lock()
	previousPrompt := console.prompt
	console.promptMu.Unlock()
	console.editors[previous] = workspaceEditorState{terminal: console.terminal, paste: console.pasteReader, overflow: console.inputOverflow.Load(), draft: draft, prompt: previousPrompt}
	state, exists := console.editors[mode]
	if !exists {
		state.paste = newBracketedPasteReader(console.reviewInput, model.MaxFreeFormTurnBytes)
		state.terminal = term.NewTerminal(terminalReadWriter{reader: state.paste, writer: console.workspace}, mode+"> ")
		state.terminal.AutoCompleteCallback = console.boundTerminalInput
		state.draft = newWorkspaceScreen(console.terminalWidth)
		state.prompt = mode + "> "
	}
	console.terminal, console.pasteReader, console.editorMode = state.terminal, state.paste, mode
	console.inputOverflow.Store(state.overflow)
	console.promptMu.Lock()
	console.prompt = state.prompt
	console.promptMu.Unlock()
	console.renderedPrompt = state.prompt
	view.mu.Lock()
	view.modeOffsets[view.conversationMode] = view.conversationOffset
	view.conversationOffset = view.modeOffsets[mode]
	view.conversationMode = mode
	view.focus, view.help = workspaceCompose, false
	if mode == "ledger" {
		view.focus, view.ledger.opened = workspaceLedgerPane, true
	}
	view.attachDraftLocked(state.draft)
	columns, rows := view.editorSizeLocked()
	err := view.drawLocked()
	view.mu.Unlock()
	if err != nil {
		return err
	}
	if err := console.terminal.SetSize(columns, rows); err != nil {
		return err
	}
	prompt := state.prompt
	if console.reviewActive && (mode == "infer" || mode == "interact") {
		prompt = ""
	}
	console.terminal.SetPrompt(prompt)
	select {
	case view.modeRequests <- mode:
	default:
		select {
		case <-view.modeRequests:
		default:
		}
		view.modeRequests <- mode
	}
	return nil
}

// Only editor projection rows are retained. Streamed transcript rows remain shared.
func (view *chatWorkspace) detachDraftLocked() workspaceScreen {
	screen := &view.conversation
	start := min(len(screen.lines)-1, max(0, view.draftRow-(screen.dropped-view.draftDropped)))
	column := min(len(screen.lines[start]), max(0, view.draftColumn))
	draft := *screen
	draft.lines = append([][]rune(nil), screen.lines[start:]...)
	draft.soft = append([]bool(nil), screen.soft[start:]...)
	draft.lines[0] = append([]rune(nil), draft.lines[0][column:]...)
	draft.row -= start
	if draft.row == 0 {
		draft.column -= column
	}
	screen.lines = append([][]rune(nil), screen.lines[:start+1]...)
	screen.soft = append([]bool(nil), screen.soft[:start+1]...)
	screen.lines[start] = append([]rune(nil), screen.lines[start][:column]...)
	screen.row, screen.column, screen.wrap = start, column, false
	return draft
}

func (view *chatWorkspace) attachDraftLocked(draft workspaceScreen) {
	screen := &view.conversation
	start, column := screen.row, screen.column
	screen.lines[start] = append(screen.lines[start], draft.lines[0]...)
	screen.lines = append(screen.lines, draft.lines[1:]...)
	screen.soft[start] = draft.soft[0]
	screen.soft = append(screen.soft, draft.soft[1:]...)
	screen.row, screen.column, screen.wrap = start+draft.row, draft.column, draft.wrap
	if draft.row == 0 {
		screen.column += column
	}
	view.draftRow, view.draftColumn, view.draftDropped = start, column, screen.dropped
}

func (router *planReviewInputRouter) requestEditorSwitch(mode string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.editorSwitch = &workspaceModeSwitch{mode}
	router.editorSwitching = true
	router.signalReadLocked()
}

func (router *planReviewInputRouter) finishEditorSwitch(mode string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.editorMode, router.editorSwitching = mode, false
	router.signalCapacityLocked()
}
