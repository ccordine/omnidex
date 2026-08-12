package semanticreview

import (
	"fmt"
	"reflect"
)

type CorrectionRuleRegistry struct {
	specificationID     ReviewSpecificationID
	specificationDigest string
	rules               map[FindingCode]CorrectionRule
}

func NewCorrectionRuleRegistry(
	specification ReviewSpecification,
	rules []CorrectionRule,
) (CorrectionRuleRegistry, error) {
	if err := preflightSpecification(specification); err != nil {
		return CorrectionRuleRegistry{}, err
	}
	if len(rules) > maxCorrectionRules {
		return CorrectionRuleRegistry{}, fmt.Errorf("%w: rule count exceeds %d", ErrInvalidRuleRegistry, maxCorrectionRules)
	}
	for index, rule := range rules {
		if !validIdentity(string(rule.FindingCode)) ||
			!validIdentity(string(rule.ObjectiveKind)) || len(rule.Acceptance) != 1 ||
			rule.Acceptance[0] != AcceptanceCorrectionArtifactVerified {
			return CorrectionRuleRegistry{}, fmt.Errorf("%w: rule %d exceeds bounds", ErrInvalidRuleRegistry, index)
		}
	}
	specification = cloneSpecification(specification)
	if err := validateSpecification(specification); err != nil {
		return CorrectionRuleRegistry{}, err
	}
	issues := make(map[FindingCode]struct{})
	for _, candidate := range specification.Candidates {
		if candidate.Kind == FindingSemanticIssue {
			issues[candidate.FindingCode] = struct{}{}
		}
	}
	registered := make(map[FindingCode]CorrectionRule, len(rules))
	for index, rule := range rules {
		if !validIdentity(string(rule.FindingCode)) || rule.FindingCode == FindingCodeNone ||
			!validIdentity(string(rule.ObjectiveKind)) ||
			!reflect.DeepEqual(rule.Acceptance, []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified}) {
			return CorrectionRuleRegistry{}, fmt.Errorf("%w: rule %d is invalid", ErrInvalidRuleRegistry, index)
		}
		if _, expected := issues[rule.FindingCode]; !expected {
			return CorrectionRuleRegistry{}, fmt.Errorf("%w: rule %q has no issue candidate", ErrInvalidRuleRegistry, rule.FindingCode)
		}
		if _, duplicate := registered[rule.FindingCode]; duplicate {
			return CorrectionRuleRegistry{}, fmt.Errorf("%w: rule %q is duplicated", ErrInvalidRuleRegistry, rule.FindingCode)
		}
		registered[rule.FindingCode] = cloneRule(rule)
	}
	if len(registered) != len(issues) {
		return CorrectionRuleRegistry{}, fmt.Errorf("%w: every issue requires exactly one rule", ErrInvalidRuleRegistry)
	}
	return CorrectionRuleRegistry{
		specificationID:     specification.ID,
		specificationDigest: specificationDigest(specification),
		rules:               registered,
	}, nil
}

func (registry CorrectionRuleRegistry) rule(code FindingCode) (CorrectionRule, error) {
	rule, exists := registry.rules[code]
	if !exists {
		return CorrectionRule{}, fmt.Errorf("%w: finding %q has no rule", ErrInvalidRuleRegistry, code)
	}
	return cloneRule(rule), nil
}

func (registry CorrectionRuleRegistry) validateFor(specification ReviewSpecification) error {
	if registry.rules == nil || registry.specificationID != specification.ID ||
		registry.specificationDigest != specificationDigest(specification) {
		return fmt.Errorf("%w: registry is not bound to the exact specification", ErrInvalidRuleRegistry)
	}
	return nil
}

func (registry CorrectionRuleRegistry) objectiveKinds() map[CorrectionObjectiveKind]struct{} {
	result := make(map[CorrectionObjectiveKind]struct{}, len(registry.rules))
	for _, rule := range registry.rules {
		result[rule.ObjectiveKind] = struct{}{}
	}
	return result
}

func cloneRule(value CorrectionRule) CorrectionRule {
	value.Acceptance = append([]CorrectionAcceptancePredicate{}, value.Acceptance...)
	return value
}
