package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericPHPServiceDocuments(
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	contexts map[string]assemblyline.ApplicationTaskContext,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
	endpoints directCodingServiceEndpointPlan,
	state directCodingServiceStatePlan,
	storage directCodingServiceStoragePlan,
) ([]assemblyline.SourceDocument, error) {
	bindings, byRequirement, err := phpServiceFeatureBindings(
		specification, workload, coverage, endpoints,
	)
	if err != nil {
		return nil, err
	}
	hasHTML := phpServiceHasHTMLResponse(endpoints)
	var routeBlocks []assemblyline.SourceBlock
	if hasHTML {
		routeBlocks, err = phpServiceRouteBlocks(bindings)
		if err != nil {
			return nil, err
		}
	}
	implementations := make([]assemblyline.SourceDocument, 0, len(bindings))
	verifications := make([]assemblyline.SourceDocument, 0, len(bindings))
	runnerDependencies := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		context, exists := contexts[binding.RequirementID]
		if !exists {
			return nil, fmt.Errorf("PHP HTTP workload omits requirement %s", binding.RequirementID)
		}
		requirementBehavior, err := compileDirectCodingApplicationTaskBehavior(
			context, capabilities[binding.RequirementID],
		)
		if err != nil {
			return nil, err
		}
		if binding.HasEndpoint {
			requirementBehavior += "\n" + phpServiceEndpointInputContract(binding.Endpoint)
		}
		implementationBehavior := requirementBehavior
		if skill, exists := skills[binding.RequirementID]; exists {
			implementationBehavior += "\nValidated procedure: " + skill.Procedure
		}
		supportID, supportSource, supportAPI, err := phpServiceCapabilityProjection(
			binding, capabilities[binding.RequirementID], byRequirement,
		)
		if err != nil {
			return nil, err
		}
		dependencies := []string{"runtime.task_input", "runtime.task_result"}
		capabilityDependencies := append([]string(nil), dependencies...)
		acceptanceDependencies := append([]string(nil), dependencies...)
		if storage.RequiresPostgreSQL() {
			// Focused stages share the project's Compose state artifacts. Retain
			// their code-owned runtime in every task without exposing it as a
			// generated-source capability to request-local tasks.
			dependencies = append(dependencies, "runtime.state")
		}
		blocks := make([]assemblyline.SourceBlock, 0, 4)
		if supportID != "" {
			blocks = append(blocks, assemblyline.SourceBlock{
				ID: supportID, Static: supportSource, API: supportAPI,
				TaskID: binding.TaskID,
				Role:   assemblyline.SourceBlockTaskSupport,
			})
			dependencies = append(dependencies, supportID)
			capabilityDependencies = append(capabilityDependencies, supportID)
			acceptanceDependencies = append(acceptanceDependencies, supportID)
		}
		taskStorage, err := storage.storageForTask(binding.TaskID)
		if err != nil {
			return nil, err
		}
		stateInterface, hasStateInterface, interfaceErr := state.interfaceForTask(binding.TaskID)
		if interfaceErr != nil {
			return nil, interfaceErr
		}
		if hasStateInterface {
			writable := taskStorage == directCodingServiceStoragePostgreSQL
			blocks = append(blocks, phpServiceStateFacadeBlock(
				binding, storage.Namespace, stateInterface, writable,
			))
			dependencies = append(dependencies, binding.StateBlockID)
			capabilityDependencies = append(capabilityDependencies, binding.StateBlockID)
		}
		featureSignature := fmt.Sprintf(
			"function %s(TaskInput $input, array $dependencies): TaskResult", binding.FeatureName,
		)
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: binding.FeatureBlockID, Signature: featureSignature,
			Contract: phpServiceFeatureContract(implementationBehavior), API: featureSignature,
			DependsOn: dependencies, Capabilities: capabilityDependencies,
			TaskID: binding.TaskID, Role: assemblyline.SourceBlockTaskImplementation,
		})
		if binding.Endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			routes, routeErr := phpServiceRendererRouteBindings(
				binding, capabilities[binding.RequirementID], byRequirement,
			)
			if routeErr != nil {
				return nil, routeErr
			}
			rendererSignature := fmt.Sprintf(
				"function %s(TaskResult $result): string", binding.RendererName,
			)
			rendererDependencies := []string{"runtime.task_result", "runtime.html"}
			for _, route := range routes {
				rendererDependencies = append(rendererDependencies, route.RouteBlockID)
			}
			blocks = append(blocks, assemblyline.SourceBlock{
				ID: binding.RendererBlockID, Signature: rendererSignature,
				Contract: phpServiceHTMLRepresentationContract(
					requirementBehavior, phpServiceRendererRouteContract(routes),
				), API: rendererSignature,
				DependsOn:    rendererDependencies,
				Capabilities: append([]string(nil), rendererDependencies...),
				TaskID:       binding.TaskID, Role: assemblyline.SourceBlockTaskRepresentation,
			})
		}
		implementations = append(implementations, assemblyline.SourceDocument{
			ID:   fmt.Sprintf("workload_implementation_%03d", binding.Sequence),
			Path: binding.Implementation, AdapterID: phpSourceAdapterID,
			Preamble: phpServiceImplementationPreamble(),
			Blocks:   blocks,
		})

		acceptanceDependencies = append(acceptanceDependencies, "runtime.assertions")
		acceptanceDependencies = append(acceptanceDependencies, binding.FeatureBlockID)
		acceptanceDependencies = append(acceptanceDependencies, binding.FixtureBlockID)
		verificationSignature := "function " + binding.VerificationName + "(): void"
		verifications = append(verifications, assemblyline.SourceDocument{
			ID:   fmt.Sprintf("workload_verification_%03d", binding.Sequence),
			Path: binding.Verification, AdapterID: phpSourceAdapterID,
			Preamble: phpServiceVerificationPreamble(binding.Implementation),
			Blocks: []assemblyline.SourceBlock{
				{
					ID: binding.FixtureBlockID, Static: phpServiceTaskInputFixtureSource(binding),
					API:       "function " + binding.FixtureName + "(): TaskInput",
					DependsOn: []string{"runtime.task_input"}, TaskID: binding.TaskID,
					Role: assemblyline.SourceBlockTaskSupport,
				},
				{
					ID: binding.AcceptanceID, Signature: verificationSignature,
					Contract: phpServiceAcceptanceContract(requirementBehavior, binding, capabilities[binding.RequirementID], byRequirement),
					API:      verificationSignature, DependsOn: acceptanceDependencies,
					Capabilities: append([]string(nil), acceptanceDependencies...),
					TaskID:       binding.TaskID, Role: assemblyline.SourceBlockTaskVerification,
				},
				{
					ID:        fmt.Sprintf("acceptance.execute.%03d", binding.Sequence),
					Static:    phpServiceFocusedVerificationSource(binding.VerificationName),
					API:       "execute the focused verification when this test file is the CLI entrypoint",
					DependsOn: []string{binding.AcceptanceID}, TaskID: binding.TaskID,
					Role: assemblyline.SourceBlockTaskSupport,
				},
			},
		})
		runnerDependencies = append(runnerDependencies, binding.AcceptanceID)
	}
	documents := []assemblyline.SourceDocument{
		phpServiceRuntimeDocument(
			hasHTML, storage.RequiresPostgreSQL(), routeBlocks,
		),
	}
	documents = append(documents, implementations...)
	documents = append(documents, verifications...)
	router, err := phpServiceRouterDocument(
		bindings, specification.Requirements, capabilities, byRequirement, state,
	)
	if err != nil {
		return nil, err
	}
	httpVerifier, err := phpServiceHTTPVerifierDocument(
		bindings, workload, capabilities, state,
	)
	if err != nil {
		return nil, err
	}
	documents = append(
		documents, router, phpServiceTestRunnerDocument(bindings, runnerDependencies, storage),
		httpVerifier,
	)
	return documents, nil
}

func phpServiceEndpointInputContract(contract assemblyline.ApplicationServiceEndpointContract) string {
	_ = contract
	return "Derive only the endpoint-independent TaskResult content from the supplied typed input."
}

func phpServiceFeatureContract(behavior string) string {
	return strings.Join([]string{
		behavior,
		"Return one TaskResult derived only from the typed TaskInput and declared direct dependency TaskResult values.",
		"Use TaskResult::success for observable output or state, or TaskResult::failure for an explicit failure.",
		"Do not declare helper functions, classes, imports, placeholders, or TODO behavior.",
	}, "\n")
}

func phpServiceHTMLRepresentationContract(behavior string, routes []string) string {
	contract := []string{
		behavior,
		"Return one server-rendered representation by calling RuntimeHtml::document with a concise semantic title and one body expression.",
		"Build the body from single-quoted static HTML fragments and only the registered RuntimeHtml helpers and route functions.",
		"A record collection may be traversed with RuntimeHtml::records; every record value inserted into markup must pass through RuntimeHtml::field and RuntimeHtml::escape.",
		"Use RuntimeHtml::formOpen and RuntimeHtml::formClose for forms; never construct an action URL directly.",
		"The body contains one main landmark, one level-one heading, accessible semantic elements, responsive Tailwind utility classes, and at least one escaped TaskResult field.",
	}
	contract = append(contract, routes...)
	return strings.Join(contract, "\n")
}

func phpServiceAcceptanceContract(
	behavior string,
	binding phpServiceFeatureBinding,
	dependencies []directCodingCapabilityBinding,
	byRequirement map[string]phpServiceFeatureBinding,
) string {
	dependencyNames := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		provider := byRequirement[dependency.RequirementID]
		dependencyNames = append(dependencyNames, fmt.Sprintf(
			"FEATURE_%s_CAPABILITY_%s", binding.FeatureNumber, provider.FeatureNumber,
		))
	}
	dependencyInstruction := "Pass an empty dependency array."
	if len(dependencyNames) > 0 {
		dependencyInstruction = "Pass representative TaskResult values keyed only by " + strings.Join(dependencyNames, ", ") + "."
	}
	return strings.Join([]string{
		behavior,
		"Call exactly " + binding.FeatureName + " with " + binding.FixtureName + "() and store its TaskResult in one local variable.",
		dependencyInstruction,
		fmt.Sprintf("Pass that variable to RuntimeAssertions::requireResult, then add at least %d distinct RuntimeAssertions::require conditions. Each condition must directly compare exactly one output, error, or state field from that result with a result-independent expected expression and include a clear failure message; do not cast or combine predicates.", binding.AcceptanceCount),
		"Do not catch failures, merely call the feature, or duplicate implementation logic.",
	}, "\n")
}

func phpServiceImplementationPreamble() string {
	return "<?php\ndeclare(strict_types=1);\n\nrequire_once __DIR__ . '/Runtime.php';"
}

func phpServiceVerificationPreamble(implementationPath string) string {
	base := strings.TrimPrefix(implementationPath, "src/")
	return "<?php\ndeclare(strict_types=1);\n\nrequire_once __DIR__ . '/../src/Runtime.php';\n" +
		"require_once __DIR__ . '/../src/" + base + "';"
}

func phpServiceFocusedVerificationSource(verificationName string) string {
	return "if (realpath((string) ($_SERVER['SCRIPT_FILENAME'] ?? '')) === __FILE__) {\n    " + verificationName + "();\n}"
}

func phpSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "'", "\\'") + "'"
}
