package main

import (
	"context"
	"fmt"
	"time"
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
	if !console.activity.Working || console.reviewActive {
		return nil
	}
	console.activityFrame++
	return console.redrawPromptLocked(now)
}

func (console *chatConsole) redrawPromptLocked(now time.Time) error {
	if console.reviewActive {
		return nil
	}
	console.promptMu.Lock()
	prompt := console.prompt
	console.promptMu.Unlock()
	activity := console.activity
	if activity.JobID > 0 {
		label := activity.Label
		if console.activityDetail != "" && console.activityDetail != label {
			label += " · " + console.activityDetail
		}
		if activity.Working {
			frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
			label = string(frames[console.activityFrame%len(frames)]) + " " + label
			if !activity.StartedAt.IsZero() {
				elapsed := max(time.Duration(0), now.Sub(activity.StartedAt)).Truncate(time.Second)
				label += " · " + elapsed.String()
			}
		}
		prompt = safeConsoleText(label) + "  " + prompt
	}
	if console.renderedPrompt == prompt {
		return nil
	}
	console.renderedPrompt = prompt
	console.terminal.SetPrompt(prompt)
	_, err := console.terminal.Write(nil)
	return err
}

// One display timer animates real active work. It performs no runtime polling.
func startChatActivityDisplay(ctx context.Context, console *chatConsole) (<-chan error, func()) {
	displayContext, cancel := context.WithCancel(ctx)
	errors := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-displayContext.Done():
				return
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
