package cognition

import (
	"fmt"
	"sort"
)

const CompletionAuthoritySchemaV1 = "omnidex.cognition-completion-authority.v1"

// CompletionAuthority is a code-owned registration of one evaluator and the
// exact predicate capabilities it may evaluate. It is not model-proposable.
type CompletionAuthority struct {
	Schema              string             `json:"schema"`
	Check               CompletionCheckRef `json:"check"`
	SupportedPredicates []PredicateName    `json:"supported_predicates"`
	SHA256              string             `json:"sha256"`
}

func NewCompletionAuthority(
	check CompletionCheckRef,
	supported []PredicateName,
) (CompletionAuthority, error) {
	values := cloneSlice(supported)
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	authority := CompletionAuthority{
		Schema: CompletionAuthoritySchemaV1, Check: check,
		SupportedPredicates: values,
	}
	digest, err := completionAuthoritySHA256(authority)
	if err != nil {
		return CompletionAuthority{}, err
	}
	authority.SHA256 = digest
	if err := authority.Validate(); err != nil {
		return CompletionAuthority{}, err
	}
	return authority, nil
}

func (authority CompletionAuthority) Validate() error {
	if authority.Schema != CompletionAuthoritySchemaV1 {
		return fmt.Errorf("%w: schema %q is not registered", ErrInvalidCompletionAuthority, authority.Schema)
	}
	if err := authority.Check.Validate(); err != nil {
		return fmt.Errorf("%w: check: %v", ErrInvalidCompletionAuthority, err)
	}
	if len(authority.SupportedPredicates) == 0 || len(authority.SupportedPredicates) > MaxCompletionPredicates {
		return fmt.Errorf(
			"%w: supported predicate count must be between 1 and %d",
			ErrInvalidCompletionAuthority, MaxCompletionPredicates,
		)
	}
	for index, name := range authority.SupportedPredicates {
		if err := validateIdentity(string(name), "completion predicate name"); err != nil {
			return fmt.Errorf("%w: predicate %d: %v", ErrInvalidCompletionAuthority, index, err)
		}
		if index > 0 && name <= authority.SupportedPredicates[index-1] {
			return fmt.Errorf("%w: supported predicates must be uniquely sorted", ErrInvalidCompletionAuthority)
		}
	}
	want, err := completionAuthoritySHA256(authority)
	if err != nil || !validSHA256(authority.SHA256) || authority.SHA256 != want {
		return fmt.Errorf("%w: hash does not bind the exact authority", ErrInvalidCompletionAuthority)
	}
	return nil
}

func (authority CompletionAuthority) Resolve(goal GoalExpression) (CompletionCheckRef, error) {
	if err := authority.Validate(); err != nil {
		return CompletionCheckRef{}, err
	}
	canonical, err := canonicalGoal(goal)
	if err != nil {
		return CompletionCheckRef{}, fmt.Errorf("%w: goal: %v", ErrInvalidCompletionAuthority, err)
	}
	registered := make(map[PredicateName]struct{}, len(authority.SupportedPredicates))
	for _, name := range authority.SupportedPredicates {
		registered[name] = struct{}{}
	}
	for _, group := range [][]Predicate{canonical.All, canonical.Any, canonical.Not} {
		for _, predicate := range group {
			if _, exists := registered[predicate.Name]; !exists {
				return CompletionCheckRef{}, fmt.Errorf(
					"%w: predicate %q has no registered evaluator",
					ErrUnsupportedCompletionPredicate, predicate.Name,
				)
			}
		}
	}
	return authority.Check, nil
}

func (authority CompletionAuthority) Clone() CompletionAuthority {
	authority.SupportedPredicates = cloneSlice(authority.SupportedPredicates)
	return authority
}

func completionAuthoritySHA256(authority CompletionAuthority) (string, error) {
	return cognitionValueSHA256(struct {
		Schema              string
		Check               CompletionCheckRef
		SupportedPredicates []PredicateName
	}{authority.Schema, authority.Check, authority.SupportedPredicates})
}
