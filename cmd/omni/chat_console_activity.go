package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/term"
)

// Activity is a presentation of observed work, never execution authority.
type chatActivity struct {
	JobID      int64
	Generation int64
	StepID     int64
	Label      string
	Working    bool
	StartedAt  time.Time
}

func (console *chatConsole) SetActivity(activity chatActivity) error {
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	previous := console.activity
	if previous.JobID != activity.JobID || previous.Generation != activity.Generation ||
		previous.StepID != activity.StepID || previous.Label != activity.Label || previous.Working != activity.Working {
		console.activityDetail = ""
		console.activityFrame = 0
	}
	console.activity = activity
	return console.redrawPromptLocked(time.Now())
}

func (console *chatConsole) SetActivityDetail(jobID, stepID int64, detail string) error {
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	if console.activity.JobID != jobID || console.activity.StepID != stepID || !console.activity.Working {
		return nil
	}
	console.activityDetail = detail
	return console.redrawPromptLocked(time.Now())
}

func (console *chatConsole) PulseActivity(now time.Time) error {
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	if console.reviewActive && console.editorMode != "research" && console.editorMode != "ledger" {
		if console.workspace != nil && console.terminalFD >= 0 {
			if w, h, err := term.GetSize(console.terminalFD); err == nil && w > 0 && h > 0 {
				_, _, resizeErr := console.workspace.Resize(w, h)
				return resizeErr
			}
		}
		return nil
	}
	if console.activity.Working {
		console.activityFrame++
	}
	return console.redrawPromptLocked(now)
}

func (console *chatConsole) redrawPromptLocked(now time.Time) error {
	if console.reviewActive && console.editorMode != "research" && console.editorMode != "ledger" {
		return nil
	}
	console.promptMu.Lock()
	prompt := console.prompt
	console.promptMu.Unlock()
	if console.terminalFD >= 0 {
		if width, height, err := term.GetSize(console.terminalFD); err == nil && width > 0 && height > 0 {
			if console.workspace != nil {
				var resizeErr error
				width, height, resizeErr = console.workspace.Resize(width, height)
				if resizeErr != nil {
					return resizeErr
				}
			}
			if width != console.terminalWidth || height != console.terminalHeight {
				if err := console.terminal.SetSize(width, height); err != nil {
					return err
				}
				console.terminalWidth, console.terminalHeight = width, height
			}
		}
	}
	width := console.terminalWidth
	if width <= 0 {
		width = 80
	}
	activity := console.activity
	if console.workspace != nil {
		if err := console.workspace.SetActivity(activity, console.activityDetail); err != nil {
			return err
		}
	}
	if activity.JobID > 0 && console.workspace == nil {
		label := activity.Label
		if console.activityDetail != "" && console.activityDetail != label {
			label += " | " + console.activityDetail
		}
		if activity.Working {
			frames := []rune("|/-\\")
			label = string(frames[console.activityFrame%len(frames)]) + " " + label
			if !activity.StartedAt.IsZero() {
				elapsed := max(time.Duration(0), now.Sub(activity.StartedAt)).Truncate(time.Second)
				label += " | " + elapsed.String()
			}
		}
		// x/term counts every rune as one column. Only this compact status
		// projection uses ASCII cells; original Unicode stays in the transcript.
		row := []rune(singleCellActivityText(label))
		if len(row) > width {
			row = append(row[:width-1], '~')
		}
		prompt = string(row) + strings.Repeat(" ", width-len(row)) + prompt
	}
	if console.renderedPrompt == prompt {
		return nil
	}
	console.renderedPrompt = prompt
	console.terminal.SetPrompt(prompt)
	// Terminal.Write moves to the editor origin before writing this trusted
	// erase. Clear old wrapped rows below a cursor parked inside the input.
	_, err := console.terminal.Write([]byte("\x1b[0J"))
	return err
}

func singleCellActivityText(value string) string {
	value = strings.NewReplacer("\n", " ", "\t", " ").Replace(value)
	var rendered strings.Builder
	for _, character := range safeConsoleText(value) {
		switch {
		case character <= '~':
			rendered.WriteRune(character)
		case character <= '\uffff':
			fmt.Fprintf(&rendered, "\\u%04x", character)
		default:
			fmt.Fprintf(&rendered, "\\U%08x", character)
		}
	}
	return rendered.String()
}

// One display timer animates real active work. It performs no runtime polling.
func startChatActivityDisplay(ctx context.Context, console *chatConsole) (<-chan error, func()) {
	displayContext, cancel := context.WithCancel(ctx)
	errors := make(chan error, 1)
	done := make(chan struct{})
	resizes, stopResize := watchChatResize()
	go func() {
		defer close(done)
		defer stopResize()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-displayContext.Done():
				return
			case <-resizes:
				if err := console.PulseActivity(time.Now()); err != nil {
					errors <- fmt.Errorf("resize chat workspace: %w", err)
					return
				}
			case now := <-ticker.C:
				if err := console.PulseActivity(now); err != nil {
					errors <- fmt.Errorf("update chat activity: %w", err)
					return
				}
			}
		}
	}()
	return errors, func() { cancel(); <-done }
}
