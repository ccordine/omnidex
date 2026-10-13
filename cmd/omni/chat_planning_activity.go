package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func (renderer *chatRenderer) planningActivity(event client.RealtimeEvent) error {
	activity := *event.PlanningActivity
	if err := activity.Validate(); err != nil {
		return fmt.Errorf("render planning observation: %w", err)
	}
	renderer.rememberPlanningCall(event)
	if renderer.verbosity == chatQuiet {
		return nil
	}
	text, err := planningActivityNarration(activity)
	if err != nil {
		return err
	}
	text += planningActivityIdentity(activity)
	var rendered strings.Builder
	renderBlock(&rendered, "check>", text)
	if activity.Truncated {
		fmt.Fprintln(&rendered, "[omni] Returned content preview truncated.")
	}
	if renderer.verbosity == chatVerbose || renderer.verbosity == chatDebug {
		if activity.WorkKind != "" {
			fmt.Fprintf(&rendered, "[omni] check %s · station %s · model %s\n", activity.WorkKind, activity.Station, activity.Model)
		}
	}
	return renderer.console.WriteOutput(rendered.String())
}

func planningActivityNarration(activity model.PlanningActivity) (string, error) {
	switch activity.Kind {
	case "semantic_started":
		if activity.OptionOffset > 0 {
			return fmt.Sprintf("Options continued (%d–%d of %d):\n%s", activity.OptionOffset+1, activity.OptionOffset+len(activity.Options), activity.OptionTotal, strings.Join(activity.Options, "\n")), nil
		}
		text := activity.Question + planningCandidateContext(activity)
		if activity.InferenceFree {
			text += "\nResolved by code."
		}
		return text + "\nWhy: " + activity.Why + "\nOptions:\n" + strings.Join(activity.Options, "\n"), nil
	case "semantic_completed":
		if activity.ResultOffset > 0 {
			return fmt.Sprintf("Result continued (bytes %d–%d of %d):\n%s", activity.ResultOffset+1, activity.ResultOffset+len(activity.Result), activity.ResultTotal, activity.Result), nil
		}
		text := activity.Question + planningCandidateContext(activity)
		if activity.InferenceFree {
			text += "\nResolved by code."
		}
		return text + "\nResult: " + activity.Result + "\nCode rule: " + activity.Consumer, nil
	case "inventory_started":
		return "Identifying outcomes", nil
	case "inventory_completed":
		return planningOutcomeCount(activity.Count, "proposed outcome", "proposed outcomes") + " found", nil
	case "candidate_started":
		return "Considering outcome: " + activity.Subject, nil
	case "candidate_resolved":
		return planningDispositionNarration(activity)
	case "selected":
		return planningOutcomeCount(activity.Count, "outcome", "outcomes") + " selected for current work", nil
	default:
		return "", fmt.Errorf("cannot render unregistered planning kind %q", activity.Kind)
	}
}

func planningActivityIdentity(activity model.PlanningActivity) string {
	if activity.WorkKind == "" {
		return ""
	}
	identity := ""
	if !activity.InferenceFree {
		identity = "\nModel: " + activity.Model
	}
	if activity.CallID > 0 {
		identity += fmt.Sprintf("\nCall: %d", activity.CallID)
	}
	return identity
}

func planningCandidateContext(activity model.PlanningActivity) string {
	var context strings.Builder
	if activity.Subject != "" {
		context.WriteString("\nCandidate: " + activity.Subject)
	}
	if activity.Against != "" {
		context.WriteString("\nCompared with: " + activity.Against)
	}
	return context.String()
}

func planningDispositionNarration(activity model.PlanningActivity) (string, error) {
	var label string
	switch activity.Outcome {
	case "retained":
		label = "Included in current work"
	case "excluded":
		label = "Excluded from current work"
	case "unrequested":
		label = "Omitted because it was not requested"
	case "duplicate":
		label = "Omitted as a duplicate"
	case "partitioned":
		label = "Split into " + planningOutcomeCount(activity.Count, "outcome", "outcomes")
	case "unresolved":
		label = "No verifiable runtime outcome established"
	case "deferred":
		label = "Deferred because candidate queue is full"
	default:
		return "", fmt.Errorf("cannot render unregistered planning disposition %q", activity.Outcome)
	}
	return label + ": " + activity.Subject, nil
}

func planningOutcomeCount(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}
