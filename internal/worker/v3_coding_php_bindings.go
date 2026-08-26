package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type phpServiceFeatureBinding struct {
	Sequence         int
	RequirementID    string
	TaskID           string
	FeatureNumber    string
	Implementation   string
	Verification     string
	FeatureBlockID   string
	RendererBlockID  string
	SupportBlockID   string
	StateBlockID     string
	RouteBlockID     string
	AcceptanceID     string
	FixtureBlockID   string
	FeatureName      string
	StateClassName   string
	RouteName        string
	RendererName     string
	VerificationName string
	FixtureName      string
	AcceptanceCount  int
	HasEndpoint      bool
	Endpoint         assemblyline.ApplicationServiceEndpointContract
	Fixture          phpServiceInputFixture
}

func phpServiceFeatureBindings(
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	coverage assemblyline.ApplicationFileCoveragePlan,
	endpoints directCodingServiceEndpointPlan,
) ([]phpServiceFeatureBinding, map[string]phpServiceFeatureBinding, error) {
	taskByRequirement := make(map[string]assemblyline.FrozenApplicationTask, len(workload.Tasks))
	for _, task := range workload.Tasks {
		taskByRequirement[task.RequirementID] = task
	}
	bindings := make([]phpServiceFeatureBinding, 0, len(specification.Requirements))
	byRequirement := make(map[string]phpServiceFeatureBinding, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		task, exists := taskByRequirement[requirement.ID]
		if !exists {
			return nil, nil, fmt.Errorf("PHP HTTP workload omits requirement %s", requirement.ID)
		}
		pair, featureNumber, err := phpServiceTaskPair(coverage, task.ID)
		if err != nil {
			return nil, nil, err
		}
		endpointRequirement, exists := endpoints.Requirements[task.ID]
		if !exists {
			return nil, nil, fmt.Errorf("PHP HTTP endpoint plan omits requirement decision for task %s", task.ID)
		}
		endpoint, hasEndpoint := endpoints.ByTask[task.ID]
		if endpointRequirement == assemblyline.ApplicationServiceEndpointRequired && !hasEndpoint {
			return nil, nil, fmt.Errorf("PHP HTTP endpoint-required task %s lacks its contract", task.ID)
		}
		if endpointRequirement == assemblyline.ApplicationServiceSupportOnly && hasEndpoint {
			return nil, nil, fmt.Errorf("PHP HTTP support-only task %s has an endpoint contract", task.ID)
		}
		fixture, err := phpServiceTaskInputFixture(endpointRequirement, endpoint)
		if err != nil {
			return nil, nil, fmt.Errorf("PHP task %s input fixture: %w", task.ID, err)
		}
		binding := phpServiceFeatureBinding{
			Sequence: index + 1, RequirementID: requirement.ID, TaskID: task.ID,
			FeatureNumber: featureNumber, Implementation: pair.ImplementationPath,
			Verification:     pair.VerificationPath,
			FeatureBlockID:   fmt.Sprintf("feature.%03d", index+1),
			RendererBlockID:  fmt.Sprintf("representation.html.%03d", index+1),
			SupportBlockID:   fmt.Sprintf("feature.capabilities.%03d", index+1),
			StateBlockID:     fmt.Sprintf("feature.state.%03d", index+1),
			RouteBlockID:     fmt.Sprintf("runtime.route.%03d", index+1),
			AcceptanceID:     fmt.Sprintf("acceptance.%03d", index+1),
			FixtureBlockID:   fmt.Sprintf("acceptance.fixture.%03d", index+1),
			FeatureName:      "feature" + featureNumber,
			StateClassName:   "FeatureState" + featureNumber,
			RouteName:        "routeFeature" + featureNumber,
			RendererName:     "renderFeature" + featureNumber + "HTML",
			VerificationName: "verifyFeature" + featureNumber,
			FixtureName:      "taskInputFixture" + featureNumber,
			AcceptanceCount:  len(task.AcceptanceCriteria),
			HasEndpoint:      hasEndpoint,
			Endpoint:         endpoint,
			Fixture:          fixture,
		}
		bindings = append(bindings, binding)
		byRequirement[requirement.ID] = binding
	}
	return bindings, byRequirement, nil
}

func phpServiceCapabilityProjection(
	owner phpServiceFeatureBinding,
	dependencies []directCodingCapabilityBinding,
	byRequirement map[string]phpServiceFeatureBinding,
) (string, string, string, error) {
	if len(dependencies) == 0 {
		return "", "", "", nil
	}
	lines := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		provider, exists := byRequirement[dependency.RequirementID]
		if !exists {
			return "", "", "", fmt.Errorf("PHP HTTP capability names unknown requirement %s", dependency.RequirementID)
		}
		name := fmt.Sprintf(
			"FEATURE_%s_CAPABILITY_%s", owner.FeatureNumber, provider.FeatureNumber,
		)
		lines = append(lines, fmt.Sprintf("const %s = %s;", name, phpSingleQuoted(dependency.CapabilityID)))
	}
	declaration := strings.Join(lines, "\n")
	return owner.SupportBlockID, declaration, declaration, nil
}
