package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func phpServiceRouterDocument(
	bindings []phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) (assemblyline.SourceDocument, error) {
	routed := make([]phpServiceFeatureBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.HasEndpoint {
			routed = append(routed, binding)
		}
	}
	if len(routed) == 0 {
		return assemblyline.SourceDocument{}, fmt.Errorf("PHP HTTP router requires one endpoint-required task")
	}
	ordered := phpServiceRouteOrder(routed)
	source, err := phpServiceRouterSourceWithState(
		ordered, requirements, capabilities, byRequirement, state,
	)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	closure, err := phpServiceRouterClosureWithState(
		ordered, requirements, capabilities, byRequirement, state,
	)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	preamble := []string{"<?php", "declare(strict_types=1);", "", "require_once __DIR__ . '/../src/Runtime.php';"}
	dependencies := []string{"runtime.http"}
	for _, binding := range closure {
		preamble = append(preamble, fmt.Sprintf(
			"require_once __DIR__ . '/../src/Feature%s.php';", binding.FeatureNumber,
		))
		if len(capabilities[binding.RequirementID]) != 0 {
			dependencies = append(dependencies, binding.SupportBlockID)
		}
		dependencies = append(dependencies, binding.FeatureBlockID)
		if binding.Endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			dependencies = append(dependencies, binding.RendererBlockID)
		}
	}
	return assemblyline.SourceDocument{
		ID: "application_http_entrypoint", Path: "public/index.php", AdapterID: phpSourceAdapterID,
		Preamble: strings.Join(preamble, "\n"),
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.http", Static: source,
			API:       "function dispatchApplicationHttp(HttpRequest $request): HttpResponse",
			DependsOn: append([]string(nil), dependencies...),
		}},
	}, nil
}

func phpServiceRouterClosure(
	routed []phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
) ([]phpServiceFeatureBinding, error) {
	return phpServiceRouterClosureWithState(
		routed, requirements, capabilities, byRequirement, directCodingServiceStatePlan{},
	)
}

func phpServiceRouterClosureWithState(
	routed []phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) ([]phpServiceFeatureBinding, error) {
	needed := make(map[string]struct{})
	for _, binding := range routed {
		order, err := phpServiceEndpointExecutionOrderWithState(
			binding, requirements, capabilities, byRequirement, state,
		)
		if err != nil {
			return nil, err
		}
		for _, execution := range order {
			needed[execution.RequirementID] = struct{}{}
		}
	}
	closure := make([]phpServiceFeatureBinding, 0, len(needed))
	for _, requirement := range requirements {
		if _, exists := needed[requirement.ID]; exists {
			closure = append(closure, byRequirement[requirement.ID])
		}
	}
	return closure, nil
}

func phpServiceRouteOrder(bindings []phpServiceFeatureBinding) []phpServiceFeatureBinding {
	ordered := append([]phpServiceFeatureBinding(nil), bindings...)
	sort.SliceStable(ordered, func(left, right int) bool {
		leftLiteral, leftParameters := phpServiceRouteSpecificity(ordered[left].Endpoint.RouteTemplate)
		rightLiteral, rightParameters := phpServiceRouteSpecificity(ordered[right].Endpoint.RouteTemplate)
		if leftLiteral != rightLiteral {
			return leftLiteral > rightLiteral
		}
		if leftParameters != rightParameters {
			return leftParameters < rightParameters
		}
		return ordered[left].Sequence < ordered[right].Sequence
	})
	return ordered
}

func phpServiceRouteSpecificity(route string) (int, int) {
	literal, parameters := 0, 0
	for _, segment := range strings.Split(strings.Trim(route, "/"), "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			parameters++
		} else if segment != "" {
			literal++
		}
	}
	return literal, parameters
}

func phpServiceRouterSource(
	bindings []phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
) (string, error) {
	return phpServiceRouterSourceWithState(
		bindings, requirements, capabilities, byRequirement, directCodingServiceStatePlan{},
	)
}

func phpServiceRouterSourceWithState(
	bindings []phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) (string, error) {
	var source strings.Builder
	source.WriteString("function dispatchApplicationHttp(HttpRequest $request): HttpResponse\n")
	source.WriteString("{\n")
	source.WriteString("try {\n")
	source.WriteString("    if ($request->method === 'GET' && $request->path === '/__omnidex/health') {\n")
	source.WriteString("        return new HttpResponse(204, 'none', '');\n")
	source.WriteString("    }\n")
	source.WriteString("    $routeMatched = false;\n")
	for _, binding := range bindings {
		variable := "route" + binding.FeatureNumber
		source.WriteString(fmt.Sprintf(
			"    $%s = RuntimeHttp::matchRoute(%s, $request->path);\n",
			variable, phpSingleQuoted(binding.Endpoint.RouteTemplate),
		))
		source.WriteString(fmt.Sprintf("    if ($%s !== null) {\n", variable))
		source.WriteString("        $routeMatched = true;\n")
		source.WriteString(fmt.Sprintf(
			"        if ($request->method === %s) {\n", phpSingleQuoted(string(binding.Endpoint.Method)),
		))
		source.WriteString(fmt.Sprintf(
			"            RuntimeHttp::assertExposure(%s);\n",
			phpSingleQuoted(string(binding.Endpoint.Exposure)),
		))
		source.WriteString(fmt.Sprintf(
			"            $input = RuntimeHttp::taskInput($request, $%s, %s);\n",
			variable, phpSingleQuoted(string(binding.Endpoint.RequestMedia)),
		))
		source.WriteString("            $results = [];\n")
		executionOrder, err := phpServiceEndpointExecutionOrderWithState(
			binding, requirements, capabilities, byRequirement, state,
		)
		if err != nil {
			return "", err
		}
		for _, execution := range executionOrder {
			invocation, invocationErr := phpServiceFeatureInvocationWithState(
				execution, capabilities, byRequirement, state,
			)
			if invocationErr != nil {
				return "", invocationErr
			}
			source.WriteString(invocation)
		}
		if binding.Endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			source.WriteString(fmt.Sprintf(
				"            return RuntimeHttp::response($result%s, %s, %d, %s($result%s));\n",
				binding.FeatureNumber, phpSingleQuoted(string(binding.Endpoint.ResponseMedia)),
				binding.Endpoint.SuccessStatus, binding.RendererName, binding.FeatureNumber,
			))
		} else {
			source.WriteString(fmt.Sprintf(
				"            return RuntimeHttp::response($result%s, %s, %d);\n",
				binding.FeatureNumber, phpSingleQuoted(string(binding.Endpoint.ResponseMedia)),
				binding.Endpoint.SuccessStatus,
			))
		}
		source.WriteString("        }\n")
		source.WriteString("    }\n")
	}
	source.WriteString("    if ($routeMatched) {\n")
	source.WriteString("        throw new HttpFailure(405, 'HTTP method is not allowed for this endpoint.');\n")
	source.WriteString("    }\n")
	source.WriteString("    throw new HttpFailure(404, 'Endpoint not found.');\n")
	source.WriteString("} catch (Throwable $failure) {\n")
	source.WriteString("    return RuntimeHttp::failure($failure);\n")
	source.WriteString("}\n")
	source.WriteString("}\n")
	source.WriteString("\nif (PHP_SAPI === 'cli-server' || realpath((string) ($_SERVER['SCRIPT_FILENAME'] ?? '')) === __FILE__) {\n")
	source.WriteString("    try {\n")
	source.WriteString("        if (PHP_SAPI === 'cli-server' && RuntimeHttp::isStaticFileRequest($_SERVER, __DIR__)) {\n")
	source.WriteString("            return false;\n")
	source.WriteString("        }\n")
	source.WriteString("        $requestBody = file_get_contents('php://input');\n")
	source.WriteString("        if (!is_string($requestBody)) {\n")
	source.WriteString("            throw new HttpFailure(400, 'Request body could not be read.');\n")
	source.WriteString("        }\n")
	source.WriteString("        $request = RuntimeHttp::fromGlobals($_SERVER, $_GET, $_POST, $_FILES, $requestBody);\n")
	source.WriteString("        $response = dispatchApplicationHttp($request);\n")
	source.WriteString("    } catch (Throwable $failure) {\n")
	source.WriteString("        $response = RuntimeHttp::failure($failure);\n")
	source.WriteString("    }\n")
	source.WriteString("    RuntimeHttp::emit($response);\n")
	source.WriteString("}\n")
	return source.String(), nil
}
