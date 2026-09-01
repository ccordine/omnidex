package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func printContextUpdates(
	contexts []model.StepContext,
	seenContextIDs map[int64]struct{},
	progress bool,
	verbose bool,
	maxChars int,
) bool {
	return printContextUpdatesWithUI(contexts, seenContextIDs, progress, verbose, maxChars, nil)
}

func printContextUpdatesWithUI(
	contexts []model.StepContext,
	seenContextIDs map[int64]struct{},
	progress bool,
	verbose bool,
	maxChars int,
	ui *chatUI,
) bool {
	printed := false
	for _, ctxValue := range contexts {
		if _, seen := seenContextIDs[ctxValue.ID]; seen {
			continue
		}
		seenContextIDs[ctxValue.ID] = struct{}{}
		value := strings.TrimSpace(ctxValue.Value)
		if value == "" {
			continue
		}
		switch ctxValue.Key {
		case "event":
			event := parseStepEventPayload(value)
			eventType := strings.TrimSpace(event.EventType)
			if !verbose && (!progress || !showStepEventInSlimProgress(eventType)) {
				continue
			}
			if eventType == "" {
				eventType = "unknown"
			}
			if !verbose {
				line := fmt.Sprintf("Model step %d: %s", ctxValue.StepID, summarizeStepEvent(event))
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
				printed = true
				continue
			}
			line := fmt.Sprintf("event step=%d type=%s", ctxValue.StepID, eventType)
			block := indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line+"\n"+block)
			} else {
				fmt.Printf("  %s\n", line)
				fmt.Println(block)
			}
			printed = true
		case "tool_stdout":
			kind, summary := summarizeProgressStream("stdout", value, maxChars)
			if verbose {
				line := fmt.Sprintf("%s step=%d", strings.ToLower(kind), ctxValue.StepID)
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else if progress {
				line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
			} else {
				continue
			}
			printed = true
		case "tool_stderr":
			kind, summary := summarizeProgressStream("stderr", value, maxChars)
			if verbose {
				line := fmt.Sprintf("%s step=%d", strings.ToLower(kind), ctxValue.StepID)
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else if progress {
				line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
			} else {
				continue
			}
			printed = true
		default:
			if !verbose {
				continue
			}
			line := fmt.Sprintf("context step=%d key=%s", ctxValue.StepID, ctxValue.Key)
			block := indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line+"\n"+block)
			} else {
				fmt.Printf("  %s\n", line)
				fmt.Println(block)
			}
			printed = true
		}
	}
	return printed
}
