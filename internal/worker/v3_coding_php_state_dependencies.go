package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func phpServiceEndpointExecutionOrderWithState(
	target phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) ([]phpServiceFeatureBinding, error) {
	required := make(map[string]struct{})
	visiting := make(map[string]bool)
	var include func(string) error
	include = func(requirementID string) error {
		if _, exists := required[requirementID]; exists {
			return nil
		}
		if visiting[requirementID] {
			return fmt.Errorf("PHP HTTP capability graph contains a cycle at %s", requirementID)
		}
		owner, exists := byRequirement[requirementID]
		if !exists {
			return fmt.Errorf("PHP HTTP capability names unknown requirement %s", requirementID)
		}
		visiting[requirementID] = true
		for _, dependency := range capabilities[requirementID] {
			provider, exists := byRequirement[dependency.RequirementID]
			if !exists {
				return fmt.Errorf(
					"PHP HTTP capability names unknown requirement %s", dependency.RequirementID,
				)
			}
			persistent, err := phpServiceDependencyUsesSharedState(owner, provider, state)
			if err != nil {
				return err
			}
			if !persistent {
				if err := include(dependency.RequirementID); err != nil {
					return err
				}
			}
		}
		visiting[requirementID] = false
		required[requirementID] = struct{}{}
		return nil
	}
	if err := include(target.RequirementID); err != nil {
		return nil, err
	}
	order, err := goCommandLineRequirementOrder(requirements, capabilities)
	if err != nil {
		return nil, fmt.Errorf("order PHP HTTP endpoint capability execution: %w", err)
	}
	bindings := make([]phpServiceFeatureBinding, 0, len(required))
	for _, requirementID := range order {
		if _, exists := required[requirementID]; exists {
			bindings = append(bindings, byRequirement[requirementID])
		}
	}
	return bindings, nil
}

func phpServiceFeatureInvocationWithState(
	binding phpServiceFeatureBinding,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) (string, error) {
	var source strings.Builder
	source.WriteString(fmt.Sprintf("            $direct%s = [\n", binding.FeatureNumber))
	for _, dependency := range capabilities[binding.RequirementID] {
		provider, exists := byRequirement[dependency.RequirementID]
		if !exists {
			return "", fmt.Errorf(
				"PHP HTTP capability names unknown requirement %s", dependency.RequirementID,
			)
		}
		constant := fmt.Sprintf(
			"FEATURE_%s_CAPABILITY_%s", binding.FeatureNumber, provider.FeatureNumber,
		)
		value := "$results[" + phpSingleQuoted(dependency.CapabilityID) + "]"
		persistent, err := phpServiceDependencyUsesSharedState(binding, provider, state)
		if err != nil {
			return "", err
		}
		if persistent {
			value = "TaskResult::success('', " + binding.StateClassName + "::load())"
		}
		source.WriteString(fmt.Sprintf(
			"                %s => %s,\n", constant, value,
		))
	}
	source.WriteString("            ];\n")
	source.WriteString(fmt.Sprintf(
		"            $result%s = %s($input, $direct%s);\n",
		binding.FeatureNumber, binding.FeatureName, binding.FeatureNumber,
	))
	source.WriteString(fmt.Sprintf(
		"            $results[%s] = $result%s;\n",
		phpSingleQuoted(genericApplicationCapabilityID(binding.Sequence)), binding.FeatureNumber,
	))
	return source.String(), nil
}

func phpServiceDependencyUsesSharedState(
	owner, provider phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) (bool, error) {
	if state.WorkloadSHA256 == "" {
		return false, nil
	}
	providerLifetime, exists := state.ByTask[provider.TaskID]
	if !exists {
		return false, fmt.Errorf("service state plan omits capability provider task %s", provider.TaskID)
	}
	if _, exists := state.ByTask[owner.TaskID]; !exists {
		return false, fmt.Errorf("service state plan omits capability owner task %s", owner.TaskID)
	}
	if providerLifetime != assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired {
		return false, nil
	}
	ownerInterface, ownerExists := state.InterfaceByTask[owner.TaskID]
	providerInterface, providerExists := state.InterfaceByTask[provider.TaskID]
	if !ownerExists || !providerExists || ownerInterface != providerInterface {
		return false, nil
	}
	if owner.StateClassName == "" {
		return false, fmt.Errorf("shared-state capability owner %s has no state facade", owner.TaskID)
	}
	return true, nil
}
