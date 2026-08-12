package semanticreview

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

type Machine struct {
	objective     Objective
	initial       Artifact
	specification ReviewSpecification
	rules         CorrectionRuleRegistry
	executors     CorrectionExecutorRegistry
	selector      cognitionreference.Selector
	verifier      Verifier
	limits        Limits
}

func NewMachine(
	objective Objective,
	initial Artifact,
	specification ReviewSpecification,
	rules CorrectionRuleRegistry,
	executors CorrectionExecutorRegistry,
	selector cognitionreference.Selector,
	verifier Verifier,
	limits Limits,
) (Machine, error) {
	if !validIdentity(string(objective.ID)) || objective.Status != ObjectivePending ||
		len(objective.Acceptance) != len(exactRootAcceptance) ||
		objective.Acceptance[0] != exactRootAcceptance[0] ||
		objective.Acceptance[1] != exactRootAcceptance[1] {
		return Machine{}, fmt.Errorf("%w: objective exceeds bounds", ErrInvalidObjective)
	}
	if err := validateArtifact(initial); err != nil {
		return Machine{}, err
	}
	if err := preflightSpecification(specification); err != nil {
		return Machine{}, err
	}
	objective = cloneObjective(objective)
	initial = cloneArtifact(initial)
	specification = cloneSpecification(specification)
	if err := validateObjective(objective); err != nil {
		return Machine{}, err
	}
	if err := validateArtifact(initial); err != nil {
		return Machine{}, err
	}
	if initial.RootObjectiveID != objective.ID || initial.Revision != 1 || initial.ParentID != "" {
		return Machine{}, fmt.Errorf("%w: initial artifact is not bound to the root objective", ErrInvalidMachine)
	}
	if err := validateSpecification(specification); err != nil {
		return Machine{}, err
	}
	if specification.ObjectiveID != objective.ID {
		return Machine{}, fmt.Errorf("%w: specification is not bound to the root objective", ErrInvalidMachine)
	}
	if err := rules.validateFor(specification); err != nil {
		return Machine{}, err
	}
	if err := executors.validateFor(rules); err != nil {
		return Machine{}, err
	}
	if nilInterface(selector) || nilInterface(verifier) {
		return Machine{}, fmt.Errorf("%w: selector and verifier are required", ErrInvalidMachine)
	}
	if limits.MaxReviewRounds < 1 || limits.MaxReviewRounds > maxReviewRounds {
		return Machine{}, fmt.Errorf("%w: review bound must be between 1 and %d", ErrInvalidMachine, maxReviewRounds)
	}
	return Machine{
		objective: objective, initial: initial, specification: specification,
		rules: cloneRuleRegistry(rules), executors: cloneExecutorRegistry(executors),
		selector: selector, verifier: verifier, limits: limits,
	}, nil
}

func cloneRuleRegistry(value CorrectionRuleRegistry) CorrectionRuleRegistry {
	result := CorrectionRuleRegistry{
		specificationID:     value.specificationID,
		specificationDigest: value.specificationDigest,
		rules:               make(map[FindingCode]CorrectionRule, len(value.rules)),
	}
	for code, rule := range value.rules {
		result.rules[code] = cloneRule(rule)
	}
	return result
}

func cloneExecutorRegistry(value CorrectionExecutorRegistry) CorrectionExecutorRegistry {
	result := CorrectionExecutorRegistry{
		ruleSpecificationID: value.ruleSpecificationID,
		ruleDigest:          value.ruleDigest,
		executors:           make(map[CorrectionObjectiveKind]CorrectionExecutor, len(value.executors)),
	}
	for kind, executor := range value.executors {
		result.executors[kind] = executor
	}
	return result
}
