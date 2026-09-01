package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func renderPlanReview(state planReviewState) (string, error) {
	if err := validatePlanReviewState(state); err != nil {
		return "", err
	}

	var rendered strings.Builder
	fmt.Fprintln(&rendered, "PLAN REVIEW")
	fmt.Fprintf(
		&rendered,
		"job %d · generation %d · state %s · scope %s · revision %d\n\n",
		state.snapshot.JobID,
		state.snapshot.Generation,
		state.snapshot.State,
		state.snapshot.ScopeMode,
		state.snapshot.Revision,
	)
	if len(state.snapshot.Leaves) == 0 {
		fmt.Fprintln(&rendered, "No proposed coding leaves were produced for this objective.")
		fmt.Fprintln(&rendered)
		fmt.Fprintln(&rendered, "Guidance is required before coding can start.")
		fmt.Fprintln(
			&rendered,
			"Controls: N note and replan this same job · Ctrl-D exit client",
		)
		return rendered.String(), nil
	}
	for index, leaf := range state.snapshot.Leaves {
		marker := " "
		if index == state.selected {
			marker = ">"
		}
		symbol, _ := planReviewAnnotationPresentation(leaf.Annotation)
		fmt.Fprintf(
			&rendered,
			"%s %d. [%s] %s %s\n",
			marker,
			index+1,
			leaf.Decision,
			symbol,
			planReviewTerminalText(leaf.Statement),
		)
	}

	fmt.Fprintln(&rendered)
	fmt.Fprintln(
		&rendered,
		"Annotations: ✓ grounded · ~ reasonable derivation · ? speculative review · ! concrete scope conflict",
	)
	approved, rejected, pending := planReviewDecisionCounts(state.snapshot)
	if state.confirming {
		fmt.Fprintf(
			&rendered,
			"Freeze this work set: %d approved · %d rejected.\n",
			approved,
			rejected,
		)
		fmt.Fprintln(
			&rendered,
			"Press Enter again to confirm and start coding · Esc cancel · N note and replan this same job",
		)
		return rendered.String(), nil
	}

	fmt.Fprintln(
		&rendered,
		"Controls: ↑/↓ move · Space approve/reject · N note and replan this same job · Enter open freeze confirmation · Ctrl-D exit client",
	)
	if pending > 0 || approved == 0 {
		parts := make([]string, 0, 2)
		if pending > 0 {
			leafLabel := "leaves"
			if pending == 1 {
				leafLabel = "leaf"
			}
			parts = append(parts, fmt.Sprintf("decide %d pending %s", pending, leafLabel))
		}
		if approved == 0 {
			parts = append(parts, "approve at least one leaf")
		}
		fmt.Fprintf(&rendered, "Freeze unavailable: %s.\n", strings.Join(parts, " and "))
	} else {
		fmt.Fprintln(&rendered, "All leaves are decided. Press Enter to inspect the frozen work set.")
	}
	return rendered.String(), nil
}

func planReviewDecisionCounts(snapshot model.CodingPlan) (approved, rejected, pending int) {
	for _, leaf := range snapshot.Leaves {
		switch leaf.Decision {
		case model.CodingPlanDecisionApproved:
			approved++
		case model.CodingPlanDecisionRejected:
			rejected++
		case model.CodingPlanDecisionPending:
			pending++
		}
	}
	return approved, rejected, pending
}

func planReviewAnnotationPresentation(annotation model.CodingPlanAnnotation) (string, string) {
	switch annotation {
	case model.CodingPlanAnnotationGrounded:
		return "✓", "grounded"
	case model.CodingPlanAnnotationReasonableDerivation:
		return "~", "reasonable derivation"
	case model.CodingPlanAnnotationSpeculativeReview:
		return "?", "speculative review"
	case model.CodingPlanAnnotationConcreteConflict:
		return "!", "concrete scope conflict"
	default:
		return "", ""
	}
}

func planReviewTerminalText(value string) string {
	quoted := strconv.QuoteToGraphic(value)
	return quoted[1 : len(quoted)-1]
}
