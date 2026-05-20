package omni

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
)

type RecipeCompletionProbeResult struct {
	RecipeID string `json:"recipe_id"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

func RunRecipeCompletionProbes(ctx context.Context, recipe Recipe, workspace string) ([]RecipeCompletionProbeResult, bool) {
	results := []RecipeCompletionProbeResult{}
	if len(recipe.CompletionChecks) == 0 {
		return results, false
	}
	allPassed := true
	for _, command := range recipe.CompletionChecks {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode, err := ExecuteStructuredCommandInDir(probeCtx, command, workspace, &stdout, &stderr)
		cancel()
		if err != nil || exitCode != 0 {
			allPassed = false
		}
		results = append(results, RecipeCompletionProbeResult{
			RecipeID: recipe.ID,
			Command:  command,
			ExitCode: exitCode,
			Stdout:   truncateStructuredObservation(stdout.String()),
			Stderr:   truncateStructuredObservation(stderr.String()),
		})
	}
	return results, allPassed && len(results) > 0
}

func ApplyRecipeProbeCompletion(ledger []StructuredObjective, recipe Recipe, passed bool, evidence string) []StructuredObjective {
	if !passed {
		return ledger
	}
	updates := make([]StructuredObjective, 0, len(recipe.Objectives))
	for _, objective := range recipe.Objectives {
		updates = append(updates, StructuredObjective{
			ID:          objective.ID,
			Description: objective.Description,
			Status:      "satisfied",
			Evidence:    evidence,
		})
	}
	return mergeStructuredObjectiveLedger(ledger, updates)
}

func FormatRecipeProbeEvidence(results []RecipeCompletionProbeResult) string {
	parts := make([]string, 0, len(results))
	for _, result := range results {
		parts = append(parts, fmt.Sprintf("%s exit=%d", result.Command, result.ExitCode))
	}
	return strings.Join(parts, "; ")
}
