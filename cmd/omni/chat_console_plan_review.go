package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

func (console *chatConsole) ShowPlanReview(value string) error {
	if console == nil || console.terminal == nil || console.reviewInput == nil || console.output == nil {
		return fmt.Errorf("interactive plan-review console is unavailable")
	}
	value = safeConsoleText(value)
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("interactive plan review requires visible content")
	}
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	console.reviewView = value
	console.reviewInput.EnableReview()
	if !console.reviewActive {
		console.terminal.SetPrompt("")
		if _, err := console.terminal.Write(nil); err != nil {
			console.reviewInput.DisableReview()
			console.restorePromptAfterPlanReviewFailureLocked()
			return err
		}
		if err := writeTerminalControl(console.output, "\x1b[?1049h\x1b[?25l"); err != nil {
			console.reviewInput.DisableReview()
			_ = writeTerminalControl(console.output, "\x1b[?25h\x1b[?1049l")
			console.restorePromptAfterPlanReviewFailureLocked()
			return err
		}
		console.reviewActive = true
	}
	if err := console.redrawPlanReviewLocked(); err != nil {
		console.reviewInput.DisableReview()
		cleanupErr := console.leavePlanReviewScreenLocked()
		return errors.Join(err, cleanupErr)
	}
	return nil
}

func (console *chatConsole) HidePlanReview() error {
	if console == nil {
		return nil
	}
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	if console.reviewInput != nil {
		console.reviewInput.DisableReview()
	}
	if !console.reviewActive {
		return nil
	}
	return console.leavePlanReviewScreenLocked()
}

func (console *chatConsole) BeginPlanReviewNote() (terminalInputAuthority, error) {
	if console == nil || console.reviewInput == nil {
		return terminalInputAuthority{}, fmt.Errorf("interactive plan-review console is unavailable")
	}
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	if !console.reviewActive {
		return terminalInputAuthority{}, fmt.Errorf("interactive plan-review screen is not active")
	}
	authority, err := console.reviewInput.BeginNoteInput()
	if err != nil {
		return terminalInputAuthority{}, err
	}
	if err := console.leavePlanReviewScreenLocked(); err != nil {
		return terminalInputAuthority{}, err
	}
	return authority, nil
}

func (console *chatConsole) EndPlanReviewNote(authority terminalInputAuthority) error {
	if console == nil || console.reviewInput == nil {
		return fmt.Errorf("interactive plan-review console is unavailable")
	}
	return console.reviewInput.EndNoteInput(authority)
}

func (console *chatConsole) CurrentInputAuthority() terminalInputAuthority {
	if console == nil || console.reviewInput == nil {
		return terminalInputAuthority{}
	}
	return console.reviewInput.CurrentInputAuthority()
}

func (console *chatConsole) PlanReviewInput() *planReviewInputRouter {
	if console == nil {
		return nil
	}
	return console.reviewInput
}

func (console *chatConsole) redrawPlanReviewLocked() error {
	if !console.reviewActive || console.output == nil {
		return fmt.Errorf("interactive plan-review screen is unavailable")
	}
	view := strings.ReplaceAll(console.reviewView, "\n", "\r\n")
	if !strings.HasSuffix(view, "\r\n") {
		view += "\r\n"
	}
	return writeTerminalControl(console.output, "\x1b[H\x1b[2J"+view)
}

func (console *chatConsole) writeAroundPlanReviewLocked(value string) error {
	if err := writeTerminalControl(console.output, "\x1b[?25h\x1b[?1049l"); err != nil {
		return err
	}
	if _, err := console.terminal.Write([]byte(value)); err != nil {
		return err
	}
	if err := writeTerminalControl(console.output, "\x1b[?1049h\x1b[?25l"); err != nil {
		return err
	}
	return console.redrawPlanReviewLocked()
}

func (console *chatConsole) leavePlanReviewScreenLocked() error {
	if err := writeTerminalControl(console.output, "\x1b[?25h\x1b[?1049l"); err != nil {
		return err
	}
	console.reviewActive = false
	console.reviewView = ""
	console.promptMu.Lock()
	prompt := console.prompt
	console.promptMu.Unlock()
	console.terminal.SetPrompt(prompt)
	_, err := console.terminal.Write(nil)
	return err
}

func (console *chatConsole) restorePromptAfterPlanReviewFailureLocked() {
	console.promptMu.Lock()
	prompt := console.prompt
	console.promptMu.Unlock()
	console.terminal.SetPrompt(prompt)
	_, _ = console.terminal.Write(nil)
}

func writeTerminalControl(destination io.Writer, value string) error {
	if destination == nil {
		return fmt.Errorf("terminal output is unavailable")
	}
	for len(value) > 0 {
		count, err := io.WriteString(destination, value)
		if err != nil {
			return err
		}
		if count < 1 {
			return io.ErrShortWrite
		}
		value = value[count:]
	}
	return nil
}
