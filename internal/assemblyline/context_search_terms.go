package assemblyline

import (
	"fmt"
	"strings"
)

const ContextSearchTermsSchemaV1 = "omnidex.context-search-terms.v1"

// ContextSearchTermsInput contains the current instruction plus optional
// code-owned domain scope. Scope selects transport policy and is never part of
// the model projection. Code owns every source queried with returned strings.
type ContextSearchTermsInput struct {
	ExactInstruction string       `json:"exact_instruction"`
	Scope            ContextScope `json:"scope,omitempty"`
}

type ContextSearchTermsDecision struct {
	Schema string   `json:"schema"`
	Terms  []string `json:"terms"`
}

func (input ContextSearchTermsInput) validate() error {
	if err := input.Scope.Validate(); err != nil {
		return err
	}
	return validateContextExactInstruction(input.ExactInstruction)
}

func (decision ContextSearchTermsDecision) ValidateFor(input ContextSearchTermsInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ContextSearchTermsSchemaV1 {
		return fmt.Errorf("context search terms schema must be %q", ContextSearchTermsSchemaV1)
	}
	if decision.Terms == nil {
		return fmt.Errorf("context search terms must be an explicit array")
	}
	if len(decision.Terms) > MaxContextSearchTerms {
		return fmt.Errorf("context search terms exceed %d items", MaxContextSearchTerms)
	}
	seen := make(map[string]struct{}, len(decision.Terms))
	for index, term := range decision.Terms {
		if err := validateContextSearchTerm(term); err != nil {
			return fmt.Errorf("context search term %d: %w", index, err)
		}
		identity := strings.ToLower(term)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("context search term %q is duplicated", term)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func validateContextSearchTerm(term string) error {
	if err := validateContextText("search term", term, MaxContextSearchTermBytes); err != nil {
		return err
	}
	if strings.ContainsAny(term, "\r\n") {
		return fmt.Errorf("search term must be one line")
	}
	return nil
}
