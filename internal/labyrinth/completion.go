package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	CompletionCheckIDV1      cognition.CompletionCheckID = "labyrinth.goal-expression"
	CompletionCheckVersionV1                             = "1.0.0"
	completionCheckSchemaV1                              = "labyrinth.goal-completion-evaluator.v1"
)

// NewCompletionCheck identifies the registered deterministic evaluator. The
// obligation carries the exact desired expression independently of this ref.
func NewCompletionCheck() (cognition.CompletionCheckRef, error) {
	digest, _, err := digestJSON(struct {
		Schema  string                      `json:"schema"`
		ID      cognition.CompletionCheckID `json:"id"`
		Version string                      `json:"version"`
	}{completionCheckSchemaV1, CompletionCheckIDV1, CompletionCheckVersionV1})
	if err != nil {
		return cognition.CompletionCheckRef{}, fmt.Errorf("hash completion check: %w", err)
	}
	check := cognition.CompletionCheckRef{
		ID: CompletionCheckIDV1, Version: CompletionCheckVersionV1, SHA256: digest,
	}
	if err := check.Validate(); err != nil {
		return cognition.CompletionCheckRef{}, err
	}
	return check, nil
}

// NewCompletionAuthority registers the generic evaluator against only names
// already visible in the public scenario. A goal name remains eligible even
// when its schema is intentionally absent from the public schema catalog.
func NewCompletionAuthority(scenario Scenario) (cognition.CompletionAuthority, error) {
	if err := scenario.Validate(); err != nil {
		return cognition.CompletionAuthority{}, err
	}
	check, err := NewCompletionCheck()
	if err != nil {
		return cognition.CompletionAuthority{}, err
	}
	public := scenario.PublicArtifact()
	known := make(map[cognition.PredicateName]struct{}, len(public.World.PredicateSchemas))
	for _, schema := range public.World.PredicateSchemas {
		known[schema.Name] = struct{}{}
	}
	goal := scenario.Goal()
	for _, group := range [][]cognition.Predicate{goal.All, goal.Any, goal.Not} {
		for _, predicate := range group {
			known[predicate.Name] = struct{}{}
		}
	}
	supported := make([]cognition.PredicateName, 0, len(known))
	for name := range known {
		supported = append(supported, name)
	}
	authority, err := cognition.NewCompletionAuthority(check, supported)
	if err != nil {
		return cognition.CompletionAuthority{}, fmt.Errorf("register Labyrinth completion authority: %w", err)
	}
	return authority, nil
}
