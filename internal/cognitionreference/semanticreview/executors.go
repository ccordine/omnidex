package semanticreview

import (
	"fmt"
	"sort"
)

type CorrectionExecutorRegistry struct {
	ruleSpecificationID ReviewSpecificationID
	ruleDigest          string
	executors           map[CorrectionObjectiveKind]CorrectionExecutor
}

func NewCorrectionExecutorRegistry(
	rules CorrectionRuleRegistry,
	registrations []CorrectionExecutorRegistration,
) (CorrectionExecutorRegistry, error) {
	if rules.rules == nil {
		return CorrectionExecutorRegistry{}, fmt.Errorf("%w: constructed rule registry is required", ErrInvalidExecutorRegistry)
	}
	wanted := rules.objectiveKinds()
	if len(registrations) > maxCorrectionRules {
		return CorrectionExecutorRegistry{}, fmt.Errorf(
			"%w: registration count exceeds %d", ErrInvalidExecutorRegistry, maxCorrectionRules,
		)
	}
	for index, registration := range registrations {
		if !validIdentity(string(registration.ObjectiveKind)) || nilInterface(registration.Executor) {
			return CorrectionExecutorRegistry{}, fmt.Errorf("%w: registration %d exceeds bounds", ErrInvalidExecutorRegistry, index)
		}
	}
	registered := make(map[CorrectionObjectiveKind]CorrectionExecutor, len(registrations))
	for index, registration := range registrations {
		if !validIdentity(string(registration.ObjectiveKind)) || nilInterface(registration.Executor) {
			return CorrectionExecutorRegistry{}, fmt.Errorf("%w: registration %d is invalid", ErrInvalidExecutorRegistry, index)
		}
		if _, expected := wanted[registration.ObjectiveKind]; !expected {
			return CorrectionExecutorRegistry{}, fmt.Errorf("%w: executor kind %q has no rule", ErrInvalidExecutorRegistry, registration.ObjectiveKind)
		}
		if _, duplicate := registered[registration.ObjectiveKind]; duplicate {
			return CorrectionExecutorRegistry{}, fmt.Errorf("%w: executor kind %q is duplicated", ErrInvalidExecutorRegistry, registration.ObjectiveKind)
		}
		registered[registration.ObjectiveKind] = registration.Executor
	}
	if len(registered) != len(wanted) {
		return CorrectionExecutorRegistry{}, fmt.Errorf("%w: every rule kind requires exactly one executor", ErrInvalidExecutorRegistry)
	}
	return CorrectionExecutorRegistry{
		ruleSpecificationID: rules.specificationID,
		ruleDigest:          ruleRegistryDigest(rules),
		executors:           registered,
	}, nil
}

func (registry CorrectionExecutorRegistry) validateFor(rules CorrectionRuleRegistry) error {
	if registry.executors == nil || registry.ruleSpecificationID != rules.specificationID ||
		registry.ruleDigest != ruleRegistryDigest(rules) {
		return fmt.Errorf("%w: registry is not bound to the exact rule registry", ErrInvalidExecutorRegistry)
	}
	for kind, executor := range registry.executors {
		if nilInterface(executor) {
			return fmt.Errorf("%w: executor kind %q is nil", ErrInvalidExecutorRegistry, kind)
		}
	}
	return nil
}

func (registry CorrectionExecutorRegistry) executor(
	kind CorrectionObjectiveKind,
) (CorrectionExecutor, error) {
	executor, exists := registry.executors[kind]
	if !exists || nilInterface(executor) {
		return nil, fmt.Errorf("%w: objective kind %q has no executor", ErrInvalidExecutorRegistry, kind)
	}
	return executor, nil
}

func ruleRegistryDigest(registry CorrectionRuleRegistry) string {
	values := []string{string(registry.specificationID), registry.specificationDigest}
	keys := make([]string, 0, len(registry.rules))
	for code := range registry.rules {
		keys = append(keys, string(code))
	}
	sort.Strings(keys)
	for _, key := range keys {
		rule := registry.rules[FindingCode(key)]
		values = append(values, key, string(rule.ObjectiveKind), string(rule.Acceptance[0]))
	}
	return digestFields(values...)
}
