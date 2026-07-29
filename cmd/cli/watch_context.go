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
		case "workspace":
			if !verbose {
				continue
			}
			line := fmt.Sprintf("Explore step %d: scanned workspace snapshot", ctxValue.StepID)
			line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "coding_diff":
			if !verbose {
				continue
			}
			line := fmt.Sprintf("Review step %d: accepted workspace diff", ctxValue.StepID)
			line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "web_search":
			if !progress && !verbose {
				continue
			}
			domains := webSearchDomainsFromContext(value)
			domainSummary := summarizeWebSearchDomains(domains, maxChars)
			line := fmt.Sprintf("Explore step %d: compiled web research context", ctxValue.StepID)
			if domainSummary != "" {
				line += " | domains: " + domainSummary
			}
			if verbose {
				if domainSummary != "" {
					line += "\n" + indentBlock("domains hit: "+domainSummary, "    ")
				}
				line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			}
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "environment", "host_environment":
			if !verbose {
				continue
			}
			line := fmt.Sprintf("Inspect step %d: captured environment details", ctxValue.StepID)
			line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "retrieved_memory", "recent_conversation":
			if !verbose {
				continue
			}
			line := fmt.Sprintf("Inspect step %d: loaded conversation/memory context", ctxValue.StepID)
			line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "llm_model_prepare":
			if !progress && !verbose {
				continue
			}
			kind, summary := summarizePreparedModelContext(value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			if verbose {
				line += "\n" + indentBlock(truncateForWatch(value, maxChars), "    ")
			}
			if ui != nil {
				emitSystem(ui, line)
			} else {
				fmt.Printf("  %s\n", line)
			}
			printed = true
		case "llm_prompt":
			if !verbose {
				continue
			}
			kind, summary := summarizeLLMTraceContext(ctxValue.Key, value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			block := indentBlock(truncateForWatch(value, maxChars), "    ")
			if ui != nil {
				emitSystem(ui, line+"\n"+block)
			} else {
				fmt.Printf("  %s\n", line)
				fmt.Println(block)
			}
			printed = true
		case "llm_response":
			if !progress && !verbose {
				continue
			}
			kind, summary := summarizeLLMTraceContext(ctxValue.Key, value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			if verbose {
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else {
				body := llmTraceBody(value)
				if body != "" {
					line += "\n" + indentBlock(truncateForWatch(body, maxChars), "    ")
				}
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
			}
			printed = true
		case "llm_error":
			if !progress && !verbose {
				continue
			}
			kind, summary := summarizeLLMTraceContext(ctxValue.Key, value, maxChars)
			line := fmt.Sprintf("%s step %d: %s", kind, ctxValue.StepID, summary)
			if verbose {
				block := indentBlock(truncateForWatch(value, maxChars), "    ")
				if ui != nil {
					emitSystem(ui, line+"\n"+block)
				} else {
					fmt.Printf("  %s\n", line)
					fmt.Println(block)
				}
			} else {
				if ui != nil {
					emitSystem(ui, line)
				} else {
					fmt.Printf("  %s\n", line)
				}
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
