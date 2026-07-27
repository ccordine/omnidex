package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func latestContextValue(contexts []model.StepContext, key string) string {
	value := ""
	var lastID int64
	for _, ctx := range contexts {
		if ctx.Key != key {
			continue
		}
		if ctx.ID >= lastID {
			lastID = ctx.ID
			value = ctx.Value
		}
	}
	return value
}

func printStepStatusUpdates(steps []model.Step, lastStepStatus map[int64]string) bool {
	return printStepStatusUpdatesWithUI(steps, lastStepStatus, nil)
}

func printStepStatusUpdatesWithUI(steps []model.Step, lastStepStatus map[int64]string, ui *chatUI) bool {
	printed := false
	for _, step := range steps {
		status := strings.TrimSpace(step.Status)
		if lastStepStatus[step.ID] == status {
			continue
		}
		lastStepStatus[step.ID] = status
		if !printed {
			line := formatWorkloadQueueStatusLine(steps, ui)
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
		}
		line := formatStepStatusLine(step, ui)
		if ui != nil {
			emitSystem(ui, line)
		} else {
			fmt.Printf("  %s\n", line)
		}
		printed = true
	}
	return printed
}

func formatWorkloadQueueStatusLine(steps []model.Step, ui *chatUI) string {
	completed := 0
	incomplete := 0
	failed := 0
	active := model.Step{}
	for _, step := range steps {
		switch strings.ToLower(strings.TrimSpace(step.Status)) {
		case model.StepStatusCompleted:
			completed++
		case model.StepStatusFailed, model.StepStatusCanceled:
			failed++
			incomplete++
		default:
			incomplete++
			if active.ID == 0 && stepStatusIsActive(step.Status) {
				active = step
			}
		}
	}
	activeText := "none"
	if active.ID != 0 {
		activeText = fmt.Sprintf("#%d %s", active.ID, strings.TrimSpace(active.Action))
		if strings.TrimSpace(active.Action) == "" {
			activeText = fmt.Sprintf("#%d", active.ID)
		}
	}
	line := fmt.Sprintf("Workload queue | active=%s | completed=%d | incomplete=%d", activeText, completed, incomplete)
	if failed > 0 {
		line += fmt.Sprintf(" | failed=%d", failed)
	}
	if ui != nil && active.ID != 0 {
		return ui.paint(line, ansiBold+ansiYellow)
	}
	if ui != nil && incomplete == 0 {
		return ui.paint(line, ansiGreen)
	}
	return line
}

func formatStepStatusLine(step model.Step, ui *chatUI) string {
	marker := stepStatusMarker(step.Status)
	line := fmt.Sprintf("%s Step %d | phase=%s | action=%s | status=%s", marker, step.ID, phaseForStepAction(step.Action), step.Action, step.Status)
	if ui == nil {
		return line
	}
	switch strings.ToLower(strings.TrimSpace(step.Status)) {
	case model.StepStatusRunning, model.StepStatusWaiting:
		return ui.paint(line, ansiBold+ansiBlink+ansiYellow)
	case model.StepStatusCompleted:
		return ui.paint(line, ansiGreen)
	case model.StepStatusFailed, model.StepStatusCanceled:
		return ui.paint(line, ansiBold+ansiRed)
	case model.StepStatusPending:
		return ui.paint(line, ansiDim)
	default:
		return line
	}
}

func stepStatusMarker(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.StepStatusRunning, model.StepStatusWaiting:
		return ">> ACTIVE"
	case model.StepStatusCompleted:
		return "OK DONE"
	case model.StepStatusFailed, model.StepStatusCanceled:
		return "!! STOP"
	case model.StepStatusPending:
		return ".. TODO"
	default:
		return "-- STEP"
	}
}

func stepStatusIsActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.StepStatusRunning, model.StepStatusWaiting:
		return true
	default:
		return false
	}
}

func phaseForStepAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "plan", "tooling", "workspace_scan", "tag", "retrieve":
		return "planning"
	case "verify":
		return "review"
	default:
		return "execution"
	}
}

func printStepDetailUpdates(steps []model.Step, lastStepDetails map[int64]string, maxChars int) bool {
	printed := false
	for _, step := range steps {
		signature := strings.Join([]string{step.Status, step.Output, step.Error}, "\x00")
		if lastStepDetails[step.ID] == signature {
			continue
		}
		lastStepDetails[step.ID] = signature
		if output := strings.TrimSpace(step.Output); output != "" {
			fmt.Printf("  step %d output\n", step.ID)
			fmt.Println(indentBlock(truncateForWatch(output, maxChars), "    "))
			printed = true
		}
		if stepErr := strings.TrimSpace(step.Error); stepErr != "" {
			fmt.Printf("  step %d error\n", step.ID)
			fmt.Println(indentBlock(truncateForWatch(stepErr, maxChars), "    "))
			printed = true
		}
	}
	return printed
}
