package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func buildImplementationReviewPrompt(item artifacts.ImplementationWorkItem, context implementationWorkContext, content string) (string, error) {
	payloadJSON, err := json.MarshalIndent(struct {
		WorkItemID          string                             `json:"work_item_id"`
		Discipline          string                             `json:"discipline"`
		Path                string                             `json:"path"`
		Responsibility      string                             `json:"responsibility"`
		AcceptanceCriteria  []string                           `json:"acceptance_criteria"`
		GlobalConstraints   []string                           `json:"global_constraints"`
		Dependencies        []implementationContextFile        `json:"dependencies"`
		DependencyContracts []implementationDependencyContract `json:"dependency_contracts"`
		ConsumerContracts   []implementationDependencyContract `json:"consumer_contracts"`
		CandidateContent    string                             `json:"candidate_content"`
	}{
		WorkItemID:          item.ID,
		Discipline:          item.Discipline,
		Path:                item.Path,
		Responsibility:      item.Responsibility,
		AcceptanceCriteria:  item.AcceptanceCriteria,
		GlobalConstraints:   context.GlobalConstraints,
		Dependencies:        context.Dependencies,
		DependencyContracts: context.DependencyContracts,
		ConsumerContracts:   context.ConsumerContracts,
		CandidateContent:    content,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal file review payload: %w", err)
	}
	return strings.Join([]string{
		"You are an independent semantic file reviewer. Review exactly one candidate file against exactly one declared contract after server-side syntax/package checks have passed where applicable.",
		"Do not redesign the application, request unrelated work, write replacement code, or infer requirements from memory. Findings must be concrete enough for the owning file worker to act on immediately.",
		"Server-side deterministic checks are authoritative. Do not contradict them, invent undeclared dependencies, or demand behavior forbidden by a global constraint.",
		"Reject speculative exported behavior, placeholder or future-work code, and APIs unsupported by the exact work item or consumer contracts.",
		"Do not speculate about compilation or demand edits to another file. Reject only missing declared behavior, dependency contract violations visible in this candidate, tests that weaken stated behavior, and documentation that claims behavior absent from dependency contracts.",
		`Return exactly one JSON object: {"role_id":"file_reviewer","work_item_id":"<exact id>","verdict":"pass|revise","findings":["specific correction"]}. A pass requires an empty findings array; revise requires 1-6 findings.`,
		"FILE_REVIEW_CONTRACT:\n" + string(payloadJSON),
		implementationControlCommand,
	}, "\n\n"), nil
}

func buildImplementationTriagePrompt(ledger artifacts.ImplementationLedgerArtifact, verification artifacts.ImplementationWorkItem, failure string) (string, error) {
	type ownerContract struct {
		ID                 string   `json:"id"`
		Discipline         string   `json:"discipline"`
		Path               string   `json:"path"`
		Responsibility     string   `json:"responsibility"`
		AcceptanceCriteria []string `json:"acceptance_criteria"`
	}
	byID := implementationItemIndexes(ledger)
	closure := implementationDependencyClosure(verification, ledger, byID)
	owners := make([]ownerContract, 0, len(closure))
	for _, item := range ledger.Items {
		if item.Kind != artifacts.ImplementationWorkKindFile {
			continue
		}
		if _, authorized := closure[item.ID]; !authorized {
			continue
		}
		owners = append(owners, ownerContract{
			ID: item.ID, Discipline: item.Discipline, Path: item.Path,
			Responsibility: item.Responsibility, AcceptanceCriteria: item.AcceptanceCriteria,
		})
	}
	payloadJSON, err := json.MarshalIndent(struct {
		VerificationItemID string          `json:"verification_item_id"`
		Owners             []ownerContract `json:"eligible_file_owners"`
		ObservedFailure    string          `json:"observed_failure"`
	}{verification.ID, owners, trimForBudget(failure, 12000)}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal implementation failure triage payload: %w", err)
	}
	return strings.Join([]string{
		"You are a failure triager. Route one observed verification failure to exactly one eligible file owner.",
		"Do not propose a redesign or edit. Select the owner whose declared responsibility should correct the root cause, not merely the first stack-frame path. Never route a product-code defect to tests so tests can be weakened.",
		"Feedback must state the observed failure and the specific correction the selected owner must make while preserving its contract.",
		`Return exactly one JSON object: {"role_id":"failure_triager","verification_item_id":"<exact id>","owner_id":"<eligible file owner id>","feedback":"<direct specific correction>"}.`,
		"FAILURE_TRIAGE_INPUT:\n" + string(payloadJSON),
		implementationControlCommand,
	}, "\n\n"), nil
}
